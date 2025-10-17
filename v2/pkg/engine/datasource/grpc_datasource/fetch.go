package grpcdatasource

import (
	"fmt"
	"sync"
)

type FetchItem struct {
	ID               int
	Plan             *RPCCall
	ServiceCall      *ServiceCall
	DependentFetches []int
}

type DependencyGraph struct {
	mu      sync.Mutex
	fetches []FetchItem
	// nodes is a list of lists of dependent calls.
	// Each node index corresponds to a call index in the execution plan
	// and the list contains the corresponding dependent calls indices.
	nodes [][]int
}

func NewDependencyGraph(executionPlan *RPCExecutionPlan) *DependencyGraph {
	graph := &DependencyGraph{
		nodes:   make([][]int, len(executionPlan.Calls)),
		fetches: make([]FetchItem, len(executionPlan.Calls)),
	}

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

func (g *DependencyGraph) TopologicalSortResolve(resolver func(nodes []FetchItem) error) error {
	// We are using a slice to store the batch index for each noded ordered.
	callHierarchyRefs := initializeSlice(len(g.nodes), -1)
	cycleChecks := make([]bool, len(g.nodes))

	var visit func(index int) error
	visit = func(index int) error {
		if cycleChecks[index] {
			return fmt.Errorf("cycle detected")
		}

		cycleChecks[index] = true

		if len(g.nodes[index]) == 0 {
			callHierarchyRefs[index] = 0
			return nil
		}

		currentLevel := 0
		for _, depCallIndex := range g.nodes[index] {
			if depCallIndex < 0 || depCallIndex >= len(g.nodes) {
				return fmt.Errorf("unable to find dependent call %d in execution plan", depCallIndex)
			}

			if depLevel := callHierarchyRefs[depCallIndex]; depLevel >= 0 {
				if depLevel > currentLevel {
					currentLevel = depLevel
				}
				continue
			}

			if err := visit(depCallIndex); err != nil {
				return err
			}

			if l := callHierarchyRefs[depCallIndex]; l > currentLevel {
				currentLevel = l
			}
		}

		callHierarchyRefs[index] = currentLevel + 1
		return nil
	}

	for node := range g.nodes {
		if err := visit(node); err != nil {
			return err
		}

		clear(cycleChecks)
	}

	chunks := make(map[int][]FetchItem)

	for callIndex, chunkIndex := range callHierarchyRefs {
		chunks[chunkIndex] = append(chunks[chunkIndex], g.fetches[callIndex])
	}

	for i := 0; i < len(chunks); i++ {
		if err := resolver(chunks[i]); err != nil {
			return err
		}
	}

	return nil
}

func (g *DependencyGraph) Fetch(index int) (FetchItem, error) {
	if index < 0 || index >= len(g.fetches) {
		return FetchItem{}, fmt.Errorf("unable to find fetch %d in execution plan", index)
	}

	return g.fetches[index], nil
}

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

func (g *DependencyGraph) SetFetchData(index int, serviceCall *ServiceCall) {
	g.mu.Lock()
	g.fetches[index].ServiceCall = serviceCall
	g.mu.Unlock()
}

func initializeSlice[T any](len int, zero T) []T {
	s := make([]T, len)
	for i := range s {
		s[i] = zero
	}
	return s
}
