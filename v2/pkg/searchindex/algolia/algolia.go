// Package algolia implements the searchindex.Index interface for Algolia.
//
// It uses only the Go standard library (net/http + encoding/json) to communicate
// with the Algolia REST API. No external Algolia SDK is used.
//
// Priority: P2
// Supports: full-text SaaS search.
// Filter translation: searchindex.Filter -> Algolia filters string.
package algolia

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

// Compile-time interface checks.
var (
	_ searchindex.IndexFactory = (*Factory)(nil)
	_ searchindex.Index        = (*Index)(nil)
)

// Config holds Algolia-specific configuration.
type Config struct {
	AppID  string `json:"app_id"`
	APIKey string `json:"api_key"`
}

// Factory implements searchindex.IndexFactory for Algolia.
type Factory struct{}

// Index implements searchindex.Index for Algolia.
type Index struct {
	name   string
	config Config
	schema searchindex.IndexConfig
	client *http.Client
	hosts  []string
}

// algoliaHosts returns the ordered list of hosts for the given AppID.
// The primary read host is {AppID}-dsn.algolia.net, with fallbacks to
// {AppID}-1.algolianet.com, {AppID}-2.algolianet.com, {AppID}-3.algolianet.com.
func algoliaHosts(appID string) []string {
	return []string{
		fmt.Sprintf("%s-dsn.algolia.net", appID),
		fmt.Sprintf("%s-1.algolianet.com", appID),
		fmt.Sprintf("%s-2.algolianet.com", appID),
		fmt.Sprintf("%s-3.algolianet.com", appID),
	}
}

// algoliaWriteHosts returns the ordered list of write hosts for the given AppID.
// The primary write host is {AppID}.algolia.net, with the same fallbacks.
func algoliaWriteHosts(appID string) []string {
	return []string{
		fmt.Sprintf("%s.algolia.net", appID),
		fmt.Sprintf("%s-1.algolianet.com", appID),
		fmt.Sprintf("%s-2.algolianet.com", appID),
		fmt.Sprintf("%s-3.algolianet.com", appID),
	}
}

// CreateIndex creates a new Algolia index with the given schema configuration.
// It configures searchable attributes, faceting attributes, and ranking based on
// the provided FieldConfig entries.
func (f *Factory) CreateIndex(ctx context.Context, name string, schema searchindex.IndexConfig, configJSON []byte) (searchindex.Index, error) {
	var cfg Config
	if len(configJSON) > 0 {
		if err := json.Unmarshal(configJSON, &cfg); err != nil {
			return nil, fmt.Errorf("algolia: invalid config: %w", err)
		}
	}
	if cfg.AppID == "" || cfg.APIKey == "" {
		return nil, fmt.Errorf("algolia: app_id and api_key are required")
	}

	idx := &Index{
		name:   name,
		config: cfg,
		schema: schema,
		client: &http.Client{Timeout: 30 * time.Second},
		hosts:  algoliaWriteHosts(cfg.AppID),
	}

	// Configure the index settings via PUT /1/indexes/{indexName}/settings
	if err := idx.configureSettings(ctx); err != nil {
		return nil, fmt.Errorf("algolia: failed to configure index settings: %w", err)
	}

	return idx, nil
}

