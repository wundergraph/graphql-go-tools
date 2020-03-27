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
