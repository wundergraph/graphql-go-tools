package postprocess

import (
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan"
)

type PostProcessor interface {
	Process(pre plan.Plan) plan.Plan
}

type Processor struct {
	postProcessors []PostProcessor
}

func DefaultProcessor() *Processor {
	return &Processor{
		[]PostProcessor{
			&ProcessDataSource{},
			&DataSourceFetch{},
		},
	}
}

func (p *Processor) Process(pre plan.Plan) (post plan.Plan) {
	post = pre
	for i := range p.postProcessors {
		post = p.postProcessors[i].Process(post)
	}
	return
}
