package plan

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/internal/unsafeparser"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/internal/unsafeprinter"
)

func TestInterfaceSelectionRewriter_RewriteOperation(t *testing.T) {
	type testCase struct {
		name               string
		definition         string
		upstreamDefinition string
		dsConfiguration    DataSource
		operation          string
		expectedOperation  string
		enclosingTypeName  string // default is "Query"
		fieldName          string // default is "iface"
		shouldRewrite      bool
	}

	run := func(t *testing.T, testCase testCase) {
		t.Helper()

		op := unsafeparser.ParseGraphqlDocumentString(testCase.operation)
		def := unsafeparser.ParseGraphqlDocumentStringWithBaseSchema(testCase.definition)

		upstreamDef := unsafeparser.ParseGraphqlDocumentStringWithBaseSchema(testCase.upstreamDefinition)

		if testCase.fieldName == "" {
			testCase.fieldName = "iface"
		}
		if testCase.enclosingTypeName == "" {
			testCase.enclosingTypeName = "Query"
		}

		fieldRef := ast.InvalidRef
		for ref := range op.Fields {
			if op.FieldNameString(ref) == testCase.fieldName {
				fieldRef = ref
				break
			}
		}

		node, _ := def.Index.FirstNodeByNameStr(testCase.enclosingTypeName)

		rewriter := newFieldSelectionRewriter(&op, &def)
		rewriter.SetUpstreamDefinition(&upstreamDef)
		rewriter.SetDatasourceConfiguration(testCase.dsConfiguration)

		rewritten, err := rewriter.RewriteFieldSelection(fieldRef, node)
		require.NoError(t, err)
		assert.Equal(t, testCase.shouldRewrite, rewritten)

		printedOp := unsafeprinter.PrettyPrint(&op)
		expectedPretty := unsafeprinter.Prettify(testCase.expectedOperation)

		assert.Equal(t, expectedPretty, printedOp)
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

		type ImplementsNodeNotInUnion implements Node {
			id: ID!
			name: String!
		}

		type Moderator implements Node {
			id: ID!
			name: String!
			isModerator: Boolean!
		}
		
		union Account = User | Admin | Moderator

		type Query {
			iface: Node!
			accounts: [Account!]!
		}
	`

	testCases := []testCase{
		{
			name:       "one field is external. query without fragments",
			definition: definition,
			upstreamDefinition: `
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
			`,
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
				DS(),
			operation: `
				query {
					iface {
						name
					}
				}`,

			expectedOperation: `
				query {
					iface {
						... on Admin {
							name
						}
						... on User {
							name
						}
					}
				}`,
			shouldRewrite: true,
		},
		{
			name: "admin name is external, moderator is from other datasource - should not have moderator fragment",
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

				type Moderator implements Node {
					id: ID!
					name: String!
				}
				
				type Query {
					iface: Node!
				}
			`,
			upstreamDefinition: `
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
				}
				
				type Query {
					iface: Node!
				}
			`,
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
				DS(),
			operation: `
				query {
					iface {
						name
					}
				}`,

			expectedOperation: `
				query {
					iface {
						... on Admin {
							name
						}
						... on User {
							name
						}
					}
				}`,
			shouldRewrite: true,
		},
		{
			name: "admin name is external, moderator is from other datasource - should remove moderator fragment",
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

				type Moderator implements Node {
					id: ID!
					name: String!
					isModerator: Boolean!
				}
				
				type Query {
					iface: Node!
				}
			`,
			upstreamDefinition: `
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
				}
				
				type Query {
					iface: Node!
				}
			`,
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
				DS(),
			operation: `
				query {
					iface {
						name
						... on Moderator {
							isModerator
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
							name
						}
					}
				}`,
			shouldRewrite: true,
		},
		{
			name: "has only moderator fragment which is from other datasource - should remove moderator fragment",
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

				type Moderator implements Node {
					id: ID!
					name: String!
					isModerator: Boolean!
				}
				
				type Query {
					iface: Node!
				}
			`,
			upstreamDefinition: `
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
			`,
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
				DS(),
			operation: `
				query {
					iface {
						... on Moderator {
							isModerator
						}
					}
				}`,

			expectedOperation: `
				query {
					iface {
						__typename
					}
				}`,
			shouldRewrite: true,
		},
		{
			name: "has user, admin, moderator fragments - should remove moderator as it is in schema, but not implements interface Node",
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

				type Moderator implements Node {
					id: ID!
					name: String!
					isModerator: Boolean!
				}
				
				type Query {
					iface: Node!
				}
			`,
			upstreamDefinition: `
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
				}

				# moderator is in schema, but not implements interface Node
				type Moderator { 
					id: ID!
					name: String!
					isModerator: Boolean!
				}
			`,
			dsConfiguration: dsb().
				RootNode("Query", "iface").
				RootNode("User", "id", "name", "isUser").
				RootNode("Admin", "id").
				RootNode("Moderator", "id", "name", "isModerator").
				KeysMetadata(FederationFieldConfigurations{
					{
						TypeName:     "User",
						SelectionSet: "id",
					},
					{
						TypeName:     "Admin",
						SelectionSet: "id",
					},
					{
						TypeName:     "Moderator",
						SelectionSet: "id",
					},
				}).
				DS(),
			operation: `
				query {
					iface {
						... on Admin {
							name
						}
						... on User {
							name
						}
						... on Moderator {
							name
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
							name
						}
					}
				}`,
			shouldRewrite: true,
		},
		{
			name:       "one field is external. query has user fragment",
			definition: definition,
			upstreamDefinition: `
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
				}
				
				union Account = User | Admin
		
				type Query {
					iface: Node!
					accounts: [Account!]!
				}
			`,
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
				DS(),
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
							name
							isUser
						}
					}
				}`,
			shouldRewrite: true,
		},
		{
			name:               "no shared fields. query has user fragment",
			definition:         definition,
			upstreamDefinition: definition,
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
				DS(),
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
			name:               "only __typename as a shared field. query has user fragment",
			definition:         definition,
			upstreamDefinition: definition,
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
				DS(),
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
			upstreamDefinition: `
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
				}
					
				type Query {
					iface: Node!
				}
			`,
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
				DS(),
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
						... on Admin {
							name
							id
						}
						... on User {
							name
							isUser
						}
					}
				}`,
			shouldRewrite: true,
		},
		{
			name:       "one field is external (Admin.name). query has admin and user fragment and shared __typename",
			definition: definition,
			upstreamDefinition: `
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
				}
					
				type Query {
					iface: Node!
				}
			`,
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
				DS(),
			operation: `
				query {
					iface {
						__typename
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
						... on Admin {
							__typename
							name
							id
						}
						... on User {
							__typename
							name
							isUser
						}
					}
				}`,
			shouldRewrite: true,
		},
		{
			name:       "one field is external (Admin.name). query has fragment on interface node inside union",
			fieldName:  "accounts",
			definition: definition,
			upstreamDefinition: `
				type User {
					id: ID!
					name: String!
					isUser: Boolean!
				}
		
				type Admin {
					id: ID!
				}

				union Account = User | Admin
			`,
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
				DS(),
			operation: `
				query {
					accounts {
						... on Node {
							__typename
							name
						}
					}
				}`,

			expectedOperation: `
				query {
					accounts {
						... on Admin {
							__typename
							name
						}	
						... on User {
							__typename
							name
						}
					}
				}`,
			shouldRewrite: true,
		},
		{
			name:       "one field is external (Admin.name). query has fragment on node and shared __typename on a union - should preserve __typename",
			fieldName:  "accounts",
			definition: definition,
			upstreamDefinition: `
				type User {
					id: ID!
					name: String!
					isUser: Boolean!
				}
		
				type Admin {
					id: ID!
				}

				union Account = User | Admin
			`,
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
				DS(),
			operation: `
				query {
					accounts {
						__typename
						... on Node {
							__typename
							name
						}
					}
				}`,

			expectedOperation: `
				query {
					accounts {
						__typename
						... on Admin {
							__typename
							name
						}	
						... on User {
							__typename
							name
						}
					}
				}`,
			shouldRewrite: true,
		},
		{
			name:       "one field is external. query has admin and user fragment, user fragment has shared field",
			definition: definition,
			upstreamDefinition: `
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
				}
					
				type Query {
					iface: Node!
				}
			`,
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
				DS(),
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
						... on Admin {
							name
							id
						}
						... on User {
							name
							isUser
						}
					}
				}`,
			shouldRewrite: true,
		},
		{
			name:               "one field is external. query has admin and user fragment, all fragments has shared field",
			definition:         definition,
			upstreamDefinition: definition,
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
				DS(),
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
			name:               "all fields local. query without fragments",
			definition:         definition,
			upstreamDefinition: definition,
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
				DS(),
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
			name:               "all fields local but one of the fields has requires directive. query without fragments",
			definition:         definition,
			upstreamDefinition: definition,
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
				WithMetadata(func(md *FederationMetaData) {
					md.Requires = []FederationFieldConfiguration{
						{
							TypeName:     "User",
							FieldName:    "name",
							SelectionSet: "any",
						},
					}
				}).
				DS(),
			operation: `
				query {
					iface {
						name
					}
				}`,

			expectedOperation: `
				query {
					iface {
						... on Admin {
							name
						}
						... on ImplementsNodeNotInUnion {
							name
						}
						... on Moderator {
							name
						}
						... on User {
							name
						}
					}
				}`,
			shouldRewrite: true,
		},
		{
			name:               "all fields local. query has user fragment",
			definition:         definition,
			upstreamDefinition: definition,
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
				DS(),
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
			name:               "all fields local. query with user fragment. types are not entities",
			definition:         definition,
			upstreamDefinition: definition,
			dsConfiguration: dsb().
				RootNode("Query", "iface").
				ChildNode("User", "id", "name", "isUser").
				ChildNode("Admin", "id", "name").
				DS(),
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
			name: "field is union - should not be touched we have all fragments and everything is local",
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
			upstreamDefinition: `
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
				DS(),
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
				DS(),
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
			upstreamDefinition: `
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
				DS(),
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
			upstreamDefinition: `
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
				DS(),
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
							... on Admin {
								name
							}
							... on User {
								name
							}
						}
					}
				}`,
			shouldRewrite: true,
		},
		{
			name:       "Union with interface fragment: no entity fragments, all fields local",
			definition: definition,
			upstreamDefinition: `
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
			
				type Moderator implements Node {
					id: ID!
					name: String!
					isModerator: Boolean!
				}
				
				union Account = User | Admin | Moderator
		
				type Query {
					iface: Node!
					accounts: [Account!]!
				}
			`,
			dsConfiguration: dsb().
				RootNode("Query", "iface", "accounts").
				RootNode("User", "id", "name", "isUser").
				RootNode("Admin", "id", "name").
				RootNode("Moderator", "id", "name", "isModerator").
				ChildNode("Node", "id", "name").
				KeysMetadata(FederationFieldConfigurations{
					{
						TypeName:     "User",
						SelectionSet: "id",
					},
					{
						TypeName:     "Admin",
						SelectionSet: "id",
					},
					{
						TypeName:     "Moderator",
						SelectionSet: "id",
					},
				}).
				DS(),
			fieldName: "accounts",
			operation: `
				query {
					accounts {
						... on Node {
							name
						}
					}
				}`,
			expectedOperation: `
				query {
					accounts {
						... on Node {
							name
						}
					}
				}`,
			shouldRewrite: false,
		},
		{
			name:       "Union with interface fragment: no entity fragments, all fields local - but one of the fields has requires",
			definition: definition,
			upstreamDefinition: `
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
					name: String! @requires(fields: "importantNote")
					importantNote: String! @external
				}
			
				type Moderator implements Node {
					id: ID!
					name: String!
					isModerator: Boolean!
				}
				
				union Account = User | Admin | Moderator
		
				type Query {
					iface: Node!
					accounts: [Account!]!
				}
			`,
			dsConfiguration: dsb().
				RootNode("Query", "iface", "accounts").
				RootNode("User", "id", "name", "isUser").
				RootNode("Admin", "id", "name").
				RootNode("Moderator", "id", "name", "isModerator").
				ChildNode("Node", "id", "name").
				KeysMetadata(FederationFieldConfigurations{
					{
						TypeName:     "User",
						SelectionSet: "id",
					},
					{
						TypeName:     "Admin",
						SelectionSet: "id",
					},
					{
						TypeName:     "Moderator",
						SelectionSet: "id",
					},
				}).
				WithMetadata(func(meta *FederationMetaData) {
					meta.Requires = []FederationFieldConfiguration{
						{
							TypeName:     "Admin",
							FieldName:    "name",
							SelectionSet: "importantNote",
						},
					}
				}).
				DS(),
			fieldName: "accounts",
			operation: `
				query {
					accounts {
						... on Node {
							name
						}
					}
				}`,
			expectedOperation: `
				query {
					accounts {
						... on Admin {
							name
						}
						... on Moderator {
							name
						}
						... on User {
							name
						}
					}
				}`,
			shouldRewrite: true,
		},
		{
			name:       "Union with interface fragment: no entity fragments, user.name is local, admin.name and moderator.name is external",
			definition: definition,
			upstreamDefinition: `
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
				}
		
				type ImplementsNodeNotInUnion implements Node {
					id: ID!
					name: String!
				}
		
				type Moderator implements Node {
					id: ID!
					isModerator: Boolean!
				}
				
				union Account = User | Admin | Moderator
		
				type Query {
					iface: Node!
					accounts: [Account!]!
				}
			`,
			dsConfiguration: dsb().
				RootNode("Query", "iface", "accounts").
				RootNode("User", "id", "name", "isUser").
				RootNode("Admin", "id").
				RootNode("Moderator", "id").
				RootNode("Node", "id", "name").
				KeysMetadata(FederationFieldConfigurations{
					{
						TypeName:     "User",
						SelectionSet: "id",
					},
					{
						TypeName:     "Admin",
						SelectionSet: "id",
					},
					{
						TypeName:     "Moderator",
						SelectionSet: "id",
					},
				}).
				DS(),
			fieldName: "accounts",
			operation: `
				query {
					accounts {
						... on Node {
							name
						}
					}
				}`,
			expectedOperation: `
				query {
					accounts {
						... on Admin {
							name
						}
						... on Moderator {
							name
						}
						... on User {
							name
						}
					}
				}`,
			shouldRewrite: true,
		},
		{
			name:       "Union with interface fragment: no entity fragments, user.name is local, admin.name is external, moderator from other datasource",
			definition: definition,
			upstreamDefinition: `
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
				}
		
				type ImplementsNodeNotInUnion implements Node {
					id: ID!
					name: String!
				}
				
				union Account = User | Admin
		
				type Query {
					iface: Node!
					accounts: [Account!]!
				}
			`,
			dsConfiguration: dsb().
				RootNode("Query", "iface", "accounts").
				RootNode("User", "id", "name", "isUser").
				RootNode("Admin", "id").
				RootNode("Node", "id", "name").
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
				DS(),
			fieldName: "accounts",
			operation: `
				query {
					accounts {
						... on Node {
							name
						}
					}
				}`,
			expectedOperation: `
				query {
					accounts {
						... on Admin {
							name
						}
						... on User {
							name
						}
					}
				}`,
			shouldRewrite: true,
		},
		{
			name:       "Union with interface fragment: user has fragment, user.name is local, admin.name is external, moderator from other datasource",
			definition: definition,
			upstreamDefinition: `
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
				}
		
				type ImplementsNodeNotInUnion implements Node {
					id: ID!
					name: String!
				}
			
				union Account = User | Admin
		
				type Query {
					iface: Node!
					accounts: [Account!]!
				}
			`,
			dsConfiguration: dsb().
				RootNode("Query", "iface", "accounts").
				RootNode("User", "id", "name", "isUser").
				RootNode("Admin", "id").
				RootNode("Node", "id", "name").
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
				DS(),
			fieldName: "accounts",
			operation: `
				query {
					accounts {
						... on Node {
							name
						}
						... on User {
							isUser
						}
					}
				}`,
			expectedOperation: `
				query {
					accounts {
						... on Admin {
							name
						}
						... on User {
							name
							isUser
						}
					}
				}`,
			shouldRewrite: true,
		},
		{
			name:       "Union with interface fragment: user has fragment, moderator has fragment, user.name is local, admin.name is external, moderator from other datasource",
			definition: definition,
			upstreamDefinition: `
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
				}
		
				type ImplementsNodeNotInUnion implements Node {
					id: ID!
					name: String!
				}
			
				union Account = User | Admin
		
				type Query {
					iface: Node!
					accounts: [Account!]!
				}
			`,
			dsConfiguration: dsb().
				RootNode("Query", "iface", "accounts").
				RootNode("User", "id", "name", "isUser").
				RootNode("Admin", "id").
				RootNode("Node", "id", "name").
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
				DS(),
			fieldName: "accounts",
			operation: `
				query {
					accounts {
						... on Node {
							name
						}
						... on Moderator {
							isModerator
						}
					}
				}`,
			expectedOperation: `
				query {
					accounts {
						... on Admin {
							name
						}
						... on User {
							name
						}
					}
				}`,
			shouldRewrite: true,
		},
		{
			name:       "Union with interface fragment: user has fragment, moderator has fragment, user.name is local, admin.name is external, moderator is not part of a union",
			definition: definition,
			upstreamDefinition: `
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
		
				type ImplementsNodeNotInUnion implements Node {
					id: ID!
					name: String!
				}
		
				type Moderator implements Node {
					id: ID!
					name: String!
					isModerator: Boolean!
				}
				
				union Account = User | Admin
		
				type Query {
					iface: Node!
					accounts: [Account!]!
				}
			`,
			dsConfiguration: dsb().
				RootNode("Query", "iface", "accounts").
				RootNode("User", "id", "name", "isUser").
				RootNode("Moderator", "id", "name", "isModerator").
				RootNode("Admin", "id").
				RootNode("Node", "id", "name").
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
				DS(),
			fieldName: "accounts",
			operation: `
				query {
					accounts {
						... on Node {
							name
						}
						... on Moderator {
							isModerator
						}
					}
				}`,
			expectedOperation: `
				query {
					accounts {
						... on Admin {
							name
						}
						... on User {
							name
						}
					}
				}`,
			shouldRewrite: true,
		},
		{
			name:       "Union with interface fragment: only moderator has fragment, user.name is local, admin.name is external, moderator from other datasource",
			definition: definition,
			upstreamDefinition: `
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
				}
		
				type ImplementsNodeNotInUnion implements Node {
					id: ID!
					name: String!
				}
			
				union Account = User | Admin
		
				type Query {
					iface: Node!
					accounts: [Account!]!
				}
			`,
			dsConfiguration: dsb().
				RootNode("Query", "iface", "accounts").
				RootNode("User", "id", "name", "isUser").
				RootNode("Admin", "id").
				RootNode("Node", "id", "name").
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
				DS(),
			fieldName: "accounts",
			operation: `
				query {
					accounts {
						... on Moderator {
							isModerator
						}
					}
				}`,
			expectedOperation: `
				query {
					accounts {
						__typename
					}
				}`,
			shouldRewrite: true,
		},
		{
			name: "interface field having only other interface fragment with fragments inside",
			definition: `
				interface HasName {
					name: String!
				}

				type User implements HasName {
					id: ID!
					name: String!
					isUser: Boolean!
				}
		
				type Admin implements HasName {
					id: ID!
					name: String!
				}
				
				type Query {
					iface: HasName!
				}`,
			upstreamDefinition: `
				interface HasName {
					name: String!
				}

				type User implements HasName {
					id: ID!
					name: String!
					isUser: Boolean!
				}
		
				type Admin implements HasName {
					id: ID!
					name: String! @external
				}
				
				type Query {
					iface: HasName!
				}`,
			dsConfiguration: dsb().
				RootNode("Query", "iface").
				RootNode("User", "id", "name", "isUser").
				RootNode("Admin", "id").
				ChildNode("HasName", "name").
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
				DS(),
			operation: `
				query {
					iface {
						... on HasName {
							name
							... on HasName {
								... on User {
									isUser
								}
							}
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
							name
							isUser
						}
					}
				}`,
			shouldRewrite: true,
		},
		{
			name:      "interface field having only other interface fragment with fragments inside",
			fieldName: "returnsUnion",
			definition: `
				interface HasName {
					name: String!
				}

				type User implements HasName {
					id: ID!
					name: String!
					isUser: Boolean!
				}
		
				type Admin implements HasName {
					id: ID!
					name: String!
				}
				
				type Moderator implements HasName {
					id: ID!
					name: String!
				}

				union Account = User | Admin | Moderator

				type Query {
					returnsUnion: Account!
				}`,
			upstreamDefinition: `
				interface HasName {
					name: String!
				}

				type User implements HasName {
					id: ID!
					name: String!
					isUser: Boolean!
				}
		
				type Admin implements HasName {
					id: ID!
					name: String! @external
				}

				union Account = User | Admin

				type Query {
					returnsUnion: Account!
				}`,
			dsConfiguration: dsb().
				RootNode("Query", "iface").
				RootNode("User", "id", "name", "isUser").
				RootNode("Admin", "id").
				ChildNode("HasName", "name").
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
				DS(),
			operation: `
				query {
					returnsUnion {
						... on HasName {
							name
							... on HasName {
								... on User {
									isUser
								}
								... on Moderator {
									name
								}
							}
						}
					}
				}`,
			expectedOperation: `
				query {
					returnsUnion {
						... on Admin {
							name
						}
						... on User {
							name
							isUser
						}
					}
				}`,
			shouldRewrite: true,
		},
		{
			name:      "don't have an interface in the datasource",
			fieldName: "returnsUnion",
			definition: `
				interface HasName {
					name: String!
				}

				type User implements HasName {
					id: ID!
					name: String!
					isUser: Boolean!
				}
		
				type Admin implements HasName {
					id: ID!
					name: String!
				}

				union Account = User | Admin

				type Query {
					returnsUnion: Account!
				}`,
			upstreamDefinition: `
				type User {
					id: ID!
					name: String!
					isUser: Boolean!
				}
		
				type Admin {
					id: ID!
					name: String! @external
				}

				union Account = User | Admin

				type Query {
					returnsUnion: Account!
				}`,
			dsConfiguration: dsb().
				RootNode("Query", "returnsUnion").
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
				DS(),
			operation: `
				query {
					returnsUnion {
						... on HasName {
							name
							... on HasName {
								... on User {
									isUser
								}
							}
						}
					}
				}`,
			expectedOperation: `
				query {
					returnsUnion {
						... on Admin {
							name
						}
						... on User {
							name
							isUser
						}
					}
				}`,
			shouldRewrite: true,
		},
		{
			name: "everything is local, nested interface selections with typename, first interface is matching field return type",
			definition: `
				interface HasName {
					name: String!
				}

				interface Title {
					title: String!
				}

				type User implements HasName & Title {
					id: ID!
					name: String!
					title: String!
				}
		
				type Admin implements HasName & Title {
					id: ID!
					name: String!
					title: String!
				}

				type Query {
					iface: HasName!
				}`,
			upstreamDefinition: `
				interface HasName {
					name: String!
				}

				interface Title {
					title: String!
				}

				type User implements HasName & Title {
					id: ID!
					name: String!
					title: String!
				}
		
				type Admin implements HasName & Title {
					id: ID!
					name: String!
					title: String!
				}

				type Query {
					iface: HasName!
				}`,
			dsConfiguration: dsb().
				RootNode("Query", "iface").
				RootNode("User", "id", "name", "title").
				RootNode("Admin", "id", "name", "title").
				ChildNode("HasName", "name").
				ChildNode("Title", "title").
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
				DS(),
			operation: `
				query {
					iface {
						... on HasName {
							__typename
							name
							... on Title {
								__typename
								title
							}
						}
					}
				}`,
			expectedOperation: `
				query {
					iface {
						... on HasName {
							__typename
							name
							... on Title {
								__typename
								title
							}
						}
					}
				}`,
			shouldRewrite: false,
		},
		{
			name: "everything is local, nested interface selections with typename, first interface is differs from field return type",
			definition: `
				interface HasName {
					name: String!
				}

				interface Title {
					title: String!
				}

				type User implements HasName & Title {
					id: ID!
					name: String!
					title: String!
				}
		
				type Admin implements HasName & Title {
					id: ID!
					name: String!
					title: String!
				}

				type Query {
					iface: HasName!
				}`,
			upstreamDefinition: `
				interface HasName {
					name: String!
				}

				interface Title {
					title: String!
				}

				type User implements HasName & Title {
					id: ID!
					name: String!
					title: String!
				}
		
				type Admin implements HasName & Title {
					id: ID!
					name: String!
					title: String!
				}

				type Query {
					iface: HasName!
				}`,
			dsConfiguration: dsb().
				RootNode("Query", "iface").
				RootNode("User", "id", "name", "title").
				RootNode("Admin", "id", "name", "title").
				ChildNode("HasName", "name").
				ChildNode("Title", "title").
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
				DS(),
			operation: `
				query {
					iface {
						... on Title {
							__typename
							title
							... on HasName {
								__typename
								name
							}
						}
					}
				}`,
			expectedOperation: `
				query {
					iface {
						... on Title {
							__typename
							title
							... on HasName {
								__typename
								name
							}
						}
					}
				}`,
			shouldRewrite: false,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			run(t, testCase)
		})
	}
}
