package http

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"sync"

	"github.com/jensneuse/graphql-go-tools/pkg/middleware"
	"github.com/jensneuse/graphql-go-tools/pkg/proxy"
)

type Proxy struct {
	proxy.Proxy
	HandleError func(err error, w http.ResponseWriter)
}

type ProxyRequest struct {
	proxy.Request
	Proxy *Proxy
}

type GraphqlJsonRequest struct {
	OperationName string `json:"operationName,omitempty"`
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
	headers := make(http.Header)
	if pr.Config.BackendHeaders != nil {
		headers = pr.Config.BackendHeaders
	}
	request := http.Request{
		Method: "POST",
		URL:    &pr.Config.BackendURL,
		Header: headers,
		Body:   ioutil.NopCloser(bytes.NewReader(out.Bytes())),
	}

	request.Header.Set("Content-Type", "application/json")
	response, err := client.Do(&request)
	if err != nil {
		return nil, err
	} else if response.StatusCode >= 400 {
		return nil, fmt.Errorf("received status code %d, body %s", response.StatusCode, response.Body)
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
		Proxy: p,
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
		p.HandleError(err, w)
		return
	}

	// todo: implement the OnResponse handlers

	_, err = io.Copy(w, responseBody)
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
			_, _ = w.Write([]byte(err.Error()))
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
