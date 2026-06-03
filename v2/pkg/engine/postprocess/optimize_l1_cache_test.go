package postprocess

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

// makeObject creates a resolve.Object with the given field names (all as scalars)
func makeObject(fieldNames ...string) *resolve.Object {
	fields := make([]*resolve.Field, len(fieldNames))
	for i, name := range fieldNames {
		fields[i] = &resolve.Field{Name: []byte(name), Value: &resolve.String{}}
	}
	return &resolve.Object{Fields: fields}
}

// Helper function to create a simple entity fetch with given fields
func makeEntityFetch(fetchID int, entityType string, fieldNames []string, dependsOnIDs []int) *resolve.EntityFetch {
	fields := make([]*resolve.Field, len(fieldNames))
	for i, name := range fieldNames {
		fields[i] = &resolve.Field{Name: []byte(name)}
	}
	return &resolve.EntityFetch{
		FetchDependencies: resolve.FetchDependencies{
			FetchID:           fetchID,
			DependsOnFetchIDs: dependsOnIDs,
		},
		Info: &resolve.FetchInfo{
			RootFields: []resolve.GraphCoordinate{
				{TypeName: entityType, FieldName: "field"},
			},
			ProvidesData: &resolve.Object{
				Fields: fields,
			},
		},
		Caching: resolve.FetchCacheConfiguration{
			UseL1Cache: true, // Default value
		},
	}
}

// Helper function to create a batch entity fetch with given fields
func makeBatchEntityFetch(fetchID int, entityType string, fieldNames []string, dependsOnIDs []int) *resolve.BatchEntityFetch {
	fields := make([]*resolve.Field, len(fieldNames))
	for i, name := range fieldNames {
		fields[i] = &resolve.Field{Name: []byte(name)}
	}
	return &resolve.BatchEntityFetch{
		FetchDependencies: resolve.FetchDependencies{
			FetchID:           fetchID,
			DependsOnFetchIDs: dependsOnIDs,
		},
		Info: &resolve.FetchInfo{
			RootFields: []resolve.GraphCoordinate{
				{TypeName: entityType, FieldName: "field"},
			},
			ProvidesData: &resolve.Object{
				Fields: fields,
			},
		},
		Caching: resolve.FetchCacheConfiguration{
			UseL1Cache: true, // Default value
		},
	}
}

// Helper function to create a root field fetch with L1 entity cache templates
// providesData describes the full response tree of the root field
func makeRootFetchWithL1Templates(fetchID int, dependsOnIDs []int, entityTypes []string, providesData *resolve.Object) *resolve.SingleFetch {
	templates := make(map[string]resolve.CacheKeyTemplate)
	for _, et := range entityTypes {
		templates["users:"+et] = &resolve.EntityQueryCacheKeyTemplate{}
	}
	return &resolve.SingleFetch{
		FetchDependencies: resolve.FetchDependencies{
			FetchID:           fetchID,
			DependsOnFetchIDs: dependsOnIDs,
		},
		Info: &resolve.FetchInfo{
			RootFields: []resolve.GraphCoordinate{
				{TypeName: "Query", FieldName: "users"},
			},
			ProvidesData: providesData,
		},
		FetchConfiguration: resolve.FetchConfiguration{
			RequiresEntityFetch:      false,
			RequiresEntityBatchFetch: false,
			Caching: resolve.FetchCacheConfiguration{
				RootFieldL1EntityCacheKeyTemplates: templates,
			},
		},
	}
}

func getUseL1Cache(fetch resolve.Fetch) bool {
	switch f := fetch.(type) {
	case *resolve.SingleFetch:
		return f.Caching.UseL1Cache
	case *resolve.EntityFetch:
		return f.Caching.UseL1Cache
	case *resolve.BatchEntityFetch:
		return f.Caching.UseL1Cache
	}
	return false
}

func TestOptimizeL1Cache_SingleEntityFetch_NoProvider_NoConsumer(t *testing.T) {
	// Single entity fetch with no prior fetches and no subsequent fetches
	// Should have UseL1Cache = false (cannot benefit from L1)
	processor := &optimizeL1Cache{}

	entityFetch := makeEntityFetch(1, "User", []string{"id", "name"}, nil)
	input := resolve.Sequence(
		resolve.Single(entityFetch),
	)

	processor.ProcessFetchTree(input)

	assert.Equal(t, false, getUseL1Cache(entityFetch), "single entity fetch with no provider/consumer should have UseL1Cache=false")
}

func TestOptimizeL1Cache_TwoEntityFetches_SameType_SameFields(t *testing.T) {
	// Two entity fetches with same type and same fields
	// First can write for second (as provider), second can read from first (as consumer)
	// Both should have UseL1Cache = true
	processor := &optimizeL1Cache{}

	fetch1 := makeEntityFetch(1, "User", []string{"id", "name"}, nil)
	fetch2 := makeEntityFetch(2, "User", []string{"id", "name"}, []int{1})

	input := resolve.Sequence(
		resolve.Single(fetch1),
		resolve.Single(fetch2),
	)

	processor.ProcessFetchTree(input)

	assert.Equal(t, true, getUseL1Cache(fetch1), "first fetch should have UseL1Cache=true (can write for second)")
	assert.Equal(t, true, getUseL1Cache(fetch2), "second fetch should have UseL1Cache=true (can read from first)")
}

