package graphql

import (
	"fmt"
	"io"
)

type Errors interface {
	error
	WriteResponse(writer io.Writer) (n int, err error)
}

type OperationValidationErrors []OperationValidationError

func (o OperationValidationErrors) Error() string {
	return fmt.Sprintf("operation contains %d error(s)", len(o))
}

func (o OperationValidationErrors) WriteResponse(writer io.Writer) (n int, err error) {
	response := Response{
		Errors: o,
	}

	responseBytes, err := response.Marshal()
	if err != nil {
		return 0, err
	}

	return writer.Write(responseBytes)
}

type OperationValidationError struct {
	Message   string          `json:"message"`
	Locations []ErrorLocation `json:"locations,omitempty"`
	Path      ErrorPath       `json:"path,omitempty"`
}

func (o OperationValidationError) Error() string {
	return o.Message
}

type SchemaValidationErrors []SchemaValidationError

func (s SchemaValidationErrors) Error() string {
	return ""
}

func (s SchemaValidationErrors) WriteResponse(writer io.Writer) (n int, err error) {
	return writer.Write(nil)
}

type SchemaValidationError struct {
}

func (s SchemaValidationError) Error() string {
	return ""
}

type ErrorPath []interface{}

type ErrorLocation struct {
	Line   int `json:"line"`
	Column int `json:"column"`
}
