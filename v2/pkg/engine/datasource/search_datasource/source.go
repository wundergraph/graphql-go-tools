package search_datasource

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/datasource/httpclient"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/searchindex"
)

// Source implements resolve.DataSource for the search datasource.
type Source struct {
	index    searchindex.Index
	config   Configuration
	embedder searchindex.Embedder // optional, for vector search query embedding
}

// searchInput represents the parsed input from the planner.
type searchInput struct {
	SearchField string           `json:"search_field"`
	Query       string           `json:"query,omitempty"`
	Vector      []float32        `json:"vector,omitempty"`
	Search      *searchInputArg  `json:"search,omitempty"`
	Filter      json.RawMessage  `json:"filter,omitempty"`
	Sort        json.RawMessage  `json:"sort,omitempty"`
	Limit       *int             `json:"limit,omitempty"`
	Offset      *int             `json:"offset,omitempty"`
	Facets      []string         `json:"facets,omitempty"`
	GeoSort     *geoSortInput    `json:"geoSort,omitempty"`
	Fuzziness   *string          `json:"fuzziness,omitempty"`
	First       *int             `json:"first,omitempty"`
	After       *string          `json:"after,omitempty"`
	Last        *int             `json:"last,omitempty"`
	Before      *string          `json:"before,omitempty"`
	Prefix      *string          `json:"prefix,omitempty"`
	IsSuggest   bool             `json:"is_suggest,omitempty"`
}

type geoSortInput struct {
	Field     string `json:"field"`
	Center    struct {
		Lat float64 `json:"lat"`
		Lon float64 `json:"lon"`
	} `json:"center"`
	Direction string `json:"direction"`
	Unit      string `json:"unit,omitempty"`
}

type searchInputArg struct {
	Query  *string   `json:"query,omitempty"`
	Vector []float32 `json:"vector,omitempty"`
}

func (s *Source) Load(ctx context.Context, _ http.Header, input []byte) ([]byte, error) {
	var si searchInput
	if err := json.Unmarshal(input, &si); err != nil {
		return nil, fmt.Errorf("search_datasource: invalid input: %w", err)
	}

	if si.IsSuggest {
		return s.loadSuggest(ctx, &si)
	}

	req, err := s.buildSearchRequest(ctx, &si)
	if err != nil {
		return nil, fmt.Errorf("search_datasource: building search request: %w", err)
	}

	result, err := s.index.Search(ctx, *req)
	if err != nil {
		return nil, fmt.Errorf("search_datasource: search failed: %w", err)
	}

	return s.formatResponse(result, &si)
}

func (s *Source) LoadWithFiles(ctx context.Context, headers http.Header, input []byte, _ []*httpclient.FileUpload) ([]byte, error) {
	return s.Load(ctx, headers, input)
}

