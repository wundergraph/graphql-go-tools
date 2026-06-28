package resolve

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func deferLeaf(id int) *DeferTreeNode {
	return DeferSingle(&DeferFetchGroup{DeferID: id})
}

func liveSet(ids ...int) map[int]DeferDescriptor {
	m := make(map[int]DeferDescriptor, len(ids))
	for _, id := range ids {
		m[id] = DeferDescriptor{}
	}
	return m
}

// leafIDs collects the DeferIDs of every Single leaf in tree order.
func leafIDs(node *DeferTreeNode) []int {
	if node == nil {
		return nil
	}
	if node.Kind == DeferTreeNodeKindSingle {
		if node.Item == nil {
			return nil
		}
		return []int{node.Item.DeferID}
	}
	var ids []int
	for _, child := range node.ChildNodes {
		ids = append(ids, leafIDs(child)...)
	}
	return ids
}

func TestTopDeferID(t *testing.T) {
	t.Parallel()

	id, ok := topDeferID(deferLeaf(7))
	require.True(t, ok)
	assert.Equal(t, 7, id)

	// Sequence: the root is the first child (the parent that runs first).
	id, ok = topDeferID(DeferSequence(deferLeaf(1), deferLeaf(2)))
	require.True(t, ok)
	assert.Equal(t, 1, id)

	// Parallel: independent children, no single root.
	_, ok = topDeferID(DeferParallel(deferLeaf(1), deferLeaf(2)))
	assert.False(t, ok, "Parallel has no single top defer id")

	// Degenerate inputs.
	_, ok = topDeferID(nil)
	assert.False(t, ok)
	_, ok = topDeferID(DeferSequence())
	assert.False(t, ok)
}

func TestPruneDeadDefers(t *testing.T) {
	t.Parallel()

	t.Run("parallel root prunes children independently", func(t *testing.T) {
		t.Parallel()
		tree := DeferParallel(deferLeaf(1), deferLeaf(2), deferLeaf(3))
		pruned := pruneDeadDefers(tree, liveSet(1, 3))
		require.NotNil(t, pruned)
		assert.Equal(t, []int{1, 3}, leafIDs(pruned))
	})

	t.Run("parallel root all dead returns nil", func(t *testing.T) {
		t.Parallel()
		tree := DeferParallel(deferLeaf(1), deferLeaf(2))
		assert.Nil(t, pruneDeadDefers(tree, liveSet()))
	})

	t.Run("sequence dead parent prunes the whole chain", func(t *testing.T) {
		t.Parallel()
		// parent 1 -> child 2. Parent dead drops the child too, even though it
		// would be "live" on its own.
		seq := DeferSequence(deferLeaf(1), deferLeaf(2))
		assert.Nil(t, pruneDeadDefers(seq, liveSet(2)))
	})

	t.Run("sequence live parent keeps the whole chain", func(t *testing.T) {
		t.Parallel()
		seq := DeferSequence(deferLeaf(1), deferLeaf(2))
		pruned := pruneDeadDefers(seq, liveSet(1))
		require.NotNil(t, pruned)
		assert.Equal(t, []int{1, 2}, leafIDs(pruned))
	})

	t.Run("sequence with parallel children is keyed on the parent", func(t *testing.T) {
		t.Parallel()
		// parent 1 -> independent children 2,3.
		seq := DeferSequence(deferLeaf(1), DeferParallel(deferLeaf(2), deferLeaf(3)))
		assert.Nil(t, pruneDeadDefers(seq, liveSet(2, 3)), "parent dead -> whole subtree pruned")

		pruned := pruneDeadDefers(seq, liveSet(1))
		require.NotNil(t, pruned, "parent live -> whole subtree kept")
		assert.Equal(t, []int{1, 2, 3}, leafIDs(pruned))
	})

	t.Run("mixed parallel root with a dead sequence", func(t *testing.T) {
		t.Parallel()
		// Parallel of: a (dead) parent-child sequence, and an independent live leaf.
		tree := DeferParallel(DeferSequence(deferLeaf(1), deferLeaf(2)), deferLeaf(3))
		pruned := pruneDeadDefers(tree, liveSet(3))
		require.NotNil(t, pruned)
		assert.Equal(t, []int{3}, leafIDs(pruned))
	})

	t.Run("does not mutate the input tree", func(t *testing.T) {
		t.Parallel()
		// The plan (response.DeferTree) is shared across requests, so pruning must
		// not modify it in place — it returns kept subtrees by reference and only
		// allocates new Parallel wrapper nodes where children were filtered out.
		tree := DeferParallel(DeferSequence(deferLeaf(1), deferLeaf(2)), deferLeaf(3))
		childCount := len(tree.ChildNodes)

		pruned := pruneDeadDefers(tree, liveSet(3))

		// Original tree is untouched...
		assert.Equal(t, []int{1, 2, 3}, leafIDs(tree), "original tree must be unchanged")
		assert.Len(t, tree.ChildNodes, childCount)
		// ...while the pruned result is a distinct node carrying only the survivor.
		assert.NotSame(t, tree, pruned)
		assert.Equal(t, []int{3}, leafIDs(pruned))
	})
}
