package operationreport

import (
	"errors"
	"fmt"
	"testing"
)

func TestConvenienceFunction(t *testing.T) {
	report := &Report{
		InternalErrors: []error{
			errors.New("internal error"),
		},
		ExternalErrors: []ExternalError{
			{
				Message:   "i'm the external one",
				Path:      nil,
				Locations: nil,
			},
			{
				Message: "I'm the second one",
			},
		},
	}

	err := fmt.Errorf("ast: %w", report)
	err = fmt.Errorf("merge: %w", err)
	//err = errors.New(fmt.Sprintf("another: %s", err.Error()))

	fmt.Println(err)

	fmt.Println("==== DASHBOARD:")
	// Dashboard code
	dashboardFormatFunc := func(report *Report) string {
		if len(report.ExternalErrors) > 0 {
			return report.ExternalErrors[0].Message
		}
		return ""
	}

	if msg, ok := GetExternalErrorMessage(err, dashboardFormatFunc); ok {
		fmt.Println(msg)
	}

	fmt.Println("==== GATEWAY:")
	gatewayFormatFunc := func(report *Report) string {
		if len(report.ExternalErrors) > 0 {
			return fmt.Sprintf("library error: %s", report.ExternalErrors[0].Message)
		}
		return ""
	}

	if msg, ok := GetExternalErrorMessage(err, gatewayFormatFunc); ok {
		fmt.Println(msg)
	}

	noReportErr := errors.New("no report found")
	noReportErr = fmt.Errorf("wrapped: %w", noReportErr)
	_, ok := GetExternalErrorMessage(noReportErr, gatewayFormatFunc)
	if !ok {
		fmt.Println(noReportErr)
	}
	fmt.Println(ok)
}
