package search_datasource

import (
	"fmt"
	"strings"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/searchindex"
)

// GenerateSubgraphSDL takes the parsed config and produces the actual subgraph SDL.
func GenerateSubgraphSDL(config *ParsedConfig) (string, error) {
	// Validate cursor pagination backend support.
	for _, entity := range config.Entities {
		if !entity.CursorBasedPagination {
			continue
		}
		// Find matching index directive.
		for _, idx := range config.Indices {
			if idx.Name == entity.IndexName {
				caps, ok := cursorBackendCaps[idx.Backend]
				if !ok || !caps.Supported {
					return "", fmt.Errorf("backend %q does not support cursor-based pagination", idx.Backend)
				}
				break
			}
		}
	}

	var b strings.Builder

	// Shared scalar/input/enum types
	writeSharedTypes(&b, config.Entities)

	// Per-entity types
	for _, entity := range config.Entities {
		writeEntityTypes(&b, &entity)
	}

	// Query type with all search fields
	writeQueryType(&b, config.Entities)

	// Entity stubs
	writeEntityStubs(&b, config.Entities)

	return b.String(), nil
}

func writeSharedTypes(b *strings.Builder, entities []SearchableEntity) {
	// Declare @oneOf if any entity uses vector search (SearchInput uses @oneOf).
	for _, e := range entities {
		if e.HasVectorSearch() {
			b.WriteString("directive @oneOf on INPUT_OBJECT\n\n")
			break
		}
	}

	b.WriteString(`input StringFilter {
  eq: String
  ne: String
  in: [String!]
  contains: String
  startsWith: String
}

input FloatFilter {
  eq: Float
  gt: Float
  gte: Float
  lt: Float
  lte: Float
}

input IntFilter {
  eq: Int
  gt: Int
  gte: Int
  lt: Int
  lte: Int
}

enum SortDirection {
  ASC
  DESC
}

enum Fuzziness {
  EXACT
  LOW
  HIGH
}

`)

	// Only emit facet types if at least one entity uses wrapper types and is not vector-only.
	needsFacets := false
	for _, e := range entities {
		if e.NeedsResponseWrapper() && !e.HasVectorSearch() {
			needsFacets = true
			break
		}
	}
	if needsFacets {
		b.WriteString(`type SearchFacet {
  field: String!
  values: [SearchFacetValue!]!
}

type SearchFacetValue {
  value: String!
  count: Int!
}

`)
	}

	// Emit SearchHighlight type if any entity uses wrapper or connection types.
	needsHighlights := false
	for _, e := range entities {
		if e.NeedsResponseWrapper() {
			needsHighlights = true
			break
		}
	}
	if needsHighlights {
		b.WriteString(`type SearchHighlight {
  field: String!
  fragments: [String!]!
}

`)
	}

	// Emit SearchPageInfo if any entity uses cursor pagination.
	needsPageInfo := false
	for _, e := range entities {
		if e.CursorBasedPagination {
			needsPageInfo = true
			break
		}
	}
	if needsPageInfo {
		b.WriteString(`type SearchPageInfo {
  hasNextPage: Boolean!
  hasPreviousPage: Boolean!
  startCursor: String
  endCursor: String
}

`)
	}

	// Emit geo types if any entity has GEO fields.
	needsGeo := false
	for _, e := range entities {
		if e.HasGeoSearch() {
			needsGeo = true
			break
		}
	}
	if needsGeo {
		b.WriteString(`input GeoPointInput {
  lat: Float!
  lon: Float!
}

input GeoDistanceFilterInput {
  center: GeoPointInput!
  distance: String!
}

input GeoBoundingBoxFilterInput {
  topLeft: GeoPointInput!
  bottomRight: GeoPointInput!
}

input GeoDistanceSortInput {
  field: String!
  center: GeoPointInput!
  direction: SortDirection!
  unit: String
}

`)
	}

	// Emit date scalars and filter types if any entity has DATE or DATETIME fields.
	needsDate := false
	for _, e := range entities {
		if e.HasDateField() {
			needsDate = true
			break
		}
	}
	if needsDate {
		b.WriteString(`scalar Date
scalar DateTime

input DateFilter {
  eq: Date
  gt: Date
  gte: Date
  lt: Date
  lte: Date
  after: Date
  before: Date
}

input DateTimeFilter {
  eq: DateTime
  gt: DateTime
  gte: DateTime
  lt: DateTime
  lte: DateTime
  after: DateTime
  before: DateTime
}

`)
	}

	// Emit SuggestTerm type if any entity has autocomplete.
	needsSuggest := false
	for _, e := range entities {
		if e.SuggestField != "" && e.HasAutocomplete() {
			needsSuggest = true
			break
		}
	}
	if needsSuggest {
		b.WriteString(`type SuggestTerm {
  term: String!
  count: Int!
}

`)
	}
}

