package search_datasource

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/searchindex"
)

// ParseFilterJSON converts a GraphQL filter argument (as JSON) into a searchindex.Filter tree.
// The JSON structure matches the generated filter input types:
//
//	{
//	  "name": {"eq": "Widget"},
//	  "price": {"gte": 10.0, "lte": 100.0},
//	  "AND": [{"category": {"eq": "Electronics"}}],
//	  "OR": [...],
//	  "NOT": {...}
//	}
func ParseFilterJSON(data json.RawMessage, fields []IndexedFieldConfig) (*searchindex.Filter, error) {
	if len(data) == 0 {
		return nil, nil
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("invalid filter JSON: %w", err)
	}

	return parseFilterMap(raw, fields)
}

func parseFilterMap(raw map[string]json.RawMessage, fields []IndexedFieldConfig) (*searchindex.Filter, error) {
	filter := &searchindex.Filter{}

	for key, val := range raw {
		switch key {
		case "AND":
			var items []json.RawMessage
			if err := json.Unmarshal(val, &items); err != nil {
				return nil, fmt.Errorf("invalid AND value: %w", err)
			}
			for _, item := range items {
				child, err := ParseFilterJSON(item, fields)
				if err != nil {
					return nil, err
				}
				if child != nil {
					filter.And = append(filter.And, child)
				}
			}
		case "OR":
			var items []json.RawMessage
			if err := json.Unmarshal(val, &items); err != nil {
				return nil, fmt.Errorf("invalid OR value: %w", err)
			}
			for _, item := range items {
				child, err := ParseFilterJSON(item, fields)
				if err != nil {
					return nil, err
				}
				if child != nil {
					filter.Or = append(filter.Or, child)
				}
			}
		case "NOT":
			child, err := ParseFilterJSON(val, fields)
			if err != nil {
				return nil, err
			}
			filter.Not = child
		default:
			// Geo filter suffixes: <field>_distance, <field>_boundingBox
			if strings.HasSuffix(key, "_distance") {
				fieldName := strings.TrimSuffix(key, "_distance")
				geoFilter, err := parseGeoDistanceFilter(fieldName, val)
				if err != nil {
					return nil, err
				}
				if geoFilter != nil {
					filter.And = append(filter.And, geoFilter)
				}
				continue
			}
			if strings.HasSuffix(key, "_boundingBox") {
				fieldName := strings.TrimSuffix(key, "_boundingBox")
				geoFilter, err := parseGeoBoundingBoxFilter(fieldName, val)
				if err != nil {
					return nil, err
				}
				if geoFilter != nil {
					filter.And = append(filter.And, geoFilter)
				}
				continue
			}

			// Field filter
			fieldFilter, err := parseFieldFilter(key, val, fields)
			if err != nil {
				return nil, err
			}
			if fieldFilter != nil {
				filter.And = append(filter.And, fieldFilter)
			}
		}
	}

	// Simplify: if only one AND clause and nothing else, unwrap it
	if len(filter.And) == 1 && len(filter.Or) == 0 && filter.Not == nil {
		return filter.And[0], nil
	}

	return filter, nil
}

func parseFieldFilter(fieldName string, data json.RawMessage, fields []IndexedFieldConfig) (*searchindex.Filter, error) {
	cfg := findFieldConfig(fieldName, fields)
	if cfg == nil {
		return nil, fmt.Errorf("unknown filter field %q", fieldName)
	}

	switch cfg.IndexType {
	case searchindex.FieldTypeBool:
		// Boolean fields are just a direct value
		var boolVal bool
		if err := json.Unmarshal(data, &boolVal); err != nil {
			return nil, fmt.Errorf("invalid boolean filter for field %q: %w", fieldName, err)
		}
		return &searchindex.Filter{
			Term: &searchindex.TermFilter{Field: fieldName, Value: boolVal},
		}, nil

	case searchindex.FieldTypeText, searchindex.FieldTypeKeyword:
		return parseStringFilter(fieldName, data)

	case searchindex.FieldTypeNumeric:
		return parseNumericFilter(fieldName, data)

	case searchindex.FieldTypeDate, searchindex.FieldTypeDateTime:
		return parseDateFilter(fieldName, data)

	default:
		return nil, fmt.Errorf("unsupported filter type for field %q", fieldName)
	}
}

func parseStringFilter(fieldName string, data json.RawMessage) (*searchindex.Filter, error) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("invalid string filter for field %q: %w", fieldName, err)
	}

	for op, val := range raw {
		switch op {
		case "eq":
			var s string
			if err := json.Unmarshal(val, &s); err != nil {
				return nil, err
			}
			return &searchindex.Filter{Term: &searchindex.TermFilter{Field: fieldName, Value: s}}, nil
		case "ne":
			var s string
			if err := json.Unmarshal(val, &s); err != nil {
				return nil, err
			}
			return &searchindex.Filter{
				Not: &searchindex.Filter{Term: &searchindex.TermFilter{Field: fieldName, Value: s}},
			}, nil
		case "in":
			var values []string
			if err := json.Unmarshal(val, &values); err != nil {
				return nil, err
			}
			anyValues := make([]any, len(values))
			for i, v := range values {
				anyValues[i] = v
			}
			return &searchindex.Filter{Terms: &searchindex.TermsFilter{Field: fieldName, Values: anyValues}}, nil
		case "contains":
			var s string
			if err := json.Unmarshal(val, &s); err != nil {
				return nil, err
			}
			// Contains is modeled as a term match in full-text context
			return &searchindex.Filter{Term: &searchindex.TermFilter{Field: fieldName, Value: s}}, nil
		case "startsWith":
			var s string
			if err := json.Unmarshal(val, &s); err != nil {
				return nil, err
			}
			return &searchindex.Filter{Prefix: &searchindex.PrefixFilter{Field: fieldName, Value: s}}, nil
		}
	}

	return nil, nil
}

