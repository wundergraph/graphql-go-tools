package astprinter

import (
	"bytes"
	"io/ioutil"
	"testing"

	"github.com/jensneuse/diffview"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/internal/pkg/unsafeparser"
	"github.com/wundergraph/graphql-go-tools/pkg/operationreport"
	"github.com/wundergraph/graphql-go-tools/pkg/testing/goldie"
)

func TestPrint(t *testing.T) {

	must := func(t *testing.T, err error) {
		t.Helper()
		if report, ok := err.(operationreport.Report); ok {
			if report.HasErrors() {
				t.Fatalf("report has errors %s", report.Error())
			}
		}
		require.NoError(t, err)
	}

	run := func(t *testing.T, raw string, expected string) {
		t.Helper()

		definition := unsafeparser.ParseGraphqlDocumentString(testDefinition)
		doc := unsafeparser.ParseGraphqlDocumentString(raw)

		buff := &bytes.Buffer{}
		// printer := Printer{indent: []byte("  ")}
		printer := Printer{}

		must(t, printer.Print(&doc, &definition, buff))

		actual := buff.String()
		assert.Equal(t, expected, actual)
	}

	t.Run("simple", func(t *testing.T) {
		run(t, "query o($id: String!){user(id: $id){id name birthday}}",
			"query o($id: String!){user(id: $id){id name birthday}}")
	})
	t.Run("complex", func(t *testing.T) {
		run(t, `	
				subscription sub {
					...multipleSubscriptions
				}
				fragment multipleSubscriptions on Subscription {
					... {
						newMessage {
							body
						}
					}
					... on Subscription {
						typedInlineFragment
					}
					newMessage {
						body
						sender
					}
					disallowedSecondRootField
				}`,
			"subscription sub {...multipleSubscriptions} fragment multipleSubscriptions on Subscription {...{newMessage {body}} ... on Subscription {typedInlineFragment} newMessage {body sender} disallowedSecondRootField}")
	})
	t.Run("multiline comments indentation", func(t *testing.T) {
		run(t, `"""
the following lines test indentation
	one tab
  two spaces
		two tabs
no indentation
example from issue:
{
	user(id: 1) {
		userID
		friends
	}
}
"""
type Query`,
			`"""
the following lines test indentation
	one tab
  two spaces
		two tabs
no indentation
example from issue:
{
	user(id: 1) {
		userID
		friends
	}
}
"""
type Query `)
	})
	t.Run("directive definition", func(t *testing.T) {
		run(t, `
"""
directive @cache
"""
directive @cache(
  "maxAge defines the maximum time in seconds a response will be understood 'fresh', defaults to 300 (5 minutes)"
  maxAge: Int! = 300
  """
  vary defines the headers to append to the cache key
  In addition to all possible headers you can also select a custom claim for authenticated requests
  Examples: 'jwt.sub', 'jwt.team' to vary the cache key based on 'sub' or 'team' fields on the jwt. 
  """
  vary: [String]! = []
) on QUERY directive @include(if: Boolean!) repeatable on FIELD
`,
			`"""
directive @cache
"""
directive @cache("maxAge defines the maximum time in seconds a response will be understood 'fresh', defaults to 300 (5 minutes)"
maxAge: Int! = 300 """
vary defines the headers to append to the cache key
In addition to all possible headers you can also select a custom claim for authenticated requests
Examples: 'jwt.sub', 'jwt.team' to vary the cache key based on 'sub' or 'team' fields on the jwt.
"""
vary: [String]! = []) on QUERY directive @include(if: Boolean!) repeatable on FIELD`)
	})
	t.Run("fragment definition with directives", func(t *testing.T) {
		run(t, `
			fragment foo on Dog @fragmentDefinition {
				name
			}
		`, `fragment foo on Dog @fragmentDefinition {name}`)
	})
	t.Run("anonymous query", func(t *testing.T) {
		run(t, `	{
						dog {
							...aliasedLyingFieldTargetNotDefined
						}
					}`, "{dog {...aliasedLyingFieldTargetNotDefined}}")
	})
	t.Run("arguments", func(t *testing.T) {
		run(t, `
				query argOnRequiredArg($catCommand: CatCommand @include(if: true), $complex: Boolean = true) {
					dog {
						doesKnowCommand(dogCommand: $catCommand)
					}
				}`, `query argOnRequiredArg($catCommand: CatCommand @include(if: true), $complex: Boolean = true){dog {doesKnowCommand(dogCommand: $catCommand)}}`)
	})
	t.Run("spacing", func(t *testing.T) {
		run(t, `query($representations: [_Any!]!){_entities (representations: $representations){... on User {reviews {body product {upc __typename}}}}}`,
			`query($representations: [_Any!]!){_entities(representations: $representations){... on User {reviews {body product {upc __typename}}}}}`)
	})
	t.Run("directives", func(t *testing.T) {
		t.Run("on field", func(t *testing.T) {
			run(t, `
			query directivesQuery @foo(bar: BAZ) {
				dog @include(if: true, or: false) {
					doesKnowCommand(dogCommand: $catCommand)
				}
			}`, `query directivesQuery @foo(bar: BAZ) {dog @include(if: true, or: false) {doesKnowCommand(dogCommand: $catCommand)}}`)
		})
		t.Run("on inline fragment", func(t *testing.T) {
			run(t, `
				{
					dog {
						name: nickname
						... @include(if: true) {
							name
						}
					}
					cat {
						name @include(if: true)
						nickname
					}
				}`, `{dog {name: nickname ... @include(if: true){name}} cat {name @include(if: true) nickname}}`)
		})
	})
	t.Run("complex operation", func(t *testing.T) {
		run(t, benchmarkTestOperation, benchmarkTestOperationFlat)
	})
	t.Run("schema definition", func(t *testing.T) {
		run(t, `
				schema {
					query: Query
					mutation: Mutation
					subscription: Subscription
				}`, `schema {query: Query mutation: Mutation subscription: Subscription}`)
	})
	t.Run("schema extension", func(t *testing.T) {
		run(t, `
				extend schema @foo {
					query: Query
					mutation: Mutation
					subscription: Subscription
				}`, `extend schema @foo {query: Query mutation: Mutation subscription: Subscription}`)
	})
	t.Run("object type definition", func(t *testing.T) {
		run(t, `
				type Foo {
					field: String
				}`, `type Foo {field: String}`)
	})
	t.Run("object type extension", func(t *testing.T) {
		run(t, `
				extend type Foo @foo {
					field: String
				}`, `extend type Foo @foo {field: String}`)
	})
	t.Run("input object type definition", func(t *testing.T) {
		run(t, `
				input Foo {
					field: String
					field2: Boolean = true
				}`, `input Foo {field: String field2: Boolean = true}`)
	})
	t.Run("input object type extension", func(t *testing.T) {
		run(t, `
				extend input Foo @foo {
					field: String
				}`, `extend input Foo @foo {field: String}`)
	})
	t.Run("interface type definition", func(t *testing.T) {
		run(t, `
				interface Foo {
					field: String
					field2: Boolean
				}`, `interface Foo {field: String field2: Boolean}`)
	})
	t.Run("interface type extension", func(t *testing.T) {
		run(t, `
				extend interface Foo @foo {
					field: String
				}`, `extend interface Foo @foo {field: String}`)
	})
	t.Run("scalar type definition", func(t *testing.T) {
		run(t, `scalar JSON`, `scalar JSON`)
	})
	t.Run("scalar type extension", func(t *testing.T) {
		run(t, `extend scalar JSON @foo`, `extend scalar JSON @foo`)
	})
	t.Run("union type definition", func(t *testing.T) {
		run(t, `union Foo = BAR | BAZ`, `union Foo = BAR | BAZ`)
	})
	t.Run("union type extension", func(t *testing.T) {
		run(t, `extend union Foo @foo = BAR | BAZ`, `extend union Foo @foo = BAR | BAZ`)
	})
	t.Run("enum type definition", func(t *testing.T) {
		run(t, `
				enum Foo {
					BAR
					BAZ
				}`, `enum Foo {BAR BAZ}`)
	})
	t.Run("enum type extension", func(t *testing.T) {
		run(t, `
				extend enum Foo @foo {
					BAR
					BAZ
				}`, `extend enum Foo @foo {BAR BAZ}`)
	})
	t.Run("multiple operations with variables", func(t *testing.T) {
		run(t, `
				mutation AddToWatchlist($a: Int!, $b: String!){
					addToWatchlist(movieID: $a, name: $b){
						id
						name
						year
					}
				}

				mutation AddWithInput($a: WatchlistInput!){
    				addToWatchlistWithInput(input: $a){
						id
						name
						year
					}
				}`,
			`mutation AddToWatchlist($a: Int!, $b: String!){addToWatchlist(movieID: $a, name: $b){id name year}} mutation AddWithInput($a: WatchlistInput!){addToWatchlistWithInput(input: $a){id name year}}`)
	})

	t.Run("ignore comments", func(t *testing.T) {
		t.Run("operation", func(t *testing.T) {
			run(t, `
				query #comment
				findUser#comment
				(#comment
					$userId#comment
				  :#comment
				  ID#comment
				  !#comment
				  #comment
					)#comment
					{#comment
				  user#comment
					  (#comment
					  id#comment
						:#comment
					$userId#comment
					  #comment
				)#comment
					  #comment
				{#comment
					...#comment
				  UserFields#comment
					... #comment
				  on #comment
				  User#comment
				  {#comment
						email#comment
					}#comment
				  }#comment
				}#comment
				
				fragment #comment
				UserFields #comment
				on #comment
				User#comment
				{#comment
				  id#comment
				  #username#comment
				  role#comment
				}#comment`,
				`query findUser($userId: ID!){user(id: $userId){...UserFields ... on User {email}}} fragment UserFields on User {id role}`)
		})

		t.Run("definition", func(t *testing.T) {
			run(t, `
				#comment
				scalar #comment
				Date #comment
				
				schema #comment
				{ #comment
				  query#comment
				  :#comment
				  #comment
				  Query#comment
				  #comment
				}#comment
				
				#comment
				type#comment
				Query#comment
				{#comment
				  me#comment
				  :#comment
				  User#comment
				  !#comment
				  user(#comment
					id#comment
					:#comment
					ID#comment
					!#comment
				  )#comment
				  :#comment
				  User#comment
				  allUsers#comment
				  :#comment
				  [#comment
					#comment
					User#comment
				  ]#comment
				  search#comment
				  (#comment
					term#comment
					:#comment
					String#comment
					!#comment
				  )#comment
				  :#comment
				  [#comment
					SearchResult#comment
					!#comment
				  ]#comment
				  !#comment
				  myChats:#comment
				  [#comment
					Chat#comment
					!#comment
				  ]!#comment
				}
				
				enum#comment
				Role#comment
				{#comment
				  #comment
				  USER#comment
				  ,#comment
				  ADMIN#comment
				  ,#comment
				  #comment
				}#comment
				
				interface#comment
				Node {#comment
				  id#comment
				  :#comment
				  ID#comment
				  !#comment
				}#comment
				
				union #comment
				SearchResult#comment
				=#comment
				User#comment
				|#comment
				Chat#comment
				|#comment
				ChatMessage#comment
				
				type#comment
				User#comment
				implements#comment
				Node#comment
				{#comment
				  id#comment
				  :#comment
				  ID#comment
				  !#comment
				  username#comment
				  :#comment
				  String#comment
				  !#comment
				  email#comment
				  :#comment
				  String#comment
				  !#comment
				  role#comment
				  :#comment
				  Role#comment
				  !#comment
				}#comment
				
				type#comment
				Chat#comment
				implements#comment
				Node#comment
				{#comment
				  id#comment
				  :#comment
				  ID#comment
				  !#comment
				  users#comment
				  :#comment
				  [#comment
					User#comment
					!#comment
				  ]!#comment
				  messages#comment
				  :#comment
				  [#comment
					ChatMessage#comment
					!#comment
				  ]#comment
				  !#comment
				  #comment
				}#comment
				
				type#comment
				ChatMessage#comment
				implements#comment
				Node#comment
				{#comment
				  id#comment
				  :#comment
				  ID#comment
				  !#comment
				  content#comment
				  :#comment
				  String#comment
				  !#comment
				  time#comment
				  :#comment
				  Date#comment
				  !#comment
				  user#comment
				  :#comment
				  User#comment
				  !#comment
				  #comment
				}#comment`,
				`scalar Date schema {query: Query} type Query {me: User! user(id: ID!): User allUsers: [User] search(term: String!): [SearchResult!]! myChats: [Chat!]!} enum Role {USER ADMIN} interface Node {id: ID!} union SearchResult = User | Chat | ChatMessage type User implements Node {id: ID! username: String! email: String! role: Role!} type Chat implements Node {id: ID! users: [User!]! messages: [ChatMessage!]!} type ChatMessage implements Node {id: ID! content: String! time: Date! user: User!}`)
		})
	})
}

