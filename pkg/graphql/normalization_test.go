package graphql

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/jensneuse/graphql-go-tools/pkg/operationreport"
	"github.com/jensneuse/graphql-go-tools/pkg/starwars"
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

	t.Run("should successfully normalize the request", func(t *testing.T) {
		schema := starwarsSchema(t)
		request := requestForQuery(t, starwars.FileFragmentsQuery)
		documentBeforeNormalization := request.document

		result, err := request.Normalize(schema)
		assert.NoError(t, err)
		assert.NotEqual(t, documentBeforeNormalization, request.document)
		assert.True(t, result.Successful)
		assert.True(t, request.isNormalized)
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