// configureSettings pushes the index settings derived from the schema to Algolia.
func (idx *Index) configureSettings(ctx context.Context) error {
	settings := make(map[string]any)

	var searchableAttrs []string
	var facetingAttrs []string

	for _, field := range idx.schema.Fields {
		switch field.Type {
		case searchindex.FieldTypeText:
			searchableAttrs = append(searchableAttrs, field.Name)
			if field.Filterable {
				facetingAttrs = append(facetingAttrs, fmt.Sprintf("searchable(%s)", field.Name))
			}
		case searchindex.FieldTypeKeyword:
			if field.Filterable {
				facetingAttrs = append(facetingAttrs, fmt.Sprintf("filterOnly(%s)", field.Name))
			}
		case searchindex.FieldTypeNumeric:
			if field.Filterable {
				facetingAttrs = append(facetingAttrs, fmt.Sprintf("filterOnly(%s)", field.Name))
			}
		case searchindex.FieldTypeBool:
			if field.Filterable {
				facetingAttrs = append(facetingAttrs, fmt.Sprintf("filterOnly(%s)", field.Name))
			}
		case searchindex.FieldTypeGeo:
			// Algolia has native _geoloc support; skip for now.
		case searchindex.FieldTypeDate, searchindex.FieldTypeDateTime:
			if field.Filterable {
				facetingAttrs = append(facetingAttrs, fmt.Sprintf("filterOnly(%s)", field.Name))
			}
		}
	}

	// Always make _typeName filterable for multi-type indices.
	facetingAttrs = append(facetingAttrs, "filterOnly(_typeName)")

	if len(searchableAttrs) > 0 {
		settings["searchableAttributes"] = searchableAttrs
	}
	if len(facetingAttrs) > 0 {
		settings["attributesForFaceting"] = facetingAttrs
	}

	// Configure custom ranking for sortable fields.
	var customRanking []string
	for _, field := range idx.schema.Fields {
		if field.Sortable {
			customRanking = append(customRanking, fmt.Sprintf("asc(%s)", field.Name))
		}
	}
	if len(customRanking) > 0 {
		settings["customRanking"] = customRanking
	}

	path := fmt.Sprintf("/1/indexes/%s/settings", url.PathEscape(idx.name))
	resp, err := idx.doRequest(ctx, http.MethodPut, path, settings)
	if err != nil {
		return err
	}

	return idx.waitForTask(ctx, resp)
}

// documentObjectID produces a deterministic objectID for a DocumentIdentity.
// Format: TypeName:key1=value1,key2=value2 (keys sorted alphabetically).
func documentObjectID(id searchindex.DocumentIdentity) string {
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
		b.WriteString(fmt.Sprintf("%v", id.KeyFields[k]))
	}
	return b.String()
}

// IndexDocument indexes a single document.
func (idx *Index) IndexDocument(ctx context.Context, doc searchindex.EntityDocument) error {
	return idx.IndexDocuments(ctx, []searchindex.EntityDocument{doc})
}

// IndexDocuments indexes a batch of documents using Algolia's batch API.
// POST /1/indexes/{indexName}/batch
func (idx *Index) IndexDocuments(ctx context.Context, docs []searchindex.EntityDocument) error {
	if len(docs) == 0 {
		return nil
	}

	requests := make([]map[string]any, 0, len(docs))
	for _, doc := range docs {
		body := make(map[string]any)
		body["objectID"] = documentObjectID(doc.Identity)
		body["_typeName"] = doc.Identity.TypeName

		keyFieldsJSON, err := json.Marshal(doc.Identity.KeyFields)
		if err != nil {
			return fmt.Errorf("algolia: failed to marshal key fields: %w", err)
		}
		body["_keyFieldsJSON"] = string(keyFieldsJSON)

		for k, v := range doc.Fields {
			body[k] = v
		}

		requests = append(requests, map[string]any{
			"action": "addObject",
			"body":   body,
		})
	}

	payload := map[string]any{
		"requests": requests,
	}

	path := fmt.Sprintf("/1/indexes/%s/batch", url.PathEscape(idx.name))
	resp, err := idx.doRequest(ctx, http.MethodPost, path, payload)
	if err != nil {
		return fmt.Errorf("algolia: batch index failed: %w", err)
	}

	return idx.waitForTask(ctx, resp)
}

// DeleteDocument deletes a single document by identity.
func (idx *Index) DeleteDocument(ctx context.Context, id searchindex.DocumentIdentity) error {
	return idx.DeleteDocuments(ctx, []searchindex.DocumentIdentity{id})
}

// DeleteDocuments deletes a batch of documents using Algolia's batch API.
// POST /1/indexes/{indexName}/batch with action "deleteObject".
func (idx *Index) DeleteDocuments(ctx context.Context, ids []searchindex.DocumentIdentity) error {
	if len(ids) == 0 {
		return nil
	}

	requests := make([]map[string]any, 0, len(ids))
	for _, id := range ids {
		requests = append(requests, map[string]any{
			"action": "deleteObject",
			"body": map[string]any{
				"objectID": documentObjectID(id),
			},
		})
	}

	payload := map[string]any{
		"requests": requests,
	}

	path := fmt.Sprintf("/1/indexes/%s/batch", url.PathEscape(idx.name))
	resp, err := idx.doRequest(ctx, http.MethodPost, path, payload)
	if err != nil {
		return fmt.Errorf("algolia: batch delete failed: %w", err)
	}

	return idx.waitForTask(ctx, resp)
}

