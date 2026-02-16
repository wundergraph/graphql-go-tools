// Package bleve provides a Bleve-backed implementation of the searchindex.Index
// and searchindex.IndexFactory interfaces. Bleve is a pure-Go full-text search
// library; it does not support vector search, so vector fields are silently
// ignored during indexing and search.
package bleve

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/blevesearch/bleve/v2"
	"github.com/blevesearch/bleve/v2/mapping"
	"github.com/blevesearch/bleve/v2/search"
	"github.com/blevesearch/bleve/v2/search/query"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/searchindex"
)

// Ensure compile-time interface conformance.
var (
	_ searchindex.Index        = (*Index)(nil)
	_ searchindex.IndexFactory = (*Factory)(nil)
)

// reservedTypeNameField is the Bleve document field used to store the entity
// type name so we can reconstruct DocumentIdentity on search results and
// filter by TypeName in SearchRequest.
const reservedTypeNameField = "_typeName"

// reservedKeyFieldsField stores the JSON-encoded key fields map so we can
// reconstruct the DocumentIdentity from a search hit.
const reservedKeyFieldsField = "_keyFieldsJSON"

// Factory implements searchindex.IndexFactory for Bleve.
type Factory struct{}

// NewFactory returns a new Bleve IndexFactory.
func NewFactory() *Factory {
	return &Factory{}
}

// CreateIndex creates a new in-memory Bleve index configured according to the
// given IndexConfig. The configJSON parameter is currently unused but reserved
// for future backend-specific tuning.
func (f *Factory) CreateIndex(_ context.Context, name string, schema searchindex.IndexConfig, _ []byte) (searchindex.Index, error) {
	indexMapping := bleve.NewIndexMapping()

	docMapping := bleve.NewDocumentMapping()

	// Map each field from the schema.
	for _, fc := range schema.Fields {
		fm := fieldMapping(fc)
		if fm == nil {
			// e.g. vector fields; skip.
			continue
		}
		docMapping.AddFieldMappingsAt(fc.Name, fm)
	}

	// Add internal metadata fields.
	kwMapping := mapping.NewKeywordFieldMapping()
	kwMapping.Store = true
	kwMapping.Index = true
	docMapping.AddFieldMappingsAt(reservedTypeNameField, kwMapping)

	keyFieldsMapping := mapping.NewKeywordFieldMapping()
	keyFieldsMapping.Store = true
	keyFieldsMapping.Index = false
	docMapping.AddFieldMappingsAt(reservedKeyFieldsField, keyFieldsMapping)

	indexMapping.DefaultMapping = docMapping

	idx, err := bleve.NewMemOnly(indexMapping)
	if err != nil {
		return nil, fmt.Errorf("bleve: failed to create in-memory index %q: %w", name, err)
	}

	return &Index{
		name:   name,
		idx:    idx,
		schema: schema,
	}, nil
}

// fieldMapping returns the appropriate Bleve field mapping for a FieldConfig,
// or nil if the field type is not supported (e.g. vectors).
func fieldMapping(fc searchindex.FieldConfig) *mapping.FieldMapping {
	switch fc.Type {
	case searchindex.FieldTypeText:
		fm := bleve.NewTextFieldMapping()
		fm.Store = true
		fm.Index = true
		fm.IncludeTermVectors = true
		return fm
	case searchindex.FieldTypeKeyword:
		fm := mapping.NewKeywordFieldMapping()
		fm.Store = true
		fm.Index = true
		return fm
	case searchindex.FieldTypeNumeric:
		fm := mapping.NewNumericFieldMapping()
		fm.Store = true
		fm.Index = true
		return fm
	case searchindex.FieldTypeBool:
		fm := mapping.NewBooleanFieldMapping()
		fm.Store = true
		fm.Index = true
		return fm
	case searchindex.FieldTypeVector:
		// Bleve does not support vector fields.
		return nil
	case searchindex.FieldTypeGeo:
		// Bleve does not support geo fields.
		return nil
	case searchindex.FieldTypeDate, searchindex.FieldTypeDateTime:
		fm := mapping.NewDateTimeFieldMapping()
		fm.Store = true
		fm.Index = true
		return fm
	default:
		return nil
	}
}

