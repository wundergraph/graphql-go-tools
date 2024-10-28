package graphqlerrors

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/operationreport"
)

const (
	httpStatusOK = 200
)

type Errors interface {
	error
	WriteResponse(writer io.Writer) (n int, err error)
	Count() int
	ErrorByIndex(i int) error
}

type RequestErrors []RequestError

func RequestErrorsFromError(err error) RequestErrors {
	if errors, ok := err.(RequestErrors); ok {
		return errors
	}
	if report, ok := err.(operationreport.Report); ok {
		if len(report.ExternalErrors) == 0 {
			return RequestErrors{
				{
					Message: "Internal Error",
				},
			}
		}
		var errors RequestErrors
		for _, externalError := range report.ExternalErrors {
			errors = append(errors, RequestError{
				Message:   externalError.Message,
				Locations: externalError.Locations,
				Path: ErrorPath{
					astPath: externalError.Path,
				},
			})
		}
		return errors
	}
	return RequestErrors{
		{
			Message: err.Error(),
		},
	}
}

func RequestErrorsFromOperationReport(report operationreport.Report) (errors RequestErrors) {
	_, errors = requestErrorsFromOperationReportWithStatusCode(report, false)
	return errors
}

func RequestErrorsFromOperationReportWithStatusCode(report operationreport.Report) (statusCode int, errors RequestErrors) {
	return requestErrorsFromOperationReportWithStatusCode(report, true)
}

func requestErrorsFromOperationReportWithStatusCode(report operationreport.Report, overrideStatusCode bool) (statusCode int, errors RequestErrors) {
	statusCode = httpStatusOK
	if len(report.ExternalErrors) == 0 {
		return statusCode, nil
	}

	for _, externalError := range report.ExternalErrors {
		locations := make([]operationreport.Location, 0)
		for _, reportLocation := range externalError.Locations {
			loc := operationreport.Location{
				Line:   reportLocation.Line,
				Column: reportLocation.Column,
			}

			locations = append(locations, loc)
		}

		validationError := RequestError{
			Message:   externalError.Message,
			Path:      ErrorPath{astPath: externalError.Path},
			Locations: locations,
		}

		if !overrideStatusCode {
			errors = append(errors, validationError)
			continue
		}

		if externalError.ExtensionCode != "" {
			validationError.Extensions = &Extensions{
				Code: externalError.ExtensionCode,
			}
			validationError.Path = ErrorPath{}
		}

		if externalError.StatusCode > 0 && statusCode == httpStatusOK {
			statusCode = externalError.StatusCode
		}

		errors = append(errors, validationError)
	}

	return statusCode, errors
}

func (o RequestErrors) Error() string {
	if len(o) > 0 { // avoid panic ...
		return o.ErrorByIndex(0).Error()
	}
	return "no error" // ... so, this should never be returned
}

// WriteResponse writes the given request errors to the http.ResponseWriter.
// It should only be used to write errors that are happening before the execution of the query e.g. validation errors.
func (o RequestErrors) WriteResponse(writer io.Writer) (n int, err error) {
	response := Response{
		Errors: o,
	}

	responseBytes, err := response.Marshal()
	if err != nil {
		return 0, err
	}

	return writer.Write(responseBytes)
}

func (o RequestErrors) Count() int {
	return len(o)
}

func (o RequestErrors) ErrorByIndex(i int) error {
	if i >= o.Count() {
		return nil
	}

	return o[i]
}

type Extensions struct {
	Code string `json:"code"`
}

type RequestError struct {
	Message   string                     `json:"message"`
	Locations []operationreport.Location `json:"locations,omitempty"`
	Path      ErrorPath                  `json:"path"`
	// Extensions is typically used for Apollo compatibility
	Extensions *Extensions `json:"extensions,omitempty"`
}

func (o RequestError) MarshalJSON() ([]byte, error) {
	if o.Path.Len() == 0 {
		return json.Marshal(struct {
			Message    string                     `json:"message"`
			Locations  []operationreport.Location `json:"locations,omitempty"`
			Extensions *Extensions                `json:"extensions,omitempty"`
		}{
			Message:    o.Message,
			Locations:  o.Locations,
			Extensions: o.Extensions,
		})
	}
	path, err := o.Path.MarshalJSON()
	if err != nil {
		return nil, err
	}
	return json.Marshal(struct {
		Message    string                     `json:"message"`
		Locations  []operationreport.Location `json:"locations,omitempty"`
		Path       json.RawMessage            `json:"path"`
		Extensions *Extensions                `json:"extensions,omitempty"`
	}{
		Message:    o.Message,
		Locations:  o.Locations,
		Path:       path,
		Extensions: o.Extensions,
	})
}

func (o RequestError) Error() string {
	return fmt.Sprintf("%s, locations: %+v, path: %s", o.Message, o.Locations, o.Path.String())
}

type ErrorPath struct {
	astPath ast.Path
}

func (e *ErrorPath) String() string {
	return e.astPath.String()
}

func (e *ErrorPath) MarshalJSON() ([]byte, error) {
	return json.Marshal(e.astPath)
}

func (e *ErrorPath) Len() int {
	return len(e.astPath)
}
