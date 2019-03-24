package http

import (
	"bufio"
	"bytes"
	"fmt"
	"github.com/jensneuse/graphql-go-tools/pkg/middleware"
	"github.com/jensneuse/graphql-go-tools/pkg/proxy"
	"net/http"
	"sync"
)

type StaticProxyConfig struct {
	MiddleWares           []middleware.GraphqlMiddleware
	RequestConfigProvider proxy.RequestConfigProvider
}

type StaticProxy struct {
	handler http.Handler
}

func NewStaticProxy(config StaticProxyConfig) *StaticProxy {

	handler := &Proxy{
		RequestConfigProvider: config.RequestConfigProvider,
		InvokerPool: sync.Pool{
			New: func() interface{} {
				return middleware.NewInvoker(config.MiddleWares...)
			},
		},
		Client: *http.DefaultClient,
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
	}

	return &StaticProxy{
		handler: handler,
	}
}

func (s *StaticProxy) ListenAndServe(addr string) error {
	fmt.Printf("ListenAndServe on: %s\n", addr)
	return http.ListenAndServe(addr, s.handler)
}
