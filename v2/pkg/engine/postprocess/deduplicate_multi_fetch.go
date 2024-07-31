package postprocess

import (
	"slices"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

// DeduplicateMultiFetch is a postprocessor that transforms multi fetches into more concrete fetch types
type DeduplicateMultiFetch struct{}

func (d *DeduplicateMultiFetch) Process(node resolve.Node) {
	d.traverseNode(node)
}

func (d *DeduplicateMultiFetch) ProcessSubscription(node resolve.Node, trigger *resolve.GraphQLSubscriptionTrigger) {
	d.traverseNode(node)
}

func (d *DeduplicateMultiFetch) traverseNode(node resolve.Node) {
	switch n := node.(type) {
	case *resolve.Object:
		//d.traverseFetch(n.Fetch)
		for i := range n.Fields {
			d.traverseNode(n.Fields[i].Value)
		}
	case *resolve.Array:
		d.traverseNode(n.Item)
	}
}

func (d *DeduplicateMultiFetch) traverseFetch(fetch resolve.Fetch) {
	if fetch == nil {
		return
	}
	switch f := fetch.(type) {
	case *resolve.SingleFetch:
		return
	case *resolve.SerialFetch:
		for i := range f.Fetches {
			d.traverseFetch(f.Fetches[i])
		}
	case *resolve.ParallelFetch:
		d.deduplicateParallelFetch(f)
		for i := range f.Fetches {
			d.traverseFetch(f.Fetches[i])
		}
	}
}

// deduplicateParallelFetch removes duplicated single fetches from a parallel fetch
func (d *DeduplicateMultiFetch) deduplicateParallelFetch(fetch *resolve.ParallelFetch) {
	for i := 0; i < len(fetch.Fetches); i++ {
		singleFetch, ok := fetch.Fetches[i].(*resolve.SingleFetch)
		if !ok {
			continue
		}

		fetch.Fetches = slices.DeleteFunc(fetch.Fetches, func(other resolve.Fetch) bool {
			if other == singleFetch {
				return false
			}

			otherSingleFetch, ok := other.(*resolve.SingleFetch)
			if !ok {
				return false
			}

			return singleFetch.FetchConfiguration.Equals(&otherSingleFetch.FetchConfiguration)
		})
	}
}
