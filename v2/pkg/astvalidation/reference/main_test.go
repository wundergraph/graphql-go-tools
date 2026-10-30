package main

import "testing"

func TestConverterPreservesRuleIdentifiers(t *testing.T) {
	tests := []struct {
		name string
		line string
		want string
	}{
		{
			name: "helper call argument",
			line: "  return expectValidationErrors(FieldsOnCorrectTypeRule, queryStr)",
			want: "  return ExpectValidationErrors(t,FieldsOnCorrectTypeRule, queryStr)",
		},
		{
			name: "multiline helper call argument",
			line: "    FieldsOnCorrectTypeRule,",
			want: "    FieldsOnCorrectTypeRule,",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			converter := &Converter{}
			got, skip := converter.transformLine(tt.line)
			if skip {
				t.Fatal("transformLine unexpectedly skipped the rule argument")
			}
			if got != tt.want {
				t.Fatalf("transformLine() = %q, want %q", got, tt.want)
			}
		})
	}
}
