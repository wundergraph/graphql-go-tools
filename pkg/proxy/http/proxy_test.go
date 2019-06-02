package http

import (
	"context"
	hackmiddleware "github.com/jensneuse/graphql-go-tools/hack/middleware"
	"github.com/jensneuse/graphql-go-tools/pkg/middleware"
	"github.com/jensneuse/graphql-go-tools/pkg/proxy"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// ProxyTestCase is a human understandable proxy test
type ProxyTestCase struct {
	// Schema is the schema exposed to the client
	Schema string
	// ClientRequest is the request from the client in front of the proxy
	ClientRequest string
	// ClientHeaders are the additional headers to be set on the client request
	ClientHeaders map[string]string
	// ExpectedProxiedRequest is the rewritten request that is proxied to the backend (origin graphql server)
	ExpectedProxiedRequest string
	// MiddleWares are the proxy middlewares to test
	MiddleWares []middleware.GraphqlMiddleware
	// BackendStatusCode is the status code returned by the backend
	BackendStatusCode int
	// BackendResponse is the response from the backend to the proxy
	BackendResponse string
	// WantClientResponseStatusCode is the http status code we expect the proxy to return
	WantClientResponseStatusCode int
	// WantClientResponseBody is the body we're expecting the proxy to return to the proxy
	WantClientResponseBody string
	// WantProxyErrorHandlerInvocation indicates if the proxy error handler should be invoced during the test
	WantProxyErrorHandlerInvocation bool
	// ProxyOnBeforeRequestMiddleware is the middleWares invoked before the proxy http handler
	ProxyOnBeforeRequestMiddleware HttpMiddleware
	// BackendOnBeforeRequestMiddleware is the middleware invoked before the backend handler
	BackendOnBeforeRequestMiddleware HttpMiddleware
	// BackendHeaders are the headers that should be statically set on requests to the backend
	BackendHeaders map[string][]string
}

/* HttpMiddleware wraps a http handler to add additional logic for certain tests

Minimum Example:

	f := func(w http.ResponseWriter, r *http.Request) {
		handler.ServeHTTP(w, r)
	}
	return http.HandlerFunc(f)

*/
type HttpMiddleware func(handler http.Handler) http.Handler

func TestProxy(t *testing.T) {
	t.Run("asset middleware", func(t *testing.T) {
		RunTestCase(t, ProxyTestCase{
			Schema: assetSchema,
			MiddleWares: []middleware.GraphqlMiddleware{
				&hackmiddleware.AssetUrlMiddleware{},
			},
			ClientRequest:                   assetInput,
			ExpectedProxiedRequest:          assetOutput,
			BackendStatusCode:               http.StatusOK,
			WantClientResponseStatusCode:    http.StatusOK,
			WantProxyErrorHandlerInvocation: false,
		})
	})
	t.Run("with backend response", func(t *testing.T) {
		RunTestCase(t, ProxyTestCase{
			Schema:                          assetSchema,
			ClientRequest:                   assetInput,
			ExpectedProxiedRequest:          assetInput,
			BackendStatusCode:               http.StatusOK,
			WantClientResponseStatusCode:    http.StatusOK,
			WantProxyErrorHandlerInvocation: false,
			BackendResponse:                 "testPayload",
			WantClientResponseBody:          "testPayload",
		})
	})
	t.Run("handle backend error correctly", func(t *testing.T) {
		RunTestCase(t, ProxyTestCase{
			Schema: assetSchema,
			MiddleWares: []middleware.GraphqlMiddleware{
				&hackmiddleware.AssetUrlMiddleware{},
			},
			ClientRequest:                   assetInput,
			ExpectedProxiedRequest:          assetOutput,
			BackendStatusCode:               http.StatusInternalServerError,
			WantClientResponseStatusCode:    http.StatusOK,
			WantProxyErrorHandlerInvocation: true,
		})
	})
	t.Run("handle request with variables", func(t *testing.T) {
		RunTestCase(t, ProxyTestCase{
			Schema: assetSchema,
			MiddleWares: []middleware.GraphqlMiddleware{
				&hackmiddleware.AssetUrlMiddleware{},
			},
			ClientRequest:                   variableAssetInput,
			ExpectedProxiedRequest:          variableAssetOutput,
			BackendStatusCode:               http.StatusOK,
			WantClientResponseStatusCode:    http.StatusOK,
			WantProxyErrorHandlerInvocation: false,
		})
	})
	t.Run("handle request response e2e", func(t *testing.T) {
		RunTestCase(t, ProxyTestCase{
			Schema: publicSchema,
			MiddleWares: []middleware.GraphqlMiddleware{
				&middleware.ContextMiddleware{},
			},
			ClientRequest:                   publicQuery,
			ExpectedProxiedRequest:          privateQuery,
			BackendStatusCode:               http.StatusOK,
			WantClientResponseStatusCode:    http.StatusOK,
			WantProxyErrorHandlerInvocation: false,
			ClientHeaders: map[string]string{
				userKey: userValue,
			},
			BackendHeaders: map[string][]string{
				Authorization: {privateAuthHeader},
			},
			BackendResponse:        backendResponse,
			WantClientResponseBody: backendResponse,
			ProxyOnBeforeRequestMiddleware: func(handler http.Handler) http.Handler {
				f := func(w http.ResponseWriter, r *http.Request) {
					headerUserValue := r.Header.Get(userKey)
					if len(headerUserValue) == 0 {
						t.Fatal("want value for header key 'user', missing!")
					}
					ctx := context.WithValue(r.Context(), "user", headerUserValue)
					handler.ServeHTTP(w, r.WithContext(ctx))
				}
				return http.HandlerFunc(f)
			},
			BackendOnBeforeRequestMiddleware: func(handler http.Handler) http.Handler {
				f := func(w http.ResponseWriter, r *http.Request) {
					authHeader := r.Header.Get(Authorization)
					if authHeader != privateAuthHeader {
						t.Fatalf("want header for key 'Authorization': '%s', got: '%s'", privateAuthHeader, authHeader)
					}
					handler.ServeHTTP(w, r)
				}
				return http.HandlerFunc(f)
			},
		})
	})
}

// RunTestCase starts a backend server + a proxy and tests a client request against it
func RunTestCase(t *testing.T, testCase ProxyTestCase) {

	backendHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := ioutil.ReadAll(r.Body)
		if err != nil {
			t.Fatal(err)
		}
		if strings.TrimSpace(string(body)) != testCase.ExpectedProxiedRequest {
			t.Fatalf("Expected:\n%s\ngot\n%s", testCase.ExpectedProxiedRequest, strings.TrimSpace(string(body)))
		}

		w.WriteHeader(testCase.BackendStatusCode)
		if len(testCase.BackendResponse) != 0 {
			_, err = w.Write([]byte(testCase.BackendResponse))
			if err != nil {
				t.Fatal(err)
			}
		}
	})

	var backendGraphqlServer *httptest.Server

	if testCase.BackendOnBeforeRequestMiddleware != nil {
		backendGraphqlServer = httptest.NewServer(testCase.BackendOnBeforeRequestMiddleware(backendHandler))
	} else {
		backendGraphqlServer = httptest.NewServer(backendHandler)
	}

	defer backendGraphqlServer.Close()

	backendURL, err := url.Parse(backendGraphqlServer.URL)
	if err != nil {
		t.Fatal(err)
	}

	schema := []byte(testCase.Schema)

	requestConfig := proxy.RequestConfig{
		Schema:         &schema,
		BackendURL:     *backendURL,
		BackendHeaders: testCase.BackendHeaders,
	}

	requestConfigProvider := proxy.NewStaticRequestConfigProvider(requestConfig)

	graphqlProxy := NewDefaultProxy(requestConfigProvider, testCase.MiddleWares...)

	errorHandlerInvoked := false

	graphqlProxy.HandleError = func(err error, w http.ResponseWriter) {
		errorHandlerInvoked = true
		w.WriteHeader(http.StatusOK)

		if !testCase.WantProxyErrorHandlerInvocation {
			t.Fatal(err)
		}
	}

	var graphqlProxyHttpServer *httptest.Server

	if testCase.ProxyOnBeforeRequestMiddleware != nil {
		graphqlProxyHttpServer = httptest.NewServer(testCase.ProxyOnBeforeRequestMiddleware(graphqlProxy))
	} else {
		graphqlProxyHttpServer = httptest.NewServer(graphqlProxy)
	}

	defer graphqlProxyHttpServer.Close()

	request, err := http.NewRequest(http.MethodPost, graphqlProxyHttpServer.URL, strings.NewReader(testCase.ClientRequest))
	if err != nil {
		t.Error(err)
	}

	request.Header.Set("Content-Type", "application/graphql")

	if testCase.ClientHeaders != nil {
		for key, value := range testCase.ClientHeaders {
			request.Header.Set(key, value)
		}
	}

	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}

	if testCase.WantProxyErrorHandlerInvocation != errorHandlerInvoked {
		t.Fatalf("want proxy error handler invocation: %t, got: %t", testCase.WantProxyErrorHandlerInvocation, errorHandlerInvoked)
	}

	if response == nil {
		t.Fatal("response must not be nil")
	}

	if response.StatusCode != testCase.WantClientResponseStatusCode {
		t.Fatalf("want proxy status code: %d, got: %d", testCase.WantClientResponseStatusCode, response.StatusCode)
	}

	responseBody, _ := ioutil.ReadAll(response.Body)
	actualClientResponseBody := string(responseBody)
	if testCase.WantClientResponseBody != actualClientResponseBody {
		t.Errorf("want response body:\n'%s'\ngot:\n'%s'", testCase.WantClientResponseBody, actualClientResponseBody)
	}
}