func (s *Source) buildSearchRequest(ctx context.Context, si *searchInput) (*searchindex.SearchRequest, error) {
	req := &searchindex.SearchRequest{
		TypeName: s.config.EntityTypeName,
	}

	// Handle query input
	if si.Search != nil {
		// Vector search entity (@oneOf input)
		if si.Search.Query != nil {
			if s.embedder != nil {
				// Auto-embed the text query and keep the text for hybrid search.
				vec, err := s.embedder.EmbedSingle(ctx, *si.Search.Query)
				if err != nil {
					return nil, fmt.Errorf("embedding query: %w", err)
				}
				req.Vector = vec
				req.VectorField = s.findVectorField()
				req.TextQuery = *si.Search.Query
			} else {
				req.TextQuery = *si.Search.Query
			}
		} else if si.Search.Vector != nil {
			req.Vector = si.Search.Vector
			req.VectorField = s.findVectorField()
		}
	} else if si.Query != "" && si.Query != "*" {
		req.TextQuery = si.Query
	}

	// Populate text field weights from configuration.
	if req.TextQuery != "" {
		for _, f := range s.config.Fields {
			if f.IndexType == searchindex.FieldTypeText {
				w := f.Weight
				if w == 0 {
					w = 1.0
				}
				req.TextFields = append(req.TextFields, searchindex.TextFieldWeight{
					Name:   f.FieldName,
					Weight: w,
				})
			}
		}
	}

	// Parse filter
	if len(si.Filter) > 0 {
		filter, err := ParseFilterJSON(si.Filter, s.config.Fields)
		if err != nil {
			return nil, err
		}
		req.Filter = filter
	}

	// Parse sort
	if len(si.Sort) > 0 {
		var sorts []sortInput
		if err := json.Unmarshal(si.Sort, &sorts); err != nil {
			return nil, fmt.Errorf("invalid sort input: %w", err)
		}
		for _, sort := range sorts {
			req.Sort = append(req.Sort, searchindex.SortField{
				Field:     s.resolveSortField(sort.Field),
				Ascending: sort.Direction == "ASC",
			})
		}
	}

	// Geo distance sort
	if si.GeoSort != nil {
		unit := si.GeoSort.Unit
		if unit == "" {
			unit = "km"
		}
		req.GeoDistanceSort = &searchindex.GeoDistanceSort{
			Field:     si.GeoSort.Field,
			Center:    searchindex.GeoPoint{Lat: si.GeoSort.Center.Lat, Lon: si.GeoSort.Center.Lon},
			Ascending: si.GeoSort.Direction == "ASC",
			Unit:      unit,
		}
	}

	// Fuzziness / typo tolerance
	if si.Fuzziness != nil {
		req.Fuzziness = parseFuzziness(*si.Fuzziness)
	}

	// Cursor-based pagination takes precedence.
	if s.config.CursorBasedPagination && (si.First != nil || si.Last != nil) {
		if si.First != nil {
			req.Limit = *si.First
		} else if si.Last != nil {
			req.Limit = *si.Last
		}
		if req.Limit <= 0 {
			req.Limit = 10
		}
		// Over-fetch by 1 to detect hasNextPage/hasPreviousPage.
		req.Limit++
		if si.Last != nil {
			// Backward pagination: reverse sort direction so the backend returns
			// items from the end. formatConnectionResponse reverses them back.
			// The "before" cursor becomes a SearchAfter in the reversed sort space.
			for i := range req.Sort {
				req.Sort[i].Ascending = !req.Sort[i].Ascending
			}
			if si.Before != nil {
				sortVals, err := DecodeCursor(*si.Before)
				if err != nil {
					return nil, fmt.Errorf("decoding before cursor: %w", err)
				}
				req.SearchAfter = sortVals
			}
		} else {
			if si.After != nil {
				sortVals, err := DecodeCursor(*si.After)
				if err != nil {
					return nil, fmt.Errorf("decoding after cursor: %w", err)
				}
				req.SearchAfter = sortVals
			}
		}
	} else if si.Limit != nil {
		req.Limit = *si.Limit
	} else if si.First != nil {
		req.Limit = *si.First
	} else {
		req.Limit = 10 // default
	}
	if si.Offset != nil {
		req.Offset = *si.Offset
	}

	// Enforce upper bound on limit to prevent excessive result sets.
	const maxLimit = 1000
	if req.Limit > maxLimit {
		req.Limit = maxLimit
	}

	for _, facet := range si.Facets {
		req.Facets = append(req.Facets, searchindex.FacetRequest{Field: facet})
	}

	return req, nil
}

type sortInput struct {
	Field     string `json:"field"`
	Direction string `json:"direction"`
}

func (s *Source) findVectorField() string {
	// Embedding fields take precedence over manually declared VECTOR fields.
	if len(s.config.EmbeddingFields) > 0 {
		return s.config.EmbeddingFields[0].FieldName
	}
	// Then check VECTOR fields.
	for _, f := range s.config.Fields {
		if f.IndexType == searchindex.FieldTypeVector {
			return f.FieldName
		}
	}
	return ""
}

// resolveSortField maps an uppercase enum value (e.g. "CREATEDAT") back to the
// original field name (e.g. "createdAt"). Falls back to strings.ToLower for
// fields not found in the config (e.g. "RELEVANCE").
func (s *Source) resolveSortField(enumValue string) string {
	upper := strings.ToUpper(enumValue)
	for _, f := range s.config.Fields {
		if strings.ToUpper(f.FieldName) == upper {
			return f.FieldName
		}
	}
	return strings.ToLower(enumValue)
}

