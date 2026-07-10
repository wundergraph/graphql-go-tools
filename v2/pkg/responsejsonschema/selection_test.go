package responsejsonschema

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBuildResponseSchema_PathLookupAndCustomRoot(t *testing.T) {
	definitionInput := `
		schema {
			query: Root
		}

		type Root {
			investigation: Investigation!
		}

		type Investigation {
			title: String!
		}
	`
	operationInput := `
		query {
			firstInvestigation: investigation {
				displayTitle: title
			}
		}
	`

	schema := buildSchema(
		t,
		definitionInput,
		operationInput,
		[]string{"firstInvestigation", "displayTitle"},
	)
	requireSchemaValidation(
		t,
		schema,
		[]string{`"open"`},
		[]string{`null`, `{"displayTitle":"open"}`, `{"firstInvestigation":{"displayTitle":"open"}}`},
	)

	definition := parseDocument(t, definitionInput)
	operation := parseDocument(t, operationInput)
	_, err := Build(&operation, &definition, []string{"investigation", "title"})
	require.ErrorContains(t, err, `response field "investigation" not found`)
}

func TestBuildResponseSchema_ObjectSelection(t *testing.T) {
	schema := buildSchema(
		t,
		investigationSchema,
		`query {
			firstInvestigation: investigation {
				title
				id
			}
		}`,
		[]string{"firstInvestigation"},
	)

	var decoded struct {
		Properties           map[string]json.RawMessage `json:"properties"`
		Required             []string                   `json:"required"`
		AdditionalProperties *bool                      `json:"additionalProperties"`
	}
	require.NoError(t, json.Unmarshal(schema, &decoded))
	require.ElementsMatch(t, []string{"id", "title"}, mapKeys(decoded.Properties))
	require.Equal(t, []string{"id", "title"}, decoded.Required)
	require.NotNil(t, decoded.AdditionalProperties)
	require.False(t, *decoded.AdditionalProperties)

	requireSchemaValidation(
		t,
		schema,
		[]string{`{"id":"inv-1","title":"Open"}`},
		[]string{
			`null`,
			`{}`,
			`{"id":"inv-1"}`,
			`{"title":"Open"}`,
			`{"id":"inv-1","title":"Open","unselected":"leak"}`,
		},
	)
}

func TestBuildResponseSchema_UsesResponseAliases(t *testing.T) {
	schema := buildSchema(
		t,
		investigationSchema,
		`query {
			firstInvestigation: investigation {
				displayTitle: title
				investigationID: id
			}
		}`,
		[]string{"firstInvestigation"},
	)

	requireSchemaValidation(
		t,
		schema,
		[]string{`{"displayTitle":"Open","investigationID":"inv-1"}`},
		[]string{
			`{"id":"inv-1","title":"Open"}`,
			`{"displayTitle":"Open","id":"inv-1"}`,
			`{"title":"Open","investigationID":"inv-1"}`,
		},
	)
}

func TestBuildResponseSchema_UnconditionalNullablePropertyIsRequired(t *testing.T) {
	schema := buildSchema(
		t,
		investigationSchema,
		`query {
			firstInvestigation: investigation {
				note
			}
		}`,
		[]string{"firstInvestigation"},
	)

	var decoded struct {
		Required []string `json:"required"`
	}
	require.NoError(t, json.Unmarshal(schema, &decoded))
	require.Equal(t, []string{"note"}, decoded.Required)

	requireSchemaValidation(
		t,
		schema,
		[]string{`{"note":"Follow up"}`, `{"note":null}`},
		[]string{`{}`, `null`, `{"note":true}`},
	)
}

func TestBuildResponseSchema_RepeatedPropertiesMergeDeterministically(t *testing.T) {
	definitionInput := `
		type Query {
			investigation: Investigation!
		}

		type Investigation {
			subject: Subject!
		}

		type Subject {
			id: ID!
			name: String!
		}
	`
	operationDirectFirst := `
		query {
			firstInvestigation: investigation {
				subject {
					name
				}
			}
			...InvestigationID
		}

		fragment InvestigationID on Query {
			firstInvestigation: investigation {
				subject {
					id
				}
			}
		}
	`
	operationFragmentFirst := `
		fragment InvestigationID on Query {
			firstInvestigation: investigation {
				subject {
					id
				}
			}
		}

		query {
			...InvestigationID
			firstInvestigation: investigation {
				subject {
					name
				}
			}
		}
	`

	directFirstSchema := buildSchema(
		t,
		definitionInput,
		operationDirectFirst,
		[]string{"firstInvestigation"},
	)
	fragmentFirstSchema := buildSchema(
		t,
		definitionInput,
		operationFragmentFirst,
		[]string{"firstInvestigation"},
	)
	require.Equal(t, directFirstSchema, fragmentFirstSchema)

	for _, schema := range []json.RawMessage{directFirstSchema, fragmentFirstSchema} {
		requireSchemaValidation(
			t,
			schema,
			[]string{`{"subject":{"id":"subject-1","name":"Ada"}}`},
			[]string{
				`{"subject":{"id":"subject-1"}}`,
				`{"subject":{"name":"Ada"}}`,
				`{"subject":{"id":"subject-1","name":"Ada","extra":true}}`,
			},
		)
	}
}

