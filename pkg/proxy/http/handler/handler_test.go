package http

import (
	"github.com/jensneuse/graphql-go-tools/hack/middleware/example"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestProxyHandler(t *testing.T) {
	es := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := ioutil.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
		}
		if string(body) != assetOutput {
			t.Errorf("Expected %s, got %s", assetOutput, body)
		}
	}))
	defer es.Close()

	ph := NewProxyHandler([]byte(assetSchema), es.URL, &example.AssetUrlMiddleware{})
	ts := httptest.NewServer(ph)
	defer ts.Close()

	t.Run("Test proxy handler", func(t *testing.T) {
		_, err := http.Post(ts.URL, "application/graphql", strings.NewReader(assetInput))
		if err != nil {
			t.Error(err)
		}
	})
}

const assetSchema = `
schema {
    query: Query
}

type Query {
    assets(first: Int): [Asset]
}

type Asset implements Node @RequestMiddleware {
    status: Status!
    updatedAt: DateTime!
    createdAt: DateTime!
    id: ID!
    handle: String!
    fileName: String!
    height: Float
    width: Float
    size: Float
    mimeType: String
    url: String!
}`

const assetInput = `query testQueryWithoutHandle {
  								assets(first: 1) {
    							id
    							fileName
    							url(transformation: {image: {resize: {width: 100, height: 100}}})
  							}
						}`

const assetOutput = "query testQueryWithoutHandle {assets(first:1) {id fileName handle}}"
