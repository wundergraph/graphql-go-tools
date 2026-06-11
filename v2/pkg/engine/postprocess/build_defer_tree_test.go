package postprocess

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

// makeDeferPlan builds a minimal DeferResponsePlan with raw fetches tagged
// by DeferID, ready for extractDeferFetches → buildDeferTree processing.
func makeDeferPlan(descriptors map[int]resolve.DeferDescriptor, deferIDs ...int) *plan.DeferResponsePlan {
	var children []*resolve.FetchTreeNode
	for _, id := range deferIDs {
		children = append(children, resolve.Single(&resolve.SingleFetch{
			FetchDependencies: resolve.FetchDependencies{DeferID: id},
		}))
	}
	return &plan.DeferResponsePlan{
		Response: &resolve.GraphQLDeferResponse{
			Response: &resolve.GraphQLResponse{
				Fetches: resolve.Sequence(children...),
				Info:    &resolve.GraphQLResponseInfo{},
			},
			DeferDescriptors: descriptors,
		},
	}
}

func runBuildDeferTree(p *plan.DeferResponsePlan) {
	ext := &extractDeferFetches{}
	ext.Process(p)
	bdt := &buildDeferTree{}
	bdt.Process(p.Response)
}

func TestBuildDeferTree_SingleRoot(t *testing.T) {
	p := makeDeferPlan(map[int]resolve.DeferDescriptor{
		1: {ID: 1, ParentID: 0},
	}, 1)
	runBuildDeferTree(p)

	require.NotNil(t, p.Response.DeferTree)
	assert.Equal(t, resolve.DeferTreeNodeKindSingle, p.Response.DeferTree.Kind)
	assert.Equal(t, 1, p.Response.DeferTree.Item.DeferID)
}

func TestBuildDeferTree_TwoSiblings(t *testing.T) {
	p := makeDeferPlan(map[int]resolve.DeferDescriptor{
		1: {ID: 1, ParentID: 0},
		2: {ID: 2, ParentID: 0},
	}, 1, 2)
	runBuildDeferTree(p)

	require.NotNil(t, p.Response.DeferTree)
	assert.Equal(t, resolve.DeferTreeNodeKindParallel, p.Response.DeferTree.Kind)
	assert.Len(t, p.Response.DeferTree.ChildNodes, 2)
	for _, child := range p.Response.DeferTree.ChildNodes {
		assert.Equal(t, resolve.DeferTreeNodeKindSingle, child.Kind)
	}
}

func TestBuildDeferTree_ParentChild(t *testing.T) {
	// A (root) → C (child of A)
	p := makeDeferPlan(map[int]resolve.DeferDescriptor{
		1: {ID: 1, ParentID: 0},
		3: {ID: 3, ParentID: 1},
	}, 1, 3)
	runBuildDeferTree(p)

	require.NotNil(t, p.Response.DeferTree)
	// Sequence(Single(1), Single(3))
	assert.Equal(t, resolve.DeferTreeNodeKindSequence, p.Response.DeferTree.Kind)
	require.Len(t, p.Response.DeferTree.ChildNodes, 2)
	assert.Equal(t, resolve.DeferTreeNodeKindSingle, p.Response.DeferTree.ChildNodes[0].Kind)
	assert.Equal(t, 1, p.Response.DeferTree.ChildNodes[0].Item.DeferID)
	assert.Equal(t, resolve.DeferTreeNodeKindSingle, p.Response.DeferTree.ChildNodes[1].Kind)
	assert.Equal(t, 3, p.Response.DeferTree.ChildNodes[1].Item.DeferID)
}

func TestBuildDeferTree_TwoSiblingsEachWithChild(t *testing.T) {
	// A (root), B (root), C (child of A), D (child of B)
	p := makeDeferPlan(map[int]resolve.DeferDescriptor{
		1: {ID: 1, ParentID: 0},
		2: {ID: 2, ParentID: 0},
		3: {ID: 3, ParentID: 1},
		4: {ID: 4, ParentID: 2},
	}, 1, 2, 3, 4)
	runBuildDeferTree(p)

	require.NotNil(t, p.Response.DeferTree)
	// Parallel(Sequence(Single(1), Single(3)), Sequence(Single(2), Single(4)))
	assert.Equal(t, resolve.DeferTreeNodeKindParallel, p.Response.DeferTree.Kind)
	require.Len(t, p.Response.DeferTree.ChildNodes, 2)
	for _, branch := range p.Response.DeferTree.ChildNodes {
		assert.Equal(t, resolve.DeferTreeNodeKindSequence, branch.Kind)
		assert.Len(t, branch.ChildNodes, 2)
	}
}