func TestBuildResponseSchema_FragmentsAndConditionals(t *testing.T) {
	definitionInput := `
		type Query {
			investigation: Investigation!
		}

		type Investigation {
			id: ID!
			title: String!
			note: String
		}
	`

	t.Run("named and inline fragments", func(t *testing.T) {
		schema := buildSchema(
			t,
			definitionInput,
			`query {
				investigation {
					id
					...InvestigationTitle
					... on Investigation {
						inlineNote: note
					}
				}
			}

			fragment InvestigationTitle on Investigation {
				namedTitle: title
			}`,
			[]string{"investigation"},
		)

		requireSchemaValidation(
			t,
			schema,
			[]string{`{"id":"inv-1","inlineNote":null,"namedTitle":"Open"}`},
			[]string{
				`{"id":"inv-1","namedTitle":"Open"}`,
				`{"id":"inv-1","inlineNote":null}`,
				`{"id":"inv-1","inlineNote":null,"namedTitle":"Open","extra":true}`,
			},
		)
	})

	t.Run("literal conditions are folded", func(t *testing.T) {
		schema := buildSchema(
			t,
			definitionInput,
			`query {
				investigation {
					id
					included: title @include(if: true)
					skippedFalse: note @skip(if: false)
					excludedInclude: title @include(if: false)
					excludedSkip: note @skip(if: true)
					...IncludedSpread @include(if: true)
					...ExcludedSpread @skip(if: true)
					... on Investigation @skip(if: false) {
						includedInline: note
					}
					... on Investigation @include(if: false) {
						excludedInline: title
					}
				}
			}

			fragment IncludedSpread on Investigation {
				includedSpread: title
			}

			fragment ExcludedSpread on Investigation {
				excludedSpread: title
			}`,
			[]string{"investigation"},
		)

		var decoded struct {
			Properties map[string]json.RawMessage `json:"properties"`
			Required   []string                   `json:"required"`
		}
		require.NoError(t, json.Unmarshal(schema, &decoded))
		require.ElementsMatch(t, []string{"id", "included", "skippedFalse", "includedSpread", "includedInline"}, mapKeys(decoded.Properties))
		require.Equal(t, []string{"id", "included", "includedInline", "includedSpread", "skippedFalse"}, decoded.Required)
	})

	t.Run("variable conditions make fields optional", func(t *testing.T) {
		schema := buildSchema(
			t,
			definitionInput,
			`query Investigation($includeTitle: Boolean!, $skipNote: Boolean!, $includeSpread: Boolean!, $skipInline: Boolean!) {
				investigation {
					id
					title @include(if: $includeTitle)
					note @skip(if: $skipNote)
					...OptionalSpread @include(if: $includeSpread)
					... on Investigation @skip(if: $skipInline) {
						inlineTitle: title
					}
				}
			}

			fragment OptionalSpread on Investigation {
				spreadNote: note
			}`,
			[]string{"investigation"},
		)

		var decoded struct {
			Properties map[string]json.RawMessage `json:"properties"`
			Required   []string                   `json:"required"`
		}
		require.NoError(t, json.Unmarshal(schema, &decoded))
		require.ElementsMatch(t, []string{"id", "title", "note", "spreadNote", "inlineTitle"}, mapKeys(decoded.Properties))
		require.Equal(t, []string{"id"}, decoded.Required)

		requireSchemaValidation(
			t,
			schema,
			[]string{
				`{"id":"inv-1"}`,
				`{"id":"inv-1","title":"Open","note":null,"spreadNote":"Later","inlineTitle":"Open"}`,
			},
			[]string{
				`{}`,
				`{"id":"inv-1","unexpected":true}`,
			},
		)
	})
}

