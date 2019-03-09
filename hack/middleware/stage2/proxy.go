package stage2

import (
	"bytes"
	"fmt"
	"github.com/jensneuse/graphql-go-tools/pkg/lookup"
	"github.com/jensneuse/graphql-go-tools/pkg/parser"
	"github.com/jensneuse/graphql-go-tools/pkg/printer"
	"github.com/jensneuse/graphql-go-tools/pkg/proxy/middleware/example"
	"github.com/jensneuse/graphql-go-tools/pkg/validator"
)

type Proxy struct {
	FakeRedis         *FakeRedis
	PrismaConnections map[string]Prisma
	parse             *parser.Parser
	look              *lookup.Lookup
	walk              *lookup.Walker
	mod               *parser.ManualAstMod
	astPrint          *printer.Printer
	buff              *bytes.Buffer
	valid             *validator.Validator
}

func NewProxy() *Proxy {

	parse := parser.NewParser()

	return &Proxy{
		FakeRedis:         NewFakeRedis(),
		PrismaConnections: make(map[string]Prisma),
		parse:             parse,
		look:              lookup.New(parse, 1024),
		walk:              lookup.NewWalker(512, 8),
		mod:               parser.NewManualAstMod(parse),
		astPrint:          printer.New(),
		buff:              &bytes.Buffer{},
		valid:             validator.New(),
	}
}

func (p *Proxy) ConfigureSchema(path string, schema string, prisma Prisma) {
	p.FakeRedis.PutSchema(path, schema)
	p.PrismaConnections[path] = prisma
}

func (p *Proxy) Request(path string, request []byte) (response []byte, err error) {

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

	err = p.parse.ParseTypeSystemDefinition([]byte(schema))
	if err != nil {
		return
	}

	err = p.parse.ParseExecutableDefinition([]byte(request))
	if err != nil {
		return
	}

	p.walk.SetLookup(p.look)
	p.walk.WalkExecutable()

	p.valid.SetInput(p.look, p.walk)
	validationResult := p.valid.ValidateExecutableDefinition(validator.DefaultExecutionRules)
	if !validationResult.Valid {
		err = fmt.Errorf("validation failed: %+v, subjectName: %s", validationResult, string(p.look.CachedName(validationResult.Meta.SubjectNameRef)))
		return
	}

	middleware := example.AssetUrlMiddleware{}
	middleware.OnRequest(p.look, p.walk, p.parse, p.mod)

	p.astPrint.SetInput(p.parse, p.look, p.walk)
	p.buff.Reset()
	p.astPrint.PrintExecutableSchema(p.buff)

	response = prisma.Query(p.buff.Bytes())

	err = middleware.OnResponse(&response, p.look, p.walk, p.parse, p.mod)
	if err != nil {
		return
	}

	return
}
