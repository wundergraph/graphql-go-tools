// Package elasticsearch implements the searchindex.Index and searchindex.IndexFactory
// interfaces for Elasticsearch and OpenSearch.
//
// It uses only net/http and encoding/json from the standard library -- no
// external Elasticsearch SDK is required. Communication happens through the
// Elasticsearch REST API (index creation, _bulk indexing, _search, _delete_by_query).
package elasticsearch

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/searchindex"
)

// Compile-time interface conformance checks.
var (
	_ searchindex.Index        = (*Index)(nil)
	_ searchindex.IndexFactory = (*Factory)(nil)
)

// reservedTypeNameField is the Elasticsearch document field used to store the
// entity type name so we can reconstruct DocumentIdentity on search results
// and filter by TypeName in SearchRequest.
const reservedTypeNameField = "_typeName"

// reservedKeyFieldsField stores the JSON-encoded key fields map so we can
// reconstruct the DocumentIdentity from a search hit.
const reservedKeyFieldsField = "_keyFieldsJSON"

// Config holds Elasticsearch-specific configuration. It is deserialized from
// the configJSON parameter of CreateIndex.
type Config struct {
	Addresses []string `json:"addresses"`
	Username  string   `json:"username,omitempty"`
	Password  string   `json:"password,omitempty"`
	CloudID   string   `json:"cloud_id,omitempty"`
	APIKey    string   `json:"api_key,omitempty"`
}

// Factory implements searchindex.IndexFactory for Elasticsearch.
type Factory struct {
	// HTTPClient allows callers to inject a custom HTTP client (e.g. for tests).
	// If nil, http.DefaultClient is used.
	HTTPClient *http.Client
}

// NewFactory returns a new Elasticsearch IndexFactory.
func NewFactory() *Factory {
	return &Factory{}
}

// CreateIndex creates a new Elasticsearch index with mappings derived from the
// IndexConfig, then returns an Index handle.
func (f *Factory) CreateIndex(ctx context.Context, name string, schema searchindex.IndexConfig, configJSON []byte) (searchindex.Index, error) {
	var cfg Config
	if len(configJSON) > 0 {
		if err := json.Unmarshal(configJSON, &cfg); err != nil {
			return nil, fmt.Errorf("elasticsearch: invalid config: %w", err)
		}
	}
	if len(cfg.Addresses) == 0 {
		cfg.Addresses = []string{"http://localhost:9200"}
	}

	client := f.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}

	idx := &Index{
		name:   name,
		config: cfg,
		schema: schema,
		client: client,
	}

	// Build the index creation request with mappings.
	mappings := buildMappings(schema)
	body := map[string]any{
		"mappings": mappings,
		"settings": map[string]any{
			"number_of_shards":   1,
			"number_of_replicas": 0,
		},
	}

	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("elasticsearch: marshal index body: %w", err)
	}

	// PUT /{indexName}
	resp, err := idx.doRequest(ctx, http.MethodPut, "/"+name, bodyBytes)
	if err != nil {
		return nil, fmt.Errorf("elasticsearch: create index %q: %w", name, err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("elasticsearch: read create index response: %w", err)
	}

	// 200 OK or 400 with "resource_already_exists_exception" are acceptable.
	if resp.StatusCode != http.StatusOK {
		var esErr esErrorResponse
		if json.Unmarshal(respBody, &esErr) == nil && esErr.Error.Type == "resource_already_exists_exception" {
			// Index already exists; proceed.
		} else {
			return nil, fmt.Errorf("elasticsearch: create index %q: status %d: %s", name, resp.StatusCode, string(respBody))
		}
	}

	return idx, nil
}

// buildMappings converts an IndexConfig into the Elasticsearch mappings
// properties object.
func buildMappings(schema searchindex.IndexConfig) map[string]any {
	properties := make(map[string]any, len(schema.Fields)+2)

	for _, fc := range schema.Fields {
		properties[fc.Name] = fieldMapping(fc)
	}

	// Internal metadata fields.
	properties[reservedTypeNameField] = map[string]any{"type": "keyword"}
	properties[reservedKeyFieldsField] = map[string]any{"type": "keyword", "index": false}

	return map[string]any{
		"properties": properties,
	}
}

