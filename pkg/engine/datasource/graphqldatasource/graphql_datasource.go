package graphqldatasource

import (
	"context"
	"fmt"

	"github.com/buger/jsonparser"

	"github.com/jensneuse/graphql-go-tools/pkg/engine/plan"
	"github.com/jensneuse/graphql-go-tools/pkg/engine/resolve"
)

type Planner struct {
	v *plan.Visitor
}

func (g *Planner) EnterField(ref int) {
	fieldName := g.v.Operation.FieldNameString(ref)
	fmt.Println("Planner", fieldName, g.v.Path.String())
}

func (g *Planner) LeaveField(ref int) {

}

func (g *Planner) Register(visitor *plan.Visitor) {
	g.v = visitor
	visitor.RegisterFieldVisitor(g)
}

type Source struct {
}

func (g *Source) Load(ctx context.Context, input []byte, bufPair *resolve.BufPair) (err error) {
	var (
		host, url, query []byte
		paths            = [][]string{
			{"host"},
			{"url"},
			{"query"},
		}
	)
	jsonparser.EachKey(input, func(i int, bytes []byte, valueType jsonparser.ValueType, err error) {
		switch i {
		case 0:
			host = bytes
		case 1:
			url = bytes
		case 2:
			query = bytes
		}
	}, paths...)

	fmt.Printf("Source.Load - host: %s, url: %s, query: %s\n", string(host), string(url), string(query))

	return nil
}
