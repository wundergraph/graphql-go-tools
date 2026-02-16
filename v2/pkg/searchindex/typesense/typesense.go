// Package typesense implements the searchindex.Index interface for Typesense.
//
// It uses only the Go standard library (net/http + encoding/json) to talk to
// the Typesense HTTP API. No external Typesense SDK is used.
package typesense

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/searchindex"
)

// Compile-time interface conformance checks.
var (
	_ searchindex.Index        = (*Index)(nil)
	_ searchindex.IndexFactory = (*Factory)(nil)
)

// Internal field names stored alongside user data.
const (
	reservedDocIDField     = "_docId"
	reservedTypeNameField  = "_typeName"
	reservedKeyFieldsField = "_keyFieldsJSON"
)

// Config holds Typesense-specific configuration.
type Config struct {
	Host     string `json:"host"`
	Port     int    `json:"port,omitempty"`
	APIKey   string `json:"api_key"`
	Protocol string `json:"protocol,omitempty"`
}

// Factory implements searchindex.IndexFactory for Typesense.
type Factory struct{}

// NewFactory returns a new Typesense IndexFactory.
func NewFactory() *Factory {
	return &Factory{}
}

// CreateIndex creates a new Typesense collection that mirrors the given schema
// and returns an Index handle for it.
func (f *Factory) CreateIndex(ctx context.Context, name string, schema searchindex.IndexConfig, configJSON []byte) (searchindex.Index, error) {
	var cfg Config
	if len(configJSON) > 0 {
		if err := json.Unmarshal(configJSON, &cfg); err != nil {
			return nil, fmt.Errorf("typesense: invalid config: %w", err)
		}
	}
	if cfg.Host == "" {
		cfg.Host = "localhost"
	}
	if cfg.Port == 0 {
		cfg.Port = 8108
	}
	if cfg.Protocol == "" {
		cfg.Protocol = "http"
	}

	idx := &Index{
		name:   name,
		config: cfg,
		schema: schema,
		client: &http.Client{},
	}

	if err := idx.createCollection(ctx); err != nil {
		return nil, err
	}

	return idx, nil
}

// Index implements searchindex.Index backed by a Typesense collection.
type Index struct {
	name   string
	config Config
	schema searchindex.IndexConfig
	client *http.Client
}

// ---------------------------------------------------------------------------
// Collection creation
// ---------------------------------------------------------------------------

// typesenseField is the JSON representation of a Typesense collection field.
type typesenseField struct {
	Name   string `json:"name"`
	Type   string `json:"type"`
	Facet  bool   `json:"facet,omitempty"`
	Sort   bool   `json:"sort,omitempty"`
	NumDim int    `json:"num_dim,omitempty"`
}

// typesenseSchema is the JSON body sent to POST /collections.
type typesenseSchema struct {
	Name                string           `json:"name"`
	Fields              []typesenseField `json:"fields"`
	DefaultSortingField string           `json:"default_sorting_field,omitempty"`
}

