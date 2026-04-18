package grpcdatasource

import (
	"fmt"
	"sync"
)

// FetchItem is a single fetch item in the execution plan.
// It contains the call information, the service call, and a list of references to the dependent fetches.
type FetchItem struct {
	ID               int
	Plan             *RPCCall
	ServiceCall      *ServiceCall
	DependentFetches []int
}

// DependencyGraph is a graph of the calls in the execution plan.
// It is used to determine the order in which to execute the calls.
//
// The graph is split into two parts:
//
//   - Static topology (`nodes`, `fetchTemplates`) — derived purely from the plan,
//     built once in NewDependencyGraph and never mutated.
//   - Per-request state (`fetches`, traversal scratch) — mutated during Load.
//
// The static topology is safe to share across requests; the per-request state
// is acquired from a sync.Pool for each Load via TopologicalSortResolve.
type DependencyGraph struct {
	// fetches is populated per-request during resolve; carries the ServiceCall pointers.
	fetches []FetchItem
	// nodes is the static dependency adjacency list.
	// Each node index corresponds to a call index in the execution plan
	// and the list contains the corresponding dependent call indices.
	nodes [][]int
}

// graphState holds the scratch slices needed by TopologicalSortResolve.
// It is pooled via graphStatePool to avoid per-request allocation.
type graphState struct {
	callHierarchyRefs []int
	cycleChecks       []bool
	// chunks maps level -> []FetchItem for the resolve callback.
	// Stored as a slice-of-slices keyed by level index since levels are dense from 0..maxLevel.
	chunks [][]FetchItem
}

// reset prepares the graph state for the given node count.
// Slices are re-sliced to the needed length, reusing underlying capacity.
func (s *graphState) reset(n int) {
	if cap(s.callHierarchyRefs) < n {
		s.callHierarchyRefs = make([]int, n)
	} else {
		s.callHierarchyRefs = s.callHierarchyRefs[:n]
	}
	for i := range s.callHierarchyRefs {
		s.callHierarchyRefs[i] = -1
	}

	if cap(s.cycleChecks) < n {
		s.cycleChecks = make([]bool, n)
	} else {
		s.cycleChecks = s.cycleChecks[:n]
		clear(s.cycleChecks)
	}

	// Reset chunks: keep capacity of outer slice and each inner slice, but zero length.
	if cap(s.chunks) < n {
		s.chunks = make([][]FetchItem, 0, n)
	} else {
		for i := range s.chunks {
			s.chunks[i] = s.chunks[i][:0]
		}
		s.chunks = s.chunks[:0]
	}
}

var graphStatePool = sync.Pool{
	New: func() any {
		return &graphState{}
	},
}

func NewDependencyGraph(executionPlan *RPCExecutionPlan) *DependencyGraph {
	graph := &DependencyGraph{
		nodes:   make([][]int, len(executionPlan.Calls)),
		fetches: make([]FetchItem, len(executionPlan.Calls)),
	}

	// Initialize the graph with the calls in the execution plan.
	// We create a FetchItem for each call and store the dependent call references.
	for _, call := range executionPlan.Calls {
		graph.nodes[call.ID] = call.DependentCalls
		graph.fetches[call.ID] = FetchItem{
			ID:               call.ID,
			Plan:             &call,
			ServiceCall:      nil,
			DependentFetches: call.DependentCalls,
		}
	}

	return graph
}

// resetForReuse clears the mutable per-request state (ServiceCall pointers) so the
// graph can be returned to a pool and reused for another Load. The static topology
// (nodes, plan pointers, IDs, dependent fetches) is preserved since it's derived
// from the immutable plan.
func (g *DependencyGraph) resetForReuse() {
	for i := range g.fetches {
		g.fetches[i].ServiceCall = nil
	}
}

