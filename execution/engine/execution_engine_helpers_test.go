package engine

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/execution/graphql"
)

type testRoundTripper func(req *http.Request) *http.Response

func (t testRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return t(req), nil
}

type roundTripperTestCase struct {
	expectedHost     string
	expectedPath     string
	expectedBody     string
	expectedMethod   string
	sendStatusCode   int
	sendResponseBody string
}

func createTestRoundTripper(t *testing.T, testCase roundTripperTestCase) testRoundTripper {
	t.Helper()

	return func(req *http.Request) *http.Response {
		t.Helper()

		assert.Equal(t, testCase.expectedHost, req.URL.Host)
		assert.Equal(t, testCase.expectedPath, req.URL.Path)

		if len(testCase.expectedBody) > 0 {
			var receivedBodyBytes []byte
			if req.Body != nil {
				var err error
				receivedBodyBytes, err = io.ReadAll(req.Body)
				require.NoError(t, err)
			}
			require.Equal(t, testCase.expectedBody, string(receivedBodyBytes), "roundTripperTestCase received unexpected body")
		}

		body := bytes.NewBuffer([]byte(testCase.sendResponseBody))
		return &http.Response{StatusCode: testCase.sendStatusCode, Body: io.NopCloser(body)}
	}
}

type conditionalTestCase struct {
	expectedHost   string
	expectedPath   string
	expectedMethod string

	// responses map an expected body to the output that should be sent
	responses map[string]sendResponse
}

type sendResponse struct {
	statusCode int
	body       string
}

func createConditionalTestRoundTripper(t *testing.T, testCase conditionalTestCase) testRoundTripper {
	t.Helper()

	require.True(t, len(testCase.responses) > 0, "no responses defined")

	return func(req *http.Request) *http.Response {
		t.Helper()

		assert.Equal(t, testCase.expectedHost, req.URL.Host)
		assert.Equal(t, testCase.expectedPath, req.URL.Path)

		require.NotNil(t, req.Body, "roundTripperTestCase received nil body")

		gotBody, err := io.ReadAll(req.Body)
		require.NoError(t, err)
		defer req.Body.Close()

		require.Containsf(t, testCase.responses, string(gotBody), "received unexpected body: %v", string(gotBody))
		response := testCase.responses[string(gotBody)]
		return &http.Response{
			StatusCode: response.statusCode,
			Body:       io.NopCloser(bytes.NewBuffer([]byte(response.body))),
		}
	}
}

func stringify(any interface{}) []byte {
	out, _ := json.Marshal(any)
	return out
}

func heroWithArgumentSchema(t *testing.T) *graphql.Schema {
	schemaString := `
		type Query {
			hero(name: String): String
			heroDefault(name: String = "Any"): String
			heroDefaultRequired(name: String! = "AnyRequired"): String
			heroes(names: [String!]!): [String!]
		}`

	schema, err := graphql.NewSchemaFromString(schemaString)
	require.NoError(t, err)
	return schema
}

func moviesSchema(t *testing.T) *graphql.Schema {
	schemaString := `
type Movie {
  id: Int!
  name: String!
  year: Int!
}

type Mutation {
  addToWatchlist(movieID: Int!): Movie
  addToWatchlistWithInput(input: WatchlistInput!): Movie
}

type Query {
  default: String
}

input WatchlistInput {
  id: Int!
}`
	schema, err := graphql.NewSchemaFromString(schemaString)
	require.NoError(t, err)
	return schema
}
