package plan

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
)

// bfsIDs returns the ids of the tree in breadth first order
func bfsIDs(tree *responseTree) []int {
	ids := []int{}
	for id := range traverseBFS(tree) {
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
		tree.add(add.fieldRef, add.parentID)
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
				tree.add(add.fieldRef, add.parentID)
			}

			assert.Equal(t, tt.expBFS, bfsIDs(tree))

			for _, id := range tt.expBFS {
				field, ok := tree.find(id)
				if assert.True(t, ok, "field %d", id) {
					assert.Equal(t, id, field.id)
				}
			}
			for _, id := range tt.expNotFound {
				_, ok := tree.find(id)
				assert.False(t, ok, "field %d", id)
			}
		})
	}

	t.Run("adding a field again keeps the field", func(t *testing.T) {
		tree := newTestTree()
		field, ok := tree.find(2)
		require.True(t, ok)
		field.suggestionIndexes = []int{7}

		// under another parent
		tree.add(2, 3)

		got, ok := tree.find(2)
		require.True(t, ok)
		assert.Same(t, field, got)
		assert.Equal(t, []int{7}, got.suggestionIndexes)
		assert.Equal(t, 1, got.parent.id)
		require.Len(t, got.children, 1)
		assert.Equal(t, 6, got.children[0].id)
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
		"invalid ref":   {id: ast.InvalidRef, expOK: false},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			field, ok := newTestTree().find(tt.id)

			assert.Equal(t, tt.expOK, ok)
			if tt.expOK {
				assert.Equal(t, tt.id, field.id)
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
		for id, field := range traverseBFS(newTestTree()) {
			assert.Equal(t, id, field.id)
		}
	})

	t.Run("stops when the loop breaks", func(t *testing.T) {
		ids := []int{}
		for id := range traverseBFS(newTestTree()) {
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
		field, ok := tree.find(id)
		require.True(t, ok, "field %d", id)
		return field
	}

	t.Run("parent", func(t *testing.T) {
		tree := newTestTree()

		field := find(t, tree, 4)
		assert.Equal(t, 3, field.parent.id)
		assert.Same(t, find(t, tree, 3), field.parent)

		root := find(t, tree, 1).parent
		assert.Equal(t, treeRootID, root.id)
		assert.Nil(t, root.parent)
	})

	t.Run("children in the order they were added", func(t *testing.T) {
		tree := newTestTree()

		children := find(t, tree, 3).children
		require.Len(t, children, 2)
		assert.Same(t, find(t, tree, 4), children[0])
		assert.Same(t, find(t, tree, 5), children[1])

		assert.Empty(t, find(t, tree, 6).children)
	})

	t.Run("remove children", func(t *testing.T) {
		tree := newTestTree()

		find(t, tree, 1).children = nil

		assert.Empty(t, find(t, tree, 1).children)
		assert.Equal(t, []int{treeRootID, 1}, bfsIDs(tree))
		// the removed children and their children stay findable
		for _, id := range []int{2, 3, 4, 5, 6} {
			assert.Equal(t, id, find(t, tree, id).id)
		}
	})

	t.Run("suggestion indexes", func(t *testing.T) {
		tree := newTestTree()
		field := find(t, tree, 2)

		assert.Nil(t, field.suggestionIndexes)

		// a suggestion is added by appending its index and setting the indexes back
		field.suggestionIndexes = append(field.suggestionIndexes, 7)
		field.suggestionIndexes = append(field.suggestionIndexes, 8)
		assert.Equal(t, []int{7, 8}, find(t, tree, 2).suggestionIndexes)

		field.suggestionIndexes = nil
		assert.Nil(t, find(t, tree, 2).suggestionIndexes)
	})
}
