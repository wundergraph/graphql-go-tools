package searchindex

// FieldType defines the type of indexing for a field.
type FieldType int

const (
	FieldTypeText    FieldType = iota // Analyzed full-text search
	FieldTypeKeyword                  // Exact match, not analyzed
	FieldTypeNumeric                  // Numeric range queries
	FieldTypeBool                     // Boolean filtering
	FieldTypeVector                   // Pre-computed embedding vector
	FieldTypeGeo                      // Latitude/longitude geo-point
	FieldTypeDate                     // Calendar date (ISO 8601 full-date, e.g. "2024-01-15")
	FieldTypeDateTime                 // Instant (RFC 3339, e.g. "2024-01-15T10:30:00.000Z")
)

func (f FieldType) String() string {
	switch f {
	case FieldTypeText:
		return "TEXT"
	case FieldTypeKeyword:
		return "KEYWORD"
	case FieldTypeNumeric:
		return "NUMERIC"
	case FieldTypeBool:
		return "BOOL"
	case FieldTypeVector:
		return "VECTOR"
	case FieldTypeGeo:
		return "GEO"
	case FieldTypeDate:
		return "DATE"
	case FieldTypeDateTime:
		return "DATETIME"
	default:
		return "UNKNOWN"
	}
}

// ParseFieldType converts a string to a FieldType.
func ParseFieldType(s string) (FieldType, bool) {
	switch s {
	case "TEXT":
		return FieldTypeText, true
	case "KEYWORD":
		return FieldTypeKeyword, true
	case "NUMERIC":
		return FieldTypeNumeric, true
	case "BOOL":
		return FieldTypeBool, true
	case "VECTOR":
		return FieldTypeVector, true
	case "GEO":
		return FieldTypeGeo, true
	case "DATE":
		return FieldTypeDate, true
	case "DATETIME":
		return FieldTypeDateTime, true
	default:
		return 0, false
	}
}

// FieldConfig describes how a field is indexed.
type FieldConfig struct {
	Name         string
	Type         FieldType
	Filterable   bool
	Sortable     bool
	Dimensions   int     // Required for FieldTypeVector
	Weight       float64 // Search boost for TEXT fields; 0 treated as 1.0
	Autocomplete bool    // Enable term autocomplete for this field
}

// IndexConfig describes the schema of an index.
type IndexConfig struct {
	Name   string
	Fields []FieldConfig
}
