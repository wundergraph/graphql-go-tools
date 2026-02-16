package searchindex

import "testing"

func TestTemplateTransformer(t *testing.T) {
	tests := []struct {
		name     string
		template string
		fields   map[string]any
		expected string
	}{
		{
			name:     "simple template",
			template: "{{title}}. {{body}}",
			fields:   map[string]any{"title": "Hello", "body": "World"},
			expected: "Hello. World",
		},
		{
			name:     "template with topic",
			template: "{{title}}. Topic: {{topic}}. {{body}}",
			fields:   map[string]any{"title": "Running Shoes", "topic": "Footwear", "body": "Great for jogging"},
			expected: "Running Shoes. Topic: Footwear. Great for jogging",
		},
		{
			name:     "already has dot prefix",
			template: "{{.title}} - {{.body}}",
			fields:   map[string]any{"title": "Test", "body": "Content"},
			expected: "Test - Content",
		},
		{
			name:     "missing field produces no-value placeholder",
			template: "{{title}}",
			fields:   map[string]any{},
			expected: "<no value>",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transformer, err := NewTemplateTransformer(tt.template)
			if err != nil {
				t.Fatalf("NewTemplateTransformer(%q): %v", tt.template, err)
			}
			got := transformer.Transform(tt.fields)
			if got != tt.expected {
				t.Errorf("Transform() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestConvertToGoTemplate(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"{{title}}", "{{.title}}"},
		{"{{.title}}", "{{.title}}"},
		{"{{title}} {{body}}", "{{.title}} {{.body}}"},
		{"no template", "no template"},
		{"{{ title }}", "{{.title}}"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := convertToGoTemplate(tt.input)
			if got != tt.expected {
				t.Errorf("convertToGoTemplate(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestFuncTransformer(t *testing.T) {
	ft := &FuncTransformer{
		Fn: func(fields map[string]any) string {
			return fields["a"].(string) + " " + fields["b"].(string)
		},
	}

	got := ft.Transform(map[string]any{"a": "hello", "b": "world"})
	if got != "hello world" {
		t.Errorf("FuncTransformer.Transform() = %q, want %q", got, "hello world")
	}
}
