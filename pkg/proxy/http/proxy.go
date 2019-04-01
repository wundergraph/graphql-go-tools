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
	proxy.Proxy
	HandleError func(err error, w http.ResponseWriter)
}

type ProxyRequest struct {
	proxy.Request
	Proxy Proxy
}

type GraphqlJsonRequest struct {
	OperationName string `json:"operationName"`
	Query         string `json:"query"`
}

func (pr *ProxyRequest) AcceptRequest(buff *bytes.Buffer) error {

	idx, invoker := pr.Proxy.InvokerPool.Get()
	defer pr.Proxy.InvokerPool.Free(idx)

	err := invoker.SetSchema(*pr.Config.Schema)
	if err != nil {
		return err
	}

	var graphqlJsonRequest GraphqlJsonRequest
	err = json.NewDecoder(pr.Body).Decode(&graphqlJsonRequest)
	if err != nil {
		return err
	}

	query := []byte(graphqlJsonRequest.Query)

	err = invoker.InvokeMiddleWares(pr.Context, query) // TODO: fix nil
	if err != nil {
		return err
	}

	err = invoker.RewriteRequest(buff)
	if err != nil {
		return err
	}

	return err
}

func (pr *ProxyRequest) DispatchRequest(buff *bytes.Buffer) (io.ReadCloser, error) {

	req := GraphqlJsonRequest{
		Query: buff.String(),
	}

	out := bytes.Buffer{}
	err := json.NewEncoder(&out).Encode(req)
	if err != nil {
		return nil, err
	}

	client := pr.Proxy.ClientPool.Get().(*http.Client)
	defer pr.Proxy.ClientPool.Put(client)
	response, err := client.Post(pr.Config.BackendURL.String(), "application/json", &out)
	if err != nil {
		return nil, err
	}
	return response.Body, nil
}

func (pr *ProxyRequest) AcceptResponse() {
	panic("implement me")
}

func (pr *ProxyRequest) DispatchResponse() {
	panic("implement me")
}

func (p *Proxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	buff := p.BufferPool.Get().(*bytes.Buffer)
	buff.Reset()

	config := p.RequestConfigProvider.GetRequestConfig(r.Context())
	pr := ProxyRequest{
		Proxy: *p,
	}
	pr.Config = &config
	pr.RequestURL = *r.URL
	pr.Body = r.Body
	pr.Context = p.SetContextValues(r.Context(), r.Header, config.AddHeadersToContext)

	err := pr.AcceptRequest(buff)
	if err != nil {
		p.BufferPool.Put(buff)
		p.HandleError(err, w)
		return
	}

	responseBody, err := pr.DispatchRequest(buff)
	if err != nil {
		p.BufferPool.Put(buff)
		r.Body.Close()
		return
	}

	// todo: implement the OnResponse handlers

	bufferedReader := bufio.NewReader(responseBody)
	_, err = bufferedReader.WriteTo(w)
	if err != nil {
		p.BufferPool.Put(buff)
		r.Body.Close()
		responseBody.Close()
		p.HandleError(err, w)
		return
	}

	p.BufferPool.Put(buff)
	r.Body.Close()
	responseBody.Close()
}

func (f *Proxy) SetContextValues(ctx context.Context, header http.Header, addHeaders [][]byte) context.Context {
	for i := range addHeaders {
		key := string(addHeaders[i])
		ctx = context.WithValue(ctx, key, header.Get(key))
	}
	return ctx
}

func NewDefaultProxy(provider proxy.RequestConfigProvider, middlewares ...middleware.GraphqlMiddleware) *Proxy {
	prx := Proxy{
		HandleError: func(err error, w http.ResponseWriter) {
			log.Printf("Error: %v", err)
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(err.Error()))
		},
	}
	prx.RequestConfigProvider = provider
	prx.InvokerPool = middleware.NewInvokerPool(8, middlewares...)
	prx.BufferPool = sync.Pool{
		New: func() interface{} {
			return &bytes.Buffer{}
		},
	}
	prx.ClientPool = sync.Pool{
		New: func() interface{} {
			return http.DefaultClient
		},
	}

	return &prx
}
