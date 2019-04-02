package http

import (
	"context"
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/literal"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/jensneuse/graphql-go-tools/pkg/middleware"
	"github.com/jensneuse/graphql-go-tools/pkg/proxy"
)

func TestProxyIntegration(t *testing.T) {

	fakeResponse := `{"data":{"documents":[{"sensitiveInformation":"jsmith"},{"sensitiveInformation":"got proxied"}]}}`

	// middleware that extracts a "security token" from a header
	checkUserMiddleware := func(h http.Handler) http.Handler {
		f := func(w http.ResponseWriter, r *http.Request) {
			userToken := r.Header.Get("user")
			if userToken == "" {
				t.Fatal("No user token found")
			} else {
				ctx := context.WithValue(r.Context(), "user", append(literal.QUOTE, append([]byte(userToken), literal.QUOTE...)...))
				h.ServeHTTP(w, r.WithContext(ctx))
			}
		}
		return http.HandlerFunc(f)
	}

	// the handler for the endpoint graphql system
	endpointHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if authHeader != privateAuthHeader {
			t.Fatalf("Expected:\n%s\ngot\n%s\n\n", privateAuthHeader, authHeader)
		}


		body, err := ioutil.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
		}

		want := privateQuery
		got := string(body)

		if want != got {
			t.Fatalf("Expected:\n%s\ngot\n%s\n\n", want, got)
		}

		_, err = w.Write([]byte(fakeResponse))
	})

	endpointServer := httptest.NewServer(endpointHandler)
	defer endpointServer.Close()

	schema := []byte(publicSchema)
	backendURL, err := url.Parse(endpointServer.URL)
	if err != nil {
		t.Fatal(err)
	}

	headers := make(http.Header)
	headers.Set("Authorization", privateAuthHeader)
	schemaProvider := proxy.NewStaticRequestConfigProvider(proxy.RequestConfig{
		Schema:     &schema,
		BackendURL: *backendURL,
		BackendHeaders: headers,
	})

	// the handler for the graphql proxy
	proxyHandler := NewDefaultProxy(schemaProvider, &middleware.ContextMiddleware{})
	proxyServer := httptest.NewServer(checkUserMiddleware(proxyHandler))
	defer proxyServer.Close()

	t.Run("Test proxy handler", func(t *testing.T) {
		request, err := http.NewRequest(http.MethodPost, proxyServer.URL, strings.NewReader(publicQuery))
		if err != nil {
			t.Error(err)
		}
		request.Header.Set("user", "jsmith@example.org")
		request.Header.Set("Content-Type", "application/graphql")
		resp, err := http.DefaultClient.Do(request)
		if err != nil {
			t.Fatal(err)
		}

		responseBody, _ := ioutil.ReadAll(resp.Body)

		gotResponse := string(responseBody)
		wantResponse := fakeResponse
		if wantResponse != gotResponse {
			t.Errorf("want response: '%s', got: '%s'", wantResponse, gotResponse)
		}
	})
}

const publicSchema = `
directive @addArgumentFromContext(
	name: String!
	contextKey: String!
) on FIELD_DEFINITION

scalar String

schema {
	query: Query
}

type Query {
	documents: [Document] @addArgumentFromContext(name: "user",contextKey: "user")
}

type Document implements Node {
	owner: String
	sensitiveInformation: String
}
`

/*

the public schema for reference

schema {
	query: Query
}

type Query {
	documents(user: String!): [Document]
}

type Document implements Node {
	owner: String
	sensitiveInformation: String
}
*/

const publicQuery = `{"query":"query myDocuments {documents {sensitiveInformation}}"}
`

const privateQuery = `{"operationName":"","query":"query myDocuments {documents(user:\"jsmith@example.org\") {sensitiveInformation}}"}
`
const privateAuthHeader = "testAuth"