func (idx *Index) createCollection(ctx context.Context) error {
	fields := make([]typesenseField, 0, len(idx.schema.Fields)+3)

	var defaultSortingField string

	for _, fc := range idx.schema.Fields {
		tf, err := mapField(fc)
		if err != nil {
			return fmt.Errorf("typesense: field %q: %w", fc.Name, err)
		}
		if fc.Sortable {
			tf.Sort = true
		}
		fields = append(fields, tf)

		// Pick the first numeric sortable field as default_sorting_field.
		if defaultSortingField == "" && fc.Type == searchindex.FieldTypeNumeric && fc.Sortable {
			defaultSortingField = fc.Name
		}
	}

	// Internal metadata fields.
	fields = append(fields,
		typesenseField{Name: reservedDocIDField, Type: "string"},
		typesenseField{Name: reservedTypeNameField, Type: "string", Facet: true},
		typesenseField{Name: reservedKeyFieldsField, Type: "string"},
	)

	schema := typesenseSchema{
		Name:                idx.name,
		Fields:              fields,
		DefaultSortingField: defaultSortingField,
	}

	body, err := json.Marshal(schema)
	if err != nil {
		return fmt.Errorf("typesense: marshal schema: %w", err)
	}

	resp, err := idx.doRequest(ctx, http.MethodPost, "/collections", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("typesense: create collection: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusConflict {
		// Collection already exists; treat as success.
		_, _ = io.Copy(io.Discard, resp.Body)
		return nil
	}

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		return idx.readError(resp, "create collection")
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	return nil
}

// mapField converts a searchindex.FieldConfig to a typesenseField.
func mapField(fc searchindex.FieldConfig) (typesenseField, error) {
	tf := typesenseField{Name: fc.Name}
	switch fc.Type {
	case searchindex.FieldTypeText:
		tf.Type = "string"
	case searchindex.FieldTypeKeyword:
		tf.Type = "string"
		tf.Facet = true
	case searchindex.FieldTypeNumeric:
		if fc.Dimensions > 0 {
			// Unusual, but guard.
			tf.Type = "float"
		} else {
			// Use float as default numeric type. Callers who want int64 can
			// use Dimensions == 0 and provide int-valued floats.
			tf.Type = "float"
		}
	case searchindex.FieldTypeBool:
		tf.Type = "bool"
	case searchindex.FieldTypeVector:
		tf.Type = "float[]"
		tf.NumDim = fc.Dimensions
	case searchindex.FieldTypeGeo:
		// Typesense supports geopoint natively.
		tf.Type = "geopoint"
	case searchindex.FieldTypeDate, searchindex.FieldTypeDateTime:
		// Typesense stores dates as unix timestamps (int64).
		tf.Type = "int64"
	default:
		return typesenseField{}, fmt.Errorf("unsupported field type %v", fc.Type)
	}

	if fc.Filterable || fc.Autocomplete {
		// For Typesense, all non-vector fields are searchable/filterable by default.
		// Facet is only needed for keyword-style fields, but we can set it
		// for any filterable field to enable filter_by. Autocomplete fields
		// need facet enabled for facet-query based autocomplete.
		tf.Facet = true
	}

	return tf, nil
}

// ---------------------------------------------------------------------------
// Indexing
// ---------------------------------------------------------------------------

// IndexDocument indexes a single document.
func (idx *Index) IndexDocument(ctx context.Context, doc searchindex.EntityDocument) error {
	return idx.IndexDocuments(ctx, []searchindex.EntityDocument{doc})
}

// IndexDocuments indexes a batch of documents using the JSONL import API.
func (idx *Index) IndexDocuments(ctx context.Context, docs []searchindex.EntityDocument) error {
	if len(docs) == 0 {
		return nil
	}

	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)

	dateFields := idx.dateFieldSet()

	for _, doc := range docs {
		flat, err := buildDocument(doc)
		if err != nil {
			return err
		}
		if len(dateFields) > 0 {
			if err := convertDateFieldsInDoc(flat, dateFields); err != nil {
				return err
			}
		}
		if err := enc.Encode(flat); err != nil {
			return fmt.Errorf("typesense: encode document: %w", err)
		}
	}

	path := fmt.Sprintf("/collections/%s/documents/import?action=upsert", url.PathEscape(idx.name))
	resp, err := idx.doRequest(ctx, http.MethodPost, path, &buf)
	if err != nil {
		return fmt.Errorf("typesense: import documents: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return idx.readError(resp, "import documents")
	}

	// The import endpoint returns one JSON object per line. Check for errors.
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("typesense: read import response: %w", err)
	}
	lines := bytes.Split(bytes.TrimSpace(respBody), []byte("\n"))
	for _, line := range lines {
		if len(line) == 0 {
			continue
		}
		var result struct {
			Success bool   `json:"success"`
			Error   string `json:"error"`
		}
		if err := json.Unmarshal(line, &result); err != nil {
			return fmt.Errorf("typesense: parse import result line: %w", err)
		}
		if !result.Success {
			return fmt.Errorf("typesense: import document failed: %s", result.Error)
		}
	}

	return nil
}

