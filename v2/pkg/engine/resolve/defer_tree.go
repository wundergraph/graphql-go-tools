// v2/pkg/engine/resolve/defer_tree.go
package resolve

type DeferTreeNodeKind int

const (
	DeferTreeNodeKindSingle DeferTreeNodeKind = iota
	DeferTreeNodeKindSequence
	DeferTreeNodeKindParallel
)

type DeferTreeNode struct {
	Kind       DeferTreeNodeKind
	Item       *DeferFetchGroup
	ChildNodes []*DeferTreeNode
}

func DeferSingle(group *DeferFetchGroup) *DeferTreeNode {
	return &DeferTreeNode{Kind: DeferTreeNodeKindSingle, Item: group}
}

func DeferSequence(children ...*DeferTreeNode) *DeferTreeNode {
	return &DeferTreeNode{Kind: DeferTreeNodeKindSequence, ChildNodes: children}
}

func DeferParallel(children ...*DeferTreeNode) *DeferTreeNode {
	return &DeferTreeNode{Kind: DeferTreeNodeKindParallel, ChildNodes: children}
}

// topDeferID returns the top-level (root) defer id of a Single or Sequence
// subtree — for a Sequence that is its first child (the parent, which runs
// first). It does NOT apply to a Parallel node: its children are independent
// top-level defers with no single root, so this returns (0, false) for Parallel
// and callers must prune Parallel children individually instead.
func topDeferID(node *DeferTreeNode) (int, bool) {
	if node == nil {
		return 0, false
	}
	switch node.Kind {
	case DeferTreeNodeKindSingle:
		if node.Item != nil {
			return node.Item.DeferID, true
		}
		return 0, false
	case DeferTreeNodeKindSequence:
		if len(node.ChildNodes) == 0 {
			return 0, false
		}
		return topDeferID(node.ChildNodes[0])
	default:
		// Parallel (or unknown kind): no single root defer id.
		return 0, false
	}
}

// pruneDeadDefers drops top-level defer subtrees whose root defer's anchor did
// not survive the initial render (liveTop holds the surviving top-level ids).
// A pruned subtree is removed whole — a dead parent takes its nested children
// with it. Returns nil when nothing survives.
func pruneDeadDefers(node *DeferTreeNode, liveTop map[int]struct{}) *DeferTreeNode {
	if node == nil {
		return nil
	}
	switch node.Kind {
	case DeferTreeNodeKindParallel:
		kept := make([]*DeferTreeNode, 0, len(node.ChildNodes))
		for _, child := range node.ChildNodes {
			if pruned := pruneDeadDefers(child, liveTop); pruned != nil {
				kept = append(kept, pruned)
			}
		}
		if len(kept) == 0 {
			return nil
		}
		return &DeferTreeNode{Kind: DeferTreeNodeKindParallel, ChildNodes: kept}
	default:
		// Single or Sequence: keyed on the top-level defer id of the subtree.
		id, ok := topDeferID(node)
		if !ok {
			return nil
		}
		if _, live := liveTop[id]; live {
			return node
		}
		return nil
	}
}
