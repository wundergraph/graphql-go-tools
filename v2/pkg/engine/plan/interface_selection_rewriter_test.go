package plan

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/v2/internal/pkg/unsafeparser"
	"github.com/wundergraph/graphql-go-tools/v2/internal/pkg/unsafeprinter"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
)

func TestInterfaceSelectionRewriter_RewriteOperation(t *testing.T) {
	run := func(t *testing.T, definition string, dsConfiguration *DataSourceConfiguration, operation string, expectedOperation string, expectedRewritten bool) {
		t.Helper()

		op := unsafeparser.ParseGraphqlDocumentString(operation)
		def := unsafeparser.ParseGraphqlDocumentStringWithBaseSchema(definition)

		fieldRef := ast.InvalidRef
		for ref := range op.Fields {
			if op.FieldNameString(ref) == "iface" {
				fieldRef = ref
				break
			}
		}

		node, _ := def.Index.FirstNodeByNameStr("Query")

		rewriter := newInterfaceSelectionRewriter(&op, &def)
		rewrittern, err := rewriter.RewriteOperation(fieldRef, node, dsConfiguration)
		require.NoError(t, err)
		require.Equal(t, expectedRewritten, rewrittern)

		printedOp := unsafeprinter.PrettyPrint(&op, &def)
		expectedPretty := unsafeprinter.Prettify(expectedOperation)

		require.Equal(t, expectedPretty, printedOp)
	}

	type testCase struct {
		name              string
		definition        string
		dsConfiguration   *DataSourceConfiguration
		operation         string
		expectedOperation string
		expectedRewritten bool
	}

	definition := `
		interface Node {
			id: ID!
			name: String!
		}
		
		type User implements Node {
			id: ID!
			name: String!
			isUser: Boolean!
		}

		type Admin implements Node {
			id: ID!
			name: String!
		}
		
		type Query {
			iface: Node!
		}
	`

	testCases := []testCase{
		{
			name:       "one field is external. query without fragments",
			definition: definition,
			dsConfiguration: dsb().
				RootNode("Query", "iface").
				RootNode("User", "id", "name", "isUser").
				RootNode("Admin", "id").
				KeysMetadata(FederationFieldConfigurations{
					{
						TypeName:     "User",
						SelectionSet: "id",
					},
					{
						TypeName:     "Admin",
						SelectionSet: "id",
					},
				}).
				DSPtr(),
			operation: `
				query {
					iface {
						name
					}
				}`,

			expectedOperation: `
				query {
					iface {
						... on User {
							name
						}
						... on Admin {
							name
						}
					}
				}`,
			expectedRewritten: true,
		},
		{
			name:       "one field is external. query has user fragment",
			definition: definition,
			dsConfiguration: dsb().
				RootNode("Query", "iface").
				RootNode("User", "id", "name", "isUser").
				RootNode("Admin", "id").
				KeysMetadata(FederationFieldConfigurations{
					{
						TypeName:     "User",
						SelectionSet: "id",
					},
					{
						TypeName:     "Admin",
						SelectionSet: "id",
					},
				}).
				DSPtr(),
			operation: `
				query {
					iface {
						name
						... on User {
							isUser
						}
					}
				}`,

			expectedOperation: `
				query {
					iface {
						... on Admin {
							name
						}
						... on User {
							isUser
							name
						}
					}
				}`,
			expectedRewritten: true,
		},
		{
			name:       "one field is external. query has admin and user fragment",
			definition: definition,
			dsConfiguration: dsb().
				RootNode("Query", "iface").
				RootNode("User", "id", "isUser").
				RootNode("Admin", "id").
				KeysMetadata(FederationFieldConfigurations{
					{
						TypeName:     "User",
						SelectionSet: "id",
					},
					{
						TypeName:     "Admin",
						SelectionSet: "id",
					},
				}).
				DSPtr(),
			operation: `
				query {
					iface {
						name
						... on User {
							isUser
						}
						... on Admin {
							id
						}
					}
				}`,

			expectedOperation: `
				query {
					iface {
						... on User {
							isUser
							name
						}
						... on Admin {
							id
							name
						}
					}
				}`,
			expectedRewritten: true,
		},
		{
			name:       "all fields local. query without fragments",
			definition: definition,
			dsConfiguration: dsb().
				RootNode("Query", "iface").
				RootNode("User", "id", "name").
				RootNode("Admin", "id", "name").
				KeysMetadata(FederationFieldConfigurations{
					{
						TypeName:     "User",
						SelectionSet: "id",
					},
					{
						TypeName:     "Admin",
						SelectionSet: "id",
					},
				}).
				DSPtr(),
			operation: `
				query {
					iface {
						name
					}
				}`,

			expectedOperation: `
				query {
					iface {
						name
					}
				}`,
			expectedRewritten: false,
		},
		{
			name:       "all fields local. query has user fragment",
			definition: definition,
			dsConfiguration: dsb().
				RootNode("Query", "iface").
				RootNode("User", "id", "name", "isUser").
				RootNode("Admin", "id", "name").
				KeysMetadata(FederationFieldConfigurations{
					{
						TypeName:     "User",
						SelectionSet: "id",
					},
					{
						TypeName:     "Admin",
						SelectionSet: "id",
					},
				}).
				DSPtr(),
			operation: `
				query {
					iface {
						name
						... on User {
							isUser
						}
					}
				}`,

			expectedOperation: `
				query {
					iface {
						name
						... on User {
							isUser
						}
					}
				}`,
			expectedRewritten: false,
		},
		{
			name:       "all fields local. query without fragment. types are not entities",
			definition: definition,
			dsConfiguration: dsb().
				RootNode("Query", "iface").
				ChildNode("User", "id", "name", "isUser").
				ChildNode("Admin", "id", "name").
				DSPtr(),
			operation: `
				query {
					iface {
						name
						... on User {
							isUser
						}
					}
				}`,

			expectedOperation: `
				query {
					iface {
						name
						... on User {
							isUser
						}
					}
				}`,
			expectedRewritten: false,
		},

		// field is not an interface
		// fragment already had a shared field
		// all fragments already had shared fields
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			run(t, testCase.definition, testCase.dsConfiguration, testCase.operation, testCase.expectedOperation, testCase.expectedRewritten)
		})
	}
}