// buildDocument creates a flat JSON-serialisable map from an EntityDocument.
func buildDocument(doc searchindex.EntityDocument) (map[string]any, error) {
	m := make(map[string]any, len(doc.Fields)+len(doc.Vectors)+3)

	for k, v := range doc.Fields {
		m[k] = v
	}
	for k, v := range doc.Vectors {
		m[k] = v
	}

	docID := documentID(doc.Identity)
	m["id"] = docID
	m[reservedDocIDField] = docID
	m[reservedTypeNameField] = doc.Identity.TypeName

	keyFieldsJSON, err := json.Marshal(doc.Identity.KeyFields)
	if err != nil {
		return nil, fmt.Errorf("typesense: marshal key fields: %w", err)
	}
	m[reservedKeyFieldsField] = string(keyFieldsJSON)

	return m, nil
}

// dateToUnix parses an ISO 8601 date or datetime string and returns the unix timestamp.
// Supported formats: "2024-01-15", "2024-01-15T10:30:00Z", "2024-01-15T10:30:00.000Z",
// "2024-01-15T10:30:00+02:00".
func dateToUnix(s string) (int64, error) {
	for _, layout := range []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02T15:04:05",
		time.DateOnly,
	} {
		if t, err := time.Parse(layout, s); err == nil {
			return t.Unix(), nil
		}
	}
	return 0, fmt.Errorf("typesense: cannot parse date %q", s)
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

// convertDateFieldsInDoc converts ISO date strings to unix timestamps for date fields.
func convertDateFieldsInDoc(doc map[string]any, dateFields map[string]bool) error {
	for name := range dateFields {
		v, ok := doc[name]
		if !ok {
			continue
		}
		s, ok := v.(string)
		if !ok {
			continue
		}
		ts, err := dateToUnix(s)
		if err != nil {
			return err
		}
		doc[name] = ts
	}
	return nil
}

// convertDateFilters walks a filter tree and converts string date values to
// unix timestamps for fields that are DATE or DATETIME type.
func convertDateFilters(f *searchindex.Filter, dateFields map[string]bool) {
	if f == nil {
		return
	}
	for _, child := range f.And {
		convertDateFilters(child, dateFields)
	}
	for _, child := range f.Or {
		convertDateFilters(child, dateFields)
	}
	if f.Not != nil {
		convertDateFilters(f.Not, dateFields)
	}
	if f.Term != nil && dateFields[f.Term.Field] {
		if s, ok := f.Term.Value.(string); ok {
			if ts, err := dateToUnix(s); err == nil {
				f.Term.Value = ts
			}
		}
	}
	if f.Range != nil && dateFields[f.Range.Field] {
		convertDateRangeValue := func(v any) any {
			if s, ok := v.(string); ok {
				if ts, err := dateToUnix(s); err == nil {
					return ts
				}
			}
			return v
		}
		if f.Range.GT != nil {
			f.Range.GT = convertDateRangeValue(f.Range.GT)
		}
		if f.Range.GTE != nil {
			f.Range.GTE = convertDateRangeValue(f.Range.GTE)
		}
		if f.Range.LT != nil {
			f.Range.LT = convertDateRangeValue(f.Range.LT)
		}
		if f.Range.LTE != nil {
			f.Range.LTE = convertDateRangeValue(f.Range.LTE)
		}
	}
}

