// Package meilisearch implements the searchindex.Index and searchindex.IndexFactory
// interfaces backed by a Meilisearch server. It uses only net/http and encoding/json
// for communication -- no external SDK is required.
//
// Supports: full-text search with typo tolerance, structured filtering, sorting, facets.
// Filter translation: searchindex.Filter -> Meilisearch filter string syntax.
package meilisearch

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

// reservedDocIDField is the Meilisearch primary key field.
const reservedDocIDField = "_docId"

// reservedTypeNameField stores the entity type name for DocumentIdentity reconstruction.
const reservedTypeNameField = "_typeName"

// reservedKeyFieldsField stores the JSON-encoded key fields for DocumentIdentity reconstruction.
const reservedKeyFieldsField = "_keyFieldsJSON"

// taskPollInterval is the interval between task status polls.
const taskPollInterval = 100 * time.Millisecond

// taskPollTimeout is the maximum time to wait for a task to complete.
const taskPollTimeout = 30 * time.Second

// Config holds Meilisearch-specific configuration.
type Config struct {
	Host   string `json:"host"`
	APIKey string `json:"api_key,omitempty"`
}

// Factory implements searchindex.IndexFactory for Meilisearch.
type Factory struct{}

// NewFactory returns a new Meilisearch IndexFactory.
func NewFactory() *Factory {
	return &Factory{}
}

// CreateIndex creates a new Meilisearch index with the given name and configuration.
// It creates the index via the Meilisearch API, then configures filterable and sortable
// attributes based on the IndexConfig schema.
func (f *Factory) CreateIndex(ctx context.Context, name string, schema searchindex.IndexConfig, configJSON []byte) (searchindex.Index, error) {
	var cfg Config
	if len(configJSON) > 0 {
		if err := json.Unmarshal(configJSON, &cfg); err != nil {
			return nil, fmt.Errorf("meilisearch: invalid config: %w", err)
		}
	}
	if cfg.Host == "" {
		cfg.Host = "http://localhost:7700"
	}
	// Normalize: strip trailing slash.
	cfg.Host = strings.TrimRight(cfg.Host, "/")

	idx := &Index{
		name:   name,
		config: cfg,
		schema: schema,
		client: &http.Client{},
	}

	// Step 1: Create the index.
	createBody := map[string]string{
		"uid":        name,
		"primaryKey": reservedDocIDField,
	}
	taskUID, err := idx.doTaskRequest(ctx, http.MethodPost, "/indexes", createBody)
	if err != nil {
		return nil, fmt.Errorf("meilisearch: create index %q: %w", name, err)
	}
	if err := idx.waitForTask(ctx, taskUID); err != nil {
		return nil, fmt.Errorf("meilisearch: create index %q wait: %w", name, err)
	}

	// Step 2: Configure filterable and sortable attributes from the schema.
	filterable, sortable := deriveAttributes(schema)
	if len(filterable) > 0 || len(sortable) > 0 {
		settings := map[string]any{}
		if len(filterable) > 0 {
			settings["filterableAttributes"] = filterable
		}
		if len(sortable) > 0 {
			settings["sortableAttributes"] = sortable
		}
		taskUID, err = idx.doTaskRequest(ctx, http.MethodPatch, "/indexes/"+name+"/settings", settings)
		if err != nil {
			return nil, fmt.Errorf("meilisearch: configure settings for %q: %w", name, err)
		}
		if err := idx.waitForTask(ctx, taskUID); err != nil {
			return nil, fmt.Errorf("meilisearch: configure settings for %q wait: %w", name, err)
		}
	}

	return idx, nil
}

// deriveAttributes computes filterable and sortable attribute lists from an IndexConfig.
// The reserved metadata fields are always included as filterable.
func deriveAttributes(schema searchindex.IndexConfig) (filterable, sortable []string) {
	filterableSet := map[string]struct{}{
		reservedTypeNameField: {},
	}
	sortableSet := map[string]struct{}{}

	for _, fc := range schema.Fields {
		if fc.Filterable || fc.Autocomplete {
			filterableSet[fc.Name] = struct{}{}
		}
		if fc.Sortable {
			sortableSet[fc.Name] = struct{}{}
		}
	}

	filterable = make([]string, 0, len(filterableSet))
	for k := range filterableSet {
		filterable = append(filterable, k)
	}
	sort.Strings(filterable)

	sortable = make([]string, 0, len(sortableSet))
	for k := range sortableSet {
		sortable = append(sortable, k)
	}
	sort.Strings(sortable)

	return filterable, sortable
}

