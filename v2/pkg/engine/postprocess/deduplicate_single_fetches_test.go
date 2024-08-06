package postprocess

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

func TestDeduplicateSingleFetches_ProcessFetchTree(t *testing.T) {

	t.Run("no duplicates", func(t *testing.T) {
		input := &resolve.FetchTreeNode{
			ChildNodes: []*resolve.FetchTreeNode{
				{
					Kind: resolve.FetchTreeNodeKindSingle,
					Item: &resolve.FetchItem{
						FetchPath: nil,
						Fetch:     &resolve.SingleFetch{FetchConfiguration: resolve.FetchConfiguration{Input: "a"}},
					},
				},
				{
					Kind: resolve.FetchTreeNodeKindSingle,
					Item: &resolve.FetchItem{
						FetchPath: nil,
						Fetch:     &resolve.SingleFetch{FetchConfiguration: resolve.FetchConfiguration{Input: "b"}},
					},
				},
			},
		}

		dedup := &deduplicateSingleFetches{}
		dedup.ProcessFetchTree(input)
		assert.Equal(t, input, input)
	})
	t.Run("same path, same input", func(t *testing.T) {
		input := &resolve.FetchTreeNode{
			ChildNodes: []*resolve.FetchTreeNode{
				{
					Kind: resolve.FetchTreeNodeKindSingle,
					Item: &resolve.FetchItem{
						FetchPath: []resolve.FetchItemPathElement{{Kind: resolve.FetchItemPathElementKindObject, Path: []string{"a"}}},
						Fetch:     &resolve.SingleFetch{FetchConfiguration: resolve.FetchConfiguration{Input: "a"}},
					},
				},
				{
					Kind: resolve.FetchTreeNodeKindSingle,
					Item: &resolve.FetchItem{
						FetchPath: []resolve.FetchItemPathElement{{Kind: resolve.FetchItemPathElementKindObject, Path: []string{"a"}}},
						Fetch:     &resolve.SingleFetch{FetchConfiguration: resolve.FetchConfiguration{Input: "a"}},
					},
				},
			},
		}

		output := &resolve.FetchTreeNode{
			ChildNodes: []*resolve.FetchTreeNode{
				{
					Kind: resolve.FetchTreeNodeKindSingle,
					Item: &resolve.FetchItem{
						FetchPath: []resolve.FetchItemPathElement{{Kind: resolve.FetchItemPathElementKindObject, Path: []string{"a"}}},
						Fetch:     &resolve.SingleFetch{FetchConfiguration: resolve.FetchConfiguration{Input: "a"}},
					},
				},
			},
		}

		dedup := &deduplicateSingleFetches{}
		dedup.ProcessFetchTree(input)

		assert.Equal(t, output, input)
	})

	t.Run("different path, same input", func(t *testing.T) {
		input := &resolve.FetchTreeNode{
			ChildNodes: []*resolve.FetchTreeNode{
				{
					Kind: resolve.FetchTreeNodeKindSingle,
					Item: &resolve.FetchItem{
						FetchPath: []resolve.FetchItemPathElement{{Kind: resolve.FetchItemPathElementKindObject, Path: []string{"a"}}},
						Fetch:     &resolve.SingleFetch{FetchConfiguration: resolve.FetchConfiguration{Input: "a"}},
					},
				},
				{
					Kind: resolve.FetchTreeNodeKindSingle,
					Item: &resolve.FetchItem{
						FetchPath: []resolve.FetchItemPathElement{{Kind: resolve.FetchItemPathElementKindObject, Path: []string{"b"}}},
						Fetch:     &resolve.SingleFetch{FetchConfiguration: resolve.FetchConfiguration{Input: "a"}},
					},
				},
			},
		}

		output := &resolve.FetchTreeNode{
			ChildNodes: []*resolve.FetchTreeNode{
				{
					Kind: resolve.FetchTreeNodeKindSingle,
					Item: &resolve.FetchItem{
						FetchPath: []resolve.FetchItemPathElement{{Kind: resolve.FetchItemPathElementKindObject, Path: []string{"a"}}},
						Fetch:     &resolve.SingleFetch{FetchConfiguration: resolve.FetchConfiguration{Input: "a"}},
					},
				},
				{
					Kind: resolve.FetchTreeNodeKindSingle,
					Item: &resolve.FetchItem{
						FetchPath: []resolve.FetchItemPathElement{{Kind: resolve.FetchItemPathElementKindObject, Path: []string{"b"}}},
						Fetch:     &resolve.SingleFetch{FetchConfiguration: resolve.FetchConfiguration{Input: "a"}},
					},
				},
			},
		}

		dedup := &deduplicateSingleFetches{}
		dedup.ProcessFetchTree(input)

		assert.Equal(t, output, input)
	})
}
