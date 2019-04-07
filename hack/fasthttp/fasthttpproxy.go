package fasthttp

import (
	"bytes"
	"context"
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/literal"
	"github.com/jensneuse/graphql-go-tools/pkg/proxy"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
	"github.com/valyala/fasthttp"
	"io"
)

type Proxy proxy.Proxy

func (f *Proxy) RewriteQuery(config proxy.RequestConfig, ctx context.Context, requestURI []byte, query []byte, out io.Writer) error {

	idx, invoker := f.InvokerPool.Get()
	defer f.InvokerPool.Free(idx)

	err := invoker.SetSchema(*config.Schema)
	if err != nil {
		return err
	}

	err = invoker.InvokeMiddleWares(ctx, query)
	if err != nil {
		return err
	}

	err = invoker.RewriteRequest(out)
	if err != nil {
		return err
	}

	return err
}

func (f *Proxy) HandleRequest(ctx *fasthttp.RequestCtx) {

	config := f.RequestConfigProvider.GetRequestConfig(ctx)
	goctx := f.SetContextValues(ctx, &ctx.Request.Header, config.AddHeadersToContext)

	body := ctx.Request.Body()

	result := gjson.GetBytes(body, "query")
	var query []byte
	if result.Index > 0 {
		query = body[result.Index : result.Index+len(result.Raw)]
	} else {
		query = []byte(result.Raw)
	}

	query = bytes.TrimPrefix(query, literal.QUOTE)
	query = bytes.TrimSuffix(query, literal.QUOTE)

	buff := f.BufferPool.Get().(*bytes.Buffer)
	buff.Reset()
	defer f.BufferPool.Put(buff)

	err := f.RewriteQuery(config, goctx, ctx.RequestURI(), query, buff)
	if err != nil {
		ctx.Error(err.Error(), fasthttp.StatusInternalServerError)
		return
	}

	body = body[:0]

	body, err = sjson.SetBytes(body, "query", buff.Bytes())
	if err != nil {
		ctx.Error(err.Error(), fasthttp.StatusInternalServerError)
		return
	}

	ctx.Request.SetRequestURIBytes([]byte(config.BackendURL.String()))
	ctx.Request.SetBody(body)

	client := f.ClientPool.Get().(*fasthttp.HostClient)
	defer f.ClientPool.Put(client)
	client.Addr = config.BackendURL.Host

	err = client.Do(&ctx.Request, &ctx.Response)
	if err != nil {
		ctx.Error(err.Error(), fasthttp.StatusInternalServerError)
		return
	}

	// todo: implement the OnResponse handlers / do something with the response
}

func (f *Proxy) SetContextValues(ctx context.Context, header *fasthttp.RequestHeader, addHeaders [][]byte) context.Context {
	for _, key := range addHeaders {
		ctx = context.WithValue(ctx, string(key), header.PeekBytes(key))
	}
	return ctx
}
