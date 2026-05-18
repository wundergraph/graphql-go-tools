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
