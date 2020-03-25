package graphql

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestUnmarshalRequest(t *testing.T) {
	t.Run("should return error when request is empty", func(t *testing.T) {
		requestBytes := []byte("")
		requestBuffer := bytes.NewBuffer(requestBytes)

		request, err := UnmarshalRequest(requestBuffer)

		assert.Error(t, err)
		assert.Equal(t, ErrEmptyRequest, err)
		assert.Nil(t, request)
	})

	t.Run("should return error when query is empty", func(t *testing.T) {
		requestBytes := []byte(`{"query": ""}`)
		requestBuffer := bytes.NewBuffer(requestBytes)

		request, err := UnmarshalRequest(requestBuffer)

		assert.Error(t, err)
		assert.Equal(t, ErrEmptyRequest, err)
		assert.Nil(t, request)
	})

	t.Run("should successfully unmarshal request", func(t *testing.T) {
		requestBytes := []byte(`{"operation_name": "Hello", "variables": "", "query": "query Hello { hello }"}`)
		requestBuffer := bytes.NewBuffer(requestBytes)

		request, err := UnmarshalRequest(requestBuffer)

		assert.NoError(t, err)
		assert.Equal(t, "Hello", request.OperationName)
		assert.Equal(t, "query Hello { hello }", request.Query)
	})
}