func TestPrintArgumentWithBeforeAfterValue(t *testing.T) {
	doc := unsafeparser.ParseGraphqlDocumentString(`
	mutation ($email: String!) {
	pge_queryRaw(query: "SELECT id, name, email from \"User\" where email = $1", parameters: [$email])
}
`)

	doc.Arguments[1].PrintBeforeValue = []byte("\"")
	doc.Arguments[1].PrintAfterValue = []byte("\"")

	buff := bytes.Buffer{}
	err := Print(&doc, nil, &buff)
	if err != nil {
		t.Fatal(err)
	}

	out := buff.Bytes()
	assert.Equal(t, "mutation($email: String!){pge_queryRaw(query: \"SELECT id, name, email from \\\"User\\\" where email = $1\", parameters: \"[$email]\")}", string(out))
}

func TestPrintSchemaDefinition(t *testing.T) {

	doc := unsafeparser.ParseGraphqlDocumentFile("./testdata/starwars.schema.graphql")

	buff := bytes.Buffer{}
	err := PrintIndent(&doc, nil, []byte("  "), &buff)
	if err != nil {
		t.Fatal(err)
	}

	out := buff.Bytes()

	goldie.Assert(t, "starwars_schema_definition", out)
	if t.Failed() {
		fixture, err := ioutil.ReadFile("./fixtures/starwars_schema_definition.golden")
		if err != nil {
			t.Fatal(err)
		}

		diffview.NewGoland().DiffViewBytes("starwars_schema_definition", fixture, out)
	}
}