// Search performs a search query against the Algolia index.
// POST /1/indexes/{indexName}/query
func (idx *Index) Search(ctx context.Context, req searchindex.SearchRequest) (*searchindex.SearchResult, error) {
	params := make(map[string]any)

	// Text query
	if req.TextQuery != "" {
		params["query"] = req.TextQuery
	} else {
		params["query"] = ""
	}

	// Restrict searchable attributes if specific text fields requested.
	// Per-field weights are not supported at query time by Algolia.
	if len(req.TextFields) > 0 {
		names := make([]string, len(req.TextFields))
		for i, tf := range req.TextFields {
			names[i] = tf.Name
		}
		params["restrictSearchableAttributes"] = names
	}

	// Filters
	var filterParts []string

	// TypeName filter for multi-type indices
	if req.TypeName != "" {
		filterParts = append(filterParts, fmt.Sprintf("_typeName:%s", quoteFilterValue(req.TypeName)))
	}

	// Structured filters
	if req.Filter != nil {
		filterStr := buildFilterString(req.Filter)
		if filterStr != "" {
			filterParts = append(filterParts, filterStr)
		}
	}

	if len(filterParts) > 0 {
		params["filters"] = strings.Join(filterParts, " AND ")
	}

	// Pagination: Algolia uses hitsPerPage + page (0-based)
	limit := req.Limit
	if limit <= 0 {
		limit = 20
	}
	params["hitsPerPage"] = limit

	if req.Offset > 0 {
		// Convert offset to page number (0-based)
		page := req.Offset / limit
		params["page"] = page
	} else {
		params["page"] = 0
	}

	// Facets
	if len(req.Facets) > 0 {
		facetFields := make([]string, 0, len(req.Facets))
		for _, f := range req.Facets {
			facetFields = append(facetFields, f.Field)
		}
		params["facets"] = facetFields

		// Find the maximum facet size requested
		maxFacetValues := 0
		for _, f := range req.Facets {
			if f.Size > maxFacetValues {
				maxFacetValues = f.Size
			}
		}
		if maxFacetValues > 0 {
			params["maxValuesPerFacet"] = maxFacetValues
		}
	}

	// Fuzziness / typo tolerance.
	if req.Fuzziness != nil && *req.Fuzziness == searchindex.FuzzinessExact {
		params["typoTolerance"] = false
	}

	// attributesToRetrieve: return all attributes
	params["attributesToRetrieve"] = []string{"*"}
	params["attributesToHighlight"] = []string{"*"}

	// Use the read hosts for search
	readHosts := algoliaHosts(idx.config.AppID)
	path := fmt.Sprintf("/1/indexes/%s/query", url.PathEscape(idx.name))

	resp, err := idx.doRequestWithHosts(ctx, http.MethodPost, path, params, readHosts)
	if err != nil {
		return nil, fmt.Errorf("algolia: search failed: %w", err)
	}

	return idx.parseSearchResponse(resp, req)
}

