package http

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/jensneuse/graphql-go-tools/pkg/middleware"
	"github.com/jensneuse/graphql-go-tools/pkg/proxy"
	"github.com/valyala/fasthttp"
	"io"
	"net/http"
	"sync"
)

type FastHttpProxy struct {
	SchemaProvider     proxy.SchemaProvider
	Host               string
	Invoker            *middleware.Invoker
	invokeMux          sync.Mutex
	Client             http.Client
	HandleError        func(err error, w http.ResponseWriter)
	BufferPool         sync.Pool
	BufferedReaderPool sync.Pool
	HostClient         *fasthttp.HostClient
}

func (f *FastHttpProxy) RewriteQuery(ctx context.Context, uri string, query *[]byte, out io.Writer) error {

	schema := f.SchemaProvider.GetSchema(uri)

	f.invokeMux.Lock()

	err := f.Invoker.SetSchema(schema)
	if err != nil {
		f.invokeMux.Unlock()
		return err
	}

	err = f.Invoker.InvokeMiddleWares(ctx, query)
	if err != nil {
		f.invokeMux.Unlock()
		return err
	}

	err = f.Invoker.RewriteRequest(out)
	if err != nil {
		f.invokeMux.Unlock()
		return err
	}

	f.invokeMux.Unlock()
	return err
}

func (f *FastHttpProxy) HandleRequest(ctx *fasthttp.RequestCtx) {

	c := context.WithValue(ctx, "user", ctx.Request.Header.Peek("user"))

	body := ctx.Request.Body()

	var graphqlJsonRequest GraphqlJsonRequest
	err := json.Unmarshal(body, &graphqlJsonRequest)
	if err != nil {
		ctx.Error(err.Error(), fasthttp.StatusInternalServerError)
		return
	}

	query := []byte(graphqlJsonRequest.Query)

	buff := f.BufferPool.Get().(*bytes.Buffer)
	defer f.BufferPool.Put(buff)

	err = f.RewriteQuery(c, ctx.URI().String(), &query, buff)
	if err != nil {
		ctx.Error(err.Error(), fasthttp.StatusInternalServerError)
		return
	}

	req := GraphqlJsonRequest{
		Query: buff.String(),
	}

	ctx.Request.ResetBody()

	err = json.NewEncoder(ctx.Request.BodyWriter()).Encode(req)
	if err != nil {
		ctx.Error(err.Error(), fasthttp.StatusInternalServerError)
		return
	}

	err = f.HostClient.Do(&ctx.Request, &ctx.Response)
	if err != nil {
		ctx.Error(err.Error(), fasthttp.StatusInternalServerError)
		return
	}

	// todo: implement the OnResponse handlers / do something with the response
}
