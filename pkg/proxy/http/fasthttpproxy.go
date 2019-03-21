package http

import (
	"bytes"
	"github.com/jensneuse/graphql-go-tools/pkg/middleware"
	"github.com/jensneuse/graphql-go-tools/pkg/proxy"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
	"github.com/valyala/fasthttp"
	"io"
	"net/http"
	"sync"
)

type FastHttpProxy struct {
	SchemaProvider     proxy.SchemaProvider
	Host               string
	InvokerPool        *middleware.InvokerPool
	UserValuePool      sync.Pool
	invokeMux          sync.Mutex
	Client             http.Client
	HandleError        func(err error, w http.ResponseWriter)
	BufferPool         sync.Pool
	BufferedReaderPool sync.Pool
	HostClient         *fasthttp.HostClient
}

func (f *FastHttpProxy) RewriteQuery(userValues map[string][]byte, requestURI []byte, query []byte, out io.Writer) error {

	schema := f.SchemaProvider.GetSchema(requestURI)

	idx, invoker := f.InvokerPool.Get()
	defer f.InvokerPool.Free(idx)

	err := invoker.SetSchema(schema)
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

	userValues := f.UserValuePool.Get().(map[string][]byte)
	f.CleanUserValues(&userValues)
	defer f.UserValuePool.Put(userValues)

	userValues["user"] = ctx.Request.Header.Peek("user")

	body := ctx.Request.Body()

	result := gjson.GetBytes(body, "query")
	var query []byte
	if result.Index > 0 {
		query = body[result.Index : result.Index+len(result.Raw)]
	} else {
		body = []byte(result.Raw)
	}

	buff := f.BufferPool.Get().(*bytes.Buffer)
	defer f.BufferPool.Put(buff)

	err := f.RewriteQuery(userValues, ctx.RequestURI(), query, buff)
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

	ctx.Request.SetBody(body)

	err = f.HostClient.Do(&ctx.Request, &ctx.Response)
	if err != nil {
		ctx.Error(err.Error(), fasthttp.StatusInternalServerError)
		return
	}

	// todo: implement the OnResponse handlers / do something with the response
}

func (f *FastHttpProxy) CleanUserValues(values *map[string][]byte) {
	for key := range *values {
		delete(*values, key)
	}
}
