package postprocess

import (
	"github.com/TykTechnologies/graphql-go-tools/pkg/engine/plan"
)

type PostProcessor interface {
	Process(pre plan.Plan) plan.Plan
}

type Processor struct {
	postProcessors []PostProcessor
}

func (p *Processor) AddPostProcessor(pr PostProcessor) {
	p.postProcessors = append([]PostProcessor{pr}, p.postProcessors...)
}

func DefaultProcessor() *Processor {
	return &Processor{
		[]PostProcessor{
			&ProcessDefer{},
			&ProcessStream{},
			&ProcessDataSource{},
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