func TestPrintOperationDefinition(t *testing.T) {

	schema := unsafeparser.ParseGraphqlDocumentString(testDefinition)
	operation := unsafeparser.ParseGraphqlDocumentFile("./testdata/introspectionquery.graphql")

	buff := bytes.Buffer{}
	err := PrintIndent(&operation, &schema, []byte("  "), &buff)
	if err != nil {
		t.Fatal(err)
	}

	out := buff.Bytes()

	goldie.Assert(t, "introspectionquery", out)
	if t.Failed() {
		fixture, err := ioutil.ReadFile("./fixtures/introspectionquery.golden")
		if err != nil {
			t.Fatal(err)
		}

		diffview.NewGoland().DiffViewBytes("introspectionquery", fixture, out)
	}
}

func BenchmarkPrint(b *testing.B) {

	must := func(err error) {
		if err != nil {
			panic(err)
		}
	}

	def := unsafeparser.ParseGraphqlDocumentString(benchmarkTestDefinition)
	doc := unsafeparser.ParseGraphqlDocumentString(benchmarkTestOperation)

	buff := &bytes.Buffer{}

	printer := Printer{}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		buff.Reset()
		must(printer.Print(&doc, &def, buff))
	}
}

const testDefinition = `
schema {
	query: Query
	subscription: Subscription
}

type Message {
	body: String
	sender: String
}

type Subscription {
	newMessage: Message
	disallowedSecondRootField: Boolean
}

input ComplexInput { name: String, owner: String }
input ComplexNonOptionalInput { name: String! }

type Query {
	human: Human
  	pet: Pet
  	dog: Dog
	cat: Cat
	catOrDog: CatOrDog
	dogOrHuman: DogOrHuman
	humanOrAlien: HumanOrAlien
	arguments: ValidArguments
	findDog(complex: ComplexInput): Dog
	findDogNonOptional(complex: ComplexNonOptionalInput): Dog
  	booleanList(booleanListArg: [Boolean!]): Boolean
	extra: Extra
}

type ValidArguments {
	multipleReqs(x: Int!, y: Int!): Int!
	booleanArgField(booleanArg: Boolean): Boolean
	floatArgField(floatArg: Float): Float
	intArgField(intArg: Int): Int
	nonNullBooleanArgField(nonNullBooleanArg: Boolean!): Boolean!
	booleanListArgField(booleanListArg: [Boolean]!): [Boolean]
	optionalNonNullBooleanArgField(optionalBooleanArg: Boolean! = false): Boolean!
}

enum DogCommand { SIT, DOWN, HEEL }

type Dog implements Pet {
	name: String!
	nickname: String
	barkVolume: Int
	doesKnowCommand(dogCommand: DogCommand!): Boolean!
	isHousetrained(atOtherHomes: Boolean): Boolean!
	owner: Human
	extra: DogExtra
	extras: [DogExtra]
	mustExtra: DogExtra!
	mustExtras: [DogExtra]!
	mustMustExtras: [DogExtra!]!
}

type DogExtra {
	string: String
	strings: [String]
	mustStrings: [String]!
	bool: Int
}

interface Sentient {
  name: String!
}

interface Pet {
  name: String!
}

type Alien implements Sentient {
  name: String!
  homePlanet: String
}

type Human implements Sentient {
  name: String!
}

enum CatCommand { JUMP }

type Cat implements Pet {
	name: String!
	nickname: String
	doesKnowCommand(catCommand: CatCommand!): Boolean!
	meowVolume: Int
	extra: CatExtra
}

type CatExtra {
	string: String
	string2: String
	strings: [String]
	mustStrings: [String]!
	bool: Boolean
}

union CatOrDog = Cat | Dog
union DogOrHuman = Dog | Human
union HumanOrAlien = Human | Alien
union Extra = CatExtra | DogExtra

directive @inline on INLINE_FRAGMENT
directive @spread on FRAGMENT_SPREAD
directive @fragmentDefinition on FRAGMENT_DEFINITION
directive @onQuery on QUERY
directive @onMutation on MUTATION
directive @onSubscription on SUBSCRIPTION

"The Int scalar type represents non-fractional signed whole numeric values. Int can represent values between -(2^31) and 2^31 - 1."
scalar Int
"The Float scalar type represents signed double-precision fractional values as specified by [IEEE 754](http://en.wikipedia.org/wiki/IEEE_floating_point)."
scalar Float
"The String scalar type represents textual data, represented as UTF-8 character sequences. The String type is most often used by GraphQL to represent free-form human-readable text."
scalar String
"The Boolean scalar type represents true or false ."
scalar Boolean
"The ID scalar type represents a unique identifier, often used to refetch an object or as key for a cache. The ID type appears in a JSON response as a String; however, it is not intended to be human-readable. When expected as an input type, any string (such as 4) or integer (such as 4) input value will be accepted as an ID."
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
Object and Interface types are described by a list of FieldSelections, each of which has
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

"""ValidArguments provided to FieldSelections or Directives and the input fields of an
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
types in GraphQL as represented by the __TypeKind enum.

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

"An enum describing what kind of type a given __Type is."
enum __TypeKind {
    "Indicates this type is a scalar."
    SCALAR
    "Indicates this type is an object. fields and interfaces are valid fields."
    OBJECT
    "Indicates this type is an interface. fields  and  possibleTypes are valid fields."
    INTERFACE
    "Indicates this type is a union. possibleTypes is a valid field."
    UNION
    "Indicates this type is an enum. enumValues is a valid field."
    ENUM
    "Indicates this type is an input object. inputFields is a valid field."
    INPUT_OBJECT
    "Indicates this type is a list. ofType is a valid field."
    LIST
    "Indicates this type is a non-null. ofType is a valid field."
    NON_NULL
}`