func TestBuildResponseSchema_EntityResponsePath(t *testing.T) {
	schema := buildSchema(
		t,
		`scalar _Any

		type Query {
			_entities(representations: [_Any!]!): [_Entity]!
		}

		union _Entity = Product

		type Product {
			upc: ID!
			generatedResult: GeneratedResult!
		}

		type GeneratedResult {
			id: ID!
			name: String!
		}`,
		`query EntityPlan($representations: [_Any!]!) {
			_entities(representations: $representations) {
				... on Product {
					__typename
					upc
					resultAlias: generatedResult {
						__typename
						id
						name
					}
				}
			}
		}`,
		[]string{"_entities", "resultAlias"},
	)

	var decoded struct {
		Properties map[string]json.RawMessage `json:"properties"`
		Required   []string                   `json:"required"`
	}
	require.NoError(t, json.Unmarshal(schema, &decoded))
	require.ElementsMatch(t, []string{"__typename", "id", "name"}, mapKeys(decoded.Properties))
	require.Equal(t, []string{"__typename", "id", "name"}, decoded.Required)

	requireSchemaValidation(
		t,
		schema,
		[]string{`{"__typename":"GeneratedResult","id":"result-1","name":"Ada"}`},
		[]string{
			`[{"__typename":"GeneratedResult","id":"result-1","name":"Ada"}]`,
			`{"__typename":"Product","upc":"product-1","resultAlias":{"__typename":"GeneratedResult","id":"result-1","name":"Ada"}}`,
			`{"__typename":"GeneratedResult","id":"result-1","name":"Ada","upc":"product-1"}`,
		},
	)
}

func TestBuildResponseSchema_RecursiveTypeUsesFiniteSelection(t *testing.T) {
	schema := buildSchema(
		t,
		`type Query {
			node: Node!
		}

		type Node {
			id: ID!
			next: Node
		}`,
		`query {
			node {
				id
				next {
					id
					next {
						id
					}
				}
			}
		}`,
		[]string{"node"},
	)

	require.NotContains(t, string(schema), `"$ref"`)
	require.NotContains(t, string(schema), `"$defs"`)
	requireSchemaValidation(
		t,
		schema,
		[]string{
			`{"id":"node-1","next":null}`,
			`{"id":"node-1","next":{"id":"node-2","next":{"id":"node-3"}}}`,
		},
		[]string{
			`{"id":"node-1"}`,
			`{"id":"node-1","next":{"id":"node-2"}}`,
			`{"id":"node-1","next":{"id":"node-2","next":{"id":"node-3","next":null}}}`,
		},
	)
}

func TestBuildResponseSchema_RepeatedAliasDomains(t *testing.T) {
	definitionInput := `
		type Query {
			result: SearchResult!
		}

		interface SearchResult {
			id: ID!
		}

		type User implements SearchResult {
			id: ID!
			name: String!
		}

		type Product implements SearchResult {
			id: ID!
			rank: Int!
		}
	`

	t.Run("mutually exclusive concrete domains may use different fields", func(t *testing.T) {
		schema := buildSchema(
			t,
			definitionInput,
			`query {
				result {
					... on User {
						value: name
					}
					... on Product {
						value: rank
					}
				}
			}`,
			[]string{"result", "value"},
		)
		requireSchemaValidation(t, schema, []string{`"Ada"`, `7`}, []string{`null`, `true`, `{}`})
	})

	t.Run("overlapping concrete domains reject incompatible fields", func(t *testing.T) {
		definition := parseDocument(t, definitionInput)
		operation := parseDocument(t, `query {
			result {
				... on SearchResult {
					value: id
				}
				... on User {
					value: name
				}
			}
		}`)

		_, err := Build(&operation, &definition, []string{"result", "value"})
		require.ErrorContains(t, err, `response field "value"`)
		require.ErrorContains(t, err, `overlapping runtime types`)
	})
}