// fieldMapping returns the Elasticsearch mapping for a single field.
func fieldMapping(fc searchindex.FieldConfig) map[string]any {
	switch fc.Type {
	case searchindex.FieldTypeText:
		m := map[string]any{"type": "text"}
		// Add a keyword sub-field for sorting/aggregation if needed.
		if fc.Sortable || fc.Filterable {
			m["fields"] = map[string]any{
				"keyword": map[string]any{
					"type":         "keyword",
					"ignore_above": 256,
				},
			}
		}
		return m
	case searchindex.FieldTypeKeyword:
		return map[string]any{"type": "keyword"}
	case searchindex.FieldTypeNumeric:
		return map[string]any{"type": "double"}
	case searchindex.FieldTypeBool:
		return map[string]any{"type": "boolean"}
	case searchindex.FieldTypeVector:
		return map[string]any{
			"type":       "dense_vector",
			"dims":       fc.Dimensions,
			"index":      true,
			"similarity": "cosine",
		}
	case searchindex.FieldTypeGeo:
		return map[string]any{"type": "geo_point"}
	case searchindex.FieldTypeDate, searchindex.FieldTypeDateTime:
		return map[string]any{"type": "date"}
	default:
		return map[string]any{"type": "keyword"}
	}
}

// Index implements searchindex.Index for Elasticsearch.
type Index struct {
	name   string
	config Config
	schema searchindex.IndexConfig
	client *http.Client
}

// documentID computes a deterministic string ID from a DocumentIdentity.
// Format: TypeName:key1=val1,key2=val2,... (keys sorted alphabetically).
// This matches the Bleve implementation's convention.
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

// IndexDocument indexes a single document.
func (idx *Index) IndexDocument(ctx context.Context, doc searchindex.EntityDocument) error {
	return idx.IndexDocuments(ctx, []searchindex.EntityDocument{doc})
}

// IndexDocuments indexes a batch of documents using the _bulk API.
func (idx *Index) IndexDocuments(ctx context.Context, docs []searchindex.EntityDocument) error {
	if len(docs) == 0 {
		return nil
	}

	var buf bytes.Buffer
	for _, doc := range docs {
		id := documentID(doc.Identity)
		body, err := buildDocBody(doc)
		if err != nil {
			return err
		}

		// Action line: index
		action := map[string]any{
			"index": map[string]any{
				"_index": idx.name,
				"_id":    id,
			},
		}
		actionBytes, err := json.Marshal(action)
		if err != nil {
			return fmt.Errorf("elasticsearch: marshal bulk action: %w", err)
		}
		buf.Write(actionBytes)
		buf.WriteByte('\n')

		docBytes, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("elasticsearch: marshal document: %w", err)
		}
		buf.Write(docBytes)
		buf.WriteByte('\n')
	}

	resp, err := idx.doRequest(ctx, http.MethodPost, "/_bulk", buf.Bytes())
	if err != nil {
		return fmt.Errorf("elasticsearch: bulk index: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("elasticsearch: read bulk response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("elasticsearch: bulk index: status %d: %s", resp.StatusCode, string(respBody))
	}

	// Check for per-item errors.
	var bulkResp bulkResponse
	if err := json.Unmarshal(respBody, &bulkResp); err != nil {
		return fmt.Errorf("elasticsearch: unmarshal bulk response: %w", err)
	}
	if bulkResp.Errors {
		// Collect the first error for diagnostics.
		for _, item := range bulkResp.Items {
			if item.Index.Error != nil {
				return fmt.Errorf("elasticsearch: bulk index error: [%s] %s: %s",
					item.Index.Error.Type, item.Index.Error.Reason,
					item.Index.ID)
			}
		}
		return fmt.Errorf("elasticsearch: bulk index reported errors but no details found")
	}

	return nil
}