func TestOptimizeL1Cache_TwoEntityFetches_DifferentTypes(t *testing.T) {
	// Two entity fetches with different types
	// Neither can help the other
	processor := &optimizeL1Cache{}

	fetch1 := makeEntityFetch(1, "User", []string{"id", "name"}, nil)
	fetch2 := makeEntityFetch(2, "Product", []string{"id", "title"}, []int{1})

	input := resolve.Sequence(
		resolve.Single(fetch1),
		resolve.Single(fetch2),
	)

	processor.ProcessFetchTree(input)

	assert.Equal(t, false, getUseL1Cache(fetch1), "first fetch should have UseL1Cache=false (different type from second)")
	assert.Equal(t, false, getUseL1Cache(fetch2), "second fetch should have UseL1Cache=false (different type from first)")
}

func TestOptimizeL1Cache_ProviderHasSuperset(t *testing.T) {
	// First fetch provides superset of fields, second needs subset
	// First can write for second, second can read from first
	processor := &optimizeL1Cache{}

	fetch1 := makeEntityFetch(1, "User", []string{"id", "name", "email"}, nil)
	fetch2 := makeEntityFetch(2, "User", []string{"id", "name"}, []int{1})

	input := resolve.Sequence(
		resolve.Single(fetch1),
		resolve.Single(fetch2),
	)

	processor.ProcessFetchTree(input)

	assert.Equal(t, true, getUseL1Cache(fetch1), "first fetch should have UseL1Cache=true (superset provider)")
	assert.Equal(t, true, getUseL1Cache(fetch2), "second fetch should have UseL1Cache=true (subset consumer)")
}

func TestOptimizeL1Cache_ProviderHasSubset(t *testing.T) {
	// First fetch provides subset of fields, second needs superset
	// First cannot write useful data for second
	processor := &optimizeL1Cache{}

	fetch1 := makeEntityFetch(1, "User", []string{"id"}, nil)
	fetch2 := makeEntityFetch(2, "User", []string{"id", "name"}, []int{1})

	input := resolve.Sequence(
		resolve.Single(fetch1),
		resolve.Single(fetch2),
	)

	processor.ProcessFetchTree(input)

	assert.Equal(t, false, getUseL1Cache(fetch1), "first fetch should have UseL1Cache=false (subset cannot help superset)")
	assert.Equal(t, false, getUseL1Cache(fetch2), "second fetch should have UseL1Cache=false (cannot read from first)")
}

func TestOptimizeL1Cache_ThreeFetchChain_AllSameFields(t *testing.T) {
	// Chain A→B→C, all same type, same fields
	// All three should be enabled:
	// - A: can write for B and C
	// - B: can read from A, can write for C
	// - C: can read from A or B
	processor := &optimizeL1Cache{}

	fetchA := makeEntityFetch(1, "User", []string{"id", "name"}, nil)
	fetchB := makeEntityFetch(2, "User", []string{"id", "name"}, []int{1})
	fetchC := makeEntityFetch(3, "User", []string{"id", "name"}, []int{2})

	input := resolve.Sequence(
		resolve.Single(fetchA),
		resolve.Single(fetchB),
		resolve.Single(fetchC),
	)

	processor.ProcessFetchTree(input)

	assert.Equal(t, true, getUseL1Cache(fetchA), "A should have UseL1Cache=true (can write for B and C)")
	assert.Equal(t, true, getUseL1Cache(fetchB), "B should have UseL1Cache=true (can read from A, write for C)")
	assert.Equal(t, true, getUseL1Cache(fetchC), "C should have UseL1Cache=true (can read from A or B)")
}

func TestOptimizeL1Cache_ThreeFetchChain_IncreasingFields(t *testing.T) {
	// Chain A→B→C where:
	// - A provides {id}
	// - B needs {id, name}
	// - C needs {id, name}
	//
	// A alone doesn't cover B or C. But A contributes {id} to the union
	// that covers C (union of A+B = {id, name}). With union-based optimization,
	// A is enabled as a writer because it participates in the chain.
	// B covers C directly.
	processor := &optimizeL1Cache{}

	fetchA := makeEntityFetch(1, "User", []string{"id"}, nil)
	fetchB := makeEntityFetch(2, "User", []string{"id", "name"}, []int{1})
	fetchC := makeEntityFetch(3, "User", []string{"id", "name"}, []int{2})

	input := resolve.Sequence(
		resolve.Single(fetchA),
		resolve.Single(fetchB),
		resolve.Single(fetchC),
	)

	processor.ProcessFetchTree(input)

	assert.Equal(t, true, getUseL1Cache(fetchA), "A should have UseL1Cache=true (contributes to union covering C)")
	assert.Equal(t, true, getUseL1Cache(fetchB), "B should have UseL1Cache=true (can write for C)")
	assert.Equal(t, true, getUseL1Cache(fetchC), "C should have UseL1Cache=true (can read from B or union)")
}

func TestOptimizeL1Cache_ThreeFetchChain_DecreasingFields(t *testing.T) {
	// Chain A→B→C where:
	// - A provides {id, name, email}
	// - B needs {id, name}
	// - C needs {id}
	//
	// All can help each other
	processor := &optimizeL1Cache{}

	fetchA := makeEntityFetch(1, "User", []string{"id", "name", "email"}, nil)
	fetchB := makeEntityFetch(2, "User", []string{"id", "name"}, []int{1})
	fetchC := makeEntityFetch(3, "User", []string{"id"}, []int{2})

	input := resolve.Sequence(
		resolve.Single(fetchA),
		resolve.Single(fetchB),
		resolve.Single(fetchC),
	)

	processor.ProcessFetchTree(input)

	assert.Equal(t, true, getUseL1Cache(fetchA), "A should have UseL1Cache=true (can write for B and C)")
	assert.Equal(t, true, getUseL1Cache(fetchB), "B should have UseL1Cache=true (can read from A, write for C)")
	assert.Equal(t, true, getUseL1Cache(fetchC), "C should have UseL1Cache=true (can read from A or B)")
}

