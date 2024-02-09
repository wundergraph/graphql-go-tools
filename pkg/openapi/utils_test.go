package openapi

import (
	"github.com/getkin/kin-openapi/openapi3"
	"github.com/stretchr/testify/assert"
	"testing"
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
