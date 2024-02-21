package plan

import (
	"fmt"

	"github.com/kingledion/go-tools/tree"
)

const treeRootID = ^uint(0)

type NodeSuggestion struct {
	TypeName       string
	FieldName      string
	DataSourceHash DSHash
	Path           string
	ParentPath     string
	IsRootNode     bool
	LessPreferable bool // is true in case the node is an entity root node and has a key with disabled resolver

	parentPathWithoutFragment *string
	onFragment                bool
	Selected                  bool
	SelectionReasons          []string

	fieldRef int
}

func (n *NodeSuggestion) appendSelectionReason(reason string) {
	n.SelectionReasons = append(n.SelectionReasons, reason)
}

func (n *NodeSuggestion) selectWithReason(reason string, saveReason bool) {
	if n.Selected {
		return
	}
	if saveReason {
		n.appendSelectionReason(reason)
	}
	n.Selected = true
}

func (n *NodeSuggestion) String() string {
	return fmt.Sprintf(`{"ds":%d,"path":"%s","typeName":"%s","fieldName":"%s","isRootNode":%t, "isSelected": %v,"select reason": %v}`,
		n.DataSourceHash, n.Path, n.TypeName, n.FieldName, n.IsRootNode, n.Selected, n.SelectionReasons)
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

func (f *NodeSuggestions) addSuggestion(node *NodeSuggestion) {
	f.items = append(f.items, node)
}

func (f *NodeSuggestions) SuggestionsForPath(typeName, fieldName, path string) (dsHashes []DSHash) {
	items, ok := f.pathSuggestions[path]
	if !ok {
		return nil
	}

	for i := range items {
		if typeName == items[i].TypeName && fieldName == items[i].FieldName {
			dsHashes = append(dsHashes, items[i].DataSourceHash)
		}
	}

	return dsHashes
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

func (f *NodeSuggestions) isNodeUniq(idx int) bool {
	for i := range f.items {
		if i == idx {
			continue
		}
		if f.items[idx].TypeName == f.items[i].TypeName && f.items[idx].FieldName == f.items[i].FieldName && f.items[idx].Path == f.items[i].Path {
			return false
		}
	}
	return true
}

func (f *NodeSuggestions) isSelectedOnOtherSource(idx int) bool {
	for i := range f.items {
		if i == idx {
			continue
		}
		if f.items[idx].TypeName == f.items[i].TypeName &&
			f.items[idx].FieldName == f.items[i].FieldName &&
			f.items[idx].Path == f.items[i].Path &&
			f.items[idx].DataSourceHash != f.items[i].DataSourceHash &&
			f.items[i].Selected {

			return true
		}
	}
	return false
}

func (f *NodeSuggestions) duplicatesOf(idx int) (out []int) {
	for i := range f.items {
		if i == idx {
			continue
		}
		if f.items[idx].TypeName == f.items[i].TypeName &&
			f.items[idx].FieldName == f.items[i].FieldName &&
			f.items[idx].Path == f.items[i].Path {
			out = append(out, i)
		}
	}
	return
}

func (f *NodeSuggestions) childNodesOnSameSource(idx int) (out []int) {
	for i := range f.items {
		if i == idx {
			continue
		}
		if f.items[i].DataSourceHash != f.items[idx].DataSourceHash {
			continue
		}

		if f.items[i].ParentPath == f.items[idx].Path || (f.items[i].parentPathWithoutFragment != nil && *f.items[i].parentPathWithoutFragment == f.items[idx].Path) {
			out = append(out, i)
		}
	}
	return
}

func (f *NodeSuggestions) siblingNodesOnSameSource(idx int) (out []int) {
	for i := range f.items {
		if i == idx {
			continue
		}
		if f.items[i].DataSourceHash != f.items[idx].DataSourceHash {
			continue
		}

		hasMatch := false
		switch {
		case f.items[i].parentPathWithoutFragment != nil && f.items[idx].parentPathWithoutFragment != nil:
			hasMatch = *f.items[i].parentPathWithoutFragment == *f.items[idx].parentPathWithoutFragment
		case f.items[i].parentPathWithoutFragment != nil && f.items[idx].parentPathWithoutFragment == nil:
			hasMatch = *f.items[i].parentPathWithoutFragment == f.items[idx].ParentPath
		case f.items[i].parentPathWithoutFragment == nil && f.items[idx].parentPathWithoutFragment != nil:
			hasMatch = f.items[i].ParentPath == *f.items[idx].parentPathWithoutFragment
		default:
			hasMatch = f.items[i].ParentPath == f.items[idx].ParentPath
		}

		if hasMatch {
			out = append(out, i)
		}
	}
	return
}

func (f *NodeSuggestions) isLeaf(idx int) bool {
	for i := range f.items {
		if i == idx {
			continue
		}
		if f.items[i].ParentPath == f.items[idx].Path {
			return false
		}
	}
	return true
}

func (f *NodeSuggestions) parentNodeOnSameSource(idx int) (parentIdx int, ok bool) {
	for i := range f.items {
		if i == idx {
			continue
		}
		if f.items[i].DataSourceHash != f.items[idx].DataSourceHash {
			continue
		}

		if f.items[i].Path == f.items[idx].ParentPath || (f.items[idx].parentPathWithoutFragment != nil && f.items[i].Path == *f.items[idx].parentPathWithoutFragment) {
			return i, true
		}
	}
	return -1, false
}

func (f *NodeSuggestions) uniqueDataSourceHashes() map[DSHash]struct{} {
	if len(f.items) == 0 {
		return nil
	}

	unique := make(map[DSHash]struct{})
	for i := range f.items {
		unique[f.items[i].DataSourceHash] = struct{}{}
	}

	return unique
}

func (f *NodeSuggestions) printNodes(msg string) {
	if msg != "" {
		fmt.Println(msg)
	}
	for i := range f.items {
		fmt.Println(f.items[i].String())
	}
}

func (f *NodeSuggestions) populateHasSuggestions() {
	for i := range f.items {
		if !f.items[i].Selected {
			continue
		}

		suggestions, _ := f.pathSuggestions[f.items[i].Path]
		suggestions = append(f.pathSuggestions[f.items[i].Path], f.items[i])
		f.pathSuggestions[f.items[i].Path] = suggestions
	}
}

type treeNode tree.Node[[]int]

type nodeCheckCallback func(node treeNode)

func (f *NodeSuggestions) AnyNodeItemIsSelected(node treeNode) bool {
	for _, itemId := range node.GetData() {
		if f.items[itemId].Selected {
			return true
		}
	}

	return false
}

func (f *NodeSuggestions) isTreeNodeUniq(node treeNode) bool {
	return len(node.GetData()) == 1
}

func (f *NodeSuggestions) isTreeNodeLeaf(node treeNode) bool {
	return len(node.GetChildren()) == 0
}

func (f *NodeSuggestions) ForEachSibling(node treeNode, callback nodeCheckCallback) {
	childrenOfParent := node.GetParent().GetChildren()

	if len(childrenOfParent) < 2 {
		return
	}

	for _, child := range childrenOfParent {
		if child.GetID() == node.GetID() {
			continue
		}

		callback(child)
	}
}

func (f *NodeSuggestions) ForEachChild(node treeNode, callback nodeCheckCallback) {
	children := node.GetChildren()

	if len(children) == 0 {
		return
	}

	for _, child := range children {
		callback(child)
	}
}
