package resolve

import "sync/atomic"

// DeferExecutionStatus is the request-local execution state of a deferred
// fragment. The zero value is omitted from planned/static trace trees.
type DeferExecutionStatus string

const (
	DeferExecutionStatusPlanned   DeferExecutionStatus = "planned"
	DeferExecutionStatusRunning   DeferExecutionStatus = "running"
	DeferExecutionStatusCompleted DeferExecutionStatus = "completed"
	DeferExecutionStatusError     DeferExecutionStatus = "error"
	DeferExecutionStatusSkipped   DeferExecutionStatus = "skipped"
)

// FetchTreeDeferDescriptor identifies the @defer fragment represented by a
// synthetic sequence wrapper in a composite execution tree.
type FetchTreeDeferDescriptor struct {
	ID    int      `json:"id"`
	Label string   `json:"label"`
	Path  []string `json:"path"`
}

type fetchTreeDeferMetadata struct {
	descriptor FetchTreeDeferDescriptor
	status     atomic.Uint32
}

type deferExecutionState uint32

const (
	deferExecutionStateNone deferExecutionState = iota
	deferExecutionStatePlanned
	deferExecutionStateRunning
	deferExecutionStateCompleted
	deferExecutionStateError
	deferExecutionStateSkipped
)

func (m *fetchTreeDeferMetadata) executionStatus() DeferExecutionStatus {
	if m == nil {
		return ""
	}
	switch deferExecutionState(m.status.Load()) {
	case deferExecutionStatePlanned:
		return DeferExecutionStatusPlanned
	case deferExecutionStateRunning:
		return DeferExecutionStatusRunning
	case deferExecutionStateCompleted:
		return DeferExecutionStatusCompleted
	case deferExecutionStateError:
		return DeferExecutionStatusError
	case deferExecutionStateSkipped:
		return DeferExecutionStatusSkipped
	default:
		return ""
	}
}

type deferTreeConverter struct {
	descriptors  map[int]DeferDescriptor
	requestLocal bool
	wrappers     map[int]*FetchTreeNode
	children     map[int][]int
}

// deferTreeToFetchTree converts defer scheduling nodes into ordinary fetch-tree
// nodes. A defer group is represented by a one-child sequence so its descriptor
// can be rendered without adding a new fetch-tree node kind. Group fetch trees
// stay shared with the planned response; only structural wrappers are allocated.
func deferTreeToFetchTree(node *DeferTreeNode, descriptors map[int]DeferDescriptor) *FetchTreeNode {
	converter := deferTreeConverter{descriptors: descriptors}
	converted, _ := converter.convert(node, 0)
	return converted
}

func (c *deferTreeConverter) convert(node *DeferTreeNode, parentID int) (*FetchTreeNode, []int) {
	if node == nil {
		return nil, nil
	}

	switch node.Kind {
	case DeferTreeNodeKindSingle:
		if node.Item == nil || node.Item.Fetches == nil {
			return nil, nil
		}
		descriptor := c.descriptors[node.Item.DeferID]
		descriptor.ID = node.Item.DeferID
		path := append([]string{}, descriptor.Path...)
		wrapper := Sequence(node.Item.Fetches)
		wrapper.deferMetadata = &fetchTreeDeferMetadata{descriptor: FetchTreeDeferDescriptor{
			ID:    descriptor.ID,
			Label: descriptor.Label,
			Path:  path,
		}}
		if c.requestLocal {
			wrapper.deferMetadata.status.Store(uint32(deferExecutionStatePlanned))
			c.wrappers[descriptor.ID] = wrapper
			if parentID != 0 {
				c.children[parentID] = append(c.children[parentID], descriptor.ID)
			}
		}
		return wrapper, []int{descriptor.ID}

	case DeferTreeNodeKindParallel:
		children := make([]*FetchTreeNode, 0, len(node.ChildNodes))
		var rootIDs []int
		for _, child := range node.ChildNodes {
			converted, childRoots := c.convert(child, parentID)
			if converted != nil {
				children = append(children, converted)
			}
			rootIDs = append(rootIDs, childRoots...)
		}
		if len(children) == 0 {
			return nil, nil
		}
		return Parallel(children...), rootIDs

	case DeferTreeNodeKindSequence:
		children := make([]*FetchTreeNode, 0, len(node.ChildNodes))
		var rootIDs []int
		semanticParent := parentID
		for _, child := range node.ChildNodes {
			converted, childRoots := c.convert(child, semanticParent)
			if converted == nil {
				continue
			}
			children = append(children, converted)
			if len(rootIDs) == 0 {
				rootIDs = append(rootIDs, childRoots...)
				if len(childRoots) != 0 {
					semanticParent = childRoots[0]
				}
			}
		}
		if len(children) == 0 {
			return nil, nil
		}
		return Sequence(children...), rootIDs

	default:
		return nil, nil
	}
}

func compositeExecutionTree(primary, deferred *FetchTreeNode) *FetchTreeNode {
	if deferred == nil {
		return primary
	}
	if primary == nil {
		return deferred
	}
	root := Sequence(primary, deferred)
	root.NormalizedQuery = primary.NormalizedQuery
	return root
}