func parseNumericFilter(fieldName string, data json.RawMessage) (*searchindex.Filter, error) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("invalid numeric filter for field %q: %w", fieldName, err)
	}

	// Check for equality first
	if eqVal, ok := raw["eq"]; ok {
		var num float64
		if err := json.Unmarshal(eqVal, &num); err != nil {
			return nil, err
		}
		return &searchindex.Filter{Term: &searchindex.TermFilter{Field: fieldName, Value: num}}, nil
	}

	// Range filter
	rf := &searchindex.RangeFilter{Field: fieldName}
	hasRange := false

	if val, ok := raw["gt"]; ok {
		var num float64
		if err := json.Unmarshal(val, &num); err != nil {
			return nil, err
		}
		rf.GT = num
		rf.HasGT = true
		hasRange = true
	}
	if val, ok := raw["gte"]; ok {
		var num float64
		if err := json.Unmarshal(val, &num); err != nil {
			return nil, err
		}
		rf.GTE = num
		hasRange = true
	}
	if val, ok := raw["lt"]; ok {
		var num float64
		if err := json.Unmarshal(val, &num); err != nil {
			return nil, err
		}
		rf.LT = num
		rf.HasLT = true
		hasRange = true
	}
	if val, ok := raw["lte"]; ok {
		var num float64
		if err := json.Unmarshal(val, &num); err != nil {
			return nil, err
		}
		rf.LTE = num
		hasRange = true
	}

	if hasRange {
		return &searchindex.Filter{Range: rf}, nil
	}

	return nil, nil
}

func parseDateFilter(fieldName string, data json.RawMessage) (*searchindex.Filter, error) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("invalid date filter for field %q: %w", fieldName, err)
	}

	// Date equality is implemented as a range with GTE == LTE because backends
	// store dates as numeric timestamps, not strings (a TermFilter would fail).
	if eqVal, ok := raw["eq"]; ok {
		var s string
		if err := json.Unmarshal(eqVal, &s); err != nil {
			return nil, err
		}
		return &searchindex.Filter{Range: &searchindex.RangeFilter{Field: fieldName, GTE: s, LTE: s}}, nil
	}

	// Range filter — after is alias for gt, before is alias for lt
	rf := &searchindex.RangeFilter{Field: fieldName}
	hasRange := false

	if val, ok := raw["gt"]; ok {
		var s string
		if err := json.Unmarshal(val, &s); err != nil {
			return nil, err
		}
		rf.GT = s
		rf.HasGT = true
		hasRange = true
	} else if val, ok := raw["after"]; ok {
		var s string
		if err := json.Unmarshal(val, &s); err != nil {
			return nil, err
		}
		rf.GT = s
		rf.HasGT = true
		hasRange = true
	}
	if val, ok := raw["gte"]; ok {
		var s string
		if err := json.Unmarshal(val, &s); err != nil {
			return nil, err
		}
		rf.GTE = s
		hasRange = true
	}
	if val, ok := raw["lt"]; ok {
		var s string
		if err := json.Unmarshal(val, &s); err != nil {
			return nil, err
		}
		rf.LT = s
		rf.HasLT = true
		hasRange = true
	} else if val, ok := raw["before"]; ok {
		var s string
		if err := json.Unmarshal(val, &s); err != nil {
			return nil, err
		}
		rf.LT = s
		rf.HasLT = true
		hasRange = true
	}
	if val, ok := raw["lte"]; ok {
		var s string
		if err := json.Unmarshal(val, &s); err != nil {
			return nil, err
		}
		rf.LTE = s
		hasRange = true
	}

	if hasRange {
		return &searchindex.Filter{Range: rf}, nil
	}

	return nil, nil
}

func findFieldConfig(name string, fields []IndexedFieldConfig) *IndexedFieldConfig {
	for i := range fields {
		if fields[i].FieldName == name {
			return &fields[i]
		}
	}
	return nil
}

func parseGeoDistanceFilter(fieldName string, data json.RawMessage) (*searchindex.Filter, error) {
	var input struct {
		Center struct {
			Lat float64 `json:"lat"`
			Lon float64 `json:"lon"`
		} `json:"center"`
		Distance string `json:"distance"`
	}
	if err := json.Unmarshal(data, &input); err != nil {
		return nil, fmt.Errorf("invalid geo distance filter for field %q: %w", fieldName, err)
	}
	return &searchindex.Filter{
		GeoDistance: &searchindex.GeoDistanceFilter{
			Field:    fieldName,
			Center:   searchindex.GeoPoint{Lat: input.Center.Lat, Lon: input.Center.Lon},
			Distance: input.Distance,
		},
	}, nil
}

func parseGeoBoundingBoxFilter(fieldName string, data json.RawMessage) (*searchindex.Filter, error) {
	var input struct {
		TopLeft struct {
			Lat float64 `json:"lat"`
			Lon float64 `json:"lon"`
		} `json:"topLeft"`
		BottomRight struct {
			Lat float64 `json:"lat"`
			Lon float64 `json:"lon"`
		} `json:"bottomRight"`
	}
	if err := json.Unmarshal(data, &input); err != nil {
		return nil, fmt.Errorf("invalid geo bounding box filter for field %q: %w", fieldName, err)
	}
	return &searchindex.Filter{
		GeoBoundingBox: &searchindex.GeoBoundingBoxFilter{
			Field:       fieldName,
			TopLeft:     searchindex.GeoPoint{Lat: input.TopLeft.Lat, Lon: input.TopLeft.Lon},
			BottomRight: searchindex.GeoPoint{Lat: input.BottomRight.Lat, Lon: input.BottomRight.Lon},
		},
	}, nil
}