// documentID computes a deterministic string ID from a DocumentIdentity.
// Format: TypeName:key1=val1,key2=val2,... (keys sorted alphabetically).
func documentID(id searchindex.DocumentIdentity) string {
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

// ---------------------------------------------------------------------------
// Deletion
// ---------------------------------------------------------------------------

// DeleteDocument deletes a single document by identity.
func (idx *Index) DeleteDocument(ctx context.Context, id searchindex.DocumentIdentity) error {
	return idx.DeleteDocuments(ctx, []searchindex.DocumentIdentity{id})
}

// DeleteDocuments deletes a batch of documents by identity.
func (idx *Index) DeleteDocuments(ctx context.Context, ids []searchindex.DocumentIdentity) error {
	var firstErr error
	for _, id := range ids {
		docID := documentID(id)
		path := fmt.Sprintf("/collections/%s/documents/%s", url.PathEscape(idx.name), url.PathEscape(docID))
		resp, err := idx.doRequest(ctx, http.MethodDelete, path, nil)
		if err != nil {
			if firstErr == nil {
				firstErr = fmt.Errorf("typesense: delete document %q: %w", docID, err)
			}
			continue
		}
		_, _ = io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNotFound {
			if firstErr == nil {
				firstErr = fmt.Errorf("typesense: delete document %q: HTTP %d", docID, resp.StatusCode)
			}
		}
	}
	return firstErr
}

// ---------------------------------------------------------------------------
// Search
// ---------------------------------------------------------------------------

// Search performs a search query against the Typesense collection.
func (idx *Index) Search(ctx context.Context, req searchindex.SearchRequest) (*searchindex.SearchResult, error) {
	params := url.Values{}

	// Query text.
	q := req.TextQuery
	if q == "" {
		q = "*"
	}
	params.Set("q", q)

	// query_by: text fields to search, with optional per-field weights.
	var queryByNames []string
	var queryByWeights []string
	hasCustomWeights := false

	if len(req.TextFields) > 0 {
		for _, tf := range req.TextFields {
			queryByNames = append(queryByNames, tf.Name)
			w := tf.Weight
			if w == 0 {
				w = 1
			}
			if w != 1 {
				hasCustomWeights = true
			}
			queryByWeights = append(queryByWeights, fmt.Sprintf("%d", int(w)))
		}
	} else if req.TextQuery != "" {
		// Default to all text fields in the schema.
		for _, fc := range idx.schema.Fields {
			if fc.Type == searchindex.FieldTypeText {
				queryByNames = append(queryByNames, fc.Name)
			}
		}
	}
	if len(queryByNames) == 0 {
		// Must have at least one query_by field for Typesense.
		// Fall back to all string-type fields.
		for _, fc := range idx.schema.Fields {
			if fc.Type == searchindex.FieldTypeText || fc.Type == searchindex.FieldTypeKeyword {
				queryByNames = append(queryByNames, fc.Name)
			}
		}
	}
	if len(queryByNames) > 0 {
		params.Set("query_by", strings.Join(queryByNames, ","))
		if hasCustomWeights {
			params.Set("query_by_weights", strings.Join(queryByWeights, ","))
		}
	}

	// Filters.
	filterParts := make([]string, 0, 2)

	if req.TypeName != "" {
		filterParts = append(filterParts, fmt.Sprintf("%s:=%s", reservedTypeNameField, escapeFilterValue(req.TypeName)))
	}

	if req.Filter != nil {
		dateFields := idx.dateFieldSet()
		if len(dateFields) > 0 {
			convertDateFilters(req.Filter, dateFields)
		}
		fStr, err := translateFilter(req.Filter)
		if err != nil {
			return nil, err
		}
		if fStr != "" {
			filterParts = append(filterParts, fStr)
		}
	}

	if len(filterParts) > 0 {
		params.Set("filter_by", strings.Join(filterParts, " && "))
	}

	// Sorting.
	if len(req.Sort) > 0 {
		sortParts := make([]string, 0, len(req.Sort))
		for _, sf := range req.Sort {
			dir := "desc"
			if sf.Ascending {
				dir = "asc"
			}
			sortParts = append(sortParts, sf.Field+":"+dir)
		}
		params.Set("sort_by", strings.Join(sortParts, ","))
	}

	// Pagination.
	limit := req.Limit
	if limit <= 0 {
		limit = 10
	}
	params.Set("per_page", strconv.Itoa(limit))

	if req.Offset > 0 {
		// Typesense uses 1-based pages.
		page := (req.Offset / limit) + 1
		params.Set("page", strconv.Itoa(page))
	}

	// Fuzziness / typo tolerance.
	if req.Fuzziness != nil {
		params.Set("num_typos", strconv.Itoa(int(*req.Fuzziness)))
	}

	// Facets.
	if len(req.Facets) > 0 {
		facetFields := make([]string, 0, len(req.Facets))
		for _, fr := range req.Facets {
			facetFields = append(facetFields, fr.Field)
		}
		params.Set("facet_by", strings.Join(facetFields, ","))
		// Use the max facet Size from the requests.
		maxSize := 0
		for _, fr := range req.Facets {
			if fr.Size > maxSize {
				maxSize = fr.Size
			}
		}
		if maxSize > 0 {
			params.Set("max_facet_values", strconv.Itoa(maxSize))
		}
	}

	// Vector search.
	if len(req.Vector) > 0 && req.VectorField != "" {
		vecStrs := make([]string, 0, len(req.Vector))
		for _, v := range req.Vector {
			vecStrs = append(vecStrs, strconv.FormatFloat(float64(v), 'f', -1, 32))
		}
		k := limit
		if k <= 0 {
			k = 10
		}
		vectorQuery := fmt.Sprintf("%s:([%s], k:%d)", req.VectorField, strings.Join(vecStrs, ", "), k)
		params.Set("vector_query", vectorQuery)
	}

	path := fmt.Sprintf("/collections/%s/documents/search?%s", url.PathEscape(idx.name), params.Encode())
	resp, err := idx.doRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, fmt.Errorf("typesense: search: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, idx.readError(resp, "search")
	}

	var tsResp typesenseSearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&tsResp); err != nil {
		return nil, fmt.Errorf("typesense: decode search response: %w", err)
	}

	return idx.convertSearchResponse(&tsResp)
}

