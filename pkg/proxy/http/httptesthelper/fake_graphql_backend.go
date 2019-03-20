package main

import (
	"github.com/valyala/fasthttp"
)

var fakeResponse = []byte(`{"data":{"documents":[{"sensitiveInformation":"jsmith"},{"sensitiveInformation":"got proxied"}]}}`)

func fastHTTPHandler(ctx *fasthttp.RequestCtx) {
	ctx.Response.SetStatusCode(fasthttp.StatusOK)
	ctx.Response.SetBody(fakeResponse)
}

func main() {
	err := fasthttp.ListenAndServe("0.0.0.0:8080", fastHTTPHandler)
	if err != nil {
		panic(err)
	}
}
