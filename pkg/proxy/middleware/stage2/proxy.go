package stage2

import (
	"bytes"
	"fmt"
	"github.com/jensneuse/graphql-go-tools/pkg/lookup"
	"github.com/jensneuse/graphql-go-tools/pkg/parser"
	"github.com/jensneuse/graphql-go-tools/pkg/printer"
	"github.com/jensneuse/graphql-go-tools/pkg/proxy/middleware/example"
)

type Proxy struct {
	FakeRedis         *FakeRedis
	PrismaConnections map[string]Prisma
}

func NewProxy() *Proxy {
	return &Proxy{
		FakeRedis:         NewFakeRedis(),
		PrismaConnections: make(map[string]Prisma),
	}
}

func (p *Proxy) ConfigureSchema(path string, schema string, prisma Prisma) {
	p.FakeRedis.PutSchema(path, schema)
	p.PrismaConnections[path] = prisma
}

func (p *Proxy) Request(path string, request string) (response string, err error) {

	prisma, exists := p.PrismaConnections[path]
	if !exists {
		err = fmt.Errorf("prisma not configured")
		return
	}

	schema, exists := p.FakeRedis.GetSchema(path)
	if !exists {
		err = fmt.Errorf("redis not configured")
		return
	}

	parse := parser.NewParser()
	err = parse.ParseTypeSystemDefinition([]byte(schema))
	if err != nil {
		return
	}

	err = parse.ParseExecutableDefinition([]byte(request))
	if err != nil {
		return
	}

	look := lookup.New(parse, 512)
	walk := lookup.NewWalker(1024, 8)
	walk.SetLookup(look)
	walk.WalkExecutable()
	mod := parser.NewManualAstMod(parse)

	middleware := example.AssetUrlMiddleware{}
	middleware.OnRequest(look, walk, parse, mod)

	astPrint := printer.New()
	astPrint.SetInput(parse, look, walk)
	var buff bytes.Buffer
	astPrint.PrintExecutableSchema(&buff)

	out := []byte(prisma.Query(string(buff.Bytes())))

	err = middleware.OnResponse(&out, look, walk, parse, mod)
	if err != nil {
		return
	}

	response = string(out)

	return
}
