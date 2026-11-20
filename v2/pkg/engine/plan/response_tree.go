package plan

import (
	"iter"
	"math"
)

// treeRootID is the id of the response tree root.
// The root is not a field, and a root field has no parent field.
// It is not ast.InvalidRef, so a lookup of an invalid field ref finds nothing.
const treeRootID = math.MinInt

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

// add adds the field under its parent.
// It does nothing when the field is in the tree already, or its parent is not.
func (t *responseTree) add(fieldRef, parentID int) {
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

func (t *responseTree) find(fieldRef int) (*treeField, bool) {
	field, ok := t.fields[fieldRef]
	return field, ok
}

// traverseBFS walks the tree breadth first, starting with the root.
func traverseBFS(t *responseTree) iter.Seq2[int, *treeField] {
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
