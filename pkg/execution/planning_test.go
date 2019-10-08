package execution

import (
	"encoding/json"
	"fmt"
	"github.com/davecgh/go-spew/spew"
	"github.com/jensneuse/diffview"
	"github.com/jensneuse/graphql-go-tools/internal/pkg/unsafeparser"
	"github.com/jensneuse/graphql-go-tools/pkg/lexer/literal"
	"github.com/jensneuse/graphql-go-tools/pkg/operationreport"
	"github.com/pkg/errors"
	"math/rand"
	"reflect"
	"testing"
	"time"
)

func init() {
	rand.Seed(time.Now().Unix())
}

func TestPlanner_Plan(t *testing.T) {
	run := func(definition string, operation string, resolverDefinitions ResolverDefinitions, want Node) func(t *testing.T) {
		return func(t *testing.T) {
			def := unsafeparser.ParseGraphqlDocumentString(definition)
			op := unsafeparser.ParseGraphqlDocumentString(operation)

			planner := NewPlanner(resolverDefinitions)
			var report operationreport.Report
			got := planner.Plan(&op, &def, &report)
			if report.HasErrors() {
				t.Fatal(report)
			}

			if !reflect.DeepEqual(want, got) {
				diffview.NewGoland().DiffViewAny("diff", want, got)
				t.Fatalf("want:\n%s\ngot:\n%s\n", spew.Sdump(want), spew.Sdump(got))
			}
		}
	}

	t.Run("introspection type query", run(withBaseSchema(complexSchema), `
				query TypeQuery($name: String! = "User") {
					__type(name: $name) {
						name
						fields {
							name
							type {
								name
							}
						}
					}
				}
`, ResolverDefinitions{
		{
			TypeName:  literal.QUERY,
			FieldName: literal.UNDERSCORETYPE,
			Resolver:  &TypeResolver{},
		},
	}, &Object{
		Fields: []Field{
			{
				Name: []byte("data"),
				Value: &Object{
					Fields: []Field{
						{
							Name: []byte("__type"),
							Resolve: &Resolve{
								Args: []Argument{
									&ContextVariableArgument{
										Name:         []byte("name"),
										VariableName: []byte("name"),
									},
								},
								Resolver: &TypeResolver{},
							},
							Value: &Object{
								Path: []string{"__type"},
								Fields: []Field{
									{
										Name: []byte("name"),
										Value: &Value{
											Path: []string{"name"},
										},
									},
									{
										Name: []byte("fields"),
										Value: &List{
											Path: []string{"fields"},
											Value: &Object{
												Fields: []Field{
													{
														Name: []byte("name"),
														Value: &Value{
															Path: []string{"name"},
														},
													},
													{
														Name: []byte("type"),
														Value: &Object{
															Path: []string{"type"},
															Fields: []Field{
																{
																	Name: []byte("name"),
																	Value: &Value{
																		Path: []string{"name"},
																	},
																},
															},
														},
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}))
	t.Run("graphql resolver", run(withBaseSchema(complexSchema), `
			query TypeQuery($id: String!) {
				user(id: $id) {
					id
					name
					birthday
				}
			}`,
		ResolverDefinitions{
			{
				TypeName:  literal.QUERY,
				FieldName: []byte("user"),
				Resolver: &GraphQLResolver{
					Upstream: "localhost:8001",
					URL:      "/graphql",
				},
			},
		},
		&Object{
			Fields: []Field{
				{
					Name: []byte("data"),
					Value: &Object{
						Fields: []Field{
							{
								Name: []byte("user"),
								Resolve: &Resolve{
									Args: []Argument{
										&StaticVariableArgument{
											Name:  literal.QUERY,
											Value: []byte("query o($id: String!){user(id: $id){id name birthday}}"),
										},
										&ContextVariableArgument{
											Name:         []byte("id"),
											VariableName: []byte("id"),
										},
									},
									Resolver: &GraphQLResolver{
										Upstream: "localhost:8001",
										URL:      "/graphql",
									},
								},
								Value: &Object{
									Path: []string{"user"},
									Fields: []Field{
										{
											Name: []byte("id"),
											Value: &Value{
												Path: []string{"id"},
											},
										},
										{
											Name: []byte("name"),
											Value: &Value{
												Path: []string{"name"},
											},
										},
										{
											Name: []byte("birthday"),
											Value: &Value{
												Path: []string{"birthday"},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		}))
}

func BenchmarkPlanner_Plan(b *testing.B) {
	def := unsafeparser.ParseGraphqlDocumentString(withBaseSchema(complexSchema))
	op := unsafeparser.ParseGraphqlDocumentString(`query TypeQuery($name: String! = "User") {
					__type(name: $name) {
						name
						fields {
							name
							type {
								name
							}
						}
					}
				}`)

	resolverDefinitions := ResolverDefinitions{
		{
			TypeName:  literal.QUERY,
			FieldName: literal.UNDERSCORETYPE,
			Resolver:  &TypeResolver{},
		},
	}

	planner := NewPlanner(resolverDefinitions)
	var report operationreport.Report

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		plan := planner.Plan(&op, &def, &report)
		if plan.Kind() != ObjectKind {
			b.Errorf("plan.Kind() != ObjectKind")
		}
	}
}

const complexExample = `
query TypeQuery($name: String! = "User", $id: String!) {
	__type(name: $name) {
		name
		fields {
			name
			type {
				name
			}
		}
	}
	user(id: $id) {
		id
		name
		birthday
		friends {
			id
			name
			birthday
		}
		pets {
			...petsFragment
		}
	}
	pets {
		...petsFragment
	}
}
fragment petsFragment on Pet {
	__typename
	name
	nickname
	... on Dog {
		woof
	}
	... on Cat {
		meow
	}
}`

const complexSchema = `
directive @resolveGraphQL (
	upstream: String!
	url: String!
	field: String!
	params: [Parameter]
) on FIELD_DEFINITION

directive @resolveREST (
	upstream: String!
	url: String!
	method: HTTP_METHOD = GET
	params: [Parameter]
	mappings: [Mapping]
) on FIELD_DEFINITION

input Mapping {
	from: String!
	to: String!
}

enum HTTP_METHOD {
	GET
	POST
	UPDATE
	DELETE
}

input Parameter {
	name: String!
	source: PARAMETER_SOURCE
	sourceName: String!
}

enum PARAMETER_SOURCE {
	CONTEXT_VARIABLE
	PARENT_FIELD_VALUE
	FIELD_ARGUMENTS
}

scalar Date

schema {
	query: Query
}

type Query {
	__type(name: String!): __Type!
	__schema: __Schema!
	user(id: String!): User
		@resolveGraphQL(
			upstream: "localhost:8001"
			url: "/graphql"
			field: "user"
			params: [
				{
					name: "id"
					source: FIELD_ARGUMENTS
					sourceName: "id"
				}
			]
		)
}
type User {
	id: String
	name: String
	birthday: Date
	friends: [User]
		@resolveREST(
			upstream: "localhost:9000"
			url: "/user/:id/friends"
			params: [
				{
					name: "id"
					source: PARENT_FIELD_VALUE
					sourceName: "id"
				}
			]
			mappings: [
				{from: "id" to: "id"},
				{from: "name" to: "name"},
				{from: "birthday" to: "birthday"}
			]
		)
	pets: [Pet]
		@resolveGraphQL(
			upstream: "localhost:8002"
			url: "/graphql"
			field: "userPets"
			params: [
				{
					name: "userId"
					source: PARENT_FIELD_VALUE
					sourceName: "id"
				}
			]
		)
}
interface Pet {
	nickname: String!
}
type Dog implements Pet {
	name: String!
	nickname: String!
	woof: String!
}
type Cat implements Pet {
	name: String!
	nickname: String!
	meow: String!
}
`

func ensureJsonEqualsPretty(want, got string) {
	wantPretty := pretty(want)
	gotPretty := pretty(got)
	if wantPretty != gotPretty {
		panic(fmt.Errorf(`ensureJsonEqualsPretty:
want:
%s

got:
%s
`, wantPretty, gotPretty))
	}
}

func pretty(input string) string {
	data := map[string]interface{}{}
	err := json.Unmarshal([]byte(input), &data)
	if err != nil {
		panic(errors.WithMessage(err, fmt.Sprintf("input: %s", input)))
	}

	pretty, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		panic(err)
	}
	return string(pretty)
}

func withBaseSchema(input string) string {
	return input + `
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
ObjectKind and Interface types are described by a list of Fields, each of which has
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
Enum types provide their values. ObjectKind and Interface types provide the fields
they describe. Abstract types, Union and Interface, provide the ObjectKind types
possible at runtime. ListKind and NonNull types compose other types.
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
}
`
}

var letterRunes = []byte("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")

func randBytes(n int) []byte {
	b := make([]byte, n)
	for i := range b {
		b[i] = letterRunes[rand.Intn(len(letterRunes))]
	}
	return b
}
