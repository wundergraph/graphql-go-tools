package http

import (
	"fmt"
	"github.com/jensneuse/graphql-go-tools/pkg/middleware"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/url"
	"runtime"
	"strings"
	"testing"
	"time"

	hackmiddleware "github.com/jensneuse/graphql-go-tools/hack/middleware"
	"github.com/jensneuse/graphql-go-tools/pkg/proxy"
)

// ProxyTestCase is a human understandable proxy test
type ProxyTestCase struct {
	// Schema is the schema exposed to the client
	Schema string
	// ClientRequest is the request from the client in front of the proxy
	ClientRequest string
	// ExpectedProxiedRequest is the rewritten request that is proxied to the backend (origin graphql server)
	ExpectedProxiedRequest string
	// MiddleWares are the proxy middlewares to test
	MiddleWares []middleware.GraphqlMiddleware
	// BackendStatusCode is the status code returned by the backend
	BackendStatusCode int
	// ExpectedProxyStatusCode is the http status code we expect the proxy to return
	ExpectedProxyStatusCode int
	// WantProxyErrorHandlerInvocation indicates if the proxy error handler should be invoced during the test
	WantProxyErrorHandlerInvocation bool
}

// RunTestCase starts a backend server + a proxy and tests a client request against it
func RunTestCase(t *testing.T, testCase ProxyTestCase) {

	backendGraphqlServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := ioutil.ReadAll(r.Body)
		if err != nil {
			t.Fatal(err)
		}
		if strings.TrimSpace(string(body)) != testCase.ExpectedProxiedRequest {
			t.Fatalf("Expected:\n%s\ngot\n%s", testCase.ExpectedProxiedRequest, string(body))
		}

		w.WriteHeader(testCase.BackendStatusCode)
	}))

	defer backendGraphqlServer.Close()

	backendURL, err := url.Parse(backendGraphqlServer.URL)
	if err != nil {
		t.Fatal(err)
	}

	schema := []byte(testCase.Schema)

	requestConfig := proxy.RequestConfig{
		Schema:     &schema,
		BackendURL: *backendURL,
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

	graphqlProxyHttpServer := httptest.NewServer(graphqlProxy)
	defer graphqlProxyHttpServer.Close()

	res, err := http.Post(graphqlProxyHttpServer.URL, "application/graphql", strings.NewReader(testCase.ClientRequest))
	if err != nil {
		t.Error(err)
	}

	if testCase.WantProxyErrorHandlerInvocation != errorHandlerInvoked {
		t.Fatalf("want proxy error handler invocation: %t, got: %t", testCase.WantProxyErrorHandlerInvocation, errorHandlerInvoked)
	}

	if res == nil {
		return
	}

	if res.StatusCode != testCase.ExpectedProxyStatusCode {
		t.Fatalf("want proxy status code: %d, got: %d", testCase.ExpectedProxyStatusCode, res.StatusCode)
	}
}

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
			ExpectedProxyStatusCode:         http.StatusOK,
			WantProxyErrorHandlerInvocation: false,
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
			ExpectedProxyStatusCode:         http.StatusOK,
			WantProxyErrorHandlerInvocation: true,
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
			ExpectedProxyStatusCode:         http.StatusOK,
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
			ExpectedProxyStatusCode:         http.StatusOK,
			WantProxyErrorHandlerInvocation: false,
		})
	})
}

func BenchmarkProxyHandler(b *testing.B) {

	//go printMemUsage()

	es := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := ioutil.ReadAll(r.Body)
		if err != nil {
			b.Error(err)
		}
		if string(body) != assetOutput {
			b.Errorf("Expected %s, got %s", assetOutput, body)
		}
	}))
	defer es.Close()

	schema := []byte(assetSchema)
	backendURL, err := url.Parse(es.URL)
	if err != nil {
		b.Fatal(err)
	}

	requestConfigProvider := proxy.NewStaticRequestConfigProvider(proxy.RequestConfig{
		Schema:     &schema,
		BackendURL: *backendURL,
	})

	ph := NewDefaultProxy(requestConfigProvider, &hackmiddleware.AssetUrlMiddleware{})
	ts := httptest.NewServer(ph)
	defer ts.Close()

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, err := http.Post(ts.URL, "application/graphql", strings.NewReader(assetInput))
		if err != nil {
			b.Error(err)
		}
	}
}

// nolint
func printMemUsage() {
	for {
		time.Sleep(time.Millisecond * time.Duration(1000))
		var m runtime.MemStats
		runtime.ReadMemStats(&m)
		// For info on each, see: https://golang.org/pkg/runtime/#MemStats
		fmt.Printf("Alloc = %v MiB", bToMb(m.Alloc))
		fmt.Printf("\tTotalAlloc = %v MiB", bToMb(m.TotalAlloc))
		fmt.Printf("\tSys = %v MiB", bToMb(m.Sys))
		fmt.Printf("\tNumGC = %v\n", m.NumGC)
	}
}

// nolint
func bToMb(b uint64) uint64 {
	return b / 1024 / 1024
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

const assetInput = `{"query":"query testQueryWithoutHandle {assets(first:1) { id fileName url(transformation: {image: {resize: {width: 100, height: 100}}})}}"}`
const assetOutput = `{"query":"query testQueryWithoutHandle {assets(first:1) {id fileName handle}}"}`

const variableAssetInput = `{"query":"query testQueryWithoutHandle {assets(first: 1) { id fileName url(transformation: {image: {resize: {width: 100, height: 100}}})}}","variables":{"id":1}}`
const variableAssetOutput = `{"query":"query testQueryWithoutHandle {assets(first:1) {id fileName handle}}","variables":{"id":1}}`
