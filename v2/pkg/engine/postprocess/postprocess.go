package postprocess

import (
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

type PostProcessor interface {
	Process(node resolve.Node)
	ProcessSubscription(node resolve.Node, trigger *resolve.GraphQLSubscriptionTrigger)
}

type Processor struct {
	postProcessors       []PostProcessor
	enableExtractFetches bool
}

func NewProcessor(postProcessors []PostProcessor, extractFetches bool) *Processor {
	return &Processor{
		postProcessors:       postProcessors,
		enableExtractFetches: extractFetches,
	}
}

func DefaultProcessor() *Processor {
	return &Processor{
		postProcessors: []PostProcessor{
			&ResolveInputTemplates{},
			&CreateMultiFetchTypes{},
			&CreateConcreteSingleFetchTypes{},
		},
		enableExtractFetches: true,
	}
}

func (p *Processor) Process(pre plan.Plan) plan.Plan {
	switch t := pre.(type) {
	case *plan.SynchronousResponsePlan:
		p.extractFetches(t.Response)
		for i := range p.postProcessors {
			if p.enableExtractFetches {
				p.postProcessors[i].Process(t.Response.FetchTree)
			} else {
				p.postProcessors[i].Process(t.Response.Data)
			}
		}

	case *plan.SubscriptionResponsePlan:
		p.extractFetches(t.Response.Response)
		for i := range p.postProcessors {
			if p.enableExtractFetches {
				p.postProcessors[i].ProcessSubscription(t.Response.Response.FetchTree, &t.Response.Trigger)
			} else {
				p.postProcessors[i].ProcessSubscription(t.Response.Response.Data, &t.Response.Trigger)
			}
		}
	}

	return pre
}

func (p *Processor) extractFetches(res *resolve.GraphQLResponse) {
	if !p.enableExtractFetches {
		return
	}

	fieldsWithFetch := NewFetchFinder().Find(res)
	createFetchesCopy := NewFetchTreeCreator(fieldsWithFetch)

	res.FetchTree = createFetchesCopy.ExtractFetchTree(res)
}
