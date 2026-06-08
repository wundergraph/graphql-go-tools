package jsonschema

import (
	"encoding/json"
)

// SchemaType represents the type of a JSON Schema property
type SchemaType string

const (
	TypeObject  SchemaType = "object"
	TypeArray   SchemaType = "array"
	TypeString  SchemaType = "string"
	TypeNumber  SchemaType = "number"
	TypeInteger SchemaType = "integer"
	TypeBoolean SchemaType = "boolean"
	TypeNull    SchemaType = "null"
)

// JsonSchema represents a JSON Schema definition
type JsonSchema struct {
	// Core schema fields
	Type                 SchemaType             `json:"type,omitempty"`
	Properties           map[string]*JsonSchema `json:"properties,omitempty"`
	Required             []string               `json:"required,omitempty"`
	AdditionalProperties *bool                  `json:"additionalProperties,omitempty"`
	Description          string                 `json:"description,omitempty"`
	// Nullable is tracked internally; serialization expresses nullability in the
	// JSON Schema 2020-12 form (type-union, anyOf, or null in enum), not the
	// OpenAPI 3.0 "nullable" keyword.
	Nullable bool `json:"-"`

	// Ref references a schema defined under the root "$defs" (e.g. "#/$defs/MyInput").
	// Used to represent recursive input types, which cannot be inlined.
	Ref string `json:"$ref,omitempty"`
	// Defs holds reusable schema definitions, referenced via Ref. Only populated
	// on the root schema.
	Defs map[string]*JsonSchema `json:"$defs,omitempty"`

	// Array-specific fields
	Items *JsonSchema `json:"items,omitempty"`

	// Enum values
	Enum []string `json:"enum,omitempty"`

	// Default value
	Default interface{} `json:"default,omitempty"`

	// String-specific fields
	Format string `json:"format,omitempty"`

	// Number-specific fields
	Minimum *float64 `json:"minimum,omitempty"`
	Maximum *float64 `json:"maximum,omitempty"`

	// Additional validation
	Pattern string `json:"pattern,omitempty"`
}

// MarshalJSON customizes JSON serialization to omit empty fields
func (s *JsonSchema) MarshalJSON() ([]byte, error) {
	// Use a map to only include non-empty fields
	m := make(map[string]interface{})

	// Nullability is expressed per JSON Schema 2020-12:
	//   - typed schemas:        "type": [<type>, "null"]
	//   - enum schemas:         null appended to the "enum" array
	//   - $ref schemas:         {"anyOf": [{"$ref": ...}, {"type": "null"}]}
	// rather than the OpenAPI 3.0 keyword "nullable: true", which standard
	// validators ignore.

	if s.Type != "" {
		if s.Nullable {
			m["type"] = []string{string(s.Type), "null"}
		} else {
			m["type"] = string(s.Type)
		}
	}

	if len(s.Properties) > 0 {
		m["properties"] = s.Properties
	}

	if len(s.Required) > 0 {
		m["required"] = s.Required
	}

	if s.AdditionalProperties != nil {
		m["additionalProperties"] = *s.AdditionalProperties
	}

	if s.Description != "" {
		m["description"] = s.Description
	}

	if s.Items != nil {
		m["items"] = s.Items
	}

	if len(s.Enum) > 0 {
		if s.Nullable {
			enum := make([]any, 0, len(s.Enum)+1)
			for _, v := range s.Enum {
				enum = append(enum, v)
			}
			enum = append(enum, nil)
			m["enum"] = enum
		} else {
			m["enum"] = s.Enum
		}
	}

	if s.Default != nil {
		m["default"] = s.Default
	}

	if s.Format != "" {
		m["format"] = s.Format
	}

	if s.Minimum != nil {
		m["minimum"] = *s.Minimum
	}

	if s.Maximum != nil {
		m["maximum"] = *s.Maximum
	}

	if s.Pattern != "" {
		m["pattern"] = s.Pattern
	}

	if s.Ref != "" {
		if s.Nullable {
			m["anyOf"] = []map[string]string{
				{"$ref": s.Ref},
				{"type": "null"},
			}
		} else {
			m["$ref"] = s.Ref
		}
	}

	if len(s.Defs) > 0 {
		m["$defs"] = s.Defs
	}

	return json.Marshal(m)
}

// NewObjectSchema creates a new schema for an object type
func NewObjectSchema() *JsonSchema {
	additionalProps := false

	return &JsonSchema{
		Type:                 TypeObject,
		Properties:           make(map[string]*JsonSchema),
		AdditionalProperties: &additionalProps,
		Required:             []string{},
		Nullable:             true, // Default to nullable
	}
}

// NewRefSchema creates a schema that references a definition under the root "$defs".
func NewRefSchema(typeName string) *JsonSchema {
	return &JsonSchema{
		Ref:      defsRef(typeName),
		Nullable: true, // Default to nullable; callers adjust based on context
	}
}

// defsRef returns the JSON Pointer to a definition under the root "$defs".
func defsRef(typeName string) string {
	return "#/$defs/" + typeName
}

// NewAnySchema creates a schema representing any value (serialized as {} in JSON)
func NewAnySchema() *JsonSchema {
	// This will represent as an empty object in JSON schema
	return &JsonSchema{
		Nullable: true, // Default to nullable
	}
}

// NewArraySchema creates a new schema for an array type
func NewArraySchema(items *JsonSchema) *JsonSchema {
	return &JsonSchema{
		Type:     TypeArray,
		Items:    items,
		Nullable: true, // Default to nullable
	}
}

// NewStringSchema creates a new schema for a string type
func NewStringSchema() *JsonSchema {
	return &JsonSchema{
		Type:     TypeString,
		Nullable: true, // Default to nullable
	}
}

// NewIntegerSchema creates a new schema for an integer type
func NewIntegerSchema() *JsonSchema {
	return &JsonSchema{
		Type:     TypeInteger,
		Nullable: true, // Default to nullable
	}
}

// NewNumberSchema creates a new schema for a number type
func NewNumberSchema() *JsonSchema {
	return &JsonSchema{
		Type:     TypeNumber,
		Nullable: true, // Default to nullable
	}
}

// NewBooleanSchema creates a new schema for a boolean type
func NewBooleanSchema() *JsonSchema {
	return &JsonSchema{
		Type:     TypeBoolean,
		Nullable: true, // Default to nullable
	}
}

// NewEnumSchema creates a new schema for an enum type
func NewEnumSchema(values []string) *JsonSchema {
	return &JsonSchema{
		Type:     TypeString,
		Enum:     values,
		Nullable: true, // Default to nullable
	}
}

// WithDescription adds a description to the schema
func (s *JsonSchema) WithDescription(description string) *JsonSchema {
	s.Description = description
	return s
}

// WithDefault adds a default value to the schema
func (s *JsonSchema) WithDefault(defaultValue interface{}) *JsonSchema {
	s.Default = defaultValue
	return s
}

// WithFormat adds a format to a string schema
func (s *JsonSchema) WithFormat(format string) *JsonSchema {
	s.Format = format
	return s
}

// WithNullable marks a schema as nullable
func (s *JsonSchema) WithNullable(nullable bool) *JsonSchema {
	s.Nullable = nullable
	return s
}
