package graphqlerrors

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/operationreport"
)

func TestOperationValidationErrors_Error(t *testing.T) {
	validationErrs := RequestErrors{
		RequestError{
			Message: "a single error",
			Locations: []operationreport.Location{
				{
					Line:   1,
					Column: 1,
				},
			},
			Path: ErrorPath{
				astPath: []ast.PathItem{
					{
						Kind:       ast.FieldName,
						ArrayIndex: 0,
						FieldName:  []byte("hello"),
					},
				},
			},
		},
	}

	assert.Equal(t, "a single error, locations: [{Line:1 Column:1}], path: [hello]", validationErrs.Error())
}

func TestOperationValidationErrors_WriteResponse(t *testing.T) {
	validationErrs := RequestErrors{
		RequestError{
			Message: "error in operation",
			Locations: []operationreport.Location{
				{
					Line:   1,
					Column: 1,
				},
			},
			Path: ErrorPath{
				astPath: []ast.PathItem{
					{
						Kind:       ast.FieldName,
						ArrayIndex: 0,
						FieldName:  []byte("hello"),
					},
				},
			},
		},
	}

	buf := new(bytes.Buffer)
	n, err := validationErrs.WriteResponse(buf)

	expectedResponse := `{"errors":[{"message":"error in operation","locations":[{"line":1,"column":1}],"path":["hello"]}],"data":null}`

	assert.NoError(t, err)
	assert.Greater(t, n, 0)
	assert.Equal(t, expectedResponse, buf.String())
}

func TestOperationValidationError_Error(t *testing.T) {
	validatonErr := RequestError{
		Message: "error in operation",
		Locations: []operationreport.Location{
			{
				Line:   1,
				Column: 1,
			},
		},
		Path: ErrorPath{
			astPath: []ast.PathItem{
				{
					Kind:       ast.FieldName,
					ArrayIndex: 0,
					FieldName:  []byte("hello"),
				},
			},
		},
	}

	assert.Equal(t, "error in operation, locations: [{Line:1 Column:1}], path: [hello]", validatonErr.Error())
}

func TestOperationValidationErrors_Count(t *testing.T) {
	validationErrs := RequestErrors{
		RequestError{
			Message: "error in operation",
		},
	}

	assert.Equal(t, 1, validationErrs.Count())
}

func TestOperationValidationErrors_ErrorByIndex(t *testing.T) {
	existingValidationError := RequestError{
		Message: "error in operation",
	}

	validationErrs := RequestErrors{
		existingValidationError,
	}

	assert.Equal(t, existingValidationError, validationErrs.ErrorByIndex(0))
	assert.Nil(t, validationErrs.ErrorByIndex(1))
}
