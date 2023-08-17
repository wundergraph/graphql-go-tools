package postprocess

import (
	"github.com/TykTechnologies/graphql-go-tools/pkg/engine/plan"
	"github.com/TykTechnologies/graphql-go-tools/pkg/engine/resolve"
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

func (f *ProcessFetchInput) Process(pre plan.Plan) plan.Plan {
	switch t := pre.(type) {
	case *plan.SynchronousResponsePlan:
		f.traverseNode(t.Response.Data)
	case *plan.StreamingResponsePlan:
		f.traverseNode(t.Response.InitialResponse.Data)
		for i := range t.Response.Patches {
			f.traverseFetch(t.Response.Patches[i].Fetch)
			f.traverseNode(t.Response.Patches[i].Value)
		}
	case *plan.SubscriptionResponsePlan:
		f.traverseTrigger(&t.Response.Trigger)
		f.traverseNode(t.Response.Response.Data)
	}
	return pre
}

func (f *ProcessFetchInput) traverseNode(node resolve.Node) {
	switch n := node.(type) {
	case *resolve.Object:
		f.traverseFetch(n.Fetch)
		for i := range n.Fields {
			f.traverseNode(n.Fields[i].Value)
		}
	case *resolve.Array:
		f.traverseNode(n.Item)
	}
}

func (f *ProcessFetchInput) traverseFetch(fetch resolve.Fetch) {
	if fetch == nil {
		return
	}
	switch fetchType := fetch.(type) {
	case *resolve.SingleFetch:
		f.traverseSingleFetch(fetchType)
	case *resolve.BatchFetch:
		f.traverseSingleFetch(fetchType.Fetch)
	case *resolve.ParallelFetch:
		for i := range fetchType.Fetches {
			f.traverseFetch(fetchType.Fetches[i])
		}
	}
}

func (f *ProcessFetchInput) traverseTrigger(trigger *resolve.GraphQLSubscriptionTrigger) {
	trigger.Input = []byte(f.fetchInputModifier(trigger.Input))
}

func (f *ProcessFetchInput) traverseSingleFetch(fetch *resolve.SingleFetch) {
	fetch.Input = f.fetchInputModifier([]byte(fetch.Input))
}
