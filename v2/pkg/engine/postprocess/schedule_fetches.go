package postprocess

import (
	"cmp"
	"fmt"
	"math"
	"slices"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

// scheduleFetches is a dependency-aware scheduler that emits nested Sequence/Parallel trees,
// collapsing independent chains onto their own branches instead of synchronizing them at wave
// barriers.
//
// This scheduler tries to break the top level DAG into weakly connected components.
// For each component it picks between two strategies of scheduling:
//   - the waves tree, that barriers all ready roots per step,
//   - the inlined tree, that pulls each root's exclusively-reachable descendants into that root's branch.
//
// The inlined tree wins only when a per-fetch predecessor-containment proof shows it can
// never be slower than the component's waves tree, no matter how long each fetch takes.
// Since components never wait on each other, mixing winners is always safe.
//
// On any scheduler or validator error the processor falls back to the legacy wave pipeline.
type scheduleFetches struct {
	disable bool
}

func (b *scheduleFetches) ProcessFetchTree(root *resolve.FetchTreeNode) {
	if b.disable || root == nil || root.Kind != resolve.FetchTreeNodeKindSequence {
		return
	}
	if err := b.buildSchedule(root); err != nil {
		(&orderSequenceByDependencies{}).ProcessFetchTree(root)
		(&createParallelNodes{}).ProcessFetchTree(root)
	}
}

func (b *scheduleFetches) buildSchedule(root *resolve.FetchTreeNode) error {
	dag, err := newFetchDAG(root.ChildNodes)
	if err != nil {
		return err
	}
	tree, err := buildScheduleTree(root.ChildNodes, dag)
	if err != nil {
		return err
	}
	if tree == nil {
		root.ChildNodes = nil
		return nil
	}
	// Replace only the scheduling-related fields: the root may carry a
	// subscription Trigger or NormalizedQuery that must survive rescheduling.
	root.Kind = tree.Kind
	root.Item = tree.Item
	root.ChildNodes = tree.ChildNodes
	return nil
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
			return nil, fmt.Errorf("non-single node in flat fetch sequence")
		}
		id := node.Item.Fetch.Dependencies().FetchID
		if _, exists := dag.nodes[id]; exists {
			return nil, fmt.Errorf("duplicate fetch id %d", id)
		}
		dag.nodes[id] = node
		dag.parents[id] = map[int]struct{}{}
		dag.children[id] = map[int]struct{}{}
	}
	for id, node := range dag.nodes {
		for _, dep := range node.Item.Fetch.Dependencies().DependsOnFetchIDs {
			if _, exists := dag.nodes[dep]; !exists {
				continue // dependency satisfied outside this tree
			}
			if id == dep {
				return nil, fmt.Errorf("self-dependent id %d", id)
			}
			dag.parents[id][dep] = struct{}{}
			dag.children[dep][id] = struct{}{}
		}
	}
	return dag, nil
}

func buildScheduleTree(roots []*resolve.FetchTreeNode, dag *fetchDAG) (*resolve.FetchTreeNode, error) {
	ids := make([]int, 0, len(roots))
	for _, root := range roots {
		if root == nil || root.Item == nil || root.Item.Fetch == nil {
			continue
		}
		ids = append(ids, root.Item.Fetch.Dependencies().FetchID)
	}
	// Pick the best strategy on the top level for weakly connected trees.
	components := weaklyConnectedComponents(sortedUnique(ids), dag)
	winners := make([]*resolve.FetchTreeNode, 0, len(components))
	for _, component := range components {
		waves, err := schedule(component, dag, false)
		if err != nil {
			return nil, err
		}
		inlined, err := schedule(component, dag, true)
		if err != nil {
			return nil, err
		}
		if dominates(inlined, waves) {
			winners = append(winners, inlined)
		} else {
			winners = append(winners, waves)
		}
	}
	winner := parallelOf(winners)
	if err := validateSchedule(winner, dag); err != nil {
		return nil, err
	}
	return winner, nil
}

