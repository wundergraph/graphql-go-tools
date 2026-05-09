package postprocess

import (
	"fmt"
	"slices"
	"strings"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

type buildScheduleTreeProcessor struct {
	disable bool
}

func (b *buildScheduleTreeProcessor) ProcessFetchTree(root *resolve.FetchTreeNode) {
	if b.disable || root == nil || root.Kind != resolve.FetchTreeNodeKindSequence {
		return
	}
	dag, err := newFetchDAG(root.ChildNodes)
	if err != nil {
		panic(err)
	}
	tree, err := buildScheduleTree(root.ChildNodes, dag)
	if err != nil {
		panic(err)
	}
	if err := validateSchedule(tree, dag); err != nil {
		panic(err)
	}
	if tree == nil {
		root.ChildNodes = nil
		return
	}
	*root = *tree
}

type fetchDAG struct {
	nodes    map[int]*resolve.FetchTreeNode
	parents  map[int]map[int]struct{}
	children map[int]map[int]struct{}
}

func newFetchDAG(nodes []*resolve.FetchTreeNode) (*fetchDAG, error) {
	dag := &fetchDAG{
		nodes:    make(map[int]*resolve.FetchTreeNode, len(nodes)),
		parents:  make(map[int]map[int]struct{}, len(nodes)),
		children: make(map[int]map[int]struct{}, len(nodes)),
	}
	for _, node := range nodes {
		if node == nil || node.Item == nil || node.Item.Fetch == nil {
			continue
		}
		id := node.Item.Fetch.Dependencies().FetchID
		if _, exists := dag.nodes[id]; exists {
			return nil, fmt.Errorf("duplicate fetch id %d", id)
		}
		dag.nodes[id] = node
		dag.parents[id] = make(map[int]struct{})
		dag.children[id] = make(map[int]struct{})
	}
	for id, node := range dag.nodes {
		for _, dep := range node.Item.Fetch.Dependencies().DependsOnFetchIDs {
			if _, exists := dag.nodes[dep]; !exists {
				continue
			}
			dag.parents[id][dep] = struct{}{}
			dag.children[dep][id] = struct{}{}
		}
	}
	return dag, nil
}

func (d *fetchDAG) sortedIDs() []int {
	ids := make([]int, 0, len(d.nodes))
	for id := range d.nodes {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	return ids
}

func (d *fetchDAG) hasCycle() bool {
	_, err := scheduleLevel(d.sortedIDs(), d)
	return err != nil
}

func buildScheduleTree(roots []*resolve.FetchTreeNode, dag *fetchDAG) (*resolve.FetchTreeNode, error) {
	ids := make([]int, 0, len(roots))
	for _, root := range roots {
		if root == nil || root.Item == nil || root.Item.Fetch == nil {
			continue
		}
		ids = append(ids, root.Item.Fetch.Dependencies().FetchID)
	}
	slices.Sort(ids)
	treeLevel, err := scheduleLevel(ids, dag)
	if err != nil {
		return nil, err
	}
	treeSP, err := scheduleSP(ids, dag)
	if err != nil {
		return nil, err
	}
	if err := validateSchedule(treeLevel, dag); err != nil {
		return nil, err
	}
	if err := validateSchedule(treeSP, dag); err != nil {
		return nil, err
	}
	if dominates(treeSP, treeLevel) {
		return treeSP, nil
	}
	return treeLevel, nil
}

func scheduleLevel(nodes []int, dag *fetchDAG) (*resolve.FetchTreeNode, error) {
	nodes = sortedUnique(nodes)
	if len(nodes) == 0 {
		return nil, nil
	}
	if len(nodes) == 1 {
		return dag.nodes[nodes[0]], nil
	}
	components := weaklyConnectedComponents(nodes, dag)
	if len(components) > 1 {
		branches := make([]*resolve.FetchTreeNode, 0, len(components))
		for _, component := range components {
			child, err := scheduleLevel(component, dag)
			if err != nil {
				return nil, err
			}
			if child != nil {
				branches = append(branches, child)
			}
		}
		sortBranches(branches)
		return parallelOf(branches...), nil
	}
	allowed := idSet(nodes)
	roots := make([]int, 0, len(nodes))
	for _, id := range nodes {
		hasParent := false
		for parent := range dag.parents[id] {
			if _, ok := allowed[parent]; ok {
				hasParent = true
				break
			}
		}
		if !hasParent {
			roots = append(roots, id)
		}
	}
	if len(roots) == 0 {
		return nil, fmt.Errorf("cycle detected in fetch dependency graph")
	}
	rootNodes := make([]*resolve.FetchTreeNode, 0, len(roots))
	rootSet := idSet(roots)
	rest := make([]int, 0, len(nodes)-len(roots))
	for _, id := range roots {
		rootNodes = append(rootNodes, dag.nodes[id])
	}
	for _, id := range nodes {
		if _, ok := rootSet[id]; !ok {
			rest = append(rest, id)
		}
	}
	rootTree := parallelOf(rootNodes...)
	restTree, err := scheduleLevel(rest, dag)
	if err != nil {
		return nil, err
	}
	if restTree == nil {
		return rootTree, nil
	}
	return sequenceOf(rootTree, restTree), nil
}

func scheduleSP(nodes []int, dag *fetchDAG) (*resolve.FetchTreeNode, error) {
	nodes = sortedUnique(nodes)
	if len(nodes) == 0 {
		return nil, nil
	}
	if len(nodes) == 1 {
		return dag.nodes[nodes[0]], nil
	}
	components := weaklyConnectedComponents(nodes, dag)
	if len(components) > 1 {
		branches := make([]*resolve.FetchTreeNode, 0, len(components))
		for _, component := range components {
			child, err := scheduleSP(component, dag)
			if err != nil {
				return nil, err
			}
			if child != nil {
				branches = append(branches, child)
			}
		}
		sortBranches(branches)
		return parallelOf(branches...), nil
	}
	return scheduleSPInline(nodes, dag)
}

type processingState struct {
	ready     []int
	unhandled map[int]map[int]struct{}
}

func scheduleSPInline(nodes []int, dag *fetchDAG) (*resolve.FetchTreeNode, error) {
	allowed := idSet(nodes)
	roots := make([]int, 0, len(nodes))
	for _, id := range nodes {
		hasParent := false
		for parent := range dag.parents[id] {
			if _, ok := allowed[parent]; ok {
				hasParent = true
				break
			}
		}
		if !hasParent {
			roots = append(roots, id)
		}
	}
	if len(roots) == 0 {
		return nil, fmt.Errorf("cycle detected in fetch dependency graph")
	}
	state := processingState{
		ready:     roots,
		unhandled: map[int]map[int]struct{}{},
	}
	sequence := make([]*resolve.FetchTreeNode, 0, len(nodes))
	parallelFirst := len(roots) > 1
	processed := make(map[int]struct{}, len(nodes))
	for len(state.ready) != 0 {
		batch, nextState, err := processBatch(state, parallelFirst, dag, allowed, processed, map[int]struct{}{})
		if err != nil {
			return nil, err
		}
		if batch != nil {
			sequence = append(sequence, batch)
		}
		state = nextState
		parallelFirst = true
	}
	if len(processed) != len(nodes) || len(state.unhandled) != 0 {
		return nil, fmt.Errorf("cycle detected in fetch dependency graph")
	}
	return sequenceOf(sequence...), nil
}

func processBatch(state processingState, parallelFirst bool, dag *fetchDAG, allowed map[int]struct{}, processed map[int]struct{}, branchDone map[int]struct{}) (*resolve.FetchTreeNode, processingState, error) {
	ready := sortedUnique(state.ready)
	branches := make([]*resolve.FetchTreeNode, 0, len(ready))
	merged := processingState{
		unhandled: clonePending(state.unhandled),
	}
	for _, id := range ready {
		if _, ok := processed[id]; ok {
			continue
		}
		subtree, subState, err := processNode(id, dag, allowed, processed, cloneSet(branchDone))
		if err != nil {
			return nil, processingState{}, err
		}
		if subtree != nil {
			branches = append(branches, subtree)
		}
		merged = mergeStates(merged, subState, dag)
		for processedID := range processed {
			delete(merged.unhandled, processedID)
		}
	}
	processedReady := idSet(ready)
	merged.ready = removeIDs(sortedUnique(merged.ready), processedReady, processed)
	if len(branches) == 0 {
		return nil, merged, nil
	}
	sortBranches(branches)
	if len(branches) == 1 {
		return branches[0], merged, nil
	}
	if parallelFirst {
		return parallelOf(branches...), merged, nil
	}
	return sequenceOf(branches...), merged, nil
}

func processNode(id int, dag *fetchDAG, allowed map[int]struct{}, processed map[int]struct{}, branchDone map[int]struct{}) (*resolve.FetchTreeNode, processingState, error) {
	if _, ok := processed[id]; ok {
		return nil, processingState{unhandled: map[int]map[int]struct{}{}}, nil
	}
	processed[id] = struct{}{}
	branchDone[id] = struct{}{}
	childState := processingState{
		unhandled: map[int]map[int]struct{}{},
	}
	for _, child := range sortedSet(dag.children[id]) {
		if _, ok := allowed[child]; !ok {
			continue
		}
		parents := filterSet(dag.parents[child], allowed)
		if len(parents) == 1 {
			childState.ready = append(childState.ready, child)
			continue
		}
		pending := cloneSet(parents)
		for done := range branchDone {
			delete(pending, done)
		}
		if len(pending) == 0 {
			childState.ready = append(childState.ready, child)
			continue
		}
		childState.unhandled[child] = pending
	}
	if len(childState.ready) == 0 {
		return dag.nodes[id], childState, nil
	}
	sequence := []*resolve.FetchTreeNode{dag.nodes[id]}
	state := childState
	for len(state.ready) != 0 {
		batch, nextState, err := processBatch(state, true, dag, allowed, processed, branchDone)
		if err != nil {
			return nil, processingState{}, err
		}
		if batch != nil {
			sequence = append(sequence, batch)
		}
		state = nextState
	}
	return sequenceOf(sequence...), state, nil
}

func mergeStates(a, b processingState, dag *fetchDAG) processingState {
	merged := processingState{
		ready:     append([]int{}, a.ready...),
		unhandled: map[int]map[int]struct{}{},
	}
	merged.ready = append(merged.ready, b.ready...)
	keys := make(map[int]struct{}, len(a.unhandled)+len(b.unhandled))
	for key := range a.unhandled {
		keys[key] = struct{}{}
	}
	for key := range b.unhandled {
		keys[key] = struct{}{}
	}
	for key := range keys {
		left, leftOK := a.unhandled[key]
		if !leftOK {
			left = dag.parents[key]
		}
		right, rightOK := b.unhandled[key]
		if !rightOK {
			right = dag.parents[key]
		}
		pending := intersectSets(left, right)
		if len(pending) == 0 {
			merged.ready = append(merged.ready, key)
			continue
		}
		merged.unhandled[key] = pending
	}
	merged.ready = sortedUnique(merged.ready)
	for _, id := range merged.ready {
		delete(merged.unhandled, id)
	}
	return merged
}

func dominates(treeA, treeB *resolve.FetchTreeNode) bool {
	pathsA := enumeratePaths(treeA)
	pathsB := enumeratePaths(treeB)
	for _, pathA := range pathsA {
		found := false
		for _, pathB := range pathsB {
			if setContainsAll(pathB, pathA) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

func enumeratePaths(node *resolve.FetchTreeNode) []map[int]struct{} {
	if node == nil {
		return []map[int]struct{}{{}}
	}
	switch node.Kind {
	case resolve.FetchTreeNodeKindSingle:
		return []map[int]struct{}{{
			node.Item.Fetch.Dependencies().FetchID: {},
		}}
	case resolve.FetchTreeNodeKindParallel:
		var paths []map[int]struct{}
		for _, child := range node.ChildNodes {
			paths = append(paths, enumeratePaths(child)...)
		}
		return paths
	case resolve.FetchTreeNodeKindSequence:
		paths := []map[int]struct{}{{}}
		for _, child := range node.ChildNodes {
			childPaths := enumeratePaths(child)
			next := make([]map[int]struct{}, 0, len(paths)*len(childPaths))
			for _, existing := range paths {
				for _, childPath := range childPaths {
					combined := cloneSet(existing)
					for id := range childPath {
						combined[id] = struct{}{}
					}
					next = append(next, combined)
				}
			}
			paths = next
		}
		return paths
	default:
		return nil
	}
}

func validateSchedule(root *resolve.FetchTreeNode, dag *fetchDAG) error {
	_, err := validateScheduleNode(root, dag, map[int]struct{}{})
	return err
}

func validateScheduleNode(node *resolve.FetchTreeNode, dag *fetchDAG, before map[int]struct{}) ([]*resolve.FetchTreeNode, error) {
	if node == nil {
		return nil, nil
	}
	switch node.Kind {
	case resolve.FetchTreeNodeKindSingle:
		id := node.Item.Fetch.Dependencies().FetchID
		for _, dep := range node.Item.Fetch.Dependencies().DependsOnFetchIDs {
			if _, known := dag.nodes[dep]; !known {
				continue
			}
			if _, ok := before[dep]; !ok {
				return nil, fmt.Errorf("fetch %d depends on fetch %d before it is available", id, dep)
			}
		}
		return []*resolve.FetchTreeNode{node}, nil
	case resolve.FetchTreeNodeKindSequence:
		available := cloneSet(before)
		leaves := make([]*resolve.FetchTreeNode, 0)
		for _, child := range node.ChildNodes {
			childLeaves, err := validateScheduleNode(child, dag, available)
			if err != nil {
				return nil, err
			}
			for _, leaf := range childLeaves {
				available[leaf.Item.Fetch.Dependencies().FetchID] = struct{}{}
				leaves = append(leaves, leaf)
			}
		}
		return leaves, nil
	case resolve.FetchTreeNodeKindParallel:
		branchLeaves := make([][]*resolve.FetchTreeNode, len(node.ChildNodes))
		leaves := make([]*resolve.FetchTreeNode, 0)
		for i, child := range node.ChildNodes {
			childLeaves, err := validateScheduleNode(child, dag, before)
			if err != nil {
				return nil, err
			}
			branchLeaves[i] = childLeaves
			leaves = append(leaves, childLeaves...)
		}
		for i := 0; i < len(branchLeaves); i++ {
			for j := i + 1; j < len(branchLeaves); j++ {
				for _, a := range branchLeaves[i] {
					for _, b := range branchLeaves[j] {
						if err := validateParallelLeaves(a, b); err != nil {
							return nil, err
						}
					}
				}
			}
		}
		return leaves, nil
	default:
		return nil, nil
	}
}

func validateParallelLeaves(a, b *resolve.FetchTreeNode) error {
	aID := a.Item.Fetch.Dependencies().FetchID
	bID := b.Item.Fetch.Dependencies().FetchID
	if slices.Contains(a.Item.Fetch.Dependencies().DependsOnFetchIDs, bID) {
		return fmt.Errorf("fetch %d depends on parallel fetch %d", aID, bID)
	}
	if slices.Contains(b.Item.Fetch.Dependencies().DependsOnFetchIDs, aID) {
		return fmt.Errorf("fetch %d depends on parallel fetch %d", bID, aID)
	}
	aProvided := providedPath(a)
	bProvided := providedPath(b)
	aResponse := responsePath(a)
	bResponse := responsePath(b)
	if len(bProvided) != 0 && pathStrictPrefix(bProvided, aResponse) {
		return fmt.Errorf("parallel fetch %d at response path %s depends on fetch %d providing %s", aID, strings.Join(aResponse, "."), bID, strings.Join(bProvided, "."))
	}
	if len(aProvided) != 0 && pathStrictPrefix(aProvided, bResponse) {
		return fmt.Errorf("parallel fetch %d at response path %s depends on fetch %d providing %s", bID, strings.Join(bResponse, "."), aID, strings.Join(aProvided, "."))
	}
	if len(aProvided) != 0 && len(bProvided) != 0 && slices.Equal(aProvided, bProvided) {
		return fmt.Errorf("parallel fetches %d and %d provide the same response path %s", aID, bID, strings.Join(aProvided, "."))
	}
	return nil
}

func providedPath(node *resolve.FetchTreeNode) []string {
	base := responsePath(node)
	var merge []string
	switch fetch := node.Item.Fetch.(type) {
	case *resolve.SingleFetch:
		merge = fetch.PostProcessing.MergePath
	case *resolve.EntityFetch:
		merge = fetch.PostProcessing.MergePath
	case *resolve.BatchEntityFetch:
		merge = fetch.PostProcessing.MergePath
	}
	out := make([]string, 0, len(base)+len(merge))
	out = append(out, base...)
	out = append(out, merge...)
	return out
}

func responsePath(node *resolve.FetchTreeNode) []string {
	if len(node.Item.ResponsePathElements) != 0 {
		return append([]string{}, node.Item.ResponsePathElements...)
	}
	if node.Item.ResponsePath == "" {
		return nil
	}
	return strings.Split(node.Item.ResponsePath, ".")
}

func pathStrictPrefix(prefix, path []string) bool {
	if len(prefix) >= len(path) {
		return false
	}
	for i := range prefix {
		if prefix[i] != path[i] {
			return false
		}
	}
	return true
}

func weaklyConnectedComponents(nodes []int, dag *fetchDAG) [][]int {
	allowed := idSet(nodes)
	seen := map[int]struct{}{}
	components := make([][]int, 0)
	for _, id := range nodes {
		if _, ok := seen[id]; ok {
			continue
		}
		queue := []int{id}
		seen[id] = struct{}{}
		component := make([]int, 0)
		for len(queue) != 0 {
			current := queue[0]
			queue = queue[1:]
			component = append(component, current)
			for neighbor := range dag.parents[current] {
				if _, ok := allowed[neighbor]; !ok {
					continue
				}
				if _, ok := seen[neighbor]; ok {
					continue
				}
				seen[neighbor] = struct{}{}
				queue = append(queue, neighbor)
			}
			for neighbor := range dag.children[current] {
				if _, ok := allowed[neighbor]; !ok {
					continue
				}
				if _, ok := seen[neighbor]; ok {
					continue
				}
				seen[neighbor] = struct{}{}
				queue = append(queue, neighbor)
			}
		}
		slices.Sort(component)
		components = append(components, component)
	}
	slices.SortFunc(components, func(a, b []int) int {
		return a[0] - b[0]
	})
	return components
}

func uniformMakespan(node *resolve.FetchTreeNode) int {
	durations := map[int]int{}
	for _, path := range enumeratePaths(node) {
		for id := range path {
			durations[id] = 1
		}
	}
	return weightedMakespan(node, durations)
}

func weightedMakespan(node *resolve.FetchTreeNode, durations map[int]int) int {
	max := 0
	for _, path := range enumeratePaths(node) {
		sum := 0
		for id := range path {
			sum += durations[id]
		}
		if sum > max {
			max = sum
		}
	}
	return max
}

func sequenceOf(children ...*resolve.FetchTreeNode) *resolve.FetchTreeNode {
	children = compactNodes(children)
	children = flattenKind(children, resolve.FetchTreeNodeKindSequence)
	if len(children) == 0 {
		return nil
	}
	if len(children) == 1 {
		return children[0]
	}
	return resolve.Sequence(children...)
}

func parallelOf(children ...*resolve.FetchTreeNode) *resolve.FetchTreeNode {
	children = compactNodes(children)
	children = flattenKind(children, resolve.FetchTreeNodeKindParallel)
	if len(children) == 0 {
		return nil
	}
	if len(children) == 1 {
		return children[0]
	}
	sortBranches(children)
	return resolve.Parallel(children...)
}

func flattenKind(nodes []*resolve.FetchTreeNode, kind resolve.FetchTreeNodeKind) []*resolve.FetchTreeNode {
	out := nodes[:0]
	for _, node := range nodes {
		if node != nil && node.Kind == kind {
			out = append(out, node.ChildNodes...)
			continue
		}
		out = append(out, node)
	}
	return out
}

func compactNodes(nodes []*resolve.FetchTreeNode) []*resolve.FetchTreeNode {
	out := nodes[:0]
	for _, node := range nodes {
		if node != nil {
			out = append(out, node)
		}
	}
	return out
}

func sortBranches(nodes []*resolve.FetchTreeNode) {
	slices.SortFunc(nodes, func(a, b *resolve.FetchTreeNode) int {
		return minReachableFetchID(a) - minReachableFetchID(b)
	})
}

func minReachableFetchID(node *resolve.FetchTreeNode) int {
	if node == nil {
		return 0
	}
	if node.Kind == resolve.FetchTreeNodeKindSingle {
		return node.Item.Fetch.Dependencies().FetchID
	}
	min := int(^uint(0) >> 1)
	for _, child := range node.ChildNodes {
		childMin := minReachableFetchID(child)
		if childMin < min {
			min = childMin
		}
	}
	return min
}

func sortedUnique(ids []int) []int {
	if len(ids) == 0 {
		return nil
	}
	out := append([]int{}, ids...)
	slices.Sort(out)
	return slices.Compact(out)
}

func sortedSet(set map[int]struct{}) []int {
	out := make([]int, 0, len(set))
	for id := range set {
		out = append(out, id)
	}
	slices.Sort(out)
	return out
}

func idSet(ids []int) map[int]struct{} {
	set := make(map[int]struct{}, len(ids))
	for _, id := range ids {
		set[id] = struct{}{}
	}
	return set
}

func clonePending(in map[int]map[int]struct{}) map[int]map[int]struct{} {
	out := make(map[int]map[int]struct{}, len(in))
	for id, pending := range in {
		out[id] = cloneSet(pending)
	}
	return out
}

func cloneSet(in map[int]struct{}) map[int]struct{} {
	out := make(map[int]struct{}, len(in))
	for id := range in {
		out[id] = struct{}{}
	}
	return out
}

func filterSet(in map[int]struct{}, allowed map[int]struct{}) map[int]struct{} {
	out := make(map[int]struct{}, len(in))
	for id := range in {
		if _, ok := allowed[id]; ok {
			out[id] = struct{}{}
		}
	}
	return out
}

func intersectSets(a, b map[int]struct{}) map[int]struct{} {
	out := make(map[int]struct{})
	for id := range a {
		if _, ok := b[id]; ok {
			out[id] = struct{}{}
		}
	}
	return out
}

func setContainsAll(set, subset map[int]struct{}) bool {
	for id := range subset {
		if _, ok := set[id]; !ok {
			return false
		}
	}
	return true
}

func removeIDs(ids []int, sets ...map[int]struct{}) []int {
	out := ids[:0]
	for _, id := range ids {
		remove := false
		for _, set := range sets {
			if _, ok := set[id]; ok {
				remove = true
				break
			}
		}
		if !remove {
			out = append(out, id)
		}
	}
	return out
}
