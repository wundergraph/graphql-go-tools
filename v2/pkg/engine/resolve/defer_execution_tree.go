package resolve

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
}

// deferTreeToFetchTree converts defer scheduling nodes into ordinary fetch-tree
// nodes. A defer group is represented by a one-child sequence so its descriptor
// can be rendered without adding a new fetch-tree node kind. Group fetch trees
// stay shared with the planned response; only structural wrappers are allocated.
func deferTreeToFetchTree(node *DeferTreeNode, descriptors map[int]DeferDescriptor) *FetchTreeNode {
	if node == nil {
		return nil
	}

	switch node.Kind {
	case DeferTreeNodeKindSingle:
		if node.Item == nil || node.Item.Fetches == nil {
			return nil
		}
		descriptor := descriptors[node.Item.DeferID]
		descriptor.ID = node.Item.DeferID
		path := append([]string{}, descriptor.Path...)
		wrapper := Sequence(node.Item.Fetches)
		wrapper.deferMetadata = &fetchTreeDeferMetadata{descriptor: FetchTreeDeferDescriptor{
			ID:    descriptor.ID,
			Label: descriptor.Label,
			Path:  path,
		}}
		return wrapper

	case DeferTreeNodeKindSequence, DeferTreeNodeKindParallel:
		children := make([]*FetchTreeNode, 0, len(node.ChildNodes))
		for _, child := range node.ChildNodes {
			if converted := deferTreeToFetchTree(child, descriptors); converted != nil {
				children = append(children, converted)
			}
		}
		if len(children) == 0 {
			return nil
		}
		if node.Kind == DeferTreeNodeKindParallel {
			return Parallel(children...)
		}
		return Sequence(children...)
	default:
		return nil
	}
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
