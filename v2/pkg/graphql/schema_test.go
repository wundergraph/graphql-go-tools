package graphql

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/astparser"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astprinter"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/asttransform"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan"
)

func TestNewSchemaFromReader(t *testing.T) {
	t.Run("should return error when an error occures internally", func(t *testing.T) {
		schemaBytes := []byte("query: Query")
		schemaReader := bytes.NewBuffer(schemaBytes)
		schema, err := NewSchemaFromReader(schemaReader)

		assert.Error(t, err)
		assert.Nil(t, schema)
	})

	t.Run("should successfully read from io.Reader", func(t *testing.T) {
		schemaBytes := []byte("schema { query: Query } type Query { hello: String }")
		schemaReader := bytes.NewBuffer(schemaBytes)
		schema, err := NewSchemaFromReader(schemaReader)

		assert.NoError(t, err)
		assert.Equal(t, schemaBytes, schema.rawInput)
	})
}

func TestNewSchemaFromString(t *testing.T) {
	t.Run("should return error when an error occures internally", func(t *testing.T) {
		schemaBytes := []byte("query: Query")
		schema, err := NewSchemaFromString(string(schemaBytes))

		assert.Error(t, err)
		assert.Nil(t, schema)
	})

	t.Run("should successfully read from string", func(t *testing.T) {
		schemaBytes := []byte("schema { query: Query } type Query { hello: String }")
		schema, err := NewSchemaFromString(string(schemaBytes))

		assert.NoError(t, err)
		assert.Equal(t, schemaBytes, schema.rawInput)
	})
}

func TestSchema_Normalize(t *testing.T) {
	t.Run("should successfully normalize schema", func(t *testing.T) {
		parsedSchema, err := NewSchemaFromString("type Query { me: String } extend type Query { you: String }")
		require.NoError(t, err)

		require.False(t, parsedSchema.IsNormalized())
		normalizationResult, err := parsedSchema.Normalize()

		assert.NoError(t, err)
		assert.True(t, normalizationResult.Successful)
		assert.Nil(t, normalizationResult.Errors)
		assert.True(t, parsedSchema.IsNormalized())

		normalizationResult, err = parsedSchema.Normalize()
		assert.NoError(t, err)
		assert.True(t, normalizationResult.Successful)
		assert.Nil(t, normalizationResult.Errors)
	})
}

func TestSchema_HasQueryType(t *testing.T) {
	run := func(schema string, expectation bool) func(t *testing.T) {
		return func(t *testing.T) {
			parsedSchema, err := createSchema([]byte(schema), false)
			require.NoError(t, err)

			result := parsedSchema.HasQueryType()
			assert.Equal(t, expectation, result)
		}
	}

	t.Run("schema without base defition", func(t *testing.T) {
		t.Run("should return false when there is no query type present", run(`
				schema {
					mutation: Mutation
				}
				type Mutation {
					save: Boolean!
				}`, false),
		)

		t.Run("should return true when there is a query type present", run(`
				schema {
					query: Query
				}
				type Query {
					hello: String!
				}`, true),
		)
	})
}

func TestSchema_QueryTypeName(t *testing.T) {
	run := func(schema string, expectation string) func(t *testing.T) {
		return func(t *testing.T) {
			parsedSchema, err := NewSchemaFromString(schema)
			require.NoError(t, err)

			result := parsedSchema.QueryTypeName()
			assert.Equal(t, expectation, result)
		}
	}

	t.Run("should return default query name when no query type is present", run(`
				schema {
					mutation: Mutation
				}
				type Mutation {
					save: Boolean!
				}`, "Query"),
	)

	t.Run("should return 'Query' when there is a query type named 'Query'", run(`
				schema {
					query: Query
				}
				type Query {
					hello: String!
				}`, "Query"),
	)

	t.Run("should return 'Other' when there is a query type named 'Other'", run(`
				schema {
					query: Other
				}
				type Other {
					hello: String!
				}`, "Other"),
	)
}

func TestSchema_HasMutationType(t *testing.T) {
	run := func(schema string, expectation bool) func(t *testing.T) {
		return func(t *testing.T) {
			parsedSchema, err := NewSchemaFromString(schema)
			require.NoError(t, err)

			result := parsedSchema.HasMutationType()
			assert.Equal(t, expectation, result)
		}
	}

	t.Run("should return false when there is no mutation type present", run(`
				schema {
					query: Query
				}
				type Query {
					hello: String!
				}`, false),
	)

	t.Run("should return true when there is a mutation type present", run(`
				schema {
					mutation: Mutation
				}
				type Mutation {
					save: Boolean!
				}`, true),
	)
}

func TestSchema_MutationTypeName(t *testing.T) {
	run := func(schema string, expectation string) func(t *testing.T) {
		return func(t *testing.T) {
			parsedSchema, err := NewSchemaFromString(schema)
			require.NoError(t, err)

			result := parsedSchema.MutationTypeName()
			assert.Equal(t, expectation, result)
		}
	}

	t.Run("should return empty string when no mutation type is present", run(`
				schema {
					query: Query
				}
				type Query {
					hello: String!
				}`, ""),
	)

	t.Run("should return 'Mutation' when there is a mutation type named 'Mutation'", run(`
				schema {
					mutation: Mutation
				}
				type Mutation {
					save: Boolean!
				}`, "Mutation"),
	)

	t.Run("should return 'Other' when there is a mutation type named 'Other'", run(`
				schema {
					mutation: Other
				}
				type Other {
					save: Boolean!
				}`, "Other"),
	)
}

func TestSchema_HasSubscriptionType(t *testing.T) {
	run := func(schema string, expectation bool) func(t *testing.T) {
		return func(t *testing.T) {
			parsedSchema, err := NewSchemaFromString(schema)
			require.NoError(t, err)

			result := parsedSchema.HasSubscriptionType()
			assert.Equal(t, expectation, result)
		}
	}

	t.Run("should return false when there is no subscription type present", run(`
				schema {
					query: Query
				}
				type Query {
					hello: String!
				}`, false),
	)

	t.Run("should return true when there is a subscription type present", run(`
				schema {
					subscription: Subscription
				}
				type Subscription {
					news: String!
				}`, true),
	)
}

