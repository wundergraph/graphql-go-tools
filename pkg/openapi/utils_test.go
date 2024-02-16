package openapi

import (
	"net/http"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/stretchr/testify/assert"
)

func TestStatusCodeToRange(t *testing.T) {

	t.Run("1XX", func(t *testing.T) {
		for _, code := range []int{100, 120, 130} {
			codeRange, err := statusCodeToRange(code)
			assert.Nil(t, err)
			assert.Equal(t, "1XX", codeRange)
		}
	})

	t.Run("2XX", func(t *testing.T) {
		for _, code := range []int{200, 220, 230} {
			codeRange, err := statusCodeToRange(code)
			assert.Nil(t, err)
			assert.Equal(t, "2XX", codeRange)
		}
	})

	t.Run("3XX", func(t *testing.T) {
		for _, code := range []int{300, 320, 330} {
			codeRange, err := statusCodeToRange(code)
			assert.Nil(t, err)
			assert.Equal(t, "3XX", codeRange)
		}
	})

	t.Run("4XX", func(t *testing.T) {
		for _, code := range []int{400, 420, 430} {
			codeRange, err := statusCodeToRange(code)
			assert.Nil(t, err)
			assert.Equal(t, "4XX", codeRange)
		}
	})

	t.Run("5XX", func(t *testing.T) {
		for _, code := range []int{500, 520, 530} {
			codeRange, err := statusCodeToRange(code)
			assert.Nil(t, err)
			assert.Equal(t, "5XX", codeRange)
		}
	})

	t.Run("Invalid status code", func(t *testing.T) {
		_, err := statusCodeToRange(620)
		assert.ErrorContains(t, err, "unknown status code: 620")
	})
}

func TestConvertStatusCode(t *testing.T) {
	rangeToCode := map[string]int{
		"1xx": 100,
		"2xx": 200,
		"3xx": 300,
		"4xx": 400,
		"5xx": 500,
	}

	for statusRange, expectedCode := range rangeToCode {
		code, err := convertStatusCode(statusRange)
		assert.Nil(t, err)
		assert.Equal(t, expectedCode, code)
	}
}

func TestGetResponseFromOperation(t *testing.T) {
	operation := &openapi3.Operation{
		Responses: openapi3.Responses{
			"200": &openapi3.ResponseRef{Value: &openapi3.Response{}},
			"3XX": &openapi3.ResponseRef{Value: &openapi3.Response{}},
		},
	}
	assert.NotNil(t, getResponseFromOperation(200, operation))
	assert.NotNil(t, getResponseFromOperation(300, operation))
	assert.Nil(t, getResponseFromOperation(400, operation))
}

func Test_sanitizeResponses(t *testing.T) {
	twoHundredResponseRef := &openapi3.ResponseRef{Value: &openapi3.Response{}}
	twoHundredRangeResponseRef := &openapi3.ResponseRef{Value: &openapi3.Response{}}
	threeHundredRangeResponseRef := &openapi3.ResponseRef{Value: &openapi3.Response{}}

	responses := openapi3.Responses{
		"200": twoHundredResponseRef,
		"2XX": twoHundredRangeResponseRef,
		"3XX": threeHundredRangeResponseRef,
	}
	result, err := sanitizeResponses(responses)
	assert.NoError(t, err)
	assert.Equal(t, 2, len(result))
	assert.Equal(t, twoHundredResponseRef, result["200"])
	assert.Equal(t, threeHundredRangeResponseRef, result["3XX"])
}

func Test_getValidResponse(t *testing.T) {
	twoHundredResponseRef := &openapi3.ResponseRef{Value: &openapi3.Response{}}
	twoHundredTwoResponseRef := &openapi3.ResponseRef{Value: &openapi3.Response{}}
	twoHundredRangeResponseRef := &openapi3.ResponseRef{Value: &openapi3.Response{}}
	threeHundredRangeResponseRef := &openapi3.ResponseRef{Value: &openapi3.Response{}}

	responses := openapi3.Responses{
		"200": twoHundredResponseRef,
		"202": twoHundredTwoResponseRef,
		"2XX": twoHundredRangeResponseRef,
		"3XX": threeHundredRangeResponseRef,
	}

	// OpenAPI-to-GraphQL translator mimics IBM/openapi-to-graphql tool. This tool accepts HTTP code 200-299 or 2XX
	// as valid responses. Other status codes are simply ignored. Currently, we follow the same convention.
	statusCode, responseRef, err := getValidResponse(responses)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, statusCode)
	assert.Equal(t, twoHundredResponseRef, responseRef)
}
