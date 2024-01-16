package graphql

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/starwars"
)

type TestingTB interface {
	Errorf(format string, args ...interface{})
	Helper()
	FailNow()
}

func StarwarsSchema(t TestingTB) *Schema {
	schemaBytes := starwars.Schema(t)

	schema, err := NewSchemaFromString(string(schemaBytes))
	require.NoError(t, err)

	return schema
}

func StarwarsRequestForQuery(t TestingTB, fileName string) Request {
	rawRequest := starwars.LoadQuery(t, fileName, nil)

	var request Request
	err := UnmarshalRequest(bytes.NewBuffer(rawRequest), &request)
	require.NoError(t, err)

	return request
}

func LoadStarWarsQuery(starwarsFile string, variables starwars.QueryVariables) func(t *testing.T) Request {
	return func(t *testing.T) Request {
		query := starwars.LoadQuery(t, starwarsFile, variables)
		request := Request{}
		err := UnmarshalRequest(bytes.NewBuffer(query), &request)
		require.NoError(t, err)

		return request
	}
}

func InputCoercionForListSchema(t *testing.T) *Schema {
	schemaString := `schema {
	query: Query
}

type Character {
	id: Int
	name: String
}

type Query {
	charactersByIds(ids: [Int]): [Character]
}`

	schema, err := NewSchemaFromString(schemaString)
	require.NoError(t, err)
	return schema
}

func CreateCountriesSchema(t *testing.T) *Schema {
	schema, err := NewSchemaFromString(CountriesSchema)
	require.NoError(t, err)
	return schema
}

var CountriesSchema = `directive @cacheControl(maxAge: Int, scope: CacheControlScope) on FIELD_DEFINITION | OBJECT | INTERFACE

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
