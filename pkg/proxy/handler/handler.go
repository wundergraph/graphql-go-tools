package handler

import (
	"bytes"
	"io/ioutil"
	"net/http"

	"github.com/jensneuse/graphql-go-tools/pkg/lookup"
	"github.com/jensneuse/graphql-go-tools/pkg/parser"
	"github.com/jensneuse/graphql-go-tools/pkg/printer"
	"github.com/jensneuse/graphql-go-tools/pkg/proxy"
)

type ProxyHandler struct {
	schema      []byte
	middlewares []proxy.GraphqlMiddleware
	host        string
	http.Handler
}

func NewProxyHandler(schema []byte, host string, middlewares ...proxy.GraphqlMiddleware) *ProxyHandler {
	return &ProxyHandler{
		schema:      schema,
		host:        host,
		middlewares: middlewares,
	}
}

func (ph *ProxyHandler) executeMiddlewares(query []byte) ([]byte, error) {
	p := parser.NewParser()
	err := p.ParseTypeSystemDefinition(ph.schema)
	if err != nil {
		return nil, err
	}

	err = p.ParseExecutableDefinition(query)
	if err != nil {
		return nil, err
	}

	l := lookup.New(p, 256)
	l.SetParser(p)
	w := lookup.NewWalker(1024, 8)
	w.SetLookup(l)
	w.WalkExecutable()

	astPrinter := printer.New()
	astPrinter.SetInput(p, l, w)

	buff := bytes.Buffer{}

	// init done

	mod := parser.NewManualAstMod(p)
	prox := proxy.NewProxy(mod)
	prox.SetMiddleWares(ph.middlewares...)

	prox.SetInput(l, w, p, mod)
	prox.OnQuery()

	astPrinter.PrintExecutableSchema(&buff)
	proxiedQuery := buff.Bytes()
	buff.Reset()

	return proxiedQuery, nil
}



func (ph *ProxyHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	incomingQuery, err := ioutil.ReadAll(r.Body)
	if err != nil {
		panic(err)
	}

	proxiedQuery, err := ph.executeMiddlewares(incomingQuery)
	if err != nil {
		panic(err)
	}

	resp, err := http.Post(ph.host, "application/graphql", bytes.NewReader(proxiedQuery))
	if err != nil {
		panic(err)
	}
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		panic(err)
	}
	if err := resp.Body.Close(); err != nil {
		panic(err)
	}

	rw.WriteHeader(resp.StatusCode)
	_, err = rw.Write(body)
	if err != nil {
		panic(err)
	}
}