// schedule builds a fetch tree for the DAG restricted to set.
// Dependencies on fetches outside set are treated as already satisfied.
// Weakly connected components run in Parallel;
// within a component the ready roots run in Parallel followed by the remainder in Sequence.
// When inlined, each root pulls the descendants reachable only through it into its branch.
func schedule(set []int, dag *fetchDAG, inline bool) (*resolve.FetchTreeNode, error) {
	sortedSet := sortedUnique(set)
	switch len(sortedSet) {
	case 0:
		return nil, nil
	case 1:
		return dag.nodes[sortedSet[0]], nil
	}
	components := weaklyConnectedComponents(sortedSet, dag)
	if len(components) > 1 {
		branches := make([]*resolve.FetchTreeNode, 0, len(components))
		for _, component := range components {
			child, err := schedule(component, dag, inline)
			if err != nil {
				return nil, err
			}
			branches = append(branches, child)
		}
		return parallelOf(branches), nil
	}
	// Find the ready fetches:
	// Roots have no parents in inSet; their dependencies outside inSet complete
	// before this subtree starts, so they are ready to run as the first wave.
	inSet := asMap(sortedSet)
	roots := make([]int, 0, len(sortedSet))
	for _, id := range sortedSet {
		if !hasParentIn(dag, id, inSet) {
			roots = append(roots, id)
		}
	}
	if len(roots) == 0 {
		return nil, fmt.Errorf("cycle detected in fetch dependency graph")
	}
	scheduled := asMap(roots)
	branches := make([]*resolve.FetchTreeNode, 0, len(roots))
	for _, root := range roots {
		branch := dag.nodes[root]
		if inline {
			exclusive := exclusiveDescendants(root, dag, inSet, scheduled)
			if len(exclusive) > 0 {
				subtree, err := schedule(exclusive, dag, inline)
				if err != nil {
					return nil, err
				}
				branch = sequenceOf([]*resolve.FetchTreeNode{branch, subtree})
			}
		}
		branches = append(branches, branch)
	}
	rest := make([]int, 0, len(sortedSet)-len(scheduled))
	for _, id := range sortedSet {
		if _, ok := scheduled[id]; !ok {
			rest = append(rest, id)
		}
	}
	restTree, err := schedule(rest, dag, inline)
	if err != nil {
		return nil, err
	}
	return sequenceOf([]*resolve.FetchTreeNode{parallelOf(branches), restTree}), nil
}

func hasParentIn(dag *fetchDAG, id int, set map[int]struct{}) bool {
	for parent := range dag.parents[id] {
		if _, ok := set[parent]; ok {
			return true
		}
	}
	return false
}

// exclusiveDescendants returns the descendants within inSet that are reachable only through root:
// every member's parent in inSet is root or another member.
// Such fetches cannot start before root finishes, inlining them into root's branch never delays
// them and frees them from waiting on sibling roots.
// Fetches returned are also recorded in scheduled.
func exclusiveDescendants(root int, dag *fetchDAG, inSet, scheduled map[int]struct{}) []int {
	member := map[int]struct{}{root: {}}
	out := make([]int, 0, len(dag.children[root]))
	queue := asSlice(dag.children[root])
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		if _, ok := inSet[id]; !ok {
			continue
		}
		if _, ok := member[id]; ok {
			continue
		}
		if _, ok := scheduled[id]; ok {
			continue
		}
		exclusive := true
		for parent := range dag.parents[id] {
			if _, in := inSet[parent]; !in {
				continue // satisfied by an enclosing recursion step
			}
			if _, ok := member[parent]; !ok {
				exclusive = false
				break
			}
		}
		if !exclusive {
			continue // revisited via the queue if its parents join later
		}
		member[id] = struct{}{}
		scheduled[id] = struct{}{}
		out = append(out, id)
		queue = append(queue, asSlice(dag.children[id])...)
	}
	return out
}

// dominates checks whether tree a is never slower than tree b, no matter how long each fetch takes.
//
// The test: every fetch's predecessor set in a must be a subset of its predecessor set in b.
// The fetches on any critical path of a are pairwise ordered in a, hence also in b, so they form
// a chain in b and b's makespan is at least that chain's weight — a's makespan.
//
// The test is also exact, not just sufficient: given any extra predecessor pair in a,
// there are fetch durations for which a is strictly slower.
func dominates(a, b *resolve.FetchTreeNode) bool {
	predA, predB := treePredecessors(a), treePredecessors(b)
	if len(predA) != len(predB) {
		return false
	}
	for id, pa := range predA {
		pb, exists := predB[id]
		if !exists {
			return false
		}
		for waitID := range pa {
			if _, ok := pb[waitID]; !ok {
				return false
			}
		}
	}
	return true
}

// treePredecessors returns, per fetch, the set of fetches the tree guarantees
// to have completed before that fetch starts.
func treePredecessors(root *resolve.FetchTreeNode) map[int]map[int]struct{} {
	preds := map[int]map[int]struct{}{}
	var walk func(node *resolve.FetchTreeNode, before map[int]struct{}) []int
	walk = func(node *resolve.FetchTreeNode, before map[int]struct{}) []int {
		if node == nil {
			return nil
		}
		switch node.Kind {
		case resolve.FetchTreeNodeKindSingle:
			id := node.Item.Fetch.Dependencies().FetchID
			set := make(map[int]struct{}, len(before))
			for k := range before {
				set[k] = struct{}{}
			}
			preds[id] = set
			return []int{id}
		case resolve.FetchTreeNodeKindParallel:
			var ids []int
			for _, child := range node.ChildNodes {
				ids = append(ids, walk(child, before)...)
			}
			return ids
		case resolve.FetchTreeNodeKindSequence:
			acc := make(map[int]struct{}, len(before))
			for k := range before {
				acc[k] = struct{}{}
			}
			var ids []int
			for _, child := range node.ChildNodes {
				childIDs := walk(child, acc)
				for _, id := range childIDs {
					acc[id] = struct{}{}
				}
				ids = append(ids, childIDs...)
			}
			return ids
		default:
			return nil
		}
	}
	walk(root, map[int]struct{}{})
	return preds
}

