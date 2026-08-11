package cache

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wundergraph/astjson"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

// templateFor builds the key template for a representation node under the
// "products" subgraph with L2 enabled, exactly as PrepareFetch does.
func templateFor(t *testing.T, node *resolve.Object) cacheKeyTemplate {
	t.Helper()
	template, ok := newCacheKeyTemplate(nil, &resolve.FetchCacheConfig{
		SubgraphName: "products",
		KeySpec:      resolve.CacheKeySpec{Scope: resolve.CacheScopeEntity, TypeName: "Product", Representation: node},
	}, 0, l2Access{enabled: true})
	require.True(t, ok)
	return template
}

// TestCacheKeyTemplateRender pins the canonical preimage of a rendered key and
// the render rules the representation model adds: @requires values participate
// in the identity, per-concrete-type conditioned fields are skipped rather than
// fatal, and a node with a static __typename keys under that static name.
func TestCacheKeyTemplateRender(t *testing.T) {
	t.Run("the preimage is __typename plus the canonical representation", func(t *testing.T) {
		template := templateFor(t, productRepresentation(t, "upc"))
		keys, ok := template.render(astjson.MustParseBytes([]byte(`{"__typename":"Product","upc":"1","name":"Table"}`)))
		require.True(t, ok)
		// Only representation fields enter the key; other item fields do not.
		assert.Equal(t, itemKeys{
			L1: `products:{"__typename":"Product","representation":{"upc":"1"}}`,
			L2: renderCacheKey("v1:products", []byte(`{"__typename":"Product","representation":{"upc":"1"}}`)),
		}, keys)
	})

	t.Run("no template without a representation node", func(t *testing.T) {
		_, ok := newCacheKeyTemplate(nil, &resolve.FetchCacheConfig{SubgraphName: "products"}, 0, l2Access{enabled: true})
		assert.False(t, ok)
		_, ok = newCacheKeyTemplate(nil, nil, 0, l2Access{enabled: true})
		assert.False(t, ok)
	})

	t.Run("@requires fields are key material: the same values render the same keys", func(t *testing.T) {
		template := templateFor(t, productRepresentation(t, "upc", "price"))
		first, ok := template.render(astjson.MustParseBytes([]byte(`{"__typename":"Product","upc":"1","price":100}`)))
		require.True(t, ok)
		second, ok := template.render(astjson.MustParseBytes([]byte(`{"__typename":"Product","upc":"1","price":100}`)))
		require.True(t, ok)
		assert.Equal(t, first, second)
		assert.Equal(t, itemKeys{
			L1: `products:{"__typename":"Product","representation":{"upc":"1","price":"100"}}`,
			L2: renderCacheKey("v1:products", []byte(`{"__typename":"Product","representation":{"upc":"1","price":"100"}}`)),
		}, first)
	})

	t.Run("@requires fields are key material: a changed required value is a NEW key", func(t *testing.T) {
		template := templateFor(t, productRepresentation(t, "upc", "price"))
		cheap, ok := template.render(astjson.MustParseBytes([]byte(`{"__typename":"Product","upc":"1","price":100}`)))
		require.True(t, ok)
		pricey, ok := template.render(astjson.MustParseBytes([]byte(`{"__typename":"Product","upc":"1","price":120}`)))
		require.True(t, ok)
		assert.NotEqual(t, cheap, pricey)
		assert.Equal(t, itemKeys{
			L1: `products:{"__typename":"Product","representation":{"upc":"1","price":"120"}}`,
			L2: renderCacheKey("v1:products", []byte(`{"__typename":"Product","representation":{"upc":"1","price":"120"}}`)),
		}, pricey)
	})

	t.Run("an absent @requires value makes the keys unrenderable", func(t *testing.T) {
		template := templateFor(t, productRepresentation(t, "upc", "price"))
		keys, ok := template.render(astjson.MustParseBytes([]byte(`{"__typename":"Product","upc":"1"}`)))
		assert.False(t, ok)
		assert.Equal(t, itemKeys{}, keys)
	})

	t.Run("a field conditioned on another concrete type is SKIPPED, not fatal", func(t *testing.T) {
		// The merged node of an abstract-path batch fetch: one conditioned key
		// field per concrete type.
		node := mergedRepresentation(t, plan.FederationMetaData{},
			plan.FederationFieldConfiguration{TypeName: "Product", SelectionSet: "upc"},
			plan.FederationFieldConfiguration{TypeName: "Brand", SelectionSet: "id"},
		)
		template := templateFor(t, node)

		productKeys, ok := template.render(astjson.MustParseBytes([]byte(`{"__typename":"Product","upc":"1"}`)))
		require.True(t, ok)
		assert.Equal(t, itemKeys{
			L1: `products:{"__typename":"Product","representation":{"upc":"1"}}`,
			L2: renderCacheKey("v1:products", []byte(`{"__typename":"Product","representation":{"upc":"1"}}`)),
		}, productKeys)

		brandKeys, ok := template.render(astjson.MustParseBytes([]byte(`{"__typename":"Brand","id":"b1"}`)))
		require.True(t, ok)
		assert.Equal(t, itemKeys{
			L1: `products:{"__typename":"Brand","representation":{"id":"b1"}}`,
			L2: renderCacheKey("v1:products", []byte(`{"__typename":"Brand","representation":{"id":"b1"}}`)),
		}, brandKeys)
		assert.NotEqual(t, productKeys, brandKeys)
	})

	t.Run("an item matching no conditioned set is unrenderable", func(t *testing.T) {
		node := mergedRepresentation(t, plan.FederationMetaData{},
			plan.FederationFieldConfiguration{TypeName: "Product", SelectionSet: "upc"},
			plan.FederationFieldConfiguration{TypeName: "Brand", SelectionSet: "id"},
		)
		keys, ok := templateFor(t, node).render(astjson.MustParseBytes([]byte(`{"__typename":"Warehouse","upc":"1","id":"b1"}`)))
		assert.False(t, ok)
		assert.Equal(t, itemKeys{}, keys)
	})

	t.Run("an item without __typename is unrenderable", func(t *testing.T) {
		keys, ok := templateFor(t, productRepresentation(t, "upc")).render(astjson.MustParseBytes([]byte(`{"upc":"1"}`)))
		assert.False(t, ok)
		assert.Equal(t, itemKeys{}, keys)
	})

	t.Run("a static node __typename keys under that name whatever the item says", func(t *testing.T) {
		// An @interfaceObject target: the planner bakes the interface name into
		// the node as a StaticString, and that is what the fetch sends.
		node := mergedRepresentation(t, plan.FederationMetaData{
			InterfaceObjects: []plan.EntityInterfaceConfiguration{
				{InterfaceTypeName: "Sellable", ConcreteTypeNames: []string{"Product"}},
			},
		}, plan.FederationFieldConfiguration{TypeName: "Product", SelectionSet: "upc"})
		template := templateFor(t, node)

		concrete, ok := template.render(astjson.MustParseBytes([]byte(`{"__typename":"Product","upc":"1"}`)))
		require.True(t, ok)
		iface, ok := template.render(astjson.MustParseBytes([]byte(`{"__typename":"Sellable","upc":"1"}`)))
		require.True(t, ok)
		// ONE entry for the entity, under the interface name the fetch sends.
		assert.Equal(t, itemKeys{
			L1: `products:{"__typename":"Sellable","representation":{"upc":"1"}}`,
			L2: renderCacheKey("v1:products", []byte(`{"__typename":"Sellable","representation":{"upc":"1"}}`)),
		}, concrete)
		assert.Equal(t, concrete, iface)
	})

	t.Run("nil item and nil representation render nothing", func(t *testing.T) {
		keys, ok := templateFor(t, productRepresentation(t, "upc")).render(nil)
		assert.False(t, ok)
		assert.Equal(t, itemKeys{}, keys)
		keys, ok = cacheKeyTemplate{subgraph: "products", prefix: "v1:products"}.render(astjson.MustParseBytes([]byte(`{"__typename":"Product","upc":"1"}`)))
		assert.False(t, ok)
		assert.Equal(t, itemKeys{}, keys)
	})
}

