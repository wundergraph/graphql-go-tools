package operation_complexity

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/v2/internal/pkg/unsafeparser"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astnormalization"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/operationreport"
)

func TestCalculateOperationComplexity(t *testing.T) {
	t.Run("query with a scalar return type", func(t *testing.T) {
		run(t, testDefinition, `
				{
				  currentPeriod
				}`,
			OperationStats{
				NodeCount:  0,
				Complexity: 0,
				Depth:      0,
			},
			[]RootFieldStats{
				{
					TypeName:  "Query",
					FieldName: "currentPeriod",
					Stats: OperationStats{
						NodeCount:  0,
						Complexity: 0,
						Depth:      0,
					},
				},
			},
		)
	})
	t.Run("one user", func(t *testing.T) {
		run(t, testDefinition, `
				{
				  users(first: 1) {
					id
					balance
					name
					address {
					  city
					  country
					}
				  }
				}`,
			OperationStats{
				NodeCount:  2,
				Complexity: 2,
				Depth:      3,
			},
			[]RootFieldStats{
				{
					TypeName:  "Query",
					FieldName: "users",
					Stats: OperationStats{
						NodeCount:  2,
						Complexity: 2,
						Depth:      2,
					},
				},
			},
		)
	})
	t.Run("one user with inline fragments", func(t *testing.T) {
		run(t, testDefinition, `
				{
				  users(first: 1) {
					... {
						id
						balance
						name
						address {
						  city
						  country
						}
					}
				  }
				}`,
			OperationStats{
				NodeCount:  2,
				Complexity: 2,
				Depth:      3,
			},
			[]RootFieldStats{
				{
					TypeName:  "Query",
					FieldName: "users",
					Stats: OperationStats{
						NodeCount:  2,
						Complexity: 2,
						Depth:      2,
					},
				},
			},
		)
	})
	t.Run("one user with fragments", func(t *testing.T) {
		run(t, testDefinition, `
				{
				  users(first: 1) {
					...UserFragment
				  }
				}
				fragment UserFragment on User {
					id
					balance
					name
					address {
					  city
					  country
					}
                }
				`,
			OperationStats{
				NodeCount:  2,
				Complexity: 2,
				Depth:      3,
			},
			[]RootFieldStats{
				{
					TypeName:  "Query",
					FieldName: "users",
					Stats: OperationStats{
						NodeCount:  2,
						Complexity: 2,
						Depth:      2,
					},
				},
			},
		)
	})
	t.Run("multiple users", func(t *testing.T) {
		run(t, testDefinition, `
				{
				  users(first: 10) {
					id
					balance
					name
					address {
					  city
					  country
					}
				  }
				}`,
			OperationStats{
				NodeCount:  20,
				Complexity: 11,
				Depth:      3,
			},
			[]RootFieldStats{
				{
					TypeName:  "Query",
					FieldName: "users",
					Stats: OperationStats{
						NodeCount:  20,
						Complexity: 11,
						Depth:      2,
					},
				},
			},
		)
	})
	t.Run("multiple users with multiple transactions", func(t *testing.T) {
		run(t, testDefinition, `
				{
				  users(first: 10) {
					id
					balance
					name
					address {
					  city
					  country
					}
					transactions(first: 5) {
						id
						amount
					}
				  }
				}`,
			OperationStats{
				NodeCount:  70,
				Complexity: 21,
				Depth:      3,
			},
			[]RootFieldStats{
				{
					TypeName:  "Query",
					FieldName: "users",
					Stats: OperationStats{
						NodeCount:  70,
						Complexity: 21,
						Depth:      2,
					},
				},
			},
		)
	})
	t.Run("multiple users with multiple transactions with nested senders", func(t *testing.T) {
		run(t, testDefinition, `
				{
				  users(first: 10) {
					id
					balance
					name
					address {
					  city
					  country
					}
					transactions(first: 5) {
						id
						amount
						sender {
							id
							transactions(first: 10) {
								id
								amount
							}
						}
						recipient {
							id
							transactions(first: 5) {
								id
								amount
							}
						}
					}
				  }
				}`,
			OperationStats{
				NodeCount:  920,
				Complexity: 221,
				Depth:      5,
			},
			[]RootFieldStats{
				{
					TypeName:  "Query",
					FieldName: "users",
					Stats: OperationStats{
						NodeCount:  920,
						Complexity: 221,
						Depth:      4,
					},
				},
			},
		)
	})
	t.Run("multiple queries and one being an alias", func(t *testing.T) {
		run(t, testDefinition, `
				{
				  person: user(id: "1") {
					name
 				  }
				  users(first: 1) {
					id
					balance
					name
					address {
					  city
					  country
					}
				  }
				  bestUsers: users(first: 10) {
					id
					balance
					name
					address {
					  city
					  country
					}
					transactions(first: 5) {
						id
						amount
					}
				  }
				}`,
			OperationStats{
				NodeCount:  73,
				Complexity: 24,
				Depth:      3,
			},
			[]RootFieldStats{
				{
					TypeName:  "Query",
					FieldName: "user",
					Alias:     "person",
					Stats: OperationStats{
						NodeCount:  1,
						Complexity: 1,
						Depth:      1,
					},
				},
				{
					TypeName:  "Query",
					FieldName: "users",
					Stats: OperationStats{
						NodeCount:  2,
						Complexity: 2,
						Depth:      2,
					},
				},
				{
					TypeName:  "Query",
					FieldName: "users",
					Alias:     "bestUsers",
					Stats: OperationStats{
						NodeCount:  70,
						Complexity: 21,
						Depth:      2,
					},
				},
			},
		)
	})
	t.Run("multiple queries with different depth higher depth first", func(t *testing.T) {
		run(t, testDefinition, `
				{
					transactions(first: 1) {
						id
						sender {
							id
							transactions(first: 1) {
								id
							}
						}
					}
					users(first: 1) {
						id
						address {
					  		city
						}
					}
				}`,
			OperationStats{
				NodeCount:  5,
				Complexity: 5,
				Depth:      4,
			},
			[]RootFieldStats{
				{
					TypeName:  "Query",
					FieldName: "transactions",
					Stats: OperationStats{
						NodeCount:  3,
						Complexity: 3,
						Depth:      3,
					},
				},
				{
					TypeName:  "Query",
					FieldName: "users",
					Stats: OperationStats{
						NodeCount:  2,
						Complexity: 2,
						Depth:      2,
					},
				},
			},
		)
	})
	t.Run("multiple queries with different depth higher depth last", func(t *testing.T) {
		run(t, testDefinition, `
				{
					users(first: 1) {
						id
						address {
					  		city
						}
					}
					transactions(first: 1) {
						id
						sender {
							id
							transactions(first: 1) {
								id
							}
						}
					}
				}`,
			OperationStats{
				NodeCount:  5,
				Complexity: 5,
				Depth:      4,
			},
			[]RootFieldStats{
				{
					TypeName:  "Query",
					FieldName: "users",
					Stats: OperationStats{
						NodeCount:  2,
						Complexity: 2,
						Depth:      2,
					},
				},
				{
					TypeName:  "Query",
					FieldName: "transactions",
					Stats: OperationStats{
						NodeCount:  3,
						Complexity: 3,
						Depth:      3,
					},
				},
			},
		)
	})
	t.Run("multiple mutations with alias", func(t *testing.T) {
		run(t, testDefinition, `
				mutation AlterUsers {
				  createJohn: createUser(input: {balance: 10, name: "John Doe", email: "john@doe.fake"}) {
                    id
				  }
				  createJane: createUser(input: {balance: 100, name: "Jane Doe", email: "jane@doe.fake"}) {
                    id
				  }
				}`,
			OperationStats{
				NodeCount:  2,
				Complexity: 2,
				Depth:      2,
			},
			[]RootFieldStats{
				{
					TypeName:  "Mutation",
					FieldName: "createUser",
					Alias:     "createJohn",
					Stats: OperationStats{
						NodeCount:  1,
						Complexity: 1,
						Depth:      1,
					},
				},
				{
					TypeName:  "Mutation",
					FieldName: "createUser",
					Alias:     "createJane",
					Stats: OperationStats{
						NodeCount:  1,
						Complexity: 1,
						Depth:      1,
					},
				},
			},
		)
	})
	t.Run("introspection query", func(t *testing.T) {
		run(t, testDefinition, introspectionQuery,
			OperationStats{
				NodeCount:  0,
				Complexity: 0,
				Depth:      0,
			},
			[]RootFieldStats{},
		)
	})
}

