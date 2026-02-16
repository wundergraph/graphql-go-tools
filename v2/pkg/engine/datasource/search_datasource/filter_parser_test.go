package search_datasource

import (
	"encoding/json"
	"testing"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/searchindex"
)

func TestParseFilterJSON(t *testing.T) {
	fields := []IndexedFieldConfig{
		{FieldName: "name", IndexType: searchindex.FieldTypeText, Filterable: true},
		{FieldName: "category", IndexType: searchindex.FieldTypeKeyword, Filterable: true},
		{FieldName: "price", IndexType: searchindex.FieldTypeNumeric, GraphQLType: "Float!", Filterable: true},
		{FieldName: "inStock", IndexType: searchindex.FieldTypeBool, Filterable: true},
	}

	t.Run("nil input", func(t *testing.T) {
		f, err := ParseFilterJSON(nil, fields)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if f != nil {
			t.Fatal("expected nil filter")
		}
	})

	t.Run("term filter string", func(t *testing.T) {
		input := json.RawMessage(`{"category": {"eq": "Electronics"}}`)
		f, err := ParseFilterJSON(input, fields)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if f == nil || f.Term == nil {
			t.Fatal("expected term filter")
		}
		if f.Term.Field != "category" {
			t.Errorf("field = %q, want %q", f.Term.Field, "category")
		}
		if f.Term.Value != "Electronics" {
			t.Errorf("value = %v, want %q", f.Term.Value, "Electronics")
		}
	})

	t.Run("boolean filter", func(t *testing.T) {
		input := json.RawMessage(`{"inStock": true}`)
		f, err := ParseFilterJSON(input, fields)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if f == nil || f.Term == nil {
			t.Fatal("expected term filter for boolean")
		}
		if f.Term.Value != true {
			t.Errorf("value = %v, want true", f.Term.Value)
		}
	})

	t.Run("numeric range filter", func(t *testing.T) {
		input := json.RawMessage(`{"price": {"gte": 10.0, "lte": 100.0}}`)
		f, err := ParseFilterJSON(input, fields)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if f == nil || f.Range == nil {
			t.Fatal("expected range filter")
		}
		if f.Range.GTE != 10.0 {
			t.Errorf("GTE = %v, want 10.0", f.Range.GTE)
		}
		if f.Range.LTE != 100.0 {
			t.Errorf("LTE = %v, want 100.0", f.Range.LTE)
		}
	})

	t.Run("prefix filter", func(t *testing.T) {
		input := json.RawMessage(`{"name": {"startsWith": "Widget"}}`)
		f, err := ParseFilterJSON(input, fields)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if f == nil || f.Prefix == nil {
			t.Fatal("expected prefix filter")
		}
		if f.Prefix.Value != "Widget" {
			t.Errorf("value = %q, want %q", f.Prefix.Value, "Widget")
		}
	})

	t.Run("terms filter (IN)", func(t *testing.T) {
		input := json.RawMessage(`{"category": {"in": ["A", "B", "C"]}}`)
		f, err := ParseFilterJSON(input, fields)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if f == nil || f.Terms == nil {
			t.Fatal("expected terms filter")
		}
		if len(f.Terms.Values) != 3 {
			t.Errorf("len(values) = %d, want 3", len(f.Terms.Values))
		}
	})

	t.Run("NOT filter", func(t *testing.T) {
		input := json.RawMessage(`{"NOT": {"category": {"eq": "Obsolete"}}}`)
		f, err := ParseFilterJSON(input, fields)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if f == nil || f.Not == nil {
			t.Fatal("expected NOT filter")
		}
		if f.Not.Term == nil {
			t.Fatal("expected term inside NOT")
		}
	})

	t.Run("AND filter", func(t *testing.T) {
		input := json.RawMessage(`{"AND": [{"category": {"eq": "A"}}, {"inStock": true}]}`)
		f, err := ParseFilterJSON(input, fields)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if f == nil || len(f.And) != 2 {
			t.Fatalf("expected 2 AND clauses, got %d", len(f.And))
		}
	})

	t.Run("OR filter", func(t *testing.T) {
		input := json.RawMessage(`{"OR": [{"category": {"eq": "A"}}, {"category": {"eq": "B"}}]}`)
		f, err := ParseFilterJSON(input, fields)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if f == nil || len(f.Or) != 2 {
			t.Fatalf("expected 2 OR clauses, got %d", len(f.Or))
		}
	})

	t.Run("numeric equality", func(t *testing.T) {
		input := json.RawMessage(`{"price": {"eq": 42.5}}`)
		f, err := ParseFilterJSON(input, fields)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if f == nil || f.Term == nil {
			t.Fatal("expected term filter for numeric equality")
		}
		if f.Term.Value != 42.5 {
			t.Errorf("value = %v, want 42.5", f.Term.Value)
		}
	})

	t.Run("ne filter (NOT eq)", func(t *testing.T) {
		input := json.RawMessage(`{"category": {"ne": "Obsolete"}}`)
		f, err := ParseFilterJSON(input, fields)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if f == nil || f.Not == nil || f.Not.Term == nil {
			t.Fatal("expected NOT(term) filter")
		}
		if f.Not.Term.Value != "Obsolete" {
			t.Errorf("value = %v, want %q", f.Not.Term.Value, "Obsolete")
		}
	})

	t.Run("unknown field", func(t *testing.T) {
		input := json.RawMessage(`{"unknown": {"eq": "val"}}`)
		_, err := ParseFilterJSON(input, fields)
		if err == nil {
			t.Fatal("expected error for unknown field")
		}
	})

	t.Run("geo distance filter", func(t *testing.T) {
		input := json.RawMessage(`{"location_distance": {"center": {"lat": 40.7128, "lon": -74.006}, "distance": "10km"}}`)
		f, err := ParseFilterJSON(input, fields)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if f == nil || f.GeoDistance == nil {
			t.Fatal("expected geo distance filter")
		}
		if f.GeoDistance.Field != "location" {
			t.Errorf("field = %q, want %q", f.GeoDistance.Field, "location")
		}
		if f.GeoDistance.Distance != "10km" {
			t.Errorf("distance = %q, want %q", f.GeoDistance.Distance, "10km")
		}
		if f.GeoDistance.Center.Lat != 40.7128 {
			t.Errorf("lat = %v, want 40.7128", f.GeoDistance.Center.Lat)
		}
		if f.GeoDistance.Center.Lon != -74.006 {
			t.Errorf("lon = %v, want -74.006", f.GeoDistance.Center.Lon)
		}
	})

	t.Run("geo bounding box filter", func(t *testing.T) {
		input := json.RawMessage(`{"location_boundingBox": {"topLeft": {"lat": 41.0, "lon": -74.5}, "bottomRight": {"lat": 40.5, "lon": -73.5}}}`)
		f, err := ParseFilterJSON(input, fields)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if f == nil || f.GeoBoundingBox == nil {
			t.Fatal("expected geo bounding box filter")
		}
		if f.GeoBoundingBox.Field != "location" {
			t.Errorf("field = %q, want %q", f.GeoBoundingBox.Field, "location")
		}
		if f.GeoBoundingBox.TopLeft.Lat != 41.0 {
			t.Errorf("topLeft.lat = %v, want 41.0", f.GeoBoundingBox.TopLeft.Lat)
		}
		if f.GeoBoundingBox.BottomRight.Lon != -73.5 {
			t.Errorf("bottomRight.lon = %v, want -73.5", f.GeoBoundingBox.BottomRight.Lon)
		}
	})
}

