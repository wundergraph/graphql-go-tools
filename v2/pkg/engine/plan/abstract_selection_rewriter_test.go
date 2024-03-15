package plan

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/graphql-go-tools/v2/internal/pkg/unsafeparser"
	"github.com/TykTechnologies/graphql-go-tools/v2/internal/pkg/unsafeprinter"
	"github.com/TykTechnologies/graphql-go-tools/v2/pkg/ast"
)

func TestInterfaceSelectionRewriter_RewriteOperation(t *testing.T) {
	type testCase struct {
		name               string
		definition         string
		upstreamDefinition string
		dsConfiguration    *DataSourceConfiguration
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

		printedOp := unsafeprinter.PrettyPrint(&op, &def)
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
				DSPtr(),
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
				DSPtr(),
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
				DSPtr(),
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
			name:       "one field is external. query has admin and user fragment and shared __typename",
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
						... on Admin {
							name
							__typename
							id
						}
						... on User {
							name
							__typename
							isUser
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
			name:               "all fields local. query with user fragment. types are not entities",
			definition:         definition,
			upstreamDefinition: definition,
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
			name:               "Union with interface fragment: no entity fragments, all fields local",
			definition:         definition,
			upstreamDefinition: definition,
			dsConfiguration: dsb().
				RootNode("Query", "iface", "accounts").
				RootNode("User", "id", "name", "isUser").
				RootNode("Admin", "id", "name").
				RootNode("Moderator", "id", "name", "isModerator").
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
				DSPtr(),
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
				DSPtr(),
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
				DSPtr(),
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
				DSPtr(),
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
				DSPtr(),
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
				DSPtr(),
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
				DSPtr(),
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
				DSPtr(),
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
				DSPtr(),
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
						}
					}
				}`,
			shouldRewrite: true,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			run(t, testCase)
		})
	}
}