// parseSearchResponse converts an Algolia search response to a SearchResult.
func (idx *Index) parseSearchResponse(resp map[string]any, req searchindex.SearchRequest) (*searchindex.SearchResult, error) {
	result := &searchindex.SearchResult{
		Facets: make(map[string]searchindex.FacetResult),
	}

	// Parse total count
	if nbHits, ok := resp["nbHits"]; ok {
		switch v := nbHits.(type) {
		case float64:
			result.TotalCount = int(v)
		case json.Number:
			n, _ := v.Int64()
			result.TotalCount = int(n)
		}
	}

	// Parse hits
	if hitsRaw, ok := resp["hits"]; ok {
		if hits, ok := hitsRaw.([]any); ok {
			for _, hitRaw := range hits {
				hit, ok := hitRaw.(map[string]any)
				if !ok {
					continue
				}

				searchHit := searchindex.SearchHit{
					Representation: make(map[string]any),
					Highlights:     make(map[string][]string),
				}

				// Extract identity
				var typeName string
				if tn, ok := hit["_typeName"].(string); ok {
					typeName = tn
				}

				var keyFields map[string]any
				if kfJSON, ok := hit["_keyFieldsJSON"].(string); ok {
					_ = json.Unmarshal([]byte(kfJSON), &keyFields)
				}

				searchHit.Identity = searchindex.DocumentIdentity{
					TypeName:  typeName,
					KeyFields: keyFields,
				}

				// Build representation from fields (excluding internal Algolia fields)
				for k, v := range hit {
					if strings.HasPrefix(k, "_") && k != "_typeName" {
						continue
					}
					if k == "objectID" {
						continue
					}
					searchHit.Representation[k] = v
				}

				// Add __typename to representation
				if typeName != "" {
					searchHit.Representation["__typename"] = typeName
				}

				// Parse highlights from _highlightResult
				if highlightResult, ok := hit["_highlightResult"]; ok {
					if hrMap, ok := highlightResult.(map[string]any); ok {
						for field, hrVal := range hrMap {
							if strings.HasPrefix(field, "_") || field == "objectID" {
								continue
							}
							if hrField, ok := hrVal.(map[string]any); ok {
								if matchedWords, ok := hrField["matchedWords"]; ok {
									if mw, ok := matchedWords.([]any); ok && len(mw) > 0 {
										if value, ok := hrField["value"].(string); ok {
											searchHit.Highlights[field] = []string{value}
										}
									}
								}
							}
						}
					}
				}

				result.Hits = append(result.Hits, searchHit)
			}
		}
	}

	// Parse facets
	if facetsRaw, ok := resp["facets"]; ok {
		if facetsMap, ok := facetsRaw.(map[string]any); ok {
			for field, valuesRaw := range facetsMap {
				if valuesMap, ok := valuesRaw.(map[string]any); ok {
					fr := searchindex.FacetResult{}
					for value, countRaw := range valuesMap {
						count := 0
						switch c := countRaw.(type) {
						case float64:
							count = int(c)
						case json.Number:
							n, _ := c.Int64()
							count = int(n)
						}
						fr.Values = append(fr.Values, searchindex.FacetValue{
							Value: value,
							Count: count,
						})
					}
					// Sort facet values by count descending for determinism
					sort.Slice(fr.Values, func(i, j int) bool {
						return fr.Values[i].Count > fr.Values[j].Count
					})

					// Apply size limit from facet request
					for _, facetReq := range req.Facets {
						if facetReq.Field == field && facetReq.Size > 0 && len(fr.Values) > facetReq.Size {
							fr.Values = fr.Values[:facetReq.Size]
						}
					}

					result.Facets[field] = fr
				}
			}
		}
	}

	return result, nil
}

// Close releases resources held by the index.
func (idx *Index) Close() error {
	idx.client.CloseIdleConnections()
	return nil
}

// buildFilterString converts a searchindex.Filter tree into an Algolia filter string.
func buildFilterString(f *Filter) string {
	if f == nil {
		return ""
	}

	var parts []string

	// AND
	if len(f.And) > 0 {
		var andParts []string
		for _, child := range f.And {
			s := buildFilterString(child)
			if s != "" {
				andParts = append(andParts, s)
			}
		}
		if len(andParts) > 0 {
			parts = append(parts, "("+strings.Join(andParts, " AND ")+")")
		}
	}

	// OR
	if len(f.Or) > 0 {
		var orParts []string
		for _, child := range f.Or {
			s := buildFilterString(child)
			if s != "" {
				orParts = append(orParts, s)
			}
		}
		if len(orParts) > 0 {
			parts = append(parts, "("+strings.Join(orParts, " OR ")+")")
		}
	}

	// NOT
	if f.Not != nil {
		s := buildFilterString(f.Not)
		if s != "" {
			parts = append(parts, fmt.Sprintf("NOT %s", s))
		}
	}

	// Term filter: field:value
	if f.Term != nil {
		parts = append(parts, formatTermFilter(f.Term.Field, f.Term.Value))
	}

	// Terms filter (IN): field:val1 OR field:val2
	if f.Terms != nil {
		var termParts []string
		for _, v := range f.Terms.Values {
			termParts = append(termParts, formatTermFilter(f.Terms.Field, v))
		}
		if len(termParts) > 0 {
			parts = append(parts, "("+strings.Join(termParts, " OR ")+")")
		}
	}

	// Range filter
	if f.Range != nil {
		parts = append(parts, formatRangeFilter(f.Range))
	}

	// Prefix filter: not natively supported in Algolia filters,
	// approximate with field:value* won't work in filters. We use a workaround.
	if f.Prefix != nil {
		// Algolia doesn't support prefix in filters directly. Best approximation
		// is to use facet filtering. This is a limitation.
		parts = append(parts, fmt.Sprintf("%s:%s", f.Prefix.Field, quoteFilterValue(f.Prefix.Value)))
	}

	// Exists filter: Algolia doesn't have a direct exists filter.
	// We skip it as it's not expressible in Algolia's filter syntax.
	// One workaround is checking field != "" but that doesn't work for all types.

	if len(parts) == 0 {
		return ""
	}

	if len(parts) == 1 {
		return parts[0]
	}

	return "(" + strings.Join(parts, " AND ") + ")"
}

