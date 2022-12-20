// Package operationreport helps generating the errors object for a GraphQL Operation.
package operationreport

import (
	"errors"
	"fmt"
)

type Report struct {
	InternalErrors []error
	ExternalErrors []ExternalError
}

func (r Report) Error() string {
	out := ""
	for i := range r.InternalErrors {
		if i != 0 {
			out += "\n"
		}
		out += fmt.Sprintf("internal: %s", r.InternalErrors[i].Error())
	}
	if len(out) > 0 && len(r.ExternalErrors) > 0 {
		out += "\n"
	}
	for i := range r.ExternalErrors {
		if i != 0 {
			out += "\n"
		}
		out += fmt.Sprintf("external: %s, locations: %+v, path: %v", r.ExternalErrors[i].Message, r.ExternalErrors[i].Locations, r.ExternalErrors[i].Path)
	}
	return out
}

func (r *Report) HasErrors() bool {
	return len(r.InternalErrors) > 0 || len(r.ExternalErrors) > 0
}

func (r *Report) Reset() {
	r.InternalErrors = r.InternalErrors[:0]
	r.ExternalErrors = r.ExternalErrors[:0]
}

func (r *Report) AddInternalError(err error) {
	r.InternalErrors = append(r.InternalErrors, err)
}

func (r *Report) AddExternalError(gqlError ExternalError) {
	r.ExternalErrors = append(r.ExternalErrors, gqlError)
}

type FormatExternalErrorMessage func(report *Report) string

func ExternalErrorMessage(err error, formatFunction FormatExternalErrorMessage) (message string, ok bool) {
	var report Report
	if errors.As(err, &report) {
		msg := formatFunction(&report)
		return msg, true
	}
	return "", false
}

func UnwrappedErrorMessage(err error) string {
	for result := err; result != nil; result = errors.Unwrap(result) {
		err = result
	}
	return err.Error()
}
