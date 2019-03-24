package printer

import (
	"bufio"
	"bytes"
	"fmt"
	"github.com/jensneuse/graphql-go-tools/pkg/lookup"
	"github.com/jensneuse/graphql-go-tools/pkg/parser"
	"github.com/sebdah/goldie"
	"io"
	"testing"
)

func TestPrinter(t *testing.T) {

	run := func(input string) {

		inputBytes := []byte(input)

		p := parser.NewParser()
		err := p.ParseExecutableDefinition(inputBytes)
		if err != nil {
			panic(err)
		}

		l := lookup.New(p)
		l.SetParser(p)

		w := lookup.NewWalker(1024, 8)
		w.SetLookup(l)
		w.WalkExecutable()

		printer := New()
		printer.SetInput(p, l, w)

		buff := bytes.Buffer{}
		out := bufio.NewWriter(&buff)
		printer.PrintExecutableSchema(out)
		if printer.err != nil {
			panic(printer.err)
		}

		err = out.Flush()
		if err != nil {
			panic(err)
		}

		printedBytes := buff.Bytes()
		if !bytes.Equal(printedBytes, inputBytes) {
			panic(fmt.Errorf("want:\n\n%s\n\ngot:\n\n%s\n", string(inputBytes), string(printedBytes)))
		}
	}

	t.Run("single field", func(t *testing.T) {
		run("{foo}")
	})
	t.Run("query prefix", func(t *testing.T) {
		run("query MyQuery {foo}")
	})
	t.Run("mutation prefix", func(t *testing.T) {
		run("mutation MyQuery {foo}")
	})
	t.Run("subscription prefix", func(t *testing.T) {
		run("subscription MyQuery {foo}")
	})
	t.Run("two fields", func(t *testing.T) {
		run("{foo bar}")
	})
	t.Run("field with subselection", func(t *testing.T) {
		run("{foo {bar}}")
	})
	t.Run("fields with spread and inline", func(t *testing.T) {
		run("{foo {bar {bat ...bal ...{bak}}} baz}")
	})
	t.Run("inline fragment with type condition", func(t *testing.T) {
		run("{foo ...on Bar{baz}}")
	})
	t.Run("inline fragment with type condition and directive", func(t *testing.T) {
		run("{foo ...on Bar @foo @bar(baz:\"bat\" bal:\"bar\"){baz}}")
	})
	t.Run("field with fragment spread", func(t *testing.T) {
		run("{foo ...Bar}")
	})
	t.Run("field with fragment spread and directive", func(t *testing.T) {
		run("{foo ...Bar @foo}")
	})
	t.Run("complex", func(t *testing.T) {
		run("{foo bar ...{baz} ...Bal ...on Bar{bat bar} bart}")
	})
	t.Run("field with arguments", func(t *testing.T) {
		run("{assets(first:1) noArgField}")
	})
	t.Run("null arg", func(t *testing.T) {
		run("{assets(first:null)}")
	})
	t.Run("enum arg", func(t *testing.T) {
		run("{assets(first:ENUM)}")
	})
	t.Run("true arg", func(t *testing.T) {
		run("{assets(first:true)}")
	})
	t.Run("false arg", func(t *testing.T) {
		run("{assets(first:false)}")
	})
	t.Run("integer arg", func(t *testing.T) {
		run("{assets(first:1337)}")
	})
	t.Run("float arg", func(t *testing.T) {
		run("{assets(first:13.37)}")
	})
	t.Run("string arg", func(t *testing.T) {
		run("{assets(first:\"foo\")}")
	})
	t.Run("variable arg", func(t *testing.T) {
		run("{assets(first:$foo)}")
	})
	t.Run("object arg", func(t *testing.T) {
		run("{assets(first:{foo:\"bar\",baz:1})}")
	})
	t.Run("list arg", func(t *testing.T) {
		run("{assets(first:[1,3,3,7])}")
	})
	t.Run("fragment definition", func(t *testing.T) {
		run("fragment MyFragment on Dog {foo bar}")
	})
	t.Run("fragment definition with directive", func(t *testing.T) {
		run("fragment MyFragment on Dog @foo @bar(baz:\"bat\") {foo bar}")
	})
	t.Run("multiple fragment definitions", func(t *testing.T) {
		run("fragment MyFragment on Dog {foo bar}\nfragment MyFragment on Dog {foo bar}")
	})
	t.Run("directive on query", func(t *testing.T) {
		run("query mQuery @foo(bar:\"baz\") {bat}")
	})
	t.Run("multiple directives on query", func(t *testing.T) {
		run("query mQuery @foo(bar:\"baz\") @foo2 {bat}")
	})
	t.Run("directive on field", func(t *testing.T) {
		run("{foo @bar(baz:\"bat\")}")
	})
	t.Run("multiple directive on field", func(t *testing.T) {
		run("{foo @bar(baz:\"bat\") @foo2}")
	})
}

