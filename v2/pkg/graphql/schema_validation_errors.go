package graphql

import (
	"fmt"
	"io"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/operationreport"
)

type SchemaValidationErrors []SchemaValidationError

func schemaValidationErrorsFromOperationReport(report operationreport.Report) (errors SchemaValidationErrors) {
	if len(report.ExternalErrors) == 0 {
		return nil
	}

	for _, externalError := range report.ExternalErrors {
		validationError := SchemaValidationError{
			Message: externalError.Message,
		}

		errors = append(errors, validationError)
	}

	return errors
}

func (s SchemaValidationErrors) Error() string {
	return fmt.Sprintf("schema contains %d error(s)", s.Count())
}

func (s SchemaValidationErrors) WriteResponse(writer io.Writer) (n int, err error) {
	return writer.Write(nil)
}

func (s SchemaValidationErrors) Count() int {
	return len(s)
}

func (s SchemaValidationErrors) ErrorByIndex(i int) error {
	if i >= s.Count() {
		return nil
	}
	return s[i]
}

type SchemaValidationError struct {
	Message string `json:"message"`
}

func (s SchemaValidationError) Error() string {
	return s.Message
}
