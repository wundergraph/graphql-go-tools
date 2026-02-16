// Package weaviate implements the searchindex.Index interface for Weaviate.
//
// Priority: P1
// Supports: vector-native + BM25 full-text, native hybrid search.
// Filter translation: searchindex.Filter -> Weaviate where clause with operators.
//
// Uses only net/http + encoding/json (no Weaviate SDK).
package weaviate

import (
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/searchindex"
)

// Compile-time interface checks.
var (
	_ searchindex.Index        = (*Index)(nil)
	_ searchindex.IndexFactory = (*Factory)(nil)
)

// reservedTypeNameField stores the entity type name for identity reconstruction.
const reservedTypeNameField = "_typeName"

// reservedKeyFieldsField stores the JSON-encoded key fields map.
const reservedKeyFieldsField = "_keyFieldsJSON"

// reservedDocIDField stores the deterministic document ID string.
const reservedDocIDField = "_docId"

// Config holds Weaviate-specific configuration.
type Config struct {
	Host   string `json:"host"`
	Scheme string `json:"scheme,omitempty"`
	APIKey string `json:"api_key,omitempty"`
}

// Index implements searchindex.Index for Weaviate.
type Index struct {
	name      string
	className string
	config    Config
	schema    searchindex.IndexConfig
	client    *http.Client
	baseURL   string
}

// Factory implements searchindex.IndexFactory for Weaviate.
type Factory struct{}

// NewFactory returns a new Weaviate IndexFactory.
func NewFactory() *Factory {
	return &Factory{}
}

