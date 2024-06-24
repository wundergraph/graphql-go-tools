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
	processFetchTree     []PostProcessor
	processData          []PostProcessor
	enableExtractFetches bool
}

func NewProcessor(postProcessors []PostProcessor, extractFetches bool) *Processor {
	return &Processor{
		processFetchTree:     postProcessors,
		enableExtractFetches: extractFetches,
	}
}

func DefaultProcessor() *Processor {
	return &Processor{
		processFetchTree: []PostProcessor{
			&CreateMultiFetchTypes{},
			&DeduplicateMultiFetch{}, // this processor must be called after CreateMultiFetchTypes, when we remove duplicates we may lack of dependency id, which required to create proper multi fetch types
			&ResolveInputTemplates{},
			&CreateConcreteSingleFetchTypes{},
		},
		processData: []PostProcessor{
			&MergeFields{},
		},
		enableExtractFetches: true,
	}
}

func (p *Processor) Process(pre plan.Plan) plan.Plan {
	switch t := pre.(type) {
	case *plan.SynchronousResponsePlan:
		p.extractFetches(t.Response)
		for i := range p.processFetchTree {
			if p.enableExtractFetches {
				p.processFetchTree[i].Process(t.Response.FetchTree)
			} else {
				p.processFetchTree[i].Process(t.Response.Data)
			}
		}
		for i := range p.processData {
			p.processData[i].Process(t.Response.Data)
		}

	case *plan.SubscriptionResponsePlan:
		p.extractFetches(t.Response.Response)
		for i := range p.processFetchTree {
			if p.enableExtractFetches {
				p.processFetchTree[i].ProcessSubscription(t.Response.Response.FetchTree, &t.Response.Trigger)
			} else {
				p.processFetchTree[i].Process(t.Response.Response.Data)
			}
		}
		for i := range p.processData {
			p.processData[i].ProcessSubscription(t.Response.Response.Data, &t.Response.Trigger)
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
