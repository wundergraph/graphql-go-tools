// Package qdrant implements the searchindex.Index interface for Qdrant.
//
// Priority: P2
// Supports: vector-native search, prefetch + fusion hybrid.
// Filter translation: searchindex.Filter -> Qdrant must/should/must_not clauses.
//
// This implementation uses only net/http + encoding/json (no external SDK).
// It communicates with Qdrant's REST API.
package qdrant

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"io"
	"net/http"
	"sort"
	"strings"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/searchindex"
)

// Compile-time interface checks.
var (
	_ searchindex.Index        = (*Index)(nil)
	_ searchindex.IndexFactory = (*Factory)(nil)
)

// reservedTypeNameField is the payload field used to store the entity type name.
const reservedTypeNameField = "_typeName"

// reservedKeyFieldsField stores the JSON-encoded key fields map so we can
// reconstruct DocumentIdentity from search results.
const reservedKeyFieldsField = "_keyFieldsJSON"

// Config holds Qdrant-specific configuration.
type Config struct {
	Host   string `json:"host"`
	Port   int    `json:"port,omitempty"`
	APIKey string `json:"api_key,omitempty"`
	UseTLS bool   `json:"use_tls,omitempty"`
}

// baseURL returns the Qdrant REST API base URL derived from the config.
func (c *Config) baseURL() string {
	scheme := "http"
	if c.UseTLS {
		scheme = "https"
	}
	return fmt.Sprintf("%s://%s:%d", scheme, c.Host, c.Port)
}

// Factory implements searchindex.IndexFactory for Qdrant.
type Factory struct{}

// NewFactory returns a new Qdrant IndexFactory.
func NewFactory() *Factory {
	return &Factory{}
}