// Filter is an alias used in the buildFilterString function for the searchindex.Filter type.
type Filter = searchindex.Filter

// formatTermFilter formats a single term filter for Algolia.
func formatTermFilter(field string, value any) string {
	switch v := value.(type) {
	case bool:
		if v {
			return fmt.Sprintf("%s:true", field)
		}
		return fmt.Sprintf("%s:false", field)
	case float64:
		// Format without trailing zeros for integers
		if v == float64(int64(v)) {
			return fmt.Sprintf("%s:%d", field, int64(v))
		}
		return fmt.Sprintf("%s:%s", field, strconv.FormatFloat(v, 'f', -1, 64))
	case float32:
		return fmt.Sprintf("%s:%s", field, strconv.FormatFloat(float64(v), 'f', -1, 32))
	case int:
		return fmt.Sprintf("%s:%d", field, v)
	case int64:
		return fmt.Sprintf("%s:%d", field, v)
	case string:
		return fmt.Sprintf("%s:%s", field, quoteFilterValue(v))
	default:
		return fmt.Sprintf("%s:%s", field, quoteFilterValue(fmt.Sprintf("%v", v)))
	}
}

// formatRangeFilter formats a range filter for Algolia.
// Algolia uses: field > 10 AND field < 100
func formatRangeFilter(r *searchindex.RangeFilter) string {
	var parts []string

	if r.GTE != nil {
		parts = append(parts, fmt.Sprintf("%s >= %s", r.Field, formatNumericValue(r.GTE)))
	} else if r.HasGT && r.GT != nil {
		parts = append(parts, fmt.Sprintf("%s > %s", r.Field, formatNumericValue(r.GT)))
	}

	if r.LTE != nil {
		parts = append(parts, fmt.Sprintf("%s <= %s", r.Field, formatNumericValue(r.LTE)))
	} else if r.HasLT && r.LT != nil {
		parts = append(parts, fmt.Sprintf("%s < %s", r.Field, formatNumericValue(r.LT)))
	}

	if len(parts) == 0 {
		return ""
	}

	return strings.Join(parts, " AND ")
}

// formatNumericValue formats a numeric value for use in Algolia filters.
func formatNumericValue(v any) string {
	switch n := v.(type) {
	case float64:
		if n == float64(int64(n)) {
			return strconv.FormatInt(int64(n), 10)
		}
		return strconv.FormatFloat(n, 'f', -1, 64)
	case float32:
		return strconv.FormatFloat(float64(n), 'f', -1, 32)
	case int:
		return strconv.Itoa(n)
	case int64:
		return strconv.FormatInt(n, 10)
	case int32:
		return strconv.FormatInt(int64(n), 10)
	case string:
		return n
	default:
		return fmt.Sprintf("%v", v)
	}
}

// quoteFilterValue quotes a string value for Algolia filters if needed.
func quoteFilterValue(v string) string {
	// If value contains spaces or special characters, wrap in quotes
	if strings.ContainsAny(v, " \t\"'():<>!=") {
		return fmt.Sprintf("%q", v)
	}
	return v
}

// Autocomplete returns terms matching the given prefix using Algolia's facet search API.
func (idx *Index) Autocomplete(ctx context.Context, req searchindex.AutocompleteRequest) (*searchindex.AutocompleteResult, error) {
	limit := req.Limit
	if limit <= 0 {
		limit = 10
	}

	body := map[string]any{
		"facetQuery":   strings.ToLower(req.Prefix),
		"maxFacetHits": limit,
	}

	path := fmt.Sprintf("/1/indexes/%s/facets/%s/query",
		url.PathEscape(idx.name), url.PathEscape(req.Field))
	readHosts := algoliaHosts(idx.config.AppID)

	resp, err := idx.doRequestWithHosts(ctx, http.MethodPost, path, body, readHosts)
	if err != nil {
		return nil, fmt.Errorf("algolia: facet search failed: %w", err)
	}

	facetHits, _ := resp["facetHits"].([]any)
	terms := make([]searchindex.AutocompleteTerm, 0, len(facetHits))
	for _, hit := range facetHits {
		hitMap, ok := hit.(map[string]any)
		if !ok {
			continue
		}
		value, _ := hitMap["value"].(string)
		count := 0
		if c, ok := hitMap["count"].(float64); ok {
			count = int(c)
		}
		terms = append(terms, searchindex.AutocompleteTerm{Term: value, Count: count})
	}

	return &searchindex.AutocompleteResult{Terms: terms}, nil
}

