package http

import (
	"bufio"
	"bytes"
	"fmt"
	"github.com/jensneuse/graphql-go-tools/pkg/middleware"
	"github.com/jensneuse/graphql-go-tools/pkg/proxy"
	"github.com/valyala/fasthttp"
	"net/http"
	"sync"
)

type FastStaticProxyConfig struct {
	BackendURL  string
	BackendHost string
	Schema      []byte
	MiddleWares []middleware.GraphqlMiddleware
}

type FastStaticProxy struct {
	prox *FastHttpProxy
}

func NewFastStaticProxy(config FastStaticProxyConfig) *FastStaticProxy {

	prox := &FastHttpProxy{
		Host:           config.BackendURL,
		SchemaProvider: proxy.NewStaticSchemaProvider(config.Schema),
		Invoker:        middleware.NewInvoker(config.MiddleWares...),
		invokeMux:      sync.Mutex{},
		Client:         *http.DefaultClient,
		HandleError: func(err error, w http.ResponseWriter) {
			fmt.Println(err.Error())
		},
		BufferPool: sync.Pool{
			New: func() interface{} {
				return &bytes.Buffer{}
			},
		},
		BufferedReaderPool: sync.Pool{
			New: func() interface{} {
				return &bufio.Reader{}
			},
		},
		HostClient: &fasthttp.HostClient{
			Addr: config.BackendHost,
		},
	}

	return &FastStaticProxy{
		prox: prox,
	}
}

func (f *FastStaticProxy) ListenAndServe(addr string) error {
	//fmt.Printf("ListenAndServe on: %s\n", addr)
	return fasthttp.ListenAndServe(addr, f.prox.HandleRequest)
}
