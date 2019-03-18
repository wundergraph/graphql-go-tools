package http

import (
	"bufio"
	"bytes"
	"context"
	"github.com/jensneuse/graphql-go-tools/pkg/middleware"
	"github.com/jensneuse/graphql-go-tools/pkg/proxy"
	"io"
	"log"
	"net/http"
	"sync"
)

type Proxy struct {
	SchemaProvider     proxy.SchemaProvider
	Host               string
	InvokerPool        sync.Pool
	Client             http.Client
	HandleError        func(err error, w http.ResponseWriter)
	BufferPool         sync.Pool
	BufferedReaderPool sync.Pool
}

func (p *Proxy) AcceptRequest(ctx context.Context, uri string, body io.ReadCloser, buff *bytes.Buffer) error {

	schema := p.SchemaProvider.GetSchema(uri)

	invoker := p.InvokerPool.Get().(*middleware.Invoker)
	defer p.InvokerPool.Put(invoker)

	err := invoker.SetSchema(schema)
	if err != nil {
		return err
	}

	_, err = buff.ReadFrom(body)
	requestData := buff.Bytes()

	err = invoker.InvokeMiddleWares(ctx, &requestData)
	if err != nil {
		return err
	}

	buff.Reset()

	err = invoker.RewriteRequest(buff)
	if err != nil {
		return err
	}

	return err
}

func (p *Proxy) DispatchRequest(buff *bytes.Buffer) (*http.Response, error) {
	return p.Client.Post(p.Host, "application/graphql", buff)
}

func (p *Proxy) AcceptResponse() {
	panic("implement me")
}

func (p *Proxy) DispatchResponse() {
	panic("implement me")
}

func (p *Proxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {

	buff := p.BufferPool.Get().(*bytes.Buffer)
	buff.Reset()

	err := p.AcceptRequest(r.Context(), r.RequestURI, r.Body, buff)
	if err != nil {
		p.BufferPool.Put(buff)
		p.HandleError(err, w)
		return
	}

	response, err := p.DispatchRequest(buff)
	if err != nil {
		p.BufferPool.Put(buff)
		p.HandleError(err, w)
		return
	}

	// todo: implement the OnResponse handlers

	bufferedReader := p.BufferedReaderPool.Get().(*bufio.Reader)
	bufferedReader.Reset(response.Body)

	_, err = bufferedReader.WriteTo(w)
	if err != nil {
		p.BufferedReaderPool.Put(bufferedReader)
		p.BufferPool.Put(buff)
		p.HandleError(err, w)
		return
	}

	p.BufferedReaderPool.Put(bufferedReader)
	p.BufferPool.Put(buff)
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
}
