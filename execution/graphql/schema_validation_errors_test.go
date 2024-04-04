package graphql

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

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