func writeEntityTypes(b *strings.Builder, entity *SearchableEntity) {
	typeName := entity.TypeName
	hasVector := entity.HasVectorSearch()

	// Search input for vector entities
	if hasVector {
		fmt.Fprintf(b, "input Search%sInput @oneOf {\n", typeName)
		b.WriteString("  query: String\n")
		b.WriteString("  vector: [Float!]\n")
		b.WriteString("}\n\n")
	}

	// Filter input type
	filterFields := filterableFields(entity)
	if len(filterFields) > 0 {
		fmt.Fprintf(b, "input %sFilter {\n", typeName)
		for _, f := range filterFields {
			if f.IndexType == searchindex.FieldTypeGeo {
				fmt.Fprintf(b, "  %s_distance: GeoDistanceFilterInput\n", f.FieldName)
				fmt.Fprintf(b, "  %s_boundingBox: GeoBoundingBoxFilterInput\n", f.FieldName)
			} else {
				filterType := graphqlFilterType(f)
				fmt.Fprintf(b, "  %s: %s\n", f.FieldName, filterType)
			}
		}
		fmt.Fprintf(b, "  AND: [%sFilter!]\n", typeName)
		fmt.Fprintf(b, "  OR: [%sFilter!]\n", typeName)
		fmt.Fprintf(b, "  NOT: %sFilter\n", typeName)
		b.WriteString("}\n\n")
	}

	// Sort enum and input
	sortFields := sortableFields(entity)
	if len(sortFields) > 0 {
		fmt.Fprintf(b, "enum %sSortField {\n", typeName)
		b.WriteString("  RELEVANCE\n")
		for _, f := range sortFields {
			fmt.Fprintf(b, "  %s\n", strings.ToUpper(f.FieldName))
		}
		b.WriteString("}\n\n")

		fmt.Fprintf(b, "input %sSort {\n", typeName)
		fmt.Fprintf(b, "  field: %sSortField!\n", typeName)
		b.WriteString("  direction: SortDirection!\n")
		b.WriteString("}\n\n")
	}

	// Cursor-based pagination types
	if entity.CursorBasedPagination {
		writeConnectionTypes(b, entity)
		return
	}

	// Legacy result wrapper types
	if entity.ResultsMetaInformation {
		fmt.Fprintf(b, "type Search%sResult {\n", typeName)
		fmt.Fprintf(b, "  hits: [Search%sHit!]!\n", typeName)
		b.WriteString("  totalCount: Int!\n")
		if !hasVector {
			b.WriteString("  facets: [SearchFacet!]\n")
		}
		b.WriteString("}\n\n")

		fmt.Fprintf(b, "type Search%sHit {\n", typeName)
		b.WriteString("  score: Float!\n")
		if hasVector {
			b.WriteString("  distance: Float\n")
		}
		if entity.HasGeoSearch() {
			b.WriteString("  geoDistance: Float\n")
		}
		b.WriteString("  highlights: [SearchHighlight!]\n")
		fmt.Fprintf(b, "  node: %s!\n", typeName)
		b.WriteString("}\n\n")
	}
}

// writeConnectionTypes emits Relay-style Connection/Edge types for cursor pagination.
func writeConnectionTypes(b *strings.Builder, entity *SearchableEntity) {
	typeName := entity.TypeName
	hasVector := entity.HasVectorSearch()

	// Connection type
	fmt.Fprintf(b, "type Search%sConnection {\n", typeName)
	fmt.Fprintf(b, "  edges: [Search%sEdge!]!\n", typeName)
	b.WriteString("  pageInfo: SearchPageInfo!\n")
	b.WriteString("  totalCount: Int!\n")
	if !hasVector {
		b.WriteString("  facets: [SearchFacet!]\n")
	}
	b.WriteString("}\n\n")

	// Edge type
	fmt.Fprintf(b, "type Search%sEdge {\n", typeName)
	b.WriteString("  cursor: String!\n")
	fmt.Fprintf(b, "  node: %s!\n", typeName)
	if entity.ResultsMetaInformation {
		b.WriteString("  score: Float!\n")
		if hasVector {
			b.WriteString("  distance: Float\n")
		}
		if entity.HasGeoSearch() {
			b.WriteString("  geoDistance: Float\n")
		}
		b.WriteString("  highlights: [SearchHighlight!]\n")
	}
	b.WriteString("}\n\n")
}