func TestOptimizeL1Cache_ParallelFetches_SameType(t *testing.T) {
	// Two parallel fetches with same type
	// They execute in parallel, so neither can read from the other
	// (no dependency relationship)
	processor := &optimizeL1Cache{}

	fetch1 := makeEntityFetch(1, "User", []string{"id", "name"}, nil)
	fetch2 := makeEntityFetch(2, "User", []string{"id", "name"}, nil)

	input := resolve.Sequence(
		resolve.Parallel(
			resolve.Single(fetch1),
			resolve.Single(fetch2),
		),
	)

	processor.ProcessFetchTree(input)

	// Neither can help the other since they run in parallel (no dependency)
	assert.Equal(t, false, getUseL1Cache(fetch1), "first parallel fetch should have UseL1Cache=false")
	assert.Equal(t, false, getUseL1Cache(fetch2), "second parallel fetch should have UseL1Cache=false")
}

func TestOptimizeL1Cache_ParallelThenSequential(t *testing.T) {
	// Two parallel fetches followed by a sequential fetch that depends on both
	processor := &optimizeL1Cache{}

	fetch1 := makeEntityFetch(1, "User", []string{"id", "name"}, nil)
	fetch2 := makeEntityFetch(2, "Product", []string{"id", "title"}, nil)
	fetch3 := makeEntityFetch(3, "User", []string{"id", "name"}, []int{1, 2})

	input := resolve.Sequence(
		resolve.Parallel(
			resolve.Single(fetch1),
			resolve.Single(fetch2),
		),
		resolve.Single(fetch3),
	)

	processor.ProcessFetchTree(input)

	assert.Equal(t, true, getUseL1Cache(fetch1), "fetch1 should have UseL1Cache=true (can write for fetch3)")
	assert.Equal(t, false, getUseL1Cache(fetch2), "fetch2 should have UseL1Cache=false (different type)")
	assert.Equal(t, true, getUseL1Cache(fetch3), "fetch3 should have UseL1Cache=true (can read from fetch1)")
}

func TestOptimizeL1Cache_RootFetchWithL1Templates_HasConsumer(t *testing.T) {
	// Root field fetch with L1 entity cache templates for User type
	// Followed by entity fetch for User
	// Root fetch provides {id, name} and entity fetch needs {id, name}
	// Root fetch should have UseL1Cache=true because it can write for entity fetch
	processor := &optimizeL1Cache{}

	// Root field provides User with {id, name}
	rootProvidesData := makeObject("id", "name")
	rootFetch := makeRootFetchWithL1Templates(0, nil, []string{"User"}, rootProvidesData)
	entityFetch := makeEntityFetch(1, "User", []string{"id", "name"}, []int{0})

	input := resolve.Sequence(
		resolve.Single(rootFetch),
		resolve.Single(entityFetch),
	)

	processor.ProcessFetchTree(input)

	// Root fetch can write for entity fetch (provides all fields consumer needs)
	assert.Equal(t, true, getUseL1Cache(rootFetch), "root fetch should have UseL1Cache=true (can write for User entity fetch)")
	// Entity fetch can read from root field's L1 cache population
	assert.Equal(t, true, getUseL1Cache(entityFetch), "entity fetch should have UseL1Cache=true (root field provides User)")
}

func TestOptimizeL1Cache_RootFetchWithL1Templates_NoConsumer(t *testing.T) {
	// Root field fetch with L1 entity cache templates for User type
	// No subsequent entity fetch for User type
	// Root fetch should have UseL1Cache=false because no one can benefit
	processor := &optimizeL1Cache{}

	rootProvidesData := makeObject("id", "name")
	rootFetch := makeRootFetchWithL1Templates(0, nil, []string{"User"}, rootProvidesData)

	input := resolve.Sequence(
		resolve.Single(rootFetch),
	)

	processor.ProcessFetchTree(input)

	// No entity fetch can read from root field's L1 cache population
	assert.Equal(t, false, getUseL1Cache(rootFetch), "root fetch should have UseL1Cache=false (no User entity fetch to benefit)")
}

func TestOptimizeL1Cache_RootFetchWithL1Templates_DifferentTypeConsumer(t *testing.T) {
	// Root field fetch with L1 entity cache templates for User type
	// But subsequent entity fetch is for Product type (different)
	// Root fetch should have UseL1Cache=false because the entity fetch cannot benefit
	processor := &optimizeL1Cache{}

	rootProvidesData := makeObject("id", "name")
	rootFetch := makeRootFetchWithL1Templates(0, nil, []string{"User"}, rootProvidesData)
	entityFetch := makeEntityFetch(1, "Product", []string{"id", "title"}, []int{0})

	input := resolve.Sequence(
		resolve.Single(rootFetch),
		resolve.Single(entityFetch),
	)

	processor.ProcessFetchTree(input)

	// Root fetch provides User, but entity fetch needs Product
	assert.Equal(t, false, getUseL1Cache(rootFetch), "root fetch should have UseL1Cache=false (no matching entity type)")
	assert.Equal(t, false, getUseL1Cache(entityFetch), "entity fetch should have UseL1Cache=false (root provides different type)")
}

