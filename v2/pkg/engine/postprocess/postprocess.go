package postprocess

import (
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

type ResponseTreeProcessor interface {
	Process(node resolve.Node)
	ProcessSubscription(node resolve.Node, trigger *resolve.GraphQLSubscriptionTrigger)
}

type FetchTreeProcessor interface {
	ProcessFetchTree(root *resolve.FetchTreeNode)
}

type Processor struct {
	processResponseTree []ResponseTreeProcessor
	processFetchTree    []FetchTreeProcessor
}

func NewProcessor() *Processor {
	return &Processor{
		processFetchTree: []FetchTreeProcessor{
			&deduplicateSingleFetches{},
		},
		/*processFetchTree: []ResponseTreeProcessor{
			//&CreateMultiFetchTypes{},
			&DeduplicateMultiFetch{}, // this processor must be called after CreateMultiFetchTypes, when we remove duplicates we may lack of dependency id, which required to create proper multi fetch types
			&ResolveInputTemplates{},
			&CreateConcreteSingleFetchTypes{},
		},
		processResponseTree: []ResponseTreeProcessor{
			&MergeFields{},
		},*/
	}
}

func (p *Processor) Process(pre plan.Plan) plan.Plan {
	switch t := pre.(type) {
	case *plan.SynchronousResponsePlan:
		for i := range p.processResponseTree {
			p.processResponseTree[i].Process(t.Response.Data)
		}
		p.createFetchTree(t.Response)
		for i := range p.processFetchTree {
			p.processFetchTree[i].ProcessFetchTree(t.Response.Fetches)
		}

	case *plan.SubscriptionResponsePlan:
		for i := range p.processResponseTree {
			p.processResponseTree[i].ProcessSubscription(t.Response.Response.Data, &t.Response.Trigger)
		}
		p.createFetchTree(t.Response.Response)
		for i := range p.processFetchTree {
			p.processFetchTree[i].ProcessFetchTree(t.Response.Response.Fetches)
		}
	}
	return pre
}

func (p *Processor) createFetchTree(res *resolve.GraphQLResponse) {
	ex := &extractor{
		info: res.Info,
	}
	fetches := ex.GetFetches(res)
	if len(fetches) == 0 {
		return
	}
	if len(fetches) == 1 {
		res.Fetches = &resolve.FetchTreeNode{
			Kind: resolve.FetchTreeNodeKindSingle,
			Item: fetches[0],
		}
		return
	}
	children := make([]*resolve.FetchTreeNode, len(fetches))
	for i := range fetches {
		children[i] = &resolve.FetchTreeNode{
			Kind: resolve.FetchTreeNodeKindSingle,
			Item: fetches[i],
		}
	}
	res.Fetches = &resolve.FetchTreeNode{
		Kind:        resolve.FetchTreeNodeKindSequence,
		SerialNodes: children,
	}
}
