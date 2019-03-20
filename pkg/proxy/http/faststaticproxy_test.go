package http

import (
	"bytes"
	"github.com/jensneuse/graphql-go-tools/pkg/middleware"
	"github.com/phayes/freeport"
	"github.com/valyala/fasthttp"
	"io/ioutil"
	"net/http"
	"strconv"
	"testing"
	"time"
)

// go test -count=1 -run=none -bench=BenchmarkFastStaticProxy -benchmem -memprofile mem.out && go tool pprof -alloc_space http.test mem.out
// go tool pprof -alloc_space http://localhost:8081/debug/pprof/heap

func BenchmarkFastStaticProxy(b *testing.B) {

	b.SetParallelism(1)

	port, _ := freeport.GetFreePort()
	backendPort := strconv.Itoa(port)
	port, _ = freeport.GetFreePort()
	proxyPort := strconv.Itoa(port)

	fakeBackendHost := "0.0.0.0:" + backendPort
	go startFakeBackend(fakeBackendHost)

	prox := NewFastStaticProxy(FastStaticProxyConfig{
		MiddleWares: []middleware.GraphqlMiddleware{
			&middleware.ContextMiddleware{},
		},
		Schema:      []byte(testSchema),
		BackendURL:  "http://0.0.0.0:" + backendPort + "/query",
		BackendHost: "0.0.0.0:" + backendPort,
	})

	go func() {
		err := prox.ListenAndServe("0.0.0.0:" + proxyPort)
		if err != nil {
			b.Fatal(err)
		}
	}()

	time.Sleep(time.Second)

	http.DefaultTransport.(*http.Transport).MaxIdleConnsPerHost = 100

	b.ResetTimer()
	b.ReportAllocs()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {

			request, err := http.NewRequest(http.MethodPost, "http://0.0.0.0:"+proxyPort+"/query", bytes.NewReader(clientData))
			if err != nil {
				b.Fatal(err)
			}

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

var clientData = []byte(`{"operationName":null,"variables":{},"query":"{\n  documents{\n    owner\n    sensitiveInformation\n  }\n}\n"}`)
var fakeResponse = []byte(`{"data":{"documents":[{"sensitiveInformation":"jsmith"},{"sensitiveInformation":"got proxied"}]}}`)

func fastHTTPHandler(ctx *fasthttp.RequestCtx) {
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
