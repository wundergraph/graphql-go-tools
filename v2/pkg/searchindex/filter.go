package searchindex

// Filter represents a composable filter tree that translates to each backend's native format.
type Filter struct {
	And    []*Filter
	Or     []*Filter
	Not    *Filter
	Term   *TermFilter
	Terms  *TermsFilter
	Range  *RangeFilter
	Prefix         *PrefixFilter
	Exists         *ExistsFilter
	GeoDistance    *GeoDistanceFilter
	GeoBoundingBox *GeoBoundingBoxFilter
}

// TermFilter matches a single exact value.
type TermFilter struct {
	Field string
	Value any
}

// TermsFilter matches any of a set of values (IN operator).
type TermsFilter struct {
	Field  string
	Values []any
}

// RangeFilter matches numeric/string ranges.
type RangeFilter struct {
	Field string
	GT    any  // greater than
	GTE   any  // greater than or equal
	LT    any  // less than
	LTE   any  // less than or equal
	HasGT bool // whether GT is set
	HasLT bool // whether LT is set
}

// PrefixFilter matches values starting with a prefix.
type PrefixFilter struct {
	Field string
	Value string
}

// ExistsFilter matches documents where a field exists.
type ExistsFilter struct {
	Field string
}

// GeoPoint represents a latitude/longitude coordinate.
type GeoPoint struct {
	Lat float64
	Lon float64
}

// GeoDistanceFilter matches documents within a radius of a point.
type GeoDistanceFilter struct {
	Field    string
	Center   GeoPoint
	Distance string // e.g. "10km", "5mi" — passed directly to backend
}

// GeoBoundingBoxFilter matches documents within a rectangular region.
type GeoBoundingBoxFilter struct {
	Field       string
	TopLeft     GeoPoint
	BottomRight GeoPoint
}
