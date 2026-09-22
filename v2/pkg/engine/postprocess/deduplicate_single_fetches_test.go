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
						Fetch:     &resolve.SingleFetch{Input: "a"},
					},
				},
				{
					Kind: resolve.FetchTreeNodeKindSingle,
					Item: &resolve.FetchItem{
						FetchPath: nil,
						Fetch:     &resolve.SingleFetch{Input: "b"},
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
						Fetch:     &resolve.SingleFetch{Input: "a"},
					},
				},
				{
					Kind: resolve.FetchTreeNodeKindSingle,
					Item: &resolve.FetchItem{
						FetchPath: []resolve.FetchItemPathElement{{Kind: resolve.FetchItemPathElementKindObject, Path: []string{"a"}}},
						Fetch:     &resolve.SingleFetch{Input: "a"},
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
						Fetch:     &resolve.SingleFetch{Input: "a"},
					},
				},
			},
		}

		dedup := &deduplicateSingleFetches{}
		dedup.ProcessFetchTree(input)

		assert.Equal(t, output, input)
	})

	t.Run("same path, same input, different fetch id, different defer id", func(t *testing.T) {
		input := &resolve.FetchTreeNode{
			ChildNodes: []*resolve.FetchTreeNode{
				{
					Kind: resolve.FetchTreeNodeKindSingle,
					Item: &resolve.FetchItem{
						FetchPath: []resolve.FetchItemPathElement{{Kind: resolve.FetchItemPathElementKindObject, Path: []string{"a"}}},
						Fetch:     &resolve.SingleFetch{Input: "a", FetchID: 1, DeferID: 1},
					},
				},
				{
					Kind: resolve.FetchTreeNodeKindSingle,
					Item: &resolve.FetchItem{
						FetchPath: []resolve.FetchItemPathElement{{Kind: resolve.FetchItemPathElementKindObject, Path: []string{"a"}}},
						Fetch:     &resolve.SingleFetch{Input: "a", FetchID: 2, DeferID: 2},
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
						Fetch:     &resolve.SingleFetch{Input: "a", FetchID: 1, DeferID: 1},
					},
				},
				{
					Kind: resolve.FetchTreeNodeKindSingle,
					Item: &resolve.FetchItem{
						FetchPath: []resolve.FetchItemPathElement{{Kind: resolve.FetchItemPathElementKindObject, Path: []string{"a"}}},
						Fetch:     &resolve.SingleFetch{Input: "a", FetchID: 2, DeferID: 2},
					},
				},
			},
		}

		dedup := &deduplicateSingleFetches{}
		dedup.ProcessFetchTree(input)

		assert.Equal(t, output, input)
	})

	t.Run("same path, same input, different fetch id, same defer id", func(t *testing.T) {
		input := &resolve.FetchTreeNode{
			ChildNodes: []*resolve.FetchTreeNode{
				{
					Kind: resolve.FetchTreeNodeKindSingle,
					Item: &resolve.FetchItem{
						FetchPath: []resolve.FetchItemPathElement{{Kind: resolve.FetchItemPathElementKindObject, Path: []string{"a"}}},
						Fetch:     &resolve.SingleFetch{Input: "a", FetchID: 1, DeferID: 1},
					},
				},
				{
					Kind: resolve.FetchTreeNodeKindSingle,
					Item: &resolve.FetchItem{
						FetchPath: []resolve.FetchItemPathElement{{Kind: resolve.FetchItemPathElementKindObject, Path: []string{"a"}}},
						Fetch:     &resolve.SingleFetch{Input: "a", FetchID: 2, DeferID: 1},
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
						Fetch:     &resolve.SingleFetch{Input: "a", FetchID: 1, DeferID: 1},
					},
				},
			},
		}

		dedup := &deduplicateSingleFetches{}
		dedup.ProcessFetchTree(input)

		assert.Equal(t, output, input)
	})

	t.Run("same path, same input, different fetch ids - should update dependencies with merged fetch ids", func(t *testing.T) {
		input := &resolve.FetchTreeNode{
			ChildNodes: []*resolve.FetchTreeNode{
				{
					Kind: resolve.FetchTreeNodeKindSingle,
					Item: &resolve.FetchItem{
						FetchPath: []resolve.FetchItemPathElement{{Kind: resolve.FetchItemPathElementKindObject, Path: []string{"root"}}},
						Fetch: &resolve.SingleFetch{
							FetchID:           0,
							DependsOnFetchIDs: []int{},
							Input:             "rootQuery",
						},
					},
				},
				{
					Kind: resolve.FetchTreeNodeKindSingle,
					Item: &resolve.FetchItem{
						FetchPath: []resolve.FetchItemPathElement{{Kind: resolve.FetchItemPathElementKindObject, Path: []string{"root.a"}}},
						Fetch: &resolve.SingleFetch{
							FetchID:           1,
							DependsOnFetchIDs: []int{0},
							Input:             "a",
							Info: &resolve.FetchInfo{
								CoordinateDependencies: []resolve.FetchDependency{
									{
										DependsOn: []resolve.FetchDependencyOrigin{
											{
												FetchID: 0,
											},
										},
									},
								},
							},
						},
					},
				},
				{
					Kind: resolve.FetchTreeNodeKindSingle,
					Item: &resolve.FetchItem{
						FetchPath: []resolve.FetchItemPathElement{{Kind: resolve.FetchItemPathElementKindObject, Path: []string{"root.a"}}},
						Fetch: &resolve.SingleFetch{
							FetchID:           2,
							DependsOnFetchIDs: []int{0},
							Input:             "a",
							Info: &resolve.FetchInfo{
								CoordinateDependencies: []resolve.FetchDependency{
									{
										DependsOn: []resolve.FetchDependencyOrigin{
											{
												FetchID: 0,
											},
										},
									},
								},
							},
						},
					},
				},
				{
					Kind: resolve.FetchTreeNodeKindSingle,
					Item: &resolve.FetchItem{
						FetchPath: []resolve.FetchItemPathElement{{Kind: resolve.FetchItemPathElementKindObject, Path: []string{"root.a.b"}}},
						Fetch: &resolve.SingleFetch{
							FetchID:           4,
							DependsOnFetchIDs: []int{0, 2},
							Input:             "b",
							Info: &resolve.FetchInfo{
								CoordinateDependencies: []resolve.FetchDependency{
									{
										DependsOn: []resolve.FetchDependencyOrigin{
											{
												FetchID: 0,
											},
											{
												FetchID: 2,
											},
										},
									},
								},
							},
						},
					},
				},
				{
					Kind: resolve.FetchTreeNodeKindSingle,
					Item: &resolve.FetchItem{
						FetchPath: []resolve.FetchItemPathElement{{Kind: resolve.FetchItemPathElementKindObject, Path: []string{"root.a.b"}}},
						Fetch: &resolve.SingleFetch{
							FetchID:           3,
							DependsOnFetchIDs: []int{0, 1},
							Input:             "b",
							Info: &resolve.FetchInfo{
								CoordinateDependencies: []resolve.FetchDependency{
									{
										DependsOn: []resolve.FetchDependencyOrigin{
											{
												FetchID: 0,
											},
											{
												FetchID: 1,
											},
										},
									},
								},
							},
						},
					},
				},
			},
		}

		output := &resolve.FetchTreeNode{
			ChildNodes: []*resolve.FetchTreeNode{
				{
					Kind: resolve.FetchTreeNodeKindSingle,
					Item: &resolve.FetchItem{
						FetchPath: []resolve.FetchItemPathElement{{Kind: resolve.FetchItemPathElementKindObject, Path: []string{"root"}}},
						Fetch: &resolve.SingleFetch{
							FetchID:           0,
							DependsOnFetchIDs: []int{},
							Input:             "rootQuery",
						},
					},
				},
				{
					Kind: resolve.FetchTreeNodeKindSingle,
					Item: &resolve.FetchItem{
						FetchPath: []resolve.FetchItemPathElement{{Kind: resolve.FetchItemPathElementKindObject, Path: []string{"root.a"}}},
						Fetch: &resolve.SingleFetch{
							FetchID:           1,
							DependsOnFetchIDs: []int{0},
							Input:             "a",
							Info: &resolve.FetchInfo{
								CoordinateDependencies: []resolve.FetchDependency{
									{
										DependsOn: []resolve.FetchDependencyOrigin{
											{
												FetchID: 0,
											},
										},
									},
								},
							},
						},
					},
				},
				{
					Kind: resolve.FetchTreeNodeKindSingle,
					Item: &resolve.FetchItem{
						FetchPath: []resolve.FetchItemPathElement{{Kind: resolve.FetchItemPathElementKindObject, Path: []string{"root.a.b"}}},
						Fetch: &resolve.SingleFetch{
							FetchID:           4,
							DependsOnFetchIDs: []int{0, 1},
							Input:             "b",
							Info: &resolve.FetchInfo{
								CoordinateDependencies: []resolve.FetchDependency{
									{
										DependsOn: []resolve.FetchDependencyOrigin{
											{
												FetchID: 0,
											},
											{
												FetchID: 1,
											},
										},
									},
								},
							},
						},
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
						Fetch:     &resolve.SingleFetch{Input: "a"},
					},
				},
				{
					Kind: resolve.FetchTreeNodeKindSingle,
					Item: &resolve.FetchItem{
						FetchPath: []resolve.FetchItemPathElement{{Kind: resolve.FetchItemPathElementKindObject, Path: []string{"b"}}},
						Fetch:     &resolve.SingleFetch{Input: "a"},
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
						Fetch:     &resolve.SingleFetch{Input: "a"},
					},
				},
				{
					Kind: resolve.FetchTreeNodeKindSingle,
					Item: &resolve.FetchItem{
						FetchPath: []resolve.FetchItemPathElement{{Kind: resolve.FetchItemPathElementKindObject, Path: []string{"b"}}},
						Fetch:     &resolve.SingleFetch{Input: "a"},
					},
				},
			},
		}

		dedup := &deduplicateSingleFetches{}
		dedup.ProcessFetchTree(input)

		assert.Equal(t, output, input)
	})
}
