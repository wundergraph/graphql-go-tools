package postprocess

import (
	"github.com/TykTechnologies/graphql-go-tools/v2/pkg/engine/plan"
	"github.com/TykTechnologies/graphql-go-tools/v2/pkg/engine/resolve"
)

type FetchInputModifier func([]byte) string

type ProcessFetchInput struct {
	fetchInputModifier FetchInputModifier
}

func NewProcessFetchInput(fetchInputModifier FetchInputModifier) *ProcessFetchInput {
	return &ProcessFetchInput{
		fetchInputModifier: fetchInputModifier,
	}
}

func (p *ProcessFetchInput) Process(pre plan.Plan) plan.Plan {
	switch t := pre.(type) {
	case *plan.SynchronousResponsePlan:
		p.traverseNode(t.Response.Data)
	case *plan.SubscriptionResponsePlan:
		p.traverseTrigger(&t.Response.Trigger)
		p.traverseNode(t.Response.Response.Data)
	}
	return pre
}

func (p *ProcessFetchInput) traverseNode(node resolve.Node) {
	switch n := node.(type) {
	case *resolve.Object:
		p.traverseFetch(n.Fetch)
		for i := range n.Fields {
			p.traverseNode(n.Fields[i].Value)
		}
	case *resolve.Array:
		p.traverseNode(n.Item)
	}
}

func (p *ProcessFetchInput) traverseFetch(fetch resolve.Fetch) {
	if fetch == nil {
		return
	}
	switch f := fetch.(type) {
	case *resolve.SingleFetch:
		p.traverseSingleFetch(f)
	case *resolve.SerialFetch:
		p.traverseSerialFetch(f)
	case *resolve.ParallelFetch:
		p.traverseParallelFetch(f)
	case *resolve.ParallelListItemFetch:
		p.traverseParallelListItemFetch(f)
	}
}

func (p *ProcessFetchInput) traverseTrigger(trigger *resolve.GraphQLSubscriptionTrigger) {
	trigger.Input = []byte(p.fetchInputModifier(trigger.Input))
}

func (p *ProcessFetchInput) traverseSingleFetch(fetch *resolve.SingleFetch) {
	fetch.Input = p.fetchInputModifier([]byte(fetch.Input))
}

func (p *ProcessFetchInput) traverseSerialFetch(fetch *resolve.SerialFetch) {
	for i := range fetch.Fetches {
		p.traverseFetch(fetch.Fetches[i])
	}
}

func (p *ProcessFetchInput) traverseParallelFetch(fetch *resolve.ParallelFetch) {
	for i := range fetch.Fetches {
		p.traverseFetch(fetch.Fetches[i])
	}
}

func (p *ProcessFetchInput) traverseParallelListItemFetch(fetch *resolve.ParallelListItemFetch) {
	p.traverseSingleFetch(fetch.Fetch)
}
