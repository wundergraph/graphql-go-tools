package proxy

import (
	"bytes"
	"context"
	"github.com/jensneuse/graphql-go-tools/pkg/middleware"
	"io"
	"net/url"
	"sync"
)

type Proxy struct {
	RequestConfigProvider RequestConfigProvider
	InvokerPool           *middleware.InvokerPool
	BufferPool            sync.Pool
	ClientPool            sync.Pool
}

type Request struct {
	Config     *RequestConfig
	RequestURL url.URL
	Body       io.Reader
	Context    context.Context
	GraphQLRequest middleware.GraphQLRequest
}

type RequestInterface interface {
	AcceptRequest(buff *bytes.Buffer) error
	DispatchRequest(buff *bytes.Buffer) (io.ReadCloser, error)
	AcceptResponse()
	DispatchResponse()
}