var run = func(t *testing.T, definition, operation string, expectedGlobalComplexityResult OperationStats, expectedFieldsComplexityResult []RootFieldStats) {
	def := unsafeparser.ParseGraphqlDocumentString(definition)
	op := unsafeparser.ParseGraphqlDocumentString(operation)
	report := operationreport.Report{}

	astnormalization.NormalizeOperation(&op, &def, &report)

	actualGlobalComplexityResult, actualFieldsComplexityResult := CalculateOperationComplexity(&op, &def, &report)
	if report.HasErrors() {
		require.NoError(t, report)
	}

	assert.Equal(t, expectedGlobalComplexityResult.NodeCount, actualGlobalComplexityResult.NodeCount, "unexpected global node count")
	assert.Equal(t, expectedGlobalComplexityResult.Complexity, actualGlobalComplexityResult.Complexity, "unexpected global complexity")
	assert.Equal(t, expectedGlobalComplexityResult.Depth, actualGlobalComplexityResult.Depth, "unexpected global depth")
	assert.Equal(t, expectedFieldsComplexityResult, actualFieldsComplexityResult, "unexpected fields complexity result")
}

func BenchmarkEstimateComplexity(b *testing.B) {
	def := unsafeparser.ParseGraphqlDocumentString(testDefinition)
	op := unsafeparser.ParseGraphqlDocumentString(complexQuery)

	estimator := NewOperationComplexityEstimator()
	report := operationreport.Report{}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		globalComplexityResult, _ := estimator.Do(&op, &def, &report)
		if report.HasErrors() {
			b.Fatal(report)
		}

		if globalComplexityResult.NodeCount != 920 {
			b.Fatalf("want nodeCount: 920, got: %d\n", globalComplexityResult.NodeCount)
		}
		if globalComplexityResult.Complexity != 221 {
			b.Fatalf("want complexity: 221, got: %d\n", globalComplexityResult.Complexity)
		}
		if globalComplexityResult.Depth != 5 {
			b.Fatalf("want depth: 5, got: %d\n", globalComplexityResult.Depth)
		}
	}
}

