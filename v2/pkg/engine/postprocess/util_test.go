package postprocess

import (
	"strings"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

// Various test helpers to reduce clutter in tests.

func nodes(items ...*resolve.FetchTreeNode) []*resolve.FetchTreeNode {
	return items
}

// sfOpt configures optional fields of a single-fetch node built by sf.
type sfOpt func(node *resolve.FetchTreeNode)

// deps sets the fetch IDs the fetch depends on.
func deps(ids ...int) sfOpt {
	return func(node *resolve.FetchTreeNode) {
		node.Item.Fetch.(*resolve.SingleFetch).FetchDependencies.DependsOnFetchIDs = ids
	}
}

// at nests the fetch at responsePath, e.g. "a.b" becomes ObjectPath("a"), ObjectPath("b").
func at(responsePath string) sfOpt {
	return func(node *resolve.FetchTreeNode) {
		segments := strings.Split(responsePath, ".")
		path := make([]resolve.FetchItemPathElement, len(segments))
		for i, segment := range segments {
			path[i] = resolve.ObjectPath(segment)
		}
		node.Item.FetchPath = path
		node.Item.ResponsePath = responsePath
		node.Item.ResponsePathElements = segments
	}
}

// provides sets the merge path the fetch provides in the response.
func provides(mergePath ...string) sfOpt {
	return func(node *resolve.FetchTreeNode) {
		node.Item.Fetch.(*resolve.SingleFetch).PostProcessing.MergePath = mergePath
	}
}

func sf(id int, opts ...sfOpt) *resolve.FetchTreeNode {
	node := resolve.Single(&resolve.SingleFetch{FetchDependencies: resolve.FetchDependencies{FetchID: id}})
	for _, opt := range opts {
		opt(node)
	}
	return node
}

func ef(id int, deps ...int) *resolve.FetchTreeNode {
	return resolve.Single(&resolve.EntityFetch{FetchDependencies: resolve.FetchDependencies{FetchID: id, DependsOnFetchIDs: deps}})
}

func bf(id int, deps ...int) *resolve.FetchTreeNode {
	return resolve.Single(&resolve.BatchEntityFetch{FetchDependencies: resolve.FetchDependencies{FetchID: id, DependsOnFetchIDs: deps}})
}

func seq(children ...*resolve.FetchTreeNode) *resolve.FetchTreeNode {
	return resolve.Sequence(children...)
}

func par(children ...*resolve.FetchTreeNode) *resolve.FetchTreeNode {
	return resolve.Parallel(children...)
}
