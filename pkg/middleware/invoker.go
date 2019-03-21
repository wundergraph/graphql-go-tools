package middleware

import (
	"github.com/jensneuse/graphql-go-tools/pkg/lookup"
	"github.com/jensneuse/graphql-go-tools/pkg/parser"
	"github.com/jensneuse/graphql-go-tools/pkg/printer"
	"io"
)

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
	look := lookup.New(parse, 256)
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

func (i *Invoker) InvokeMiddleWares(userValues map[string][]byte, request []byte) (err error) {

	err = i.parse.ParseExecutableDefinition(request)
	if err != nil {
		return err
	}

	i.walk.SetLookup(i.look)

	return i.invokeMiddleWares(userValues)
}

func (i *Invoker) RewriteRequest(w io.Writer) error {
	i.walk.SetLookup(i.look)
	i.walk.WalkExecutable()
	return i.astPrint.PrintExecutableSchema(w)
}

func (i *Invoker) invokeMiddleWares(userValues map[string][]byte) error {
	for j := range i.middleWares {
		err := i.middleWares[j].OnRequest(userValues, i.look, i.walk, i.parse, i.mod)
		if err != nil {
			return err
		}
	}
	return nil
}
