package graphql

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestOperationValidationErrors_Error(t *testing.T) {
	validationErrs := OperationValidationErrors{
		OperationValidationError{},
	}

	assert.Equal(t, "operation contains 1 error(s)", validationErrs.Error())
}

func TestOperationValidationErrors_WriteResponse(t *testing.T) {
	validationErrs := OperationValidationErrors{
		OperationValidationError{
			Message: "error in operation",
		},
	}

	buf := new(bytes.Buffer)
	n, err := validationErrs.WriteResponse(buf)

	expectedResponse := `{"errors":[{"message":"error in operation"}]}`

	assert.NoError(t, err)
	assert.Greater(t, n, 0)
	assert.Equal(t, expectedResponse, buf.String())
}

func TestOperationValidationError_Error(t *testing.T) {
	validatonErr := OperationValidationError{
		Message:   "error in operation",
		Locations: nil,
		Path:      nil,
	}

	assert.Equal(t, "error in operation", validatonErr.Error())
}

func TestOperationValidationErrors_Count(t *testing.T) {
	validationErrs := OperationValidationErrors{
		OperationValidationError{
			Message: "error in operation",
		},
	}

	assert.Equal(t, 1, validationErrs.Count())
}

func TestOperationValidationErrors_ErrorByIndex(t *testing.T) {
	existingValidationError := OperationValidationError{
		Message: "error in operation",
	}

	validationErrs := OperationValidationErrors{
		existingValidationError,
	}

	assert.Equal(t, existingValidationError, validationErrs.ErrorByIndex(0))
	assert.Nil(t, validationErrs.ErrorByIndex(1))
}

func TestSchemaValidationErrors_Error(t *testing.T) {
	validationErrs := SchemaValidationErrors{
		SchemaValidationError{
			Message: "there can be only one query type in schema",
		},
	}

	assert.Equal(t, "schema contains 1 error(s)", validationErrs.Error())
}

func TestSchemaValidationErrors_Count(t *testing.T) {
	validationErrs := SchemaValidationErrors{
		SchemaValidationError{
			Message: "there can be only one query type in schema",
		},
	}

	assert.Equal(t, 1, validationErrs.Count())
}

func TestSchemaValidationErrors_ErrorByIndex(t *testing.T) {
	existingValidationError := SchemaValidationError{
		Message: "there can be only one query type in schema",
	}

	validationErrs := SchemaValidationErrors{
		existingValidationError,
	}

	assert.Equal(t, existingValidationError, validationErrs.ErrorByIndex(0))
	assert.Nil(t, validationErrs.ErrorByIndex(1))
}

func TestSchemaValidationError_Error(t *testing.T) {
	validationError := SchemaValidationError{
		Message: "there can be only one query type in schema",
	}

	assert.Equal(t, "there can be only one query type in schema", validationError.Error())
}