// CreateIndex creates a new Weaviate class with properties mapped from the IndexConfig.
func (f *Factory) CreateIndex(ctx context.Context, name string, schema searchindex.IndexConfig, configJSON []byte) (searchindex.Index, error) {
	var cfg Config
	if len(configJSON) > 0 {
		if err := json.Unmarshal(configJSON, &cfg); err != nil {
			return nil, fmt.Errorf("weaviate: invalid config: %w", err)
		}
	}
	if cfg.Host == "" {
		cfg.Host = "localhost:8080"
	}
	if cfg.Scheme == "" {
		cfg.Scheme = "http"
	}

	className := toClassName(name)
	baseURL := cfg.Scheme + "://" + cfg.Host

	idx := &Index{
		name:      name,
		className: className,
		config:    cfg,
		schema:    schema,
		client:    &http.Client{},
		baseURL:   baseURL,
	}

	// Build class definition.
	classDef, err := idx.buildClassDefinition()
	if err != nil {
		return nil, fmt.Errorf("weaviate: build class definition: %w", err)
	}

	body, err := json.Marshal(classDef)
	if err != nil {
		return nil, fmt.Errorf("weaviate: marshal class definition: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/v1/schema", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("weaviate: create schema request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	idx.setAuthHeader(req)

	resp, err := idx.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("weaviate: create class: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		respBody, _ := io.ReadAll(resp.Body)
		// If class already exists (422), treat as success.
		if resp.StatusCode == http.StatusUnprocessableEntity && strings.Contains(string(respBody), "already exists") {
			return idx, nil
		}
		return nil, fmt.Errorf("weaviate: create class failed (status %d): %s", resp.StatusCode, string(respBody))
	}

	return idx, nil
}

// buildClassDefinition builds the Weaviate class JSON body from IndexConfig.
func (idx *Index) buildClassDefinition() (map[string]any, error) {
	properties := []map[string]any{
		{
			"name":         reservedTypeNameField,
			"dataType":     []string{"text"},
			"tokenization": "field",
		},
		{
			"name":     reservedKeyFieldsField,
			"dataType": []string{"text"},
		},
		{
			"name":         reservedDocIDField,
			"dataType":     []string{"text"},
			"tokenization": "field",
		},
	}

	var vectorDimensions int

	for _, fc := range idx.schema.Fields {
		prop := fieldToProperty(fc)
		if prop != nil {
			properties = append(properties, prop)
		}
		if fc.Type == searchindex.FieldTypeVector && fc.Dimensions > 0 {
			vectorDimensions = fc.Dimensions
		}
	}

	classDef := map[string]any{
		"class":      idx.className,
		"properties": properties,
		"vectorizer": "none",
	}

	if vectorDimensions > 0 {
		classDef["vectorIndexConfig"] = map[string]any{
			"distance": "cosine",
		}
	}

	return classDef, nil
}

// fieldToProperty converts a FieldConfig to a Weaviate property definition.
// Returns nil for vector fields (handled via the class-level vectorizer).
func fieldToProperty(fc searchindex.FieldConfig) map[string]any {
	switch fc.Type {
	case searchindex.FieldTypeText:
		return map[string]any{
			"name":         fc.Name,
			"dataType":     []string{"text"},
			"tokenization": "word",
		}
	case searchindex.FieldTypeKeyword:
		return map[string]any{
			"name":         fc.Name,
			"dataType":     []string{"text"},
			"tokenization": "field",
		}
	case searchindex.FieldTypeNumeric:
		return map[string]any{
			"name":     fc.Name,
			"dataType": []string{"number"},
		}
	case searchindex.FieldTypeBool:
		return map[string]any{
			"name":     fc.Name,
			"dataType": []string{"boolean"},
		}
	case searchindex.FieldTypeVector:
		// Vectors are provided at object level, not as a property.
		return nil
	case searchindex.FieldTypeGeo:
		// Weaviate does not support geo fields yet.
		return nil
	case searchindex.FieldTypeDate, searchindex.FieldTypeDateTime:
		return map[string]any{
			"name":     fc.Name,
			"dataType": []string{"date"},
		}
	default:
		return nil
	}
}

// IndexDocument indexes a single document with upsert semantics.
// It first tries POST (create). If the object already exists (422), it falls
// back to PUT (update).
func (idx *Index) IndexDocument(ctx context.Context, doc searchindex.EntityDocument) error {
	obj, err := idx.buildObject(doc)
	if err != nil {
		return err
	}

	body, err := json.Marshal(obj)
	if err != nil {
		return fmt.Errorf("weaviate: marshal object: %w", err)
	}

	// Try POST first (create new object).
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, idx.baseURL+"/v1/objects", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("weaviate: create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	idx.setAuthHeader(req)

	resp, err := idx.client.Do(req)
	if err != nil {
		return fmt.Errorf("weaviate: index document: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusCreated {
		return nil
	}

	respBody, _ := io.ReadAll(resp.Body)

	// If already exists (422), fall back to PUT (update).
	if resp.StatusCode == http.StatusUnprocessableEntity && strings.Contains(string(respBody), "already exists") {
		id, _ := obj["id"].(string)
		putBody, _ := json.Marshal(obj)
		putReq, err := http.NewRequestWithContext(ctx, http.MethodPut, idx.baseURL+"/v1/objects/"+idx.className+"/"+id, bytes.NewReader(putBody))
		if err != nil {
			return fmt.Errorf("weaviate: create put request: %w", err)
		}
		putReq.Header.Set("Content-Type", "application/json")
		idx.setAuthHeader(putReq)

		putResp, err := idx.client.Do(putReq)
		if err != nil {
			return fmt.Errorf("weaviate: update document: %w", err)
		}
		defer putResp.Body.Close()

		if putResp.StatusCode != http.StatusOK && putResp.StatusCode != http.StatusNoContent {
			putRespBody, _ := io.ReadAll(putResp.Body)
			return fmt.Errorf("weaviate: update document failed (status %d): %s", putResp.StatusCode, string(putRespBody))
		}
		return nil
	}

	return fmt.Errorf("weaviate: index document failed (status %d): %s", resp.StatusCode, string(respBody))
}

// IndexDocuments indexes a batch of documents via the batch API.
func (idx *Index) IndexDocuments(ctx context.Context, docs []searchindex.EntityDocument) error {
	if len(docs) == 0 {
		return nil
	}
	if len(docs) == 1 {
		return idx.IndexDocument(ctx, docs[0])
	}

	objects := make([]map[string]any, 0, len(docs))
	for _, doc := range docs {
		obj, err := idx.buildObject(doc)
		if err != nil {
			return err
		}
		// For batch, we need to add the class field.
		obj["class"] = idx.className
		objects = append(objects, obj)
	}

	batchBody := map[string]any{
		"objects": objects,
	}

	body, err := json.Marshal(batchBody)
	if err != nil {
		return fmt.Errorf("weaviate: marshal batch: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, idx.baseURL+"/v1/batch/objects", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("weaviate: create batch request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	idx.setAuthHeader(req)

	resp, err := idx.client.Do(req)
	if err != nil {
		return fmt.Errorf("weaviate: batch index: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("weaviate: read batch response: %w", err)
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("weaviate: batch index failed (status %d): %s", resp.StatusCode, string(respBody))
	}

	// Check individual results for errors.
	var batchResults []struct {
		Result struct {
			Errors *struct {
				Error []struct {
					Message string `json:"message"`
				} `json:"error"`
			} `json:"errors"`
		} `json:"result"`
	}

	if err := json.Unmarshal(respBody, &batchResults); err != nil {
		// Not all responses are parseable; if the status was OK, accept it.
		return nil
	}

	for i, r := range batchResults {
		if r.Result.Errors != nil && len(r.Result.Errors.Error) > 0 {
			return fmt.Errorf("weaviate: batch object %d error: %s", i, r.Result.Errors.Error[0].Message)
		}
	}

	return nil
}

// buildObject converts an EntityDocument to a Weaviate object for indexing.
func (idx *Index) buildObject(doc searchindex.EntityDocument) (map[string]any, error) {
	docIDStr := documentIDString(doc.Identity)
	id := deterministicUUID(docIDStr)

	keyFieldsJSON, err := json.Marshal(doc.Identity.KeyFields)
	if err != nil {
		return nil, fmt.Errorf("weaviate: marshal key fields: %w", err)
	}

	properties := make(map[string]any, len(doc.Fields)+3)
	for k, v := range doc.Fields {
		properties[k] = v
	}
	// Weaviate requires all date properties to be RFC 3339 formatted.
	// Normalize date-only strings (e.g. "2024-01-15") to RFC 3339.
	dateFields := idx.dateFieldSet()
	for name := range dateFields {
		if s, ok := properties[name].(string); ok {
			properties[name] = normalizeDateToRFC3339(s)
		}
	}
	properties[reservedTypeNameField] = doc.Identity.TypeName
	properties[reservedKeyFieldsField] = string(keyFieldsJSON)
	properties[reservedDocIDField] = docIDStr

	obj := map[string]any{
		"id":         id,
		"class":      idx.className,
		"properties": properties,
	}

	// If there are vectors, use the first one as the object vector.
	if len(doc.Vectors) > 0 {
		for _, vec := range doc.Vectors {
			obj["vector"] = vec
			break
		}
	}

	return obj, nil
}

// dateFieldSet returns the set of field names that are DATE or DATETIME type.
func (idx *Index) dateFieldSet() map[string]bool {
	m := make(map[string]bool)
	for _, fc := range idx.schema.Fields {
		if fc.Type == searchindex.FieldTypeDate || fc.Type == searchindex.FieldTypeDateTime {
			m[fc.Name] = true
		}
	}
	return m
}

// normalizeDateToRFC3339 ensures a date string is in RFC 3339 format.
// Date-only strings like "2024-01-15" are converted to "2024-01-15T00:00:00Z".
// Already RFC 3339 strings are returned as-is.
func normalizeDateToRFC3339(s string) string {
	// Already RFC 3339
	if _, err := time.Parse(time.RFC3339, s); err == nil {
		return s
	}
	if _, err := time.Parse(time.RFC3339Nano, s); err == nil {
		return s
	}
	// Date-only: append time component
	if t, err := time.Parse(time.DateOnly, s); err == nil {
		return t.UTC().Format(time.RFC3339)
	}
	return s
}

// DeleteDocument deletes a single document by its deterministic ID.
func (idx *Index) DeleteDocument(ctx context.Context, id searchindex.DocumentIdentity) error {
	docIDStr := documentIDString(id)
	uuid := deterministicUUID(docIDStr)

	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, idx.baseURL+"/v1/objects/"+idx.className+"/"+uuid, nil)
	if err != nil {
		return fmt.Errorf("weaviate: create delete request: %w", err)
	}
	idx.setAuthHeader(req)

	resp, err := idx.client.Do(req)
	if err != nil {
		return fmt.Errorf("weaviate: delete document: %w", err)
	}
	defer resp.Body.Close()

	// 204 No Content is success; 404 means already gone.
	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusNotFound {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("weaviate: delete document failed (status %d): %s", resp.StatusCode, string(respBody))
	}

	return nil
}

// DeleteDocuments deletes a batch of documents by identity.
func (idx *Index) DeleteDocuments(ctx context.Context, ids []searchindex.DocumentIdentity) error {
	// Use batch delete with a where filter matching _docId values.
	if len(ids) == 0 {
		return nil
	}
	if len(ids) == 1 {
		return idx.DeleteDocument(ctx, ids[0])
	}

	// Delete one by one since batch delete by multiple IDs requires
	// constructing an OR filter on _docId, which is straightforward.
	operands := make([]map[string]any, 0, len(ids))
	for _, id := range ids {
		docIDStr := documentIDString(id)
		operands = append(operands, map[string]any{
			"path":        []string{reservedDocIDField},
			"operator":    "Equal",
			"valueText": docIDStr,
		})
	}

	var whereFilter map[string]any
	if len(operands) == 1 {
		whereFilter = operands[0]
	} else {
		whereFilter = map[string]any{
			"operator": "Or",
			"operands": operands,
		}
	}

	batchDeleteBody := map[string]any{
		"match": map[string]any{
			"class": idx.className,
			"where": whereFilter,
		},
		"output": "minimal",
	}

	body, err := json.Marshal(batchDeleteBody)
	if err != nil {
		return fmt.Errorf("weaviate: marshal batch delete: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, idx.baseURL+"/v1/batch/objects", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("weaviate: create batch delete request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	idx.setAuthHeader(req)

	resp, err := idx.client.Do(req)
	if err != nil {
		return fmt.Errorf("weaviate: batch delete: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("weaviate: batch delete failed (status %d): %s", resp.StatusCode, string(respBody))
	}

	return nil
}

// Search performs a search query against Weaviate using its GraphQL API.
func (idx *Index) Search(ctx context.Context, req searchindex.SearchRequest) (*searchindex.SearchResult, error) {
	gqlQuery := idx.buildGraphQLQuery(req)

	body, err := json.Marshal(map[string]string{
		"query": gqlQuery,
	})
	if err != nil {
		return nil, fmt.Errorf("weaviate: marshal graphql query: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, idx.baseURL+"/v1/graphql", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("weaviate: create graphql request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	idx.setAuthHeader(httpReq)

	resp, err := idx.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("weaviate: graphql request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("weaviate: read graphql response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("weaviate: graphql request failed (status %d): %s", resp.StatusCode, string(respBody))
	}

	return idx.parseGraphQLResponse(respBody)
}

// Autocomplete is not supported by Weaviate — it has no term dictionary API.
func (idx *Index) Autocomplete(_ context.Context, _ searchindex.AutocompleteRequest) (*searchindex.AutocompleteResult, error) {
	return nil, fmt.Errorf("weaviate: autocomplete is not supported")
}

// Close releases resources held by the index. No persistent connection to close.
func (idx *Index) Close() error {
	return nil
}

// buildGraphQLQuery constructs the Weaviate GraphQL query string from a SearchRequest.
func (idx *Index) buildGraphQLQuery(req searchindex.SearchRequest) string {
	var b strings.Builder
	b.WriteString("{ Get { ")
	b.WriteString(idx.className)

	// Build arguments.
	var args []string

	// Search operator.
	hasText := req.TextQuery != ""
	hasVector := len(req.Vector) > 0

	if hasText && hasVector {
		// Hybrid search.
		vectorStr := formatVector(req.Vector)
		args = append(args, fmt.Sprintf("hybrid: {query: %s, vector: %s}", quoteString(req.TextQuery), vectorStr))
	} else if hasText {
		// BM25 text search.
		bm25Arg := fmt.Sprintf("bm25: {query: %s", quoteString(req.TextQuery))
		if len(req.TextFields) > 0 {
			props := make([]string, len(req.TextFields))
			for i, tf := range req.TextFields {
				if tf.Weight != 0 && tf.Weight != 1.0 {
					props[i] = fmt.Sprintf("%s^%g", tf.Name, tf.Weight)
				} else {
					props[i] = tf.Name
				}
			}
			bm25Arg += fmt.Sprintf(", properties: [%s]", quoteStringSlice(props))
		}
		bm25Arg += "}"
		args = append(args, bm25Arg)
	} else if hasVector {
		// Vector search.
		vectorStr := formatVector(req.Vector)
		args = append(args, fmt.Sprintf("nearVector: {vector: %s}", vectorStr))
	}

	// Where filter.
	whereClause := idx.buildWhereClause(req)
	if whereClause != "" {
		args = append(args, "where: "+whereClause)
	}

	// Sort. Weaviate does not support sort with BM25 or hybrid search operators;
	// only include sort for plain object fetches (no text or vector search).
	if len(req.Sort) > 0 && !hasText && !hasVector {
		sortArgs := make([]string, 0, len(req.Sort))
		for _, sf := range req.Sort {
			order := "desc"
			if sf.Ascending {
				order = "asc"
			}
			sortArgs = append(sortArgs, fmt.Sprintf("{path: [%s], order: %s}", quoteString(sf.Field), order))
		}
		args = append(args, fmt.Sprintf("sort: [%s]", strings.Join(sortArgs, ", ")))
	}

	// Limit.
	limit := effectiveLimit(req.Limit)
	args = append(args, fmt.Sprintf("limit: %d", limit))

	// Offset.
	if req.Offset > 0 {
		args = append(args, fmt.Sprintf("offset: %d", req.Offset))
	}

	if len(args) > 0 {
		b.WriteString("(")
		b.WriteString(strings.Join(args, ", "))
		b.WriteString(")")
	}

	// Fields to return.
	b.WriteString(" { ")

	// Request all schema fields plus reserved fields.
	fieldNames := make([]string, 0, len(idx.schema.Fields)+3)
	for _, fc := range idx.schema.Fields {
		if fc.Type == searchindex.FieldTypeVector {
			continue // Vectors are not returned as properties.
		}
		fieldNames = append(fieldNames, fc.Name)
	}
	fieldNames = append(fieldNames, reservedTypeNameField, reservedKeyFieldsField, reservedDocIDField)
	b.WriteString(strings.Join(fieldNames, " "))

	// Additional fields.
	b.WriteString(" _additional { id score distance }")

	b.WriteString(" } ")
	b.WriteString("} }")

	return b.String()
}

// buildWhereClause constructs the Weaviate where filter from the SearchRequest.
func (idx *Index) buildWhereClause(req searchindex.SearchRequest) string {
	var parts []string

	// TypeName filter.
	if req.TypeName != "" {
		parts = append(parts, fmt.Sprintf("{path: [%s], operator: Equal, valueText: %s}",
			quoteString(reservedTypeNameField), quoteString(req.TypeName)))
	}

	// Structured filter.
	if req.Filter != nil {
		filterStr := translateFilter(req.Filter)
		if filterStr != "" {
			parts = append(parts, filterStr)
		}
	}

	if len(parts) == 0 {
		return ""
	}
	if len(parts) == 1 {
		return parts[0]
	}

	// Combine with AND.
	return fmt.Sprintf("{operator: And, operands: [%s]}", strings.Join(parts, ", "))
}

// translateFilter recursively converts a searchindex.Filter to a Weaviate where clause string.
func translateFilter(f *searchindex.Filter) string {
	if f == nil {
		return ""
	}

	// AND
	if len(f.And) > 0 {
		children := make([]string, 0, len(f.And))
		for _, child := range f.And {
			c := translateFilter(child)
			if c != "" {
				children = append(children, c)
			}
		}
		if len(children) == 0 {
			return ""
		}
		if len(children) == 1 {
			return children[0]
		}
		return fmt.Sprintf("{operator: And, operands: [%s]}", strings.Join(children, ", "))
	}

	// OR
	if len(f.Or) > 0 {
		children := make([]string, 0, len(f.Or))
		for _, child := range f.Or {
			c := translateFilter(child)
			if c != "" {
				children = append(children, c)
			}
		}
		if len(children) == 0 {
			return ""
		}
		if len(children) == 1 {
			return children[0]
		}
		return fmt.Sprintf("{operator: Or, operands: [%s]}", strings.Join(children, ", "))
	}

	// NOT – Weaviate does not support a "Not" operator. We negate the inner
	// filter directly: Term → NotEqual, Bool Term → inverted value, And/Or →
	// De Morgan's law (NOT(A AND B) = NOT A OR NOT B, NOT(A OR B) = NOT A AND NOT B).
	if f.Not != nil {
		return translateNegatedFilter(f.Not)
	}

	// Term
	if f.Term != nil {
		return translateTermFilter(f.Term)
	}

	// Terms (IN) - expressed as OR of Equal conditions.
	if f.Terms != nil {
		return translateTermsFilter(f.Terms)
	}

	// Range
	if f.Range != nil {
		return translateRangeFilter(f.Range)
	}

	// Prefix
	if f.Prefix != nil {
		return fmt.Sprintf("{path: [%s], operator: Like, valueText: %s}",
			quoteString(f.Prefix.Field), quoteString(f.Prefix.Value+"*"))
	}

	// Exists - use IsNull: false.
	if f.Exists != nil {
		return fmt.Sprintf("{path: [%s], operator: IsNull, valueBoolean: false}",
			quoteString(f.Exists.Field))
	}

	return ""
}

// translateNegatedFilter converts a filter into its negation without using the
// Weaviate "Not" operator (which is unsupported). Leaf filters are negated
// directly (Equal → NotEqual, bool values inverted), and compound filters use
// De Morgan's law.
func translateNegatedFilter(f *searchindex.Filter) string {
	if f == nil {
		return ""
	}

	// De Morgan: NOT(A AND B) = NOT(A) OR NOT(B)
	if len(f.And) > 0 {
		children := make([]string, 0, len(f.And))
		for _, child := range f.And {
			c := translateNegatedFilter(child)
			if c != "" {
				children = append(children, c)
			}
		}
		if len(children) == 0 {
			return ""
		}
		if len(children) == 1 {
			return children[0]
		}
		return fmt.Sprintf("{operator: Or, operands: [%s]}", strings.Join(children, ", "))
	}

	// De Morgan: NOT(A OR B) = NOT(A) AND NOT(B)
	if len(f.Or) > 0 {
		children := make([]string, 0, len(f.Or))
		for _, child := range f.Or {
			c := translateNegatedFilter(child)
			if c != "" {
				children = append(children, c)
			}
		}
		if len(children) == 0 {
			return ""
		}
		if len(children) == 1 {
			return children[0]
		}
		return fmt.Sprintf("{operator: And, operands: [%s]}", strings.Join(children, ", "))
	}

	// Double negation: NOT(NOT(x)) = x
	if f.Not != nil {
		return translateFilter(f.Not)
	}

	// Term: negate the equality check.
	if f.Term != nil {
		return translateNegatedTermFilter(f.Term)
	}

	// Terms (IN): NOT(a IN [x,y]) = a != x AND a != y
	if f.Terms != nil {
		parts := make([]string, 0, len(f.Terms.Values))
		for _, val := range f.Terms.Values {
			p := translateNegatedTermFilter(&searchindex.TermFilter{
				Field: f.Terms.Field,
				Value: val,
			})
			if p != "" {
				parts = append(parts, p)
			}
		}
		if len(parts) == 0 {
			return ""
		}
		if len(parts) == 1 {
			return parts[0]
		}
		return fmt.Sprintf("{operator: And, operands: [%s]}", strings.Join(parts, ", "))
	}

	// Range: negate by inverting bounds. NOT(x >= a AND x <= b) = x < a OR x > b.
	// For simplicity, apply De Morgan on the range parts.
	if f.Range != nil {
		var parts []string
		rf := f.Range
		if rf.GTE != nil {
			parts = append(parts, fmt.Sprintf("{path: [%s], operator: LessThan, %s}",
				quoteString(rf.Field), rangeValueClause(rf.GTE)))
		} else if rf.HasGT && rf.GT != nil {
			parts = append(parts, fmt.Sprintf("{path: [%s], operator: LessThanEqual, %s}",
				quoteString(rf.Field), rangeValueClause(rf.GT)))
		}
		if rf.LTE != nil {
			parts = append(parts, fmt.Sprintf("{path: [%s], operator: GreaterThan, %s}",
				quoteString(rf.Field), rangeValueClause(rf.LTE)))
		} else if rf.HasLT && rf.LT != nil {
			parts = append(parts, fmt.Sprintf("{path: [%s], operator: GreaterThanEqual, %s}",
				quoteString(rf.Field), rangeValueClause(rf.LT)))
		}
		if len(parts) == 0 {
			return ""
		}
		if len(parts) == 1 {
			return parts[0]
		}
		return fmt.Sprintf("{operator: Or, operands: [%s]}", strings.Join(parts, ", "))
	}

	// Prefix: NOT(LIKE "foo*") — no clean inverse in Weaviate. Fall back to
	// wrapping in a GraphQL-level workaround. For now, not supported.
	// Exists: NOT(IsNull: false) → IsNull: true
	if f.Exists != nil {
		return fmt.Sprintf("{path: [%s], operator: IsNull, valueBoolean: true}",
			quoteString(f.Exists.Field))
	}

	return ""
}

// translateNegatedTermFilter converts a TermFilter to a Weaviate where clause
// with a NotEqual operator instead of Equal, effectively negating the match.
// For boolean values, it inverts the value instead (since NotEqual on booleans
// can be unintuitive in some backends).
func translateNegatedTermFilter(tf *searchindex.TermFilter) string {
	switch v := tf.Value.(type) {
	case bool:
		// Negate by flipping the boolean value with Equal operator.
		return fmt.Sprintf("{path: [%s], operator: Equal, valueBoolean: %t}",
			quoteString(tf.Field), !v)
	case string:
		return fmt.Sprintf("{path: [%s], operator: NotEqual, valueText: %s}",
			quoteString(tf.Field), quoteString(v))
	case float64:
		return fmt.Sprintf("{path: [%s], operator: NotEqual, valueNumber: %s}",
			quoteString(tf.Field), formatFloat(v))
	case float32:
		return fmt.Sprintf("{path: [%s], operator: NotEqual, valueNumber: %s}",
			quoteString(tf.Field), formatFloat(float64(v)))
	case int:
		return fmt.Sprintf("{path: [%s], operator: NotEqual, valueInt: %d}",
			quoteString(tf.Field), v)
	case int64:
		return fmt.Sprintf("{path: [%s], operator: NotEqual, valueInt: %d}",
			quoteString(tf.Field), v)
	case json.Number:
		if i, err := v.Int64(); err == nil {
			return fmt.Sprintf("{path: [%s], operator: NotEqual, valueInt: %d}",
				quoteString(tf.Field), i)
		}
		if f, err := v.Float64(); err == nil {
			return fmt.Sprintf("{path: [%s], operator: NotEqual, valueNumber: %s}",
				quoteString(tf.Field), formatFloat(f))
		}
		return fmt.Sprintf("{path: [%s], operator: NotEqual, valueText: %s}",
			quoteString(tf.Field), quoteString(v.String()))
	default:
		return fmt.Sprintf("{path: [%s], operator: NotEqual, valueText: %s}",
			quoteString(tf.Field), quoteString(fmt.Sprintf("%v", v)))
	}
}

// translateTermFilter converts a TermFilter to a Weaviate where clause.
func translateTermFilter(tf *searchindex.TermFilter) string {
	switch v := tf.Value.(type) {
	case string:
		return fmt.Sprintf("{path: [%s], operator: Equal, valueText: %s}",
			quoteString(tf.Field), quoteString(v))
	case float64:
		return fmt.Sprintf("{path: [%s], operator: Equal, valueNumber: %s}",
			quoteString(tf.Field), formatFloat(v))
	case float32:
		return fmt.Sprintf("{path: [%s], operator: Equal, valueNumber: %s}",
			quoteString(tf.Field), formatFloat(float64(v)))
	case int:
		return fmt.Sprintf("{path: [%s], operator: Equal, valueInt: %d}",
			quoteString(tf.Field), v)
	case int64:
		return fmt.Sprintf("{path: [%s], operator: Equal, valueInt: %d}",
			quoteString(tf.Field), v)
	case bool:
		return fmt.Sprintf("{path: [%s], operator: Equal, valueBoolean: %t}",
			quoteString(tf.Field), v)
	case json.Number:
		// Try int first, then float.
		if i, err := v.Int64(); err == nil {
			return fmt.Sprintf("{path: [%s], operator: Equal, valueInt: %d}",
				quoteString(tf.Field), i)
		}
		if f, err := v.Float64(); err == nil {
			return fmt.Sprintf("{path: [%s], operator: Equal, valueNumber: %s}",
				quoteString(tf.Field), formatFloat(f))
		}
		return fmt.Sprintf("{path: [%s], operator: Equal, valueText: %s}",
			quoteString(tf.Field), quoteString(v.String()))
	default:
		return fmt.Sprintf("{path: [%s], operator: Equal, valueText: %s}",
			quoteString(tf.Field), quoteString(fmt.Sprintf("%v", v)))
	}
}

// translateTermsFilter converts a TermsFilter (IN) to a Weaviate where clause.
func translateTermsFilter(tf *searchindex.TermsFilter) string {
	if len(tf.Values) == 0 {
		return ""
	}
	parts := make([]string, 0, len(tf.Values))
	for _, val := range tf.Values {
		p := translateTermFilter(&searchindex.TermFilter{
			Field: tf.Field,
			Value: val,
		})
		if p != "" {
			parts = append(parts, p)
		}
	}
	if len(parts) == 0 {
		return ""
	}
	if len(parts) == 1 {
		return parts[0]
	}
	return fmt.Sprintf("{operator: Or, operands: [%s]}", strings.Join(parts, ", "))
}

// rangeValueClause returns the value clause fragment for a Weaviate where filter.
// String values (from date filters) use valueDate; numeric values use valueNumber.
func rangeValueClause(v any) string {
	if s, ok := v.(string); ok {
		return fmt.Sprintf("valueDate: %s", quoteString(normalizeDateToRFC3339(s)))
	}
	return fmt.Sprintf("valueNumber: %s", formatAnyNumber(v))
}

// translateRangeFilter converts a RangeFilter to Weaviate where clause(s).
func translateRangeFilter(rf *searchindex.RangeFilter) string {
	var parts []string

	if rf.GTE != nil {
		parts = append(parts, fmt.Sprintf("{path: [%s], operator: GreaterThanEqual, %s}",
			quoteString(rf.Field), rangeValueClause(rf.GTE)))
	} else if rf.HasGT && rf.GT != nil {
		parts = append(parts, fmt.Sprintf("{path: [%s], operator: GreaterThan, %s}",
			quoteString(rf.Field), rangeValueClause(rf.GT)))
	}

	if rf.LTE != nil {
		parts = append(parts, fmt.Sprintf("{path: [%s], operator: LessThanEqual, %s}",
			quoteString(rf.Field), rangeValueClause(rf.LTE)))
	} else if rf.HasLT && rf.LT != nil {
		parts = append(parts, fmt.Sprintf("{path: [%s], operator: LessThan, %s}",
			quoteString(rf.Field), rangeValueClause(rf.LT)))
	}

	if len(parts) == 0 {
		return ""
	}
	if len(parts) == 1 {
		return parts[0]
	}
	return fmt.Sprintf("{operator: And, operands: [%s]}", strings.Join(parts, ", "))
}

// parseGraphQLResponse parses the Weaviate GraphQL response into a SearchResult.
func (idx *Index) parseGraphQLResponse(body []byte) (*searchindex.SearchResult, error) {
	var gqlResp struct {
		Data   map[string]map[string][]json.RawMessage `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}

	if err := json.Unmarshal(body, &gqlResp); err != nil {
		return nil, fmt.Errorf("weaviate: parse graphql response: %w", err)
	}

	if len(gqlResp.Errors) > 0 {
		return nil, fmt.Errorf("weaviate: graphql error: %s", gqlResp.Errors[0].Message)
	}

	getResult, ok := gqlResp.Data["Get"]
	if !ok {
		return &searchindex.SearchResult{}, nil
	}

	classResults, ok := getResult[idx.className]
	if !ok {
		return &searchindex.SearchResult{}, nil
	}

	hits := make([]searchindex.SearchHit, 0, len(classResults))
	for _, raw := range classResults {
		var obj map[string]any
		if err := json.Unmarshal(raw, &obj); err != nil {
			return nil, fmt.Errorf("weaviate: parse result object: %w", err)
		}

		hit, err := idx.convertHit(obj)
		if err != nil {
			return nil, err
		}
		hits = append(hits, hit)
	}

	return &searchindex.SearchResult{
		Hits:       hits,
		TotalCount: len(hits),
	}, nil
}

// convertHit converts a single Weaviate result object to a SearchHit.
func (idx *Index) convertHit(obj map[string]any) (searchindex.SearchHit, error) {
	identity, err := extractIdentity(obj)
	if err != nil {
		return searchindex.SearchHit{}, err
	}

	// Build representation from fields, excluding internal fields.
	representation := make(map[string]any)
	for k, v := range obj {
		if k == reservedTypeNameField || k == reservedKeyFieldsField || k == reservedDocIDField || k == "_additional" {
			continue
		}
		representation[k] = v
	}
	representation["__typename"] = identity.TypeName
	for k, v := range identity.KeyFields {
		representation[k] = v
	}

	var score float64
	var distance float64

	if additional, ok := obj["_additional"].(map[string]any); ok {
		if s, ok := additional["score"]; ok {
			score = toFloat64Safe(s)
		}
		if d, ok := additional["distance"]; ok {
			distance = toFloat64Safe(d)
		}
	}

	return searchindex.SearchHit{
		Identity:       identity,
		Score:          score,
		Distance:       distance,
		Representation: representation,
	}, nil
}

// extractIdentity reconstructs a DocumentIdentity from stored fields.
func extractIdentity(obj map[string]any) (searchindex.DocumentIdentity, error) {
	typeName, _ := obj[reservedTypeNameField].(string)
	keyFieldsRaw, _ := obj[reservedKeyFieldsField].(string)

	var keyFields map[string]any
	if keyFieldsRaw != "" {
		if err := json.Unmarshal([]byte(keyFieldsRaw), &keyFields); err != nil {
			return searchindex.DocumentIdentity{}, fmt.Errorf("weaviate: failed to unmarshal key fields: %w", err)
		}
	}
	if keyFields == nil {
		keyFields = make(map[string]any)
	}

	return searchindex.DocumentIdentity{
		TypeName:  typeName,
		KeyFields: keyFields,
	}, nil
}

// setAuthHeader sets the Authorization header if an API key is configured.
func (idx *Index) setAuthHeader(req *http.Request) {
	if idx.config.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+idx.config.APIKey)
	}
}

// documentIDString computes a deterministic string ID from a DocumentIdentity.
// Format: TypeName:key1=val1,key2=val2,... (keys sorted alphabetically).
func documentIDString(id searchindex.DocumentIdentity) string {
	if len(id.KeyFields) == 0 {
		return id.TypeName
	}
	keys := make([]string, 0, len(id.KeyFields))
	for k := range id.KeyFields {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var b strings.Builder
	b.WriteString(id.TypeName)
	b.WriteByte(':')
	for i, k := range keys {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(k)
		b.WriteByte('=')
		fmt.Fprintf(&b, "%v", id.KeyFields[k])
	}
	return b.String()
}

// deterministicUUID generates a UUID v5 using SHA-1 from the given name string.
// Uses the DNS namespace UUID as the base (6ba7b810-9dad-11d1-80b4-00c04fd430c8).
func deterministicUUID(name string) string {
	// UUID v5 namespace (DNS namespace from RFC 4122).
	namespace := [16]byte{
		0x6b, 0xa7, 0xb8, 0x10,
		0x9d, 0xad, 0x11, 0xd1,
		0x80, 0xb4, 0x00, 0xc0,
		0x4f, 0xd4, 0x30, 0xc8,
	}

	h := sha1.New()
	h.Write(namespace[:])
	h.Write([]byte(name))
	sum := h.Sum(nil)

	// Set version 5.
	sum[6] = (sum[6] & 0x0f) | 0x50
	// Set variant bits.
	sum[8] = (sum[8] & 0x3f) | 0x80

	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		sum[0:4], sum[4:6], sum[6:8], sum[8:10], sum[10:16])
}

// toClassName converts an index name to a valid Weaviate class name.
// Weaviate class names must start with an uppercase letter.
func toClassName(name string) string {
	if name == "" {
		return "Index"
	}
	// Replace any non-alphanumeric characters with underscores.
	cleaned := strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return r
		}
		return '_'
	}, name)

	// Capitalize first letter.
	runes := []rune(cleaned)
	runes[0] = unicode.ToUpper(runes[0])
	return string(runes)
}

// effectiveLimit returns a sensible default if limit is zero or negative.
func effectiveLimit(limit int) int {
	if limit <= 0 {
		return 10
	}
	return limit
}

// quoteString returns a GraphQL-escaped quoted string.
func quoteString(s string) string {
	// Use JSON encoding for proper escaping.
	b, _ := json.Marshal(s)
	return string(b)
}

// quoteStringSlice formats a slice of strings as GraphQL list of quoted strings.
func quoteStringSlice(ss []string) string {
	parts := make([]string, len(ss))
	for i, s := range ss {
		parts[i] = quoteString(s)
	}
	return strings.Join(parts, ", ")
}

// formatVector formats a float32 slice as a GraphQL list.
func formatVector(v []float32) string {
	parts := make([]string, len(v))
	for i, f := range v {
		parts[i] = strconv.FormatFloat(float64(f), 'f', -1, 32)
	}
	return "[" + strings.Join(parts, ", ") + "]"
}

// formatFloat formats a float64 for GraphQL.
func formatFloat(f float64) string {
	return strconv.FormatFloat(f, 'f', -1, 64)
}

// formatAnyNumber converts an any value to a numeric string for GraphQL.
func formatAnyNumber(v any) string {
	switch n := v.(type) {
	case float64:
		return formatFloat(n)
	case float32:
		return formatFloat(float64(n))
	case int:
		return strconv.Itoa(n)
	case int64:
		return strconv.FormatInt(n, 10)
	case int32:
		return strconv.FormatInt(int64(n), 10)
	case json.Number:
		return n.String()
	default:
		return fmt.Sprintf("%v", v)
	}
}

// toFloat64Safe converts an any value to float64, returning 0 on failure.
func toFloat64Safe(v any) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case float32:
		return float64(n)
	case string:
		f, _ := strconv.ParseFloat(n, 64)
		return f
	case json.Number:
		f, _ := n.Float64()
		return f
	default:
		return 0
	}
}
