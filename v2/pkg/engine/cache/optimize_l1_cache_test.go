package cache

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

// narrowingTree builds an entity tree with the given scalar fields (nested
// object fields expressed as "parent.child").
func narrowingTree(fieldNames ...string) *resolve.Object {
	obj := &resolve.Object{}
	for _, name := range fieldNames {
		obj.Fields = append(obj.Fields, &resolve.Field{
			Name:  []byte(name),
			Value: &resolve.Scalar{Path: []string{name}},
		})
	}
	return obj
}

// narrowingFetch builds one entity fetch node for the pass.
func narrowingFetch(fetchID int, dependsOn []int, typeName string, provides *resolve.Object, l1, l2 bool) *resolve.FetchTreeNode {
	fetch := &resolve.EntityFetch{
		FetchDependencies: resolve.FetchDependencies{FetchID: fetchID, DependsOnFetchIDs: dependsOn},
		Info:              &resolve.FetchInfo{RootFields: []resolve.GraphCoordinate{{TypeName: typeName}}},
		Cache: &resolve.FetchCacheConfig{
			L1:           l1,
			L2:           l2,
			CacheName:    "entities",
			ProvidesData: provides,
		},
	}
	return &resolve.FetchTreeNode{Kind: resolve.FetchTreeNodeKindSingle, Item: &resolve.FetchItem{Fetch: fetch}}
}

func tree(nodes ...*resolve.FetchTreeNode) *resolve.FetchTreeNode {
	return &resolve.FetchTreeNode{Kind: resolve.FetchTreeNodeKindSequence, ChildNodes: nodes}
}

func l1Of(node *resolve.FetchTreeNode) bool {
	cfg := node.Item.Fetch.CacheConfig()
	return cfg != nil && cfg.L1
}