// PlannedExecutionTree returns the canonical query-plan tree for a deferred
// response. Existing primary and deferred fetch nodes are preserved by pointer;
// only the composite and defer descriptor wrappers are new.
func (r *GraphQLDeferResponse) PlannedExecutionTree() *FetchTreeNode {
	if r == nil {
		return nil
	}

	var primary *FetchTreeNode
	if r.Response != nil {
		primary = r.Response.Fetches
	}
	deferred := deferTreeToFetchTree(r.DeferTree, r.DeferDescriptors)
	return compositeExecutionTree(primary, deferred)
}

// DeferExecutionTraceTree is a request-local view of a deferred response's
// execution topology. Root shares the original fetch nodes so their load traces
// remain visible, while every defer wrapper and status value is newly allocated.
type DeferExecutionTraceTree struct {
	Root *FetchTreeNode

	wrappers map[int]*FetchTreeNode
	children map[int][]int
}

// NewDeferExecutionTraceTree creates an isolated runtime-status tree for one
// request. It never mutates the cached GraphQLDeferResponse or its DeferTree.
func (r *GraphQLDeferResponse) NewDeferExecutionTraceTree() *DeferExecutionTraceTree {
	tree := &DeferExecutionTraceTree{
		wrappers: make(map[int]*FetchTreeNode),
		children: make(map[int][]int),
	}
	if r == nil {
		return tree
	}

	converter := deferTreeConverter{
		descriptors:  r.DeferDescriptors,
		requestLocal: true,
		wrappers:     tree.wrappers,
		children:     tree.children,
	}
	deferred, _ := converter.convert(r.DeferTree, 0)
	var primary *FetchTreeNode
	if r.Response != nil {
		primary = r.Response.Fetches
	}
	tree.Root = compositeExecutionTree(primary, deferred)
	return tree
}

// Status returns the current request-local status for id.
func (t *DeferExecutionTraceTree) Status(id int) (DeferExecutionStatus, bool) {
	metadata, ok := t.metadata(id)
	if !ok {
		return "", false
	}
	return metadata.executionStatus(), true
}

// MarkRunning transitions a planned defer wrapper to running.
func (t *DeferExecutionTraceTree) MarkRunning(id int) bool {
	return t.transition(id, deferExecutionStatePlanned, deferExecutionStateRunning)
}

// MarkCompleted transitions a running defer wrapper to completed.
func (t *DeferExecutionTraceTree) MarkCompleted(id int) bool {
	return t.transition(id, deferExecutionStateRunning, deferExecutionStateCompleted)
}

// MarkError transitions a running defer wrapper to error.
func (t *DeferExecutionTraceTree) MarkError(id int) bool {
	return t.transition(id, deferExecutionStateRunning, deferExecutionStateError)
}

// MarkSkipped marks a planned wrapper and all of its semantically nested defer
// descendants as skipped. It returns false when id is missing or already left
// the planned state.
func (t *DeferExecutionTraceTree) MarkSkipped(id int) bool {
	if !t.transition(id, deferExecutionStatePlanned, deferExecutionStateSkipped) {
		return false
	}
	t.MarkDescendantsSkipped(id)
	return true
}

// MarkDescendantsSkipped marks every planned semantic child subtree of id as
// skipped without changing the status of id itself. This is used when an
// already-running parent errors or completes without releasing all children.
func (t *DeferExecutionTraceTree) MarkDescendantsSkipped(id int) bool {
	if t == nil {
		return false
	}
	if _, ok := t.wrappers[id]; !ok {
		return false
	}
	marked := false
	for _, childID := range t.children[id] {
		if t.MarkSkipped(childID) {
			marked = true
		}
	}
	return marked
}

// AllTerminal reports whether every request-local defer wrapper has reached a
// completed, error, or skipped state.
func (t *DeferExecutionTraceTree) AllTerminal() bool {
	if t == nil {
		return true
	}
	for id := range t.wrappers {
		status, _ := t.Status(id)
		switch status {
		case DeferExecutionStatusCompleted, DeferExecutionStatusError, DeferExecutionStatusSkipped:
		default:
			return false
		}
	}
	return true
}

// PruneDeadDefers applies liveness pruning without mutating the planned defer
// tree and records every rejected defer subtree as skipped in this request.
func (t *DeferExecutionTraceTree) PruneDeadDefers(node *DeferTreeNode, live map[int]DeferDescriptor) *DeferTreeNode {
	return pruneDeadDefersWithObserver(node, live, func(id int) {
		t.MarkSkipped(id)
	})
}

func (t *DeferExecutionTraceTree) metadata(id int) (*fetchTreeDeferMetadata, bool) {
	if t == nil {
		return nil, false
	}
	wrapper, ok := t.wrappers[id]
	if !ok || wrapper == nil || wrapper.deferMetadata == nil {
		return nil, false
	}
	return wrapper.deferMetadata, true
}

func (t *DeferExecutionTraceTree) transition(id int, from, to deferExecutionState) bool {
	metadata, ok := t.metadata(id)
	if !ok {
		return false
	}
	return metadata.status.CompareAndSwap(uint32(from), uint32(to))
}
