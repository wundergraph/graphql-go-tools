package plan

import (
	"fmt"
)

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
	selected                  bool
	selectionReasons          []string

	fieldRef int
}

func (n *NodeSuggestion) appendSelectionReason(reason string) {
	// fmt.Println("ds:", n.DataSourceHash, fmt.Sprintf("%s.%s", n.TypeName, n.FieldName), "reason:", reason) // NOTE: debug do not remove
	n.selectionReasons = append(n.selectionReasons, reason)
}

func (n *NodeSuggestion) selectWithReason(reason string) {
	if n.selected {
		return
	}
	// n.appendSelectionReason(reason) // NOTE: debug do not remove
	n.selected = true
}

func (n *NodeSuggestion) String() string {
	return fmt.Sprintf(`{"ds":%d,"path":"%s","typeName":"%s","fieldName":"%s","isRootNode":%t, "isSelected": %v,"select reason": %v}`,
		n.DataSourceHash, n.Path, n.TypeName, n.FieldName, n.IsRootNode, n.selected, n.selectionReasons)
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
}

func NewNodeSuggestions() *NodeSuggestions {
	return NewNodeSuggestionsWithSize(32)
}

func NewNodeSuggestionsWithSize(size int) *NodeSuggestions {
	return &NodeSuggestions{
		items:           make([]*NodeSuggestion, 0, size),
		seenFields:      make(map[int]struct{}, size),
		pathSuggestions: make(map[string][]*NodeSuggestion),
	}
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
		if typeName == items[i].TypeName && fieldName == items[i].FieldName && items[i].selected {
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
			f.items[i].selected {

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
		if !f.items[i].selected {
			continue
		}

		suggestions, _ := f.pathSuggestions[f.items[i].Path]
		suggestions = append(f.pathSuggestions[f.items[i].Path], f.items[i])
		f.pathSuggestions[f.items[i].Path] = suggestions
	}
}
