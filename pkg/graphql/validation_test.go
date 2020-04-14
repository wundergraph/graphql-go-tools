package graphql

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/jensneuse/graphql-go-tools/pkg/operationreport"
)

func Test_operationValidationResultFromReport(t *testing.T) {
	t.Run("should return result for valid when report does not have errors", func(t *testing.T) {
		report := operationreport.Report{}
		result, err := operationValidationResultFromReport(report)

		assert.NoError(t, err)
		assert.Equal(t, ValidationResult{Valid: true, Errors: nil}, result)
	})

	t.Run("should return validation error and internal error when report contain them", func(t *testing.T) {
		internalErr := errors.New("errors occurred")
		externalErr := operationreport.ExternalError{
			Message:   "graphql error",
			Path:      nil,
			Locations: nil,
		}

		report := operationreport.Report{}
		report.AddInternalError(internalErr)
		report.AddExternalError(externalErr)

		result, err := operationValidationResultFromReport(report)

		assert.Error(t, err)
		assert.Equal(t, internalErr, err)
		assert.False(t, result.Valid)
		assert.Len(t, result.Errors.(OperationValidationErrors), 1)
		assert.Equal(t, "graphql error", result.Errors.(OperationValidationErrors)[0].Message)
	})
}