func TestOptimizeL1Cache_RootFetchWithL1Templates_ProvidesMissingFields(t *testing.T) {
	// Root field provides {id, name} but entity fetch needs {id, name, email}
	// Root fetch should have UseL1Cache=false because it doesn't provide all fields
	// This is critical: we should NOT populate L1 with incomplete data
	processor := &optimizeL1Cache{}

	// Root field provides User with {id, name} only
	rootProvidesData := makeObject("id", "name")
	rootFetch := makeRootFetchWithL1Templates(0, nil, []string{"User"}, rootProvidesData)
	// Entity fetch needs {id, name, email} - email is missing from root field
	entityFetch := makeEntityFetch(1, "User", []string{"id", "name", "email"}, []int{0})

	input := resolve.Sequence(
		resolve.Single(rootFetch),
		resolve.Single(entityFetch),
	)

	processor.ProcessFetchTree(input)

	// Root fetch should NOT use L1 because it doesn't provide all fields consumer needs
	assert.Equal(t, false, getUseL1Cache(rootFetch),
		"root fetch should have UseL1Cache=false (doesn't provide email field consumer needs)")
	// Entity fetch cannot read from root field (missing fields)
	assert.Equal(t, false, getUseL1Cache(entityFetch),
		"entity fetch should have UseL1Cache=false (root field doesn't provide email)")
}

func TestOptimizeL1Cache_RootFetchWithL1Templates_ProvidesSuperset(t *testing.T) {
	// Root field provides {id, name, email} and entity fetch needs {id, name}
	// Root fetch should have UseL1Cache=true because it provides more than needed
	processor := &optimizeL1Cache{}

	// Root field provides User with {id, name, email}
	rootProvidesData := makeObject("id", "name", "email")
	rootFetch := makeRootFetchWithL1Templates(0, nil, []string{"User"}, rootProvidesData)
	// Entity fetch needs {id, name} - subset of what root field provides
	entityFetch := makeEntityFetch(1, "User", []string{"id", "name"}, []int{0})

	input := resolve.Sequence(
		resolve.Single(rootFetch),
		resolve.Single(entityFetch),
	)

	processor.ProcessFetchTree(input)

	// Root fetch should use L1 because it provides all fields (and more) consumer needs
	assert.Equal(t, true, getUseL1Cache(rootFetch),
		"root fetch should have UseL1Cache=true (provides superset of consumer's fields)")
	// Entity fetch can read from root field
	assert.Equal(t, true, getUseL1Cache(entityFetch),
		"entity fetch should have UseL1Cache=true (root field provides all needed fields)")
}

func TestOptimizeL1Cache_RootFetchWithL1Templates_NestedEntityFields(t *testing.T) {
	// Root field returns a nested structure: Query.products -> [Product] -> author: User
	// The User entity is nested inside the Product response
	// Entity fetch for User should be able to read from root field's L1 cache
	processor := &optimizeL1Cache{}

	// Root field provides: { products: [{ id, name, author: { id, username } }] }
	// The User entity is at the "author" path with fields {id, username}
	rootProvidesData := &resolve.Object{
		Fields: []*resolve.Field{
			{Name: []byte("products"), Value: &resolve.Array{
				Item: &resolve.Object{
					Fields: []*resolve.Field{
						{Name: []byte("id"), Value: &resolve.String{}},
						{Name: []byte("name"), Value: &resolve.String{}},
						{Name: []byte("author"), Value: &resolve.Object{
							Fields: []*resolve.Field{
								{Name: []byte("id"), Value: &resolve.String{}},
								{Name: []byte("username"), Value: &resolve.String{}},
							},
						}},
					},
				},
			}},
		},
	}
	rootFetch := makeRootFetchWithL1Templates(0, nil, []string{"User"}, rootProvidesData)
	// Entity fetch needs User with {id, username}
	entityFetch := makeEntityFetch(1, "User", []string{"id", "username"}, []int{0})

	input := resolve.Sequence(
		resolve.Single(rootFetch),
		resolve.Single(entityFetch),
	)

	processor.ProcessFetchTree(input)

	// Root fetch provides User nested at products[].author with all needed fields
	assert.Equal(t, true, getUseL1Cache(rootFetch),
		"root fetch should have UseL1Cache=true (nested User has all fields consumer needs)")
	// Entity fetch can read from root field's nested User
	assert.Equal(t, true, getUseL1Cache(entityFetch),
		"entity fetch should have UseL1Cache=true (root field provides nested User)")
}

func TestOptimizeL1Cache_RootFetchWithL1Templates_NestedEntityMissingFields(t *testing.T) {
	// Root field returns nested User but missing fields
	// Root field provides: { products: [{ author: { id } }] } (missing username)
	// Entity fetch for User needs {id, username}
	processor := &optimizeL1Cache{}

	rootProvidesData := &resolve.Object{
		Fields: []*resolve.Field{
			{Name: []byte("products"), Value: &resolve.Array{
				Item: &resolve.Object{
					Fields: []*resolve.Field{
						{Name: []byte("id"), Value: &resolve.String{}},
						{Name: []byte("author"), Value: &resolve.Object{
							Fields: []*resolve.Field{
								{Name: []byte("id"), Value: &resolve.String{}},
								// Missing username!
							},
						}},
					},
				},
			}},
		},
	}
	rootFetch := makeRootFetchWithL1Templates(0, nil, []string{"User"}, rootProvidesData)
	// Entity fetch needs User with {id, username}
	entityFetch := makeEntityFetch(1, "User", []string{"id", "username"}, []int{0})

	input := resolve.Sequence(
		resolve.Single(rootFetch),
		resolve.Single(entityFetch),
	)

	processor.ProcessFetchTree(input)

	// Root fetch provides User at products[].author but missing username
	assert.Equal(t, false, getUseL1Cache(rootFetch),
		"root fetch should have UseL1Cache=false (nested User missing username)")
	// Entity fetch cannot read from root field
	assert.Equal(t, false, getUseL1Cache(entityFetch),
		"entity fetch should have UseL1Cache=false (root field's User missing username)")
}

