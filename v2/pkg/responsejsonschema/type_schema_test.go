package responsejsonschema

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBuildResponseSchema_BuiltInScalars(t *testing.T) {
	tests := []struct {
		name    string
		typeRef string
		valid   []string
		invalid []string
	}{
		{
			name:    "String",
			typeRef: "String!",
			valid:   []string{`"hello"`, `""`},
			invalid: []string{`null`, `true`, `1`, `1.5`, `{}`, `[]`},
		},
		{
			name:    "Boolean",
			typeRef: "Boolean!",
			valid:   []string{`true`, `false`},
			invalid: []string{`null`, `"true"`, `0`, `{}`, `[]`},
		},
		{
			name:    "Int",
			typeRef: "Int!",
			valid:   []string{`0`, `-12`, `2147483647`},
			invalid: []string{`null`, `1.5`, `"1"`, `true`, `{}`, `[]`},
		},
		{
			name:    "Float",
			typeRef: "Float!",
			valid:   []string{`0`, `-12.5`, `1.25e3`},
			invalid: []string{`null`, `"1.5"`, `true`, `{}`, `[]`},
		},
		{
			name:    "ID",
			typeRef: "ID!",
			valid:   []string{`"product-1"`, `0`, `42`},
			invalid: []string{`null`, `1.5`, `true`, `{}`, `[]`},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			schema := buildSchema(
				t,
				fmt.Sprintf(`type Query { value: %s }`, test.typeRef),
				`query { value }`,
				[]string{"value"},
			)
			requireSchemaValidation(t, schema, test.valid, test.invalid)
		})
	}
}

func TestBuildResponseSchema_GraphQLNullability(t *testing.T) {
	tests := []struct {
		name    string
		typeRef string
		valid   []string
		invalid []string
	}{
		{
			name:    "nullable",
			typeRef: "String",
			valid:   []string{`"hello"`, `null`},
			invalid: []string{`true`, `1`, `{}`, `[]`},
		},
		{
			name:    "non-null",
			typeRef: "String!",
			valid:   []string{`"hello"`},
			invalid: []string{`null`, `true`, `1`, `{}`, `[]`},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			schema := buildSchema(
				t,
				fmt.Sprintf(`type Query { value: %s }`, test.typeRef),
				`query { value }`,
				[]string{"value"},
			)
			requireSchemaValidation(t, schema, test.valid, test.invalid)
		})
	}
}

func TestBuildResponseSchema_ListAndItemNullability(t *testing.T) {
	tests := []struct {
		name    string
		typeRef string
		valid   []string
		invalid []string
	}{
		{
			name:    "nullable list and nullable items",
			typeRef: "[String]",
			valid:   []string{`null`, `[]`, `["one"]`, `["one",null]`},
			invalid: []string{`"one"`, `[true]`, `[1]`, `{}`},
		},
		{
			name:    "non-null list and nullable items",
			typeRef: "[String]!",
			valid:   []string{`[]`, `["one"]`, `["one",null]`},
			invalid: []string{`null`, `"one"`, `[true]`, `[1]`, `{}`},
		},
		{
			name:    "nullable list and non-null items",
			typeRef: "[String!]",
			valid:   []string{`null`, `[]`, `["one"]`},
			invalid: []string{`[null]`, `"one"`, `[true]`, `[1]`, `{}`},
		},
		{
			name:    "non-null list and non-null items",
			typeRef: "[String!]!",
			valid:   []string{`[]`, `["one"]`},
			invalid: []string{`null`, `[null]`, `"one"`, `[true]`, `[1]`, `{}`},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			schema := buildSchema(
				t,
				fmt.Sprintf(`type Query { values: %s }`, test.typeRef),
				`query { values }`,
				[]string{"values"},
			)
			requireSchemaValidation(t, schema, test.valid, test.invalid)
		})
	}
}

