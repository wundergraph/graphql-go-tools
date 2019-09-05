package middleware

import (
	"context"
	"github.com/jensneuse/graphql-go-tools/pkg/lookup"
	"github.com/jensneuse/graphql-go-tools/pkg/parser"
	"github.com/jensneuse/graphql-go-tools/pkg/printer"
	"io"
)

type GraphQLRequest struct {
	OperationName string                 `json:"operationName,omitempty"`
	Query         string                 `json:"query"`
	Variables     map[string]interface{} `json:"variables,omitempty"`
}

type Invoker struct {
	middleWares []GraphqlMiddleware
	parse       *parser.Parser
	look        *lookup.Lookup
	walk        *lookup.Walker
	mod         *parser.ManualAstMod
	astPrint    *printer.Printer
}

func NewInvoker(middleWares ...GraphqlMiddleware) *Invoker {
	parse := parser.NewParser()
	look := lookup.New(parse)
	walk := lookup.NewWalker(512, 8)
	astPrint := printer.New()
	astPrint.SetInput(parse, look, walk)
	return &Invoker{
		parse:       parse,
		look:        look,
		walk:        walk,
		mod:         parser.NewManualAstMod(parse),
		astPrint:    astPrint,
		middleWares: middleWares,
	}
}

func (i *Invoker) SetSchema(schema []byte) error {
	return i.parse.ParseTypeSystemDefinition(schema)
}

func (i *Invoker) InvokeMiddleWares(ctx context.Context, request []byte) (err error) {

	err = i.middlewaresPrepareSchema(ctx)
	if err != nil {
		return err
	}

	err = i.parse.ParseExecutableDefinition(request)
	if err != nil {
		return err
	}

	i.walk.SetLookup(i.look)

	return i.middlewaresOnRequest(ctx)
}

func (i *Invoker) RewriteRequest(w io.Writer) error {
	i.walk.SetLookup(i.look)
	i.walk.WalkExecutable()
	return i.astPrint.PrintExecutableSchema(w)
}

func (i *Invoker) middlewaresPrepareSchema(ctx context.Context) error {
	for j := range i.middleWares {
		err := i.middleWares[j].PrepareSchema(ctx, i.look, i.walk, i.parse, i.mod)
		if err != nil {
			return err
		}
	}
	return nil
}

func (i *Invoker) middlewaresOnRequest(ctx context.Context) error {
	for j := range i.middleWares {
		err := i.middleWares[j].OnRequest(ctx, i.look, i.walk, i.parse, i.mod)
		if err != nil {
			return err
		}
	}
	return nil
}