// TopologicalSortResolve sorts the calls in the execution plan in a topological order.
// In order to perform calls in the correct order, we need to determine the dependencies between the calls.
// We are using a depth-first search to determine the dependencies between the calls by
// building an index map of the call hierarchy. Each index in the index map corresponds to a call index in the execution plan.
// The map contains the level of the call in the hierarchy. The root call has a level of 0.
func (g *DependencyGraph) TopologicalSortResolve(resolver func(nodes []FetchItem) error) error {
	state := graphStatePool.Get().(*graphState)
	defer graphStatePool.Put(state)

	n := len(g.nodes)
	state.reset(n)

	var visit func(index int) error
	visit = func(index int) error {
		if state.cycleChecks[index] {
			return fmt.Errorf("cycle detected")
		}

		// We are marking the call as visited to avoid cycles.
		state.cycleChecks[index] = true

		if len(g.nodes[index]) == 0 {
			// If the call has no dependencies, we are setting the level to 0 and return early.
			state.callHierarchyRefs[index] = 0
			return nil
		}

		currentLevel := 0
		// We are iterating over the dependent calls of the current call.
		for _, depCallIndex := range g.nodes[index] {
			if depCallIndex < 0 || depCallIndex >= n {
				return fmt.Errorf("unable to find dependent call %d in execution plan", depCallIndex)
			}

			// If the dependent call has already been visited, we are checking if the level of the dependent call is greater than the current level.
			// If it is, we are updating the current level to the level of the dependent call.
			if depLevel := state.callHierarchyRefs[depCallIndex]; depLevel >= 0 {
				if depLevel > currentLevel {
					currentLevel = depLevel
				}
				continue
			}

			// If the dependent call has not been visited, we are visiting it.
			if err := visit(depCallIndex); err != nil {
				return err
			}

			// If the level of the dependent call is greater than the current level, we are updating the current level to the level of the dependent call.
			if l := state.callHierarchyRefs[depCallIndex]; l > currentLevel {
				currentLevel = l
			}
		}

		// After receiving the maximum level of the dependent calls, we increment the level by 1.
		state.callHierarchyRefs[index] = currentLevel + 1
		return nil
	}

	for node := range g.nodes {
		if err := visit(node); err != nil {
			return err
		}

		clear(state.cycleChecks)
	}

	// After setting up the call hierarchy, group the calls by their level.
	// Levels are dense from 0..maxLevel; a slice-of-slices avoids the map allocation.
	maxLevel := -1
	for _, l := range state.callHierarchyRefs {
		if l > maxLevel {
			maxLevel = l
		}
	}
	for i := 0; i <= maxLevel; i++ {
		if i < len(state.chunks) {
			state.chunks[i] = state.chunks[i][:0]
		} else {
			state.chunks = append(state.chunks, nil)
		}
	}
	for callIndex, level := range state.callHierarchyRefs {
		state.chunks[level] = append(state.chunks[level], g.fetches[callIndex])
	}

	// Iterate over the chunks and resolve the calls in the correct order.
	for i := 0; i <= maxLevel; i++ {
		if err := resolver(state.chunks[i]); err != nil {
			return err
		}
	}

	return nil
}

// Fetch returns the fetch item for a given index.
func (g *DependencyGraph) Fetch(index int) (FetchItem, error) {
	if index < 0 || index >= len(g.fetches) {
		return FetchItem{}, fmt.Errorf("unable to find fetch %d in execution plan", index)
	}

	return g.fetches[index], nil
}

// FetchDependencies returns the dependencies for a given fetch item.
func (g *DependencyGraph) FetchDependencies(fetch *FetchItem) ([]FetchItem, error) {
	dependencies := make([]FetchItem, 0, len(fetch.DependentFetches))

	for _, dependentFetch := range fetch.DependentFetches {
		dependency, err := g.Fetch(dependentFetch)
		if err != nil {
			return nil, err
		}
		dependencies = append(dependencies, dependency)
	}

	return dependencies, nil
}

// SetFetchData sets the service call for a given index.
func (g *DependencyGraph) SetFetchData(index int, serviceCall *ServiceCall) {
	g.fetches[index].ServiceCall = serviceCall
}
