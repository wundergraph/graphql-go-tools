package handler

import (
	"bytes"
	"github.com/jensneuse/graphql-go-tools/pkg/middleware"
	"io/ioutil"
	"net/http"
	"sync"
)

type HttpProxyHandler struct {
	schemaProvider SchemaProvider
	host           string
	invokerPool    sync.Pool
	middleWares    []middleware.GraphqlMiddleware
	http.Handler
}

func NewHttpProxyHandler(host string, schemaProvider SchemaProvider, graphqlMiddleWares ...middleware.GraphqlMiddleware) *HttpProxyHandler {
	return &HttpProxyHandler{
		schemaProvider: schemaProvider,
		host:           host,
		invokerPool: sync.Pool{
			New: func() interface{} {
				return middleware.NewInvoker(graphqlMiddleWares...)
			},
		},
	}
}

func (p *HttpProxyHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {

	input, err := ioutil.ReadAll(r.Body)
	if err != nil {
		panic(err)
	}

	var schema []byte
	p.schemaProvider.GetSchema(r.RequestURI, &schema)

	invoker := p.invokerPool.Get().(*middleware.Invoker)
	defer p.invokerPool.Put(invoker)

	err = invoker.SetSchema(&schema)
	if err != nil {
		panic(err)
	}

	err = invoker.InvokeMiddleWares(r.Context(), &input)
	if err != nil {
		panic(err)
	}

	buff := bytes.Buffer{}

	err = invoker.RewriteRequest(&buff)
	if err != nil {
		panic(err)
	}

	resp, err := http.Post(p.host, "application/graphql", &buff)
	if err != nil {
		panic(err)
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		panic(err)
	}
	if err := resp.Body.Close(); err != nil {
		panic(err)
	}

	w.WriteHeader(resp.StatusCode)
	_, err = w.Write(body)
	if err != nil {
		panic(err)
	}
}