func TestSchema_SubscriptionTypeName(t *testing.T) {
	run := func(schema string, expectation string) func(t *testing.T) {
		return func(t *testing.T) {
			parsedSchema, err := NewSchemaFromString(schema)
			require.NoError(t, err)

			result := parsedSchema.SubscriptionTypeName()
			assert.Equal(t, expectation, result)
		}
	}

	t.Run("should return empty string when no subscription type is present", run(`
				schema {
					query: Query
				}
				type Query {
					hello: String!
				}`, ""),
	)

	t.Run("should return 'Subscription' when there is a subscription type named 'Subscription'", run(`
				schema {
					subscription: Subscription
				}
				type Subscription {
					news: String!
				}`, "Subscription"),
	)

	t.Run("should return 'Other' when there is a subscription type named 'Other'", run(`
				schema {
					subscription: Other
				}
				type Other {
					news: String!
				}`, "Other"),
	)
}

func TestSchema_Document(t *testing.T) {
	schemaBytes := []byte("schema { query: Query } type Query { hello: String }")
	schema, err := NewSchemaFromString(string(schemaBytes))
	require.NoError(t, err)

	document, report := astparser.ParseGraphqlDocumentBytes(schemaBytes)
	require.False(t, report.HasErrors())

	err = asttransform.MergeDefinitionWithBaseSchema(&document)
	require.NoError(t, err)

	expectedSchemaBytesBuffer := &bytes.Buffer{}
	err = astprinter.PrintIndent(&document, nil, []byte("  "), expectedSchemaBytesBuffer)
	require.NoError(t, err)

	assert.Equal(t, expectedSchemaBytesBuffer.Bytes(), schema.Document())
}

func TestValidateSchemaString(t *testing.T) {
	run := func(schema string, expectedValid bool, expectedValidationErrorCount int) func(t *testing.T) {
		return func(t *testing.T) {
			validationResult, err := ValidateSchemaString(schema)
			assert.NoError(t, err)
			assert.Equal(t, expectedValid, validationResult.Valid)
			assert.Equal(t, expectedValidationErrorCount, validationResult.Errors.Count())
		}
	}

	t.Run("should successfuly validate broken schema as invalid", run(
		`type Query {`,
		false,
		1,
	))

	t.Run("should successfully validate schema with duplicate fields on query as invalid", run(
		`type Mutation {
					default: String
				}
				type Query {
					default: String
					default: String
				}`,
		false,
		1,
	))

	t.Run("should successfully validate invalid schema schema as invalid", run(
		invalidSchema,
		false,
		1,
	))

	t.Run("should successfully validate countries schema as valid", run(
		countriesSchema,
		true,
		0,
	))

	t.Run("should successfully validate swapi schema as valid", run(
		swapiSchema,
		true,
		0,
	))
}

func TestSchema_Validate(t *testing.T) {
	run := func(schema string, expectedValid bool, expectedValidationErrorCount int) func(t *testing.T) {
		return func(t *testing.T) {
			parsedSchema, err := NewSchemaFromString(schema)
			require.NoError(t, err)

			validationResult, err := parsedSchema.Validate()
			assert.NoError(t, err)
			assert.Equal(t, expectedValid, validationResult.Valid)
			assert.Equal(t, expectedValidationErrorCount, validationResult.Errors.Count())
		}
	}

	t.Run("should successfully validate invalid schema schema as invalid", run(
		invalidSchema,
		false,
		1,
	))

	t.Run("should successfully validate schema with duplicate fields on query as invalid", run(
		`type Mutation {
					default: String
				}
				type Query {
					default: String
					default: String
				}`,
		false,
		1,
	))

	t.Run("should successfully validate countries schema as valid", run(
		countriesSchema,
		true,
		0,
	))

	t.Run("should successfully validate swapi schema as valid", run(
		swapiSchema,
		true,
		0,
	))
}

func TestSchema_GetAllFieldArguments(t *testing.T) {
	schema, err := NewSchemaFromString(schemaWithChildren)
	require.NoError(t, err)

	t.Run("should get all field arguments without skip function", func(t *testing.T) {
		fieldArguments := schema.GetAllFieldArguments()
		expectedFieldArguments := []TypeFieldArguments{
			{
				TypeName:      "Query",
				FieldName:     "singleArgLevel1",
				ArgumentNames: []string{"lvl"},
			},
			{
				TypeName:      "Query",
				FieldName:     "_entities",
				ArgumentNames: []string{"representations"},
			},
			{
				TypeName:      "Query",
				FieldName:     "__type",
				ArgumentNames: []string{"name"},
			},
			{
				TypeName:      "Query",
				FieldName:     "multiArgLevel1",
				ArgumentNames: []string{"lvl", "number"},
			},
			{
				TypeName:      "SingleArgLevel1",
				FieldName:     "singleArgLevel2",
				ArgumentNames: []string{"lvl"},
			},
			{
				TypeName:      "MultiArgLevel1",
				FieldName:     "multiArgLevel2",
				ArgumentNames: []string{"lvl", "number"},
			},
			{
				TypeName:      "__Type",
				FieldName:     "fields",
				ArgumentNames: []string{"includeDeprecated"},
			},
			{
				TypeName:      "__Type",
				FieldName:     "enumValues",
				ArgumentNames: []string{"includeDeprecated"},
			},
		}
		assert.Equal(t, expectedFieldArguments, fieldArguments)
	})

	t.Run("should get all field arguments excluding skipped fields by skip field funcs", func(t *testing.T) {
		fieldArguments := schema.GetAllFieldArguments(NewSkipReservedNamesFunc())
		expectedFieldArguments := []TypeFieldArguments{
			{
				TypeName:      "Query",
				FieldName:     "singleArgLevel1",
				ArgumentNames: []string{"lvl"},
			},
			{
				TypeName:      "Query",
				FieldName:     "_entities",
				ArgumentNames: []string{"representations"},
			},
			{
				TypeName:      "Query",
				FieldName:     "multiArgLevel1",
				ArgumentNames: []string{"lvl", "number"},
			},
			{
				TypeName:      "SingleArgLevel1",
				FieldName:     "singleArgLevel2",
				ArgumentNames: []string{"lvl"},
			},
			{
				TypeName:      "MultiArgLevel1",
				FieldName:     "multiArgLevel2",
				ArgumentNames: []string{"lvl", "number"},
			},
		}
		assert.Equal(t, expectedFieldArguments, fieldArguments)
	})
}

