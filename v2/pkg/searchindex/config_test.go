package searchindex

import "testing"

func TestParseFieldType(t *testing.T) {
	tests := []struct {
		input    string
		expected FieldType
		ok       bool
	}{
		{"TEXT", FieldTypeText, true},
		{"KEYWORD", FieldTypeKeyword, true},
		{"NUMERIC", FieldTypeNumeric, true},
		{"BOOL", FieldTypeBool, true},
		{"VECTOR", FieldTypeVector, true},
		{"UNKNOWN", 0, false},
		{"", 0, false},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			ft, ok := ParseFieldType(tt.input)
			if ok != tt.ok {
				t.Errorf("ParseFieldType(%q): ok = %v, want %v", tt.input, ok, tt.ok)
			}
			if ok && ft != tt.expected {
				t.Errorf("ParseFieldType(%q) = %v, want %v", tt.input, ft, tt.expected)
			}
		})
	}
}

func TestFieldTypeString(t *testing.T) {
	tests := []struct {
		ft       FieldType
		expected string
	}{
		{FieldTypeText, "TEXT"},
		{FieldTypeKeyword, "KEYWORD"},
		{FieldTypeNumeric, "NUMERIC"},
		{FieldTypeBool, "BOOL"},
		{FieldTypeVector, "VECTOR"},
		{FieldType(99), "UNKNOWN"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			if got := tt.ft.String(); got != tt.expected {
				t.Errorf("FieldType.String() = %q, want %q", got, tt.expected)
			}
		})
	}
}
