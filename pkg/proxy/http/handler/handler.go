package http

import (
	"bytes"
	"github.com/jensneuse/graphql-go-tools/handler/request"
	"github.com/jensneuse/graphql-go-tools/middleware"
	"io/ioutil"
	"net/http"
)

type RequestHandler struct {
	schemaProvider SchemaProvider
	host           string
	requestHandler *request.Handler
	http.Handler
}

func NewProxyHandler(host string, schemaProvider SchemaProvider, middlewares ...middleware.GraphqlMiddleware) *RequestHandler {
	return &RequestHandler{
		schemaProvider: schemaProvider,
		host:           host,
		requestHandler: request.NewRequestHandler(middlewares...),
	}
}

/*func (ph *RequestHandler) executeMiddlewares(query []byte) ([]byte, error) {
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
}*/

func (p *RequestHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {

	input, err := ioutil.ReadAll(r.Body)
	if err != nil {
		panic(err)
	}

	var schema []byte
	p.schemaProvider.GetSchema(r.RequestURI, &schema)

	err = p.requestHandler.SetSchema(&schema)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	err = p.requestHandler.HandleRequest(&input)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	/*proxiedQuery, err := ph.executeMiddlewares(incomingQuery)
	if err != nil {
		panic(err)
	}*/

	resp, err := http.Post(p.host, "application/graphql", bytes.NewReader(input))
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
