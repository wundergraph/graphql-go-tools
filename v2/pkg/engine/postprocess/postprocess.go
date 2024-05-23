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
	postProcessors []PostProcessor
}

func DefaultProcessor() *Processor {
	return &Processor{
		[]PostProcessor{
			&ResolveInputTemplates{},
			&CreateMultiFetchTypes{},
			&CreateConcreteSingleFetchTypes{},
		},
	}
}

func (p *Processor) Process(pre plan.Plan) plan.Plan {
	switch t := pre.(type) {
	case *plan.SynchronousResponsePlan:
		p.extractFetches(t.Response)
		for i := range p.postProcessors {
			p.postProcessors[i].Process(t.Response.FetchTree)
		}

	case *plan.SubscriptionResponsePlan:
		p.extractFetches(t.Response.Response)
		for i := range p.postProcessors {
			p.postProcessors[i].ProcessSubscription(t.Response.Response.FetchTree, &t.Response.Trigger)
		}
	}

	return pre
}

func (p *Processor) extractFetches(res *resolve.GraphQLResponse) {
	fieldsWithFetch := NewFetchFinder().Find(res)
	createFetchesCopy := NewFetchTreeCreator(fieldsWithFetch)

	res.FetchTree = createFetchesCopy.ExtractFetchTree(res)
}