func TestOptimizeL1Cache_BatchEntityFetch(t *testing.T) {
	// Test with BatchEntityFetch type
	processor := &optimizeL1Cache{}

	fetch1 := makeBatchEntityFetch(1, "User", []string{"id", "name"}, nil)
	fetch2 := makeBatchEntityFetch(2, "User", []string{"id", "name"}, []int{1})

	input := resolve.Sequence(
		resolve.Single(fetch1),
		resolve.Single(fetch2),
	)

	processor.ProcessFetchTree(input)

	assert.Equal(t, true, getUseL1Cache(fetch1), "first batch fetch should have UseL1Cache=true")
	assert.Equal(t, true, getUseL1Cache(fetch2), "second batch fetch should have UseL1Cache=true")
}

func TestOptimizeL1Cache_MixedEntityAndBatchFetch(t *testing.T) {
	// Mix of EntityFetch and BatchEntityFetch
	processor := &optimizeL1Cache{}

	fetch1 := makeEntityFetch(1, "User", []string{"id", "name"}, nil)
	fetch2 := makeBatchEntityFetch(2, "User", []string{"id"}, []int{1})

	input := resolve.Sequence(
		resolve.Single(fetch1),
		resolve.Single(fetch2),
	)

	processor.ProcessFetchTree(input)

	assert.Equal(t, true, getUseL1Cache(fetch1), "entity fetch should have UseL1Cache=true (can write for batch)")
	assert.Equal(t, true, getUseL1Cache(fetch2), "batch fetch should have UseL1Cache=true (can read from entity)")
}

func TestOptimizeL1Cache_DisabledProcessor(t *testing.T) {
	// When processor is disabled, it should not modify any flags
	processor := &optimizeL1Cache{disable: true}

	fetch := makeEntityFetch(1, "User", []string{"id", "name"}, nil)
	fetch.Caching.UseL1Cache = true // Set to true initially

	input := resolve.Sequence(
		resolve.Single(fetch),
	)

	processor.ProcessFetchTree(input)

	// Should remain unchanged (true) since processor is disabled
	assert.Equal(t, true, getUseL1Cache(fetch), "disabled processor should not change UseL1Cache flag")
}

func TestOptimizeL1Cache_TransitiveDependencies(t *testing.T) {
	// Test transitive dependencies: A→B→C where C needs same type as A
	processor := &optimizeL1Cache{}

	fetchA := makeEntityFetch(1, "User", []string{"id", "name"}, nil)
	fetchB := makeEntityFetch(2, "Product", []string{"id", "title"}, []int{1})
	fetchC := makeEntityFetch(3, "User", []string{"id", "name"}, []int{2})

	input := resolve.Sequence(
		resolve.Single(fetchA),
		resolve.Single(fetchB),
		resolve.Single(fetchC),
	)

	processor.ProcessFetchTree(input)

	// C transitively depends on A (through B), so A can help C
	assert.Equal(t, true, getUseL1Cache(fetchA), "A should have UseL1Cache=true (can write for C)")
	assert.Equal(t, false, getUseL1Cache(fetchB), "B should have UseL1Cache=false (different type)")
	assert.Equal(t, true, getUseL1Cache(fetchC), "C should have UseL1Cache=true (can read from A)")
}

func TestOptimizeL1Cache_NilRoot(t *testing.T) {
	// Test nil root handling
	processor := &optimizeL1Cache{}
	processor.ProcessFetchTree(nil) // Should not panic
}

func TestOptimizeL1Cache_EmptyTree(t *testing.T) {
	// Test empty tree handling
	processor := &optimizeL1Cache{}
	input := resolve.Sequence()
	processor.ProcessFetchTree(input) // Should not panic
}