// TestCacheKeyLayout pins the key LAYOUT itself: the format version leads every
// store key, each subgraph owns its keyspace in both layers, the header hash
// varies the visible prefix and the hashed preimage together, and an L1-only
// fetch derives no store key at all.
func TestCacheKeyLayout(t *testing.T) {
	item := astjson.MustParseBytes([]byte(`{"__typename":"Product","upc":"1"}`))
	representation := `{"__typename":"Product","representation":{"upc":"1"}}`

	t.Run("the visible store key is version, subgraph, and the preimage digest", func(t *testing.T) {
		keys, ok := templateFor(t, productRepresentation(t, "upc")).render(item)
		require.True(t, ok)
		assert.Equal(t, "v1:products:"+hashHex([]byte("v1:products:"+representation)), keys.L2)
	})

	t.Run("the same entity in two subgraphs is two entries", func(t *testing.T) {
		spec := resolve.CacheKeySpec{Scope: resolve.CacheScopeEntity, TypeName: "Product", Representation: productRepresentation(t, "upc")}
		products, ok := newCacheKeyTemplate(nil, &resolve.FetchCacheConfig{SubgraphName: "products", KeySpec: spec}, 0, l2Access{enabled: true})
		require.True(t, ok)
		inventory, ok := newCacheKeyTemplate(nil, &resolve.FetchCacheConfig{SubgraphName: "inventory", KeySpec: spec}, 0, l2Access{enabled: true})
		require.True(t, ok)

		productsKeys, ok := products.render(item)
		require.True(t, ok)
		inventoryKeys, ok := inventory.render(item)
		require.True(t, ok)
		assert.Equal(t, itemKeys{
			L1: "products:" + representation,
			L2: "v1:products:" + hashHex([]byte("v1:products:"+representation)),
		}, productsKeys)
		assert.Equal(t, itemKeys{
			L1: "inventory:" + representation,
			L2: "v1:inventory:" + hashHex([]byte("v1:inventory:"+representation)),
		}, inventoryKeys)
	})

	t.Run("the header hash varies the prefix only when the config asks", func(t *testing.T) {
		spec := resolve.CacheKeySpec{Scope: resolve.CacheScopeEntity, TypeName: "Product", Representation: productRepresentation(t, "upc")}
		plain, ok := newCacheKeyTemplate(nil, &resolve.FetchCacheConfig{SubgraphName: "products", KeySpec: spec}, 42, l2Access{enabled: true})
		require.True(t, ok)
		varying, ok := newCacheKeyTemplate(nil, &resolve.FetchCacheConfig{SubgraphName: "products", IncludeSubgraphHeaders: true, KeySpec: spec}, 42, l2Access{enabled: true})
		require.True(t, ok)
		assert.Equal(t, "v1:products", plain.prefix)
		assert.Equal(t, "v1:products:h"+hex64(42), varying.prefix)

		varyingKeys, ok := varying.render(item)
		require.True(t, ok)
		// The header hash rides in the visible prefix AND in the hashed
		// preimage; the L1 key never carries it (the map is per-requester).
		assert.Equal(t, itemKeys{
			L1: "products:" + representation,
			L2: "v1:products:h" + hex64(42) + ":" + hashHex([]byte("v1:products:h"+hex64(42)+":"+representation)),
		}, varyingKeys)
	})

	t.Run("an L1-only fetch derives no store key and no prefix", func(t *testing.T) {
		spec := resolve.CacheKeySpec{Scope: resolve.CacheScopeEntity, TypeName: "Product", Representation: productRepresentation(t, "upc")}
		template, ok := newCacheKeyTemplate(nil, &resolve.FetchCacheConfig{SubgraphName: "products", IncludeSubgraphHeaders: true, KeySpec: spec}, 42, l2Access{})
		require.True(t, ok)
		assert.Equal(t, "", template.prefix)

		keys, ok := template.render(item)
		require.True(t, ok)
		assert.Equal(t, itemKeys{L1: "products:" + representation}, keys)
	})
}

