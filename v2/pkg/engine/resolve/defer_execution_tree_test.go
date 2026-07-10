package resolve

import (
	"encoding/json"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func executionTreeFetch(id int, source string) *FetchTreeNode {
	return Single(&SingleFetch{
		FetchDependencies: FetchDependencies{FetchID: id},
		Info: &FetchInfo{
			DataSourceID:   source,
			DataSourceName: source,
			QueryPlan:      &QueryPlan{Query: "query { " + source + " }"},
		},
	})
}

func executionTraceResponse() (*GraphQLDeferResponse, []*FetchTreeNode) {
	primary := executionTreeFetch(1, "primary")
	parent := executionTreeFetch(2, "parent")
	nested := executionTreeFetch(3, "nested")
	sibling := executionTreeFetch(4, "sibling")
	response := &GraphQLDeferResponse{
		Response: &GraphQLResponse{Fetches: primary},
		DeferDescriptors: map[int]DeferDescriptor{
			1: {ID: 1, Label: "Parent", Path: []string{"parent"}},
			2: {ID: 2, ParentID: 1, Label: "Nested", Path: []string{"parent", "nested"}},
			3: {ID: 3, Label: "Sibling", Path: []string{"sibling"}},
		},
		DeferTree: DeferParallel(
			DeferSequence(
				DeferSingle(executionTreeGroup(1, parent)),
				DeferSingle(executionTreeGroup(2, nested)),
			),
			DeferSingle(executionTreeGroup(3, sibling)),
		),
	}
	return response, []*FetchTreeNode{primary, parent, nested, sibling}
}

func executionTreeGroup(id int, fetches *FetchTreeNode) *DeferFetchGroup {
	return &DeferFetchGroup{DeferID: id, Fetches: fetches}
}

func collectExecutionTreeFetches(node *FetchTreeNode) []*FetchTreeNode {
	if node == nil {
		return nil
	}
	if node.Kind == FetchTreeNodeKindSingle {
		return []*FetchTreeNode{node}
	}
	var out []*FetchTreeNode
	for _, child := range node.ChildNodes {
		out = append(out, collectExecutionTreeFetches(child)...)
	}
	return out
}

func TestGraphQLDeferResponsePlannedExecutionTree(t *testing.T) {
	t.Run("returns primary unchanged without a defer tree", func(t *testing.T) {
		primary := executionTreeFetch(1, "primary")
		response := &GraphQLDeferResponse{Response: &GraphQLResponse{Fetches: primary}}

		assert.Same(t, primary, response.PlannedExecutionTree())
	})

	t.Run("preserves nested sequential and sibling parallel topology", func(t *testing.T) {
		primary := executionTreeFetch(1, "primary")
		primary.NormalizedQuery = "query Example { primary nested sibling }"
		parent := executionTreeFetch(2, "parent")
		nested := executionTreeFetch(3, "nested")
		sibling := executionTreeFetch(4, "sibling")
		response := &GraphQLDeferResponse{
			Response: &GraphQLResponse{Fetches: primary},
			DeferDescriptors: map[int]DeferDescriptor{
				1: {ID: 1, Label: "Parent", Path: []string{"parent"}},
				2: {ID: 2, ParentID: 1, Label: "Nested", Path: []string{"parent", "nested"}},
				3: {ID: 3, Label: "Sibling", Path: []string{"sibling"}},
			},
			DeferTree: DeferParallel(
				DeferSequence(
					DeferSingle(executionTreeGroup(1, parent)),
					DeferSingle(executionTreeGroup(2, nested)),
				),
				DeferSingle(executionTreeGroup(3, sibling)),
			),
		}

		planned := response.PlannedExecutionTree()

		require.NotNil(t, planned)
		assert.Equal(t, FetchTreeNodeKindSequence, planned.Kind)
		assert.Equal(t, primary.NormalizedQuery, planned.NormalizedQuery)
		require.Len(t, planned.ChildNodes, 2)
		assert.Same(t, primary, planned.ChildNodes[0])

		deferred := planned.ChildNodes[1]
		assert.Equal(t, FetchTreeNodeKindParallel, deferred.Kind)
		require.Len(t, deferred.ChildNodes, 2)
		assert.Equal(t, FetchTreeNodeKindSequence, deferred.ChildNodes[0].Kind)

		fetches := collectExecutionTreeFetches(planned)
		require.Len(t, fetches, 4)
		assert.Same(t, primary, fetches[0])
		assert.Same(t, parent, fetches[1])
		assert.Same(t, nested, fetches[2])
		assert.Same(t, sibling, fetches[3])
	})

	t.Run("ignores nil and empty defer nodes", func(t *testing.T) {
		primary := executionTreeFetch(1, "primary")
		response := &GraphQLDeferResponse{
			Response:  &GraphQLResponse{Fetches: primary},
			DeferTree: DeferParallel(nil, DeferSingle(nil), DeferSequence()),
		}

		assert.Same(t, primary, response.PlannedExecutionTree())
		assert.Nil(t, (*GraphQLDeferResponse)(nil).PlannedExecutionTree())
		assert.Nil(t, (&GraphQLDeferResponse{}).PlannedExecutionTree())
	})

	t.Run("retains a defer tree when the primary tree is nil", func(t *testing.T) {
		deferred := executionTreeFetch(1, "deferred")
		response := &GraphQLDeferResponse{
			DeferDescriptors: map[int]DeferDescriptor{1: {ID: 1, Label: "Deferred"}},
			DeferTree:        DeferSingle(executionTreeGroup(1, deferred)),
		}

		planned := response.PlannedExecutionTree()
		require.NotNil(t, planned)
		assert.Equal(t, FetchTreeNodeKindSequence, planned.Kind)
		require.Len(t, planned.ChildNodes, 1)
		assert.Same(t, deferred, planned.ChildNodes[0])
	})
}

func TestGraphQLDeferResponseQueryPlanUsesCompositeExecutionTree(t *testing.T) {
	primary := executionTreeFetch(1, "primary")
	primary.NormalizedQuery = "query Example { primary slow }"
	deferred := executionTreeFetch(2, "slow")
	response := &GraphQLDeferResponse{
		Response: &GraphQLResponse{Fetches: primary},
		DeferDescriptors: map[int]DeferDescriptor{
			1: {ID: 1, Label: "Slow", Path: []string{"slow"}},
		},
		DeferTree: DeferSingle(executionTreeGroup(1, deferred)),
	}

	plan := response.QueryPlan()
	require.NotNil(t, plan)
	assert.Equal(t, primary.NormalizedQuery, plan.NormalizedQuery)
	require.Len(t, plan.Children, 2)
	require.NotNil(t, plan.Children[1].Defer)
	assert.Equal(t, FetchTreeDeferDescriptor{ID: 1, Label: "Slow", Path: []string{"slow"}}, *plan.Children[1].Defer)
	assert.Nil(t, plan.Children[0].Defer)

	encoded, err := json.Marshal(plan)
	require.NoError(t, err)
	assert.JSONEq(t, `{
		"version":"1",
		"kind":"Sequence",
		"children":[
			{"kind":"Single","fetch":{"kind":"Single","subgraphName":"primary","subgraphId":"primary","fetchId":1,"query":"query { primary }"},"normalizedQuery":"query Example { primary slow }"},
			{"kind":"Sequence","children":[{"kind":"Single","fetch":{"kind":"Single","subgraphName":"slow","subgraphId":"slow","fetchId":2,"query":"query { slow }"}}],"defer":{"id":1,"label":"Slow","path":["slow"]}}
		],
		"normalizedQuery":"query Example { primary slow }"
	}`, string(encoded))

	pretty := response.QueryPlanString()
	assert.Equal(t, strings.TrimSpace(`
QueryPlan {
  Sequence {
    Fetch(service: "primary") {
      query { primary }
    }
    Defer(label: "Slow") {
      Fetch(service: "slow") {
        query { slow }
      }
    }
  }
}`), strings.TrimSpace(pretty))
}

func TestFetchTreeDeferMetadataIsOmittedFromNonDeferJSON(t *testing.T) {
	nonDefer := Sequence(executionTreeFetch(1, "primary"))

	plan, err := json.Marshal(nonDefer.QueryPlan())
	require.NoError(t, err)
	assert.Equal(t, `{"version":"1","kind":"Sequence","children":[{"kind":"Single","fetch":{"kind":"Single","subgraphName":"primary","subgraphId":"primary","fetchId":1,"query":"query { primary }"}}]}`, string(plan))

	trace, err := json.Marshal(nonDefer.Trace())
	require.NoError(t, err)
	assert.Equal(t, `{"kind":"Sequence","children":[{"kind":"Single","fetch":{"kind":"Single","path":"","source_id":"primary","source_name":"primary"}}]}`, string(trace))
}

func TestGraphQLDeferResponseExecutionTreeRuntimeStatuses(t *testing.T) {
	response, fetches := executionTraceResponse()

	runtime := response.NewDeferExecutionTraceTree()

	require.NotNil(t, runtime)
	require.NotNil(t, runtime.Root)
	actualFetches := collectExecutionTreeFetches(runtime.Root)
	require.Len(t, actualFetches, len(fetches))
	for i := range fetches {
		assert.Same(t, fetches[i], actualFetches[i])
	}
	for _, id := range []int{1, 2, 3} {
		status, ok := runtime.Status(id)
		assert.True(t, ok)
		assert.Equal(t, DeferExecutionStatusPlanned, status)
	}
	assert.False(t, runtime.AllTerminal())

	assert.True(t, runtime.MarkRunning(1))
	assert.False(t, runtime.MarkRunning(1), "running is not a repeatable transition")
	assert.True(t, runtime.MarkCompleted(1))
	assert.False(t, runtime.MarkError(1), "a terminal wrapper cannot change status")

	assert.False(t, runtime.MarkCompleted(2), "planned must transition through running")
	assert.True(t, runtime.MarkRunning(2))
	assert.True(t, runtime.MarkError(2))
	assert.True(t, runtime.MarkSkipped(3))
	assert.True(t, runtime.AllTerminal())

	trace := runtime.Root.Trace()
	require.NotNil(t, trace)
	traceStatuses := make(map[int]DeferExecutionStatus)
	var visit func(*FetchTreeTraceNode)
	visit = func(node *FetchTreeTraceNode) {
		if node == nil {
			return
		}
		if node.Defer != nil {
			traceStatuses[node.Defer.ID] = node.Defer.Status
		}
		for _, child := range node.Children {
			visit(child)
		}
	}
	visit(trace)
	assert.Equal(t, map[int]DeferExecutionStatus{
		1: DeferExecutionStatusCompleted,
		2: DeferExecutionStatusError,
		3: DeferExecutionStatusSkipped,
	}, traceStatuses)
}

func TestDeferExecutionTreeMarkSkippedIncludesDescendants(t *testing.T) {
	response, _ := executionTraceResponse()
	runtime := response.NewDeferExecutionTraceTree()

	assert.True(t, runtime.MarkSkipped(1))
	assert.Equal(t, DeferExecutionStatusSkipped, mustDeferStatus(t, runtime, 1))
	assert.Equal(t, DeferExecutionStatusSkipped, mustDeferStatus(t, runtime, 2))
	assert.Equal(t, DeferExecutionStatusPlanned, mustDeferStatus(t, runtime, 3))
	assert.False(t, runtime.MarkSkipped(1), "skipping an already terminal subtree is a no-op")
}

func TestDeferExecutionTreeMarkDescendantsSkippedAfterParentError(t *testing.T) {
	response, _ := executionTraceResponse()
	runtime := response.NewDeferExecutionTraceTree()

	require.True(t, runtime.MarkRunning(1))
	require.True(t, runtime.MarkError(1))
	assert.True(t, runtime.MarkDescendantsSkipped(1))
	assert.Equal(t, DeferExecutionStatusError, mustDeferStatus(t, runtime, 1))
	assert.Equal(t, DeferExecutionStatusSkipped, mustDeferStatus(t, runtime, 2))
	assert.Equal(t, DeferExecutionStatusPlanned, mustDeferStatus(t, runtime, 3))
}

func TestDeferExecutionTreePruneDeadDefersMarksRejectedSubtrees(t *testing.T) {
	t.Run("top-level pruning", func(t *testing.T) {
		response, _ := executionTraceResponse()
		runtime := response.NewDeferExecutionTraceTree()

		live := runtime.PruneDeadDefers(response.DeferTree, map[int]DeferDescriptor{
			3: response.DeferDescriptors[3],
		})

		require.NotNil(t, live)
		assert.Equal(t, []int{3}, leafIDs(live))
		assert.Equal(t, DeferExecutionStatusSkipped, mustDeferStatus(t, runtime, 1))
		assert.Equal(t, DeferExecutionStatusSkipped, mustDeferStatus(t, runtime, 2))
		assert.Equal(t, DeferExecutionStatusPlanned, mustDeferStatus(t, runtime, 3))
	})

	t.Run("nested pruning after a parent completes", func(t *testing.T) {
		response, _ := executionTraceResponse()
		runtime := response.NewDeferExecutionTraceTree()
		parentBranch := response.DeferTree.ChildNodes[0]
		nestedBranch := parentBranch.ChildNodes[1]

		require.True(t, runtime.MarkRunning(1))
		require.True(t, runtime.MarkCompleted(1))
		live := runtime.PruneDeadDefers(nestedBranch, map[int]DeferDescriptor{})

		assert.Nil(t, live)
		assert.Equal(t, DeferExecutionStatusCompleted, mustDeferStatus(t, runtime, 1))
		assert.Equal(t, DeferExecutionStatusSkipped, mustDeferStatus(t, runtime, 2))
		assert.Equal(t, DeferExecutionStatusPlanned, mustDeferStatus(t, runtime, 3))
	})
}

func TestDeferExecutionTreeStatusIsRequestLocal(t *testing.T) {
	response, _ := executionTraceResponse()
	first := response.NewDeferExecutionTraceTree()
	second := response.NewDeferExecutionTraceTree()

	var wg sync.WaitGroup
	results := make(chan bool, 2)
	wg.Add(2)
	go func() {
		defer wg.Done()
		results <- first.MarkRunning(1) && first.MarkCompleted(1)
	}()
	go func() {
		defer wg.Done()
		results <- second.MarkSkipped(1)
	}()
	wg.Wait()
	close(results)
	for result := range results {
		assert.True(t, result)
	}

	assert.Equal(t, DeferExecutionStatusCompleted, mustDeferStatus(t, first, 1))
	assert.Equal(t, DeferExecutionStatusPlanned, mustDeferStatus(t, first, 2))
	assert.Equal(t, DeferExecutionStatusSkipped, mustDeferStatus(t, second, 1))
	assert.Equal(t, DeferExecutionStatusSkipped, mustDeferStatus(t, second, 2))

	plannedTrace := response.PlannedExecutionTree().Trace()
	var plannedStatus DeferExecutionStatus
	var visit func(*FetchTreeTraceNode)
	visit = func(node *FetchTreeTraceNode) {
		if node == nil {
			return
		}
		if node.Defer != nil && node.Defer.ID == 1 {
			plannedStatus = node.Defer.Status
		}
		for _, child := range node.Children {
			visit(child)
		}
	}
	visit(plannedTrace)
	assert.Empty(t, plannedStatus, "request statuses must not leak into the cached plan")
}

func TestDeferExecutionTreeNilAndUnknownIDs(t *testing.T) {
	runtime := (*GraphQLDeferResponse)(nil).NewDeferExecutionTraceTree()
	require.NotNil(t, runtime)
	assert.Nil(t, runtime.Root)
	assert.True(t, runtime.AllTerminal())
	_, ok := runtime.Status(1)
	assert.False(t, ok)
	assert.False(t, runtime.MarkRunning(1))
	assert.False(t, runtime.MarkCompleted(1))
	assert.False(t, runtime.MarkError(1))
	assert.False(t, runtime.MarkSkipped(1))
	assert.False(t, runtime.MarkDescendantsSkipped(1))
	assert.Nil(t, runtime.PruneDeadDefers(nil, nil))
}

func mustDeferStatus(t *testing.T, tree *DeferExecutionTraceTree, id int) DeferExecutionStatus {
	t.Helper()
	status, ok := tree.Status(id)
	require.True(t, ok)
	return status
}