// doRequest performs an HTTP request against the Algolia API using write hosts.
func (idx *Index) doRequest(ctx context.Context, method, path string, body any) (map[string]any, error) {
	return idx.doRequestWithHosts(ctx, method, path, body, idx.hosts)
}

// doRequestWithHosts performs an HTTP request with the given host fallback list.
func (idx *Index) doRequestWithHosts(ctx context.Context, method, path string, body any, hosts []string) (map[string]any, error) {
	var bodyReader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("algolia: failed to marshal request body: %w", err)
		}
		bodyReader = bytes.NewReader(data)
	}

	var lastErr error
	for _, host := range hosts {
		reqURL := fmt.Sprintf("https://%s%s", host, path)

		// We need to re-create the reader for each attempt since it may have been consumed.
		if body != nil {
			data, _ := json.Marshal(body)
			bodyReader = bytes.NewReader(data)
		}

		req, err := http.NewRequestWithContext(ctx, method, reqURL, bodyReader)
		if err != nil {
			lastErr = fmt.Errorf("algolia: failed to create request: %w", err)
			continue
		}

		req.Header.Set("X-Algolia-Application-Id", idx.config.AppID)
		req.Header.Set("X-Algolia-API-Key", idx.config.APIKey)
		req.Header.Set("Content-Type", "application/json; charset=UTF-8")

		resp, err := idx.client.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("algolia: request to %s failed: %w", host, err)
			continue
		}

		respBody, err := io.ReadAll(resp.Body)
		resp.Body.Close()

		if err != nil {
			lastErr = fmt.Errorf("algolia: failed to read response body: %w", err)
			continue
		}

		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			var result map[string]any
			if err := json.Unmarshal(respBody, &result); err != nil {
				return nil, fmt.Errorf("algolia: failed to parse response: %w", err)
			}
			return result, nil
		}

		// For 4xx errors, don't retry (client error)
		if resp.StatusCode >= 400 && resp.StatusCode < 500 {
			return nil, fmt.Errorf("algolia: API error (status %d): %s", resp.StatusCode, string(respBody))
		}

		// For 5xx errors, try next host
		lastErr = fmt.Errorf("algolia: API error (status %d): %s", resp.StatusCode, string(respBody))
	}

	return nil, fmt.Errorf("algolia: all hosts failed, last error: %w", lastErr)
}

// waitForTask waits for an Algolia async task to complete.
// Algolia returns a taskID for write operations. We poll until the task is "published".
func (idx *Index) waitForTask(ctx context.Context, resp map[string]any) error {
	if resp == nil {
		return nil
	}

	taskIDRaw, ok := resp["taskID"]
	if !ok {
		return nil
	}

	var taskID int64
	switch v := taskIDRaw.(type) {
	case float64:
		taskID = int64(v)
	case json.Number:
		n, err := v.Int64()
		if err != nil {
			return fmt.Errorf("algolia: invalid taskID: %w", err)
		}
		taskID = n
	default:
		return nil
	}

	path := fmt.Sprintf("/1/indexes/%s/task/%d", url.PathEscape(idx.name), taskID)
	readHosts := algoliaHosts(idx.config.AppID)

	// Poll with exponential backoff
	backoff := 100 * time.Millisecond
	maxBackoff := 5 * time.Second
	maxWait := 2 * time.Minute

	deadline := time.Now().Add(maxWait)

	for {
		if time.Now().After(deadline) {
			return fmt.Errorf("algolia: timeout waiting for task %d", taskID)
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(backoff):
		}

		result, err := idx.doRequestWithHosts(ctx, http.MethodGet, path, nil, readHosts)
		if err != nil {
			// Don't fail immediately on transient errors during polling
			backoff = backoff * 2
			if backoff > maxBackoff {
				backoff = maxBackoff
			}
			continue
		}

		status, _ := result["status"].(string)
		if status == "published" {
			return nil
		}

		backoff = backoff * 2
		if backoff > maxBackoff {
			backoff = maxBackoff
		}
	}
}
