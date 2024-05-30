package plan

import (
	"fmt"

	"github.com/kingledion/go-tools/tree"
)

const treeRootID = ^uint(0)

type NodeSuggestion struct {
	TypeName                  string
	FieldName                 string
	DataSourceHash            DSHash
	Path                      string
	ParentPath                string
	IsRootNode                bool
	IsProvided                bool
	DisabledEntityResolver    bool // is true in case the node is an entity root node and has a key with disabled resolver
	IsEntityInterfaceTypeName bool

	parentPathWithoutFragment *string
	onFragment                bool
	Selected                  bool
	SelectionReasons          []string

	fieldRef int
}

func (n *NodeSuggestion) treeNodeID() uint {
	return TreeNodeID(n.fieldRef)
}

func (n *NodeSuggestion) appendSelectionReason(reason string, saveReason bool) {
	if !saveReason {
		return
	}
	n.SelectionReasons = append(n.SelectionReasons, reason)
	if n.IsProvided {
		n.SelectionReasons = append(n.SelectionReasons, ReasonProvidesProvidedByPlanner)
	}
}

func (n *NodeSuggestion) selectWithReason(reason string, saveReason bool) {
	if n.Selected {
		return
	}
	n.appendSelectionReason(reason, saveReason)
	n.Selected = true
}

func (n *NodeSuggestion) String() string {
	return fmt.Sprintf(`{"ds":%d,"path":"%s","typeName":"%s","fieldName":"%s","isRootNode":%t,"isProvided":%t, "entityResolver":%t, "isSelected": %v,"selectReason": %v}`,
		n.DataSourceHash, n.Path, n.TypeName, n.FieldName, n.IsRootNode, n.IsProvided, !n.DisabledEntityResolver, n.Selected, n.SelectionReasons)
}

type NodeSuggestionHint struct {
	fieldRef int
	dsHash   DSHash

	fieldName  string
	parentPath string
}

type NodeSuggestions struct {
	items           []*NodeSuggestion
	pathSuggestions map[string][]*NodeSuggestion
	seenFields      map[int]struct{}
	responseTree    tree.Tree[[]int]
}

func NewNodeSuggestions() *NodeSuggestions {
	return NewNodeSuggestionsWithSize(32)
}

func NewNodeSuggestionsWithSize(size int) *NodeSuggestions {
	responseTree := tree.Empty[[]int]()
	responseTree.Add(treeRootID, 0, nil)

	return &NodeSuggestions{
		items:           make([]*NodeSuggestion, 0, size),
		seenFields:      make(map[int]struct{}, size),
		pathSuggestions: make(map[string][]*NodeSuggestion),
		responseTree:    *responseTree,
	}
}

func (f *NodeSuggestions) AddItems(items ...*NodeSuggestion) {
	f.items = append(f.items, items...)
	f.populateHasSuggestions()
}

func (f *NodeSuggestions) IsFieldSeen(fieldRef int) bool {
	_, ok := f.seenFields[fieldRef]
	return ok
}

func (f *NodeSuggestions) AddSeenField(fieldRef int) {
	f.seenFields[fieldRef] = struct{}{}
}

func (f *NodeSuggestions) addSuggestion(node *NodeSuggestion) (suggestionIdx int) {
	f.items = append(f.items, node)
	return len(f.items) - 1
}

func (f *NodeSuggestions) SuggestionsForPath(typeName, fieldName, path string) (suggestions []*NodeSuggestion) {
	items, ok := f.pathSuggestions[path]
	if !ok {
		return nil
	}

	for i := range items {
		if typeName == items[i].TypeName && fieldName == items[i].FieldName {
			suggestions = append(suggestions, items[i])
		}
	}

	return suggestions
}

func (f *NodeSuggestions) HasSuggestionForPath(typeName, fieldName, path string) (dsHash DSHash, ok bool) {
	items, ok := f.pathSuggestions[path]
	if !ok {
		return 0, false
	}

	for i := range items {
		if typeName == items[i].TypeName && fieldName == items[i].FieldName && items[i].Selected {
			return items[i].DataSourceHash, true
		}
	}

	return 0, false
}

func (f *NodeSuggestions) isNodeUnique(idx int) bool {
	treeNode := f.treeNode(idx)

	return isTreeNodeUniq(treeNode)
}

