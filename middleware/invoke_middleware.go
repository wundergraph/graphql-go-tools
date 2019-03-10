package middleware

import (
	"bytes"
	"context"
	"github.com/jensneuse/graphql-go-tools/pkg/lookup"
	"github.com/jensneuse/graphql-go-tools/pkg/parser"
	"github.com/jensneuse/graphql-go-tools/pkg/printer"
)

func InvokeMiddleware(middleware GraphqlMiddleware, schema, request string) string {
	parse := parser.NewParser()
	if err := parse.ParseTypeSystemDefinition([]byte(schema)); err != nil {
		panic(err)
	}
	if err := parse.ParseExecutableDefinition([]byte(request)); err != nil {
		panic(err)
	}
	astPrint := printer.New()
	look := lookup.New(parse, 512)
	walk := lookup.NewWalker(1024, 8)
	mod := parser.NewManualAstMod(parse)
	walk.SetLookup(look)

	if err := middleware.OnRequest(context.Background(), look, walk, parse, mod); err != nil {
		panic(err)
	}

	walk.SetLookup(look)
	walk.WalkExecutable()

	astPrint.SetInput(parse, look, walk)
	buff := bytes.Buffer{}
	if err := astPrint.PrintExecutableSchema(&buff); err != nil {
		panic(err)
	}
	return buff.String()
}
