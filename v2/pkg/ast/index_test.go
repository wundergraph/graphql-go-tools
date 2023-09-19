package ast

import (
	"testing"

	"github.com/cespare/xxhash/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func emptyIndex() Index {
	return Index{nodes: make(map[uint64][]Node, 48)}
}

func TestIndex_Add(t *testing.T) {
	var (
		node     = Node{Kind: NodeKindSchemaDefinition, Ref: 0}
		name     = "mynode"
		nodeHash = uint64(8262329559045341693)
	)

	t.Run("AddNodeStr", func(t *testing.T) {
		idx := emptyIndex()

		idx.AddNodeBytes([]byte(name), node)
		expectedIdx := Index{nodes: map[uint64][]Node{nodeHash: {node}}}
		assert.Equal(t, expectedIdx, idx)

		t.Run("add same node second time", func(t *testing.T) {
			idx.AddNodeBytes([]byte(name), node)
			expectedIdx = Index{nodes: map[uint64][]Node{nodeHash: {node, node}}}
			assert.Equal(t, expectedIdx, idx)
		})
	})

	t.Run("AddNodeBytes", func(t *testing.T) {
		idx := emptyIndex()

		idx.AddNodeStr(name, node)
		expectedIdx := Index{nodes: map[uint64][]Node{nodeHash: {node}}}
		assert.Equal(t, expectedIdx, idx)

		t.Run("add same node second time", func(t *testing.T) {
			idx.AddNodeStr(name, node)
			expectedIdx = Index{nodes: map[uint64][]Node{nodeHash: {node, node}}}
			assert.Equal(t, expectedIdx, idx)
		})
	})

	t.Run("Add a few nodes", func(t *testing.T) {
		var (
			otherNode     = Node{Kind: NodeKindField, Ref: 0}
			otherName     = "myothernode"
			otherNodeHash = uint64(3699437218065365471)
		)

		idx := emptyIndex()
		idx.AddNodeStr(name, node)
		idx.AddNodeStr(otherName, otherNode)

		expectedIdx := Index{nodes: map[uint64][]Node{
			nodeHash:      {node},
			otherNodeHash: {otherNode},
		}}
		assert.Equal(t, expectedIdx, idx)
	})
}

func TestIndex_Reset(t *testing.T) {
	var (
		nodeHash = uint64(8262329559045341693)
		node     = Node{Kind: NodeKindField, Ref: 0}
	)

	idx := Index{nodes: map[uint64][]Node{nodeHash: {node}}}
	idx.Reset()
	assert.Equal(t, emptyIndex(), idx)
}

func TestIndex_RemoveNodeByName(t *testing.T) {
	var (
		node     = Node{Kind: NodeKindSchemaDefinition, Ref: 0}
		nodeName = []byte("Schema")
		nodeHash = uint64(419120142902365632)

		queryNode     = Node{Kind: NodeKindObjectTypeDefinition, Ref: 1}
		queryTypeName = []byte("QueryType")
		queryNodeHash = uint64(11055297907069788501)

		mutationNode     = Node{Kind: NodeKindObjectTypeDefinition, Ref: 2}
		mutationTypeName = []byte("MutationType")
		mutationNodeHash = uint64(11562542109605516176)

		subNode     = Node{Kind: NodeKindObjectTypeDefinition, Ref: 3}
		subTypeName = []byte("SubscriptionType")
		subNodeHash = uint64(621582639512712731)
	)

	createIndex := func() Index {
		return Index{
			QueryTypeName:        queryTypeName,
			MutationTypeName:     mutationTypeName,
			SubscriptionTypeName: subTypeName,
			nodes: map[uint64][]Node{
				nodeHash:         {node},
				queryNodeHash:    {queryNode},
				mutationNodeHash: {mutationNode},
				subNodeHash:      {subNode},
			},
		}
	}

	t.Run("remove node", func(t *testing.T) {
		idx := createIndex()
		idx.RemoveNodeByName(nodeName)
		expectedIdx := Index{
			QueryTypeName:        queryTypeName,
			MutationTypeName:     mutationTypeName,
			SubscriptionTypeName: subTypeName,
			nodes: map[uint64][]Node{
				queryNodeHash:    {queryNode},
				mutationNodeHash: {mutationNode},
				subNodeHash:      {subNode},
			},
		}
		assert.Equal(t, expectedIdx, idx)
	})

	t.Run("remove query", func(t *testing.T) {
		idx := createIndex()
		idx.RemoveNodeByName(queryTypeName)
		expectedIdx := Index{
			MutationTypeName:     mutationTypeName,
			SubscriptionTypeName: subTypeName,
			nodes: map[uint64][]Node{
				nodeHash:         {node},
				mutationNodeHash: {mutationNode},
				subNodeHash:      {subNode},
			},
		}
		assert.Equal(t, expectedIdx, idx)
	})

	t.Run("remove mutation", func(t *testing.T) {
		idx := createIndex()
		idx.RemoveNodeByName(mutationTypeName)
		expectedIdx := Index{
			QueryTypeName:        queryTypeName,
			SubscriptionTypeName: subTypeName,
			nodes: map[uint64][]Node{
				nodeHash:      {node},
				queryNodeHash: {queryNode},
				subNodeHash:   {subNode},
			},
		}
		assert.Equal(t, expectedIdx, idx)
	})

	t.Run("remove subscription", func(t *testing.T) {
		idx := createIndex()
		idx.RemoveNodeByName(subTypeName)
		expectedIdx := Index{
			QueryTypeName:    queryTypeName,
			MutationTypeName: mutationTypeName,
			nodes: map[uint64][]Node{
				nodeHash:         {node},
				queryNodeHash:    {queryNode},
				mutationNodeHash: {mutationNode},
			},
		}
		assert.Equal(t, expectedIdx, idx)
	})

	t.Run("remove root types", func(t *testing.T) {
		idx := createIndex()
		idx.RemoveNodeByName(queryTypeName)
		idx.RemoveNodeByName(mutationTypeName)
		idx.RemoveNodeByName(subTypeName)

		expectedIdx := Index{
			nodes: map[uint64][]Node{
				nodeHash: {node},
			},
		}
		assert.Equal(t, expectedIdx, idx)
	})
}

func TestIndex_ReplaceNode(t *testing.T) {
	t.Run("should replace node (kind) with a new node", func(t *testing.T) {
		idx := emptyIndex()

		unrelatedNode := Node{
			Kind: NodeKindObjectTypeDefinition,
			Ref:  5,
		}
		nodeBefore := Node{
			Kind: NodeKindObjectTypeDefinition,
			Ref:  1,
		}

		idx.AddNodeStr("User", unrelatedNode)
		idx.AddNodeStr("User", nodeBefore)
		expectedIndexBefore := Index{
			nodes: map[uint64][]Node{
				xxhash.Sum64String("User"): {unrelatedNode, nodeBefore},
			},
		}

		require.Equal(t, expectedIndexBefore, idx)

		newNode := Node{
			Kind: NodeKindObjectTypeExtension,
			Ref:  2,
		}
		idx.ReplaceNode([]byte("User"), nodeBefore, newNode)
		expectedIndexAfter := Index{
			nodes: map[uint64][]Node{
				xxhash.Sum64String("User"): {unrelatedNode, newNode},
			},
		}

		assert.Equal(t, expectedIndexAfter, idx)
	})
}