const assetSchema = `
schema {
    query: Query
}

type Query {
    assets(first: Int): [Asset]
}

type Asset implements Node {
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

const assetInput = `{"query":"query testQueryWithoutHandle {assets(first:1) {id fileName url(transformation:{image:{resize:{width:100,height:100}}})}}"}`
const assetOutput = `{"query":"query testQueryWithoutHandle {assets(first:1) {id fileName handle}}"}`

const variableAssetInput = `{"query":"query testQueryWithoutHandle {assets(first: 1) { id fileName url(transformation: {image: {resize: {width: 100, height: 100}}})}}","variables":{"id":1}}`
const variableAssetOutput = `{"query":"query testQueryWithoutHandle {assets(first:1) {id fileName handle}}","variables":{"id":1}}`

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

// e2e test data
const (
	publicSchema = `
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
	publicQuery       = `{"query":"query myDocuments {documents {sensitiveInformation}}"}`
	privateQuery      = `{"query":"query myDocuments {documents(user:\"jsmith@example.org\") {sensitiveInformation}}"}`
	privateAuthHeader = "testAuth"
	backendResponse   = `{"data":{"documents":[{"sensitiveInformation":"jsmith"},{"sensitiveInformation":"got proxied"}]}}`
	Authorization     = "Authorization"
	userKey           = "user"
	userValue         = "jsmith@example.org"
)
