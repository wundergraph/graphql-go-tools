package postprocess

import (
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

// CreateMultiFetchTypes is a postprocessor that transforms multi fetches into more concrete fetch types
type CreateMultiFetchTypes struct{}

func (d *CreateMultiFetchTypes) Process(pre plan.Plan) plan.Plan {
	switch t := pre.(type) {
	case *plan.SynchronousResponsePlan:
		d.traverseNode(t.Response.Data)
	case *plan.SubscriptionResponsePlan:
		d.traverseNode(t.Response.Response.Data)
	}
	return pre
}

func (d *CreateMultiFetchTypes) traverseNode(node resolve.Node) {
	switch n := node.(type) {
	case *resolve.Object:
		n.Fetch = d.traverseFetch(n.Fetch)
		for i := range n.Fields {
			d.traverseNode(n.Fields[i].Value)
		}
	case *resolve.Array:
		d.traverseNode(n.Item)
	}
}

func (d *CreateMultiFetchTypes) traverseFetch(fetch resolve.Fetch) resolve.Fetch {
	if fetch == nil {
		return nil
	}
	switch f := fetch.(type) {
	case *resolve.SingleFetch:
		return f
	case *resolve.MultiFetch:
		return d.processMultiFetch(f)
	}

	return fetch
}

func (d *CreateMultiFetchTypes) processMultiFetch(fetch *resolve.MultiFetch) resolve.Fetch {

	return fetch
}
