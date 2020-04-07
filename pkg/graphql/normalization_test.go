package graphql

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/jensneuse/graphql-go-tools/pkg/operationreport"
)

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
