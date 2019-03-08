package example

import (
	"bytes"
	"fmt"
	"github.com/jensneuse/graphql-go-tools/pkg/lookup"
	"github.com/jensneuse/graphql-go-tools/pkg/parser"
	"github.com/jensneuse/graphql-go-tools/pkg/printer"
	"github.com/jensneuse/graphql-go-tools/pkg/proxy"
	"testing"
)

func TestProxy(t *testing.T) {

	run := func(schema, queryBefore, queryAfter string, middleWares ...proxy.GraphqlMiddleware) {
		p := parser.NewParser()
		schemaBytes := []byte(schema)
		err := p.ParseTypeSystemDefinition(&schemaBytes)
		if err != nil {
			panic(err)
		}

		queryBeforeBytes := []byte(queryBefore)
		err = p.ParseExecutableDefinition(&queryBeforeBytes)
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

		queryAfterBytes := []byte(queryAfter)
		err = p.ParseExecutableDefinition(&queryAfterBytes)
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

	t.Run("asset handle transform", func(t *testing.T) {
		run(assetSchema,
			`query testQueryWithoutHandle {
  								assets(first: 1) {
    							id
    							fileName
    							url(transformation: {image: {resize: {width: 100, height: 100}}})
  							}
						}`,
			`	query testQueryWithoutHandle {
	  						assets(first: 1) {
    							id
    							fileName
								handle
  							}
						}`, &AssetUrlMiddleware{})
	})
	t.Run("asset handle transform on empty selectionSet", func(t *testing.T) {
		run(assetSchema,
			`query testQueryWithoutHandle {
  							assets(first: 1) {
  							}
						}`,
			`	query testQueryWithoutHandle {
	  						assets(first: 1) {
    							handle
  							}
						}`, &AssetUrlMiddleware{})
	})

}

var assetSchema = `
schema {
    query: Query
}

type Query {
    assets(first: Int): [Asset]
}

type Asset implements Node @RequestMiddleware {
    status: Status!
    updatedAt: DateTime!
    createdAt: DateTime!
    id: ID!
    handle: String!
    fileName: String!
    height: Float
    width: Float
    size: Float
    mimeType: String
    url: String!
}`

func BenchmarkProxy(b *testing.B) {

	run := func(b *testing.B, schema, queryBefore, queryAfter string, middleWares ...proxy.GraphqlMiddleware) {
		p := parser.NewParser()
		schemaBytes := []byte(schema)
		err := p.ParseTypeSystemDefinition(&schemaBytes)
		if err != nil {
			panic(err)
		}

		l := lookup.New(p, 256)
		l.SetParser(p)
		w := lookup.NewWalker(1024, 8)

		mod := parser.NewManualAstMod(p)
		prox := proxy.NewProxy(mod)
		prox.SetMiddleWares(middleWares...)

		input := []byte(queryBefore)

		b.ReportAllocs()
		b.ResetTimer()

		for i := 0; i < b.N; i++ {

			err = p.ParseExecutableDefinition(&input)
			if err != nil {
				panic(err)
			}
			w.SetLookup(l)
			w.WalkExecutable()

			prox.SetInput(l, w, p, mod)
			prox.OnQuery()

			/*printer := printer.New()
			printer.SetInput(p, l, w)
			out := bytes.Buffer{}
			printer.PrintExecutableSchema(&out)
			fmt.Println(string(out.Bytes()))
			return*/
		}
	}

	b.Run("asset handle transform", func(b *testing.B) {
		run(b, `
schema {
    query: Query
}

type Query {
    assets(first: Int): [Asset]
}

type Asset implements Node @RequestMiddleware {
    status: Status!
    updatedAt: DateTime!
    createdAt: DateTime!
    id: ID!
    handle: String!
    fileName: String!
    height: Float
    width: Float
    size: Float
    mimeType: String
    url: String!
}`,
			`query testQueryWithoutHandle {
  assets(first: 1) {
    id
    fileName
    url(transformation: {image: {resize: {width: 100, height: 100}}})
  }
}`,
			`query testQueryWithoutHandle {
  assets(first: 1) {
    id
    fileName
	handle
  }
}`, &AssetUrlMiddleware{})
	})

}