// Index implements searchindex.Index backed by a Meilisearch server.
type Index struct {
	name   string
	config Config
	schema searchindex.IndexConfig
	client *http.Client
}

// documentID computes a deterministic string ID from a DocumentIdentity.
// Meilisearch only allows alphanumeric characters, hyphens (-), and underscores (_).
// Format: TypeName_key1-val1_key2-val2 (keys sorted alphabetically).
func documentID(id searchindex.DocumentIdentity) string {
	if len(id.KeyFields) == 0 {
		return sanitizeMeiliID(id.TypeName)
	}
	keys := make([]string, 0, len(id.KeyFields))
	for k := range id.KeyFields {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var b strings.Builder
	b.WriteString(sanitizeMeiliID(id.TypeName))
	b.WriteByte('_')
	for i, k := range keys {
		if i > 0 {
			b.WriteByte('_')
		}
		b.WriteString(sanitizeMeiliID(k))
		b.WriteByte('-')
		b.WriteString(sanitizeMeiliID(fmt.Sprintf("%v", id.KeyFields[k])))
	}
	return b.String()
}

// sanitizeMeiliID replaces characters not allowed in Meilisearch document IDs.
func sanitizeMeiliID(s string) string {
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// buildDoc converts an EntityDocument into a flat map suitable for Meilisearch indexing.
// It includes all Fields, the _docId primary key, and internal metadata fields.
func buildDoc(doc searchindex.EntityDocument) (map[string]any, error) {
	m := make(map[string]any, len(doc.Fields)+3)
	for k, v := range doc.Fields {
		m[k] = v
	}
	m[reservedDocIDField] = documentID(doc.Identity)
	m[reservedTypeNameField] = doc.Identity.TypeName

	keyFieldsJSON, err := json.Marshal(doc.Identity.KeyFields)
	if err != nil {
		return nil, fmt.Errorf("meilisearch: failed to marshal key fields: %w", err)
	}
	m[reservedKeyFieldsField] = string(keyFieldsJSON)
	return m, nil
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

// dateToUnix parses an ISO 8601 date or datetime string and returns the unix timestamp.
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
	return 0, fmt.Errorf("meilisearch: cannot parse date %q", s)
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
		convertRangeValue := func(v any) any {
			if s, ok := v.(string); ok {
				if ts, err := dateToUnix(s); err == nil {
					return ts
				}
			}
			return v
		}
		if f.Range.GT != nil {
			f.Range.GT = convertRangeValue(f.Range.GT)
		}
		if f.Range.GTE != nil {
			f.Range.GTE = convertRangeValue(f.Range.GTE)
		}
		if f.Range.LT != nil {
			f.Range.LT = convertRangeValue(f.Range.LT)
		}
		if f.Range.LTE != nil {
			f.Range.LTE = convertRangeValue(f.Range.LTE)
		}
	}
}

// IndexDocument indexes a single document.
func (idx *Index) IndexDocument(ctx context.Context, doc searchindex.EntityDocument) error {
	return idx.IndexDocuments(ctx, []searchindex.EntityDocument{doc})
}

// IndexDocuments indexes a batch of documents.
func (idx *Index) IndexDocuments(ctx context.Context, docs []searchindex.EntityDocument) error {
	dateFields := idx.dateFieldSet()
	msDocs := make([]map[string]any, 0, len(docs))
	for _, doc := range docs {
		m, err := buildDoc(doc)
		if err != nil {
			return err
		}
		if len(dateFields) > 0 {
			if err := convertDateFieldsInDoc(m, dateFields); err != nil {
				return err
			}
		}
		msDocs = append(msDocs, m)
	}

	taskUID, err := idx.doTaskRequest(ctx, http.MethodPost, "/indexes/"+idx.name+"/documents", msDocs)
	if err != nil {
		return fmt.Errorf("meilisearch: index documents: %w", err)
	}
	return idx.waitForTask(ctx, taskUID)
}

// DeleteDocument deletes a single document by identity.
func (idx *Index) DeleteDocument(ctx context.Context, id searchindex.DocumentIdentity) error {
	return idx.DeleteDocuments(ctx, []searchindex.DocumentIdentity{id})
}

// DeleteDocuments deletes a batch of documents by identity.
func (idx *Index) DeleteDocuments(ctx context.Context, ids []searchindex.DocumentIdentity) error {
	docIDs := make([]string, 0, len(ids))
	for _, id := range ids {
		docIDs = append(docIDs, documentID(id))
	}

	taskUID, err := idx.doTaskRequest(ctx, http.MethodPost, "/indexes/"+idx.name+"/documents/delete-batch", docIDs)
	if err != nil {
		return fmt.Errorf("meilisearch: delete documents: %w", err)
	}
	return idx.waitForTask(ctx, taskUID)
}

// Search performs a search query and returns results.
func (idx *Index) Search(ctx context.Context, req searchindex.SearchRequest) (*searchindex.SearchResult, error) {
	body := map[string]any{}

	// Text query.
	if req.TextQuery != "" {
		body["q"] = req.TextQuery
	} else {
		body["q"] = ""
	}

	// Build filter string.
	filterParts := []string{}
	if req.TypeName != "" {
		filterParts = append(filterParts, fmt.Sprintf("%s = %q", reservedTypeNameField, req.TypeName))
	}
	if req.Filter != nil {
		dateFields := idx.dateFieldSet()
		if len(dateFields) > 0 {
			convertDateFilters(req.Filter, dateFields)
		}
		filterStr, err := translateFilter(req.Filter)
		if err != nil {
			return nil, err
		}
		if filterStr != "" {
			filterParts = append(filterParts, filterStr)
		}
	}
	if len(filterParts) > 0 {
		body["filter"] = strings.Join(filterParts, " AND ")
	}

	// Sort.
	if len(req.Sort) > 0 {
		sortArr := make([]string, 0, len(req.Sort))
		for _, sf := range req.Sort {
			dir := "desc"
			if sf.Ascending {
				dir = "asc"
			}
			sortArr = append(sortArr, sf.Field+":"+dir)
		}
		body["sort"] = sortArr
	}

	// Limit and offset.
	limit := req.Limit
	if limit <= 0 {
		limit = 10
	}
	body["limit"] = limit
	if req.Offset > 0 {
		body["offset"] = req.Offset
	}

	// Facets.
	if len(req.Facets) > 0 {
		facetFields := make([]string, 0, len(req.Facets))
		for _, fr := range req.Facets {
			facetFields = append(facetFields, fr.Field)
		}
		body["facets"] = facetFields
	}

	// Text field restriction: Meilisearch supports attributesToSearchOn.
	// Per-field weights are not supported at query time by Meilisearch.
	if len(req.TextFields) > 0 {
		names := make([]string, len(req.TextFields))
		for i, tf := range req.TextFields {
			names[i] = tf.Name
		}
		body["attributesToSearchOn"] = names
	}

	respBody, err := idx.doRequest(ctx, http.MethodPost, "/indexes/"+idx.name+"/search", body)
	if err != nil {
		return nil, fmt.Errorf("meilisearch: search: %w", err)
	}

	var msResult meiliSearchResponse
	if err := json.Unmarshal(respBody, &msResult); err != nil {
		return nil, fmt.Errorf("meilisearch: unmarshal search response: %w", err)
	}

	hits := make([]searchindex.SearchHit, 0, len(msResult.Hits))
	for _, hitRaw := range msResult.Hits {
		var hitMap map[string]any
		if err := json.Unmarshal(hitRaw, &hitMap); err != nil {
			return nil, fmt.Errorf("meilisearch: unmarshal hit: %w", err)
		}
		sh, err := convertHit(hitMap)
		if err != nil {
			return nil, err
		}
		hits = append(hits, sh)
	}

	facets := convertFacets(msResult.FacetDistribution)

	return &searchindex.SearchResult{
		Hits:       hits,
		TotalCount: msResult.EstimatedTotalHits,
		Facets:     facets,
	}, nil
}

// Autocomplete returns terms matching the given prefix using a search query.
// Searches for documents containing terms that match the prefix, then extracts
// unique matching tokens from the field values.
func (idx *Index) Autocomplete(ctx context.Context, req searchindex.AutocompleteRequest) (*searchindex.AutocompleteResult, error) {
	limit := req.Limit
	if limit <= 0 {
		limit = 10
	}

	prefix := strings.ToLower(req.Prefix)

	// Search for documents matching the prefix.
	body := map[string]any{
		"q":                    prefix,
		"limit":                100,
		"attributesToSearchOn": []string{req.Field},
		"attributesToRetrieve": []string{req.Field},
	}

	path := fmt.Sprintf("/indexes/%s/search", url.PathEscape(idx.name))
	respBody, err := idx.doRequest(ctx, "POST", path, body)
	if err != nil {
		return nil, fmt.Errorf("meilisearch: autocomplete search failed: %w", err)
	}

	var result struct {
		Hits []map[string]any `json:"hits"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("meilisearch: unmarshal autocomplete response: %w", err)
	}

	// Extract unique terms from the field values that match the prefix.
	termCounts := make(map[string]int)
	for _, hit := range result.Hits {
		val, ok := hit[req.Field]
		if !ok {
			continue
		}
		text, ok := val.(string)
		if !ok {
			continue
		}
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

// tokenize splits text into lowercase tokens, mimicking standard text analysis.
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

// Close releases resources. For the HTTP-based Meilisearch client, this is a no-op.
func (idx *Index) Close() error {
	return nil
}

// ---------- HTTP helpers ----------

// doRequest performs an HTTP request and returns the response body bytes.
func (idx *Index) doRequest(ctx context.Context, method, path string, body any) ([]byte, error) {
	var bodyReader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("meilisearch: marshal request body: %w", err)
		}
		bodyReader = bytes.NewReader(data)
	}

	url := idx.config.Host + path
	httpReq, err := http.NewRequestWithContext(ctx, method, url, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("meilisearch: create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if idx.config.APIKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+idx.config.APIKey)
	}

	resp, err := idx.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("meilisearch: do request %s %s: %w", method, path, err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("meilisearch: read response body: %w", err)
	}

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("meilisearch: %s %s returned %d: %s", method, path, resp.StatusCode, string(respBody))
	}

	return respBody, nil
}

// taskResponse represents a Meilisearch async task response.
type taskResponse struct {
	TaskUID int    `json:"taskUid"`
	Status  string `json:"status"`
}

// taskStatusResponse represents a Meilisearch task status response from GET /tasks/{uid}.
type taskStatusResponse struct {
	UID    int    `json:"uid"`
	Status string `json:"status"`
	Error  *struct {
		Message string `json:"message"`
		Code    string `json:"code"`
		Type    string `json:"type"`
	} `json:"error,omitempty"`
}

// doTaskRequest performs an HTTP request that returns a task UID.
func (idx *Index) doTaskRequest(ctx context.Context, method, path string, body any) (int, error) {
	respBody, err := idx.doRequest(ctx, method, path, body)
	if err != nil {
		return 0, err
	}

	var task taskResponse
	if err := json.Unmarshal(respBody, &task); err != nil {
		return 0, fmt.Errorf("meilisearch: unmarshal task response: %w (body: %s)", err, string(respBody))
	}
	return task.TaskUID, nil
}

// waitForTask polls GET /tasks/{taskUid} until status is "succeeded" or "failed".
func (idx *Index) waitForTask(ctx context.Context, taskUID int) error {
	path := "/tasks/" + strconv.Itoa(taskUID)
	deadline := time.Now().Add(taskPollTimeout)

	for {
		if time.Now().After(deadline) {
			return fmt.Errorf("meilisearch: task %d timed out after %v", taskUID, taskPollTimeout)
		}

		respBody, err := idx.doRequest(ctx, http.MethodGet, path, nil)
		if err != nil {
			return fmt.Errorf("meilisearch: poll task %d: %w", taskUID, err)
		}

		var status taskStatusResponse
		if err := json.Unmarshal(respBody, &status); err != nil {
			return fmt.Errorf("meilisearch: unmarshal task status: %w", err)
		}

		switch status.Status {
		case "succeeded":
			return nil
		case "failed":
			errMsg := "unknown error"
			if status.Error != nil {
				errMsg = status.Error.Message
			}
			return fmt.Errorf("meilisearch: task %d failed: %s", taskUID, errMsg)
		case "enqueued", "processing":
			// Still running; wait and retry.
		default:
			return fmt.Errorf("meilisearch: task %d unexpected status: %s", taskUID, status.Status)
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(taskPollInterval):
		}
	}
}

// ---------- Search response types ----------

// meiliSearchResponse represents the Meilisearch search response.
type meiliSearchResponse struct {
	Hits               []json.RawMessage         `json:"hits"`
	EstimatedTotalHits int                       `json:"estimatedTotalHits"`
	FacetDistribution  map[string]map[string]int `json:"facetDistribution,omitempty"`
}

// ---------- Hit conversion ----------

// convertHit transforms a Meilisearch search hit map into a searchindex.SearchHit.
func convertHit(hitMap map[string]any) (searchindex.SearchHit, error) {
	identity, err := extractIdentity(hitMap)
	if err != nil {
		return searchindex.SearchHit{}, err
	}

	// Build representation, excluding internal fields.
	representation := make(map[string]any, len(hitMap))
	for k, v := range hitMap {
		if k == reservedDocIDField || k == reservedTypeNameField || k == reservedKeyFieldsField {
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
		Representation: representation,
	}, nil
}

// extractIdentity reconstructs a DocumentIdentity from stored Meilisearch fields.
func extractIdentity(fields map[string]any) (searchindex.DocumentIdentity, error) {
	typeName, _ := fields[reservedTypeNameField].(string)
	keyFieldsRaw, _ := fields[reservedKeyFieldsField].(string)

	var keyFields map[string]any
	if keyFieldsRaw != "" {
		if err := json.Unmarshal([]byte(keyFieldsRaw), &keyFields); err != nil {
			return searchindex.DocumentIdentity{}, fmt.Errorf("meilisearch: failed to unmarshal key fields: %w", err)
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

// ---------- Facet conversion ----------

// convertFacets transforms Meilisearch facetDistribution into the searchindex format.
func convertFacets(facetDist map[string]map[string]int) map[string]searchindex.FacetResult {
	if len(facetDist) == 0 {
		return nil
	}
	facets := make(map[string]searchindex.FacetResult, len(facetDist))
	for field, counts := range facetDist {
		values := make([]searchindex.FacetValue, 0, len(counts))
		for val, count := range counts {
			values = append(values, searchindex.FacetValue{
				Value: val,
				Count: count,
			})
		}
		// Sort by count descending for deterministic output.
		sort.Slice(values, func(i, j int) bool {
			if values[i].Count != values[j].Count {
				return values[i].Count > values[j].Count
			}
			return values[i].Value < values[j].Value
		})
		facets[field] = searchindex.FacetResult{Values: values}
	}
	return facets
}

// ---------- Filter translation ----------

// translateFilter recursively converts a searchindex.Filter tree into a Meilisearch
// filter string.
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
				parts = append(parts, "("+s+")")
			}
		}
		if len(parts) == 0 {
			return "", nil
		}
		return strings.Join(parts, " AND "), nil
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
				parts = append(parts, "("+s+")")
			}
		}
		if len(parts) == 0 {
			return "", nil
		}
		return strings.Join(parts, " OR "), nil
	}

	// NOT
	if f.Not != nil {
		inner, err := translateFilter(f.Not)
		if err != nil {
			return "", err
		}
		if inner == "" {
			return "", nil
		}
		return "NOT (" + inner + ")", nil
	}

	// Term
	if f.Term != nil {
		return translateTermFilter(f.Term), nil
	}

	// Terms (IN)
	if f.Terms != nil {
		return translateTermsFilter(f.Terms), nil
	}

	// Range
	if f.Range != nil {
		return translateRangeFilter(f.Range)
	}

	// Prefix: Meilisearch does not natively support prefix filters on filterable
	// attributes. As a best-effort approximation we cannot do a true prefix match
	// with the filter syntax, so we return an error indicating this limitation.
	// Callers should use TextQuery for prefix-style matching.
	if f.Prefix != nil {
		// Meilisearch does not support prefix filters in the filter parameter.
		// As a workaround, we return an unsupported error.
		return "", fmt.Errorf("meilisearch: prefix filter is not supported in Meilisearch filter syntax")
	}

	// Exists: Meilisearch supports "field EXISTS".
	if f.Exists != nil {
		return f.Exists.Field + " EXISTS", nil
	}

	return "", nil
}

// translateTermFilter converts a TermFilter to a Meilisearch filter expression.
func translateTermFilter(tf *searchindex.TermFilter) string {
	return tf.Field + " = " + formatFilterValue(tf.Value)
}

// translateTermsFilter converts a TermsFilter (IN) to a Meilisearch filter expression.
func translateTermsFilter(tf *searchindex.TermsFilter) string {
	if len(tf.Values) == 0 {
		return ""
	}
	vals := make([]string, 0, len(tf.Values))
	for _, v := range tf.Values {
		vals = append(vals, formatFilterValue(v))
	}
	return tf.Field + " IN [" + strings.Join(vals, ", ") + "]"
}

// translateRangeFilter converts a RangeFilter to a Meilisearch filter expression.
func translateRangeFilter(rf *searchindex.RangeFilter) (string, error) {
	var parts []string

	if rf.GTE != nil {
		v, err := formatNumericValue(rf.GTE)
		if err != nil {
			return "", fmt.Errorf("meilisearch: range GTE: %w", err)
		}
		parts = append(parts, rf.Field+" >= "+v)
	} else if rf.HasGT && rf.GT != nil {
		v, err := formatNumericValue(rf.GT)
		if err != nil {
			return "", fmt.Errorf("meilisearch: range GT: %w", err)
		}
		parts = append(parts, rf.Field+" > "+v)
	}

	if rf.LTE != nil {
		v, err := formatNumericValue(rf.LTE)
		if err != nil {
			return "", fmt.Errorf("meilisearch: range LTE: %w", err)
		}
		parts = append(parts, rf.Field+" <= "+v)
	} else if rf.HasLT && rf.LT != nil {
		v, err := formatNumericValue(rf.LT)
		if err != nil {
			return "", fmt.Errorf("meilisearch: range LT: %w", err)
		}
		parts = append(parts, rf.Field+" < "+v)
	}

	if len(parts) == 0 {
		return "", nil
	}
	return strings.Join(parts, " AND "), nil
}

// formatFilterValue formats a value for use in a Meilisearch filter expression.
// Strings are quoted; numbers and bools are unquoted.
func formatFilterValue(v any) string {
	switch val := v.(type) {
	case string:
		// Escape double quotes inside the string.
		escaped := strings.ReplaceAll(val, `\`, `\\`)
		escaped = strings.ReplaceAll(escaped, `"`, `\"`)
		return `"` + escaped + `"`
	case float64:
		return strconv.FormatFloat(val, 'f', -1, 64)
	case float32:
		return strconv.FormatFloat(float64(val), 'f', -1, 32)
	case int:
		return strconv.Itoa(val)
	case int64:
		return strconv.FormatInt(val, 10)
	case int32:
		return strconv.FormatInt(int64(val), 10)
	case bool:
		if val {
			return "true"
		}
		return "false"
	case json.Number:
		return val.String()
	default:
		return fmt.Sprintf("%q", fmt.Sprintf("%v", v))
	}
}

// formatNumericValue converts a numeric value to its string representation for
// range filter expressions.
func formatNumericValue(v any) (string, error) {
	switch n := v.(type) {
	case float64:
		return strconv.FormatFloat(n, 'f', -1, 64), nil
	case float32:
		return strconv.FormatFloat(float64(n), 'f', -1, 32), nil
	case int:
		return strconv.Itoa(n), nil
	case int64:
		return strconv.FormatInt(n, 10), nil
	case int32:
		return strconv.FormatInt(int64(n), 10), nil
	case json.Number:
		return n.String(), nil
	default:
		return "", fmt.Errorf("cannot convert %T to numeric", v)
	}
}