// TestRootFieldCacheKeyLayout pins that root-field keys follow the entity
// layout — version, subgraph, digest — over their coordinate + variables
// preimage.
func TestRootFieldCacheKeyLayout(t *testing.T) {
	ctx := resolve.NewContext(t.Context())
	ctx.Variables = astjson.MustParseBytes([]byte(`{"first":5}`))
	cfg := &resolve.FetchCacheConfig{
		SubgraphName: "products",
		KeySpec:      resolve.CacheKeySpec{Scope: resolve.CacheScopeRootField, TypeName: "Query", FieldName: "topProducts"},
	}

	assert.Equal(t,
		"v1:products:"+hashHex([]byte(`v1:products:Query.topProducts:{"first":5}`)),
		rootFieldCacheKey(cfg, 0, ctx, ""),
	)
}

// TestControllerRequiresCoherence is the @requires coherence property at the
// controller level: a value derived from price 100 is cached under a key that
// INCLUDES price 100, so the same entity fetched with price 120 misses instead
// of serving a value computed from stale inputs.
func TestControllerRequiresCoherence(t *testing.T) {
	store := newTestStore()
	// The fetch sends @key(upc) plus the @requires field price.
	cfg := entityConfigForRepresentation(t, time.Minute, productRepresentation(t, "upc", "price"))

	cheapItem := astjson.MustParseBytes([]byte(`{"__typename":"Product","upc":"1","price":100}`))
	cheapKey := writeThrough(t, NewController(store, nil).BeginRequest(nil), cfg, cheapItem,
		`{"__typename":"Product","name":"Table","price":100}`)
	store.ops = nil // the second request's ops assert in isolation

	// Same entity, changed required input: a different key, hence a MISS.
	rc := NewController(store, nil).BeginRequest(nil)
	priceyItem := astjson.MustParseBytes([]byte(`{"__typename":"Product","upc":"1","price":120}`))
	decision, handle := prepare(t, rc, cfg, priceyItem)
	assert.Equal(t, resolve.DecisionFetch, decision)
	assert.Nil(t, handle.Items[0].FromCache)
	assert.NotEqual(t, cheapKey, handle.Items[0].RenderedKey)
	assert.Equal(t, []testStoreOp{
		{Kind: "GetMany", Keys: []string{handle.Items[0].RenderedKey}, Hits: []bool{false}},
	}, store.ops)

	// The unchanged inputs still hit their own entry.
	decision, _ = prepare(t, NewController(store, nil).BeginRequest(nil), cfg,
		astjson.MustParseBytes([]byte(`{"__typename":"Product","upc":"1","price":100}`)))
	assert.Equal(t, resolve.DecisionSkipFullHit, decision)
}
