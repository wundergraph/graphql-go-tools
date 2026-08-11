package cache

import (
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wundergraph/astjson"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

// l1Fetch runs one miss->fetch->result cycle on rc, feeding the L1 (and L2
// when enabled) from responseData.
func l1Fetch(t *testing.T, rc resolve.RequestCache, cfg *resolve.FetchCacheConfig, item *astjson.Value, responseData *astjson.Value) *resolve.FetchCacheHandle {
	t.Helper()
	decision, handle := prepare(t, rc, cfg, item)
	require.Equal(t, resolve.DecisionFetch, decision)
	require.NoError(t, rc.OnFetchResult(handle, resolve.MergeInput{
		Items:        []*astjson.Value{item},
		ResponseData: responseData,
		Arena:        beginner(),
	}))
	return handle
}

// TestControllerL1Rows covers the request-lifetime L1 store rows.
func TestControllerL1Rows(t *testing.T) {
	fresh := `{"__typename":"Product","name":"Table","price":100}`

	t.Run("in-request reuse: fetch A populates L1, fetch B hits with zero store ops (shared preimage)", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			store := newTestStore()
			cfg := entityConfig(t, time.Minute) // L1 + L2
			rc := NewController(store, nil).BeginRequest(nil)

			handleA := l1Fetch(t, rc, cfg, productItem(t, "1"), astjson.MustParseBytes([]byte(fresh)))

			itemB := productItem(t, "1")
			decision, handleB := prepare(t, rc, cfg, itemB)
			assert.Equal(t, resolve.DecisionSkipFullHit, decision)
			// SHARED PREIMAGE: both layers' keys come from one rendering per item.
			assert.Equal(t, handleA.Items[0].L1Key, handleB.Items[0].L1Key)
			assert.Equal(t, handleA.Items[0].RenderedKey, handleB.Items[0].RenderedKey)
			require.NoError(t, rc.OnFetchSkipped(handleB, resolve.MergeInput{
				Items: []*astjson.Value{itemB},
				Arena: beginner(),
			}))
			assert.Equal(t,
				`{"__typename":"Product","upc":"1","name":"Table","price":100}`,
				string(itemB.MarshalTo(nil)))
			rc.EndRequest()
			// A's miss read and A's flush write are the ONLY store ops: B's L1 hit
			// kept its key out of the batch read entirely (and a served hit owes no
			// write).
			key := handleA.Items[0].RenderedKey
			assert.Equal(t, []testStoreOp{
				{Kind: "GetMany", Keys: []string{key}, Hits: []bool{false}},
				{
					Kind: "SetMany",
					Items: []testStoreItem{
						{
							Key:   key,
							Value: `{"data":{"__typename":"Product","name":"Table","price":100},"cc":{"ttl":60,"created":946684800,"scope":"public"}}`,
							TTL:   time.Minute,
							Tags: []string{
								"subgraph:products",
								"type:products:Product",
								"entity:products:Product:d3cc039c7a9789e7", // upc "1"
							},
						},
					},
				},
			}, store.ops)
		})
	})

	t.Run("[J] L1-only: full round-trip with ZERO store ops (zero marshaling path)", func(t *testing.T) {
		store := newTestStore()
		cfg := entityConfig(t, 0) // TTL 0: L1 true, L2 false
		rc := NewController(store, nil).BeginRequest(nil)

		l1Fetch(t, rc, cfg, productItem(t, "1"), astjson.MustParseBytes([]byte(fresh)))
		itemB := productItem(t, "1")
		decision, handleB := prepare(t, rc, cfg, itemB)
		assert.Equal(t, resolve.DecisionSkipFullHit, decision)
		// An L1-only fetch keys off the RAW PREIMAGE and derives no store key
		// at all: no version segment, no hashing.
		assert.Equal(t, `products:{"__typename":"Product","representation":{"upc":"1"}}`, handleB.Items[0].L1Key)
		assert.Equal(t, "", handleB.Items[0].RenderedKey)
		require.NoError(t, rc.OnFetchSkipped(handleB, resolve.MergeInput{
			Items: []*astjson.Value{itemB},
			Arena: beginner(),
		}))
		rc.EndRequest()
		assert.Equal(t,
			`{"__typename":"Product","upc":"1","name":"Table","price":100}`,
			string(itemB.MarshalTo(nil)))
		assert.Empty(t, store.ops)
	})

	t.Run("write isolation: mutating the response after the write leaves L1 unaffected", func(t *testing.T) {
		cfg := entityConfig(t, 0)
		rc := NewController(newTestStore(), nil).BeginRequest(nil)
		responseData := astjson.MustParseBytes([]byte(fresh))
		l1Fetch(t, rc, cfg, productItem(t, "1"), responseData)

		// Corrupt the source AFTER the L1 write.
		responseData.Set(nil, "name", astjson.MustParseBytes([]byte(`"CORRUPTED"`)))

		itemB := productItem(t, "1")
		_, handleB := prepare(t, rc, cfg, itemB)
		require.NotNil(t, handleB.Items[0].FromCache)
		assert.Equal(t, fresh, string(handleB.Items[0].FromCache.MarshalTo(nil)))
	})

	t.Run("read isolation: mutating a served value leaves L1 unaffected", func(t *testing.T) {
		cfg := entityConfig(t, 0)
		rc := NewController(newTestStore(), nil).BeginRequest(nil)
		l1Fetch(t, rc, cfg, productItem(t, "1"), astjson.MustParseBytes([]byte(fresh)))

		itemB := productItem(t, "1")
		_, handleB := prepare(t, rc, cfg, itemB)
		require.NotNil(t, handleB.Items[0].FromCache)
		// Corrupt the SERVED copy.
		handleB.Items[0].FromCache.Set(nil, "name", astjson.MustParseBytes([]byte(`"CORRUPTED"`)))

		itemC := productItem(t, "1")
		_, handleC := prepare(t, rc, cfg, itemC)
		require.NotNil(t, handleC.Items[0].FromCache)
		assert.Equal(t, fresh, string(handleC.Items[0].FromCache.MarshalTo(nil)))
	})

	t.Run("an L2 hit populates L1: the second lookup emits no second store read", func(t *testing.T) {
		store := newTestStore()
		cfg := entityConfig(t, time.Minute)
		key := writeThrough(t, NewController(store, nil).BeginRequest(nil), cfg, productItem(t, "1"), fresh)
		store.ops = nil // request 2's ops assert in isolation

		rc := NewController(store, nil).BeginRequest(nil)
		itemA := productItem(t, "1")
		decisionA, _ := prepare(t, rc, cfg, itemA)
		require.Equal(t, resolve.DecisionSkipFullHit, decisionA)

		itemB := productItem(t, "1")
		decisionB, _ := prepare(t, rc, cfg, itemB)
		assert.Equal(t, resolve.DecisionSkipFullHit, decisionB)
		rc.EndRequest()
		// Fetch A's single L2 read is request 2's ONLY store op — fetch B rode L1.
		assert.Equal(t, []testStoreOp{
			{Kind: "GetMany", Keys: []string{key}, Hits: []bool{true}},
		}, store.ops)
	})

	t.Run("a DIFFERENT representation shape never rides the same L1 entry", func(t *testing.T) {
		store := newTestStore()
		// Both fetches resolve Product, but one sends {upc} and the other
		// {sku}: two independent representations, hence two independent
		// entries. The sku-keyed lookup must MISS what the upc-keyed fetch
		// cached, even though it is the same entity.
		upcCfg := entityConfig(t, 0)
		skuCfg := entityConfigForRepresentation(t, 0, productRepresentation(t, "sku"))
		rc := NewController(store, nil).BeginRequest(nil)

		l1Fetch(t, rc, upcCfg,
			astjson.MustParseBytes([]byte(`{"__typename":"Product","upc":"1","sku":"S1"}`)),
			astjson.MustParseBytes([]byte(fresh)))

		itemB := astjson.MustParseBytes([]byte(`{"__typename":"Product","upc":"1","sku":"S1"}`))
		decision, handleB := prepare(t, rc, skuCfg, itemB)
		assert.Equal(t, resolve.DecisionFetch, decision)
		assert.Nil(t, handleB.Items[0].FromCache)
		assert.Empty(t, store.ops)
	})

	t.Run("negative L1: an EmptyEntity result serves the next lookup as a negative hit", func(t *testing.T) {
		store := newTestStore()
		cfg := entityConfig(t, 0)
		// The coverage tree must allow a null entity for the sentinel path.
		cfg.ProvidesData = &resolve.Object{
			Nullable: true,
			Fields:   cfg.ProvidesData.Fields,
		}
		rc := NewController(store, nil).BeginRequest(nil)
		itemA := productItem(t, "404")
		decision, handleA := prepare(t, rc, cfg, itemA)
		require.Equal(t, resolve.DecisionFetch, decision)
		require.NoError(t, rc.OnFetchResult(handleA, resolve.MergeInput{
			Items:        []*astjson.Value{itemA},
			ResponseData: astjson.MustParseBytes([]byte(`null`)),
			EmptyEntity:  true,
			Arena:        beginner(),
		}))

		itemB := productItem(t, "404")
		decisionB, handleB := prepare(t, rc, cfg, itemB)
		assert.Equal(t, resolve.DecisionSkipFullHit, decisionB)
		assert.True(t, handleB.Items[0].NegativeHit)
		assert.Empty(t, store.ops)
	})

	t.Run("[H4] shadow stashes an L1-selected value too: read-never-serve stays absolute", func(t *testing.T) {
		store := newTestStore()
		cfg := shadowConfig(t) // L1 + L2 + shadow
		rc := NewController(store, nil).BeginRequest(nil)
		l1Fetch(t, rc, cfg, productItem(t, "1"), astjson.MustParseBytes([]byte(fresh)))

		itemB := productItem(t, "1")
		decision, handleB := prepare(t, rc, cfg, itemB)
		assert.Equal(t, resolve.DecisionFetchShadow, decision)
		assert.Nil(t, handleB.Items[0].FromCache)
		require.Contains(t, handleB.ShadowStash, 0)
		assert.Equal(t, fresh, string(handleB.ShadowStash[0].CachedValue.MarshalTo(nil)))
	})

	t.Run("no bleed across requests: a fresh request cache starts with an empty L1", func(t *testing.T) {
		store := newTestStore()
		cfg := entityConfig(t, 0)
		controller := NewController(store, nil)
		l1Fetch(t, controller.BeginRequest(nil), cfg, productItem(t, "1"), astjson.MustParseBytes([]byte(fresh)))

		// A NEW request (e.g. the next subscription event after clone): miss.
		decision, _ := prepare(t, controller.BeginRequest(nil), cfg, productItem(t, "1"))
		assert.Equal(t, resolve.DecisionFetch, decision)
	})
}
