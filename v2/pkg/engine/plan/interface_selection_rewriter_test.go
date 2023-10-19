package plan

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/v2/internal/pkg/unsafeparser"
	"github.com/wundergraph/graphql-go-tools/v2/internal/pkg/unsafeprinter"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
)

func TestInterfaceSelectionRewriter_RewriteOperation(t *testing.T) {
	run := func(t *testing.T, definition string, dsConfiguration *DataSourceConfiguration, operation string, expectedOperation string, shouldRewrite bool, enclosingTypeName string, fieldName string) {
		t.Helper()

		op := unsafeparser.ParseGraphqlDocumentString(operation)
		def := unsafeparser.ParseGraphqlDocumentStringWithBaseSchema(definition)

		if fieldName == "" {
			fieldName = "iface"
		}
		if enclosingTypeName == "" {
			enclosingTypeName = "Query"
		}

		fieldRef := ast.InvalidRef
		for ref := range op.Fields {
			if op.FieldNameString(ref) == fieldName {
				fieldRef = ref
				break
			}
		}

		node, _ := def.Index.FirstNodeByNameStr(enclosingTypeName)

		rewriter := newFieldSelectionRewriter(&op, &def)
		rewritten, err := rewriter.RewriteFieldSelection(fieldRef, node, dsConfiguration)
		require.NoError(t, err)
		assert.Equal(t, shouldRewrite, rewritten)

		printedOp := unsafeprinter.PrettyPrint(&op, &def)
		expectedPretty := unsafeprinter.Prettify(expectedOperation)

		assert.Equal(t, expectedPretty, printedOp)
	}

	type testCase struct {
		name              string
		definition        string
		dsConfiguration   *DataSourceConfiguration
		operation         string
		expectedOperation string
		enclosingTypeName string
		fieldName         string
		shouldRewrite     bool
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
			shouldRewrite: true,
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
			shouldRewrite: true,
		},
		{
			name:       "no shared fields. query has user fragment",
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
						... on User {
							isUser
						}
					}
				}`,

			expectedOperation: `
				query {
					iface {
						... on User {
							isUser
						}
					}
				}`,
			shouldRewrite: false,
		},
		{
			name:       "only __typename as a shared field. query has user fragment",
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
						__typename
						... on User {
							isUser
						}
					}
				}`,

			expectedOperation: `
				query {
					iface {
						__typename
						... on User {
							isUser
						}
					}
				}`,
			shouldRewrite: false,
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
			shouldRewrite: true,
		},
		{
			name:       "one field is external. query has admin and user fragment and shared __typename",
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
						__typename
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
						__typename
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
			shouldRewrite: true,
		},
		{
			name:       "one field is external. query has admin and user fragment, user fragment has shared field",
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
							name
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
			shouldRewrite: true,
		},
		{
			name:       "one field is external. query has admin and user fragment, all fragments has shared field",
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
							name
						}
						... on Admin {
							id
							name
						}
					}
				}`,
			expectedOperation: `
				query {
					iface {
						name
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
			shouldRewrite: false,
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
			shouldRewrite: false,
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
			shouldRewrite: false,
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
			shouldRewrite: false,
		},
		{
			name: "field is union",
			definition: `
				union Node = User | Admin
				
				type User {
					id: ID!
					name: String!
					isUser: Boolean!
				}
		
				type Admin {
					id: ID!
					name: String!
				}
				
				type Query {
					iface: Node!
				}`,
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
						... on User {
							isUser
						}
						... on Admin {
							name
						}
					}
				}`,
			expectedOperation: `
				query {
					iface {
						... on User {
							isUser
						}
						... on Admin {
							name
						}
					}
				}`,
			shouldRewrite: false,
		},
		{
			name: "field is a type",
			definition: `
				type User {
					id: ID!
					name: String!
					isUser: Boolean!
				}
				
				type Query {
					iface: User!
				}`,
			dsConfiguration: dsb().
				RootNode("Query", "iface").
				ChildNode("User", "id", "name", "isUser").
				KeysMetadata(FederationFieldConfigurations{
					{
						TypeName:     "User",
						SelectionSet: "id",
					},
				}).
				DSPtr(),
			operation: `
				query {
					iface {
						isUser
					}
				}`,
			expectedOperation: `
				query {
					iface {
						isUser
					}
				}`,
			shouldRewrite: false,
		},
		{
			name:              "interface nesting. check container field",
			enclosingTypeName: "Query",
			fieldName:         "container",
			definition: `
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
				
				type Container implements ContainerInterface {
					iface: Node!
				}

				interface ContainerInterface {
					iface: Node!
				}

				type Query {
					container: ContainerInterface!
				}`,
			dsConfiguration: dsb().
				RootNode("Query", "container").
				ChildNode("Container", "iface").
				ChildNode("ContainerInterface", "iface").
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
					container {
						iface {
							name
						}
					}
				}`,
			expectedOperation: `
				query {
					container {
						iface {
							name
						}
					}
				}`,
			shouldRewrite: false,
		},
		{
			name:              "interface nesting. check nested iface field",
			enclosingTypeName: "ContainerInterface",
			fieldName:         "node",
			definition: `
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
				
				type Container implements ContainerInterface {
					node: Node!
				}

				interface ContainerInterface {
					node: Node!
				}

				type Query {
					container: ContainerInterface!
				}`,
			dsConfiguration: dsb().
				RootNode("Query", "container").
				ChildNode("Container", "node").
				ChildNode("ContainerInterface", "node").
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
					container {
						node {
							name
						}
					}
				}`,
			expectedOperation: `
				query {
					container {
						node {
							... on User {
								name
							}
							... on Admin {
								name
							}
						}
					}
				}`,
			shouldRewrite: true,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			run(t, testCase.definition, testCase.dsConfiguration, testCase.operation, testCase.expectedOperation, testCase.shouldRewrite, testCase.enclosingTypeName, testCase.fieldName)
		})
	}
}