func TestBuildResponseSchema_ErrorContext(t *testing.T) {
	t.Run("unresolved selected field", func(t *testing.T) {
		definition := parseDocument(t, `
			type Query { investigation: Investigation! }
			type Investigation { title: String! }
		`)
		operation := parseDocument(t, `query { investigation { missing } }`)

		_, err := Build(&operation, &definition, []string{"investigation"})
		require.ErrorContains(t, err, `response path "investigation"`)
		require.ErrorContains(t, err, `field "missing" is not defined on object type "Investigation"`)
	})

	t.Run("invalid fragment type condition", func(t *testing.T) {
		definition := parseDocument(t, `
			type Query { investigation: Investigation! }
			type Investigation { title: String! }
		`)
		operation := parseDocument(t, `query {
			investigation {
				... on MissingInvestigation { title }
			}
		}`)

		_, err := Build(&operation, &definition, []string{"investigation"})
		require.ErrorContains(t, err, `response path "investigation"`)
		require.ErrorContains(t, err, `fragment type condition "MissingInvestigation" is not defined`)
	})

	t.Run("invalid fragment directive condition", func(t *testing.T) {
		definition := parseDocument(t, `
			type Query { investigation: Investigation! }
			type Investigation { title: String! }
		`)
		operation := parseDocument(t, `query {
			investigation {
				... on Investigation @include(if: "yes") { title }
			}
		}`)

		_, err := Build(&operation, &definition, []string{"investigation"})
		require.ErrorContains(t, err, `response path "investigation"`)
		require.ErrorContains(t, err, `inline fragment on "Investigation"`)
		require.ErrorContains(t, err, `@include directive has invalid if condition kind "ValueKindString"`)
	})

	t.Run("overlapping incompatible aliases", func(t *testing.T) {
		definition := parseDocument(t, `
			type Query { result: SearchResult! }
			interface SearchResult { id: ID! }
			type User implements SearchResult { id: ID!, name: String! }
			type Product implements SearchResult { id: ID! }
		`)
		operation := parseDocument(t, `query {
			result {
				... on SearchResult { value: id }
				... on User { value: name }
			}
		}`)

		_, err := Build(&operation, &definition, []string{"result", "value"})
		require.ErrorContains(t, err, `response path "result.value"`)
		require.ErrorContains(t, err, `response field "value" combines incompatible fields "id" and "name"`)
		require.ErrorContains(t, err, `overlapping runtime types ["User"]`)
	})

	t.Run("malformed custom scalar mapping name", func(t *testing.T) {
		definition := parseDocument(t, `scalar DateTime
			type Query { value: DateTime }
		`)
		operation := parseDocument(t, `query { value }`)

		_, err := Build(
			&operation,
			&definition,
			[]string{"value"},
			WithCustomScalarMappings(map[string]json.RawMessage{
				"MissingScalar": json.RawMessage(`{"type":"string"}`),
			}),
		)
		require.ErrorContains(t, err, `custom scalar mapping "MissingScalar"`)
		require.ErrorContains(t, err, `does not name a defined custom scalar`)
	})

	t.Run("unreachable response path", func(t *testing.T) {
		definition := parseDocument(t, `
			type Query { investigation: Investigation! }
			type Investigation { title: String! }
		`)
		operation := parseDocument(t, `query {
			investigation {
				hidden: title @include(if: false)
			}
		}`)

		_, err := Build(&operation, &definition, []string{"investigation", "hidden"})
		require.ErrorContains(t, err, `response path "investigation.hidden"`)
		require.ErrorContains(t, err, `response field "hidden" not found at path "investigation.hidden"`)
	})
}

func TestBuildResponseSchema_ConcurrentReadOnlyDocuments(t *testing.T) {
	definition := parseDocument(t, `
		directive @inaccessible on OBJECT
		scalar DateTime

		type Query { nodes: [Node!]! }

		interface Node {
			id: ID!
			updatedAt: DateTime!
		}

		type Product implements Node {
			id: ID!
			updatedAt: DateTime!
			sku: String!
		}

		type User implements Node {
			id: ID!
			updatedAt: DateTime!
			username: String!
		}

		type InternalNode implements Node @inaccessible {
			id: ID!
			updatedAt: DateTime!
		}
	`)
	operation := parseDocument(t, `
		query Nodes($includeUpdatedAt: Boolean!) {
			nodes {
				__typename
				id
				updatedAt @include(if: $includeUpdatedAt)
				... on Product { sku }
				...UserFields
			}
		}

		fragment UserFields on User { username }
	`)
	fieldPath := []string{"nodes"}
	option := WithCustomScalarMappings(map[string]json.RawMessage{
		"DateTime": json.RawMessage(`{"type":"string","pattern":"^dt:"}`),
	})

	expected, err := Build(&operation, &definition, fieldPath, option)
	require.NoError(t, err)

	for index := 0; index < 32; index++ {
		t.Run(fmt.Sprintf("build-%02d", index), func(t *testing.T) {
			t.Parallel()

			actual, err := Build(&operation, &definition, fieldPath, option)
			require.NoError(t, err)
			require.Equal(t, expected, actual)
		})
	}
}

const investigationSchema = `
	type Query {
		investigation: Investigation!
	}

	type Investigation {
		id: ID!
		title: String!
		note: String
		unselected: String
	}
`

func mapKeys(input map[string]json.RawMessage) []string {
	keys := make([]string, 0, len(input))
	for key := range input {
		keys = append(keys, key)
	}
	return keys
}