func TestPrinter_Regression(t *testing.T) {

	type action func(printer *Printer, out io.Writer)
	type walk func(w *lookup.Walker)
	type parse func(p *parser.Parser, input []byte)

	parseTypeSystemDefinition := func(p *parser.Parser, input []byte) {
		if err := p.ParseTypeSystemDefinition(input); err != nil {
			panic(err)
		}
	}

	parseExecutableDefinition := func(p *parser.Parser, input []byte) {
		if err := p.ParseExecutableDefinition(input); err != nil {
			panic(err)
		}
	}

	walkExecutable := func(w *lookup.Walker) {
		w.WalkExecutable()
	}

	walkTypeSystemDefinition := func(w *lookup.Walker) {
		w.WalkTypeSystemDefinition()
	}

	printExecutableSchema := func(printer *Printer, out io.Writer) {
		printer.PrintExecutableSchema(out)
	}

	printTypeSystemDefinition := func(printer *Printer, out io.Writer) {
		printer.PrintTypeSystemDefinition(out)
	}

	run := func(input, name string, parse parse, walk walk, action action) {
		inputBytes := []byte(input)

		p := parser.NewParser()
		parse(p, inputBytes)

		l := lookup.New(p)
		w := lookup.NewWalker(1024, 8)
		w.SetLookup(l)
		walk(w)

		printer := New()
		printer.SetInput(p, l, w)

		buff := bytes.Buffer{}
		out := bufio.NewWriter(&buff)
		action(printer, out)
		if printer.err != nil {
			panic(printer.err)
		}

		err := out.Flush()
		if err != nil {
			panic(err)
		}

		printedBytes := buff.Bytes()
		goldie.Assert(t, name, printedBytes)
	}

	t.Run("introspection", func(t *testing.T) {
		run(introspectionQuery, "introspection", parseExecutableDefinition, walkExecutable, printExecutableSchema)
	})
	t.Run("starwars_typesystem", func(t *testing.T) {
		run(starwarsSchema, "starwars_typesystem", parseTypeSystemDefinition, walkTypeSystemDefinition, printTypeSystemDefinition)
	})
}

func BenchmarkPrinter_PrintExecutableSchema(b *testing.B) {

	inputBytes := []byte("{foo bar ...{baz} ...Bal ...on Bar{bat bar} bart assets(first:{foo:\"bar\",baz:1}) assets(first:[1,3,3,7]) assets(first:null)}\nfragment MyFrag on Dog {foo bar}")

	p := parser.NewParser()
	err := p.ParseExecutableDefinition(inputBytes)
	if err != nil {
		panic(err)
	}

	printer := New()

	buff := bytes.Buffer{}
	bufOut := bufio.NewWriter(&buff)

	l := lookup.New(p)
	w := lookup.NewWalker(1024, 8)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {

		w.SetLookup(l)
		w.WalkExecutable()

		printer.SetInput(p, l, w)
		printer.PrintExecutableSchema(bufOut)
		if printer.err != nil {
			panic(printer.err)
		}

		if err := bufOut.Flush(); err != nil {
			panic(err)
		}

		printedBytes := buff.Bytes()
		if !bytes.Equal(printedBytes, inputBytes) {
			panic(fmt.Errorf("want:\n\n%s\n\ngot:\n\n%s\n", string(inputBytes), string(printedBytes)))
		}

		buff.Reset()
		bufOut.Reset(&buff)
	}
}

var introspectionQuery = `query IntrospectionQuery {
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

var starwarsSchema = `
schema {
    query: Query
    mutation: Mutation
    subscription: Subscription
}

"The query type, represents all of the entry points into our object graph"
type Query @directiveOnObject {
	hero(episode: Episode): Character @directiveOnField @directiveOnField2(with: "argument")
	reviews(episode: Episode!): [Review]
	search(text: String): [SearchResult]
	character(id: ID!): Character
	droid(id: ID!): Droid
	human(id: ID!): Human
	starship(id: ID!): Starship
}

"The mutation type, represents all updates we can make to our data"
type Mutation {
	createReview(episode: Episode review: ReviewInput!): Review
}

"The subscription type, represents all subscriptions we can make to our data"
type Subscription {
	reviewAdded(episode: Episode): Review
}

"The episodes in the Star Wars trilogy"
enum Episode @directiveOnEnum {
	"Star Wars Episode IV: A New Hope, released in 1977."
	NEWHOPE
	"Star Wars Episode V: The Empire Strikes Back, released in 1980."
	EMPIRE
	"Star Wars Episode VI: Return of the Jedi, released in 1983."
	JEDI
}

