package graphql

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type testRoundTripper func(req *http.Request) *http.Response

func (t testRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return t(req), nil
}

type roundTripperTestCase struct {
	expectedHost     string
	expectedPath     string
	expectedBody     string
	sendStatusCode   int
	sendResponseBody string
}

func createTestRoundTripper(t *testing.T, testCase roundTripperTestCase) testRoundTripper {
	return func(req *http.Request) *http.Response {
		assert.Equal(t, testCase.expectedHost, req.URL.Host)
		assert.Equal(t, testCase.expectedPath, req.URL.Path)

		if len(testCase.expectedBody) > 0 {
			var receivedBodyBytes []byte
			if req.Body != nil {
				var err error
				receivedBodyBytes, err = io.ReadAll(req.Body)
				require.NoError(t, err)
			}
			require.Equal(t, testCase.expectedBody, string(receivedBodyBytes), "roundTripperTestCase body do not match")
		}

		body := bytes.NewBuffer([]byte(testCase.sendResponseBody))
		return &http.Response{StatusCode: testCase.sendStatusCode, Body: io.NopCloser(body)}
	}
}

func stringify(any interface{}) []byte {
	out, _ := json.Marshal(any)
	return out
}

func heroWithArgumentSchema(t *testing.T) *Schema {
	schemaString := `
		type Query {
			hero(name: String): String
			heroDefault(name: String = "Any"): String
			heroDefaultRequired(name: String! = "AnyRequired"): String
			heroes(names: [String!]!): [String!]
		}`

	schema, err := NewSchemaFromString(schemaString)
	require.NoError(t, err)
	return schema
}

func moviesSchema(t *testing.T) *Schema {
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
	schema, err := NewSchemaFromString(schemaString)
	require.NoError(t, err)
	return schema
}