// validateSchedule walks the tree once, checking that every fetch's declared
// dependencies are sequenced strictly before it, and that the tree contains
// every DAG fetch exactly once (a schedule must never lose or duplicate work).
func validateSchedule(root *resolve.FetchTreeNode, dag *fetchDAG) error {
	seen := make(map[int]int, len(dag.nodes))
	var walk func(node *resolve.FetchTreeNode, before map[int]struct{}) ([]int, error)
	walk = func(node *resolve.FetchTreeNode, before map[int]struct{}) ([]int, error) {
		if node == nil {
			return nil, nil
		}
		switch node.Kind {
		case resolve.FetchTreeNodeKindSingle:
			id := node.Item.Fetch.Dependencies().FetchID
			if _, ok := dag.nodes[id]; !ok {
				return nil, fmt.Errorf("fetch %d not found in dag", id)
			}
			seen[id]++
			for _, dep := range node.Item.Fetch.Dependencies().DependsOnFetchIDs {
				if _, known := dag.nodes[dep]; !known {
					continue
				}
				if _, ok := before[dep]; !ok {
					return nil, fmt.Errorf("fetch %d is scheduled before its dependency %d completes", id, dep)
				}
			}
			return []int{id}, nil
		case resolve.FetchTreeNodeKindParallel:
			var ids []int
			for _, child := range node.ChildNodes {
				childIDs, err := walk(child, before)
				if err != nil {
					return nil, err
				}
				ids = append(ids, childIDs...)
			}
			return ids, nil
		case resolve.FetchTreeNodeKindSequence:
			available := make(map[int]struct{}, len(before))
			for k := range before {
				available[k] = struct{}{}
			}
			var ids []int
			for _, child := range node.ChildNodes {
				childIDs, err := walk(child, available)
				if err != nil {
					return nil, err
				}
				for _, id := range childIDs {
					available[id] = struct{}{}
				}
				ids = append(ids, childIDs...)
			}
			return ids, nil
		default:
			return nil, fmt.Errorf("unexpected node kind %q in schedule", node.Kind)
		}
	}
	if _, err := walk(root, map[int]struct{}{}); err != nil {
		return err
	}
	for id, count := range seen {
		if count > 1 {
			return fmt.Errorf("fetch %d scheduled %d times", id, count)
		}
	}
	for id := range dag.nodes {
		if seen[id] == 0 {
			return fmt.Errorf("fetch %d missing from schedule", id)
		}
	}
	return nil
}

// weaklyConnectedComponents returns a slice of weakly connected components containing
// nodes which is an ordered set.
func weaklyConnectedComponents(nodes []int, dag *fetchDAG) [][]int {
	allowed := asMap(nodes)
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
			for _, neighbors := range []map[int]struct{}{dag.parents[current], dag.children[current]} {
				for neighbor := range neighbors {
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
		}
		slices.Sort(component)
		components = append(components, component)
	}
	return components
}

// sequenceOf and parallelOf normalize child lists into fresh slices:
// nils dropped, same-kind children spliced inline, singleton unwrapped.
func sequenceOf(children []*resolve.FetchTreeNode) *resolve.FetchTreeNode {
	return combineOf(resolve.FetchTreeNodeKindSequence, children)
}

func parallelOf(children []*resolve.FetchTreeNode) *resolve.FetchTreeNode {
	return combineOf(resolve.FetchTreeNodeKindParallel, children)
}

func combineOf(kind resolve.FetchTreeNodeKind, children []*resolve.FetchTreeNode) *resolve.FetchTreeNode {
	out := make([]*resolve.FetchTreeNode, 0, len(children))
	for _, child := range children {
		switch {
		case child == nil:
		case child.Kind == kind:
			out = append(out, child.ChildNodes...)
		default:
			out = append(out, child)
		}
	}
	switch len(out) {
	case 0:
		return nil
	case 1:
		return out[0]
	}
	if kind == resolve.FetchTreeNodeKindParallel {
		slices.SortFunc(out, func(a, b *resolve.FetchTreeNode) int {
			return cmp.Compare(minReachableFetchID(a), minReachableFetchID(b))
		})
		return resolve.Parallel(out...)
	}
	return resolve.Sequence(out...)
}

func minReachableFetchID(node *resolve.FetchTreeNode) int {
	minID := math.MaxInt
	if node == nil {
		return minID
	}
	if node.Kind == resolve.FetchTreeNodeKindSingle {
		return node.Item.Fetch.Dependencies().FetchID
	}
	for _, child := range node.ChildNodes {
		if childMin := minReachableFetchID(child); childMin < minID {
			minID = childMin
		}
	}
	return minID
}

func sortedUnique(ids []int) []int {
	if len(ids) == 0 {
		return nil
	}
	out := append([]int{}, ids...)
	slices.Sort(out)
	return slices.Compact(out)
}

func asSlice(set map[int]struct{}) []int {
	out := make([]int, 0, len(set))
	for id := range set {
		out = append(out, id)
	}
	return out
}

func asMap(ids []int) map[int]struct{} {
	set := make(map[int]struct{}, len(ids))
	for _, id := range ids {
		set[id] = struct{}{}
	}
	return set
}