func TestSchema_GetAllNestedFieldChildrenFromTypeField(t *testing.T) {
	schema, err := NewSchemaFromString(schemaWithChildren)
	require.NoError(t, err)

	t.Run("should return nil when type or field does not exist", func(t *testing.T) {
		typeFields := schema.GetAllNestedFieldChildrenFromTypeField("Not", "existent")
		assert.Equal(t, []TypeFields(nil), typeFields)
	})

	t.Run("should get field children without skip function", func(t *testing.T) {
		typeFields := schema.GetAllNestedFieldChildrenFromTypeField("Query", "withChildren")
		expectedTypeFields := []TypeFields{
			{
				TypeName:   "WithChildren",
				FieldNames: []string{"id", "name", "nested", "__typename"},
			},
			{
				TypeName:   "Nested",
				FieldNames: []string{"id", "name", "__typename"},
			},
		}

		assert.Equal(t, expectedTypeFields, typeFields)
	})

	t.Run("should get field children without skip function on field with interface type", func(t *testing.T) {
		typeFields := schema.GetAllNestedFieldChildrenFromTypeField("Query", "idType")
		expectedTypeFields := []TypeFields{
			{
				TypeName:   "WithChildren",
				FieldNames: []string{"id", "name", "nested", "__typename"},
			},
			{
				TypeName:   "Nested",
				FieldNames: []string{"id", "name", "__typename"},
			},
			{
				TypeName:   "IDType",
				FieldNames: []string{"id", "__typename"},
			},
		}

		assert.Equal(t, expectedTypeFields, typeFields)
	})

	t.Run("should get field children with skip function for engine v2 data source config", func(t *testing.T) {
		dataSources := []plan.DataSourceConfiguration{
			{
				RootNodes: []plan.TypeField{
					{
						TypeName:   "WithChildren",
						FieldNames: []string{"nested"},
					},
				},
			},
		}
		typeFields := schema.GetAllNestedFieldChildrenFromTypeField("Query", "withChildren", NewIsDataSourceConfigV2RootFieldSkipFunc(dataSources))
		expectedTypeFields := []TypeFields{
			{
				TypeName:   "WithChildren",
				FieldNames: []string{"id", "name", "__typename"},
			},
		}

		assert.Equal(t, expectedTypeFields, typeFields)
	})

	t.Run("should get field children from schema with recursive references", func(t *testing.T) {
		schema, err = NewSchemaFromString(countriesSchema)
		require.NoError(t, err)

		typeFields := schema.GetAllNestedFieldChildrenFromTypeField("Query", "countries")
		expectedTypeFields := []TypeFields{
			{
				TypeName:   "Country",
				FieldNames: []string{"code", "name", "native", "phone", "continent", "capital", "currency", "languages", "emoji", "emojiU", "states", "__typename"},
			},
			{
				TypeName:   "Continent",
				FieldNames: []string{"code", "name", "countries", "__typename"},
			},
			{
				TypeName:   "Language",
				FieldNames: []string{"code", "name", "native", "rtl", "__typename"},
			},
			{
				TypeName:   "State",
				FieldNames: []string{"code", "name", "country", "__typename"},
			},
		}

		assert.Equal(t, expectedTypeFields, typeFields)
	})

	t.Run("should get field children from schema with recursive references on field with interface type", func(t *testing.T) {
		schema, err = NewSchemaFromString(countriesSchema)
		require.NoError(t, err)

		typeFields := schema.GetAllNestedFieldChildrenFromTypeField("Query", "codeType")
		expectedTypeFields := []TypeFields{
			{
				TypeName:   "Continent",
				FieldNames: []string{"code", "name", "countries", "__typename"},
			},
			{
				TypeName:   "Country",
				FieldNames: []string{"code", "name", "native", "phone", "continent", "capital", "currency", "languages", "emoji", "emojiU", "states", "__typename"},
			},
			{
				TypeName:   "Language",
				FieldNames: []string{"code", "name", "native", "rtl", "__typename"},
			},
			{
				TypeName:   "State",
				FieldNames: []string{"code", "name", "country", "__typename"},
			},
			{
				TypeName:   "CodeNameType",
				FieldNames: []string{"code", "name", "__typename"},
			},
			{
				TypeName:   "CodeType",
				FieldNames: []string{"code", "__typename"},
			},
		}

		assert.Equal(t, expectedTypeFields, typeFields)
	})
}

var invalidSchema = `type Query {
	foo: Bar
}`

var schemaWithChildren = `scalar _Any
union _Entity = WithChildren

type Query {
	withChildren: WithChildren
	singleArgLevel1(lvl: int): SingleArgLevel1
	_entities(representations: [_Any!]!): [_Entity]!
	idType: IDType!
}

extend type Query {
	multiArgLevel1(lvl: int, number: int): MultiArgLevel1
}

interface IDType {
	id: ID!
}

type WithChildren implements IDType { 
	id: ID!
	name: String
	nested: Nested
}

type Nested implements IDType { 
	id: ID! 
	name: String! 
} 

type SingleArgLevel1 {
	singleArgLevel2(lvl: int): String
}

type MultiArgLevel1 {
	multiArgLevel2(lvl: int, number: int): String
}`

var countriesSchema = `directive @cacheControl(maxAge: Int, scope: CacheControlScope) on FIELD_DEFINITION | OBJECT | INTERFACE

schema {
	query: Query
}

interface CodeType {
	code: ID!
}

interface CodeNameType implements CodeType {
	code: ID!
	name: String!
}

enum CacheControlScope {
  PUBLIC
  PRIVATE
}

type Continent implements CodeNameType & CodeType {
  code: ID!
  name: String!
  countries: [Country!]!
}

input ContinentFilterInput {
  code: StringQueryOperatorInput
}

type Country implements CodeNameType & CodeType {
  code: ID!
  name: String!
  native: String!
  phone: String!
  continent: Continent!
  capital: String
  currency: String
  languages: [Language!]!
  emoji: String!
  emojiU: String!
  states: [State!]!
}

input CountryFilterInput {
  code: StringQueryOperatorInput
  currency: StringQueryOperatorInput
  continent: StringQueryOperatorInput
}

type Language {
  code: ID!
  name: String
  native: String
  rtl: Boolean!
}

input LanguageFilterInput {
  code: StringQueryOperatorInput
}

type Query {
  continents(filter: ContinentFilterInput): [Continent!]!
  continent(code: ID!): Continent
  countries(filter: CountryFilterInput): [Country!]!
  country(code: ID!): Country
  languages(filter: LanguageFilterInput): [Language!]!
  language(code: ID!): Language
  codeType: CodeType!
}

type State {
  code: String
  name: String!
  country: Country!
}

input StringQueryOperatorInput {
  eq: String
  ne: String
  in: [String]
  nin: [String]
  regex: String
  glob: String
}

"""The Upload scalar type represents a file upload."""
scalar Upload`