// buildDocBody converts an EntityDocument into a flat map for indexing.
func buildDocBody(doc searchindex.EntityDocument) (map[string]any, error) {
	m := make(map[string]any, len(doc.Fields)+len(doc.Vectors)+2)
	for k, v := range doc.Fields {
		m[k] = v
	}
	for k, v := range doc.Vectors {
		m[k] = v
	}
	m[reservedTypeNameField] = doc.Identity.TypeName

	keyFieldsJSON, err := json.Marshal(doc.Identity.KeyFields)
	if err != nil {
		return nil, fmt.Errorf("elasticsearch: marshal key fields: %w", err)
	}
	m[reservedKeyFieldsField] = string(keyFieldsJSON)
	return m, nil
}

// DeleteDocument deletes a single document by identity.
func (idx *Index) DeleteDocument(ctx context.Context, id searchindex.DocumentIdentity) error {
	return idx.DeleteDocuments(ctx, []searchindex.DocumentIdentity{id})
}

// DeleteDocuments deletes a batch of documents using the _bulk API.
func (idx *Index) DeleteDocuments(ctx context.Context, ids []searchindex.DocumentIdentity) error {
	if len(ids) == 0 {
		return nil
	}

	var buf bytes.Buffer
	for _, id := range ids {
		docID := documentID(id)
		action := map[string]any{
			"delete": map[string]any{
				"_index": idx.name,
				"_id":    docID,
			},
		}
		actionBytes, err := json.Marshal(action)
		if err != nil {
			return fmt.Errorf("elasticsearch: marshal bulk delete action: %w", err)
		}
		buf.Write(actionBytes)
		buf.WriteByte('\n')
	}

	resp, err := idx.doRequest(ctx, http.MethodPost, "/_bulk", buf.Bytes())
	if err != nil {
		return fmt.Errorf("elasticsearch: bulk delete: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("elasticsearch: read bulk delete response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("elasticsearch: bulk delete: status %d: %s", resp.StatusCode, string(respBody))
	}

	return nil
}

// Search builds an Elasticsearch query from the SearchRequest and executes it.
func (idx *Index) Search(ctx context.Context, req searchindex.SearchRequest) (*searchindex.SearchResult, error) {
	esQuery := idx.buildSearchBody(req)

	bodyBytes, err := json.Marshal(esQuery)
	if err != nil {
		return nil, fmt.Errorf("elasticsearch: marshal search body: %w", err)
	}

	path := "/" + idx.name + "/_search"
	resp, err := idx.doRequest(ctx, http.MethodPost, path, bodyBytes)
	if err != nil {
		return nil, fmt.Errorf("elasticsearch: search: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("elasticsearch: read search response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("elasticsearch: search: status %d: %s", resp.StatusCode, string(respBody))
	}

	var esResp esSearchResponse
	if err := json.Unmarshal(respBody, &esResp); err != nil {
		return nil, fmt.Errorf("elasticsearch: unmarshal search response: %w", err)
	}

	return idx.convertSearchResponse(esResp, req.GeoDistanceSort != nil), nil
}

// Close releases resources. For the HTTP-based implementation there is
// nothing to close, but we send a DELETE for the index to clean up if desired.
// In practice callers should manage index lifecycle separately; Close is a
// no-op here.
func (idx *Index) Close() error {
	return nil
}

// ---------------------------------------------------------------------------
// Query building
// ---------------------------------------------------------------------------

// buildSearchBody constructs the full ES search request body.
func (idx *Index) buildSearchBody(req searchindex.SearchRequest) map[string]any {
	body := make(map[string]any)

	// Size / from.
	limit := effectiveLimit(req.Limit)
	body["size"] = limit
	if len(req.SearchAfter) > 0 {
		// Cursor mode: use search_after, no from.
		body["search_after"] = req.SearchAfter
	} else if req.Offset > 0 {
		body["from"] = req.Offset
	}

	// Build the main query.
	var mustClauses []any
	var filterClauses []any

	// Text query.
	if req.TextQuery != "" {
		if len(req.TextFields) > 0 {
			fields := make([]string, len(req.TextFields))
			for i, tf := range req.TextFields {
				if tf.Weight != 0 && tf.Weight != 1.0 {
					fields[i] = fmt.Sprintf("%s^%g", tf.Name, tf.Weight)
				} else {
					fields[i] = tf.Name
				}
			}
			mm := map[string]any{
				"query":  req.TextQuery,
				"fields": fields,
			}
			if req.Fuzziness != nil {
				mm["fuzziness"] = int(*req.Fuzziness)
			}
			mustClauses = append(mustClauses, map[string]any{
				"multi_match": mm,
			})
		} else {
			// Search across all text fields from the schema.
			textFields := idx.allTextFields()
			if len(textFields) > 0 {
				mustClauses = append(mustClauses, map[string]any{
					"multi_match": map[string]any{
						"query":  req.TextQuery,
						"fields": textFields,
					},
				})
			} else {
				// Use simple_query_string instead of query_string to prevent
				// Lucene query syntax injection (field targeting, regex, wildcards).
				mustClauses = append(mustClauses, map[string]any{
					"simple_query_string": map[string]any{
						"query": req.TextQuery,
					},
				})
			}
		}
	}

	// TypeName filter.
	if req.TypeName != "" {
		filterClauses = append(filterClauses, map[string]any{
			"term": map[string]any{
				reservedTypeNameField: req.TypeName,
			},
		})
	}

	// Structured filter.
	if req.Filter != nil {
		fq := translateFilter(req.Filter)
		if fq != nil {
			filterClauses = append(filterClauses, fq)
		}
	}

	// Vector (kNN) query.
	hasKNN := len(req.Vector) > 0 && req.VectorField != ""
	if hasKNN {
		knnK := limit
		knnCandidates := limit * 2
		if req.TextQuery != "" {
			// For hybrid search, fetch more kNN candidates for better RRF fusion.
			knnK = limit * 3
			if knnK < 100 {
				knnK = 100
			}
			knnCandidates = knnK * 2
		}
		knnQuery := map[string]any{
			"field":          req.VectorField,
			"query_vector":   req.Vector,
			"k":              knnK,
			"num_candidates": knnCandidates,
		}
		// Apply filters inside kNN so they are enforced during candidate selection.
		if len(filterClauses) > 0 {
			if len(filterClauses) == 1 {
				knnQuery["filter"] = filterClauses[0]
			} else {
				knnQuery["filter"] = map[string]any{
					"bool": map[string]any{
						"filter": filterClauses,
					},
				}
			}
		}
		body["knn"] = knnQuery
	}

	// Assemble the bool query.
	if len(mustClauses) > 0 || len(filterClauses) > 0 {
		boolQuery := make(map[string]any)
		if len(mustClauses) > 0 {
			boolQuery["must"] = mustClauses
		}
		if len(filterClauses) > 0 {
			boolQuery["filter"] = filterClauses
		}
		body["query"] = map[string]any{"bool": boolQuery}
	} else if len(req.Vector) == 0 {
		// No text, no filter, no vector: match all.
		body["query"] = map[string]any{"match_all": map[string]any{}}
	}

	// Sorting.
	var sortClauses []any
	if len(req.Sort) > 0 {
		sortClauses = make([]any, 0, len(req.Sort))
		for _, sf := range req.Sort {
			order := "asc"
			if !sf.Ascending {
				order = "desc"
			}
			fieldName := idx.sortFieldName(sf.Field)
			sortClauses = append(sortClauses, map[string]any{
				fieldName: map[string]any{"order": order},
			})
		}
	}
	if req.GeoDistanceSort != nil {
		order := "asc"
		if !req.GeoDistanceSort.Ascending {
			order = "desc"
		}
		unit := req.GeoDistanceSort.Unit
		if unit == "" {
			unit = "km"
		}
		sortClauses = append(sortClauses, map[string]any{
			"_geo_distance": map[string]any{
				req.GeoDistanceSort.Field: map[string]any{
					"lat": req.GeoDistanceSort.Center.Lat,
					"lon": req.GeoDistanceSort.Center.Lon,
				},
				"order": order,
				"unit":  unit,
			},
		})
	}
	if len(sortClauses) > 0 {
		body["sort"] = sortClauses
	}

	// Facets (aggregations).
	if len(req.Facets) > 0 {
		aggs := make(map[string]any, len(req.Facets))
		for _, fr := range req.Facets {
			size := fr.Size
			if size <= 0 {
				size = 10
			}
			aggFieldName := idx.aggFieldName(fr.Field)
			aggs[fr.Field] = map[string]any{
				"terms": map[string]any{
					"field": aggFieldName,
					"size":  size,
				},
			}
		}
		body["aggs"] = aggs
	}

	// Highlights: request highlighted fragments for all text fields when a text query is present.
	if req.TextQuery != "" {
		hlFields := make(map[string]any)
		if len(req.TextFields) > 0 {
			for _, tf := range req.TextFields {
				hlFields[tf.Name] = map[string]any{}
			}
		} else {
			for _, fc := range idx.schema.Fields {
				if fc.Type == searchindex.FieldTypeText {
					hlFields[fc.Name] = map[string]any{}
				}
			}
		}
		if len(hlFields) > 0 {
			body["highlight"] = map[string]any{
				"fields": hlFields,
			}
		}
	}

	// When both text query and kNN are present, Elasticsearch combines the
	// scores from both the query and kNN clauses automatically. We do not use
	// rank.rrf because it requires a paid (platinum+) license.

	return body
}

// allTextFields returns the names of all text-type fields in the schema,
// with optional boost syntax (e.g. "name^2") when Weight is set.
func (idx *Index) allTextFields() []string {
	var fields []string
	for _, fc := range idx.schema.Fields {
		if fc.Type == searchindex.FieldTypeText {
			if fc.Weight != 0 && fc.Weight != 1.0 {
				fields = append(fields, fmt.Sprintf("%s^%g", fc.Name, fc.Weight))
			} else {
				fields = append(fields, fc.Name)
			}
		}
	}
	return fields
}

// sortFieldName returns the appropriate field name for sorting. Text fields
// need the .keyword sub-field for sorting.
func (idx *Index) sortFieldName(field string) string {
	for _, fc := range idx.schema.Fields {
		if fc.Name == field && fc.Type == searchindex.FieldTypeText {
			return field + ".keyword"
		}
	}
	return field
}

// aggFieldName returns the appropriate field name for aggregations. Text
// fields need the .keyword sub-field.
func (idx *Index) aggFieldName(field string) string {
	return idx.sortFieldName(field)
}

// effectiveLimit returns a sensible default if limit is zero or negative.
func effectiveLimit(limit int) int {
	if limit <= 0 {
		return 10
	}
	return limit
}

// ---------------------------------------------------------------------------
// Filter translation
// ---------------------------------------------------------------------------

// translateFilter recursively converts a searchindex.Filter tree to an
// Elasticsearch query DSL map.
func translateFilter(f *searchindex.Filter) map[string]any {
	if f == nil {
		return nil
	}

	// AND
	if len(f.And) > 0 {
		children := make([]any, 0, len(f.And))
		for _, child := range f.And {
			cq := translateFilter(child)
			if cq != nil {
				children = append(children, cq)
			}
		}
		if len(children) == 0 {
			return nil
		}
		return map[string]any{
			"bool": map[string]any{
				"must": children,
			},
		}
	}

	// OR
	if len(f.Or) > 0 {
		children := make([]any, 0, len(f.Or))
		for _, child := range f.Or {
			cq := translateFilter(child)
			if cq != nil {
				children = append(children, cq)
			}
		}
		if len(children) == 0 {
			return nil
		}
		return map[string]any{
			"bool": map[string]any{
				"should":               children,
				"minimum_should_match": 1,
			},
		}
	}

	// NOT
	if f.Not != nil {
		inner := translateFilter(f.Not)
		if inner == nil {
			return nil
		}
		return map[string]any{
			"bool": map[string]any{
				"must_not": []any{inner},
			},
		}
	}

	// Term
	if f.Term != nil {
		return map[string]any{
			"term": map[string]any{
				f.Term.Field: f.Term.Value,
			},
		}
	}

	// Terms (IN)
	if f.Terms != nil {
		return map[string]any{
			"terms": map[string]any{
				f.Terms.Field: f.Terms.Values,
			},
		}
	}

	// Range
	if f.Range != nil {
		return translateRangeFilter(f.Range)
	}

	// Prefix
	if f.Prefix != nil {
		return map[string]any{
			"prefix": map[string]any{
				f.Prefix.Field: f.Prefix.Value,
			},
		}
	}

	// Exists
	if f.Exists != nil {
		return map[string]any{
			"exists": map[string]any{
				"field": f.Exists.Field,
			},
		}
	}

	// Geo distance
	if f.GeoDistance != nil {
		return map[string]any{
			"geo_distance": map[string]any{
				"distance": f.GeoDistance.Distance,
				f.GeoDistance.Field: map[string]any{
					"lat": f.GeoDistance.Center.Lat,
					"lon": f.GeoDistance.Center.Lon,
				},
			},
		}
	}

	// Geo bounding box
	if f.GeoBoundingBox != nil {
		return map[string]any{
			"geo_bounding_box": map[string]any{
				f.GeoBoundingBox.Field: map[string]any{
					"top_left": map[string]any{
						"lat": f.GeoBoundingBox.TopLeft.Lat,
						"lon": f.GeoBoundingBox.TopLeft.Lon,
					},
					"bottom_right": map[string]any{
						"lat": f.GeoBoundingBox.BottomRight.Lat,
						"lon": f.GeoBoundingBox.BottomRight.Lon,
					},
				},
			},
		}
	}

	return nil
}

// translateRangeFilter converts a RangeFilter to an Elasticsearch range query.
func translateRangeFilter(rf *searchindex.RangeFilter) map[string]any {
	rangeClause := make(map[string]any)

	if rf.GTE != nil {
		rangeClause["gte"] = rf.GTE
	} else if rf.HasGT && rf.GT != nil {
		rangeClause["gt"] = rf.GT
	}

	if rf.LTE != nil {
		rangeClause["lte"] = rf.LTE
	} else if rf.HasLT && rf.LT != nil {
		rangeClause["lt"] = rf.LT
	}

	if len(rangeClause) == 0 {
		return nil
	}

	return map[string]any{
		"range": map[string]any{
			rf.Field: rangeClause,
		},
	}
}

// ---------------------------------------------------------------------------
// Response parsing
// ---------------------------------------------------------------------------

// convertSearchResponse transforms the raw ES response into a SearchResult.
func (idx *Index) convertSearchResponse(resp esSearchResponse, hasGeoSort bool) *searchindex.SearchResult {
	hits := make([]searchindex.SearchHit, 0, len(resp.Hits.Hits))
	for _, hit := range resp.Hits.Hits {
		sh := idx.convertHit(hit, hasGeoSort)
		hits = append(hits, sh)
	}

	facets := convertAggregations(resp.Aggregations)

	totalCount := resp.Hits.Total.Value

	return &searchindex.SearchResult{
		Hits:       hits,
		TotalCount: totalCount,
		Facets:     facets,
	}
}

// convertHit transforms a single ES hit into a SearchHit.
func (idx *Index) convertHit(hit esHit, hasGeoSort bool) searchindex.SearchHit {
	identity := extractIdentity(hit.Source)

	// Build representation from source, excluding internal fields.
	representation := make(map[string]any, len(hit.Source))
	for k, v := range hit.Source {
		if k == reservedTypeNameField || k == reservedKeyFieldsField {
			continue
		}
		representation[k] = v
	}
	representation["__typename"] = identity.TypeName
	for k, v := range identity.KeyFields {
		representation[k] = v
	}

	// Highlights.
	var highlights map[string][]string
	if len(hit.Highlight) > 0 {
		highlights = hit.Highlight
	}

	// Populate SortValues for cursor-based pagination.
	var sortValues []string
	if len(hit.Sort) > 0 {
		sortValues = make([]string, len(hit.Sort))
		for i, v := range hit.Sort {
			sortValues[i] = fmt.Sprintf("%v", v)
		}
	}

	// When geo distance sort is active, ES appends the distance as the last sort value.
	var geoDistance *float64
	if hasGeoSort && len(hit.Sort) > 0 {
		if dist, ok := hit.Sort[len(hit.Sort)-1].(float64); ok {
			geoDistance = &dist
		}
	}

	return searchindex.SearchHit{
		Identity:       identity,
		Score:          hit.Score,
		Highlights:     highlights,
		Representation: representation,
		SortValues:     sortValues,
		GeoDistance:    geoDistance,
	}
}

// extractIdentity reconstructs a DocumentIdentity from the _source fields.
func extractIdentity(source map[string]any) searchindex.DocumentIdentity {
	typeName, _ := source[reservedTypeNameField].(string)
	keyFieldsRaw, _ := source[reservedKeyFieldsField].(string)

	var keyFields map[string]any
	if keyFieldsRaw != "" {
		_ = json.Unmarshal([]byte(keyFieldsRaw), &keyFields)
	}
	if keyFields == nil {
		keyFields = make(map[string]any)
	}

	return searchindex.DocumentIdentity{
		TypeName:  typeName,
		KeyFields: keyFields,
	}
}

// convertAggregations converts ES aggregation results to searchindex facets.
func convertAggregations(aggs map[string]esAggResult) map[string]searchindex.FacetResult {
	if len(aggs) == 0 {
		return nil
	}
	facets := make(map[string]searchindex.FacetResult, len(aggs))
	for name, agg := range aggs {
		values := make([]searchindex.FacetValue, 0, len(agg.Buckets))
		for _, bucket := range agg.Buckets {
			values = append(values, searchindex.FacetValue{
				Value: fmt.Sprintf("%v", bucket.Key),
				Count: bucket.DocCount,
			})
		}
		facets[name] = searchindex.FacetResult{Values: values}
	}
	return facets
}

// Autocomplete returns terms from the Elasticsearch index matching the given prefix.
// Uses a prefix query to find matching documents and extracts unique terms from the
// field values. This is more reliable than the _terms_enum API for text fields.
func (idx *Index) Autocomplete(ctx context.Context, req searchindex.AutocompleteRequest) (*searchindex.AutocompleteResult, error) {
	limit := req.Limit
	if limit <= 0 {
		limit = 10
	}

	prefix := strings.ToLower(req.Prefix)

	body := map[string]any{
		"query": map[string]any{
			"prefix": map[string]any{
				req.Field: map[string]any{
					"value": prefix,
				},
			},
		},
		"size":    100,
		"_source": []string{req.Field},
	}
	bodyJSON, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("elasticsearch: marshal autocomplete body: %w", err)
	}

	resp, err := idx.doRequest(ctx, "POST", "/"+idx.name+"/_search", bodyJSON)
	if err != nil {
		return nil, fmt.Errorf("elasticsearch: autocomplete search request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("elasticsearch: read autocomplete response: %w", err)
	}

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("elasticsearch: autocomplete search failed (HTTP %d): %s", resp.StatusCode, string(respBody))
	}

	var searchResult struct {
		Hits struct {
			Hits []struct {
				Source map[string]any `json:"_source"`
			} `json:"hits"`
		} `json:"hits"`
	}
	if err := json.Unmarshal(respBody, &searchResult); err != nil {
		return nil, fmt.Errorf("elasticsearch: unmarshal autocomplete response: %w", err)
	}

	// Extract unique terms from field values that match the prefix.
	termCounts := make(map[string]int)
	for _, hit := range searchResult.Hits.Hits {
		val, ok := hit.Source[req.Field]
		if !ok {
			continue
		}
		text, ok := val.(string)
		if !ok {
			continue
		}
		// Tokenize: split on non-alphanumeric boundaries and lowercase.
		for _, token := range tokenize(text) {
			if strings.HasPrefix(token, prefix) {
				termCounts[token]++
			}
		}
	}

	terms := make([]searchindex.AutocompleteTerm, 0, len(termCounts))
	for term, count := range termCounts {
		terms = append(terms, searchindex.AutocompleteTerm{Term: term, Count: count})
	}
	sort.Slice(terms, func(i, j int) bool {
		if terms[i].Count != terms[j].Count {
			return terms[i].Count > terms[j].Count
		}
		return terms[i].Term < terms[j].Term
	})
	if len(terms) > limit {
		terms = terms[:limit]
	}

	return &searchindex.AutocompleteResult{Terms: terms}, nil
}

// tokenize splits text into lowercase tokens, mimicking Elasticsearch's standard analyzer.
func tokenize(text string) []string {
	var tokens []string
	var current strings.Builder
	for _, r := range strings.ToLower(text) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			current.WriteRune(r)
		} else if current.Len() > 0 {
			tokens = append(tokens, current.String())
			current.Reset()
		}
	}
	if current.Len() > 0 {
		tokens = append(tokens, current.String())
	}
	return tokens
}