// TestOptimizeL1CacheRows covers the narrowing rows, including the adversarial
// ones the OLD test set lacked.
func TestOptimizeL1CacheRows(t *testing.T) {
	pass := &optimizeL1Cache{}

	t.Run("provider/consumer pair via a dependency edge keeps both", func(t *testing.T) {
		provider := narrowingFetch(1, nil, "Product", narrowingTree("name", "price"), true, true)
		consumer := narrowingFetch(2, []int{1}, "Product", narrowingTree("name"), true, true)
		pass.optimize([]*resolve.FetchTreeNode{tree(provider, consumer)}, nil)
		assert.True(t, l1Of(provider))
		assert.True(t, l1Of(consumer))
	})

	t.Run("a lone fetch is narrowed off", func(t *testing.T) {
		lone := narrowingFetch(1, nil, "Product", narrowingTree("name"), true, true)
		pass.optimize([]*resolve.FetchTreeNode{tree(lone)}, nil)
		assert.False(t, l1Of(lone))
	})

	t.Run("narrowing a lone L1-only fetch re-nils the inert config", func(t *testing.T) {
		lone := narrowingFetch(1, nil, "Product", narrowingTree("name"), true, false)
		pass.optimize([]*resolve.FetchTreeNode{tree(lone)}, nil)
		assert.Nil(t, lone.Item.Fetch.CacheConfig())
	})

	t.Run("never turns L1 on: a configurator-false fetch stays false despite a pair", func(t *testing.T) {
		provider := narrowingFetch(1, nil, "Product", narrowingTree("name", "price"), false, true)
		consumer := narrowingFetch(2, []int{1}, "Product", narrowingTree("name"), true, true)
		pass.optimize([]*resolve.FetchTreeNode{tree(provider, consumer)}, nil)
		assert.False(t, l1Of(provider))
		// The consumer's only candidate provider is ineligible: no pair, off.
		assert.False(t, l1Of(consumer))
	})

	t.Run("union of two partial providers covers the consumer: all three keep L1", func(t *testing.T) {
		providerA := narrowingFetch(1, nil, "Product", narrowingTree("name"), true, true)
		providerB := narrowingFetch(2, nil, "Product", narrowingTree("price"), true, true)
		consumer := narrowingFetch(3, []int{1, 2}, "Product", narrowingTree("name", "price"), true, true)
		pass.optimize([]*resolve.FetchTreeNode{tree(providerA, providerB, consumer)}, nil)
		assert.True(t, l1Of(providerA))
		assert.True(t, l1Of(providerB))
		assert.True(t, l1Of(consumer))
	})

	t.Run("[flaw pin] an irrelevant provider sharing a consumer is narrowed off", func(t *testing.T) {
		relevant := narrowingFetch(1, nil, "Product", narrowingTree("name"), true, true)
		irrelevant := narrowingFetch(2, nil, "Product", narrowingTree("weight"), true, true)
		consumer := narrowingFetch(3, []int{1, 2}, "Product", narrowingTree("name"), true, true)
		pass.optimize([]*resolve.FetchTreeNode{tree(relevant, irrelevant, consumer)}, nil)
		assert.True(t, l1Of(relevant))
		// Shares NO field with the consumer: the union fallback must not keep
		// it merely because the OTHER provider covers the consumer.
		assert.False(t, l1Of(irrelevant))
		assert.True(t, l1Of(consumer))
	})

	t.Run("[flaw pin] nested NAME-only overlap is not a contribution", func(t *testing.T) {
		// The irrelevant provider shares the OBJECT NAME "warehouse" but none
		// of the leaves the consumer reads; the relevant provider covers the
		// consumer alone. The contribution gate must recurse past the name.
		warehouseWith := func(leaf string) *resolve.Object {
			return &resolve.Object{Fields: []*resolve.Field{
				{Name: []byte("warehouse"), Value: &resolve.Object{Fields: []*resolve.Field{
					{Name: []byte(leaf), Value: &resolve.Scalar{Path: []string{leaf}}},
				}}},
			}}
		}
		relevant := narrowingFetch(1, nil, "Product", warehouseWith("location"), true, true)
		irrelevant := narrowingFetch(2, nil, "Product", warehouseWith("id"), true, true)
		consumer := narrowingFetch(3, []int{1, 2}, "Product", warehouseWith("location"), true, true)
		pass.optimize([]*resolve.FetchTreeNode{tree(relevant, irrelevant, consumer)}, nil)
		assert.True(t, l1Of(relevant))
		assert.False(t, l1Of(irrelevant))
		assert.True(t, l1Of(consumer))
	})

	t.Run("partial overlap without full coverage narrows both off", func(t *testing.T) {
		provider := narrowingFetch(1, nil, "Product", narrowingTree("name", "price"), true, true)
		consumer := narrowingFetch(2, []int{1}, "Product", narrowingTree("price", "weight"), true, true)
		pass.optimize([]*resolve.FetchTreeNode{tree(provider, consumer)}, nil)
		assert.False(t, l1Of(provider))
		assert.False(t, l1Of(consumer))
	})

	t.Run("empty union: a consumer with no prior providers is narrowed off", func(t *testing.T) {
		later := narrowingFetch(2, nil, "Product", narrowingTree("name"), true, true)
		unrelated := narrowingFetch(1, nil, "User", narrowingTree("name"), true, true)
		pass.optimize([]*resolve.FetchTreeNode{tree(unrelated, later)}, nil)
		assert.False(t, l1Of(later))
		assert.False(t, l1Of(unrelated))
	})

	t.Run("transitive dependency chains order fetches", func(t *testing.T) {
		provider := narrowingFetch(1, nil, "Product", narrowingTree("name", "price"), true, true)
		bridge := narrowingFetch(2, []int{1}, "User", narrowingTree("id"), true, true)
		consumer := narrowingFetch(3, []int{2}, "Product", narrowingTree("name"), true, true)
		pass.optimize([]*resolve.FetchTreeNode{tree(provider, bridge, consumer)}, nil)
		assert.True(t, l1Of(provider))
		assert.True(t, l1Of(consumer))
		assert.False(t, l1Of(bridge))
	})

	t.Run("cross-tree: a root provider is kept because its only consumer is deferred", func(t *testing.T) {
		// NO dependency edge between the two — different branches; only tree
		// order relates them.
		provider := narrowingFetch(1, nil, "Product", narrowingTree("name", "price"), true, true)
		consumer := narrowingFetch(7, []int{5}, "Product", narrowingTree("name"), true, true)
		pass.optimize([]*resolve.FetchTreeNode{tree(provider), tree(consumer)}, nil)
		assert.True(t, l1Of(provider))
		assert.True(t, l1Of(consumer))

		// Per-tree-only behavior is rejected: the SAME shapes narrowed off when
		// the pass only sees the root tree.
		soloProvider := narrowingFetch(1, nil, "Product", narrowingTree("name", "price"), true, true)
		pass.optimize([]*resolve.FetchTreeNode{tree(soloProvider)}, nil)
		assert.False(t, l1Of(soloProvider))
	})

	t.Run("a parent defer group orders before its NESTED child group", func(t *testing.T) {
		provider := narrowingFetch(3, nil, "Product", narrowingTree("name", "price"), true, true)
		consumer := narrowingFetch(4, nil, "Product", narrowingTree("name"), true, true)
		// treeParents: tree 1 (outer group) encloses tree 2 (nested group).
		pass.optimize([]*resolve.FetchTreeNode{tree(), tree(provider), tree(consumer)}, []int{-1, 0, 1})
		assert.True(t, l1Of(provider))
		assert.True(t, l1Of(consumer))
	})

	t.Run("defer groups among themselves stay unordered", func(t *testing.T) {
		deferA := narrowingFetch(3, nil, "Product", narrowingTree("name", "price"), true, true)
		deferB := narrowingFetch(4, nil, "Product", narrowingTree("name"), true, true)
		pass.optimize([]*resolve.FetchTreeNode{tree(), tree(deferA), tree(deferB)}, nil)
		assert.False(t, l1Of(deferA))
		assert.False(t, l1Of(deferB))
	})

	t.Run("a malformed treeParents cycle degrades to unordered, never hangs", func(t *testing.T) {
		// treeParents crosses the public ConfigureCaching API: probing whether
		// the ROOT tree encloses a member of a 1<->2 parent cycle must
		// terminate (the walk never reaches -1), leaving root and cycle
		// unordered — the root provider loses its only consumer and both
		// narrow off.
		provider := narrowingFetch(1, nil, "Product", narrowingTree("name", "price"), true, true)
		consumer := narrowingFetch(4, nil, "Product", narrowingTree("name"), true, true)
		pass.optimize([]*resolve.FetchTreeNode{tree(provider), tree(consumer), tree()}, []int{-1, 2, 1})
		assert.False(t, l1Of(provider))
		assert.False(t, l1Of(consumer))
	})

	t.Run("determinism: a second run leaves the narrowed state unchanged", func(t *testing.T) {
		provider := narrowingFetch(1, nil, "Product", narrowingTree("name", "price"), true, true)
		consumer := narrowingFetch(2, []int{1}, "Product", narrowingTree("name"), true, true)
		lone := narrowingFetch(3, nil, "User", narrowingTree("id"), true, true)
		trees := []*resolve.FetchTreeNode{tree(provider, consumer, lone)}
		pass.optimize(trees, nil)
		first := []bool{l1Of(provider), l1Of(consumer), l1Of(lone)}
		pass.optimize(trees, nil)
		assert.Equal(t, first, []bool{l1Of(provider), l1Of(consumer), l1Of(lone)})
		assert.Equal(t, []bool{true, true, false}, first)
	})

	t.Run("union computation never mutates the provider trees", func(t *testing.T) {
		providerA := narrowingFetch(1, nil, "Product", &resolve.Object{Fields: []*resolve.Field{
			{Name: []byte("brand"), Value: &resolve.Object{Fields: []*resolve.Field{
				{Name: []byte("id"), Value: &resolve.Scalar{Path: []string{"id"}}},
			}}},
		}}, true, true)
		providerB := narrowingFetch(2, nil, "Product", &resolve.Object{Fields: []*resolve.Field{
			{Name: []byte("brand"), Value: &resolve.Object{Fields: []*resolve.Field{
				{Name: []byte("location"), Value: &resolve.Scalar{Path: []string{"location"}}},
			}}},
		}}, true, true)
		consumer := narrowingFetch(3, []int{1, 2}, "Product", &resolve.Object{Fields: []*resolve.Field{
			{Name: []byte("brand"), Value: &resolve.Object{Fields: []*resolve.Field{
				{Name: []byte("id"), Value: &resolve.Scalar{Path: []string{"id"}}},
				{Name: []byte("location"), Value: &resolve.Scalar{Path: []string{"location"}}},
			}}},
		}}, true, true)
		pass.optimize([]*resolve.FetchTreeNode{tree(providerA, providerB, consumer)}, nil)
		assert.True(t, l1Of(consumer)) // served by the recursive union
		// The FIRST-PASS aliasing bug: the union merged INTO provider A's live
		// tree. A's brand object must still have exactly its own field.
		brandA := providerA.Item.Fetch.CacheConfig().ProvidesData.Fields[0].Value.(*resolve.Object)
		require.Len(t, brandA.Fields, 1)
		assert.Equal(t, "id", string(brandA.Fields[0].Name))
	})
}
