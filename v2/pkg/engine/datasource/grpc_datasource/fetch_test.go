package grpcdatasource

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func BenchmarkBuildDependencyGraph(b *testing.B) {
	executionPlan := &RPCExecutionPlan{
		Calls: []RPCCall{
			{
				Kind:           CallKindStandard,
				DependentCalls: []int{1},
				MethodName:     "Method1",
			},
			{
				Kind:       CallKindStandard,
				MethodName: "Method2",
			},
			{
				Kind:           CallKindStandard,
				DependentCalls: []int{0},
				MethodName:     "Method3",
			},
			{
				Kind:           CallKindStandard,
				DependentCalls: []int{0},
				MethodName:     "Method4",
			},
			{
				Kind:           CallKindStandard,
				DependentCalls: []int{0, 2},
				MethodName:     "Method5",
			},
			{
				Kind:       CallKindStandard,
				MethodName: "Method6",
			},
		},
	}
	graph := NewDependencyGraph(executionPlan)
	for b.Loop() {
		_ = graph.TopologicalSortResolve(func(nodes []FetchItem) error {
			return nil
		})
	}
}

func TestBuildDependencyGraph(t *testing.T) {
	t.Parallel()
	t.Run("Simple execution plan with single root dependencies", func(t *testing.T) {
		t.Parallel()
		executionPlan := &RPCExecutionPlan{
			Calls: []RPCCall{
				{
					Kind:           CallKindStandard,
					DependentCalls: []int{1},
					MethodName:     "Method1",
				},
				{
					Kind:       CallKindStandard,
					MethodName: "Method2",
				},
				{
					Kind:           CallKindStandard,
					DependentCalls: []int{0},
					MethodName:     "Method3",
				},
				{
					Kind:           CallKindStandard,
					DependentCalls: []int{0},
					MethodName:     "Method4",
				},
				{
					Kind:           CallKindStandard,
					DependentCalls: []int{0, 2},
					MethodName:     "Method5",
				},
				{
					Kind:       CallKindStandard,
					MethodName: "Method6",
				},
			},
		}

		graph := NewDependencyGraph(executionPlan)
		require.Equal(t, 6, len(graph.nodes))

		result := make([]FetchItem, 0)
		err := graph.TopologicalSortResolve(func(nodes []FetchItem) error {
			result = append(result, nodes...)
			return nil
		})
		require.NoError(t, err)
		require.Equal(t, []FetchItem{graph.fetches[1], graph.fetches[5], graph.fetches[0], graph.fetches[2], graph.fetches[3], graph.fetches[4]}, result)
	})

	t.Run("Should resolve the nodes in the correct order", func(t *testing.T) {
		t.Parallel()
		executionPlan := &RPCExecutionPlan{
			Calls: []RPCCall{
				{
					Kind:           CallKindStandard,
					DependentCalls: []int{1},
				},
				{
					Kind: CallKindStandard,
				},
			},
		}

		graph := NewDependencyGraph(executionPlan)
		require.Equal(t, 2, len(graph.nodes))

		result := make([]FetchItem, 0)

		err := graph.TopologicalSortResolve(func(nodes []FetchItem) error {
			result = append(result, nodes...)
			return nil
		})

		require.Equal(t, []FetchItem{graph.fetches[1], graph.fetches[0]}, result)
		require.NoError(t, err)
	})

	t.Run("Should raise error if there is a cycle", func(t *testing.T) {
		t.Parallel()
		executionPlan := &RPCExecutionPlan{
			Calls: []RPCCall{
				{
					Kind:           CallKindStandard,
					DependentCalls: []int{1},
				},
				{
					Kind:           CallKindStandard,
					DependentCalls: []int{0},
				},
			},
		}

		graph := NewDependencyGraph(executionPlan)
		require.Equal(t, 2, len(graph.nodes))
		require.Equal(t, []int{1}, graph.nodes[0])
		require.Equal(t, []int{0}, graph.nodes[1])

		err := graph.TopologicalSortResolve(func(nodes []FetchItem) error {
			return nil
		})
		require.Error(t, err)
		require.Contains(t, err.Error(), "cycle detected")
	})

	t.Run("Should raise an error if the execution plan is missing a call", func(t *testing.T) {
		t.Parallel()
		executionPlan := &RPCExecutionPlan{
			Calls: []RPCCall{
				{
					Kind:           CallKindStandard,
					DependentCalls: []int{1},
				},
				{
					Kind:           CallKindStandard,
					DependentCalls: []int{2},
				},
			},
		}

		graph := NewDependencyGraph(executionPlan)
		require.Equal(t, 2, len(graph.nodes))

		err := graph.TopologicalSortResolve(func(nodes []FetchItem) error {
			return nil
		})
		require.Error(t, err)
		require.Contains(t, err.Error(), "unable to find dependent call 2 in execution plan")
	})
}