func (f *NodeSuggestions) treeNode(idx int) treeNode {
	nodeID := f.items[idx].treeNodeID()
	treeNode, _ := f.responseTree.Find(nodeID)
	return treeNode
}

func (f *NodeSuggestions) isSelectedOnOtherSource(idx int) bool {
	treeNode := f.treeNode(idx)

	if isTreeNodeUniq(treeNode) {
		return false
	}

	duplicatesIndexes := treeNode.GetData()

	for _, duplicateIdx := range duplicatesIndexes {
		if idx == duplicateIdx {
			continue
		}
		if f.items[idx].DataSourceHash != f.items[duplicateIdx].DataSourceHash &&
			f.items[duplicateIdx].Selected {

			return true
		}
	}

	return false
}

func (f *NodeSuggestions) duplicatesOf(idx int) (out []int) {
	treeNode := f.treeNode(idx)

	if isTreeNodeUniq(treeNode) {
		return nil
	}

	duplicatesIndexes := treeNode.GetData()

	out = make([]int, 0, len(duplicatesIndexes))
	for _, duplicateIdx := range duplicatesIndexes {
		if idx == duplicateIdx {
			continue
		}
		out = append(out, duplicateIdx)
	}

	return
}

func (f *NodeSuggestions) childNodesOnSameSource(idx int) (out []int) {
	treeNode := f.treeNode(idx)
	childIndexes := treeNodeChildren(treeNode)

	out = make([]int, 0, len(childIndexes))

	for _, childIdx := range childIndexes {
		if f.items[childIdx].DataSourceHash != f.items[idx].DataSourceHash {
			continue
		}

		out = append(out, childIdx)
	}
	return
}

func (f *NodeSuggestions) siblingNodesOnSameSource(idx int) (out []int) {
	treeNode := f.treeNode(idx)
	siblingIndexes := treeNodeSiblings(treeNode)

	out = make([]int, 0, len(siblingIndexes))

	for _, siblingIndex := range siblingIndexes {
		if f.items[siblingIndex].DataSourceHash != f.items[idx].DataSourceHash {
			continue
		}

		out = append(out, siblingIndex)
	}
	return
}

func (f *NodeSuggestions) isLeaf(idx int) bool {
	treeNode := f.treeNode(idx)

	return isTreeNodeLeaf(treeNode)
}

func (f *NodeSuggestions) parentNodeOnSameSource(idx int) (parentIdx int, ok bool) {
	treeNode := f.treeNode(idx)
	parentNodeIndexes := treeNode.GetParent().GetData()

	for _, parentIdx := range parentNodeIndexes {
		if f.items[parentIdx].DataSourceHash == f.items[idx].DataSourceHash {
			return parentIdx, true
		}
	}

	return -1, false
}

func (f *NodeSuggestions) printNodes(msg string, filterNotSelected bool) {
	if msg != "" {
		fmt.Println(msg)
	}
	for i := range f.items {
		if filterNotSelected && !f.items[i].Selected {
			continue
		}
		fmt.Println(f.items[i].String())
	}
}

func (f *NodeSuggestions) populateHasSuggestions() map[DSHash]struct{} {
	unique := make(map[DSHash]struct{})
	f.pathSuggestions = make(map[string][]*NodeSuggestion, len(f.pathSuggestions))

	for i := range f.items {
		if !f.items[i].Selected {
			continue
		}

		unique[f.items[i].DataSourceHash] = struct{}{}
		f.pathSuggestions[f.items[i].Path] = append(f.pathSuggestions[f.items[i].Path], f.items[i])
	}

	return unique
}

type treeNode tree.Node[[]int]

func isTreeNodeUniq(node treeNode) bool {
	return len(node.GetData()) == 1
}

func isTreeNodeLeaf(node treeNode) bool {
	return len(node.GetChildren()) == 0
}

func treeNodeSiblings(node treeNode) []int {
	childrenOfParent := node.GetParent().GetChildren()

	if len(childrenOfParent) < 2 {
		return nil
	}

	out := make([]int, 0, len(childrenOfParent))

	for _, child := range childrenOfParent {
		if child.GetID() == node.GetID() {
			continue
		}

		out = append(out, child.GetData()...)
	}

	return out
}

func treeNodeChildren(node treeNode) []int {
	children := node.GetChildren()

	if len(children) == 0 {
		return nil
	}

	out := make([]int, 0, len(children))

	for _, child := range children {
		out = append(out, child.GetData()...)
	}

	return out
}
