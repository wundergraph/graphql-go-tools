package http

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jensneuse/abstractlogger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGraphQLHTTPRequestHandler_HandleHTTP(t *testing.T) {
	handler := NewGraphqlHTTPHandlerFunc(newStarWarsExecutionHandler(t), abstractlogger.NoopLogger, nil).(*GraphQLHTTPRequestHandler)

	t.Run("should return 400 Bad Request when query does not fit to schema", func(t *testing.T) {
		bodyBytes := invalidQueryRequestBody(t)
		req, err := http.NewRequest(http.MethodPost, "http://localhost:8080/graphql", bytes.NewBuffer(bodyBytes))
		require.NoError(t, err)

		w := httptest.NewRecorder()
		handler.handleHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("should successfully handle http request and return 200 OK", func(t *testing.T) {
		bodyBytes := starWarsHeroQueryRequestBody(t)
		req, err := http.NewRequest(http.MethodPost, "http://localhost:8080/graphql", bytes.NewBuffer(bodyBytes))
		require.NoError(t, err)

		w := httptest.NewRecorder()
		handler.handleHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Contains(t, httpContentTypeApplicationJson, w.Header().Get(httpHeaderContentType))
		assert.Equal(t, `{"data":null}`, w.Body.String())
	})
}