func (s *Source) formatResponse(result *searchindex.SearchResult, si *searchInput) ([]byte, error) {
	if s.config.CursorBasedPagination {
		return s.formatConnectionResponse(result, si)
	}
	if !s.config.ResultsMetaInformation {
		return s.formatInlineResponse(result)
	}
	return s.formatWrapperResponse(result)
}

func (s *Source) formatInlineResponse(result *searchindex.SearchResult) ([]byte, error) {
	entities := make([]map[string]any, 0, len(result.Hits))
	for _, hit := range result.Hits {
		entities = append(entities, hit.Representation)
	}
	wrapped := map[string]any{
		"data": map[string]any{
			s.config.SearchField: entities,
		},
	}
	return json.Marshal(wrapped)
}

func (s *Source) formatWrapperResponse(result *searchindex.SearchResult) ([]byte, error) {
	hits := make([]map[string]any, 0, len(result.Hits))
	for _, hit := range result.Hits {
		h := map[string]any{
			"score": hit.Score,
			"node":  hit.Representation,
		}
		if hit.Distance != 0 {
			h["distance"] = hit.Distance
		}
		if hit.GeoDistance != nil {
			h["geoDistance"] = *hit.GeoDistance
		}
		if len(hit.Highlights) > 0 {
			h["highlights"] = formatHighlights(hit.Highlights)
		}
		hits = append(hits, h)
	}

	resp := map[string]any{
		"hits":       hits,
		"totalCount": result.TotalCount,
	}

	if result.Facets != nil {
		facets := make([]map[string]any, 0)
		for field, fr := range result.Facets {
			values := make([]map[string]any, 0, len(fr.Values))
			for _, fv := range fr.Values {
				values = append(values, map[string]any{
					"value": fv.Value,
					"count": fv.Count,
				})
			}
			facets = append(facets, map[string]any{
				"field":  field,
				"values": values,
			})
		}
		resp["facets"] = facets
	}

	// Wrap in {"data": {"<searchField>": ...}} to match PostProcessing.SelectResponseDataPath: ["data"].
	// After the resolver extracts "data", the result is keyed by the search field name,
	// which aligns with the response tree built by the plan visitor.
	wrapped := map[string]any{
		"data": map[string]any{
			s.config.SearchField: resp,
		},
	}
	return json.Marshal(wrapped)
}

func (s *Source) formatConnectionResponse(result *searchindex.SearchResult, si *searchInput) ([]byte, error) {
	// Determine the requested limit (before over-fetch).
	requestedLimit := 10
	isBackward := si.Last != nil
	if si.First != nil {
		requestedLimit = *si.First
	} else if si.Last != nil {
		requestedLimit = *si.Last
	}

	hits := result.Hits
	hasMore := len(hits) > requestedLimit
	if hasMore {
		hits = hits[:requestedLimit]
	}

	// For backward pagination, reverse the results (backend returns in reversed order).
	if isBackward {
		for i, j := 0, len(hits)-1; i < j; i, j = i+1, j-1 {
			hits[i], hits[j] = hits[j], hits[i]
		}
	}

	// Compute pageInfo.
	var hasNextPage, hasPreviousPage bool
	if isBackward {
		hasPreviousPage = hasMore
		hasNextPage = si.Before != nil
	} else {
		hasNextPage = hasMore
		hasPreviousPage = si.After != nil
	}

	// Build edges.
	edges := make([]map[string]any, 0, len(hits))
	for _, hit := range hits {
		cursor := EncodeCursor(hit.SortValues)
		edge := map[string]any{
			"cursor": cursor,
			"node":   hit.Representation,
		}
		if s.config.ResultsMetaInformation {
			edge["score"] = hit.Score
			if hit.Distance != 0 {
				edge["distance"] = hit.Distance
			}
			if hit.GeoDistance != nil {
				edge["geoDistance"] = *hit.GeoDistance
			}
			if len(hit.Highlights) > 0 {
				edge["highlights"] = formatHighlights(hit.Highlights)
			}
		}
		edges = append(edges, edge)
	}

	var startCursor, endCursor *string
	if len(edges) > 0 {
		sc := edges[0]["cursor"].(string)
		ec := edges[len(edges)-1]["cursor"].(string)
		startCursor = &sc
		endCursor = &ec
	}

	pageInfo := map[string]any{
		"hasNextPage":     hasNextPage,
		"hasPreviousPage": hasPreviousPage,
		"startCursor":     startCursor,
		"endCursor":       endCursor,
	}

	resp := map[string]any{
		"edges":      edges,
		"pageInfo":   pageInfo,
		"totalCount": result.TotalCount,
	}

	if result.Facets != nil {
		facets := make([]map[string]any, 0)
		for field, fr := range result.Facets {
			values := make([]map[string]any, 0, len(fr.Values))
			for _, fv := range fr.Values {
				values = append(values, map[string]any{
					"value": fv.Value,
					"count": fv.Count,
				})
			}
			facets = append(facets, map[string]any{
				"field":  field,
				"values": values,
			})
		}
		resp["facets"] = facets
	}

	wrapped := map[string]any{
		"data": map[string]any{
			s.config.SearchField: resp,
		},
	}
	return json.Marshal(wrapped)
}

