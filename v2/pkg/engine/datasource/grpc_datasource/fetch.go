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
type DependencyGraph struct {
	mu      sync.Mutex
	fetches []FetchItem
	// nodes is a list of lists of dependent calls.
	// Each node index corresponds to a call index in the execution plan
	// and the list contains the corresponding dependent calls indices.
	// Visual representation of the nodes:
	// 0 -> [] // no dependencies
	// 1 -> [0] // depends on 0
	// 2 -> [0] // depends on 0
	// 3 -> [0] // depends on 0
	// 4 -> [0, 2] // depends on 0 and 2
	// 5 -> [0, 2, 3] // depends on 0, 2 and 3
	nodes [][]int
}

func NewDependencyGraph(executionPlan *RPCExecutionPlan) *DependencyGraph {
	graph := &DependencyGraph{
		nodes:   make([][]int, len(executionPlan.Calls)),
		fetches: make([]FetchItem, len(executionPlan.Calls)),
	}

	// Initialize the graph with the calls in the execution plan.
	// We create a FetchItem for each call and store the dependent call references.
	for index, call := range executionPlan.Calls {
		graph.nodes[index] = call.DependentCalls
		graph.fetches[index] = FetchItem{
			ID:               index,
			Plan:             &call,
			ServiceCall:      nil,
			DependentFetches: call.DependentCalls,
		}
	}

	return graph
}

// TopologicalSortResolve sorts the calls in the execution plan in a topological order.
// In order to perform calls in the correct order, we need to determine the dependencies between the calls.
// We are using a depth-first search to determine the dependencies between the calls by
// building an index map of the call hierarchy. Each index in the index map corresponds to a call index in the execution plan.
// The map contains the level of the call in the hierarchy. The root call has a level of 0.
func (g *DependencyGraph) TopologicalSortResolve(resolver func(nodes []FetchItem) error) error {
	// We are using a slice to store the batch index for each noded ordered.
	callHierarchyRefs := initializeSlice(len(g.nodes), -1)
	cycleChecks := make([]bool, len(g.nodes))

	var visit func(index int) error
	visit = func(index int) error {
		if cycleChecks[index] {
			return fmt.Errorf("cycle detected")
		}

		// We are marking the call as visited to avoid cycles.
		cycleChecks[index] = true

		if len(g.nodes[index]) == 0 {
			// If the call has no dependencies, we are setting the level to 0 and return early.
			callHierarchyRefs[index] = 0
			return nil
		}

		currentLevel := 0
		// We are iterating over the dependent calls of the current call.
		for _, depCallIndex := range g.nodes[index] {
			if depCallIndex < 0 || depCallIndex >= len(g.nodes) {
				return fmt.Errorf("unable to find dependent call %d in execution plan", depCallIndex)
			}

			// If the dependent call has already been visited, we are checking if the level of the dependent call is greater than the current level.
			// If it is, we are updating the current level to the level of the dependent call.
			if depLevel := callHierarchyRefs[depCallIndex]; depLevel >= 0 {
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
			if l := callHierarchyRefs[depCallIndex]; l > currentLevel {
				currentLevel = l
			}
		}

		// After receiving the maximum level of the dependent calls, we increment the level by 1.
		callHierarchyRefs[index] = currentLevel + 1
		return nil
	}

	for node := range g.nodes {
		if err := visit(node); err != nil {
			return err
		}

		clear(cycleChecks)
	}

	// After setting up the call hierarchy, we are grouping the calls by their level.
	chunks := make(map[int][]FetchItem)
	for callIndex, chunkIndex := range callHierarchyRefs {
		chunks[chunkIndex] = append(chunks[chunkIndex], g.fetches[callIndex])
	}

	// We are iterating over the chunks and resolving the calls in the correct order.
	for i := 0; i < len(chunks); i++ {
		if err := resolver(chunks[i]); err != nil {
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
	g.mu.Lock()
	g.fetches[index].ServiceCall = serviceCall
	g.mu.Unlock()
}

// initializeSlice initializes a slice with a given length and a given value.
func initializeSlice[T any](len int, zero T) []T {
	s := make([]T, len)
	for i := range s {
		s[i] = zero
	}
	return s
}