func TestObjectProvidesAllFields(t *testing.T) {
	t.Run("nil consumer", func(t *testing.T) {
		provider := &resolve.Object{
			Fields: []*resolve.Field{
				{Name: []byte("id")},
			},
		}
		assert.True(t, objectProvidesAllFields(provider, nil))
	})

	t.Run("nil provider with empty consumer", func(t *testing.T) {
		consumer := &resolve.Object{Fields: []*resolve.Field{}}
		assert.True(t, objectProvidesAllFields(nil, consumer))
	})

	t.Run("nil provider with non-empty consumer", func(t *testing.T) {
		consumer := &resolve.Object{
			Fields: []*resolve.Field{
				{Name: []byte("id")},
			},
		}
		assert.False(t, objectProvidesAllFields(nil, consumer))
	})

	t.Run("provider has all consumer fields", func(t *testing.T) {
		provider := &resolve.Object{
			Fields: []*resolve.Field{
				{Name: []byte("id")},
				{Name: []byte("name")},
				{Name: []byte("email")},
			},
		}
		consumer := &resolve.Object{
			Fields: []*resolve.Field{
				{Name: []byte("id")},
				{Name: []byte("name")},
			},
		}
		assert.True(t, objectProvidesAllFields(provider, consumer))
	})

	t.Run("provider equals consumer fields", func(t *testing.T) {
		provider := &resolve.Object{
			Fields: []*resolve.Field{
				{Name: []byte("id")},
				{Name: []byte("name")},
			},
		}
		consumer := &resolve.Object{
			Fields: []*resolve.Field{
				{Name: []byte("id")},
				{Name: []byte("name")},
			},
		}
		assert.True(t, objectProvidesAllFields(provider, consumer))
	})

	t.Run("provider missing consumer field", func(t *testing.T) {
		provider := &resolve.Object{
			Fields: []*resolve.Field{
				{Name: []byte("id")},
			},
		}
		consumer := &resolve.Object{
			Fields: []*resolve.Field{
				{Name: []byte("id")},
				{Name: []byte("name")},
			},
		}
		assert.False(t, objectProvidesAllFields(provider, consumer))
	})

	t.Run("nested object - provider has all nested fields", func(t *testing.T) {
		provider := &resolve.Object{
			Fields: []*resolve.Field{
				{Name: []byte("id")},
				{
					Name: []byte("address"),
					Value: &resolve.Object{
						Fields: []*resolve.Field{
							{Name: []byte("street")},
							{Name: []byte("city")},
							{Name: []byte("country")},
						},
					},
				},
			},
		}
		consumer := &resolve.Object{
			Fields: []*resolve.Field{
				{Name: []byte("id")},
				{
					Name: []byte("address"),
					Value: &resolve.Object{
						Fields: []*resolve.Field{
							{Name: []byte("street")},
							{Name: []byte("city")},
						},
					},
				},
			},
		}
		assert.True(t, objectProvidesAllFields(provider, consumer))
	})

	t.Run("nested object - provider missing nested field", func(t *testing.T) {
		provider := &resolve.Object{
			Fields: []*resolve.Field{
				{Name: []byte("id")},
				{
					Name: []byte("address"),
					Value: &resolve.Object{
						Fields: []*resolve.Field{
							{Name: []byte("street")},
						},
					},
				},
			},
		}
		consumer := &resolve.Object{
			Fields: []*resolve.Field{
				{Name: []byte("id")},
				{
					Name: []byte("address"),
					Value: &resolve.Object{
						Fields: []*resolve.Field{
							{Name: []byte("street")},
							{Name: []byte("city")}, // Provider doesn't have this
						},
					},
				},
			},
		}
		assert.False(t, objectProvidesAllFields(provider, consumer))
	})

	t.Run("array of objects - provider has all fields", func(t *testing.T) {
		provider := &resolve.Object{
			Fields: []*resolve.Field{
				{Name: []byte("id")},
				{
					Name: []byte("friends"),
					Value: &resolve.Array{
						Item: &resolve.Object{
							Fields: []*resolve.Field{
								{Name: []byte("id")},
								{Name: []byte("name")},
								{Name: []byte("email")},
							},
						},
					},
				},
			},
		}
		consumer := &resolve.Object{
			Fields: []*resolve.Field{
				{Name: []byte("id")},
				{
					Name: []byte("friends"),
					Value: &resolve.Array{
						Item: &resolve.Object{
							Fields: []*resolve.Field{
								{Name: []byte("id")},
								{Name: []byte("name")},
							},
						},
					},
				},
			},
		}
		assert.True(t, objectProvidesAllFields(provider, consumer))
	})

	t.Run("array of objects - provider missing nested field", func(t *testing.T) {
		provider := &resolve.Object{
			Fields: []*resolve.Field{
				{Name: []byte("id")},
				{
					Name: []byte("friends"),
					Value: &resolve.Array{
						Item: &resolve.Object{
							Fields: []*resolve.Field{
								{Name: []byte("id")},
							},
						},
					},
				},
			},
		}
		consumer := &resolve.Object{
			Fields: []*resolve.Field{
				{Name: []byte("id")},
				{
					Name: []byte("friends"),
					Value: &resolve.Array{
						Item: &resolve.Object{
							Fields: []*resolve.Field{
								{Name: []byte("id")},
								{Name: []byte("name")}, // Provider doesn't have this in array item
							},
						},
					},
				},
			},
		}
		assert.False(t, objectProvidesAllFields(provider, consumer))
	})

	t.Run("deeply nested objects", func(t *testing.T) {
		provider := &resolve.Object{
			Fields: []*resolve.Field{
				{
					Name: []byte("user"),
					Value: &resolve.Object{
						Fields: []*resolve.Field{
							{
								Name: []byte("profile"),
								Value: &resolve.Object{
									Fields: []*resolve.Field{
										{Name: []byte("bio")},
										{Name: []byte("avatar")},
									},
								},
							},
						},
					},
				},
			},
		}
		consumer := &resolve.Object{
			Fields: []*resolve.Field{
				{
					Name: []byte("user"),
					Value: &resolve.Object{
						Fields: []*resolve.Field{
							{
								Name: []byte("profile"),
								Value: &resolve.Object{
									Fields: []*resolve.Field{
										{Name: []byte("bio")},
									},
								},
							},
						},
					},
				},
			},
		}
		assert.True(t, objectProvidesAllFields(provider, consumer))
	})
}

// =============================================================================
// UNION-BASED L1 OPTIMIZATION TESTS
//
// These tests verify that the optimizer computes the UNION of ancestor providers'
// ProvidesData fields. Currently, hasValidProvider checks each provider individually.
// With the union fix, it should check if the combined fields of all prior providers
// cover the consumer's needs.
// =============================================================================

