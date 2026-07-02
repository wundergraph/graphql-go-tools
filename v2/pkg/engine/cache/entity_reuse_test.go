package cache

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wundergraph/astjson"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astparser"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

const reuseDefinition = `
	scalar String
	scalar ID

	type Query {
		product(upc: String!): Product
		productByName(name: String!): Product
		products(first: String): [Product!]!
	}

	type Product {
		upc: String!
		sku: String!
		name: String!
	}
`

func parseReuseDefinition(t *testing.T) *ast.Document {
	t.Helper()
	definition, report := astparser.ParseGraphqlDocumentString(reuseDefinition)
	require.False(t, report.HasErrors(), "parse definition: %v", report)
	return &definition
}

func reuseRootFieldInfo(fieldName string) *resolve.FetchInfo {
	return &resolve.FetchInfo{
		DataSourceID: "products",
		RootFields:   []resolve.GraphCoordinate{{TypeName: "Query", FieldName: fieldName}},
	}
}

// TestBuildRootFieldSpecEntityKeyMappings covers the builder rows: mappings
// derived structurally from definition + federation, entity candidates frozen
// with the type-name fallback, and the no-mapping cases.
func TestBuildRootFieldSpecEntityKeyMappings(t *testing.T) {
	builder := &cacheKeyBuilder{
		federation: newKeyBuilder(t, parseReuseDefinition(t), newKeyBuilderFederation(t, "upc", "sku")).federation,
		definition: parseReuseDefinition(t),
	}

	t.Run("by-key root field gets the mapping and the FULL entity candidate set", func(t *testing.T) {
		spec, ok := builder.buildRootFieldSpec(reuseRootFieldInfo("product"))
		require.True(t, ok)
		assert.Equal(t, []resolve.EntityKeyMapping{
			{
				EntityTypeName: "Product",
				FieldMappings: []resolve.EntityFieldMapping{
					{EntityKeyField: "upc", ArgumentPath: []string{"upc"}, ArgumentIsEntityKey: true},
				},
			},
		}, spec.EntityKeyMappings)
		// BOTH entity candidates are frozen (sorted: sku first), each carrying
		// the entity type name for arg-derived rendering.
		require.Len(t, spec.Candidates, 2)
		assert.Equal(t, "Product", spec.Candidates[0].Representation.TypeName)
		assert.Equal(t, "Product", spec.Candidates[1].Representation.TypeName)
		assert.Equal(t, resolve.CacheScopeRootField, spec.Scope)
		assert.Equal(t, "Query", spec.TypeName)
		assert.Equal(t, "product", spec.FieldName)
	})

	t.Run("arguments not covering any key set derive no mapping", func(t *testing.T) {
		spec, ok := builder.buildRootFieldSpec(reuseRootFieldInfo("productByName"))
		require.True(t, ok)
		assert.Nil(t, spec.EntityKeyMappings)
		assert.Nil(t, spec.Candidates)
	})

	t.Run("non-entity return types derive no mapping", func(t *testing.T) {
		spec, ok := builder.buildRootFieldSpec(&resolve.FetchInfo{
			DataSourceID: "unknown",
			RootFields:   []resolve.GraphCoordinate{{TypeName: "Query", FieldName: "product"}},
		})
		require.True(t, ok)
		assert.Nil(t, spec.EntityKeyMappings)
	})

	t.Run("multi-root-field fetches derive no mapping", func(t *testing.T) {
		info := reuseRootFieldInfo("product")
		info.RootFields = append(info.RootFields, resolve.GraphCoordinate{TypeName: "Query", FieldName: "products"})
		spec, ok := builder.buildRootFieldSpec(info)
		require.True(t, ok)
		assert.Nil(t, spec.EntityKeyMappings)
	})
}

// reuseConfig builds the by-key root-field config through the REAL builder.
func reuseConfig(t *testing.T) *resolve.FetchCacheConfig {
	t.Helper()
	builder := &cacheKeyBuilder{
		federation: newKeyBuilder(t, parseReuseDefinition(t), newKeyBuilderFederation(t, "upc", "sku")).federation,
		definition: parseReuseDefinition(t),
	}
	spec, ok := builder.buildRootFieldSpec(reuseRootFieldInfo("product"))
	require.True(t, ok)
	require.Len(t, spec.Candidates, 2)
	cfg := &resolve.FetchCacheConfig{
		L2:        true,
		CacheName: "entities", // SHARED with the entity policy: read key == write key
		TTL:       time.Minute,
		KeySpec:   spec,
		ProvidesData: &resolve.Object{
			Fields: []*resolve.Field{
				{
					Name: []byte("product"),
					Value: &resolve.Object{
						Nullable: true,
						Path:     []string{"product"},
						Fields: []*resolve.Field{
							{Name: []byte("name"), Value: &resolve.Scalar{Nullable: false, Path: []string{"name"}}},
						},
					},
				},
			},
		},
	}
	resolve.ComputeHasAliases(cfg.ProvidesData)
	return cfg
}

// entityWriteKeys derives the entity-space keys (sku, upc) exactly as an
// entity fetch renders them, for cross-checking read key == write key.
func entityWriteKeys(t *testing.T, cacheName string, item string) (string, string) {
	t.Helper()
	cfg := multiKeyConfig(t)
	cfg.CacheName = cacheName
	rc := NewController(newTestStore(), nil).BeginRequest(nil)
	_, handle := prepare(t, rc, cfg, astjson.MustParseBytes([]byte(item)))
	require.Len(t, handle.Items[0].RenderedKeys, 2)
	return handle.Items[0].RenderedKeys[0], handle.Items[0].RenderedKeys[1]
}