func writeQueryType(b *strings.Builder, entities []SearchableEntity) {
	b.WriteString("type Query {\n")
	for _, entity := range entities {
		writeSearchField(b, &entity)
		if entity.SuggestField != "" && entity.HasAutocomplete() {
			writeSuggestField(b, &entity)
		}
	}
	b.WriteString("}\n\n")
}

func writeSuggestField(b *strings.Builder, entity *SearchableEntity) {
	fmt.Fprintf(b, "  %s(\n", entity.SuggestField)
	b.WriteString("    prefix: String!\n")
	b.WriteString("    limit: Int\n")
	fmt.Fprintf(b, "  ): [SuggestTerm!]!\n")
}

func writeSearchField(b *strings.Builder, entity *SearchableEntity) {
	hasVector := entity.HasVectorSearch()
	hasFilter := len(filterableFields(entity)) > 0
	hasSort := len(sortableFields(entity)) > 0
	hasGeoSort := hasSortableGeoField(entity)

	fmt.Fprintf(b, "  %s(\n", entity.SearchField)

	if hasVector {
		fmt.Fprintf(b, "    search: Search%sInput!\n", entity.TypeName)
	} else {
		b.WriteString("    query: String!\n")
	}

	b.WriteString("    fuzziness: Fuzziness\n")

	if hasFilter {
		fmt.Fprintf(b, "    filter: %sFilter\n", entity.TypeName)
	}

	if hasSort {
		fmt.Fprintf(b, "    sort: [%sSort!]\n", entity.TypeName)
	}

	if hasGeoSort {
		b.WriteString("    geoSort: GeoDistanceSortInput\n")
	}

	if entity.CursorBasedPagination {
		// Cursor pagination args
		b.WriteString("    first: Int\n")
		b.WriteString("    after: String\n")
		if entity.CursorBidirectional {
			b.WriteString("    last: Int\n")
			b.WriteString("    before: String\n")
		}
		if !hasVector {
			b.WriteString("    facets: [String!]\n")
		}
		fmt.Fprintf(b, "  ): Search%sConnection!\n", entity.TypeName)
	} else {
		// Offset pagination args
		b.WriteString("    limit: Int\n")
		b.WriteString("    offset: Int\n")
		if !hasVector && entity.ResultsMetaInformation {
			b.WriteString("    facets: [String!]\n")
		}
		if entity.ResultsMetaInformation {
			fmt.Fprintf(b, "  ): Search%sResult!\n", entity.TypeName)
		} else {
			fmt.Fprintf(b, "  ): [%s!]!\n", entity.TypeName)
		}
	}
}

func writeEntityStubs(b *strings.Builder, entities []SearchableEntity) {
	for _, entity := range entities {
		keyFields := strings.Join(entity.KeyFields, " ")
		fmt.Fprintf(b, "type %s @key(fields: \"%s\") {\n", entity.TypeName, keyFields)
		for _, kf := range entity.KeyFields {
			fmt.Fprintf(b, "  %s: ID! @external\n", kf)
		}
		b.WriteString("}\n\n")
	}
}

func filterableFields(entity *SearchableEntity) []IndexedField {
	var result []IndexedField
	for _, f := range entity.Fields {
		if f.Filterable {
			result = append(result, f)
		}
	}
	return result
}

func sortableFields(entity *SearchableEntity) []IndexedField {
	var result []IndexedField
	for _, f := range entity.Fields {
		if f.Sortable && f.IndexType != searchindex.FieldTypeGeo {
			result = append(result, f)
		}
	}
	return result
}

func hasSortableGeoField(entity *SearchableEntity) bool {
	for _, f := range entity.Fields {
		if f.IndexType == searchindex.FieldTypeGeo && f.Sortable {
			return true
		}
	}
	return false
}

func graphqlFilterType(f IndexedField) string {
	switch f.IndexType {
	case searchindex.FieldTypeText, searchindex.FieldTypeKeyword:
		return "StringFilter"
	case searchindex.FieldTypeNumeric:
		if isFloatType(f.GraphQLType) {
			return "FloatFilter"
		}
		return "IntFilter"
	case searchindex.FieldTypeBool:
		return "Boolean"
	case searchindex.FieldTypeDate:
		return "DateFilter"
	case searchindex.FieldTypeDateTime:
		return "DateTimeFilter"
	default:
		return "StringFilter"
	}
}

func isFloatType(graphqlType string) bool {
	return strings.Contains(graphqlType, "Float")
}
