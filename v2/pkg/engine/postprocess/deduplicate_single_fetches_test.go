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

	t.Run("same path, same input, different fetch ids - should update dependencies with merged fetch ids", func(t *testing.T) {
		input := &resolve.FetchTreeNode{
			ChildNodes: []*resolve.FetchTreeNode{
				{
					Kind: resolve.FetchTreeNodeKindSingle,
					Item: &resolve.FetchItem{
						FetchPath: []resolve.FetchItemPathElement{{Kind: resolve.FetchItemPathElementKindObject, Path: []string{"root"}}},
						Fetch: &resolve.SingleFetch{
							FetchDependencies: resolve.FetchDependencies{
								FetchID:           0,
								DependsOnFetchIDs: []int{},
							},
							FetchConfiguration: resolve.FetchConfiguration{Input: "rootQuery"},
						},
					},
				},
				{
					Kind: resolve.FetchTreeNodeKindSingle,
					Item: &resolve.FetchItem{
						FetchPath: []resolve.FetchItemPathElement{{Kind: resolve.FetchItemPathElementKindObject, Path: []string{"root.a"}}},
						Fetch: &resolve.SingleFetch{
							FetchDependencies: resolve.FetchDependencies{
								FetchID:           1,
								DependsOnFetchIDs: []int{0},
							},
							FetchConfiguration: resolve.FetchConfiguration{
								Input: "a",
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
							FetchDependencies: resolve.FetchDependencies{
								FetchID:           2,
								DependsOnFetchIDs: []int{0},
							},
							FetchConfiguration: resolve.FetchConfiguration{
								Input: "a",
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
							FetchDependencies: resolve.FetchDependencies{
								FetchID:           4,
								DependsOnFetchIDs: []int{0, 2},
							},
							FetchConfiguration: resolve.FetchConfiguration{
								Input: "b",
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
							FetchDependencies: resolve.FetchDependencies{
								FetchID:           3,
								DependsOnFetchIDs: []int{0, 1},
							},
							FetchConfiguration: resolve.FetchConfiguration{
								Input: "b",
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
							FetchDependencies: resolve.FetchDependencies{
								FetchID:           0,
								DependsOnFetchIDs: []int{},
							},
							FetchConfiguration: resolve.FetchConfiguration{Input: "rootQuery"},
						},
					},
				},
				{
					Kind: resolve.FetchTreeNodeKindSingle,
					Item: &resolve.FetchItem{
						FetchPath: []resolve.FetchItemPathElement{{Kind: resolve.FetchItemPathElementKindObject, Path: []string{"root.a"}}},
						Fetch: &resolve.SingleFetch{
							FetchDependencies: resolve.FetchDependencies{
								FetchID:           1,
								DependsOnFetchIDs: []int{0},
							},
							FetchConfiguration: resolve.FetchConfiguration{
								Input: "a",
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
							FetchDependencies: resolve.FetchDependencies{
								FetchID:           4,
								DependsOnFetchIDs: []int{0, 1},
							},
							FetchConfiguration: resolve.FetchConfiguration{
								Input: "b",
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