// TestEntityReuseRuntimeRows covers E3/E5 for by-key root fields.
func TestEntityReuseRuntimeRows(t *testing.T) {
	t.Run("[E3] lookup renders ONLY the arg-derived candidate; data-derived key backfills", func(t *testing.T) {
		skuKey, upcKey := entityWriteKeys(t, "entities", `{"__typename":"Product","upc":"1","sku":"S1"}`)
		store := newTestStore()
		ctx := variableContext(t, `{"upc":"1"}`)
		rc := NewController(store, nil).BeginRequest(ctx)
		cfg := reuseConfig(t)

		dataRoot := astjson.MustParseBytes([]byte(`{}`))
		decision, handle := prepare(t, rc, cfg, dataRoot)
		require.Equal(t, resolve.DecisionFetch, decision)
		// Only the upc candidate rendered from the argument; sku is pending.
		assert.Equal(t, []string{upcKey}, handle.Items[0].RenderedKeys)
		require.Len(t, handle.Items[0].PendingCandidates, 1)
		assert.Equal(t, []string{"product"}, handle.Items[0].EntityMergePath)

		// The response carries sku, so the pending candidate backfills.
		require.NoError(t, rc.OnFetchResult(handle, resolve.MergeInput{
			Items:        []*astjson.Value{dataRoot},
			ResponseData: astjson.MustParseBytes([]byte(`{"product":{"__typename":"Product","name":"Table","sku":"S1"}}`)),
			Arena:        beginner(),
		}))
		rc.EndRequest()
		assert.Equal(t, []testStoreOp{
			{Kind: "Get", Key: upcKey},
			{Kind: "Set", Key: upcKey, Value: `{"__typename":"Product","name":"Table","sku":"S1"}`, TTL: time.Minute, Reason: resolve.CacheWriteReasonRefresh},
			{Kind: "Set", Key: skuKey, Value: `{"__typename":"Product","name":"Table","sku":"S1"}`, TTL: time.Minute, Reason: resolve.CacheWriteReasonBackfill},
		}, store.ops)
	})

	t.Run("a hit on the entity entry serves the root field at its response key", func(t *testing.T) {
		_, upcKey := entityWriteKeys(t, "entities", `{"__typename":"Product","upc":"1","sku":"S1"}`)
		store := newTestStore()
		// Primed exactly as an entity fetch would write it.
		store.seed(upcKey, []byte(`{"__typename":"Product","name":"Table"}`), time.Minute)

		ctx := variableContext(t, `{"upc":"1"}`)
		rc := NewController(store, nil).BeginRequest(ctx)
		cfg := reuseConfig(t)
		dataRoot := astjson.MustParseBytes([]byte(`{}`))
		decision, handle := prepare(t, rc, cfg, dataRoot)
		require.Equal(t, resolve.DecisionSkipFullHit, decision)

		require.NoError(t, rc.OnFetchSkipped(handle, resolve.MergeInput{
			Items: []*astjson.Value{dataRoot},
			Arena: beginner(),
		}))
		assert.Equal(t, `{"product":{"name":"Table","__typename":"Product"}}`, string(dataRoot.MarshalTo(nil)))
	})

	t.Run("[E5] read-hit backfill: the pending key renders from the served entity value", func(t *testing.T) {
		skuKey, upcKey := entityWriteKeys(t, "entities", `{"__typename":"Product","upc":"1","sku":"S1"}`)
		store := newTestStore()
		store.seed(upcKey, []byte(`{"__typename":"Product","name":"Table","sku":"S1"}`), time.Minute)

		ctx := variableContext(t, `{"upc":"1"}`)
		rc := NewController(store, nil).BeginRequest(ctx)
		cfg := reuseConfig(t)
		dataRoot := astjson.MustParseBytes([]byte(`{}`))
		decision, handle := prepare(t, rc, cfg, dataRoot)
		require.Equal(t, resolve.DecisionSkipFullHit, decision)
		assert.True(t, handle.MustWriteBack)

		require.NoError(t, rc.OnFetchSkipped(handle, resolve.MergeInput{
			Items: []*astjson.Value{dataRoot},
			Arena: beginner(),
		}))
		rc.EndRequest()
		assert.Equal(t, []testStoreOp{
			{Kind: "Get", Key: upcKey},
			{Kind: "Set", Key: skuKey, Value: `{"__typename":"Product","name":"Table","sku":"S1"}`, TTL: time.Minute, Reason: resolve.CacheWriteReasonBackfill},
		}, store.ops)
	})

	t.Run("a missing argument variable falls back to a plain fetch and backfills post-response", func(t *testing.T) {
		store := newTestStore()
		rc := NewController(store, nil).BeginRequest(variableContext(t, `{}`))
		cfg := reuseConfig(t)
		dataRoot := astjson.MustParseBytes([]byte(`{}`))
		decision, handle := prepare(t, rc, cfg, dataRoot)
		require.Equal(t, resolve.DecisionFetch, decision)
		// Nothing renderable at lookup: no Gets at all, both candidates pending.
		assert.Nil(t, handle.Items[0].RenderedKeys)
		assert.Len(t, handle.Items[0].PendingCandidates, 2)
		assert.Empty(t, store.ops)
	})
}
