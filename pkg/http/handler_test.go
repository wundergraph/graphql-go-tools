package http

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/jensneuse/abstractlogger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jensneuse/graphql-go-tools/pkg/execution"
)

func TestGraphQLHTTPRequestHandler_ServeHTTP(t *testing.T) {

}

func TestGraphQLHTTPRequestHandler_IsWebsocketUpgrade(t *testing.T) {
	handler := NewGraphqlHTTPHandlerFunc(nil, nil, nil).(*GraphQLHTTPRequestHandler)

	t.Run("should return false if upgrade header does not exist", func(t *testing.T) {
		req, err := http.NewRequest(http.MethodGet, "/", nil)
		require.NoError(t, err)

		isWebsocketUpgrade := handler.isWebsocketUpgrade(req)
		assert.False(t, isWebsocketUpgrade)
	})

	t.Run("should return false if upgrade header does not contain websocket", func(t *testing.T) {
		req, err := http.NewRequest(http.MethodGet, "/", nil)
		require.NoError(t, err)

		req.Header = map[string][]string{
			httpHeaderUpgrade: {"any"},
		}

		isWebsocketUpgrade := handler.isWebsocketUpgrade(req)
		assert.False(t, isWebsocketUpgrade)
	})

	t.Run("should return true if upgrade header contains websocket", func(t *testing.T) {
		req, err := http.NewRequest(http.MethodGet, "/", nil)
		require.NoError(t, err)

		req.Header = map[string][]string{
			httpHeaderUpgrade: {"any", "websocket"},
		}

		isWebsocketUpgrade := handler.isWebsocketUpgrade(req)
		assert.True(t, isWebsocketUpgrade)
	})
}

func TestGraphQLHTTPRequestHandler_ExtraVariables(t *testing.T) {
	handler := NewGraphqlHTTPHandlerFunc(nil, nil, nil).(*GraphQLHTTPRequestHandler)

	t.Run("should create extra variables successfully", func(t *testing.T) {
		req, err := http.NewRequest(http.MethodGet, "http://localhost:8080/path", nil)
		require.NoError(t, err)

		req.Header = map[string][]string{
			httpHeaderUpgrade: {"websocket"},
		}

		req.AddCookie(&http.Cookie{})

		extraVariablesBytes := &bytes.Buffer{}
		err = handler.extraVariables(req, extraVariablesBytes)

		assert.NoError(t, err)

		expectedJson := fmt.Sprintf("%s\n", `{"request":{"cookies":{},"headers":{"Cookie":"=","Upgrade":"websocket"},"host":"localhost:8080","method":"GET","uri":""}}`)
		assert.Equal(t, expectedJson, extraVariablesBytes.String())
	})
}

func starWarsSchema() []byte {
	schema := "schema { query: Query } type Query { hero: Hero } type Hero { name: String }"
	return []byte(schema)
}

func starWarsHeroQueryRequestBody(t *testing.T) []byte {
	return starWarsRequestBody(t, "query { hero { name } }", nil)
}

func invalidQueryRequestBody(t *testing.T) []byte {
	return starWarsRequestBody(t, "query { trap { meme } }", nil)
}

func starWarsRequestBody(t *testing.T, query string, variables map[string]interface{}) []byte {
	var variableJsonBytes []byte
	if len(variables) > 0 {
		var err error
		variableJsonBytes, err = json.Marshal(variables)
		require.NoError(t, err)
	}

	body := execution.GraphqlRequest{
		OperationName: "",
		Variables:     variableJsonBytes,
		Query:         query,
	}

	jsonBytes, err := json.Marshal(body)
	require.NoError(t, err)

	return jsonBytes
}

func newStarWarsExecutionHandler(t *testing.T) *execution.Handler {
	executionHandler, err := execution.NewHandler(starWarsSchema(), execution.PlannerConfiguration{}, nil, abstractlogger.NoopLogger)
	require.NoError(t, err)

	return executionHandler
}