// typesenseSearchResponse mirrors the Typesense search result JSON.
type typesenseSearchResponse struct {
	Found       int                      `json:"found"`
	Hits        []typesenseHit           `json:"hits"`
	FacetCounts []typesenseFacetCount    `json:"facet_counts"`
}

type typesenseHit struct {
	Document  map[string]any        `json:"document"`
	TextMatch json.Number           `json:"text_match"`
	Highlights []typesenseHighlight `json:"highlights"`
	VectorDistance *float64          `json:"vector_distance,omitempty"`
}

type typesenseHighlight struct {
	Field    string   `json:"field"`
	Snippet  string   `json:"snippet"`
	Snippets []string `json:"snippets"`
}

type typesenseFacetCount struct {
	FieldName string               `json:"field_name"`
	Counts    []typesenseFacetVal  `json:"counts"`
}

type typesenseFacetVal struct {
	Value string `json:"value"`
	Count int    `json:"count"`
}

func (idx *Index) convertSearchResponse(tsResp *typesenseSearchResponse) (*searchindex.SearchResult, error) {
	hits := make([]searchindex.SearchHit, 0, len(tsResp.Hits))
	for _, h := range tsResp.Hits {
		hit, err := convertHit(&h)
		if err != nil {
			return nil, err
		}
		hits = append(hits, hit)
	}

	facets := convertFacets(tsResp.FacetCounts)

	return &searchindex.SearchResult{
		Hits:       hits,
		TotalCount: tsResp.Found,
		Facets:     facets,
	}, nil
}

func convertHit(h *typesenseHit) (searchindex.SearchHit, error) {
	identity, err := extractIdentity(h.Document)
	if err != nil {
		return searchindex.SearchHit{}, err
	}

	// Build representation from the document, excluding internal fields.
	representation := make(map[string]any, len(h.Document))
	for k, v := range h.Document {
		if k == reservedDocIDField || k == reservedTypeNameField || k == reservedKeyFieldsField || k == "id" {
			continue
		}
		representation[k] = v
	}
	representation["__typename"] = identity.TypeName
	for k, v := range identity.KeyFields {
		representation[k] = v
	}

	// Score.
	var score float64
	if h.TextMatch.String() != "" {
		score, _ = h.TextMatch.Float64()
	}

	// Distance for vector search.
	var distance float64
	if h.VectorDistance != nil {
		distance = *h.VectorDistance
	}

	// Highlights.
	var highlights map[string][]string
	if len(h.Highlights) > 0 {
		highlights = make(map[string][]string, len(h.Highlights))
		for _, hl := range h.Highlights {
			if len(hl.Snippets) > 0 {
				highlights[hl.Field] = hl.Snippets
			} else if hl.Snippet != "" {
				highlights[hl.Field] = []string{hl.Snippet}
			}
		}
	}

	return searchindex.SearchHit{
		Identity:       identity,
		Score:          score,
		Distance:       distance,
		Highlights:     highlights,
		Representation: representation,
	}, nil
}

