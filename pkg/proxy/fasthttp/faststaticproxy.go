package fasthttp

import (
	"bytes"
	"github.com/jensneuse/graphql-go-tools/pkg/middleware"
	"github.com/jensneuse/graphql-go-tools/pkg/proxy"
	"github.com/valyala/fasthttp"
	"sync"
)

type FastStaticProxy struct {
	prox *FastHttpProxy
}

func NewFastStaticProxy(requestConfigProvider proxy.RequestConfigProvider, middlewares ...middleware.GraphqlMiddleware) *FastStaticProxy {

	prox := &FastHttpProxy{
		requestConfigProvider: requestConfigProvider,
		invokerPool:           middleware.NewInvokerPool(8, middlewares...),
		userValuePool: sync.Pool{
			New: func() interface{} {
				return make(map[string][]byte)
			},
		},
		bufferPool: sync.Pool{
			New: func() interface{} {
				return &bytes.Buffer{}
			},
		},
		hostClientPool: sync.Pool{
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
