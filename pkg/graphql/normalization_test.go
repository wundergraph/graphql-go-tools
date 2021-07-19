package graphql

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/jensneuse/graphql-go-tools/internal/pkg/unsafeprinter"
	"github.com/jensneuse/graphql-go-tools/pkg/ast"
	"github.com/jensneuse/graphql-go-tools/pkg/operationreport"
	"github.com/jensneuse/graphql-go-tools/pkg/starwars"
)

func TestRequest_Normalize(t *testing.T) {
	assertNormalizedOperation := func(t *testing.T, expected string, document *ast.Document) {
		t.Helper()

		op := unsafeprinter.PrettyPrint(document, nil)
		assert.Equal(t, expected, op)
	}

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

		assertNormalizedOperation(t, `query Fragments($droidID: ID!){
    hero {
        name
    }
    droid(id: $droidID){
        name
    }
}`, &request.document)

	})

	t.Run("should successfully normalize single query with arguments", func(t *testing.T) {
		schema := starwarsSchema(t)
		request := requestForQuery(t, starwars.FileDroidWithArgQuery)
		documentBeforeNormalization := request.document

		result, err := request.Normalize(schema)
		assert.NoError(t, err)
		assert.NotEqual(t, documentBeforeNormalization, request.document)
		assert.Equal(t, []byte(`{"a":"R2D2"}`), request.document.Input.Variables)
		assert.True(t, result.Successful)
		assert.True(t, request.isNormalized)

		assertNormalizedOperation(t, `query($a: ID!){
    droid(id: $a){
        name
    }
}`, &request.document)
	})

	t.Run("should successfully normalize query and remove unused variables", func(t *testing.T) {
		schema := starwarsSchema(t)
		request := Request{
			OperationName: "MySearch",
			Variables: stringify(map[string]interface{}{
				"s":     "Luke",
				"other": "other",
			}),
			Query: `query MySearch($s: String!, $other: String) {search(name: $s) {...on Human {name}}}`,
		}
		documentBeforeNormalization := request.document

		result, err := request.Normalize(schema)
		assert.NoError(t, err)
		assert.NotEqual(t, documentBeforeNormalization, request.document)
		assert.Equal(t, []byte(`{"s":"Luke"}`), request.document.Input.Variables)
		assert.True(t, result.Successful)
		assert.True(t, request.isNormalized)

		assertNormalizedOperation(t, `query MySearch($s: String!){
    search(name: $s){
        ... on Human {
            name
        }
    }
}`, &request.document)
	})

	t.Run("should successfully normalize multiple queries with arguments", func(t *testing.T) {
		schema := starwarsSchema(t)
		request := requestForQuery(t, starwars.FileMultiQueriesWithArguments)
		request.OperationName = "GetDroid"
		documentBeforeNormalization := request.document

		result, err := request.Normalize(schema)
		assert.NoError(t, err)
		assert.NotEqual(t, documentBeforeNormalization, request.document)
		assert.Equal(t, []byte(`{"a":"1"}`), request.document.Input.Variables)
		assert.True(t, result.Successful)
		assert.True(t, request.isNormalized)

		assertNormalizedOperation(t, `query GetDroid($a: ID!){
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
}`, &request.document)
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
		assert.Equal(t, "graphql error", result.Errors.(OperationValidationErrors)[0].Message)
	})
}
