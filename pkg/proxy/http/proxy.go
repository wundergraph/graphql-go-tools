package http

import (
	"bytes"
	"context"
	"github.com/jensneuse/graphql-go-tools/pkg/middleware"
	"github.com/jensneuse/graphql-go-tools/pkg/proxy"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"sync"
)

type Proxy struct {
	SchemaProvider proxy.SchemaProvider
	Host           string
	InvokerPool    sync.Pool
	Client         http.Client
	HandleError    func(err error, w http.ResponseWriter)
}

func (p *Proxy) AcceptRequest(uri string, body io.ReadCloser, ctx context.Context) (*bytes.Buffer, error) {
	var schema []byte
	p.SchemaProvider.GetSchema(uri, &schema)

	invoker := p.InvokerPool.Get().(*middleware.Invoker)
	defer p.InvokerPool.Put(invoker)

	err := invoker.SetSchema(&schema)
	if err != nil {
		return nil, err
	}

	input, err := ioutil.ReadAll(body)
	if err != nil {
		return nil, err
	}

	err = invoker.InvokeMiddleWares(ctx, &input)
	if err != nil {
		return nil, err
	}

	buff := bytes.Buffer{}

	err = invoker.RewriteRequest(&buff)
	if err != nil {
		return nil, err
	}

	return &buff, err
}

func (p *Proxy) DispatchRequest(input []byte) ([]byte, error) {
	resp, err := p.Client.Post(p.Host, "application/graphql", bytes.NewReader(input))
	if err != nil {
		return nil, err
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if err := resp.Body.Close(); err != nil {
		return nil, err
	}
	return body, nil
}

func (p *Proxy) AcceptResponse() {
	panic("implement me")
}

func (p *Proxy) DispatchResponse() {
	panic("implement me")
}

func (p *Proxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	buff, err := p.AcceptRequest(r.RequestURI, r.Body, r.Context())
	if err != nil {
		p.HandleError(err, w)
	}
	resp, err := p.DispatchRequest(buff.Bytes())
	if err != nil {
		p.HandleError(err, w)
	}

	// todo: implement the OnResponse handlers

	if _, err := w.Write(resp); err != nil {
		p.HandleError(err, w)
	}
}

func NewDefaultProxy(host string, provider proxy.SchemaProvider, middlewares ...middleware.GraphqlMiddleware) *Proxy {
	return &Proxy{
		Host:           host,
		SchemaProvider: provider,
		InvokerPool: sync.Pool{
			New: func() interface{} {
				return middleware.NewInvoker(middlewares...)
			},
		},
		Client: *http.DefaultClient,
		HandleError: func(err error, w http.ResponseWriter) {
			log.Printf("Error: %v", err)
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(err.Error()))
		},
	}
}
