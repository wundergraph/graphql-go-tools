package astprinter

import (
	"bytes"
	"os"
	"testing"

	"github.com/jensneuse/diffview"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/internal/unsafeparser"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/operationreport"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/testing/goldie"
)

func must(t *testing.T, err error) {
	t.Helper()
	if report, ok := err.(operationreport.Report); ok {
		if report.HasErrors() {
			t.Fatalf("report has errors %s", report.Error())
		}
	}
	require.NoError(t, err)
}

func runWithIndent(t *testing.T, raw string, expected string, indent bool) {
	t.Helper()

	doc := unsafeparser.ParseGraphqlDocumentString(raw)

	buff := &bytes.Buffer{}
	printer := Printer{}

	if indent {
		printer.indent = []byte("    ")
	}

	must(t, printer.Print(&doc, buff))

	actual := buff.String()
	assert.Equal(t, expected, actual)
}

func runIndent(t *testing.T, raw string, expected string) {
	runWithIndent(t, raw, expected, true)
}

func run(t *testing.T, raw string, expected string) {
	runWithIndent(t, raw, expected, false)
}

func TestPrint(t *testing.T) {
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
		t.Run("no indentation", func(t *testing.T) {
			t.Run("on field with selections", func(t *testing.T) {
				run(t, `
					query directivesQuery @foo(bar: BAZ) {
						dog @include(if: true, or: false) {
							doesKnowCommand(dogCommand: $catCommand)
						}
					}`, `query directivesQuery @foo(bar: BAZ) {dog @include(if: true, or: false) {doesKnowCommand(dogCommand: $catCommand)}}`)
			})
			t.Run("on field with selections and selections after", func(t *testing.T) {
				run(t, `
					query directivesQuery @foo(bar: BAZ) {
						dog @include(if: true, or: false) {
							doesKnowCommand(dogCommand: $catCommand)
						}
						anotherField
					}`, `query directivesQuery @foo(bar: BAZ) {dog @include(if: true, or: false) {doesKnowCommand(dogCommand: $catCommand)} anotherField}`)
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
			t.Run("on fragment spread", func(t *testing.T) {
				run(t, `
					{
						dog {
							...NameFragment @include(if: true)
						}
					}
					fragment NameFragment on Dog {
						name
					}
					`, `{dog {...NameFragment @include(if: true)}} fragment NameFragment on Dog {name}`)
			})
		})

		t.Run("with indentation", func(t *testing.T) {
			t.Run("on field with selections", func(t *testing.T) {
				runIndent(t, `
					query directivesQuery @foo(bar: BAZ) {
						dog @include(if: true, or: false) {
							doesKnowCommand(dogCommand: $catCommand)
						}
					}`,
					`query directivesQuery @foo(bar: BAZ) {
    dog @include(if: true, or: false) {
        doesKnowCommand(dogCommand: $catCommand)
    }
}`)
			})

			t.Run("on field with selections and selections after", func(t *testing.T) {
				runIndent(t, `
					query directivesQuery @foo(bar: BAZ) {
						dog @include(if: true, or: false) {
							doesKnowCommand(dogCommand: $catCommand)
						}
						anotherField
					}`,
					`query directivesQuery @foo(bar: BAZ) {
    dog @include(if: true, or: false) {
        doesKnowCommand(dogCommand: $catCommand)
    }
    anotherField
}`)
			})
			t.Run("on field without selections", func(t *testing.T) {
				runIndent(t, `
					{
						cat {
							name @include(if: true)
							nickname
						}
					}`,
					`{
    cat {
        name @include(if: true)
        nickname
    }
}`)
			})

			t.Run("on inline fragment", func(t *testing.T) {
				runIndent(t, `
					{
						dog {
							... @include(if: true) {
								name
							}
						}
					}`,
					`{
    dog {
        ... @include(if: true) {
            name
        }
    }
}`)
			})

			t.Run("on inline fragment and selections after", func(t *testing.T) {
				runIndent(t, `
					{
						dog {
							... @include(if: true) {
								name
							}
							name: nickname
						}
					}`,
					`{
    dog {
        ... @include(if: true) {
            name
        }
        name: nickname
    }
}`)

			})

			t.Run("on fragment spread", func(t *testing.T) {
				runIndent(t, `
				{
					dog {
						...NameFragment @include(if: true)
					}
				}
				fragment NameFragment on Dog {
					name
				}
				`, `{
    dog {
        ...NameFragment @include(if: true)
    }
}

fragment NameFragment on Dog {
    name
}`)
			})

			t.Run("on fragment spread and selections after", func(t *testing.T) {
				runIndent(t, `
				{
					dog {
						...NameFragment @include(if: true)
						otherField
					}
				}
				fragment NameFragment on Dog {
					name
				}
				`, `{
    dog {
        ...NameFragment @include(if: true)
        otherField
    }
}

fragment NameFragment on Dog {
    name
}`)
			})

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

	t.Run("schema extension only directives", func(t *testing.T) {
		run(t, `extend schema @foo `, `extend schema @foo `)
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
	t.Run("transitive interfaces", func(t *testing.T) {
		run(t, "interface I1 {id: ID!} interface I2 implements I1 {id: ID!} interface I3 implements I1 & I2 {id: ID!}",
			"interface I1 {id: ID!} interface I2 implements I1 {id: ID!} interface I3 implements I1 & I2 {id: ID!}")
	})
	t.Run("operation with description", func(t *testing.T) {
		t.Run("block string description", func(t *testing.T) {
			run(t, `"""
This is a query description
"""
query GetUser {
	user {
		id
		name
	}
}`, `"""
This is a query description
"""
query GetUser {user {id name}}`)
		})
		t.Run("single line description", func(t *testing.T) {
			run(t, `"This is a mutation description"
mutation CreateUser {
	createUser {
		id
	}
}`, `"This is a mutation description"
mutation CreateUser {createUser {id}}`)
		})
		t.Run("subscription with description", func(t *testing.T) {
			run(t, `"""
Subscribe to new messages
"""
subscription OnNewMessage {
	newMessage {
		body
	}
}`, `"""
Subscribe to new messages
"""
subscription OnNewMessage {newMessage {body}}`)
		})
		t.Run("anonymous query without description", func(t *testing.T) {
			run(t, `{
	user {
		id
	}
}`, `{user {id}}`)
		})
		t.Run("operation with description and variables", func(t *testing.T) {
			run(t, `"Get user by ID"
query GetUser($id: ID!) {
	user(id: $id) {
		id
		name
	}
}`, `"Get user by ID"
query GetUser($id: ID!){user(id: $id){id name}}`)
		})
		t.Run("operation with description and directives", func(t *testing.T) {
			run(t, `"""
Query with directive
"""
query GetUser @cached {
	user {
		id
	}
}`, `"""
Query with directive
"""
query GetUser @cached {user {id}}`)
		})
	})
	t.Run("fragment with description", func(t *testing.T) {
		t.Run("block string description", func(t *testing.T) {
			run(t, `"""
User fields fragment
"""
fragment UserFields on User {
	id
	name
	email
}`, `"""
User fields fragment
"""
fragment UserFields on User {id name email}`)
		})
		t.Run("single line description", func(t *testing.T) {
			run(t, `"Basic user info"
fragment BasicUser on User {
	id
	name
}`, `"Basic user info"
fragment BasicUser on User {id name}`)
		})
		t.Run("fragment without description", func(t *testing.T) {
			run(t, `fragment UserFields on User {
	id
	name
}`, `fragment UserFields on User {id name}`)
		})
		t.Run("fragment with description and directives", func(t *testing.T) {
			run(t, `"""
Fragment with directive
"""
fragment UserFields on User @fragmentDefinition {
	id
	name
}`, `"""
Fragment with directive
"""
fragment UserFields on User @fragmentDefinition {id name}`)
		})
	})
	t.Run("mixed operations and fragments with descriptions", func(t *testing.T) {
		run(t, `"Get user query"
query GetUser {
	user {
		...UserFields
	}
}

"""
User fields fragment
"""
fragment UserFields on User {
	id
	name
}`, `"Get user query"
query GetUser {user {...UserFields}} """
User fields fragment
"""
fragment UserFields on User {id name}`)
	})
	t.Run("variable descriptions", func(t *testing.T) {
		t.Run("single-line description", func(t *testing.T) {
			run(t, `query GetUser("The user ID" $id: ID!) {
	user(id: $id) {
		id
	}
}`, `query GetUser("The user ID" $id: ID!){user(id: $id){id}}`)
		})
		t.Run("block string description", func(t *testing.T) {
			run(t, `query GetUser("""The unique identifier""" $id: ID!) {
	user(id: $id) {
		id
	}
}`, `query GetUser("""
The unique identifier
""" $id: ID!){user(id: $id){id}}`)
		})
		t.Run("multiple variables with mixed descriptions", func(t *testing.T) {
			run(t, `query Search("The search query" $query: String!, $limit: Int) {
	search(query: $query, limit: $limit) {
		id
	}
}`, `query Search("The search query" $query: String!, $limit: Int){search(query: $query, limit: $limit){id}}`)
		})
		t.Run("without description unchanged", func(t *testing.T) {
			run(t, `query GetUser($id: ID!) {
	user(id: $id) {
		id
	}
}`, `query GetUser($id: ID!){user(id: $id){id}}`)
		})
		t.Run("single-line operation with variable description", func(t *testing.T) {
			run(t, `query GetUser("The user ID" $id: ID!) { user(id: $id) { id } }`,
				`query GetUser("The user ID" $id: ID!){user(id: $id){id}}`)
		})
	})
}

func TestPrintDescriptionRoundTrip(t *testing.T) {
	t.Run("preserves inner indentation of multi-line field descriptions", func(t *testing.T) {
		runIndent(t, `type Query {
  """
  An example query:

      query {
        users {
          id
          name
        }
      }
  """
  example: String
}`, `type Query {
    """
    An example query:

    query {
      users {
        id
        name
      }
    }
    """
    example: String
}`)
	})

	t.Run("strips only common indent in multi-line description", func(t *testing.T) {
		runIndent(t, `type Query {
  """
  Outer.
      Deep.
  Outer again.
  """
  example: String
}`, `type Query {
    """
    Outer.
        Deep.
    Outer again.
    """
    example: String
}`)
	})
}

func TestPrintFieldArgsWithDescriptions(t *testing.T) {
	t.Run("inline when no arg has a description", func(t *testing.T) {
		runIndent(t, `type Query { foo(a: Int, b: String): Int }`,
			`type Query {
    foo(a: Int, b: String): Int
}`)
	})

	t.Run("breaks across lines when any arg has a description", func(t *testing.T) {
		runIndent(t, `type Query {
  foo(
    "Limit on results."
    limit: Int = 25

    "Page number."
    page: Int = 1
  ): String
}`, `type Query {
    foo(
        "Limit on results."
        limit: Int = 25
        "Page number."
        page: Int = 1
    ): String
}`)
	})

	t.Run("round-trips byte-identically across two parse-print cycles", func(t *testing.T) {
		raw := `type Query {
  foo(
    "Limit on results."
    limit: Int = 25
    "Page number."
    page: Int = 1
  ): String
}`
		first, err := PrintStringIndent(parseDoc(t, raw), "    ")
		require.NoError(t, err)
		second, err := PrintStringIndent(parseDoc(t, first), "    ")
		require.NoError(t, err)
		assert.Equal(t, first, second)
	})

	t.Run("stays inline in compact mode even with descriptions", func(t *testing.T) {
		run(t, `type Query {
  foo(
    "Limit"
    limit: Int = 25
  ): String
}`, `type Query {foo("Limit"
limit: Int = 25): String}`)
	})
}

func parseDoc(t *testing.T, raw string) *ast.Document {
	t.Helper()
	doc := unsafeparser.ParseGraphqlDocumentString(raw)
	return &doc
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
	err := Print(&doc, &buff)
	if err != nil {
		t.Fatal(err)
	}

	out := buff.Bytes()
	assert.Equal(t, "mutation($email: String!){pge_queryRaw(query: \"SELECT id, name, email from \\\"User\\\" where email = $1\", parameters: \"[$email]\")}", string(out))
}

func TestPrintSchemaDefinition(t *testing.T) {

	doc := unsafeparser.ParseGraphqlDocumentFile("./testdata/starwars.schema.graphql")

	buff := bytes.Buffer{}
	err := PrintIndent(&doc, []byte("    "), &buff)
	if err != nil {
		t.Fatal(err)
	}

	out := buff.Bytes()

	goldie.Assert(t, "starwars_schema_definition", out)
	if t.Failed() {
		fixture, err := os.ReadFile("./fixtures/starwars_schema_definition.golden")
		if err != nil {
			t.Fatal(err)
		}

		diffview.NewGoland().DiffViewBytes("starwars_schema_definition", fixture, out)
	}
}

func TestPrintOperationDefinition(t *testing.T) {

	operation := unsafeparser.ParseGraphqlDocumentFile("./testdata/introspectionquery.graphql")

	buff := bytes.Buffer{}
	err := PrintIndent(&operation, []byte("    "), &buff)
	if err != nil {
		t.Fatal(err)
	}

	out := buff.Bytes()

	goldie.Assert(t, "introspectionquery", out)
	if t.Failed() {
		fixture, err := os.ReadFile("./fixtures/introspectionquery.golden")
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

	doc := unsafeparser.ParseGraphqlDocumentString(benchmarkTestOperation)

	buff := &bytes.Buffer{}

	printer := Printer{}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		buff.Reset()
		must(printer.Print(&doc, buff))
	}
}

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