func TestParseFilterJSON_DateFields(t *testing.T) {
	fields := []IndexedFieldConfig{
		{FieldName: "eventDate", IndexType: searchindex.FieldTypeDate, Filterable: true},
		{FieldName: "createdAt", IndexType: searchindex.FieldTypeDateTime, Filterable: true},
	}

	t.Run("date eq", func(t *testing.T) {
		input := json.RawMessage(`{"eventDate": {"eq": "2024-01-15"}}`)
		f, err := ParseFilterJSON(input, fields)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if f == nil || f.Range == nil {
			t.Fatal("expected range filter (date eq is translated to GTE+LTE)")
		}
		if f.Range.Field != "eventDate" {
			t.Errorf("field = %q, want %q", f.Range.Field, "eventDate")
		}
		if f.Range.GTE != "2024-01-15" {
			t.Errorf("GTE = %v, want %q", f.Range.GTE, "2024-01-15")
		}
		if f.Range.LTE != "2024-01-15" {
			t.Errorf("LTE = %v, want %q", f.Range.LTE, "2024-01-15")
		}
	})

	t.Run("datetime range gte/lte", func(t *testing.T) {
		input := json.RawMessage(`{"createdAt": {"gte": "2024-01-01T00:00:00Z", "lte": "2024-12-31T23:59:59Z"}}`)
		f, err := ParseFilterJSON(input, fields)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if f == nil || f.Range == nil {
			t.Fatal("expected range filter")
		}
		if f.Range.Field != "createdAt" {
			t.Errorf("field = %q, want %q", f.Range.Field, "createdAt")
		}
		if f.Range.GTE != "2024-01-01T00:00:00Z" {
			t.Errorf("GTE = %v, want %q", f.Range.GTE, "2024-01-01T00:00:00Z")
		}
		if f.Range.LTE != "2024-12-31T23:59:59Z" {
			t.Errorf("LTE = %v, want %q", f.Range.LTE, "2024-12-31T23:59:59Z")
		}
	})

	t.Run("date after alias", func(t *testing.T) {
		input := json.RawMessage(`{"eventDate": {"after": "2024-06-01"}}`)
		f, err := ParseFilterJSON(input, fields)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if f == nil || f.Range == nil {
			t.Fatal("expected range filter")
		}
		if f.Range.GT != "2024-06-01" {
			t.Errorf("GT = %v, want %q", f.Range.GT, "2024-06-01")
		}
		if !f.Range.HasGT {
			t.Error("HasGT should be true")
		}
	})

	t.Run("date before alias", func(t *testing.T) {
		input := json.RawMessage(`{"eventDate": {"before": "2025-01-01"}}`)
		f, err := ParseFilterJSON(input, fields)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if f == nil || f.Range == nil {
			t.Fatal("expected range filter")
		}
		if f.Range.LT != "2025-01-01" {
			t.Errorf("LT = %v, want %q", f.Range.LT, "2025-01-01")
		}
		if !f.Range.HasLT {
			t.Error("HasLT should be true")
		}
	})

	t.Run("gt takes precedence over after", func(t *testing.T) {
		input := json.RawMessage(`{"eventDate": {"gt": "2024-03-01", "after": "2024-06-01"}}`)
		f, err := ParseFilterJSON(input, fields)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if f == nil || f.Range == nil {
			t.Fatal("expected range filter")
		}
		// gt should win over after
		if f.Range.GT != "2024-03-01" {
			t.Errorf("GT = %v, want %q (gt should take precedence)", f.Range.GT, "2024-03-01")
		}
	})

	t.Run("combined after and before", func(t *testing.T) {
		input := json.RawMessage(`{"createdAt": {"after": "2024-01-01T00:00:00Z", "before": "2025-01-01T00:00:00Z"}}`)
		f, err := ParseFilterJSON(input, fields)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if f == nil || f.Range == nil {
			t.Fatal("expected range filter")
		}
		if f.Range.GT != "2024-01-01T00:00:00Z" {
			t.Errorf("GT = %v, want %q", f.Range.GT, "2024-01-01T00:00:00Z")
		}
		if f.Range.LT != "2025-01-01T00:00:00Z" {
			t.Errorf("LT = %v, want %q", f.Range.LT, "2025-01-01T00:00:00Z")
		}
	})
}
