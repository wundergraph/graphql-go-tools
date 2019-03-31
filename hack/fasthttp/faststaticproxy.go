package fasthttp

import (
	"bytes"
	"github.com/jensneuse/graphql-go-tools/pkg/middleware"
	"github.com/jensneuse/graphql-go-tools/pkg/proxy"
	"github.com/valyala/fasthttp"
	"sync"
)

type FastStaticProxy struct {
	prox *Proxy
}

func NewFastStaticProxy(requestConfigProvider proxy.RequestConfigProvider, middlewares ...middleware.GraphqlMiddleware) *FastStaticProxy {

	prox := &Proxy{
		RequestConfigProvider: requestConfigProvider,
		InvokerPool:           middleware.NewInvokerPool(8, middlewares...),
		BufferPool: sync.Pool{
			New: func() interface{} {
				return &bytes.Buffer{}
			},
		},
		ClientPool: sync.Pool{
			New: func() interface{} {
				return &fasthttp.HostClient{}
			},
		},
	}

	return &FastStaticProxy{
		prox: prox,
	}
}

func (f *FastStaticProxy) ListenAndServe(addr string) error {
	return fasthttp.ListenAndServe(addr, f.prox.HandleRequest)
}
