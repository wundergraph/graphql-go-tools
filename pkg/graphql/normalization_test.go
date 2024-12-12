package graphql

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/TykTechnologies/graphql-go-tools/internal/pkg/unsafeprinter"
	"github.com/TykTechnologies/graphql-go-tools/pkg/operationreport"
	"github.com/TykTechnologies/graphql-go-tools/pkg/starwars"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRequest_Normalize(t *testing.T) {
	t.Run("should return error when schema is nil", func(t *testing.T) {
		request := Request{
			OperationName: "Hello",
			Variables:     nil,
			Query:         `query Hello { hello }`,
		}

		result, err := request.Normalize(nil)
		assert.Error(t, err)
		assert.Equal(t, ErrNilSchema, err)
		assert.False(t, result.Successful)
		assert.False(t, request.isNormalized)
	})

	t.Run("should successfully normalize request with fragments", func(t *testing.T) {
		schema := starwarsSchema(t)
		request := requestForQuery(t, starwars.FileFragmentsQuery)
		documentBeforeNormalization := request.document

		result, err := request.Normalize(schema)
		assert.NoError(t, err)
		assert.NotEqual(t, documentBeforeNormalization, request.document)
		assert.True(t, result.Successful)
		assert.True(t, request.isNormalized)

		normalizedOperation := `query Fragments($droidID: ID!){
    hero {
        name
    }
    droid(id: $droidID){
        name
    }
}`
		op := unsafeprinter.PrettyPrint(&request.document, nil)
		assert.Equal(t, normalizedOperation, op)
	})

	runNormalizationWithSchema := func(t *testing.T, schema *Schema, request *Request, expectedVars string, expectedNormalizedOperation string) {
		t.Helper()

		documentBeforeNormalization := request.document

		result, err := request.Normalize(schema)
		assert.NoError(t, err)
		assert.NotEqual(t, documentBeforeNormalization, request.document)
		assert.Equal(t, []byte(expectedVars), request.document.Input.Variables)
		assert.True(t, result.Successful)
		assert.True(t, request.isNormalized)

		op := unsafeprinter.PrettyPrint(&request.document, nil)
		assert.Equal(t, expectedNormalizedOperation, op)
	}

	runNormalization := func(t *testing.T, request *Request, expectedVars string, expectedNormalizedOperation string) {
		t.Helper()

		schema := starwarsSchema(t)

		runNormalizationWithSchema(t, schema, request, expectedVars, expectedNormalizedOperation)
	}

	t.Run("should successfully normalize single query with arguments", func(t *testing.T) {
		request := requestForQuery(t, starwars.FileDroidWithArgQuery)

		runNormalization(t, &request, `{"a":"R2D2"}`, `query($a: ID!){
    droid(id: $a){
        name
    }
}`)
	})

	t.Run("should successfully normalize query and remove unused variables", func(t *testing.T) {
		request := Request{
			OperationName: "MySearch",
			Variables: stringify(map[string]interface{}{
				"s":     "Luke",
				"other": "other",
			}),
			Query: `query MySearch($s: String!, $other: String) {search(name: $s) {...on Human {name}}}`,
		}

		runNormalization(t, &request, `{"s":"Luke"}`, `query MySearch($s: String!){
    search(name: $s){
        ... on Human {
            name
        }
    }
}`)
	})

	t.Run("should successfully normalize query and remove unused variables and values", func(t *testing.T) {
		const expectedVar = "query MySearch($s: String!){\n    search(name: $s){\n        ... on Human {\n            name\n        }\n    }\n}"
		for _, v := range []Request{
			{
				OperationName: "MySearch",
				Variables: stringify(map[string]interface{}{
					"s":  "Luke",
					"s2": nil,
					"s3": nil,
				}),
				Query: `query MySearch($s: String!, $s2: String, $s3: String) {search(name: $s) {...on Human {name}}}`,
			},
			{
				OperationName: "MySearch",
				Variables: stringify(map[string]interface{}{
					"s":  "Luke",
					"s2": 12,
					"s3": "",
				}),
				Query: `query MySearch($s: String!, $s2: Int, $s3: String) {search(name: $s) {...on Human {name}}}`,
			},
			{
				OperationName: "MySearch",
				Variables: stringify(map[string]interface{}{
					"s":  "Luke",
					"s3": "value",
				}),
				Query: `query MySearch($s: String!, $s2: Int, $s3: String) {search(name: $s) {...on Human {name}}}`,
			},
			{
				OperationName: "MySearch",
				Variables:     []byte(`{"s":"Luke", "s2": null, "s3": 78.8}`),
				Query:         `query MySearch($s: String!, $s2: String, $s3: String) {search(name: $s) {...on Human {name}}}`,
			},
		} {
			runNormalization(t, &v, `{"s":"Luke"}`, expectedVar)
		}
	})

	t.Run("should successfully normalize query and remove variables with no value provided", func(t *testing.T) {
		request := Request{
			OperationName: "MySearch",
			Variables: stringify(map[string]interface{}{
				"s": "Luke",
			}),
			Query: `query MySearch($s: String!, $other: String) {search(name: $s) {...on Human {name}}}`,
		}
		runNormalization(t, &request, `{"s":"Luke"}`, `query MySearch($s: String!){
    search(name: $s){
        ... on Human {
            name
        }
    }
}`)
	})

	t.Run("should successfully normalize multiple queries with arguments", func(t *testing.T) {
		request := requestForQuery(t, starwars.FileMultiQueriesWithArguments)
		request.OperationName = "GetDroid"

		runNormalization(t, &request, `{"a":"1"}`, `query GetDroid($a: ID!){
    droid(id: $a){
        name
    }
}

query Search {
    search(name: "C3PO"){
        ... on Droid {
            name
            primaryFunction
        }
        ... on Human {
            name
            height
        }
        ... on Starship {
            name
            length
        }
    }
}`)
	})

	t.Run("input coercion for lists without variables", func(t *testing.T) {
		schema := inputCoercionForListSchema(t)
		request := Request{
			OperationName: "charactersByIds",
			Variables:     stringify(map[string]interface{}{"a": 1}),
			Query:         `query ($a: [Int]) { charactersByIds(ids: $a) { name }}`,
		}
		runNormalizationWithSchema(t, schema, &request, `{"a":[1]}`, `query($a: [Int]){
    charactersByIds(ids: $a){
        name
    }
}`)
	})

	t.Run("input coercion for lists with variable extraction", func(t *testing.T) {
		schema := inputCoercionForListSchema(t)
		request := Request{
			OperationName: "GetCharactersByIds",
			Variables:     stringify(map[string]interface{}{}),
			Query:         `query GetCharactersByIds { charactersByIds(ids: 1) { name }}`,
		}
		runNormalizationWithSchema(t, schema, &request, `{"a":[1]}`, `query GetCharactersByIds($a: [Int]){
    charactersByIds(ids: $a){
        name
    }
}`)
	})

	t.Run("input coercion for lists with variables", func(t *testing.T) {
		schema := inputCoercionForListSchema(t)
		request := Request{
			OperationName: "charactersByIds",
			Variables: stringify(map[string]interface{}{
				"ids": 1,
			}),
			Query: `query($ids: [Int]) {charactersByIds(ids: $ids) { name }}`,
		}
		runNormalizationWithSchema(t, schema, &request, `{"ids":[1]}`, `query($ids: [Int]){
    charactersByIds(ids: $ids){
        name
    }
}`)
	})
}

