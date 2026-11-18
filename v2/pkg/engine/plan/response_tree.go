package plan

import (
	"iter"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
)

// treeRootID is the id of the response tree root.
// The root is not a field, and a root field has no parent field.
const treeRootID = ast.InvalidRef

// responseTree is the tree of the operation fields.
// The id of a field in the tree is its field ref.
type responseTree struct {
	root   *treeField
	fields map[int]*treeField
}

// treeField is a field in the response tree.
type treeField struct {
	id                int
	parent            *treeField
	children          []*treeField
	suggestionIndexes []int
}

func newResponseTree() *responseTree {
	root := &treeField{id: treeRootID}
	return &responseTree{
		root:   root,
		fields: map[int]*treeField{treeRootID: root},
	}
}

// Add adds the field under its parent.
// It does nothing when the field is in the tree already, or its parent is not.
func (t *responseTree) Add(fieldRef, parentID int) {
	if _, ok := t.fields[fieldRef]; ok {
		return
	}
	parent, ok := t.fields[parentID]
	if !ok {
		return
	}
	field := &treeField{id: fieldRef, parent: parent}
	parent.children = append(parent.children, field)
	t.fields[fieldRef] = field
}

func (t *responseTree) Find(fieldRef int) (*treeField, bool) {
	field, ok := t.fields[fieldRef]
	return field, ok
}

// TraverseBFS walks the tree breadth first, starting with the root.
func TraverseBFS(t *responseTree) iter.Seq2[int, *treeField] {
	return func(yield func(int, *treeField) bool) {
		queue := []*treeField{t.root}
		for len(queue) > 0 {
			field := queue[0]
			queue = append(queue[1:], field.children...)
			if !yield(field.id, field) {
				return
			}
		}
	}
}

func (f *treeField) GetID() int {
	return f.id
}

// GetParentID returns the id of the parent, the field must not be the root.
func (f *treeField) GetParentID() int {
	return f.parent.id
}

func (f *treeField) GetParent() *treeField {
	return f.parent
}

func (f *treeField) GetChildren() []*treeField {
	return f.children
}

// RemoveChildren detaches the children from the field.
// The children stay findable by their ids.
func (f *treeField) RemoveChildren() {
	f.children = nil
}

func (f *treeField) SuggestionIndexes() []int {
	return f.suggestionIndexes
}

func (f *treeField) SetSuggestionIndexes(items []int) {
	f.suggestionIndexes = items
}
