package main

import (
	"github.com/valyala/fasthttp"
	"github.com/valyala/fasthttp/pprofhandler"
)

func fastHTTPHandler(ctx *fasthttp.RequestCtx) {
	ctx.Response.SetStatusCode(fasthttp.StatusOK)
}

func main() {

	go runPprof()

	err := fasthttp.ListenAndServe("0.0.0.0:8080", fastHTTPHandler)
	if err != nil {
		panic(err)
	}
}

func runPprof() {
	err := fasthttp.ListenAndServe("0.0.0.0:8081", pprofhandler.PprofHandler)
	if err != nil {
		panic(err)
	}
}
