package fasthttp

import (
	"bytes"
	"fmt"
	"github.com/jensneuse/graphql-go-tools/pkg/middleware"
	"github.com/jensneuse/graphql-go-tools/pkg/proxy"
	"github.com/phayes/freeport"
	"github.com/valyala/fasthttp"
	"io/ioutil"
	"net/http"
	"net/url"
	"strconv"
	"testing"
	"time"
)

// go test -count=1 -run=none -bench=BenchmarkFastStaticProxy -benchmem -memprofile mem.out && go tool pprof -alloc_space http.test mem.out
// go tool pprof -alloc_space http://localhost:8081/debug/pprof/heap

func TestFastStaticProxy(t *testing.T) {

	port, _ := freeport.GetFreePort()
	backendPort := strconv.Itoa(port)
	port, _ = freeport.GetFreePort()
	proxyPort := strconv.Itoa(port)

	fakeBackendHost := "0.0.0.0:" + backendPort
	go startFakeBackend(fakeBackendHost)

	schema := []byte(testSchema)
	backendURL, err := url.Parse("http://0.0.0.0:" + backendPort + "/query")
	if err != nil {
		t.Fatal(err)
	}

	prox := NewFastStaticProxy(FastStaticProxyConfig{
		MiddleWares: []middleware.GraphqlMiddleware{
			&middleware.ContextMiddleware{},
		},
		RequestConfigProvider: proxy.NewStaticSchemaProvider(proxy.RequestConfig{
			Schema:      &schema,
			BackendURL:  *backendURL,
			AddHeadersToContext: [][]byte{
				[]byte("user"),
			},
		}),
	})

	go func() {
		err := prox.ListenAndServe("0.0.0.0:" + proxyPort)
		if err != nil {
			t.Fatal(err)
		}
	}()

	time.Sleep(time.Millisecond)

	request, err := http.NewRequest(http.MethodPost, "http://0.0.0.0:"+proxyPort+"/query", bytes.NewReader(clientData))
	if err != nil {
		t.Fatal(err)
	}

	request.Header.Set("user", `"jsmith@example.org"`)

	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}

	if response.StatusCode != http.StatusOK {
		t.Fatalf("want: %d, got: %d", http.StatusOK, response.StatusCode)
	}

	body, err := ioutil.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(body, fakeResponse) {
		t.Fatalf("want:\n%s\ngot:\n%s", string(fakeResponse), string(body))
	}
}

func BenchmarkFastStaticProxy(b *testing.B) {

	b.SetParallelism(4)

	port, _ := freeport.GetFreePort()
	backendPort := strconv.Itoa(port)
	port, _ = freeport.GetFreePort()
	proxyPort := strconv.Itoa(port)

	fakeBackendHost := "0.0.0.0:" + backendPort
	go startFakeBackend(fakeBackendHost)

	schema := []byte(testSchema)
	backendURL, err := url.Parse("http://0.0.0.0:" + backendPort + "/query")
	if err != nil {
		b.Fatal(err)
	}

	prox := NewFastStaticProxy(FastStaticProxyConfig{
		MiddleWares: []middleware.GraphqlMiddleware{
			&middleware.ContextMiddleware{},
		},
		RequestConfigProvider: proxy.NewStaticSchemaProvider(proxy.RequestConfig{
			Schema:      &schema,
			BackendURL:  *backendURL,
			AddHeadersToContext: [][]byte{
				[]byte("user"),
			},
		}),
	})

	go func() {
		err := prox.ListenAndServe("0.0.0.0:" + proxyPort)
		if err != nil {
			b.Fatal(err)
		}
	}()

	time.Sleep(time.Millisecond)

	http.DefaultTransport.(*http.Transport).MaxIdleConnsPerHost = 100

	b.ResetTimer()
	b.ReportAllocs()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {

			request, err := http.NewRequest(http.MethodPost, "http://0.0.0.0:"+proxyPort+"/query", bytes.NewReader(clientData))
			if err != nil {
				b.Fatal(err)
			}

			request.Header.Set("user", `"jsmith@example.org"`)

			response, err := http.DefaultClient.Do(request)
			if err != nil {
				b.Fatal(err)
			}

			if response.StatusCode != http.StatusOK {
				b.Fatalf("want: %d, got: %d", http.StatusOK, response.StatusCode)
			}

			body, err := ioutil.ReadAll(response.Body)
			if err != nil {
				b.Fatal(err)
			}

			if !bytes.Equal(body, fakeResponse) {
				b.Fatalf("want:\n%s\ngot:\n%s", string(fakeResponse), string(body))
			}
		}
	})
}

var clientData = []byte("{\"operationName\":null,\"variables\":{},\"query\":\"{\n  documents{\n    owner\n    sensitiveInformation\n  }\n}\n\"}")
var fakeResponse = []byte(`{"data":{"documents":[{"sensitiveInformation":"jsmith"},{"sensitiveInformation":"got proxied"}]}}`)
var wantBackendRequestBody = []byte(`{"query":"{documents(user:\"jsmith@example.org\") {owner sensitiveInformation}}"}`)

func fastHTTPHandler(ctx *fasthttp.RequestCtx) {

	got := ctx.PostBody()
	if !bytes.Equal(got, wantBackendRequestBody) {
		panic(fmt.Errorf("want:\n%s\ngot:\n%s", string(wantBackendRequestBody), string(got)))
	}

	ctx.Response.SetStatusCode(fasthttp.StatusOK)
	ctx.Response.SetBody(fakeResponse)
}

func startFakeBackend(addr string) {
	err := fasthttp.ListenAndServe(addr, fastHTTPHandler)
	if err != nil {
		panic(err)
	}
}

const testSchema = `
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

type Document {
	owner: String
	sensitiveInformation: String
}
`
