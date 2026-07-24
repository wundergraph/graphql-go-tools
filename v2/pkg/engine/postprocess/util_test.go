package postprocess

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

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

// fetchesByID indexes flat input fetch nodes by their FetchID.
func fetchesByID(input []*resolve.FetchTreeNode) map[int]*resolve.FetchTreeNode {
	byID := make(map[int]*resolve.FetchTreeNode, len(input))
	for _, node := range input {
		byID[node.Item.Fetch.Dependencies().FetchID] = node
	}
	return byID
}

// materialize returns shape with every leaf replaced by the input node carrying the same fetch ID.
func materialize(t *testing.T, shape *resolve.FetchTreeNode, byID map[int]*resolve.FetchTreeNode) *resolve.FetchTreeNode {
	t.Helper()
	if shape == nil {
		return nil
	}
	if shape.Kind == resolve.FetchTreeNodeKindSingle {
		id := shape.Item.Fetch.Dependencies().FetchID
		node, ok := byID[id]
		require.Truef(t, ok, "expected tree references fetch %d not present in input", id)
		return node
	}
	children := make([]*resolve.FetchTreeNode, len(shape.ChildNodes))
	for i, child := range shape.ChildNodes {
		children[i] = materialize(t, child, byID)
	}
	return &resolve.FetchTreeNode{Kind: shape.Kind, ChildNodes: children}
}

// requireEqualTrees compares two fetch trees. The rendered tree is valid Go.
func requireEqualTrees(t *testing.T, expected, actual *resolve.FetchTreeNode) {
	t.Helper()
	require.Equal(t, renderShape(expected), renderShape(actual))
	require.Equal(t, expected, actual)
}

func renderShape(node *resolve.FetchTreeNode) string {
	var b strings.Builder
	writeShape(&b, node, 0)
	return b.String()
}

func writeShape(b *strings.Builder, node *resolve.FetchTreeNode, depth int) {
	indent := strings.Repeat("\t", depth)
	if node == nil {
		b.WriteString(indent + "nil")
		return
	}
	switch node.Kind {
	case resolve.FetchTreeNodeKindSingle:
		id := node.Item.Fetch.Dependencies().FetchID
		switch node.Item.Fetch.(type) {
		case *resolve.SingleFetch:
			fmt.Fprintf(b, "%ssf(%d)", indent, id)
		case *resolve.EntityFetch:
			fmt.Fprintf(b, "%sef(%d)", indent, id)
		case *resolve.BatchEntityFetch:
			fmt.Fprintf(b, "%sbf(%d)", indent, id)
		default:
			fmt.Fprintf(b, "%sfetch(%d)", indent, id)
		}
	case resolve.FetchTreeNodeKindSequence, resolve.FetchTreeNodeKindParallel:
		name := "seq"
		if node.Kind == resolve.FetchTreeNodeKindParallel {
			name = "par"
		}
		b.WriteString(indent + name + "(\n")
		for _, child := range node.ChildNodes {
			writeShape(b, child, depth+1)
			b.WriteString(",\n")
		}
		b.WriteString(indent + ")")
	default:
		fmt.Fprintf(b, "%s%s(?)", indent, node.Kind)
	}
}
