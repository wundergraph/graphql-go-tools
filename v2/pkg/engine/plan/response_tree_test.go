package plan

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// bfsIDs returns the ids of the tree in breadth first order
func bfsIDs(tree *responseTree) []int {
	ids := []int{}
	for id := range TraverseBFS(tree) {
		ids = append(ids, id)
	}
	return ids
}

type treeAdd struct {
	fieldRef int
	parentID int
}

// newTestTree builds root -> 1 -> (2 -> 6, 3 -> (4, 5))
func newTestTree() *responseTree {
	tree := newResponseTree()
	for _, add := range []treeAdd{{1, treeRootID}, {2, 1}, {3, 1}, {6, 2}, {4, 3}, {5, 3}} {
		tree.Add(add.fieldRef, add.parentID)
	}
	return tree
}

func TestResponseTreeAdd(t *testing.T) {
	tests := map[string]struct {
		adds        []treeAdd
		expBFS      []int
		expNotFound []int
	}{
		"empty tree has only the root": {
			expBFS: []int{treeRootID},
		},
		"three level parent-child": {
			adds:   []treeAdd{{0, treeRootID}, {1, 0}, {2, 1}},
			expBFS: []int{treeRootID, 0, 1, 2},
		},
		"three level multi-children": {
			adds:   []treeAdd{{0, treeRootID}, {1, 0}, {2, 1}, {3, 0}},
			expBFS: []int{treeRootID, 0, 1, 3, 2},
		},
		"field in the tree already is not added again": {
			adds:   []treeAdd{{0, treeRootID}, {1, 0}, {2, 1}, {1, 2}},
			expBFS: []int{treeRootID, 0, 1, 2},
		},
		"field with a missing parent is not added": {
			adds:        []treeAdd{{0, treeRootID}, {1, 5}},
			expBFS:      []int{treeRootID, 0},
			expNotFound: []int{1},
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			tree := newResponseTree()
			for _, add := range tt.adds {
				tree.Add(add.fieldRef, add.parentID)
			}

			assert.Equal(t, tt.expBFS, bfsIDs(tree))

			for _, id := range tt.expBFS {
				field, ok := tree.Find(id)
				if assert.True(t, ok, "field %d", id) {
					assert.Equal(t, id, field.GetID())
				}
			}
			for _, id := range tt.expNotFound {
				_, ok := tree.Find(id)
				assert.False(t, ok, "field %d", id)
			}
		})
	}

	t.Run("adding a field again keeps the field", func(t *testing.T) {
		tree := newTestTree()
		field, ok := tree.Find(2)
		require.True(t, ok)
		field.SetSuggestionIndexes([]int{7})

		// under another parent
		tree.Add(2, 3)

		got, ok := tree.Find(2)
		require.True(t, ok)
		assert.Same(t, field, got)
		assert.Equal(t, []int{7}, got.SuggestionIndexes())
		assert.Equal(t, 1, got.GetParentID())
		require.Len(t, got.GetChildren(), 1)
		assert.Equal(t, 6, got.GetChildren()[0].GetID())
		assert.Equal(t, []int{treeRootID, 1, 2, 3, 6, 4, 5}, bfsIDs(tree))
	})
}

func TestResponseTreeFind(t *testing.T) {
	tests := map[string]struct {
		id    int
		expOK bool
	}{
		"missing field": {id: 7, expOK: false},
		"leaf":          {id: 6, expOK: true},
		"mid tree":      {id: 3, expOK: true},
		"root":          {id: treeRootID, expOK: true},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			field, ok := newTestTree().Find(tt.id)

			assert.Equal(t, tt.expOK, ok)
			if tt.expOK {
				assert.Equal(t, tt.id, field.GetID())
			} else {
				assert.Nil(t, field)
			}
		})
	}
}

func TestTraverseBFS(t *testing.T) {
	t.Run("visits the fields level by level, children in the order they were added", func(t *testing.T) {
		assert.Equal(t, []int{treeRootID, 1, 2, 3, 6, 4, 5}, bfsIDs(newTestTree()))
	})

	t.Run("yields the field with its id", func(t *testing.T) {
		for id, field := range TraverseBFS(newTestTree()) {
			assert.Equal(t, id, field.GetID())
		}
	})

	t.Run("stops when the loop breaks", func(t *testing.T) {
		ids := []int{}
		for id := range TraverseBFS(newTestTree()) {
			if len(ids) == 3 {
				break
			}
			ids = append(ids, id)
		}
		assert.Equal(t, []int{treeRootID, 1, 2}, ids)
	})
}

func TestTreeField(t *testing.T) {
	find := func(t *testing.T, tree *responseTree, id int) *treeField {
		t.Helper()
		field, ok := tree.Find(id)
		require.True(t, ok, "field %d", id)
		return field
	}

	t.Run("parent", func(t *testing.T) {
		tree := newTestTree()

		field := find(t, tree, 4)
		assert.Equal(t, 3, field.GetParentID())
		assert.Same(t, find(t, tree, 3), field.GetParent())

		root := find(t, tree, 1).GetParent()
		assert.Equal(t, treeRootID, root.GetID())
		assert.Nil(t, root.GetParent())
		assert.Panics(t, func() { root.GetParentID() })
	})

	t.Run("children in the order they were added", func(t *testing.T) {
		tree := newTestTree()

		children := find(t, tree, 3).GetChildren()
		require.Len(t, children, 2)
		assert.Same(t, find(t, tree, 4), children[0])
		assert.Same(t, find(t, tree, 5), children[1])

		assert.Empty(t, find(t, tree, 6).GetChildren())
	})

	t.Run("remove children", func(t *testing.T) {
		tree := newTestTree()

		find(t, tree, 1).RemoveChildren()

		assert.Empty(t, find(t, tree, 1).GetChildren())
		assert.Equal(t, []int{treeRootID, 1}, bfsIDs(tree))
		// the removed children and their children stay findable
		for _, id := range []int{2, 3, 4, 5, 6} {
			assert.Equal(t, id, find(t, tree, id).GetID())
		}
	})

	t.Run("suggestion indexes", func(t *testing.T) {
		tree := newTestTree()
		field := find(t, tree, 2)

		assert.Nil(t, field.SuggestionIndexes())

		// a suggestion is added by appending its index and setting the indexes back
		field.SetSuggestionIndexes(append(field.SuggestionIndexes(), 7))
		field.SetSuggestionIndexes(append(field.SuggestionIndexes(), 8))
		assert.Equal(t, []int{7, 8}, find(t, tree, 2).SuggestionIndexes())

		field.SetSuggestionIndexes(nil)
		assert.Nil(t, find(t, tree, 2).SuggestionIndexes())
	})
}