func Test_normalizationResultFromReport(t *testing.T) {
	t.Run("should return successful result when report does not have errors", func(t *testing.T) {
		report := operationreport.Report{}
		result, err := normalizationResultFromReport(report)

		assert.NoError(t, err)
		assert.Equal(t, NormalizationResult{Successful: true, Errors: nil}, result)
	})

	t.Run("should return graphql errors and internal error when report contains them", func(t *testing.T) {
		internalErr := errors.New("errors occurred")
		externalErr := operationreport.ExternalError{
			Message:   "graphql error",
			Path:      nil,
			Locations: nil,
		}

		report := operationreport.Report{}
		report.AddInternalError(internalErr)
		report.AddExternalError(externalErr)

		result, err := normalizationResultFromReport(report)

		assert.Error(t, err)
		assert.Equal(t, internalErr, err)
		assert.False(t, result.Successful)
		assert.Equal(t, result.Errors.Count(), 1)
		assert.Equal(t, "graphql error", result.Errors.(RequestErrors)[0].Message)
	})
}

const schemaStringForTT13608 = `
scalar UUID
scalar DateTime

enum SortEnumType {
  ASC
  DESC
}

input DefinitionSortInput {
  definitionKey: SortEnumType
  createdBy: SortEnumType
  createdOn: SortEnumType
  modifiedBy: SortEnumType
  modifiedOn: SortEnumType
  publishedBy: SortEnumType
  publishedOn: SortEnumType
  actionKey: SortEnumType
}

input StringOperationFilterInput {
  and: [StringOperationFilterInput!]
  or: [StringOperationFilterInput!]
  eq: String
  neq: String
  contains: String
  ncontains: String
  in: [String]
  nin: [String]
  startsWith: String
  nstartsWith: String
  endsWith: String
  nendsWith: String
}

input DateTimeOperationFilterInput {
  eq: DateTime
  neq: DateTime
  in: [DateTime]
  nin: [DateTime]
  gt: DateTime
  ngt: DateTime
  gte: DateTime
  ngte: DateTime
  lt: DateTime
  nlt: DateTime
  lte: DateTime
  nlte: DateTime
}

input UuidOperationFilterInput {
  eq: UUID
  neq: UUID
  in: [UUID]
  nin: [UUID]
  gt: UUID
  ngt: UUID
  gte: UUID
  ngte: UUID
  lt: UUID
  nlt: UUID
  lte: UUID
  nlte: UUID
}

input DefinitionFilterInput {
  and: [DefinitionFilterInput!]
  or: [DefinitionFilterInput!]
  createdBy: StringOperationFilterInput
  createdOn: DateTimeOperationFilterInput
  publishedBy: StringOperationFilterInput
  actionKey: UuidOperationFilterInput
}

type DefinitionsCollectionSegment {
  """
  Information to aid in pagination.
  """
  pageInfo: CollectionSegmentInfo!
  """
  A flattened list of the items.
  """
  items: [Definition!]
  totalCount: Int!
}

type Definition {
  definitionKey: UUID!
  createdBy: String!
  createdOn: DateTime!
  modifiedBy: String
  modifiedOn: DateTime
  publishedBy: String
  publishedOn: DateTime
  actionKey: UUID!
}

type Query {
  definitions(
    skip: Int
    take: Int
    where: DefinitionFilterInput
    order: [DefinitionSortInput!]
  ): DefinitionsCollectionSegment
}
`

func TestRequest_Normalize_TT13608(t *testing.T) {
	// See TT-13608 for details.
	// Error message before the fix: mismatched input value
	// Input coercion visitor should make `order` a list, instead it was leaving it as-is.
	r := Request{
		OperationName: "getDefinition",
		Variables:     json.RawMessage(`{"actionKey": "46d69656-99be-4faa-af82-fcf778bca8ed"}`),
		Query: `query getDefinition($actionKey: UUID) {
	definitions(
		take: 10
		skip: 0
        where: {
            publishedBy: {
                eq: null
            },
            actionKey: {
                eq: $actionKey
            },
            createdBy: {
                startsWith: "v"
            }
        }
		order: { createdOn: DESC, createdBy: ASC }
	) {
		items {
            publishedOn
            createdBy
            createdOn
            definitionKey
            modifiedBy
            modifiedOn
		}
		totalCount
	}
}`,
	}

	schema, err := NewSchemaFromString(schemaStringForTT13608)
	require.NoError(t, err)

	result, err := r.Normalize(schema)
	assert.NoError(t, err)
	assert.True(t, result.Successful)
	assert.True(t, r.isNormalized)
}

func inputCoercionForListSchema(t *testing.T) *Schema {
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
