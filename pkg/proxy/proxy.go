package proxy

import (
	"github.com/jensneuse/graphql-go-tools/pkg/lookup"
	"github.com/jensneuse/graphql-go-tools/pkg/parser"
)

type Proxy struct {
	l           *lookup.Lookup
	p           *parser.Parser
	w           *lookup.Walker
	mod         *parser.ManualAstMod
	middleWares []GraphqlMiddleware
}

type GraphqlMiddleware interface {
	OnRequest(l *lookup.Lookup, w *lookup.Walker, parser *parser.Parser, mod *parser.ManualAstMod)
	OnResponse(response *[]byte, l *lookup.Lookup, w *lookup.Walker, parser *parser.Parser, mod *parser.ManualAstMod) error
}

func NewProxy(mod *parser.ManualAstMod) *Proxy {
	return &Proxy{
		mod: mod,
	}
}

func (p *Proxy) SetMiddleWares(middleWares ...GraphqlMiddleware) {
	p.middleWares = middleWares
}

func (p *Proxy) SetInput(l *lookup.Lookup, w *lookup.Walker, parser *parser.Parser, mod *parser.ManualAstMod) {
	p.p = parser
	p.l = l
	p.w = w
}

func (p *Proxy) OnQuery() {
	for i := range p.middleWares {
		p.middleWares[i].OnRequest(p.l, p.w, p.p, p.mod)
	}
}
