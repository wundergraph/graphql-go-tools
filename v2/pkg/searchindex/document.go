package searchindex

// DocumentIdentity uniquely identifies an entity document.
type DocumentIdentity struct {
	TypeName  string
	KeyFields map[string]any
}

// EntityDocument represents an entity to be indexed.
type EntityDocument struct {
	Identity DocumentIdentity
	Fields   map[string]any       // text/keyword/numeric/bool fields
	Vectors  map[string][]float32 // vector fields (field name → embedding)
}

// Fuzziness controls typo tolerance for text search.
type Fuzziness int

const (
	FuzzinessExact Fuzziness = 0 // no typo tolerance
	FuzzinessLow   Fuzziness = 1 // 1 edit distance
	FuzzinessHigh  Fuzziness = 2 // 2 edit distances
)

// TextFieldWeight pairs a field name with its search weight/boost.
type TextFieldWeight struct {
	Name   string
	Weight float64 // 0 or 1.0 = default (no boost)
}

// SearchRequest describes a search query.
type SearchRequest struct {
	// TextQuery and Vector can both be set for hybrid search (text + vector combined).
	// When only TextQuery is set: BM25/full-text search.
	// When only Vector is set: vector/semantic search.
	// When both are set: hybrid search combining text and vector scores.
	TextQuery  string            // free-text (BM25 for text-only, or combined with vector for hybrid)
	TextFields []TextFieldWeight // text fields to search with optional per-field boost

	Vector      []float32 // query embedding (can coexist with TextQuery for hybrid)
	VectorField string    // which vector field to search

	Filter *Filter // structured filtering

	Sort   []SortField
	Limit  int
	Offset int
	Facets []FacetRequest

	TypeName string // filter to specific entity type in multi-type index

	GeoDistanceSort *GeoDistanceSort // sort by distance from a geographic point

	Fuzziness *Fuzziness // typo tolerance level (nil = backend default)

	// Cursor-based pagination: sort values from a previous hit's SortValues.
	SearchAfter  []string // forward cursor sort values (ignore Offset when set)
	SearchBefore []string // backward cursor sort values (for last/before)
}

// SortField defines a sort clause.
type SortField struct {
	Field     string
	Ascending bool
}

// GeoDistanceSort sorts results by distance from a geographic point.
type GeoDistanceSort struct {
	Field     string
	Center    GeoPoint
	Ascending bool
	Unit      string // "km", "mi", "m" — defaults to "km" if empty
}

// FacetRequest requests facet counts for a field.
type FacetRequest struct {
	Field string
	Size  int // max number of facet values to return
}

// SearchResult contains the results of a search query.
type SearchResult struct {
	Hits       []SearchHit
	TotalCount int
	Facets     map[string]FacetResult
}

// SearchHit represents a single search result.
type SearchHit struct {
	Identity       DocumentIdentity
	Score          float64
	Distance       float64             // for vector search
	Highlights     map[string][]string // field → highlighted fragments
	Representation map[string]any      // e.g. {"__typename":"Product","id":"123"}
	SortValues     []string            // sort keys for this hit, used to build cursors
	GeoDistance    *float64            // distance in sort unit, populated when GeoDistanceSort is used
}

// FacetResult contains facet counts for a field.
type FacetResult struct {
	Values []FacetValue
}

// FacetValue is a single facet count entry.
type FacetValue struct {
	Value string
	Count int
}