// CreateIndex creates a new Qdrant collection with the given name and schema,
// then returns an Index that can be used for CRUD and search operations.
func (f *Factory) CreateIndex(ctx context.Context, name string, schema searchindex.IndexConfig, configJSON []byte) (searchindex.Index, error) {
	var cfg Config
	if len(configJSON) > 0 {
		if err := json.Unmarshal(configJSON, &cfg); err != nil {
			return nil, fmt.Errorf("qdrant: invalid config: %w", err)
		}
	}
	if cfg.Host == "" {
		cfg.Host = "localhost"
	}
	if cfg.Port == 0 {
		cfg.Port = 6333
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

	if err := idx.createPayloadIndexes(ctx); err != nil {
		return nil, err
	}

	return idx, nil
}

// Index implements searchindex.Index for Qdrant.
type Index struct {
	name   string
	config Config
	schema searchindex.IndexConfig
	client *http.Client
}

// createCollection creates (or recreates) a Qdrant collection via PUT /collections/{name}.
func (idx *Index) createCollection(ctx context.Context) error {
	// Determine vector config from schema.
	var vectorSize int
	for _, fc := range idx.schema.Fields {
		if fc.Type == searchindex.FieldTypeVector && fc.Dimensions > 0 {
			vectorSize = fc.Dimensions
			break
		}
	}
	if vectorSize == 0 {
		// No vector fields defined; use a small dummy vector so the collection can be created.
		vectorSize = 4
	}

	body := map[string]any{
		"vectors": map[string]any{
			"size":     vectorSize,
			"distance": "Cosine",
		},
	}

	_, err := idx.doRequest(ctx, http.MethodPut, fmt.Sprintf("/collections/%s", idx.name), body)
	if err != nil {
		return fmt.Errorf("qdrant: create collection %q: %w", idx.name, err)
	}
	return nil
}

// createPayloadIndexes creates payload indexes for filterable and sortable fields.
func (idx *Index) createPayloadIndexes(ctx context.Context) error {
	for _, fc := range idx.schema.Fields {
		if fc.Type == searchindex.FieldTypeVector {
			continue
		}
		if !fc.Filterable && !fc.Sortable {
			continue
		}
		fieldSchema := qdrantFieldSchema(fc.Type)
		if fieldSchema == "" {
			continue
		}
		body := map[string]any{
			"field_name":   fc.Name,
			"field_schema": fieldSchema,
		}
		_, err := idx.doRequest(ctx, http.MethodPut, fmt.Sprintf("/collections/%s/index", idx.name), body)
		if err != nil {
			return fmt.Errorf("qdrant: create payload index for field %q: %w", fc.Name, err)
		}
	}

	// Also create indexes for metadata fields.
	for _, meta := range []struct {
		name   string
		schema string
	}{
		{reservedTypeNameField, "keyword"},
		{reservedKeyFieldsField, "keyword"},
	} {
		body := map[string]any{
			"field_name":   meta.name,
			"field_schema": meta.schema,
		}
		_, err := idx.doRequest(ctx, http.MethodPut, fmt.Sprintf("/collections/%s/index", idx.name), body)
		if err != nil {
			return fmt.Errorf("qdrant: create payload index for field %q: %w", meta.name, err)
		}
	}

	return nil
}

// qdrantFieldSchema maps a searchindex.FieldType to the Qdrant payload field_schema string.
func qdrantFieldSchema(ft searchindex.FieldType) string {
	switch ft {
	case searchindex.FieldTypeText:
		return "text"
	case searchindex.FieldTypeKeyword:
		return "keyword"
	case searchindex.FieldTypeNumeric:
		return "float"
	case searchindex.FieldTypeBool:
		return "bool"
	case searchindex.FieldTypeGeo:
		return "geo"
	case searchindex.FieldTypeDate, searchindex.FieldTypeDateTime:
		return "datetime"
	default:
		return ""
	}
}

// IndexDocument indexes a single document.
func (idx *Index) IndexDocument(ctx context.Context, doc searchindex.EntityDocument) error {
	return idx.IndexDocuments(ctx, []searchindex.EntityDocument{doc})
}

// IndexDocuments indexes a batch of documents via PUT /collections/{name}/points.
func (idx *Index) IndexDocuments(ctx context.Context, docs []searchindex.EntityDocument) error {
	points := make([]map[string]any, 0, len(docs))
	for _, doc := range docs {
		point, err := idx.buildPoint(doc)
		if err != nil {
			return err
		}
		points = append(points, point)
	}

	body := map[string]any{
		"points": points,
	}

	_, err := idx.doRequest(ctx, http.MethodPut, fmt.Sprintf("/collections/%s/points?wait=true", idx.name), body)
	if err != nil {
		return fmt.Errorf("qdrant: index documents: %w", err)
	}
	return nil
}

// buildPoint converts an EntityDocument into a Qdrant point.
func (idx *Index) buildPoint(doc searchindex.EntityDocument) (map[string]any, error) {
	pointID := documentIDHash(doc.Identity)

	// Build payload from all fields plus metadata.
	payload := make(map[string]any, len(doc.Fields)+2)
	for k, v := range doc.Fields {
		payload[k] = v
	}
	payload[reservedTypeNameField] = doc.Identity.TypeName

	keyFieldsJSON, err := json.Marshal(doc.Identity.KeyFields)
	if err != nil {
		return nil, fmt.Errorf("qdrant: failed to marshal key fields: %w", err)
	}
	payload[reservedKeyFieldsField] = string(keyFieldsJSON)

	// Extract vector: use the first vector field from the document's Vectors map.
	var vector []float32
	if len(doc.Vectors) > 0 {
		// Pick the first vector. If the schema specifies a vector field, prefer that.
		for _, fc := range idx.schema.Fields {
			if fc.Type == searchindex.FieldTypeVector {
				if v, ok := doc.Vectors[fc.Name]; ok {
					vector = v
					break
				}
			}
		}
		// If not found via schema, just pick the first one.
		if vector == nil {
			for _, v := range doc.Vectors {
				vector = v
				break
			}
		}
	}

	// If no vector is provided, use a zero vector matching the collection's vector size.
	if vector == nil {
		size := idx.vectorSize()
		vector = make([]float32, size)
	}

	point := map[string]any{
		"id":      pointID,
		"vector":  vector,
		"payload": payload,
	}
	return point, nil
}

// vectorSize returns the configured vector dimension size from the schema.
func (idx *Index) vectorSize() int {
	for _, fc := range idx.schema.Fields {
		if fc.Type == searchindex.FieldTypeVector && fc.Dimensions > 0 {
			return fc.Dimensions
		}
	}
	return 4 // dummy size matching createCollection default
}

// documentIDHash computes a deterministic uint64 hash from a DocumentIdentity
// using FNV-1a. The identity is serialized as TypeName:key1=val1,key2=val2,...
// with keys sorted alphabetically.
func documentIDHash(id searchindex.DocumentIdentity) uint64 {
	s := documentIDString(id)
	h := fnv.New64a()
	_, _ = h.Write([]byte(s))
	return h.Sum64()
}

// documentIDString computes a deterministic string ID from a DocumentIdentity.
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

// DeleteDocument deletes a single document by identity.
func (idx *Index) DeleteDocument(ctx context.Context, id searchindex.DocumentIdentity) error {
	return idx.DeleteDocuments(ctx, []searchindex.DocumentIdentity{id})
}

// DeleteDocuments deletes a batch of documents by identity via
// POST /collections/{name}/points/delete.
func (idx *Index) DeleteDocuments(ctx context.Context, ids []searchindex.DocumentIdentity) error {
	pointIDs := make([]uint64, 0, len(ids))
	for _, id := range ids {
		pointIDs = append(pointIDs, documentIDHash(id))
	}

	body := map[string]any{
		"points": pointIDs,
	}

	_, err := idx.doRequest(ctx, http.MethodPost, fmt.Sprintf("/collections/%s/points/delete?wait=true", idx.name), body)
	if err != nil {
		return fmt.Errorf("qdrant: delete documents: %w", err)
	}
	return nil
}

// Search performs a search query and returns results.
func (idx *Index) Search(ctx context.Context, req searchindex.SearchRequest) (*searchindex.SearchResult, error) {
	// Build the Qdrant filter (may be nil).
	filter := idx.buildFilter(req)

	limit := req.Limit
	if limit <= 0 {
		limit = 10
	}

	// Determine which search mode to use.
	if len(req.Vector) > 0 {
		return idx.vectorSearch(ctx, req.Vector, filter, req.Sort, limit, req.Offset)
	}

	// No vector provided: use scroll to retrieve with filter.
	return idx.scrollSearch(ctx, filter, req.Sort, limit, req.Offset)
}

// vectorSearch performs a vector search via POST /collections/{name}/points/search.
func (idx *Index) vectorSearch(ctx context.Context, vector []float32, filter map[string]any, sortFields []searchindex.SortField, limit, offset int) (*searchindex.SearchResult, error) {
	body := map[string]any{
		"vector":       vector,
		"limit":        limit,
		"with_payload": true,
	}
	if offset > 0 {
		body["offset"] = offset
	}
	if filter != nil {
		body["filter"] = filter
	}

	respBody, err := idx.doRequest(ctx, http.MethodPost, fmt.Sprintf("/collections/%s/points/search", idx.name), body)
	if err != nil {
		return nil, fmt.Errorf("qdrant: vector search: %w", err)
	}

	var resp struct {
		Result []struct {
			ID      json.RawMessage        `json:"id"`
			Score   float64                `json:"score"`
			Payload map[string]interface{} `json:"payload"`
		} `json:"result"`
	}
	if err := json.Unmarshal(respBody, &resp); err != nil {
		return nil, fmt.Errorf("qdrant: decode search response: %w", err)
	}

	hits := make([]searchindex.SearchHit, 0, len(resp.Result))
	for _, r := range resp.Result {
		hit, err := convertPayloadToHit(r.Payload, r.Score)
		if err != nil {
			return nil, err
		}
		hits = append(hits, hit)
	}

	// Apply payload-based sorting if sort fields are specified.
	if len(sortFields) > 0 {
		sortHits(hits, sortFields)
	}

	return &searchindex.SearchResult{
		Hits:       hits,
		TotalCount: len(hits),
	}, nil
}

// scrollSearch performs a filtered retrieval using POST /collections/{name}/points/scroll.
func (idx *Index) scrollSearch(ctx context.Context, filter map[string]any, sortFields []searchindex.SortField, limit, offset int) (*searchindex.SearchResult, error) {
	body := map[string]any{
		"limit":        limit + offset, // fetch enough to handle offset
		"with_payload": true,
	}
	if filter != nil {
		body["filter"] = filter
	}

	// If sort is requested and Qdrant supports order_by (v1.7+), add it.
	if len(sortFields) > 0 {
		sf := sortFields[0] // Qdrant scroll supports single order_by
		direction := "asc"
		if !sf.Ascending {
			direction = "desc"
		}
		body["order_by"] = map[string]any{
			"key":       sf.Field,
			"direction": direction,
		}
	}

	respBody, err := idx.doRequest(ctx, http.MethodPost, fmt.Sprintf("/collections/%s/points/scroll", idx.name), body)
	if err != nil {
		return nil, fmt.Errorf("qdrant: scroll search: %w", err)
	}

	var resp struct {
		Result struct {
			Points []struct {
				ID      json.RawMessage        `json:"id"`
				Payload map[string]interface{} `json:"payload"`
			} `json:"points"`
		} `json:"result"`
	}
	if err := json.Unmarshal(respBody, &resp); err != nil {
		return nil, fmt.Errorf("qdrant: decode scroll response: %w", err)
	}

	allPoints := resp.Result.Points

	// Apply offset.
	if offset > 0 && offset < len(allPoints) {
		allPoints = allPoints[offset:]
	} else if offset >= len(allPoints) {
		allPoints = nil
	}

	// Apply limit.
	if len(allPoints) > limit {
		allPoints = allPoints[:limit]
	}

	hits := make([]searchindex.SearchHit, 0, len(allPoints))
	for _, p := range allPoints {
		hit, err := convertPayloadToHit(p.Payload, 0)
		if err != nil {
			return nil, err
		}
		hits = append(hits, hit)
	}

	// If we couldn't use order_by (multiple sort fields), do client-side sort.
	if len(sortFields) > 1 {
		sortHits(hits, sortFields)
	}

	return &searchindex.SearchResult{
		Hits:       hits,
		TotalCount: len(hits),
	}, nil
}

// buildFilter constructs a Qdrant filter object from the SearchRequest.
func (idx *Index) buildFilter(req searchindex.SearchRequest) map[string]any {
	var conditions []map[string]any

	// TypeName filter.
	if req.TypeName != "" {
		conditions = append(conditions, map[string]any{
			"key": reservedTypeNameField,
			"match": map[string]any{
				"value": req.TypeName,
			},
		})
	}

	// Structured filter.
	if req.Filter != nil {
		filterCond := translateFilter(req.Filter)
		if filterCond != nil {
			conditions = append(conditions, filterCond)
		}
	}

	switch len(conditions) {
	case 0:
		return nil
	case 1:
		// If the single condition is already a compound filter (has must/should/must_not),
		// return it directly. Otherwise wrap it in must.
		if _, hasMust := conditions[0]["must"]; hasMust {
			return conditions[0]
		}
		if _, hasShould := conditions[0]["should"]; hasShould {
			return conditions[0]
		}
		if _, hasMustNot := conditions[0]["must_not"]; hasMustNot {
			return conditions[0]
		}
		return map[string]any{
			"must": conditions,
		}
	default:
		return map[string]any{
			"must": conditions,
		}
	}
}

// translateFilter recursively converts a searchindex.Filter tree to a Qdrant
// filter condition.
func translateFilter(f *searchindex.Filter) map[string]any {
	if f == nil {
		return nil
	}

	// AND
	if len(f.And) > 0 {
		children := make([]map[string]any, 0, len(f.And))
		for _, child := range f.And {
			c := translateFilter(child)
			if c != nil {
				children = append(children, c)
			}
		}
		if len(children) == 0 {
			return nil
		}
		return map[string]any{
			"must": children,
		}
	}

	// OR
	if len(f.Or) > 0 {
		children := make([]map[string]any, 0, len(f.Or))
		for _, child := range f.Or {
			c := translateFilter(child)
			if c != nil {
				children = append(children, c)
			}
		}
		if len(children) == 0 {
			return nil
		}
		return map[string]any{
			"should": children,
		}
	}

	// NOT
	if f.Not != nil {
		inner := translateFilter(f.Not)
		if inner == nil {
			return nil
		}
		return map[string]any{
			"must_not": []map[string]any{inner},
		}
	}

	// Term
	if f.Term != nil {
		return map[string]any{
			"key": f.Term.Field,
			"match": map[string]any{
				"value": f.Term.Value,
			},
		}
	}

	// Terms (IN)
	if f.Terms != nil {
		return map[string]any{
			"key": f.Terms.Field,
			"match": map[string]any{
				"any": f.Terms.Values,
			},
		}
	}

	// Range
	if f.Range != nil {
		rangeMap := make(map[string]any)
		if f.Range.GTE != nil {
			rangeMap["gte"] = toFloat(f.Range.GTE)
		}
		if f.Range.HasGT && f.Range.GT != nil {
			rangeMap["gt"] = toFloat(f.Range.GT)
		}
		if f.Range.LTE != nil {
			rangeMap["lte"] = toFloat(f.Range.LTE)
		}
		if f.Range.HasLT && f.Range.LT != nil {
			rangeMap["lt"] = toFloat(f.Range.LT)
		}
		return map[string]any{
			"key":   f.Range.Field,
			"range": rangeMap,
		}
	}

	// Prefix
	if f.Prefix != nil {
		return map[string]any{
			"key": f.Prefix.Field,
			"match": map[string]any{
				"text": f.Prefix.Value,
			},
		}
	}

	// Exists: Qdrant has no direct "exists" condition, so we negate "is_empty".
	if f.Exists != nil {
		return map[string]any{
			"must_not": []map[string]any{
				{
					"is_empty": map[string]any{
						"key": f.Exists.Field,
					},
				},
			},
		}
	}

	return nil
}

// toFloat converts an any value to float64 for range filters, returning the
// original value if conversion is not straightforward (Qdrant accepts numbers directly).
func toFloat(v any) any {
	switch n := v.(type) {
	case float64:
		return n
	case float32:
		return float64(n)
	case int:
		return float64(n)
	case int64:
		return float64(n)
	case int32:
		return float64(n)
	case json.Number:
		f, err := n.Float64()
		if err != nil {
			return v
		}
		return f
	default:
		return v
	}
}

// convertPayloadToHit converts a Qdrant payload map into a searchindex.SearchHit.
func convertPayloadToHit(payload map[string]interface{}, score float64) (searchindex.SearchHit, error) {
	identity, err := extractIdentity(payload)
	if err != nil {
		return searchindex.SearchHit{}, err
	}

	// Build representation from payload, excluding internal fields.
	representation := make(map[string]any, len(payload))
	for k, v := range payload {
		if k == reservedTypeNameField || k == reservedKeyFieldsField {
			continue
		}
		representation[k] = v
	}

	// Add __typename.
	representation["__typename"] = identity.TypeName
	// Merge key fields into representation.
	for k, v := range identity.KeyFields {
		representation[k] = v
	}

	return searchindex.SearchHit{
		Identity:       identity,
		Score:          score,
		Distance:       score, // Qdrant returns similarity score which can serve as distance metric
		Representation: representation,
	}, nil
}

// extractIdentity reconstructs a DocumentIdentity from a Qdrant payload.
func extractIdentity(payload map[string]interface{}) (searchindex.DocumentIdentity, error) {
	typeName, _ := payload[reservedTypeNameField].(string)
	keyFieldsRaw, _ := payload[reservedKeyFieldsField].(string)

	var keyFields map[string]any
	if keyFieldsRaw != "" {
		if err := json.Unmarshal([]byte(keyFieldsRaw), &keyFields); err != nil {
			return searchindex.DocumentIdentity{}, fmt.Errorf("qdrant: failed to unmarshal key fields: %w", err)
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

// sortHits sorts search hits by the given sort fields (client-side).
func sortHits(hits []searchindex.SearchHit, sortFields []searchindex.SortField) {
	sort.SliceStable(hits, func(i, j int) bool {
		for _, sf := range sortFields {
			vi := hits[i].Representation[sf.Field]
			vj := hits[j].Representation[sf.Field]
			cmp := compareValues(vi, vj)
			if cmp == 0 {
				continue
			}
			if sf.Ascending {
				return cmp < 0
			}
			return cmp > 0
		}
		return false
	})
}

// compareValues compares two arbitrary values for sorting purposes.
// Returns -1, 0, or 1.
func compareValues(a, b any) int {
	fa, aOK := toFloat64(a)
	fb, bOK := toFloat64(b)
	if aOK && bOK {
		switch {
		case fa < fb:
			return -1
		case fa > fb:
			return 1
		default:
			return 0
		}
	}

	sa := fmt.Sprintf("%v", a)
	sb := fmt.Sprintf("%v", b)
	switch {
	case sa < sb:
		return -1
	case sa > sb:
		return 1
	default:
		return 0
	}
}

// toFloat64 attempts to convert a value to float64.
func toFloat64(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case float32:
		return float64(n), true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	case int32:
		return float64(n), true
	case json.Number:
		f, err := n.Float64()
		return f, err == nil
	default:
		return 0, false
	}
}

// Autocomplete is not supported by Qdrant — it has no term dictionary API.
func (idx *Index) Autocomplete(_ context.Context, _ searchindex.AutocompleteRequest) (*searchindex.AutocompleteResult, error) {
	return nil, fmt.Errorf("qdrant: autocomplete is not supported")
}

// Close releases resources held by the index. For the HTTP-based Qdrant client,
// there is nothing to release.
func (idx *Index) Close() error {
	return nil
}

// doRequest performs an HTTP request to the Qdrant REST API.
func (idx *Index) doRequest(ctx context.Context, method, path string, body any) ([]byte, error) {
	url := idx.config.baseURL() + path

	var bodyReader io.Reader
	if body != nil {
		bodyBytes, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("qdrant: marshal request body: %w", err)
		}
		bodyReader = bytes.NewReader(bodyBytes)
	}

	req, err := http.NewRequestWithContext(ctx, method, url, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("qdrant: create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if idx.config.APIKey != "" {
		req.Header.Set("api-key", idx.config.APIKey)
	}

	resp, err := idx.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("qdrant: request %s %s: %w", method, path, err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("qdrant: read response body: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("qdrant: %s %s returned status %d: %s", method, path, resp.StatusCode, string(respBody))
	}

	return respBody, nil
}