func TestBuildResponseSchema_EnumUsesAccessibleValues(t *testing.T) {
	schema := buildSchema(
		t,
		`directive @inaccessible on ENUM_VALUE

		type Query {
			status: Status!
		}

		enum Status {
			OPEN
			INTERNAL @inaccessible
			CLOSED
		}`,
		`query { status }`,
		[]string{"status"},
	)

	var decoded struct {
		Enum []string `json:"enum"`
	}
	require.NoError(t, json.Unmarshal(schema, &decoded))
	require.ElementsMatch(t, []string{"OPEN", "CLOSED"}, decoded.Enum)

	requireSchemaValidation(
		t,
		schema,
		[]string{`"OPEN"`, `"CLOSED"`},
		[]string{`"INTERNAL"`, `"UNKNOWN"`, `null`, `true`, `1`, `{}`, `[]`},
	)
}

func TestBuildResponseSchema_CustomScalarMappings(t *testing.T) {
	definitionInput := `
		scalar DateTime
		scalar Opaque

		type Query {
			mappedNullable: DateTime
			mappedRequired: DateTime!
			unmappedNullable: Opaque
			unmappedRequired: Opaque!
		}
	`
	operationInput := `query {
		mappedNullable
		mappedRequired
		unmappedNullable
		unmappedRequired
	}`
	mapping := json.RawMessage(`{"anyOf":[{"type":"string","pattern":"^dt:"},{"type":"null"}]}`)
	option := WithCustomScalarMappings(map[string]json.RawMessage{"DateTime": mapping})

	t.Run("mapped nullable is the supplied schema or null", func(t *testing.T) {
		schema := buildSchema(t, definitionInput, operationInput, []string{"mappedNullable"}, option)
		require.NotContains(t, string(schema), `"format"`)
		requireSchemaValidation(
			t,
			schema,
			[]string{`"dt:2026-07-10"`, `null`},
			[]string{`"2026-07-10"`, `1`, `true`, `{}`, `[]`},
		)
	})

	t.Run("mapped non-null explicitly excludes null", func(t *testing.T) {
		schema := buildSchema(t, definitionInput, operationInput, []string{"mappedRequired"}, option)
		require.NotContains(t, string(schema), `"format"`)
		require.Contains(t, string(schema), `"not"`)
		requireSchemaValidation(
			t,
			schema,
			[]string{`"dt:2026-07-10"`},
			[]string{`null`, `"2026-07-10"`, `1`, `true`, `{}`, `[]`},
		)
	})

	t.Run("unmapped nullable accepts any JSON value", func(t *testing.T) {
		schema := buildSchema(t, definitionInput, operationInput, []string{"unmappedNullable"})
		require.JSONEq(t, `{}`, string(schema))
		requireSchemaValidation(
			t,
			schema,
			[]string{`null`, `"opaque"`, `1`, `true`, `{}`, `[]`},
			nil,
		)
	})

	t.Run("unmapped non-null accepts any JSON value except null", func(t *testing.T) {
		schema := buildSchema(t, definitionInput, operationInput, []string{"unmappedRequired"})
		requireSchemaValidation(
			t,
			schema,
			[]string{`"opaque"`, `1`, `true`, `{}`, `[]`},
			[]string{`null`},
		)
	})
}

func TestBuildResponseSchema_RejectsInvalidCustomScalarMapping(t *testing.T) {
	definition := parseDocument(t, `
		scalar DateTime
		type Query { value: DateTime }
	`)
	operation := parseDocument(t, `query { value }`)

	tests := []struct {
		name      string
		mapping   json.RawMessage
		wantError string
	}{
		{
			name:      "malformed JSON",
			mapping:   json.RawMessage(`{"type":`),
			wantError: `custom scalar mapping "DateTime" is not valid JSON`,
		},
		{
			name:      "JSON value that is not a schema",
			mapping:   json.RawMessage(`[]`),
			wantError: `custom scalar mapping "DateTime" is not a JSON Schema`,
		},
		{
			name:      "invalid JSON Schema keyword value",
			mapping:   json.RawMessage(`{"type":17}`),
			wantError: `custom scalar mapping "DateTime" is not a valid JSON Schema`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := Build(
				&operation,
				&definition,
				[]string{"value"},
				WithCustomScalarMappings(map[string]json.RawMessage{"DateTime": test.mapping}),
			)
			require.ErrorContains(t, err, test.wantError)
		})
	}
}
