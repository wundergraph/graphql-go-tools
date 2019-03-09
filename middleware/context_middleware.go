package middleware

import (
	"github.com/jensneuse/graphql-go-tools/pkg/lookup"
	"github.com/jensneuse/graphql-go-tools/pkg/parser"
)

type ContextMiddleware struct{}

func (a *ContextMiddleware) OnResponse(response *[]byte, l *lookup.Lookup, w *lookup.Walker, parser *parser.Parser, mod *parser.ManualAstMod) error {
	return nil
}

func (a *ContextMiddleware) OnRequest(l *lookup.Lookup, w *lookup.Walker, parser *parser.Parser, mod *parser.ManualAstMod) error {
	return nil
}