func TestOptimizeL1Cache_Union_BasicDisjointFields(t *testing.T) {
	// A={name}, B={email}, C needs {name, email}
	// Neither A nor B individually covers C.
	// Union: {name, email} covers C.
	processor := &optimizeL1Cache{}

	fetchA := makeEntityFetch(1, "User", []string{"name"}, nil)
	fetchB := makeEntityFetch(2, "User", []string{"email"}, []int{1})
	fetchC := makeEntityFetch(3, "User", []string{"name", "email"}, []int{2})

	input := resolve.Sequence(
		resolve.Single(fetchA),
		resolve.Single(fetchB),
		resolve.Single(fetchC),
	)

	processor.ProcessFetchTree(input)

	assert.Equal(t, true, getUseL1Cache(fetchA), "A should be true (contributes name to union covering C)")
	assert.Equal(t, true, getUseL1Cache(fetchB), "B should be true (contributes email to union covering C)")
	assert.Equal(t, true, getUseL1Cache(fetchC), "C should be true (union of A+B covers {name, email})")
}

func TestOptimizeL1Cache_Union_InsufficientUnion(t *testing.T) {
	// A={name}, B={email}, C needs {name, phone}
	// Union: {name, email} does NOT cover {name, phone} — missing phone.
	processor := &optimizeL1Cache{}

	fetchA := makeEntityFetch(1, "User", []string{"name"}, nil)
	fetchB := makeEntityFetch(2, "User", []string{"email"}, []int{1})
	fetchC := makeEntityFetch(3, "User", []string{"name", "phone"}, []int{2})

	input := resolve.Sequence(
		resolve.Single(fetchA),
		resolve.Single(fetchB),
		resolve.Single(fetchC),
	)

	processor.ProcessFetchTree(input)

	assert.Equal(t, false, getUseL1Cache(fetchA), "A should be false (union still can't cover C)")
	assert.Equal(t, false, getUseL1Cache(fetchB), "B should be false (union still can't cover C)")
	assert.Equal(t, false, getUseL1Cache(fetchC), "C should be false (union {name,email} missing phone)")
}

func TestOptimizeL1Cache_Union_OverlappingFields(t *testing.T) {
	// A={name, id}, B={id, email}, C needs {name, email}
	// A has name but not email. B has email but not name.
	// Union: {name, id, email} covers C.
	// Overlap: both have id.
	processor := &optimizeL1Cache{}

	fetchA := makeEntityFetch(1, "User", []string{"name", "id"}, nil)
	fetchB := makeEntityFetch(2, "User", []string{"id", "email"}, []int{1})
	fetchC := makeEntityFetch(3, "User", []string{"name", "email"}, []int{2})

	input := resolve.Sequence(
		resolve.Single(fetchA),
		resolve.Single(fetchB),
		resolve.Single(fetchC),
	)

	processor.ProcessFetchTree(input)

	assert.Equal(t, true, getUseL1Cache(fetchA), "A should be true (contributes name to union)")
	assert.Equal(t, true, getUseL1Cache(fetchB), "B should be true (contributes email to union)")
	assert.Equal(t, true, getUseL1Cache(fetchC), "C should be true (union covers {name, email})")
}

func TestOptimizeL1Cache_Union_FourFetchChain(t *testing.T) {
	// A={a}, B={b}, C={c}, D needs {a, b, c}
	// No single ancestor covers D. Union of A+B+C = {a,b,c} covers D.
	processor := &optimizeL1Cache{}

	fetchA := makeEntityFetch(1, "User", []string{"a"}, nil)
	fetchB := makeEntityFetch(2, "User", []string{"b"}, []int{1})
	fetchC := makeEntityFetch(3, "User", []string{"c"}, []int{2})
	fetchD := makeEntityFetch(4, "User", []string{"a", "b", "c"}, []int{3})

	input := resolve.Sequence(
		resolve.Single(fetchA),
		resolve.Single(fetchB),
		resolve.Single(fetchC),
		resolve.Single(fetchD),
	)

	processor.ProcessFetchTree(input)

	assert.Equal(t, true, getUseL1Cache(fetchA), "A should be true (contributes a)")
	assert.Equal(t, true, getUseL1Cache(fetchB), "B should be true (contributes b)")
	assert.Equal(t, true, getUseL1Cache(fetchC), "C should be true (contributes c)")
	assert.Equal(t, true, getUseL1Cache(fetchD), "D should be true (union covers {a,b,c})")
}

func TestOptimizeL1Cache_Union_MiddleFetchRedundant(t *testing.T) {
	// A={name, email}, B={phone}, C needs {name, email}
	// A alone covers C. B's {phone} is not needed by C.
	// With union-based optimization, B is still enabled because it
	// participates in the ancestor chain and the union covers C.
	// This is a benign false positive — the cost is just a cheap L1 write.
	processor := &optimizeL1Cache{}

	fetchA := makeEntityFetch(1, "User", []string{"name", "email"}, nil)
	fetchB := makeEntityFetch(2, "User", []string{"phone"}, []int{1})
	fetchC := makeEntityFetch(3, "User", []string{"name", "email"}, []int{2})

	input := resolve.Sequence(
		resolve.Single(fetchA),
		resolve.Single(fetchB),
		resolve.Single(fetchC),
	)

	processor.ProcessFetchTree(input)

	assert.Equal(t, true, getUseL1Cache(fetchA), "A should be true (covers C directly)")
	assert.Equal(t, true, getUseL1Cache(fetchB), "B should be true (participates in chain; benign false positive)")
	assert.Equal(t, true, getUseL1Cache(fetchC), "C should be true (A covers it)")
}

