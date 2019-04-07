package middleware

import (
	"context"
	"github.com/jensneuse/graphql-go-tools/pkg/lookup"
	"github.com/jensneuse/graphql-go-tools/pkg/parser"
)

// GraphqlMiddleware is the interface to be implemented when writing middlewares
type GraphqlMiddleware interface {
	// PrepareSchema is used to bring the schema in a valid state
	// Example usages might be:
	// - adding necessary directives to the schema, e.g. adding the context directive so that the context middleware works
	// - define the graphql internal scalar types so that the validation middleware can do its thing
	PrepareSchema(ctx context.Context, l *lookup.Lookup, w *lookup.Walker, parser *parser.Parser, mod *parser.ManualAstMod) error
	// OnRequest is the handler func for a request from the client
	// this can be used to transform the query and/or variables before sending it to the backend
	OnRequest(ctx context.Context, l *lookup.Lookup, w *lookup.Walker, parser *parser.Parser, mod *parser.ManualAstMod) error
	// OnResponse is the handler func for the response from the backend server
	// this can be used to transform the response before sending the result back to the client
	OnResponse(ctx context.Context, response *[]byte, l *lookup.Lookup, w *lookup.Walker, parser *parser.Parser, mod *parser.ManualAstMod) (err error)
}