const complexQuery = `
{
  users(first: 10) {
	id
	balance
	name
	address {
	  city
	  country
	}
	transactions(first: 5) {
		id
		amount
		sender {
			id
			transactions(first: 10) {
				id
				amount
			}
		}
		recipient {
			id
			transactions(first: 5) {
				id
				amount
			}
		}
	}
  }
}`

const testDefinition = `

directive @nodeCountMultiply on ARGUMENT_DEFINITION
directive @nodeCountSkip on FIELD

scalar Date

type User {
    id: ID!
    balance: Int!
    name: String!
    email: String!
    address: Address
    transactions(first: Int! @nodeCountMultiply, afterID: ID): [Transaction]
}

type Address {
    street: String
    city: String
    postalCode: String
    country: String
}

type Transaction {
    id: ID!
    date: Date!
    amount: Int!
    sender: User!
    recipient: User!
}

input NewTransaction {
    sender: ID!
    recipient: ID!
    amount: Int!
}

input AddressInput {
    street: String!
    city: String!
    postalCode: String!
    country: String!
}

input UpdateUserDetailsInput {
    name: String
    address: AddressInput
}

input NewUserInput {
    balance: Int!
    name: String!
    email: String!
}

type Query {
	__schema: __Schema! @nodeCountSkip
    user(id: ID!): User
    users(first: Int! @nodeCountMultiply, afterID: ID): [User]
    transactions(first: Int! @nodeCountMultiply, afterID: ID): [Transaction]
    currentPeriod: String
}

type Mutation {
    createUser(input: NewUserInput!): User
    makeTransaction(input: NewTransaction!): Transaction!
    updateUserDetails(userID: ID!,input: UpdateUserDetailsInput!): User
}

schema {
	query: Query
	mutation: Mutation
}

"The 'Int' scalar type represents non-fractional signed whole numeric values. Int can represent values between -(2^31) and 2^31 - 1."
scalar Int
"The 'Float' scalar type represents signed double-precision fractional values as specified by [IEEE 754](http://en.wikipedia.org/wiki/IEEE_floating_point)."
scalar Float
"The 'String' scalar type represents textual data, represented as UTF-8 character sequences. The String type is most often used by GraphQL to represent free-form human-readable text."
scalar String
"The 'Boolean' scalar type represents 'true' or 'false' ."
scalar Boolean
"The 'ID' scalar type represents a unique identifier, often used to refetch an object or as key for a cache. The ID type appears in a JSON response as a String; however, it is not intended to be human-readable. When expected as an input type, any string (such as '4') or integer (such as 4) input value will be accepted as an ID."
scalar ID @custom(typeName: "string")
"Directs the executor to include this field or fragment only when the argument is true."
directive @include(
    " Included when true."
    if: Boolean!
) on FIELD | FRAGMENT_SPREAD | INLINE_FRAGMENT
"Directs the executor to skip this field or fragment when the argument is true."
directive @skip(
    "Skipped when true."
    if: Boolean!
) on FIELD | FRAGMENT_SPREAD | INLINE_FRAGMENT
"Marks an element of a GraphQL schema as no longer supported."
directive @deprecated(
    """
    Explains why this element was deprecated, usually also including a suggestion
    for how to access supported similar data. Formatted in
    [Markdown](https://daringfireball.net/projects/markdown/).
    """
    reason: String = "No longer supported"
) on FIELD_DEFINITION | ENUM_VALUE

"""
A Directive provides a way to describe alternate runtime execution and type validation behavior in a GraphQL document.
In some cases, you need to provide options to alter GraphQL's execution behavior
in ways field arguments will not suffice, such as conditionally including or
skipping a field. Directives provide this by describing additional information
to the executor.
"""
type __Directive {
    name: String!
    description: String
    locations: [__DirectiveLocation!]!
    args: [__InputValue!]!
}

"""
A Directive can be adjacent to many parts of the GraphQL language, a
__DirectiveLocation describes one such possible adjacencies.
"""
enum __DirectiveLocation {
    "Location adjacent to a query operation."
    QUERY
    "Location adjacent to a mutation operation."
    MUTATION
    "Location adjacent to a subscription operation."
    SUBSCRIPTION
    "Location adjacent to a field."
    FIELD
    "Location adjacent to a fragment definition."
    FRAGMENT_DEFINITION
    "Location adjacent to a fragment spread."
    FRAGMENT_SPREAD
    "Location adjacent to an inline fragment."
    INLINE_FRAGMENT
    "Location adjacent to a schema definition."
    SCHEMA
    "Location adjacent to a scalar definition."
    SCALAR
    "Location adjacent to an object type definition."
    OBJECT
    "Location adjacent to a field definition."
    FIELD_DEFINITION
    "Location adjacent to an argument definition."
    ARGUMENT_DEFINITION
    "Location adjacent to an interface definition."
    INTERFACE
    "Location adjacent to a union definition."
    UNION
    "Location adjacent to an enum definition."
    ENUM
    "Location adjacent to an enum value definition."
    ENUM_VALUE
    "Location adjacent to an input object type definition."
    INPUT_OBJECT
    "Location adjacent to an input object field definition."
    INPUT_FIELD_DEFINITION
}
"""
One possible value for a given Enum. Enum values are unique values, not a
placeholder for a string or numeric value. However an Enum value is returned in
a JSON response as a string.
"""
type __EnumValue {
    name: String!
    description: String
    isDeprecated: Boolean!
    deprecationReason: String
}

"""
Object and Interface types are described by a list of Fields, each of which has
a name, potentially a list of arguments, and a return type.
"""
type __Field {
    name: String!
    description: String
    args: [__InputValue!]!
    type: __Type!
    isDeprecated: Boolean!
    deprecationReason: String
}

"""Arguments provided to Fields or Directives and the input fields of an
InputObject are represented as Input Values which describe their type and
optionally a default value.
"""
type __InputValue {
    name: String!
    description: String
    type: __Type!
    "A GraphQL-formatted string representing the default value for this input value."
    defaultValue: String
}

"""
A GraphQL Schema defines the capabilities of a GraphQL server. It exposes all
available types and directives on the server, as well as the entry points for
query, mutation, and subscription operations.
"""
type __Schema {
    "A list of all types supported by this server."
    types: [__Type!]!
    "The type that query operations will be rooted at."
    queryType: __Type!
    "If this server supports mutation, the type that mutation operations will be rooted at."
    mutationType: __Type
    "If this server support subscription, the type that subscription operations will be rooted at."
    subscriptionType: __Type
    "A list of all directives supported by this server."
    directives: [__Directive!]!
}

"""
The fundamental unit of any GraphQL Schema is the type. There are many kinds of
types in GraphQL as represented by the '__TypeKind' enum.

Depending on the kind of a type, certain fields describe information about that
type. Scalar types provide no information beyond a name and description, while
Enum types provide their values. Object and Interface types provide the fields
they describe. Abstract types, Union and Interface, provide the Object types
possible at runtime. List and NonNull types compose other types.
"""
type __Type {
    kind: __TypeKind!
    name: String
    description: String
    fields(includeDeprecated: Boolean = false): [__Field!]
    interfaces: [__Type!]
    possibleTypes: [__Type!]
    enumValues(includeDeprecated: Boolean = false): [__EnumValue!]
    inputFields: [__InputValue!]
    ofType: __Type
}

"An enum describing what kind of type a given '__Type' is."
enum __TypeKind {
    "Indicates this type is a scalar."
    SCALAR
    "Indicates this type is an object. 'fields' and 'interfaces' are valid fields."
    OBJECT
    "Indicates this type is an interface. 'fields' ' and ' 'possibleTypes' are valid fields."
    INTERFACE
    "Indicates this type is a union. 'possibleTypes' is a valid field."
    UNION
    "Indicates this type is an enum. 'enumValues' is a valid field."
    ENUM
    "Indicates this type is an input object. 'inputFields' is a valid field."
    INPUT_OBJECT
    "Indicates this type is a list. 'ofType' is a valid field."
    LIST
    "Indicates this type is a non-null. 'ofType' is a valid field."
    NON_NULL
}`

const introspectionQuery = `
query IntrospectionQuery {
  __schema {
    queryType {
      name
    }
    mutationType {
      name
    }
    subscriptionType {
      name
    }
    types {
      ...FullType
    }
    directives {
      name
      description
      locations
      args {
        ...InputValue
      }
    }
  }
}

fragment FullType on __Type {
  kind
  name
  description
  fields(includeDeprecated: true) {
    name
    description
    args {
      ...InputValue
    }
    type {
      ...TypeRef
    }
    isDeprecated
    deprecationReason
  }
  inputFields {
    ...InputValue
  }
  interfaces {
    ...TypeRef
  }
  enumValues(includeDeprecated: true) {
    name
    description
    isDeprecated
    deprecationReason
  }
  possibleTypes {
    ...TypeRef
  }
}

fragment InputValue on __InputValue {
  name
  description
  type {
    ...TypeRef
  }
  defaultValue
}

fragment TypeRef on __Type {
  kind
  name
  ofType {
    kind
    name
    ofType {
      kind
      name
      ofType {
        kind
        name
        ofType {
          kind
          name
          ofType {
            kind
            name
            ofType {
              kind
              name
              ofType {
                kind
                name
              }
            }
          }
        }
      }
    }
  }
}`
