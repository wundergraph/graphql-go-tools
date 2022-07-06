package operationreport

import (
	"errors"
	"fmt"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestExternalErrorMessage(t *testing.T) {
	runExternalErrorMessage := func(err error, expectedSuccess bool, expectedMessage string) func(t *testing.T) {
		return func(t *testing.T) {
			msg, ok := ExternalErrorMessage(err, testFormatExternalErrorMessage)
			assert.Equal(t, expectedSuccess, ok)
			assert.Equal(t, expectedMessage, msg)
		}
	}

	t.Run("Passing a non-report returns false",
		runExternalErrorMessage(testErrorLevel1, false, ""),
	)

	t.Run("Passing a report retrieves the inner error",
		runExternalErrorMessage(testWrappedReport, true, externalErrorString),
	)
}

func TestUnwrappedErrorMessage(t *testing.T) {
	actual := UnwrappedErrorMessage(testErrorLevel2)
	assert.Equal(t, testErrorString, actual)
}

const (
	externalErrorString = "example external error 1"
	testErrorString     = "test error string"
)

var testFormatExternalErrorMessage = func(report *Report) string {
	if len(report.ExternalErrors) > 0 {
		return report.ExternalErrors[0].Message
	}
	return ""
}

var testReport = Report{
	InternalErrors: []error{
		errors.New("example internal error"),
	},
	ExternalErrors: []ExternalError{
		{
			Message:   externalErrorString,
			Path:      nil,
			Locations: nil,
		},
		{
			Message: "example external error 2",
		},
	},
}

var testErrorLevel1 = errors.New(testErrorString)
var testErrorLevel2 = fmt.Errorf("level 2: %w", testErrorLevel1)
var testWrappedReport = fmt.Errorf("level 2: %w", testReport)
