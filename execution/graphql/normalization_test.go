package graphql

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/astprinter"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/graphqlerrors"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/operationreport"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/starwars"
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
		schema := StarwarsSchema(t)
		request := StarwarsRequestForQuery(t, starwars.FileFragmentsQuery)
		request.OperationName = "Fragments"
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
		op, _ := astprinter.PrintStringIndent(&request.document, "    ")
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

		op, _ := astprinter.PrintStringIndent(&request.document, "    ")
		assert.Equal(t, expectedNormalizedOperation, op)
	}

	runNormalization := func(t *testing.T, request *Request, expectedVars string, expectedNormalizedOperation string) {
		t.Helper()

		schema := StarwarsSchema(t)

		runNormalizationWithSchema(t, schema, request, expectedVars, expectedNormalizedOperation)
	}

	t.Run("should successfully normalize single query with arguments", func(t *testing.T) {
		request := StarwarsRequestForQuery(t, starwars.FileDroidWithArgQuery)

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

		runNormalization(t, &request, `{"other":"other","s":"Luke"}`, `query MySearch($s: String!, $other: String){
    search(name: $s){
        ... on Human {
            name
        }
    }
}`)
	})

	t.Run("should successfully normalize query and remove variables with no value provided", func(t *testing.T) {
		request := Request{
			OperationName: "MySearch",
			Variables: stringify(map[string]interface{}{
				"s": "Luke",
			}),
			Query: `query MySearch($s: String!, $other: String) {search(name: $s) {...on Human {name}}}`,
		}
		runNormalization(t, &request, `{"s":"Luke"}`, `query MySearch($s: String!, $other: String){
    search(name: $s){
        ... on Human {
            name
        }
    }
}`)
	})

	t.Run("should successfully normalize multiple queries with arguments", func(t *testing.T) {
		request := StarwarsRequestForQuery(t, starwars.FileMultiQueriesWithArguments)
		request.OperationName = "GetDroid"

		runNormalization(t, &request, `{"a":"1"}`,
			`query GetDroid($a: ID!){
    droid(id: $a){
        name
    }
}`)
	})

	t.Run("input coercion for lists without variables", func(t *testing.T) {
		schema := InputCoercionForListSchema(t)
		request := Request{
			OperationName: "charactersByIds",
			Variables:     stringify(map[string]interface{}{"a": 1}),
			Query:         `query charactersByIds($a: [Int]) { charactersByIds(ids: $a) { name }}`,
		}
		runNormalizationWithSchema(t, schema, &request, `{"a":[1]}`, `query charactersByIds($a: [Int]){
    charactersByIds(ids: $a){
        name
    }
}`)
	})

	t.Run("input coercion for lists with variable extraction", func(t *testing.T) {
		schema := InputCoercionForListSchema(t)
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
		schema := InputCoercionForListSchema(t)
		request := Request{
			OperationName: "charactersByIds",
			Variables: stringify(map[string]interface{}{
				"ids": 1,
			}),
			Query: `query charactersByIds($ids: [Int]) {charactersByIds(ids: $ids) { name }}`,
		}
		runNormalizationWithSchema(t, schema, &request, `{"ids":[1]}`, `query charactersByIds($ids: [Int]){
    charactersByIds(ids: $ids){
        name
    }
}`)
	})
}

func Test_normalizationResultFromReport(t *testing.T) {
	t.Run("should return successful result when report does not have errors", func(t *testing.T) {
		report := operationreport.Report{}
		result, err := NormalizationResultFromReport(report)

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

		result, err := NormalizationResultFromReport(report)

		assert.Error(t, err)
		assert.Equal(t, internalErr, err)
		assert.False(t, result.Successful)
		assert.Equal(t, result.Errors.Count(), 1)
		assert.Equal(t, "graphql error", result.Errors.(graphqlerrors.RequestErrors)[0].Message)
	})
}

func stringify(any interface{}) []byte {
	out, _ := json.Marshal(any)
	return out
}
