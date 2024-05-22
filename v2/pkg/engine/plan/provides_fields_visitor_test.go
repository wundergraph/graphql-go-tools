package plan

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/astprinter"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/internal/unsafeparser"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/operationreport"
)

func TestProvidesSuggestions(t *testing.T) {
	keySDL := `name info {age} address {street zip}`
	definitionSDL := `
		type Query {
			me: User! @provides(fields: "name info {age} address {street zip}")
		}

		type User {
			name: String!
			info: Info!
			address: Address!
		}

		type Info {
			age: Int!
		}

		type Address {
			street: String!
			zip: String!
		}`

	fieldSet, report := RequiredFieldsFragment("User", keySDL, false)
	assert.False(t, report.HasErrors())
	definition := unsafeparser.ParseGraphqlDocumentStringWithBaseSchema(definitionSDL)

	cases := []struct {
		operation       string
		selectionSetRef int
		expected        []*NodeSuggestion
	}{
		{
			operation: `query {
				me { # selection set ref 1
					info {
						age
					}
				}
			}`,
			selectionSetRef: 1,
			expected: []*NodeSuggestion{
				{
					fieldRef:       1,
					TypeName:       "User",
					FieldName:      "info",
					DataSourceHash: 2023,
					Path:           "query.me.info",
					ParentPath:     "query.me",
					Selected:       true,
					IsProvided:     true,
				},
				{
					fieldRef:       0,
					TypeName:       "Info",
					FieldName:      "age",
					DataSourceHash: 2023,
					Path:           "query.me.info.age",
					ParentPath:     "query.me.info",
					Selected:       true,
					IsProvided:     true,
				},
			},
		},
		{
			operation: `query {
				me {
					name
					info {
						age
					}
				}
			}`,
			selectionSetRef: 1,
			expected: []*NodeSuggestion{
				{
					fieldRef:       0,
					TypeName:       "User",
					FieldName:      "name",
					DataSourceHash: 2023,
					Path:           "query.me.name",
					ParentPath:     "query.me",
					Selected:       true,
					IsProvided:     true,
				},
				{
					fieldRef:       2,
					TypeName:       "User",
					FieldName:      "info",
					DataSourceHash: 2023,
					Path:           "query.me.info",
					ParentPath:     "query.me",
					Selected:       true,
					IsProvided:     true,
				},
				{
					fieldRef:       1,
					TypeName:       "Info",
					FieldName:      "age",
					DataSourceHash: 2023,
					Path:           "query.me.info.age",
					ParentPath:     "query.me.info",
					Selected:       true,
					IsProvided:     true,
				},
			},
		},
		{
			operation: `query {
				me {
					address {
						street
					}
				}
			}`,
			selectionSetRef: 1,
			expected: []*NodeSuggestion{
				{
					fieldRef:       1,
					TypeName:       "User",
					FieldName:      "address",
					DataSourceHash: 2023,
					Path:           "query.me.address",
					ParentPath:     "query.me",
					Selected:       true,
					IsProvided:     true,
				},
				{
					fieldRef:       0,
					TypeName:       "Address",
					FieldName:      "street",
					DataSourceHash: 2023,
					Path:           "query.me.address.street",
					ParentPath:     "query.me.address",
					Selected:       true,
					IsProvided:     true,
				},
			},
		},
		{
			operation: `query {
				me { # selection set ref 2
					name
					info { # selection set ref 0
						age
					}
					address { # selection set ref 1
						street
						zip
					}
				}
			}`,
			selectionSetRef: 2,
			expected: []*NodeSuggestion{
				{
					fieldRef:       0,
					TypeName:       "User",
					FieldName:      "name",
					DataSourceHash: 2023,
					Path:           "query.me.name",
					ParentPath:     "query.me",
					Selected:       true,
					IsProvided:     true,
				},
				{
					fieldRef:       2,
					TypeName:       "User",
					FieldName:      "info",
					DataSourceHash: 2023,
					Path:           "query.me.info",
					ParentPath:     "query.me",
					Selected:       true,
					IsProvided:     true,
				},
				{
					fieldRef:       1,
					TypeName:       "Info",
					FieldName:      "age",
					DataSourceHash: 2023,
					Path:           "query.me.info.age",
					ParentPath:     "query.me.info",
					Selected:       true,
					IsProvided:     true,
				},
				{
					fieldRef:       5,
					TypeName:       "User",
					FieldName:      "address",
					DataSourceHash: 2023,
					Path:           "query.me.address",
					ParentPath:     "query.me",
					Selected:       true,
					IsProvided:     true,
				},
				{
					fieldRef:       3,
					TypeName:       "Address",
					FieldName:      "street",
					DataSourceHash: 2023,
					Path:           "query.me.address.street",
					ParentPath:     "query.me.address",
					Selected:       true,
					IsProvided:     true,
				},
				{
					fieldRef:       4,
					TypeName:       "Address",
					FieldName:      "zip",
					DataSourceHash: 2023,
					Path:           "query.me.address.zip",
					ParentPath:     "query.me.address",
					Selected:       true,
					IsProvided:     true,
				},
			},
		},
	}

	for _, c := range cases {
		t.Run(c.operation, func(t *testing.T) {
			operation := unsafeparser.ParseGraphqlDocumentString(c.operation)
			p, _ := astprinter.PrintStringIndentDebug(&operation, nil, "  ")
			fmt.Println(p)
			report := &operationreport.Report{}

			input := &providesInput{
				operationSelectionSet: c.selectionSetRef,
				providesFieldSet:      fieldSet,
				operation:             &operation,
				definition:            &definition,
				report:                report,
				parentPath:            "query.me",
				DSHash:                2023,
			}

			suggestions := providesSuggestions(input)
			assert.False(t, report.HasErrors())
			assert.Equal(t, c.expected, suggestions)
		})
	}
}