"A character from the Star Wars universe"
interface Character @directiveOnInterface {
	"The ID of the character"
	id: ID!
	"The name of the character"
	name: String!
	"The friends of the character, or an empty list if they have none"
	friends: [Character]
	"The friends of the character exposed as a connection with edges"
	friendsConnection(first: Int after: ID): FriendsConnection!
	"The movies this character appears in"
	appearsIn: [Episode]!
}

"Units of height"
enum LengthUnit {
	"The standard unit around the world"
	METER
	"Primarily used in the United States"
	FOOT
}

"A humanoid creature from the Star Wars universe"
type Human {
	"The ID of the human"
	id: ID!
	"What this human calls themselves"
	name: String!
	"The home planet of the human, or null if unknown"
	homePlanet: String
	"Height in the preferred unit, default is meters"
	height(unit: LengthUnit = METER): Float
	"Mass in kilograms, or null if unknown"
	mass: Float
	"This human's friends, or an empty list if they have none"
	friends: [Character]
	"The friends of the human exposed as a connection with edges"
	friendsConnection(first: Int after: ID): FriendsConnection!
	"The movies this human appears in"
	appearsIn: [Episode]!
	"A list of starships this person has piloted, or an empty list if none"
	starships: [Starship]
}

"An autonomous mechanical character in the Star Wars universe"
type Droid {
	"The ID of the droid"
	id: ID!
	"What others call this droid"
	name: String!
	"This droid's friends, or an empty list if they have none"
	friends: [Character]
	"The friends of the droid exposed as a connection with edges"
	friendsConnection(first: Int after: ID @directiveOnArgument): FriendsConnection!
	"The movies this droid appears in"
	appearsIn: [Episode]!
	"This droid's primary function"
	primaryFunction: String
}

"A connection object for a character's friends"
type FriendsConnection {
	"The total number of friends"
	totalCount: Int
	"The edges for each of the character's friends."
	edges: [FriendsEdge]
	"A list of the friends, as a convenience when edges are not needed."
	friends: [Character]
	"Information for paginating this connection"
	pageInfo: PageInfo!
}

"An edge object for a character's friends"
type FriendsEdge {
	"A cursor used for pagination"
	cursor: ID!
	"The character represented by this friendship edge"
	node: Character
}

"Information for paginating this connection"
type PageInfo {
	startCursor: ID
	endCursor: ID
	hasNextPage: Boolean!
}

"Represents a review for a movie"
type Review {
	"The movie"
	episode: Episode
	"The number of stars this review gave, 1-5"
	stars: Int!
	"Comment about the movie"
	commentary: String
}

"The input object sent when someone is creating a new review"
input ReviewInput {
	"0-5 stars"
	stars: Int!
	"Comment about the movie, optional"
	commentary: String
	"Favorite color, optional"
	favorite_color: ColorInput @directiveOnInputField
}

"The input object sent when passing in a color"
input ColorInput {
	red: Int!
	green: Int!
	blue: Int!
}

type Starship {
	"The ID of the starship"
	id: ID!
	"The name of the starship"
	name: String!
	"Length of the starship, along the longest axis"
	length(unit: LengthUnit = METER @directiveOnArgument): Float
}

union SearchResult @directiveOnUnion = Human | Droid | Starship

"The 'Int' scalar type represents non-fractional signed whole numeric values. Int can represent values between -(2^31) and 2^31 - 1."
scalar Int @directiveOnScalar

"The 'Float' scalar type represents signed double-precision fractional values as specified by [IEEE 754](http://en.wikipedia.org/wiki/IEEE_floating_point)."
scalar Float

"The 'String' scalar type represents textual data, represented as UTF-8 character sequences. The String type is most often used by GraphQL to represent free-form human-readable text."
scalar String

"The 'Boolean' scalar type represents 'true' or 'false' ."
scalar Boolean

"The 'ID' scalar type represents a unique identifier, often used to refetch an object or as key for a cache. The ID type appears in a JSON response as a String; however, it is not intended to be human-readable. When expected as an input type, any string (such as '4') or integer (such as 4) input value will be accepted as an ID."
scalar ID

"Directs the executor to include this field or fragment only when the argument is true."
directive @include (
	" Included when true."
	if: Boolean!
) on FIELD | FRAGMENT_SPREAD | INLINE_FRAGMENT

"Directs the executor to skip this field or fragment when the argument is true."
directive @skip (
	"Skipped when true."
	if: Boolean!
) on FIELD | FRAGMENT_SPREAD | INLINE_FRAGMENT

"Marks an element of a GraphQL schema as no longer supported."
directive @deprecated (
	"""
	Explains why this element was deprecated, usually also including a suggestion
    for how to access supported similar data. Formatted in
    [Markdown](https://daringfireball.net/projects/markdown/).
	"""
	reason: String
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

"""
Arguments provided to Fields or Directives and the input fields of an
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
