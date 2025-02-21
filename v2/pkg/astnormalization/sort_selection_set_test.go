package astnormalization

import (
	"github.com/wundergraph/graphql-go-tools/v2/pkg/asttransform"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/astparser"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astprinter"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/operationreport"
)

func TestSortSelectionSets(t *testing.T) {
	schema := `
		type Query {
			findEmployees(criteria: Criteria!): [Employee!]!
			user: User
		}

		type Employee {
			id: ID!
			details: Details!
		}

		type Details {
			forename: String!
			surname: String!
		}

		type User {
			id: ID!
			email: String!
		}

		input Criteria {
			nationality: String!
		}
	`

	testCases := []struct {
		name   string
		input  string
		output string
	}{
		{
			name: "sorts basic fields alphabetically",
			input: `
				query MyQuery {
					findEmployees(criteria: { nationality: AMERICAN }) {
						id
						details {
							surname
							forename
						}
					}
				}`,
			output: `
				query MyQuery {
					findEmployees(criteria: {nationality: AMERICAN}) {
						details {
							forename
							surname
						}
						id
					}
				}`,
		},
		{
			name: "sorts fields with aliases and nested selections",
			input: `
				query MyQuery {
					user {
						id
						email
						... on User {
							details: id
							email
						}
					}
				}`,
			output: `
				query MyQuery {
					user {
						details: id
						email
						id
						... on User {
							details: id
							email
						}
					}
				}`,
		},
		{
			name: "sorts fragment spreads and inline fragments",
			input: `
				query MyQuery {
					user {
						...UserFragment
						id
						... on User {
							email
						}
					}
				}

				fragment UserFragment on User {
					id
					email
				}`,

			output: `
				query MyQuery {
					user {
						email
						...UserFragment
						id
    					}
				}

				fragment UserFragment on User {
					email
					id
				}`,
		},
	}

	schemaDoc, _ := astparser.ParseGraphqlDocumentString(schema)
	require.NotNil(t, schemaDoc, "schemaDoc should not be nil")
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			operationDoc, _ := astparser.ParseGraphqlDocumentString(tc.input)
			require.NotNil(t, operationDoc, "operationDoc should not be nil")

			err := asttransform.MergeDefinitionWithBaseSchema(&schemaDoc)
			assert.NoError(t, err)

			normalizer := NewWithOpts(
				WithSortSelectionSets(),
			)

			report := &operationreport.Report{}
			normalizer.NormalizeOperation(&operationDoc, &schemaDoc, report)
			require.False(t, report.HasErrors(), report.Error())

			output, err := astprinter.PrintStringIndent(&operationDoc, "  ")
			require.NoError(t, err)

			expectedOutput, _ := astparser.ParseGraphqlDocumentString(tc.output)
			expectedPrinted, err := astprinter.PrintStringIndent(&expectedOutput, "  ")
			require.NoError(t, err)

			assert.Equal(t, expectedPrinted, output)
		})
	}
}
