package httpclient

import (
	"github.com/stretchr/testify/assert"
	"github.com/wundergraph/graphql-go-tools/pkg/engine/resolve"
	"net/http"
	"testing"
)

func TestSetRequestHeaders(t *testing.T) {
	expectedHeaderKey := "Test-Header"
	expectedHeaderValue := "test value"
	expectedRequest := resolve.Request{Header: http.Header{expectedHeaderKey: []string{expectedHeaderValue}}}
	ctx := &resolve.Context{Request: expectedRequest}

	actualRequest, _ := buildRequest(ctx, []byte{})

	assert.Equal(t, expectedHeaderValue, actualRequest.Header.Get(expectedHeaderKey))
}
