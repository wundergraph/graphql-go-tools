package resolve

import (
	"encoding/json"
	"strings"
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