const benchmarkTestOperation = `
query PostsUserQuery {
	posts {
		id
		description
		user {
			id
			name
		}
	}
}
fragment FirstFragment on Post {
	id
}
query ArgsQuery {
	foo(bar: "barValue", baz: true){
		fooField
	}
}
query VariableQuery($bar: String, $baz: Boolean) {
	foo(bar: $bar, baz: $baz){
		fooField
	}
}
query VariableQuery {
	posts {
		id @include(if: true)
		user
	}
}
`

const benchmarkTestOperationFlat = `query PostsUserQuery {posts {id description user {id name}}} fragment FirstFragment on Post {id} query ArgsQuery {foo(bar: "barValue", baz: true){fooField}} query VariableQuery($bar: String, $baz: Boolean){foo(bar: $bar, baz: $baz){fooField}} query VariableQuery {posts {id @include(if: true) user}}`

const benchmarkTestDefinition = `
directive @include(if: Boolean!) on FIELD | FRAGMENT_SPREAD | INLINE_FRAGMENT
schema {
	query: Query
}
type Query {
	posts: [Post]
	foo(bar: String!, baz: Boolean!): Foo
}
type User {
	id: ID
	name: String
}
type Post {
	id: ID
	description: String
	user: User
}
type Foo {
	fooField: String
}
scalar ID
scalar String
`
