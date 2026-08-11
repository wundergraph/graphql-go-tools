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

// singleFetchOption configures optional fields of a single-fetch node built by sf.
type singleFetchOption func(node *resolve.FetchTreeNode)

// dependsOn sets the fetch IDs the fetch depends on.
func dependsOn(ids ...int) singleFetchOption {
	return func(node *resolve.FetchTreeNode) {
		node.Item.Fetch.(*resolve.SingleFetch).FetchDependencies.DependsOnFetchIDs = ids
	}
}

// responsePath nests the fetch responsePath responsePath, e.g. "a.b" becomes ObjectPath("a"), ObjectPath("b").
func responsePath(responsePath string) singleFetchOption {
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

// mergePath sets the merge path the fetch mergePath in the response.
func mergePath(mergePath ...string) singleFetchOption {
	return func(node *resolve.FetchTreeNode) {
		node.Item.Fetch.(*resolve.SingleFetch).PostProcessing.MergePath = mergePath
	}
}

func sf(id int, opts ...singleFetchOption) *resolve.FetchTreeNode {
	node := resolve.Single(&resolve.SingleFetch{FetchDependencies: resolve.FetchDependencies{FetchID: id}})
	for _, opt := range opts {
		opt(node)
	}
	return node
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

// materialize returns shape with every leaf node replaced by the input node carrying the same fetch ID.
// It needed to prevent clutter in the expected part of the test as not important.
func materialize(t *testing.T, shape *resolve.FetchTreeNode, input map[int]*resolve.FetchTreeNode) *resolve.FetchTreeNode {
	t.Helper()
	if shape == nil {
		return nil
	}
	if shape.Kind == resolve.FetchTreeNodeKindSingle {
		id := shape.Item.Fetch.Dependencies().FetchID
		node, ok := input[id]
		require.Truef(t, ok, "expected tree references fetch %d not present in input", id)
		return node
	}
	children := make([]*resolve.FetchTreeNode, len(shape.ChildNodes))
	for i, child := range shape.ChildNodes {
		children[i] = materialize(t, child, input)
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
		b.WriteString(indent + leafShape(node))
	case resolve.FetchTreeNodeKindSequence, resolve.FetchTreeNodeKindParallel:
		name := "seq"
		if node.Kind == resolve.FetchTreeNodeKindParallel {
			name = "par"
		}
		// A group of only leaves stays on one line.
		if leaves := leafShapes(node.ChildNodes); leaves != nil {
			b.WriteString(indent + name + "(" + strings.Join(leaves, ", ") + ")")
			return
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

// leafShapes returns the rendered leaves of children, or nil if any child is not a leaf.
func leafShapes(children []*resolve.FetchTreeNode) []string {
	leaves := make([]string, len(children))
	for i, child := range children {
		if child == nil || child.Kind != resolve.FetchTreeNodeKindSingle {
			return nil
		}
		leaves[i] = leafShape(child)
	}
	return leaves
}

func leafShape(node *resolve.FetchTreeNode) string {
	id := node.Item.Fetch.Dependencies().FetchID
	switch node.Item.Fetch.(type) {
	case *resolve.SingleFetch:
		return fmt.Sprintf("sf(%d)", id)
	case *resolve.EntityFetch:
		return fmt.Sprintf("ef(%d)", id)
	case *resolve.BatchEntityFetch:
		return fmt.Sprintf("bf(%d)", id)
	default:
		return fmt.Sprintf("fetch(%d)", id)
	}
}
