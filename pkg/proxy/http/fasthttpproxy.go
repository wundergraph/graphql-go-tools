package http

import (
	"bytes"
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/literal"
	"github.com/jensneuse/graphql-go-tools/pkg/middleware"
	"github.com/jensneuse/graphql-go-tools/pkg/proxy"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
	"github.com/valyala/fasthttp"
	"io"
	"sync"
)

type FastHttpProxy struct {
	requestConfigProvider proxy.RequestConfigProvider
	invokerPool           *middleware.InvokerPool
	userValuePool         sync.Pool
	bufferPool            sync.Pool
	hostClientPool        sync.Pool
}

func (f *FastHttpProxy) RewriteQuery(config proxy.RequestConfig, userValues map[string][]byte, requestURI []byte, query []byte, out io.Writer) error {

	idx, invoker := f.invokerPool.Get()
	defer f.invokerPool.Free(idx)

	err := invoker.SetSchema(*config.Schema)
	if err != nil {
		return err
	}

	err = invoker.InvokeMiddleWares(userValues, query)
	if err != nil {
		return err
	}

	err = invoker.RewriteRequest(out)
	if err != nil {
		return err
	}

	return err
}

func (f *FastHttpProxy) HandleRequest(ctx *fasthttp.RequestCtx) {

	config := f.requestConfigProvider.GetRequestConfig(ctx.RequestURI())

	userValues := f.userValuePool.Get().(map[string][]byte)
	defer f.userValuePool.Put(userValues)

	f.SetUserValues(&userValues, ctx.Request.Header, config.AddHeadersToContext)

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

	buff := f.bufferPool.Get().(*bytes.Buffer)
	buff.Reset()
	defer f.bufferPool.Put(buff)

	err := f.RewriteQuery(config, userValues, ctx.RequestURI(), query, buff)
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

	ctx.Request.SetRequestURIBytes(config.BackendAddr)
	ctx.Request.SetBody(body)

	client := f.hostClientPool.Get().(*fasthttp.HostClient)
	defer f.hostClientPool.Put(client)
	client.Addr = config.BackendHost

	err = client.Do(&ctx.Request, &ctx.Response)
	if err != nil {
		ctx.Error(err.Error(), fasthttp.StatusInternalServerError)
		return
	}

	// todo: implement the OnResponse handlers / do something with the response
}

func (f *FastHttpProxy) SetUserValues(userValues *map[string][]byte, header fasthttp.RequestHeader, addHeaders [][]byte) {
	for key := range *userValues {
		delete(*userValues, key)
	}
	for _, key := range addHeaders {
		(*userValues)[string(key)] = header.PeekBytes(key)
	}
}