// Index implements searchindex.Index backed by a Bleve in-memory index.
type Index struct {
	name   string
	idx    bleve.Index
	schema searchindex.IndexConfig
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

// buildDoc converts an EntityDocument into a flat map suitable for Bleve
// indexing. It includes all Fields plus internal metadata.
func buildDoc(doc searchindex.EntityDocument) (map[string]any, error) {
	m := make(map[string]any, len(doc.Fields)+2)
	for k, v := range doc.Fields {
		m[k] = v
	}
	m[reservedTypeNameField] = doc.Identity.TypeName

	keyFieldsJSON, err := json.Marshal(doc.Identity.KeyFields)
	if err != nil {
		return nil, fmt.Errorf("bleve: failed to marshal key fields: %w", err)
	}
	m[reservedKeyFieldsField] = string(keyFieldsJSON)
	return m, nil
}

// IndexDocument indexes a single document.
func (idx *Index) IndexDocument(_ context.Context, doc searchindex.EntityDocument) error {
	id := documentID(doc.Identity)
	m, err := buildDoc(doc)
	if err != nil {
		return err
	}
	if err := idx.idx.Index(id, m); err != nil {
		return fmt.Errorf("bleve: index document %q: %w", id, err)
	}
	return nil
}

// IndexDocuments indexes a batch of documents. Bleve's Batch API is used for
// efficiency.
func (idx *Index) IndexDocuments(_ context.Context, docs []searchindex.EntityDocument) error {
	batch := idx.idx.NewBatch()
	for _, doc := range docs {
		id := documentID(doc.Identity)
		m, err := buildDoc(doc)
		if err != nil {
			return err
		}
		if err := batch.Index(id, m); err != nil {
			return fmt.Errorf("bleve: batch index document %q: %w", id, err)
		}
	}
	if err := idx.idx.Batch(batch); err != nil {
		return fmt.Errorf("bleve: batch commit: %w", err)
	}
	return nil
}

// DeleteDocument deletes a single document by identity.
func (idx *Index) DeleteDocument(_ context.Context, id searchindex.DocumentIdentity) error {
	docID := documentID(id)
	if err := idx.idx.Delete(docID); err != nil {
		return fmt.Errorf("bleve: delete document %q: %w", docID, err)
	}
	return nil
}

// DeleteDocuments deletes a batch of documents by identity.
func (idx *Index) DeleteDocuments(_ context.Context, ids []searchindex.DocumentIdentity) error {
	batch := idx.idx.NewBatch()
	for _, id := range ids {
		batch.Delete(documentID(id))
	}
	if err := idx.idx.Batch(batch); err != nil {
		return fmt.Errorf("bleve: batch delete: %w", err)
	}
	return nil
}

// Search performs a search query and returns results.
func (idx *Index) Search(_ context.Context, req searchindex.SearchRequest) (*searchindex.SearchResult, error) {
	q, err := idx.buildQuery(req)
	if err != nil {
		return nil, err
	}

	isCursorMode := len(req.SearchAfter) > 0 || len(req.SearchBefore) > 0
	offset := req.Offset
	if isCursorMode {
		offset = 0 // cursor mode ignores offset
	}

	bleveReq := bleve.NewSearchRequestOptions(q, effectiveLimit(req.Limit), offset, false)
	bleveReq.IncludeLocations = true

	// Highlight.
	bleveReq.Highlight = bleve.NewHighlight()

	// Sorting.
	if len(req.Sort) > 0 {
		sortOrder := make(search.SortOrder, 0, len(req.Sort))
		for _, sf := range req.Sort {
			ss := &search.SortField{
				Field:   sf.Field,
				Desc:    !sf.Ascending,
				Type:    search.SortFieldAuto,
				Missing: search.SortFieldMissingLast,
			}
			sortOrder = append(sortOrder, ss)
		}
		bleveReq.SortByCustom(sortOrder)
	}

	// Cursor-based pagination.
	if len(req.SearchAfter) > 0 {
		bleveReq.SearchAfter = req.SearchAfter
	}
	if len(req.SearchBefore) > 0 {
		bleveReq.SearchBefore = req.SearchBefore
	}

	// Request stored fields so we can reconstruct the document.
	bleveReq.Fields = []string{"*"}

	// Facets.
	for _, fr := range req.Facets {
		size := fr.Size
		if size <= 0 {
			size = 10
		}
		bleveReq.AddFacet(fr.Field, bleve.NewFacetRequest(fr.Field, size))
	}

	result, err := idx.idx.Search(bleveReq)
	if err != nil {
		return nil, fmt.Errorf("bleve: search failed: %w", err)
	}

	hits := make([]searchindex.SearchHit, 0, len(result.Hits))
	for _, hit := range result.Hits {
		sh, err := convertHit(hit)
		if err != nil {
			return nil, err
		}
		hits = append(hits, sh)
	}

	facets := convertFacets(result.Facets)

	return &searchindex.SearchResult{
		Hits:       hits,
		TotalCount: int(result.Total),
		Facets:     facets,
	}, nil
}

// Autocomplete returns terms from the Bleve index dictionary matching the given prefix.
func (idx *Index) Autocomplete(_ context.Context, req searchindex.AutocompleteRequest) (*searchindex.AutocompleteResult, error) {
	prefix := strings.ToLower(req.Prefix)
	limit := req.Limit
	if limit <= 0 {
		limit = 10
	}

	dict, err := idx.idx.FieldDictPrefix(req.Field, []byte(prefix))
	if err != nil {
		return nil, fmt.Errorf("bleve: field dict prefix for %q: %w", req.Field, err)
	}
	defer dict.Close()

	var terms []searchindex.AutocompleteTerm
	for {
		entry, err := dict.Next()
		if err != nil {
			return nil, fmt.Errorf("bleve: iterating field dict: %w", err)
		}
		if entry == nil {
			break
		}
		terms = append(terms, searchindex.AutocompleteTerm{
			Term:  entry.Term,
			Count: int(entry.Count),
		})
		if len(terms) >= limit {
			break
		}
	}

	return &searchindex.AutocompleteResult{Terms: terms}, nil
}

// Close releases resources held by the index.
func (idx *Index) Close() error {
	if err := idx.idx.Close(); err != nil {
		return fmt.Errorf("bleve: close index %q: %w", idx.name, err)
	}
	return nil
}

// effectiveLimit returns a sensible default if limit is zero or negative.
func effectiveLimit(limit int) int {
	if limit <= 0 {
		return 10
	}
	return limit
}

// buildQuery constructs the top-level Bleve query from a SearchRequest.
func (idx *Index) buildQuery(req searchindex.SearchRequest) (query.Query, error) {
	var parts []query.Query

	// Text query.
	if req.TextQuery != "" {
		if len(req.TextFields) > 0 {
			// Build a disjunction of match queries scoped to each field.
			fieldQueries := make([]query.Query, 0, len(req.TextFields))
			for _, tf := range req.TextFields {
				mq := bleve.NewMatchQuery(req.TextQuery)
				mq.SetField(tf.Name)
				if tf.Weight != 0 && tf.Weight != 1.0 {
					mq.SetBoost(tf.Weight)
				}
				if req.Fuzziness != nil {
					mq.SetFuzziness(int(*req.Fuzziness))
				}
				fieldQueries = append(fieldQueries, mq)
			}
			fieldDisjunction := bleve.NewDisjunctionQuery(fieldQueries...)
			parts = append(parts, fieldDisjunction)
		} else {
			mq := bleve.NewMatchQuery(req.TextQuery)
			if req.Fuzziness != nil {
				mq.SetFuzziness(int(*req.Fuzziness))
			}
			parts = append(parts, mq)
		}
	}

	// Vector query: Bleve doesn't support vectors; silently ignore.

	// TypeName filter.
	if req.TypeName != "" {
		tq := bleve.NewTermQuery(req.TypeName)
		tq.SetField(reservedTypeNameField)
		parts = append(parts, tq)
	}

	// Structured filter.
	if req.Filter != nil {
		fq, err := translateFilter(req.Filter)
		if err != nil {
			return nil, err
		}
		if fq != nil {
			parts = append(parts, fq)
		}
	}

	// Combine everything.
	switch len(parts) {
	case 0:
		return bleve.NewMatchAllQuery(), nil
	case 1:
		return parts[0], nil
	default:
		return bleve.NewConjunctionQuery(parts...), nil
	}
}

// translateFilter recursively converts a searchindex.Filter tree to a Bleve
// query tree.
func translateFilter(f *searchindex.Filter) (query.Query, error) {
	if f == nil {
		return nil, nil
	}

	// AND
	if len(f.And) > 0 {
		children := make([]query.Query, 0, len(f.And))
		for _, child := range f.And {
			cq, err := translateFilter(child)
			if err != nil {
				return nil, err
			}
			if cq != nil {
				children = append(children, cq)
			}
		}
		if len(children) == 0 {
			return nil, nil
		}
		return bleve.NewConjunctionQuery(children...), nil
	}

	// OR
	if len(f.Or) > 0 {
		children := make([]query.Query, 0, len(f.Or))
		for _, child := range f.Or {
			cq, err := translateFilter(child)
			if err != nil {
				return nil, err
			}
			if cq != nil {
				children = append(children, cq)
			}
		}
		if len(children) == 0 {
			return nil, nil
		}
		return bleve.NewDisjunctionQuery(children...), nil
	}

	// NOT
	if f.Not != nil {
		inner, err := translateFilter(f.Not)
		if err != nil {
			return nil, err
		}
		if inner == nil {
			return nil, nil
		}
		boolQ := bleve.NewBooleanQuery()
		boolQ.AddMustNot(inner)
		// A boolean query with only MustNot needs a Must to establish the
		// universe of documents.
		boolQ.AddMust(bleve.NewMatchAllQuery())
		return boolQ, nil
	}

	// Term
	if f.Term != nil {
		return translateTermFilter(f.Term)
	}

	// Terms (IN)
	if f.Terms != nil {
		return translateTermsFilter(f.Terms)
	}

	// Range
	if f.Range != nil {
		return translateRangeFilter(f.Range)
	}

	// Prefix
	if f.Prefix != nil {
		pq := bleve.NewPrefixQuery(f.Prefix.Value)
		pq.SetField(f.Prefix.Field)
		return pq, nil
	}

	// Exists: use a wildcard query that matches any term in the field.
	if f.Exists != nil {
		// A regexp ".*" on the field will match any value.
		rq := bleve.NewRegexpQuery(".*")
		rq.SetField(f.Exists.Field)
		return rq, nil
	}

	return nil, nil
}

// translateTermFilter converts a TermFilter to a Bleve query. For string
// values it uses TermQuery; for numeric values it uses a point
// NumericRangeQuery (min==max, inclusive); for bool it uses a bool field query.
func translateTermFilter(tf *searchindex.TermFilter) (query.Query, error) {
	switch v := tf.Value.(type) {
	case string:
		tq := bleve.NewTermQuery(v)
		tq.SetField(tf.Field)
		return tq, nil
	case float64:
		return numericPoint(tf.Field, v), nil
	case float32:
		return numericPoint(tf.Field, float64(v)), nil
	case int:
		return numericPoint(tf.Field, float64(v)), nil
	case int64:
		return numericPoint(tf.Field, float64(v)), nil
	case json.Number:
		n, err := v.Float64()
		if err != nil {
			return nil, fmt.Errorf("bleve: invalid numeric term value: %w", err)
		}
		return numericPoint(tf.Field, n), nil
	case bool:
		bq := bleve.NewBoolFieldQuery(v)
		bq.SetField(tf.Field)
		return bq, nil
	default:
		// Fall back to string representation.
		tq := bleve.NewTermQuery(fmt.Sprintf("%v", v))
		tq.SetField(tf.Field)
		return tq, nil
	}
}

// numericPoint creates a numeric range query matching exactly one value.
func numericPoint(field string, val float64) query.Query {
	inclusive := true
	q := bleve.NewNumericRangeInclusiveQuery(&val, &val, &inclusive, &inclusive)
	q.SetField(field)
	return q
}

// translateTermsFilter converts a TermsFilter (IN operator) to a Bleve
// disjunction of term queries.
func translateTermsFilter(tf *searchindex.TermsFilter) (query.Query, error) {
	if len(tf.Values) == 0 {
		return nil, nil
	}
	parts := make([]query.Query, 0, len(tf.Values))
	for _, val := range tf.Values {
		tq, err := translateTermFilter(&searchindex.TermFilter{
			Field: tf.Field,
			Value: val,
		})
		if err != nil {
			return nil, err
		}
		parts = append(parts, tq)
	}
	return bleve.NewDisjunctionQuery(parts...), nil
}

// translateRangeFilter converts a RangeFilter to a Bleve numeric or date range query.
// Date range is used when the bound values are strings (ISO 8601 date/datetime).
func translateRangeFilter(rf *searchindex.RangeFilter) (query.Query, error) {
	if isDateRange(rf) {
		return translateDateRangeFilter(rf)
	}

	var minVal, maxVal *float64
	var minInclusive, maxInclusive *bool

	// Determine lower bound.
	if rf.GTE != nil {
		v, err := toFloat64(rf.GTE)
		if err != nil {
			return nil, fmt.Errorf("bleve: range GTE: %w", err)
		}
		minVal = &v
		t := true
		minInclusive = &t
	} else if rf.HasGT && rf.GT != nil {
		v, err := toFloat64(rf.GT)
		if err != nil {
			return nil, fmt.Errorf("bleve: range GT: %w", err)
		}
		minVal = &v
		f := false
		minInclusive = &f
	}

	// Determine upper bound.
	if rf.LTE != nil {
		v, err := toFloat64(rf.LTE)
		if err != nil {
			return nil, fmt.Errorf("bleve: range LTE: %w", err)
		}
		maxVal = &v
		t := true
		maxInclusive = &t
	} else if rf.HasLT && rf.LT != nil {
		v, err := toFloat64(rf.LT)
		if err != nil {
			return nil, fmt.Errorf("bleve: range LT: %w", err)
		}
		maxVal = &v
		f := false
		maxInclusive = &f
	}

	q := bleve.NewNumericRangeInclusiveQuery(minVal, maxVal, minInclusive, maxInclusive)
	q.SetField(rf.Field)
	return q, nil
}

// isDateRange returns true if any of the range bound values are strings (date values).
func isDateRange(rf *searchindex.RangeFilter) bool {
	for _, v := range []any{rf.GTE, rf.GT, rf.LTE, rf.LT} {
		if _, ok := v.(string); ok {
			return true
		}
	}
	return false
}

// translateDateRangeFilter converts a RangeFilter with string date values to a Bleve date range query.
func translateDateRangeFilter(rf *searchindex.RangeFilter) (query.Query, error) {
	var minStr, maxStr string
	var minInclusive, maxInclusive *bool

	if rf.GTE != nil {
		minStr = rf.GTE.(string)
		t := true
		minInclusive = &t
	} else if rf.HasGT && rf.GT != nil {
		minStr = rf.GT.(string)
		f := false
		minInclusive = &f
	}

	if rf.LTE != nil {
		maxStr = rf.LTE.(string)
		t := true
		maxInclusive = &t
	} else if rf.HasLT && rf.LT != nil {
		maxStr = rf.LT.(string)
		f := false
		maxInclusive = &f
	}

	q := bleve.NewDateRangeInclusiveStringQuery(minStr, maxStr, minInclusive, maxInclusive)
	q.SetField(rf.Field)
	return q, nil
}

// toFloat64 converts an any value to float64.
func toFloat64(v any) (float64, error) {
	switch n := v.(type) {
	case float64:
		return n, nil
	case float32:
		return float64(n), nil
	case int:
		return float64(n), nil
	case int64:
		return float64(n), nil
	case int32:
		return float64(n), nil
	case json.Number:
		return n.Float64()
	default:
		return 0, fmt.Errorf("cannot convert %T to float64", v)
	}
}

// convertHit transforms a Bleve search.DocumentMatch into a searchindex.SearchHit.
func convertHit(hit *search.DocumentMatch) (searchindex.SearchHit, error) {
	identity, err := extractIdentity(hit.Fields)
	if err != nil {
		return searchindex.SearchHit{}, err
	}

	// Build representation from stored fields, excluding internal fields.
	representation := make(map[string]any, len(hit.Fields))
	for k, v := range hit.Fields {
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

	// Highlights.
	var highlights map[string][]string
	if len(hit.Fragments) > 0 {
		highlights = make(map[string][]string, len(hit.Fragments))
		for field, frags := range hit.Fragments {
			highlights[field] = frags
		}
	}

	// Populate SortValues from hit.Sort for cursor-based pagination.
	var sortValues []string
	if len(hit.Sort) > 0 {
		sortValues = make([]string, len(hit.Sort))
		copy(sortValues, hit.Sort)
	}

	return searchindex.SearchHit{
		Identity:       identity,
		Score:          hit.Score,
		Highlights:     highlights,
		Representation: representation,
		SortValues:     sortValues,
	}, nil
}

// extractIdentity reconstructs a DocumentIdentity from stored Bleve fields.
func extractIdentity(fields map[string]any) (searchindex.DocumentIdentity, error) {
	typeName, _ := fields[reservedTypeNameField].(string)
	keyFieldsRaw, _ := fields[reservedKeyFieldsField].(string)

	var keyFields map[string]any
	if keyFieldsRaw != "" {
		if err := json.Unmarshal([]byte(keyFieldsRaw), &keyFields); err != nil {
			return searchindex.DocumentIdentity{}, fmt.Errorf("bleve: failed to unmarshal key fields: %w", err)
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

// convertFacets transforms Bleve facet results into the searchindex format.
func convertFacets(bleveFacets search.FacetResults) map[string]searchindex.FacetResult {
	if len(bleveFacets) == 0 {
		return nil
	}
	facets := make(map[string]searchindex.FacetResult, len(bleveFacets))
	for name, fr := range bleveFacets {
		var values []searchindex.FacetValue
		if fr.Terms != nil {
			terms := fr.Terms.Terms()
			values = make([]searchindex.FacetValue, 0, len(terms))
			for _, term := range terms {
				values = append(values, searchindex.FacetValue{
					Value: term.Term,
					Count: term.Count,
				})
			}
		}
		facets[name] = searchindex.FacetResult{
			Values: values,
		}
	}
	return facets
}

