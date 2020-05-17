package staticdatasource

import (
	"context"

	"github.com/jensneuse/graphql-go-tools/pkg/engine/plan"
	"github.com/jensneuse/graphql-go-tools/pkg/engine/resolve"
)

type Planner struct {
	v *plan.Visitor
}

func (p *Planner) Register(visitor *plan.Visitor) {
	p.v = visitor
	visitor.RegisterEnterFieldVisitor(p)
}

func (p *Planner) EnterField(ref int) {
	rootField, config := p.v.IsRootField(ref)
	if !rootField {
		return
	}

	data := config.Attributes.ValueForKey("data")

	bufferID := p.v.NextBufferID()
	p.v.SetBufferIDForCurrentFieldSet(bufferID)
	p.v.SetCurrentObjectFetch(&resolve.SingleFetch{
		BufferId:   bufferID,
		Input:      data,
		DataSource: Source{},
	}, config)
}

type Source struct {
}

func (_ Source) Load(ctx context.Context, input []byte, bufPair *resolve.BufPair) (err error) {
	_, err = bufPair.Data.Write(input)
	return
}
