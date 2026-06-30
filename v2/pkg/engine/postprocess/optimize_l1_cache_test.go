package postprocess

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

func TestOptimizeL1CacheNarrowsEntityFetches(t *testing.T) {
	tests := []struct {
		name     string
		roots    func() []*resolve.FetchTreeNode
		expected map[int]bool
	}{
		{
			name: "fetch without provider or consumer narrows L1 off",
			roots: func() []*resolve.FetchTreeNode {
				return []*resolve.FetchTreeNode{
					resolve.Single(testL1EntityFetch(1, "User", nil, testProvidesObject("id"))),
				}
			},
			expected: map[int]bool{
				1: false,
			},
		},
		{
			name: "provider and consumer keep L1 on",
			roots: func() []*resolve.FetchTreeNode {
				return []*resolve.FetchTreeNode{
					resolve.Sequence(
						resolve.Single(testL1EntityFetch(1, "User", nil, testProvidesObject("id", "name"))),
						resolve.Single(testL1EntityFetch(2, "User", []int{1}, testProvidesObject("name"))),
					),
				}
			},
			expected: map[int]bool{
				1: true,
				2: true,
			},
		},
		{
			name: "union of prior providers keeps L1 on",
			roots: func() []*resolve.FetchTreeNode {
				return []*resolve.FetchTreeNode{
					resolve.Sequence(
						resolve.Single(testL1EntityFetch(1, "User", nil, testProvidesObject("id"))),
						resolve.Single(testL1EntityFetch(2, "User", nil, testProvidesObject("name"))),
						resolve.Single(testL1EntityFetch(3, "User", []int{1, 2}, testProvidesObject("id", "name"))),
					),
				}
			},
			expected: map[int]bool{
				1: true,
				2: true,
				3: true,
			},
		},
		{
			name: "transitive dependency chain establishes prior provider",
			roots: func() []*resolve.FetchTreeNode {
				return []*resolve.FetchTreeNode{
					resolve.Sequence(
						resolve.Single(testL1EntityFetch(1, "User", nil, testProvidesObject("name"))),
						resolve.Single(testEntityFetchWithL1(2, "Review", []int{1}, testProvidesObject("body"), false)),
						resolve.Single(testL1EntityFetch(3, "User", []int{2}, testProvidesObject("name"))),
					),
				}
			},
			expected: map[int]bool{
				1: true,
				2: false,
				3: true,
			},
		},
		{
			name: "ineligible fetch is never turned on or used as provider",
			roots: func() []*resolve.FetchTreeNode {
				return []*resolve.FetchTreeNode{
					resolve.Sequence(
						resolve.Single(testEntityFetchWithL1(1, "User", nil, testProvidesObject("id", "name"), false)),
						resolve.Single(testL1EntityFetch(2, "User", []int{1}, testProvidesObject("name"))),
					),
				}
			},
			expected: map[int]bool{
				1: false,
				2: false,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			roots := tt.roots()

			optimizer := &optimizeL1Cache{}
			optimizer.processTrees(roots...)

			assert.Equal(t, tt.expected, testL1Assignments(roots...))
		})
	}
}

func TestOptimizeL1CacheCrossTreeProviderConsumer(t *testing.T) {
	root := resolve.Single(testL1EntityFetch(1, "User", nil, testProvidesObject("id", "name")))
	deferFetches := resolve.Single(testL1EntityFetch(2, "User", []int{1}, testProvidesObject("name")))

	optimizer := &optimizeL1Cache{}
	optimizer.processTrees(root, deferFetches)

	expected := map[int]bool{
		1: true,
		2: true,
	}
	assert.Equal(t, expected, testL1Assignments(root, deferFetches))
}

func TestOptimizeL1CacheDeterministicAcrossRuns(t *testing.T) {
	roots := []*resolve.FetchTreeNode{
		resolve.Sequence(
			resolve.Single(testL1EntityFetch(1, "User", nil, testProvidesObject("id"))),
			resolve.Single(testL1EntityFetch(2, "User", nil, testProvidesObject("name"))),
			resolve.Single(testL1EntityFetch(3, "User", []int{1, 2}, testProvidesObject("id", "name"))),
			resolve.Single(testL1EntityFetch(4, "Product", nil, testProvidesObject("sku"))),
		),
	}

	optimizer := &optimizeL1Cache{}
	optimizer.processTrees(roots...)
	first := testL1Assignments(roots...)

	optimizer.processTrees(roots...)
	second := testL1Assignments(roots...)

	expected := map[int]bool{
		1: true,
		2: true,
		3: true,
		4: false,
	}
	assert.Equal(t, expected, first)
	assert.Equal(t, expected, second)
	assert.Equal(t, first, second)
}

func testL1EntityFetch(fetchID int, entityType string, dependsOn []int, providesData *resolve.Object) *resolve.EntityFetch {
	return testEntityFetchWithL1(fetchID, entityType, dependsOn, providesData, true)
}

func testEntityFetchWithL1(fetchID int, entityType string, dependsOn []int, providesData *resolve.Object, l1 bool) *resolve.EntityFetch {
	return &resolve.EntityFetch{
		FetchDependencies: resolve.FetchDependencies{
			FetchID:           fetchID,
			DependsOnFetchIDs: dependsOn,
		},
		Info: testEntityFetchInfo("ds", entityType),
		Cache: &resolve.FetchCacheConfig{
			L1:           l1,
			ProvidesData: providesData,
		},
	}
}

func testProvidesObject(fieldNames ...string) *resolve.Object {
	fields := make([]*resolve.Field, 0, len(fieldNames))
	for _, fieldName := range fieldNames {
		fields = append(fields, &resolve.Field{
			Name: []byte(fieldName),
			Value: &resolve.String{
				Path: []string{fieldName},
			},
		})
	}
	return &resolve.Object{Fields: fields}
}

func testL1Assignments(roots ...*resolve.FetchTreeNode) map[int]bool {
	assignments := map[int]bool{}
	for _, root := range roots {
		testCollectL1Assignments(root, assignments)
	}
	return assignments
}

func testCollectL1Assignments(node *resolve.FetchTreeNode, assignments map[int]bool) {
	if node == nil {
		return
	}
	if node.Kind == resolve.FetchTreeNodeKindSingle && node.Item != nil && node.Item.Fetch != nil {
		deps := node.Item.Fetch.Dependencies()
		assignments[deps.FetchID] = testFetchL1(node.Item.Fetch)
		return
	}
	for _, child := range node.ChildNodes {
		testCollectL1Assignments(child, assignments)
	}
}

func testFetchL1(fetch resolve.Fetch) bool {
	switch f := fetch.(type) {
	case *resolve.SingleFetch:
		return f.Cache != nil && f.Cache.L1
	case *resolve.EntityFetch:
		return f.Cache != nil && f.Cache.L1
	case *resolve.BatchEntityFetch:
		return f.Cache != nil && f.Cache.L1
	default:
		return false
	}
}