func parseFuzziness(val string) *searchindex.Fuzziness {
	var f searchindex.Fuzziness
	switch val {
	case "EXACT":
		f = searchindex.FuzzinessExact
	case "LOW":
		f = searchindex.FuzzinessLow
	case "HIGH":
		f = searchindex.FuzzinessHigh
	default:
		return nil
	}
	return &f
}

const (
	defaultSuggestLimit = 10
	minPrefixLength     = 2
)

func (s *Source) loadSuggest(ctx context.Context, si *searchInput) ([]byte, error) {
	prefix := ""
	if si.Prefix != nil {
		prefix = *si.Prefix
	}
	if len(prefix) < minPrefixLength {
		return s.formatSuggestResponse(nil)
	}

	prefix = strings.ToLower(prefix)

	limit := defaultSuggestLimit
	if si.Limit != nil && *si.Limit > 0 {
		limit = *si.Limit
	}

	// Collect autocomplete-enabled fields and query each.
	var allTerms []searchindex.AutocompleteTerm
	for _, f := range s.config.Fields {
		if !f.Autocomplete {
			continue
		}
		result, err := s.index.Autocomplete(ctx, searchindex.AutocompleteRequest{
			Field:  f.FieldName,
			Prefix: prefix,
			Limit:  limit,
		})
		if err != nil {
			return nil, fmt.Errorf("search_datasource: autocomplete failed for field %s: %w", f.FieldName, err)
		}
		allTerms = append(allTerms, result.Terms...)
	}

	// Deduplicate terms across fields, summing counts.
	termMap := make(map[string]int)
	for _, t := range allTerms {
		termMap[t.Term] += t.Count
	}

	deduped := make([]searchindex.AutocompleteTerm, 0, len(termMap))
	for term, count := range termMap {
		deduped = append(deduped, searchindex.AutocompleteTerm{Term: term, Count: count})
	}
	sort.Slice(deduped, func(i, j int) bool {
		if deduped[i].Count != deduped[j].Count {
			return deduped[i].Count > deduped[j].Count
		}
		return deduped[i].Term < deduped[j].Term
	})
	if len(deduped) > limit {
		deduped = deduped[:limit]
	}

	return s.formatSuggestResponse(deduped)
}

func (s *Source) formatSuggestResponse(terms []searchindex.AutocompleteTerm) ([]byte, error) {
	suggestions := make([]map[string]any, 0, len(terms))
	for _, t := range terms {
		suggestions = append(suggestions, map[string]any{
			"term":  t.Term,
			"count": t.Count,
		})
	}
	wrapped := map[string]any{
		"data": map[string]any{
			s.config.SearchField: suggestions,
		},
	}
	return json.Marshal(wrapped)
}

// formatHighlights converts the backend highlight map to the GraphQL SearchHighlight array format.
func formatHighlights(highlights map[string][]string) []map[string]any {
	result := make([]map[string]any, 0, len(highlights))
	for field, fragments := range highlights {
		result = append(result, map[string]any{
			"field":     field,
			"fragments": fragments,
		})
	}
	return result
}
