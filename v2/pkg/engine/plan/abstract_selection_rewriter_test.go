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
		upstreamDefinition string // will be used by dsBuilder for dataSourceConfiguration.factory
		dsBuilder          *dsBuilder
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
		require.NotEqual(t, ast.InvalidRef, fieldRef)

		// Schema is working here, but it parses just the schema without merging with the base,
		// as we do for the upstreamDef.
		ds := testCase.dsBuilder.
			Schema(testCase.definition).
			DS()

		node, _ := def.Index.FirstNodeByNameStr(testCase.enclosingTypeName)

		rewriter := newFieldSelectionRewriter(&op, &def)
		rewriter.SetUpstreamDefinition(&upstreamDef)
		rewriter.SetDatasourceConfiguration(ds)

		result, err := rewriter.RewriteFieldSelection(fieldRef, node)
		require.NoError(t, err)
		assert.Equal(t, testCase.shouldRewrite, result.rewritten)

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

	definitionA := `
		type Query {
			u1: Union1
			i1: Inter1
		}
		
		union Union1 = A | B | C
		union Union2 = A | B | C
		union Union3 = A | B | C

		interface Inter1 {
			id: ID
		}
		
		interface Inter2 {
			id: ID
		}

		interface Inter3 {
			id: ID
		}

		type A implements Inter1 & Inter2 & Inter3 @key(fields: "id") {
			id: ID!
			name: String!
		}
		
		type B implements Inter1 & Inter2 & Inter3 @key(fields: "id") {
			id: ID!
			name: String!
		}

		type C @key(fields: "id") {
			id: ID!
			name: String!
		}`

	upstreamDefinitionA := `
		type Query {
			u1: Union1
			i1: Inter1
		}
		
		union Union1 = A | B
		union Union2 = A | B
		
		interface Inter1 {
			id: ID
		}
		
		interface Inter2 {
			id: ID
		}
		
		type A implements Inter1 & Inter2 @key(fields: "id") {
			id: ID!
		}
		
		type B implements Inter1 & Inter2 @key(fields: "id") {
			id: ID!
		}`

	dsConfigurationA := dsb().
		RootNode("Query", "u1").
		RootNode("A", "id").
		RootNode("B", "id").
		ChildNode("Inter1", "id").
		ChildNode("Inter2", "id").
		KeysMetadata(FederationFieldConfigurations{
			{
				TypeName:     "A",
				SelectionSet: "id",
			},
			{
				TypeName:     "B",
				SelectionSet: "id",
			},
		})

	definitionB := `
		type Query {
			named: Named
			union: U
		}

		union U = User

		interface Named {
			name: String
		}

		interface Numbered {
			number: Int
		}

		type User implements Named & Numbered {
			id: ID
			named: String
			number: Int
		}`

	testCases := []testCase{
		{
			name:               "should flatten interfaces for grpc",
			fieldName:          "named",
			definition:         definitionB,
			upstreamDefinition: definitionB,
			dsBuilder: dsb().
				UpstreamKind("grpc").
				RootNode("Query", "named", "union").
				RootNode("User", "id", "name", "number").
				ChildNode("Named", "name").
				ChildNode("Numbered", "number").
				KeysMetadata(FederationFieldConfigurations{
					{
						TypeName:     "User",
						SelectionSet: "id",
					},
				}),
			operation: `
				query {
					named {
						... on Numbered {
							... on User {
								name
							}
						}
						... on User {
							id
						}
					}
				}`,
			expectedOperation: `
				query {
					named {
						... on User {
							id
							name
						}
					}
				}`,
			shouldRewrite: true,
		},
		{
			name:               "should flatten union for grpc",
			fieldName:          "union",
			definition:         definitionB,
			upstreamDefinition: definitionB,
			dsBuilder: dsb().
				UpstreamKind("grpc").
				RootNode("Query", "named", "union").
				RootNode("User", "id", "name", "number").
				ChildNode("Named", "name").
				ChildNode("Numbered", "number").
				KeysMetadata(FederationFieldConfigurations{
					{
						TypeName:     "User",
						SelectionSet: "id",
					},
				}),
			operation: `
				query {
					union {
						... on Numbered {
							... on User {
								name
							}
						}
						... on User {
							id
						}
					}
				}`,
			expectedOperation: `
				query {
					union {
						... on User {
							id
							name
						}
					}
				}`,
			shouldRewrite: true,
		},
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
			dsBuilder: dsb().
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
				}),
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
			dsBuilder: dsb().
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
				}),
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
			dsBuilder: dsb().
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
				}),
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
			dsBuilder: dsb().
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
				}),
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

				type Query {
					iface: Node!
				}
			`,
			dsBuilder: dsb().
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
				}),
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
			dsBuilder: dsb().
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
				}),
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
			dsBuilder: dsb().
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
				}),
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
			dsBuilder: dsb().
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
				}),
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
			dsBuilder: dsb().
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
				}),
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
			dsBuilder: dsb().
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
				}),
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

				type Query {
					accounts: [Account!]!
				}
			`,
			dsBuilder: dsb().
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
				}),
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

				type Query {
					accounts: [Account!]!
				}
			`,
			dsBuilder: dsb().
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
				}),
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
			dsBuilder: dsb().
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
				}),
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
			dsBuilder: dsb().
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
				}),
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
			dsBuilder: dsb().
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
				}),
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
			dsBuilder: dsb().
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
				}),
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
			dsBuilder: dsb().
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
				}),
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
			dsBuilder: dsb().
				RootNode("Query", "iface").
				ChildNode("User", "id", "name", "isUser").
				ChildNode("Admin", "id", "name"),
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
			dsBuilder: dsb().
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
				}),
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
			dsBuilder: dsb().
				RootNode("Query", "iface").
				ChildNode("User", "id", "name", "isUser").
				KeysMetadata(FederationFieldConfigurations{
					{
						TypeName:     "User",
						SelectionSet: "id",
					},
				}),
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
			dsBuilder: dsb().
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
				}),
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
			dsBuilder: dsb().
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
				}),
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
			dsBuilder: dsb().
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
				}),
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
			dsBuilder: dsb().
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
				}),
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
			dsBuilder: dsb().
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
				}),
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
			dsBuilder: dsb().
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
				}),
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
			dsBuilder: dsb().
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
				}),
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
						... on User {
							isUser
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
			dsBuilder: dsb().
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
				}),
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
			dsBuilder: dsb().
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
				}),
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
			dsBuilder: dsb().
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
				}),
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
			dsBuilder: dsb().
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
				}),
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
			dsBuilder: dsb().
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
				}),
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
			dsBuilder: dsb().
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
				}),
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
			name:      "don't have all type implementing interface in the datasource",
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
				type User implements HasName {
					id: ID!
					name: String!
					isUser: Boolean!
				}

				type Admin {
					id: ID!
				}

				union Account = User | Admin

				type Query {
					returnsUnion: Account!
				}`,
			dsBuilder: dsb().
				RootNode("Query", "returnsUnion").
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
				}),
			operation: `
				query {
					returnsUnion {
						... on HasName {
							name	
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
			dsBuilder: dsb().
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
				}),
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
			name: "everything is local, nested interface selections with typename, first interface differs from field return type",
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
			dsBuilder: dsb().
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
				}),
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
		{
			name:      "field is a concrete type from a union in the given subgraph ",
			fieldName: "returnsUser",
			definition: `
				type User {
					id: ID!
					name: String!
				}
		
				type Admin {
					id: ID!
					name: String!
				}

				union Account = User | Admin

				type Query {
					returnsUnion: Account!
					returnsUser: Account!
				}`,
			upstreamDefinition: `
				type User {
					id: ID!
					name: String!
				}

				type Query {
					returnsUser: User!
				}`,
			dsBuilder: dsb().
				RootNode("Query", "returnsUser").
				RootNode("User", "id", "name").
				KeysMetadata(FederationFieldConfigurations{
					{
						TypeName:     "User",
						SelectionSet: "id",
					},
				}),
			operation: `
				query {
					returnsUser {
						... on User {
							name
						}
						... on Admin {
							name
						}
					}
				}`,
			expectedOperation: `
				query {
					returnsUser {
						... on User {
							name
						}
					}
				}`,
			shouldRewrite: true,
		},
		{
			name: "field is a concrete type from an interface members which do not implement interface in the given subgraph ",
			definition: `
				type User implements Account {
					id: ID!
					name: String!
				}
		
				type Admin implements Account {
					id: ID!
					name: String!
				}

				interface Account {
					id: ID!
				}

				type Query {
					iface: Account!
				}`,
			upstreamDefinition: `
				type User {
					id: ID!
					name: String!
				}

				type Admin {
					id: ID!
					name: String!
				}

				type Query {
					iface: User!
				}`,
			dsBuilder: dsb().
				RootNode("Query", "iface").
				RootNode("User", "id", "name").
				ChildNode("Account", "id").
				KeysMetadata(FederationFieldConfigurations{
					{
						TypeName:     "User",
						SelectionSet: "id",
					},
				}),
			operation: `
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
			expectedOperation: `
				query {
					iface {
						... on User {
							name
						}
					}
				}`,
			shouldRewrite: true,
		},
		{
			name: "field is a concrete type from an interface members which do not implement interface in the given subgraph - query type renamed",
			definition: `
				type User implements Account {
					id: ID!
					name: String!
				}
		
				type Admin implements Account {
					id: ID!
					name: String!
				}

				interface Account {
					id: ID!
				}

				type Query {
					iface: Account!
				}`,
			upstreamDefinition: `
				schema {
					query: QueryType
				}

				type User {
					id: ID!
					name: String!
				}

				type Admin {
					id: ID!
					name: String!
				}

				type QueryType {
					iface: User!
				}`,
			dsBuilder: dsb().
				RootNode("Query", "iface").
				RootNode("User", "id", "name").
				ChildNode("Account", "id").
				KeysMetadata(FederationFieldConfigurations{
					{
						TypeName:     "User",
						SelectionSet: "id",
					},
				}),
			operation: `
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
			expectedOperation: `
				query {
					iface {
						... on User {
							name
						}
					}
				}`,
			shouldRewrite: true,
		},
		{
			name:               "field is union - union fragment wrapped into concrete type fragment with different from wrapping type fragments",
			definition:         definitionA,
			upstreamDefinition: upstreamDefinitionA,
			dsBuilder:          dsConfigurationA,
			fieldName:          "u1",
			operation: `
				query {
					u1 {
						...	on A {
							title
							... on Union2 {
								... on A {
									id
								}
								... on C {
									title
								}
							}
						}
					}
				}`,
			expectedOperation: `
				query {
					u1 {
						... on A {
							title
							id
						}
					}
				}`,
			shouldRewrite: true,
		},
		{
			name:               "field is union - union fragment wrapped into concrete type fragment with matching to wrapping type fragment",
			definition:         definitionA,
			upstreamDefinition: upstreamDefinitionA,
			dsBuilder:          dsConfigurationA,
			fieldName:          "u1",
			operation: `
				query {
					u1 {
						...	on A {
							title
							... on Union2 {
								... on A {
									id
								}
							}
						}
					}
				}`,
			expectedOperation: `
				query {
					u1 {
						... on A {
							title
							id
						}
					}
				}`,
			shouldRewrite: true,
		},
		{
			name:               "field is union - select not existing in the current subgraph type",
			definition:         definitionA,
			upstreamDefinition: upstreamDefinitionA,
			dsBuilder:          dsConfigurationA,
			fieldName:          "u1",
			operation: `
				query {
					u1 {
						... on Union2 {
							... on A {
								id
							}
							... on C {
								title
							}
						}
					}
				}`,
			expectedOperation: `
				query {
					u1 {
						... on A {
							id
						}
					}
				}`,
			shouldRewrite: true,
		},
		{
			name:               "field is interface - select not existing in the current subgraph type",
			definition:         definitionA,
			upstreamDefinition: upstreamDefinitionA,
			dsBuilder:          dsConfigurationA,
			fieldName:          "i1",
			operation: `
				query {
					i1 {
						... on Union2 {
							... on A {
								id
							}
							... on C {
								title
							}
						}
					}
				}`,
			expectedOperation: `
				query {
					i1 {
						... on A {
							id
						}
					}
				}`,
			shouldRewrite: true,
		},
		{
			name:               "field is an interface - interface fragment inside concrete type fragment with matching type",
			definition:         definitionA,
			upstreamDefinition: upstreamDefinitionA,
			dsBuilder:          dsConfigurationA,
			fieldName:          "i1",
			operation: `
				query {
					i1 {
						... on A {
							... on Inter2 {
								... on A {
									id
								}
							}
						}
					}
				}`,
			expectedOperation: `
				query {
					i1 {
						... on A {
							id
						}
					}
				}`,
			shouldRewrite: true,
		},
		{
			name:               "field is an interface - interface fragment inside concrete type fragment with not matching type",
			definition:         definitionA,
			upstreamDefinition: upstreamDefinitionA,
			dsBuilder:          dsConfigurationA,
			fieldName:          "i1",
			operation: `
				query {
					i1 {
						... on A {
							... on Inter2 {
								... on B {
									id
								}
							}
						}
					}
				}`,
			expectedOperation: `
				query {
					i1 {
						__typename
					}
				}`,
			shouldRewrite: true,
		},
		{
			name:               "field is an interface - interface fragment inside concrete type fragment select shared field",
			definition:         definitionA,
			upstreamDefinition: upstreamDefinitionA,
			dsBuilder:          dsConfigurationA,
			fieldName:          "i1",
			operation: `
				query {
					i1 {
						... on A {
							... on Inter2 {
								id
							}
						}
					}
				}`,
			expectedOperation: `
				query {
					i1 {
						... on A {
							id
						}
					}
				}`,
			shouldRewrite: true,
		},
		{
			name:               "field is an interface - multiple level of nesting interface and union fragments with concrete types on different levels",
			definition:         definitionA,
			upstreamDefinition: upstreamDefinitionA,
			dsBuilder:          dsConfigurationA,
			fieldName:          "i1",
			operation: `
				query {
					i1 {
						... on Union2 {
							... on Inter2 {
								... on A {
									id
								}
								... on Union1 {
									... on Inter1 {
										... on B {
											id
										}
									}
								}
							}
						}
					}
				}`,
			expectedOperation: `
				query {
					i1 {
						... on A {
							id
						}
						... on B {
							id
						}
					}
				}`,
			shouldRewrite: true,
		},
		{
			name:               "field is a interface - union fragment is not exists in the current subgraph",
			definition:         definitionA,
			upstreamDefinition: upstreamDefinitionA,
			dsBuilder:          dsConfigurationA,
			fieldName:          "i1",
			operation: `
				query {
					i1 {
						... on Union3 {
							... on A {
								id
							}
							... on B {
								id
							}
						}
					}
				}`,
			expectedOperation: `
				query {
					i1 {
						... on A {
							id
						}
						... on B {
							id
						}
					}
				}`,
			shouldRewrite: true,
		},
		{
			name:               "field is a interface - interface fragment is not exists in the current subgraph",
			definition:         definitionA,
			upstreamDefinition: upstreamDefinitionA,
			dsBuilder:          dsConfigurationA,
			fieldName:          "i1",
			operation: `
				query {
					i1 {
						... on Inter3 {
							... on A {
								id
							}
							... on B {
								id
							}
						}
					}
				}`,
			expectedOperation: `
				query {
					i1 {
						... on A {
							id
						}
						... on B {
							id
						}
					}
				}`,
			shouldRewrite: true,
		},
		{
			name:               "field is a union - union fragment is not exists in the current subgraph",
			definition:         definitionA,
			upstreamDefinition: upstreamDefinitionA,
			dsBuilder:          dsConfigurationA,
			fieldName:          "u1",
			operation: `
				query {
					u1 {
						... on Union3 {
							... on A {
								id
							}
							... on B {
								id
							}
							... on C {
								id
							}
						}
					}
				}`,
			expectedOperation: `
				query {
					u1 {
						... on A {
							id
						}
						... on B {
							id
						}
					}
				}`,
			shouldRewrite: true,
		},
		{
			name:               "field is a union - interface fragment is not exists in the current subgraph",
			definition:         definitionA,
			upstreamDefinition: upstreamDefinitionA,
			dsBuilder:          dsConfigurationA,
			fieldName:          "u1",
			operation: `
				query {
					u1 {
						... on Inter3 {
							... on A {
								id
							}
							... on B {
								id
							}
						}
					}
				}`,
			expectedOperation: `
				query {
					u1 {
						... on A {
							id
						}
						... on B {
							id
						}
					}
				}`,
			shouldRewrite: true,
		},
		{
			name: "field is an object having interface fragments inside with non matching type",
			definition: `
				type User implements Account {
					id: ID!
					name: String!
				}
		
				type Admin implements Account {
					id: ID!
					name: String!
				}

				interface Account {
					id: ID!
				}

				type Query {
					user: User!
				}`,
			upstreamDefinition: `
				type User implements Account {
					id: ID!
				}
		
				type Admin implements Account {
					id: ID!
				}

				interface Account {
					id: ID!
				}

				type Query {
					user: User!
				}`,
			dsBuilder: dsb().
				RootNode("Query", "user").
				RootNode("User", "id").
				ChildNode("Account", "id").
				KeysMetadata(FederationFieldConfigurations{
					{
						TypeName:     "User",
						SelectionSet: "id",
					},
					{
						TypeName:     "Admin",
						SelectionSet: "id",
					},
				}),
			fieldName: "user",
			operation: `
				query {
					user {
						... on Account {
							... on User {
								name
							}
							... on Admin {
								name
							}
						}
					}
				}`,
			expectedOperation: `
				query {
					user {
						name
					}
				}`,
			shouldRewrite: true,
		},
		{
			name: "field is an object having union fragments inside with non matching type",
			definition: `
				type User implements Account {
					id: ID!
					name: String!
					surname: String!
				}
		
				type Admin implements Account {
					id: ID!
					name: String!
					login: String!
				}

				interface Account {
					id: ID!
				}

				union AccountUnion = User | Admin

				type Query {
					user: User!
				}`,
			upstreamDefinition: `
				type User implements Account {
					id: ID!
				}
		
				type Admin implements Account {
					id: ID!
				}

				interface Account {
					id: ID!
				}

				union AccountUnion = User | Admin

				type Query {
					user: User!
				}`,
			dsBuilder: dsb().
				RootNode("Query", "user").
				RootNode("User", "id").
				ChildNode("Account", "id").
				KeysMetadata(FederationFieldConfigurations{
					{
						TypeName:     "User",
						SelectionSet: "id",
					},
					{
						TypeName:     "Admin",
						SelectionSet: "id",
					},
				}),
			fieldName: "user",
			operation: `
				query {
					user {
						...	on AccountUnion {
							... on Account {
								... on User {
									name
								}
								... on Admin {
									name
								}
							}
							... on User {
								id
							}
						}
						... on Account {
							... on User {
								... on AccountUnion {
									... on User {
										surname
									}
									... on Admin {
										login
									}
								}
							}
						}
					}
				}`,
			// Note: order of fields changes because we are handling each type of fragments separately
			expectedOperation: `
				query {
					user {
						surname
						id
						name
					}
				}`,
			shouldRewrite: true,
		},
		{
			name: "field is an object having union fragments inside with non matching type and fields on object itself",
			definition: `
				type User implements Account {
					id: ID!
					name: String!
					surname: String!
					address: String!
					someField: String!
				}
		
				type Admin implements Account {
					id: ID!
					name: String!
					login: String!
				}

				interface Account {
					id: ID!
				}

				union AccountUnion = User | Admin

				type Query {
					user: User!
				}`,
			upstreamDefinition: `
				type User implements Account {
					id: ID!
				}
		
				type Admin implements Account {
					id: ID!
				}

				interface Account {
					id: ID!
				}

				union AccountUnion = User | Admin

				type Query {
					user: User!
				}`,
			dsBuilder: dsb().
				RootNode("Query", "user").
				RootNode("User", "id").
				ChildNode("Account", "id").
				KeysMetadata(FederationFieldConfigurations{
					{
						TypeName:     "User",
						SelectionSet: "id",
					},
					{
						TypeName:     "Admin",
						SelectionSet: "id",
					},
				}),
			fieldName: "user",
			operation: `
				query {
					user {
						__typename
						address
						someField
						...	on AccountUnion {
							... on Account {
								... on User {
									name
								}
								... on Admin {
									name
								}
							}
							... on User {
								id
							}
						}
						... on Account {
							... on User {
								... on AccountUnion {
									... on User {
										surname
									}
									... on Admin {
										login
									}
								}
							}
						}
					}
				}`,
			// Note: order of fields changes because we are handling each type of fragments separately
			expectedOperation: `
				query {
					user {
						__typename
						address
						someField
						surname
						id
						name
					}
				}`,
			shouldRewrite: true,
		},
		{
			name: "field is an object which is not an entity having union fragments inside with non matching type, everything is local",
			definition: `
				type User implements Account {
					id: ID!
					name: String!
					surname: String!
				}
		
				type Admin implements Account {
					id: ID!
					name: String!
					login: String!
				}

				interface Account {
					id: ID!
				}

				union AccountUnion = User | Admin

				type Query {
					user: User!
				}`,
			upstreamDefinition: `
				type User implements Account {
					id: ID!
					name: String!
					surname: String!
				}
		
				type Admin implements Account {
					id: ID!
					name: String!
					login: String!
				}

				interface Account {
					id: ID!
				}

				union AccountUnion = User | Admin

				type Query {
					user: User!
				}`,
			dsBuilder: dsb().
				RootNode("Query", "user").
				ChildNode("User", "id", "name", "surname").
				ChildNode("Admin", "id", "name", "login").
				ChildNode("Account", "id"),
			fieldName: "user",
			operation: `
				query {
					user {
						...	on AccountUnion {
							... on Account {
								... on User {
									name
								}
								... on Admin {
									name
								}
							}
							... on User {
								id
							}
						}
						... on Account {
							... on User {
								... on AccountUnion {
									... on User {
										surname
									}
									... on Admin {
										login
									}
								}
							}
						}
					}
				}`,
			expectedOperation: `
				query {
					user {
						...	on AccountUnion {
							... on Account {
								... on User {
									name
								}
								... on Admin {
									name
								}
							}
							... on User {
								id
							}
						}
						... on Account {
							... on User {
								... on AccountUnion {
									... on User {
										surname
									}
									... on Admin {
										login
									}
								}
							}
						}
					}
				}`,
			shouldRewrite: false,
		},
		{
			name: "field is an interface object with concrete type fragments",
			definition: `
				type User implements Account {
					id: ID!
					name: String!
					surname: String!
				}
		
				type Admin implements Account {
					id: ID!
					name: String!
					login: String!
				}

				interface Account {
					id: ID!
					title: String!
				}

				type Query {
					user: Account!
				}`,
			upstreamDefinition: `
				type Account @key(fields: "id") @interfaceObject {
					id: ID!
					title: String!
				}`,
			dsBuilder: dsb().
				RootNode("Account", "id", "title").
				WithMetadata(func(m *FederationMetaData) {
					m.InterfaceObjects = []EntityInterfaceConfiguration{
						{
							InterfaceTypeName: "Account",
							ConcreteTypeNames: []string{"Admin", "User"},
						},
					}
				}),
			fieldName: "name",
			operation: `
				query {
					__typename
					user {
						... on User {
							name
						}
					}
				}`,
			expectedOperation: `
				query {
					__typename
					user {
						... on User {
							name
						}
					}
				}`,
			shouldRewrite: false,
		},
		{
			name: "field is an interface with concrete type fragments. one of implementing types is interface object",
			definition: `
				interface Named {
					name: String!
				}

				type User implements Named {
					id: ID!
					name: String!
					surname: String!
				}
		
				type Admin implements Account & Named {
					id: ID!
					name: String!
					title: String!
				}

				interface Account implements Named {
					id: ID!
					name: String!
					title: String!
				}

				type Query {
					user: Named!
				}`,
			upstreamDefinition: `
				interface Named {
					name: String!
				}

				type User implements Named {
					id: ID!
					name: String!
					surname: String!
				}

				type Account implements Named  @interfaceObject @key(fields: "id") {
					id: ID!
					name: String!
				}

				type Query {
					user: Named!
				}`,
			dsBuilder: dsb().
				RootNode("Account", "id", "title").
				RootNode("User", "id", "name", "surname").
				WithMetadata(func(m *FederationMetaData) {
					m.InterfaceObjects = []EntityInterfaceConfiguration{
						{
							InterfaceTypeName: "Account",
							ConcreteTypeNames: []string{"Admin", "User"},
						},
					}
				}),
			fieldName: "user",
			operation: `
				query {
					__typename
					user {
						id
						... on Account {
							... on Admin {
								title
							}
						}
					}
				}`,
			expectedOperation: `
				query {
					__typename
					user {
						... on Admin {
							id
							title
						}
						... on User {
							id
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