var swapiSchema = `schema {
  query: Root
}

"""A single film."""
type Film implements Node {
  """The title of this film."""
  title: String

  """The episode number of this film."""
  episodeID: Int

  """The opening paragraphs at the beginning of this film."""
  openingCrawl: String

  """The name of the director of this film."""
  director: String

  """The name(s) of the producer(s) of this film."""
  producers: [String]

  """The ISO 8601 date format of film release at original creator country."""
  releaseDate: String
  speciesConnection(after: String, first: Int, before: String, last: Int): FilmSpeciesConnection
  starshipConnection(after: String, first: Int, before: String, last: Int): FilmStarshipsConnection
  vehicleConnection(after: String, first: Int, before: String, last: Int): FilmVehiclesConnection
  characterConnection(after: String, first: Int, before: String, last: Int): FilmCharactersConnection
  planetConnection(after: String, first: Int, before: String, last: Int): FilmPlanetsConnection

  """The ISO 8601 date format of the time that this resource was created."""
  created: String

  """The ISO 8601 date format of the time that this resource was edited."""
  edited: String

  """The ID of an object"""
  id: ID!
}

"""A connection to a list of items."""
type FilmCharactersConnection {
  """Information to aid in pagination."""
  pageInfo: PageInfo!

  """A list of edges."""
  edges: [FilmCharactersEdge]

  """
  A count of the total number of objects in this connection, ignoring pagination.
  This allows a client to fetch the first five objects by passing "5" as the
  argument to "first", then fetch the total count so it could display "5 of 83",
  for example.
  """
  totalCount: Int

  """
  A list of all of the objects returned in the connection. This is a convenience
  field provided for quickly exploring the API; rather than querying for
  "{ edges { node } }" when no edge data is needed, this field can be be used
  instead. Note that when clients like Relay need to fetch the "cursor" field on
  the edge to enable efficient pagination, this shortcut cannot be used, and the
  full "{ edges { node } }" version should be used instead.
  """
  characters: [Person]
}

"""An edge in a connection."""
type FilmCharactersEdge {
  """The item at the end of the edge"""
  node: Person

  """A cursor for use in pagination"""
  cursor: String!
}

"""A connection to a list of items."""
type FilmPlanetsConnection {
  """Information to aid in pagination."""
  pageInfo: PageInfo!

  """A list of edges."""
  edges: [FilmPlanetsEdge]

  """
  A count of the total number of objects in this connection, ignoring pagination.
  This allows a client to fetch the first five objects by passing "5" as the
  argument to "first", then fetch the total count so it could display "5 of 83",
  for example.
  """
  totalCount: Int

  """
  A list of all of the objects returned in the connection. This is a convenience
  field provided for quickly exploring the API; rather than querying for
  "{ edges { node } }" when no edge data is needed, this field can be be used
  instead. Note that when clients like Relay need to fetch the "cursor" field on
  the edge to enable efficient pagination, this shortcut cannot be used, and the
  full "{ edges { node } }" version should be used instead.
  """
  planets: [Planet]
}

"""An edge in a connection."""
type FilmPlanetsEdge {
  """The item at the end of the edge"""
  node: Planet

  """A cursor for use in pagination"""
  cursor: String!
}

"""A connection to a list of items."""
type FilmsConnection {
  """Information to aid in pagination."""
  pageInfo: PageInfo!

  """A list of edges."""
  edges: [FilmsEdge]

  """
  A count of the total number of objects in this connection, ignoring pagination.
  This allows a client to fetch the first five objects by passing "5" as the
  argument to "first", then fetch the total count so it could display "5 of 83",
  for example.
  """
  totalCount: Int

  """
  A list of all of the objects returned in the connection. This is a convenience
  field provided for quickly exploring the API; rather than querying for
  "{ edges { node } }" when no edge data is needed, this field can be be used
  instead. Note that when clients like Relay need to fetch the "cursor" field on
  the edge to enable efficient pagination, this shortcut cannot be used, and the
  full "{ edges { node } }" version should be used instead.
  """
  films: [Film]
}

"""An edge in a connection."""
type FilmsEdge {
  """The item at the end of the edge"""
  node: Film

  """A cursor for use in pagination"""
  cursor: String!
}

"""A connection to a list of items."""
type FilmSpeciesConnection {
  """Information to aid in pagination."""
  pageInfo: PageInfo!

  """A list of edges."""
  edges: [FilmSpeciesEdge]

  """
  A count of the total number of objects in this connection, ignoring pagination.
  This allows a client to fetch the first five objects by passing "5" as the
  argument to "first", then fetch the total count so it could display "5 of 83",
  for example.
  """
  totalCount: Int

  """
  A list of all of the objects returned in the connection. This is a convenience
  field provided for quickly exploring the API; rather than querying for
  "{ edges { node } }" when no edge data is needed, this field can be be used
  instead. Note that when clients like Relay need to fetch the "cursor" field on
  the edge to enable efficient pagination, this shortcut cannot be used, and the
  full "{ edges { node } }" version should be used instead.
  """
  species: [Species]
}

"""An edge in a connection."""
type FilmSpeciesEdge {
  """The item at the end of the edge"""
  node: Species

  """A cursor for use in pagination"""
  cursor: String!
}

"""A connection to a list of items."""
type FilmStarshipsConnection {
  """Information to aid in pagination."""
  pageInfo: PageInfo!

  """A list of edges."""
  edges: [FilmStarshipsEdge]

  """
  A count of the total number of objects in this connection, ignoring pagination.
  This allows a client to fetch the first five objects by passing "5" as the
  argument to "first", then fetch the total count so it could display "5 of 83",
  for example.
  """
  totalCount: Int

  """
  A list of all of the objects returned in the connection. This is a convenience
  field provided for quickly exploring the API; rather than querying for
  "{ edges { node } }" when no edge data is needed, this field can be be used
  instead. Note that when clients like Relay need to fetch the "cursor" field on
  the edge to enable efficient pagination, this shortcut cannot be used, and the
  full "{ edges { node } }" version should be used instead.
  """
  starships: [Starship]
}

"""An edge in a connection."""
type FilmStarshipsEdge {
  """The item at the end of the edge"""
  node: Starship

  """A cursor for use in pagination"""
  cursor: String!
}

"""A connection to a list of items."""
type FilmVehiclesConnection {
  """Information to aid in pagination."""
  pageInfo: PageInfo!

  """A list of edges."""
  edges: [FilmVehiclesEdge]

  """
  A count of the total number of objects in this connection, ignoring pagination.
  This allows a client to fetch the first five objects by passing "5" as the
  argument to "first", then fetch the total count so it could display "5 of 83",
  for example.
  """
  totalCount: Int

  """
  A list of all of the objects returned in the connection. This is a convenience
  field provided for quickly exploring the API; rather than querying for
  "{ edges { node } }" when no edge data is needed, this field can be be used
  instead. Note that when clients like Relay need to fetch the "cursor" field on
  the edge to enable efficient pagination, this shortcut cannot be used, and the
  full "{ edges { node } }" version should be used instead.
  """
  vehicles: [Vehicle]
}

"""An edge in a connection."""
type FilmVehiclesEdge {
  """The item at the end of the edge"""
  node: Vehicle

  """A cursor for use in pagination"""
  cursor: String!
}

"""An object with an ID"""
interface Node {
  """The id of the object."""
  id: ID!
}

"""Information about pagination in a connection."""
type PageInfo {
  """When paginating forwards, are there more items?"""
  hasNextPage: Boolean!

  """When paginating backwards, are there more items?"""
  hasPreviousPage: Boolean!

  """When paginating backwards, the cursor to continue."""
  startCursor: String

  """When paginating forwards, the cursor to continue."""
  endCursor: String
}

"""A connection to a list of items."""
type PeopleConnection {
  """Information to aid in pagination."""
  pageInfo: PageInfo!

  """A list of edges."""
  edges: [PeopleEdge]

  """
  A count of the total number of objects in this connection, ignoring pagination.
  This allows a client to fetch the first five objects by passing "5" as the
  argument to "first", then fetch the total count so it could display "5 of 83",
  for example.
  """
  totalCount: Int

  """
  A list of all of the objects returned in the connection. This is a convenience
  field provided for quickly exploring the API; rather than querying for
  "{ edges { node } }" when no edge data is needed, this field can be be used
  instead. Note that when clients like Relay need to fetch the "cursor" field on
  the edge to enable efficient pagination, this shortcut cannot be used, and the
  full "{ edges { node } }" version should be used instead.
  """
  people: [Person]
}

"""An edge in a connection."""
type PeopleEdge {
  """The item at the end of the edge"""
  node: Person

  """A cursor for use in pagination"""
  cursor: String!
}

"""An individual person or character within the Star Wars universe."""
type Person implements Node {
  """The name of this person."""
  name: String

  """
  The birth year of the person, using the in-universe standard of BBY or ABY -
  Before the Battle of Yavin or After the Battle of Yavin. The Battle of Yavin is
  a battle that occurs at the end of Star Wars episode IV: A New Hope.
  """
  birthYear: String

  """
  The eye color of this person. Will be "unknown" if not known or "n/a" if the
  person does not have an eye.
  """
  eyeColor: String

  """
  The gender of this person. Either "Male", "Female" or "unknown",
  "n/a" if the person does not have a gender.
  """
  gender: String

  """
  The hair color of this person. Will be "unknown" if not known or "n/a" if the
  person does not have hair.
  """
  hairColor: String

  """The height of the person in centimeters."""
  height: Int

  """The mass of the person in kilograms."""
  mass: Float

  """The skin color of this person."""
  skinColor: String

  """A planet that this person was born on or inhabits."""
  homeworld: Planet
  filmConnection(after: String, first: Int, before: String, last: Int): PersonFilmsConnection

  """The species that this person belongs to, or null if unknown."""
  species: Species
  starshipConnection(after: String, first: Int, before: String, last: Int): PersonStarshipsConnection
  vehicleConnection(after: String, first: Int, before: String, last: Int): PersonVehiclesConnection

  """The ISO 8601 date format of the time that this resource was created."""
  created: String

  """The ISO 8601 date format of the time that this resource was edited."""
  edited: String

  """The ID of an object"""
  id: ID!
}

"""A connection to a list of items."""
type PersonFilmsConnection {
  """Information to aid in pagination."""
  pageInfo: PageInfo!

  """A list of edges."""
  edges: [PersonFilmsEdge]

  """
  A count of the total number of objects in this connection, ignoring pagination.
  This allows a client to fetch the first five objects by passing "5" as the
  argument to "first", then fetch the total count so it could display "5 of 83",
  for example.
  """
  totalCount: Int

  """
  A list of all of the objects returned in the connection. This is a convenience
  field provided for quickly exploring the API; rather than querying for
  "{ edges { node } }" when no edge data is needed, this field can be be used
  instead. Note that when clients like Relay need to fetch the "cursor" field on
  the edge to enable efficient pagination, this shortcut cannot be used, and the
  full "{ edges { node } }" version should be used instead.
  """
  films: [Film]
}

"""An edge in a connection."""
type PersonFilmsEdge {
  """The item at the end of the edge"""
  node: Film

  """A cursor for use in pagination"""
  cursor: String!
}

"""A connection to a list of items."""
type PersonStarshipsConnection {
  """Information to aid in pagination."""
  pageInfo: PageInfo!

  """A list of edges."""
  edges: [PersonStarshipsEdge]

  """
  A count of the total number of objects in this connection, ignoring pagination.
  This allows a client to fetch the first five objects by passing "5" as the
  argument to "first", then fetch the total count so it could display "5 of 83",
  for example.
  """
  totalCount: Int

  """
  A list of all of the objects returned in the connection. This is a convenience
  field provided for quickly exploring the API; rather than querying for
  "{ edges { node } }" when no edge data is needed, this field can be be used
  instead. Note that when clients like Relay need to fetch the "cursor" field on
  the edge to enable efficient pagination, this shortcut cannot be used, and the
  full "{ edges { node } }" version should be used instead.
  """
  starships: [Starship]
}

"""An edge in a connection."""
type PersonStarshipsEdge {
  """The item at the end of the edge"""
  node: Starship

  """A cursor for use in pagination"""
  cursor: String!
}

"""A connection to a list of items."""
type PersonVehiclesConnection {
  """Information to aid in pagination."""
  pageInfo: PageInfo!

  """A list of edges."""
  edges: [PersonVehiclesEdge]

  """
  A count of the total number of objects in this connection, ignoring pagination.
  This allows a client to fetch the first five objects by passing "5" as the
  argument to "first", then fetch the total count so it could display "5 of 83",
  for example.
  """
  totalCount: Int

  """
  A list of all of the objects returned in the connection. This is a convenience
  field provided for quickly exploring the API; rather than querying for
  "{ edges { node } }" when no edge data is needed, this field can be be used
  instead. Note that when clients like Relay need to fetch the "cursor" field on
  the edge to enable efficient pagination, this shortcut cannot be used, and the
  full "{ edges { node } }" version should be used instead.
  """
  vehicles: [Vehicle]
}

"""An edge in a connection."""
type PersonVehiclesEdge {
  """The item at the end of the edge"""
  node: Vehicle

  """A cursor for use in pagination"""
  cursor: String!
}

"""
A large mass, planet or planetoid in the Star Wars Universe, at the time of
0 ABY.
"""
type Planet implements Node {
  """The name of this planet."""
  name: String

  """The diameter of this planet in kilometers."""
  diameter: Int

  """
  The number of standard hours it takes for this planet to complete a single
  rotation on its axis.
  """
  rotationPeriod: Int

  """
  The number of standard days it takes for this planet to complete a single orbit
  of its local star.
  """
  orbitalPeriod: Int

  """
  A number denoting the gravity of this planet, where "1" is normal or 1 standard
  G. "2" is twice or 2 standard Gs. "0.5" is half or 0.5 standard Gs.
  """
  gravity: String

  """The average population of sentient beings inhabiting this planet."""
  population: Float

  """The climates of this planet."""
  climates: [String]

  """The terrains of this planet."""
  terrains: [String]

  """
  The percentage of the planet surface that is naturally occuring water or bodies
  of water.
  """
  surfaceWater: Float
  residentConnection(after: String, first: Int, before: String, last: Int): PlanetResidentsConnection
  filmConnection(after: String, first: Int, before: String, last: Int): PlanetFilmsConnection

  """The ISO 8601 date format of the time that this resource was created."""
  created: String

  """The ISO 8601 date format of the time that this resource was edited."""
  edited: String

  """The ID of an object"""
  id: ID!
}

"""A connection to a list of items."""
type PlanetFilmsConnection {
  """Information to aid in pagination."""
  pageInfo: PageInfo!

  """A list of edges."""
  edges: [PlanetFilmsEdge]

  """
  A count of the total number of objects in this connection, ignoring pagination.
  This allows a client to fetch the first five objects by passing "5" as the
  argument to "first", then fetch the total count so it could display "5 of 83",
  for example.
  """
  totalCount: Int

  """
  A list of all of the objects returned in the connection. This is a convenience
  field provided for quickly exploring the API; rather than querying for
  "{ edges { node } }" when no edge data is needed, this field can be be used
  instead. Note that when clients like Relay need to fetch the "cursor" field on
  the edge to enable efficient pagination, this shortcut cannot be used, and the
  full "{ edges { node } }" version should be used instead.
  """
  films: [Film]
}

"""An edge in a connection."""
type PlanetFilmsEdge {
  """The item at the end of the edge"""
  node: Film

  """A cursor for use in pagination"""
  cursor: String!
}

"""A connection to a list of items."""
type PlanetResidentsConnection {
  """Information to aid in pagination."""
  pageInfo: PageInfo!

  """A list of edges."""
  edges: [PlanetResidentsEdge]

  """
  A count of the total number of objects in this connection, ignoring pagination.
  This allows a client to fetch the first five objects by passing "5" as the
  argument to "first", then fetch the total count so it could display "5 of 83",
  for example.
  """
  totalCount: Int

  """
  A list of all of the objects returned in the connection. This is a convenience
  field provided for quickly exploring the API; rather than querying for
  "{ edges { node } }" when no edge data is needed, this field can be be used
  instead. Note that when clients like Relay need to fetch the "cursor" field on
  the edge to enable efficient pagination, this shortcut cannot be used, and the
  full "{ edges { node } }" version should be used instead.
  """
  residents: [Person]
}

"""An edge in a connection."""
type PlanetResidentsEdge {
  """The item at the end of the edge"""
  node: Person

  """A cursor for use in pagination"""
  cursor: String!
}

"""A connection to a list of items."""
type PlanetsConnection {
  """Information to aid in pagination."""
  pageInfo: PageInfo!

  """A list of edges."""
  edges: [PlanetsEdge]

  """
  A count of the total number of objects in this connection, ignoring pagination.
  This allows a client to fetch the first five objects by passing "5" as the
  argument to "first", then fetch the total count so it could display "5 of 83",
  for example.
  """
  totalCount: Int

  """
  A list of all of the objects returned in the connection. This is a convenience
  field provided for quickly exploring the API; rather than querying for
  "{ edges { node } }" when no edge data is needed, this field can be be used
  instead. Note that when clients like Relay need to fetch the "cursor" field on
  the edge to enable efficient pagination, this shortcut cannot be used, and the
  full "{ edges { node } }" version should be used instead.
  """
  planets: [Planet]
}

"""An edge in a connection."""
type PlanetsEdge {
  """The item at the end of the edge"""
  node: Planet

  """A cursor for use in pagination"""
  cursor: String!
}

type Root {
  allFilms(after: String, first: Int, before: String, last: Int): FilmsConnection
  film(id: ID, filmID: ID): Film
  allPeople(after: String, first: Int, before: String, last: Int): PeopleConnection
  person(id: ID, personID: ID): Person
  allPlanets(after: String, first: Int, before: String, last: Int): PlanetsConnection
  planet(id: ID, planetID: ID): Planet
  allSpecies(after: String, first: Int, before: String, last: Int): SpeciesConnection
  species(id: ID, speciesID: ID): Species
  allStarships(after: String, first: Int, before: String, last: Int): StarshipsConnection
  starship(id: ID, starshipID: ID): Starship
  allVehicles(after: String, first: Int, before: String, last: Int): VehiclesConnection
  vehicle(id: ID, vehicleID: ID): Vehicle

  """Fetches an object given its ID"""
  node(
    """The ID of an object"""
    id: ID!
  ): Node
}

"""A type of person or character within the Star Wars Universe."""
type Species implements Node {
  """The name of this species."""
  name: String

  """The classification of this species, such as "mammal" or "reptile"."""
  classification: String

  """The designation of this species, such as "sentient"."""
  designation: String

  """The average height of this species in centimeters."""
  averageHeight: Float

  """The average lifespan of this species in years, null if unknown."""
  averageLifespan: Int

  """
  Common eye colors for this species, null if this species does not typically
  have eyes.
  """
  eyeColors: [String]

  """
  Common hair colors for this species, null if this species does not typically
  have hair.
  """
  hairColors: [String]

  """
  Common skin colors for this species, null if this species does not typically
  have skin.
  """
  skinColors: [String]

  """The language commonly spoken by this species."""
  language: String

  """A planet that this species originates from."""
  homeworld: Planet
  personConnection(after: String, first: Int, before: String, last: Int): SpeciesPeopleConnection
  filmConnection(after: String, first: Int, before: String, last: Int): SpeciesFilmsConnection

  """The ISO 8601 date format of the time that this resource was created."""
  created: String

  """The ISO 8601 date format of the time that this resource was edited."""
  edited: String

  """The ID of an object"""
  id: ID!
}

"""A connection to a list of items."""
type SpeciesConnection {
  """Information to aid in pagination."""
  pageInfo: PageInfo!

  """A list of edges."""
  edges: [SpeciesEdge]

  """
  A count of the total number of objects in this connection, ignoring pagination.
  This allows a client to fetch the first five objects by passing "5" as the
  argument to "first", then fetch the total count so it could display "5 of 83",
  for example.
  """
  totalCount: Int

  """
  A list of all of the objects returned in the connection. This is a convenience
  field provided for quickly exploring the API; rather than querying for
  "{ edges { node } }" when no edge data is needed, this field can be be used
  instead. Note that when clients like Relay need to fetch the "cursor" field on
  the edge to enable efficient pagination, this shortcut cannot be used, and the
  full "{ edges { node } }" version should be used instead.
  """
  species: [Species]
}

"""An edge in a connection."""
type SpeciesEdge {
  """The item at the end of the edge"""
  node: Species

  """A cursor for use in pagination"""
  cursor: String!
}

"""A connection to a list of items."""
type SpeciesFilmsConnection {
  """Information to aid in pagination."""
  pageInfo: PageInfo!

  """A list of edges."""
  edges: [SpeciesFilmsEdge]

  """
  A count of the total number of objects in this connection, ignoring pagination.
  This allows a client to fetch the first five objects by passing "5" as the
  argument to "first", then fetch the total count so it could display "5 of 83",
  for example.
  """
  totalCount: Int

  """
  A list of all of the objects returned in the connection. This is a convenience
  field provided for quickly exploring the API; rather than querying for
  "{ edges { node } }" when no edge data is needed, this field can be be used
  instead. Note that when clients like Relay need to fetch the "cursor" field on
  the edge to enable efficient pagination, this shortcut cannot be used, and the
  full "{ edges { node } }" version should be used instead.
  """
  films: [Film]
}

"""An edge in a connection."""
type SpeciesFilmsEdge {
  """The item at the end of the edge"""
  node: Film

  """A cursor for use in pagination"""
  cursor: String!
}

"""A connection to a list of items."""
type SpeciesPeopleConnection {
  """Information to aid in pagination."""
  pageInfo: PageInfo!

  """A list of edges."""
  edges: [SpeciesPeopleEdge]

  """
  A count of the total number of objects in this connection, ignoring pagination.
  This allows a client to fetch the first five objects by passing "5" as the
  argument to "first", then fetch the total count so it could display "5 of 83",
  for example.
  """
  totalCount: Int

  """
  A list of all of the objects returned in the connection. This is a convenience
  field provided for quickly exploring the API; rather than querying for
  "{ edges { node } }" when no edge data is needed, this field can be be used
  instead. Note that when clients like Relay need to fetch the "cursor" field on
  the edge to enable efficient pagination, this shortcut cannot be used, and the
  full "{ edges { node } }" version should be used instead.
  """
  people: [Person]
}

"""An edge in a connection."""
type SpeciesPeopleEdge {
  """The item at the end of the edge"""
  node: Person

  """A cursor for use in pagination"""
  cursor: String!
}

"""A single transport craft that has hyperdrive capability."""
type Starship implements Node {
  """The name of this starship. The common name, such as "Death Star"."""
  name: String

  """
  The model or official name of this starship. Such as "T-65 X-wing" or "DS-1
  Orbital Battle Station".
  """
  model: String

  """
  The class of this starship, such as "Starfighter" or "Deep Space Mobile
  Battlestation"
  """
  starshipClass: String

  """The manufacturers of this starship."""
  manufacturers: [String]

  """The cost of this starship new, in galactic credits."""
  costInCredits: Float

  """The length of this starship in meters."""
  length: Float

  """The number of personnel needed to run or pilot this starship."""
  crew: String

  """The number of non-essential people this starship can transport."""
  passengers: String

  """
  The maximum speed of this starship in atmosphere. null if this starship is
  incapable of atmosphering flight.
  """
  maxAtmospheringSpeed: Int

  """The class of this starships hyperdrive."""
  hyperdriveRating: Float

  """
  The Maximum number of Megalights this starship can travel in a standard hour.
  A "Megalight" is a standard unit of distance and has never been defined before
  within the Star Wars universe. This figure is only really useful for measuring
  the difference in speed of starships. We can assume it is similar to AU, the
  distance between our Sun (Sol) and Earth.
  """
  MGLT: Int

  """The maximum number of kilograms that this starship can transport."""
  cargoCapacity: Float

  """
  The maximum length of time that this starship can provide consumables for its
  entire crew without having to resupply.
  """
  consumables: String
  pilotConnection(after: String, first: Int, before: String, last: Int): StarshipPilotsConnection
  filmConnection(after: String, first: Int, before: String, last: Int): StarshipFilmsConnection

  """The ISO 8601 date format of the time that this resource was created."""
  created: String

  """The ISO 8601 date format of the time that this resource was edited."""
  edited: String

  """The ID of an object"""
  id: ID!
}

"""A connection to a list of items."""
type StarshipFilmsConnection {
  """Information to aid in pagination."""
  pageInfo: PageInfo!

  """A list of edges."""
  edges: [StarshipFilmsEdge]

  """
  A count of the total number of objects in this connection, ignoring pagination.
  This allows a client to fetch the first five objects by passing "5" as the
  argument to "first", then fetch the total count so it could display "5 of 83",
  for example.
  """
  totalCount: Int

  """
  A list of all of the objects returned in the connection. This is a convenience
  field provided for quickly exploring the API; rather than querying for
  "{ edges { node } }" when no edge data is needed, this field can be be used
  instead. Note that when clients like Relay need to fetch the "cursor" field on
  the edge to enable efficient pagination, this shortcut cannot be used, and the
  full "{ edges { node } }" version should be used instead.
  """
  films: [Film]
}

"""An edge in a connection."""
type StarshipFilmsEdge {
  """The item at the end of the edge"""
  node: Film

  """A cursor for use in pagination"""
  cursor: String!
}

"""A connection to a list of items."""
type StarshipPilotsConnection {
  """Information to aid in pagination."""
  pageInfo: PageInfo!

  """A list of edges."""
  edges: [StarshipPilotsEdge]

  """
  A count of the total number of objects in this connection, ignoring pagination.
  This allows a client to fetch the first five objects by passing "5" as the
  argument to "first", then fetch the total count so it could display "5 of 83",
  for example.
  """
  totalCount: Int

  """
  A list of all of the objects returned in the connection. This is a convenience
  field provided for quickly exploring the API; rather than querying for
  "{ edges { node } }" when no edge data is needed, this field can be be used
  instead. Note that when clients like Relay need to fetch the "cursor" field on
  the edge to enable efficient pagination, this shortcut cannot be used, and the
  full "{ edges { node } }" version should be used instead.
  """
  pilots: [Person]
}

"""An edge in a connection."""
type StarshipPilotsEdge {
  """The item at the end of the edge"""
  node: Person

  """A cursor for use in pagination"""
  cursor: String!
}

"""A connection to a list of items."""
type StarshipsConnection {
  """Information to aid in pagination."""
  pageInfo: PageInfo!

  """A list of edges."""
  edges: [StarshipsEdge]

  """
  A count of the total number of objects in this connection, ignoring pagination.
  This allows a client to fetch the first five objects by passing "5" as the
  argument to "first", then fetch the total count so it could display "5 of 83",
  for example.
  """
  totalCount: Int

  """
  A list of all of the objects returned in the connection. This is a convenience
  field provided for quickly exploring the API; rather than querying for
  "{ edges { node } }" when no edge data is needed, this field can be be used
  instead. Note that when clients like Relay need to fetch the "cursor" field on
  the edge to enable efficient pagination, this shortcut cannot be used, and the
  full "{ edges { node } }" version should be used instead.
  """
  starships: [Starship]
}

"""An edge in a connection."""
type StarshipsEdge {
  """The item at the end of the edge"""
  node: Starship

  """A cursor for use in pagination"""
  cursor: String!
}

"""A single transport craft that does not have hyperdrive capability"""
type Vehicle implements Node {
  """
  The name of this vehicle. The common name, such as "Sand Crawler" or "Speeder
  bike".
  """
  name: String

  """
  The model or official name of this vehicle. Such as "All-Terrain Attack
  Transport".
  """
  model: String

  """The class of this vehicle, such as "Wheeled" or "Repulsorcraft"."""
  vehicleClass: String

  """The manufacturers of this vehicle."""
  manufacturers: [String]

  """The cost of this vehicle new, in Galactic Credits."""
  costInCredits: Float

  """The length of this vehicle in meters."""
  length: Float

  """The number of personnel needed to run or pilot this vehicle."""
  crew: String

  """The number of non-essential people this vehicle can transport."""
  passengers: String

  """The maximum speed of this vehicle in atmosphere."""
  maxAtmospheringSpeed: Int

  """The maximum number of kilograms that this vehicle can transport."""
  cargoCapacity: Float

  """
  The maximum length of time that this vehicle can provide consumables for its
  entire crew without having to resupply.
  """
  consumables: String
  pilotConnection(after: String, first: Int, before: String, last: Int): VehiclePilotsConnection
  filmConnection(after: String, first: Int, before: String, last: Int): VehicleFilmsConnection

  """The ISO 8601 date format of the time that this resource was created."""
  created: String

  """The ISO 8601 date format of the time that this resource was edited."""
  edited: String

  """The ID of an object"""
  id: ID!
}

"""A connection to a list of items."""
type VehicleFilmsConnection {
  """Information to aid in pagination."""
  pageInfo: PageInfo!

  """A list of edges."""
  edges: [VehicleFilmsEdge]

  """
  A count of the total number of objects in this connection, ignoring pagination.
  This allows a client to fetch the first five objects by passing "5" as the
  argument to "first", then fetch the total count so it could display "5 of 83",
  for example.
  """
  totalCount: Int

  """
  A list of all of the objects returned in the connection. This is a convenience
  field provided for quickly exploring the API; rather than querying for
  "{ edges { node } }" when no edge data is needed, this field can be be used
  instead. Note that when clients like Relay need to fetch the "cursor" field on
  the edge to enable efficient pagination, this shortcut cannot be used, and the
  full "{ edges { node } }" version should be used instead.
  """
  films: [Film]
}

"""An edge in a connection."""
type VehicleFilmsEdge {
  """The item at the end of the edge"""
  node: Film

  """A cursor for use in pagination"""
  cursor: String!
}

"""A connection to a list of items."""
type VehiclePilotsConnection {
  """Information to aid in pagination."""
  pageInfo: PageInfo!

  """A list of edges."""
  edges: [VehiclePilotsEdge]

  """
  A count of the total number of objects in this connection, ignoring pagination.
  This allows a client to fetch the first five objects by passing "5" as the
  argument to "first", then fetch the total count so it could display "5 of 83",
  for example.
  """
  totalCount: Int

  """
  A list of all of the objects returned in the connection. This is a convenience
  field provided for quickly exploring the API; rather than querying for
  "{ edges { node } }" when no edge data is needed, this field can be be used
  instead. Note that when clients like Relay need to fetch the "cursor" field on
  the edge to enable efficient pagination, this shortcut cannot be used, and the
  full "{ edges { node } }" version should be used instead.
  """
  pilots: [Person]
}

"""An edge in a connection."""
type VehiclePilotsEdge {
  """The item at the end of the edge"""
  node: Person

  """A cursor for use in pagination"""
  cursor: String!
}

"""A connection to a list of items."""
type VehiclesConnection {
  """Information to aid in pagination."""
  pageInfo: PageInfo!

  """A list of edges."""
  edges: [VehiclesEdge]

  """
  A count of the total number of objects in this connection, ignoring pagination.
  This allows a client to fetch the first five objects by passing "5" as the
  argument to "first", then fetch the total count so it could display "5 of 83",
  for example.
  """
  totalCount: Int

  """
  A list of all of the objects returned in the connection. This is a convenience
  field provided for quickly exploring the API; rather than querying for
  "{ edges { node } }" when no edge data is needed, this field can be be used
  instead. Note that when clients like Relay need to fetch the "cursor" field on
  the edge to enable efficient pagination, this shortcut cannot be used, and the
  full "{ edges { node } }" version should be used instead.
  """
  vehicles: [Vehicle]
}

"""An edge in a connection."""
type VehiclesEdge {
  """The item at the end of the edge"""
  node: Vehicle

  """A cursor for use in pagination"""
  cursor: String!
}`
