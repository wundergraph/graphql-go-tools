package middleware

import (
	"bytes"
	"github.com/jensneuse/graphql-go-tools/pkg/lookup"
	"github.com/jensneuse/graphql-go-tools/pkg/parser"
	"github.com/jensneuse/graphql-go-tools/pkg/printer"
)

// InvokeMiddleware is a one off middleware invocation helper
// This should only be used for testing as it's a waste of resources
// It makes use of panics to don't use this in production!
func InvokeMiddleware(middleware GraphqlMiddleware, userValues map[string][]byte, schema, request string) (result string, err error) {
	parse := parser.NewParser()
	if err = parse.ParseTypeSystemDefinition([]byte(schema)); err != nil {
		return
	}
	if err = parse.ParseExecutableDefinition([]byte(request)); err != nil {
		return
	}
	astPrint := printer.New()
	look := lookup.New(parse)
	walk := lookup.NewWalker(1024, 8)
	mod := parser.NewManualAstMod(parse)
	walk.SetLookup(look)

	if err = middleware.OnRequest(userValues, look, walk, parse, mod); err != nil {
		return
	}

	walk.SetLookup(look)
	walk.WalkExecutable()

	astPrint.SetInput(parse, look, walk)
	buff := bytes.Buffer{}
	if err = astPrint.PrintExecutableSchema(&buff); err != nil {
		return
	}

	result = buff.String()
	return
}
