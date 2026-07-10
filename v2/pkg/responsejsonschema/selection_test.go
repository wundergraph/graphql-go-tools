package responsejsonschema

import (
	"encoding/json"
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
