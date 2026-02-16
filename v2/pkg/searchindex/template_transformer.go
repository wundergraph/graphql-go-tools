package searchindex

import (
	"bytes"
	"fmt"
	"strings"
	"text/template"
)

// TemplateTransformer is the default TextTransformer, driven by the @embedding template string.
// Template uses Go text/template syntax: "{{.title}}. Topic: {{.topic}}. {{.body}}"
type TemplateTransformer struct {
	tmpl *template.Template
}

// NewTemplateTransformer creates a TemplateTransformer from a template string.
// The template string uses the syntax "{{title}}. Topic: {{topic}}. {{body}}"
// which is automatically converted to Go template syntax "{{.title}}. Topic: {{.topic}}. {{.body}}".
func NewTemplateTransformer(templateStr string) (*TemplateTransformer, error) {
	// Convert shorthand {{fieldName}} to Go template syntax {{.fieldName}}
	goTemplate := convertToGoTemplate(templateStr)
	tmpl, err := template.New("embedding").Parse(goTemplate)
	if err != nil {
		return nil, fmt.Errorf("searchindex: invalid embedding template: %w", err)
	}
	return &TemplateTransformer{tmpl: tmpl}, nil
}

// Transform applies the template to the entity fields and returns the resulting string.
func (t *TemplateTransformer) Transform(fields map[string]any) string {
	var buf bytes.Buffer
	if err := t.tmpl.Execute(&buf, fields); err != nil {
		return ""
	}
	return buf.String()
}

// convertToGoTemplate converts shorthand {{fieldName}} to {{.fieldName}}.
// It handles the case where the user writes templates without the dot prefix.
func convertToGoTemplate(s string) string {
	var result strings.Builder
	i := 0
	for i < len(s) {
		if i+1 < len(s) && s[i] == '{' && s[i+1] == '{' {
			// Find the closing }}
			end := strings.Index(s[i+2:], "}}")
			if end == -1 {
				result.WriteString(s[i:])
				break
			}
			content := strings.TrimSpace(s[i+2 : i+2+end])
			// Only add dot prefix if content doesn't already start with a dot or special character
			if len(content) > 0 && content[0] != '.' && content[0] != '$' {
				result.WriteString("{{.")
				result.WriteString(content)
				result.WriteString("}}")
			} else {
				result.WriteString("{{")
				result.WriteString(content)
				result.WriteString("}}")
			}
			i = i + 2 + end + 2
		} else {
			result.WriteByte(s[i])
			i++
		}
	}
	return result.String()
}
