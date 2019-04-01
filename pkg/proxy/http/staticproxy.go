package http

import (
	"fmt"
	"github.com/jensneuse/graphql-go-tools/pkg/middleware"
	"github.com/jensneuse/graphql-go-tools/pkg/proxy"
	"net/http"
)

type StaticProxy struct {
	handler http.Handler
}

func NewDefaultStaticProxy(config proxy.RequestConfig, middlewares ...middleware.GraphqlMiddleware) *StaticProxy {

	provider := proxy.NewStaticRequestConfigProvider(config)
	handler := NewDefaultProxy(provider, middlewares...)

	return &StaticProxy{
		handler: handler,
	}
}

func (s *StaticProxy) ListenAndServe(addr string) error {
	fmt.Printf("ListenAndServe on: %s\n", addr)
	return http.ListenAndServe(addr, s.handler)
}
