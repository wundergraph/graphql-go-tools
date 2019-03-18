package http

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
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

type GraphqlJsonRequest struct {
	OperationName string `json:"operationName"`
	Query         string `json:"query"`
}

func (p *Proxy) AcceptRequest(ctx context.Context, uri string, body io.ReadCloser, buff *bytes.Buffer) error {

	schema := p.SchemaProvider.GetSchema(uri)

	invoker := p.InvokerPool.Get().(*middleware.Invoker)
	defer p.InvokerPool.Put(invoker)

	err := invoker.SetSchema(schema)
	if err != nil {
		return err
	}

	var graphqlJsonRequest GraphqlJsonRequest
	err = json.NewDecoder(body).Decode(&graphqlJsonRequest)
	if err != nil {
		return err
	}

	query := []byte(graphqlJsonRequest.Query)

	err = invoker.InvokeMiddleWares(ctx, &query)
	if err != nil {
		return err
	}

	err = invoker.RewriteRequest(buff)
	if err != nil {
		return err
	}

	return err
}

func (p *Proxy) DispatchRequest(buff *bytes.Buffer) (*http.Response, error) {

	req := GraphqlJsonRequest{
		Query: buff.String(),
	}

	out := bytes.Buffer{}
	err := json.NewEncoder(&out).Encode(req)
	if err != nil {
		return nil, err
	}

	//return p.Client.Post(p.Host, "application/graphql", buff)
	return p.Client.Post(p.Host, "application/json", &out)
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

	ctx := r.Context()
	ctx = context.WithValue(ctx, "user", []byte(r.Header.Get("user")))

	err := p.AcceptRequest(ctx, r.RequestURI, r.Body, buff)
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