// ---------------------------------------------------------------------------
// HTTP helpers
// ---------------------------------------------------------------------------

// doRequest performs an HTTP request against the first available ES address.
func (idx *Index) doRequest(ctx context.Context, method, path string, body []byte) (*http.Response, error) {
	// Use the first address for simplicity. A production implementation would
	// rotate or load-balance.
	baseURL := strings.TrimRight(idx.config.Addresses[0], "/")
	url := baseURL + path

	var bodyReader io.Reader
	if body != nil {
		bodyReader = bytes.NewReader(body)
	}

	req, err := http.NewRequestWithContext(ctx, method, url, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("elasticsearch: create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	// Authentication.
	if idx.config.APIKey != "" {
		req.Header.Set("Authorization", "ApiKey "+idx.config.APIKey)
	} else if idx.config.Username != "" {
		req.SetBasicAuth(idx.config.Username, idx.config.Password)
	}

	return idx.client.Do(req)
}

// ---------------------------------------------------------------------------
// Elasticsearch response types
// ---------------------------------------------------------------------------

// esErrorResponse is the top-level error envelope from ES.
type esErrorResponse struct {
	Error struct {
		Type   string `json:"type"`
		Reason string `json:"reason"`
	} `json:"error"`
	Status int `json:"status"`
}

// bulkResponse is the response from the _bulk API.
type bulkResponse struct {
	Errors bool       `json:"errors"`
	Items  []bulkItem `json:"items"`
}

type bulkItem struct {
	Index  bulkItemResult `json:"index"`
	Delete bulkItemResult `json:"delete"`
}

type bulkItemResult struct {
	ID     string       `json:"_id"`
	Status int          `json:"status"`
	Error  *bulkItemErr `json:"error,omitempty"`
}

type bulkItemErr struct {
	Type   string `json:"type"`
	Reason string `json:"reason"`
}

// esSearchResponse is the top-level search response from ES.
type esSearchResponse struct {
	Hits         esHitsWrapper          `json:"hits"`
	Aggregations map[string]esAggResult `json:"aggregations"`
}

type esHitsWrapper struct {
	Total esTotal `json:"total"`
	Hits  []esHit `json:"hits"`
}

type esTotal struct {
	Value    int    `json:"value"`
	Relation string `json:"relation"`
}

type esHit struct {
	Index     string              `json:"_index"`
	ID        string              `json:"_id"`
	Score     float64             `json:"_score"`
	Source    map[string]any      `json:"_source"`
	Highlight map[string][]string `json:"highlight,omitempty"`
	Sort      []any               `json:"sort,omitempty"`
}

type esAggResult struct {
	Buckets []esAggBucket `json:"buckets"`
}

type esAggBucket struct {
	Key      any `json:"key"`
	DocCount int `json:"doc_count"`
}