func TestOptimizeL1Cache_Union_MiddleFetchEssential(t *testing.T) {
	// A={name}, B={email}, C needs {name, email}
	// B is essential: without B, union = {name} doesn't cover C.
	// B should be true because it contributes to the union.
	processor := &optimizeL1Cache{}

	fetchA := makeEntityFetch(1, "User", []string{"name"}, nil)
	fetchB := makeEntityFetch(2, "User", []string{"email"}, []int{1})
	fetchC := makeEntityFetch(3, "User", []string{"name", "email"}, []int{2})

	input := resolve.Sequence(
		resolve.Single(fetchA),
		resolve.Single(fetchB),
		resolve.Single(fetchC),
	)

	processor.ProcessFetchTree(input)

	assert.Equal(t, true, getUseL1Cache(fetchA), "A should be true (essential for union)")
	assert.Equal(t, true, getUseL1Cache(fetchB), "B should be true (essential for union)")
	assert.Equal(t, true, getUseL1Cache(fetchC), "C should be true (union covers it)")
}

func TestOptimizeL1Cache_Union_RootFieldPlusEntity(t *testing.T) {
	// Root field provides {name} for User, entity fetch A provides {email},
	// consumer C needs {name, email}.
	// Root alone doesn't cover C. A alone doesn't cover C.
	// Union: {name, email} covers C.
	processor := &optimizeL1Cache{}

	rootProvidesData := &resolve.Object{
		Fields: []*resolve.Field{
			{Name: []byte("users"), Value: &resolve.Array{
				Item: &resolve.Object{
					Fields: []*resolve.Field{
						{Name: []byte("name"), Value: &resolve.String{}},
					},
				},
			}},
		},
	}
	rootFetch := makeRootFetchWithL1Templates(0, nil, []string{"User"}, rootProvidesData)
	fetchA := makeEntityFetch(1, "User", []string{"email"}, []int{0})
	fetchC := makeEntityFetch(2, "User", []string{"name", "email"}, []int{1})

	input := resolve.Sequence(
		resolve.Single(rootFetch),
		resolve.Single(fetchA),
		resolve.Single(fetchC),
	)

	processor.ProcessFetchTree(input)

	assert.Equal(t, true, getUseL1Cache(rootFetch), "root should be true (contributes name to union)")
	assert.Equal(t, true, getUseL1Cache(fetchA), "A should be true (contributes email to union)")
	assert.Equal(t, true, getUseL1Cache(fetchC), "C should be true (union of root+A covers {name, email})")
}

func TestOptimizeL1Cache_Union_IncreasingFieldsRevisited(t *testing.T) {
	// Revisits the existing ThreeFetchChain_IncreasingFields scenario:
	// A={id}, B={id, name}, C={id, name}
	// Currently A=false. With union: A should be true because A's {id}
	// contributes to the union {id, name} that covers C.
	// (This is the same as the existing test but with the union expectation.)
	processor := &optimizeL1Cache{}

	fetchA := makeEntityFetch(1, "User", []string{"id"}, nil)
	fetchB := makeEntityFetch(2, "User", []string{"id", "name"}, []int{1})
	fetchC := makeEntityFetch(3, "User", []string{"id", "name"}, []int{2})

	input := resolve.Sequence(
		resolve.Single(fetchA),
		resolve.Single(fetchB),
		resolve.Single(fetchC),
	)

	processor.ProcessFetchTree(input)

	// With union: A contributes {id} to union for C.
	// B alone covers C, so A's contribution is redundant. But A should
	// still be enabled because its write to L1 accumulates data that
	// downstream fetches can use.
	assert.Equal(t, true, getUseL1Cache(fetchA), "A should be true (contributes to union covering C)")
	assert.Equal(t, true, getUseL1Cache(fetchB), "B should be true (covers C directly)")
	assert.Equal(t, true, getUseL1Cache(fetchC), "C should be true (B or union covers it)")
}

func TestOptimizeL1Cache_Union_ParallelProvidersCannotUnion(t *testing.T) {
	// A and B run in parallel (no dependency between them).
	// C depends on both. C needs {name, email}.
	// A={name}, B={email}.
	// Even though A+B union covers C, parallel fetches write to L1 concurrently.
	// Phase 1 L1 check runs before Phase 2 HTTP, so parallel L1 writes from
	// sibling fetches aren't visible to each other. But C runs AFTER both
	// A and B complete, so C CAN read the union of A+B from L1.
	processor := &optimizeL1Cache{}

	fetchA := makeEntityFetch(1, "User", []string{"name"}, nil)
	fetchB := makeEntityFetch(2, "User", []string{"email"}, nil)
	fetchC := makeEntityFetch(3, "User", []string{"name", "email"}, []int{1, 2})

	input := resolve.Sequence(
		resolve.Parallel(
			resolve.Single(fetchA),
			resolve.Single(fetchB),
		),
		resolve.Single(fetchC),
	)

	processor.ProcessFetchTree(input)

	// A and B are parallel but both execute before C (C depends on both).
	// Union of A+B = {name, email} covers C.
	assert.Equal(t, true, getUseL1Cache(fetchA), "A should be true (contributes name for C)")
	assert.Equal(t, true, getUseL1Cache(fetchB), "B should be true (contributes email for C)")
	assert.Equal(t, true, getUseL1Cache(fetchC), "C should be true (union of parallel A+B covers it)")
}
