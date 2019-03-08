package authorization

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"testing"

	"github.com/jensneuse/graphql-go-tools/pkg/lookup"
	"github.com/jensneuse/graphql-go-tools/pkg/parser"
	"github.com/jensneuse/graphql-go-tools/pkg/printer"
	"github.com/jensneuse/graphql-go-tools/pkg/proxy"
)

func TestAuthProxy(t *testing.T) {
	run := func(schema, queryBefore, queryAfter string, req http.Request, middleWares ...proxy.GraphqlMiddleware) {
		p := parser.NewParser()
		err := p.ParseTypeSystemDefinition([]byte(schema))
		if err != nil {
			panic(err)
		}

		err = p.ParseExecutableDefinition([]byte(queryBefore))
		if err != nil {
			panic(err)
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
		prox.SetMiddleWares(middleWares...)

		prox.SetInput(l, w, p, mod)
		prox.OnQuery()

		astPrinter.PrintExecutableSchema(&buff)
		got := buff.Bytes()
		buff.Reset()

		// parse expectation and compare

		err = p.ParseExecutableDefinition([]byte(queryAfter))
		l.ResetPool()
		w.SetLookup(l)
		w.WalkExecutable()

		buff2 := bytes.Buffer{}

		astPrinter.PrintExecutableSchema(&buff2)
		want := buff2.Bytes()

		if !bytes.Equal(want, got) {
			panic(fmt.Errorf("want:\n%s\ngot:\n%s\n", string(want), string(got)))
		}
	}

	t.Run("authorization injection", func(t *testing.T) {
		req := http.Request{}
		ctx := context.WithValue(context.Background(), "user", "jsmith@example.org")
		req.WithContext(ctx)
		run(publicSchema, publicQuery, privateQuery, req, &AuthorizationMiddleware{})
	})
}

const publicSchema = `
schema {
	query: Query
}

type Query {
	documents: [Document] @context_insert(user: "user")
}

type Document implements Node {
	owner: String
	sensitiveInformation: String
}
`

// This schema is unused, left for reference
const privateSchema = `
schema {
	query: Query
}

type Query {
	documents(user: String!): [Document]
}

type Document implements Node {
	owner: String
	sensitiveInformation: String
}
`

const publicQuery = `
query myDocuments {
	documents {
		sensitiveInformation
	}
}
`

const privateQuery = `
query myDocuments {
	documents(user: "jsmith@example.org") {
		sensitiveInformation
	}
}
`