func extractIdentity(doc map[string]any) (searchindex.DocumentIdentity, error) {
	typeName, _ := doc[reservedTypeNameField].(string)
	keyFieldsRaw, _ := doc[reservedKeyFieldsField].(string)

	var keyFields map[string]any
	if keyFieldsRaw != "" {
		if err := json.Unmarshal([]byte(keyFieldsRaw), &keyFields); err != nil {
			return searchindex.DocumentIdentity{}, fmt.Errorf("typesense: unmarshal key fields: %w", err)
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

func convertFacets(tsFacets []typesenseFacetCount) map[string]searchindex.FacetResult {
	if len(tsFacets) == 0 {
		return nil
	}
	facets := make(map[string]searchindex.FacetResult, len(tsFacets))
	for _, fc := range tsFacets {
		values := make([]searchindex.FacetValue, 0, len(fc.Counts))
		for _, cv := range fc.Counts {
			values = append(values, searchindex.FacetValue{
				Value: cv.Value,
				Count: cv.Count,
			})
		}
		facets[fc.FieldName] = searchindex.FacetResult{Values: values}
	}
	return facets
}

// ---------------------------------------------------------------------------
// Filter translation
// ---------------------------------------------------------------------------

// translateFilter recursively converts a searchindex.Filter tree to a
// Typesense filter_by string.
func translateFilter(f *searchindex.Filter) (string, error) {
	if f == nil {
		return "", nil
	}

	// AND
	if len(f.And) > 0 {
		parts := make([]string, 0, len(f.And))
		for _, child := range f.And {
			s, err := translateFilter(child)
			if err != nil {
				return "", err
			}
			if s != "" {
				parts = append(parts, s)
			}
		}
		if len(parts) == 0 {
			return "", nil
		}
		return "(" + strings.Join(parts, " && ") + ")", nil
	}

	// OR
	if len(f.Or) > 0 {
		parts := make([]string, 0, len(f.Or))
		for _, child := range f.Or {
			s, err := translateFilter(child)
			if err != nil {
				return "", err
			}
			if s != "" {
				parts = append(parts, s)
			}
		}
		if len(parts) == 0 {
			return "", nil
		}
		return "(" + strings.Join(parts, " || ") + ")", nil
	}

	// NOT — Typesense uses :!= for negation at the field level.
	if f.Not != nil {
		return translateNotFilter(f.Not)
	}

	// Term — exact match.
	if f.Term != nil {
		return translateTermFilter(f.Term)
	}

	// Terms — IN operator.
	if f.Terms != nil {
		return translateTermsFilter(f.Terms)
	}

	// Range.
	if f.Range != nil {
		return translateRangeFilter(f.Range)
	}

	// Prefix.
	if f.Prefix != nil {
		// Typesense does not have a native prefix filter in filter_by.
		// Approximate via range: field:[prefixValue, prefixValue~] is not
		// supported. Use a workaround: we cannot perfectly replicate prefix
		// with filter_by alone. Return an unsupported error for now.
		return "", fmt.Errorf("typesense: prefix filter is not supported in filter_by; use text search instead")
	}

	// Exists.
	if f.Exists != nil {
		// Typesense supports: field:!=''  (field is not empty string) for strings.
		// For general "exists" semantics, use: field:!=null (Typesense 0.25+).
		return fmt.Sprintf("%s:!=%s", f.Exists.Field, "``"), nil
	}

	return "", nil
}

func translateTermFilter(tf *searchindex.TermFilter) (string, error) {
	switch v := tf.Value.(type) {
	case string:
		return fmt.Sprintf("%s:=%s", tf.Field, escapeFilterValue(v)), nil
	case bool:
		return fmt.Sprintf("%s:=%t", tf.Field, v), nil
	case float64:
		return fmt.Sprintf("%s:=%s", tf.Field, formatNumber(v)), nil
	case float32:
		return fmt.Sprintf("%s:=%s", tf.Field, formatNumber(float64(v))), nil
	case int:
		return fmt.Sprintf("%s:=%d", tf.Field, v), nil
	case int64:
		return fmt.Sprintf("%s:=%d", tf.Field, v), nil
	case json.Number:
		return fmt.Sprintf("%s:=%s", tf.Field, v.String()), nil
	default:
		return fmt.Sprintf("%s:=%s", tf.Field, escapeFilterValue(fmt.Sprintf("%v", v))), nil
	}
}

// translateNotFilter negates a filter using Typesense's :!= operator.
func translateNotFilter(inner *searchindex.Filter) (string, error) {
	// Term: field:=value → field:!=value
	if inner.Term != nil {
		tf := inner.Term
		switch v := tf.Value.(type) {
		case string:
			return fmt.Sprintf("%s:!=%s", tf.Field, escapeFilterValue(v)), nil
		case bool:
			return fmt.Sprintf("%s:!=%t", tf.Field, v), nil
		case float64:
			return fmt.Sprintf("%s:!=%s", tf.Field, formatNumber(v)), nil
		case float32:
			return fmt.Sprintf("%s:!=%s", tf.Field, formatNumber(float64(v))), nil
		case int:
			return fmt.Sprintf("%s:!=%d", tf.Field, v), nil
		case int64:
			return fmt.Sprintf("%s:!=%d", tf.Field, v), nil
		case json.Number:
			return fmt.Sprintf("%s:!=%s", tf.Field, v.String()), nil
		default:
			return fmt.Sprintf("%s:!=%s", tf.Field, escapeFilterValue(fmt.Sprintf("%v", v))), nil
		}
	}

	// AND: NOT(a AND b) → NOT(a) || NOT(b) (De Morgan's)
	if len(inner.And) > 0 {
		parts := make([]string, 0, len(inner.And))
		for _, child := range inner.And {
			s, err := translateNotFilter(child)
			if err != nil {
				return "", err
			}
			parts = append(parts, s)
		}
		return "(" + strings.Join(parts, " || ") + ")", nil
	}

	// OR: NOT(a OR b) → NOT(a) && NOT(b) (De Morgan's)
	if len(inner.Or) > 0 {
		parts := make([]string, 0, len(inner.Or))
		for _, child := range inner.Or {
			s, err := translateNotFilter(child)
			if err != nil {
				return "", err
			}
			parts = append(parts, s)
		}
		return "(" + strings.Join(parts, " && ") + ")", nil
	}

	// Double negation: NOT(NOT(x)) → x
	if inner.Not != nil {
		return translateFilter(inner.Not)
	}

	return "", fmt.Errorf("typesense: NOT filter is not supported for this filter type")
}

func translateTermsFilter(tf *searchindex.TermsFilter) (string, error) {
	if len(tf.Values) == 0 {
		return "", nil
	}
	vals := make([]string, 0, len(tf.Values))
	for _, v := range tf.Values {
		vals = append(vals, formatFilterValue(v))
	}
	return fmt.Sprintf("%s:[%s]", tf.Field, strings.Join(vals, ", ")), nil
}

func translateRangeFilter(rf *searchindex.RangeFilter) (string, error) {
	parts := make([]string, 0, 2)

	if rf.GTE != nil {
		v := formatFilterValue(rf.GTE)
		parts = append(parts, fmt.Sprintf("%s:>=%s", rf.Field, v))
	} else if rf.HasGT && rf.GT != nil {
		v := formatFilterValue(rf.GT)
		parts = append(parts, fmt.Sprintf("%s:>%s", rf.Field, v))
	}

	if rf.LTE != nil {
		v := formatFilterValue(rf.LTE)
		parts = append(parts, fmt.Sprintf("%s:<=%s", rf.Field, v))
	} else if rf.HasLT && rf.LT != nil {
		v := formatFilterValue(rf.LT)
		parts = append(parts, fmt.Sprintf("%s:<%s", rf.Field, v))
	}

	if len(parts) == 0 {
		return "", nil
	}
	return strings.Join(parts, " && "), nil
}

// formatFilterValue formats an arbitrary value for use in a Typesense filter_by string.
func formatFilterValue(v any) string {
	switch val := v.(type) {
	case string:
		return escapeFilterValue(val)
	case bool:
		if val {
			return "true"
		}
		return "false"
	case float64:
		return formatNumber(val)
	case float32:
		return formatNumber(float64(val))
	case int:
		return strconv.Itoa(val)
	case int64:
		return strconv.FormatInt(val, 10)
	case json.Number:
		return val.String()
	default:
		return escapeFilterValue(fmt.Sprintf("%v", val))
	}
}

// escapeFilterValue wraps a string value in backticks for Typesense filter_by syntax.
func escapeFilterValue(s string) string {
	// Typesense uses backtick quoting for values that contain special characters.
	if strings.ContainsAny(s, " ,[]()&|:!=<>`") {
		return "`" + strings.ReplaceAll(s, "`", "\\`") + "`"
	}
	return s
}

// formatNumber formats a float64, preferring integer representation when possible.
func formatNumber(f float64) string {
	if f == float64(int64(f)) {
		return strconv.FormatInt(int64(f), 10)
	}
	return strconv.FormatFloat(f, 'f', -1, 64)
}

// ---------------------------------------------------------------------------
// Close
// ---------------------------------------------------------------------------

// Autocomplete returns terms matching the given prefix using Typesense's facet query.
func (idx *Index) Autocomplete(ctx context.Context, req searchindex.AutocompleteRequest) (*searchindex.AutocompleteResult, error) {
	limit := req.Limit
	if limit <= 0 {
		limit = 10
	}

	params := url.Values{
		"q":                 {"*"},
		"query_by":          {req.Field},
		"facet_by":          {req.Field},
		"facet_query":       {req.Field + ":" + strings.ToLower(req.Prefix)},
		"per_page":          {"0"},
		"max_facet_values":  {strconv.Itoa(limit)},
	}

	path := fmt.Sprintf("/collections/%s/documents/search?%s", url.PathEscape(idx.name), params.Encode())
	resp, err := idx.doRequest(ctx, "GET", path, nil)
	if err != nil {
		return nil, fmt.Errorf("typesense: autocomplete request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return nil, idx.readError(resp, "autocomplete")
	}

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("typesense: read autocomplete response: %w", err)
	}

	var result struct {
		FacetCounts []struct {
			Counts []struct {
				Value string `json:"value"`
				Count int    `json:"count"`
			} `json:"counts"`
		} `json:"facet_counts"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("typesense: unmarshal autocomplete response: %w", err)
	}

	var terms []searchindex.AutocompleteTerm
	if len(result.FacetCounts) > 0 {
		for _, c := range result.FacetCounts[0].Counts {
			terms = append(terms, searchindex.AutocompleteTerm{Term: c.Value, Count: c.Count})
		}
	}

	return &searchindex.AutocompleteResult{Terms: terms}, nil
}

// Close releases resources held by the index. The underlying HTTP client is
// shared and does not need explicit cleanup.
func (idx *Index) Close() error {
	return nil
}

// ---------------------------------------------------------------------------
// HTTP helpers
// ---------------------------------------------------------------------------

func (idx *Index) baseURL() string {
	return fmt.Sprintf("%s://%s:%d", idx.config.Protocol, idx.config.Host, idx.config.Port)
}

func (idx *Index) doRequest(ctx context.Context, method, path string, body io.Reader) (*http.Response, error) {
	u := idx.baseURL() + path
	req, err := http.NewRequestWithContext(ctx, method, u, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if idx.config.APIKey != "" {
		req.Header.Set("X-TYPESENSE-API-KEY", idx.config.APIKey)
	}
	return idx.client.Do(req)
}

func (idx *Index) readError(resp *http.Response, action string) error {
	bodyBytes, _ := io.ReadAll(resp.Body)
	return fmt.Errorf("typesense: %s failed (HTTP %d): %s", action, resp.StatusCode, string(bodyBytes))
}
