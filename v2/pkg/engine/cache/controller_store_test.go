package cache

import (
	"context"
	"errors"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wundergraph/astjson"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

// storeFetchInput is prepareInput carrying the fetch item the controller reads
// the subgraph name from for store-failure reporting.
func storeFetchInput(cfg *resolve.FetchCacheConfig, items ...*astjson.Value) resolve.PrepareFetchInput {
	in := prepareInput(cfg, items...)
	in.Item = &resolve.FetchItem{
		Fetch: &resolve.EntityFetch{Info: &resolve.FetchInfo{DataSourceName: "inventory"}},
	}
	return in
}

// storeErrorObserver is the observer double for the store-failure rows: it
// records OnStoreError as the very StoreError the production observer keeps,
// and nothing else.
type storeErrorObserver struct {
	noopObserver

	storeErrors []StoreError
}

func (o *storeErrorObserver) OnStoreError(op string, subgraph string, keyCount int, err error) {
	o.storeErrors = append(o.storeErrors, StoreError{
		Op:       op,
		Subgraph: subgraph,
		KeyCount: keyCount,
		Err:      err,
	})
}

// ctxCapturingStore records the context each call ran under.
type ctxCapturingStore struct {
	getCtx context.Context
	setCtx context.Context
}

func (s *ctxCapturingStore) GetMany(ctx context.Context, keys []string) ([]Entry, error) {
	s.getCtx = ctx
	return make([]Entry, len(keys)), nil
}

func (s *ctxCapturingStore) SetMany(ctx context.Context, _ []Item) error {
	s.setCtx = ctx
	return nil
}

// misalignedStore answers with FEWER entries than it was asked keys — the
// contract violation that would otherwise serve values under the wrong keys.
type misalignedStore struct {
	entries []Entry
}

func (s *misalignedStore) GetMany(context.Context, []string) ([]Entry, error) {
	return s.entries, nil
}

func (s *misalignedStore) SetMany(context.Context, []Item) error { return nil }

// TestControllerStoreBatching pins the store call SHAPE: one read per fetch
// carrying every key that fetch still needs, one write per request carrying
// every deferred item, and no call at all when there is nothing to ask for.
func TestControllerStoreBatching(t *testing.T) {
	table := `{"__typename":"Product","name":"Table","price":100}`
	desk := `{"__typename":"Product","name":"Desk","price":80}`

	t.Run("a 3-entity batch issues ONE read carrying all three keys", func(t *testing.T) {
		store := newTestStore()
		cfg := entityConfig(t, time.Minute)
		rc := NewController(store, nil).BeginRequest(nil)

		buckets := [][]*astjson.Value{
			{productItem(t, "1")},
			{productItem(t, "2")},
			{productItem(t, "3")},
		}
		decision, handle := rc.PrepareFetch(batchInput(cfg, buckets...))
		require.Equal(t, resolve.DecisionFetch, decision)
		require.Len(t, handle.Items, 3)

		assert.Equal(t, []testStoreOp{
			{
				Kind: "GetMany",
				Keys: []string{
					handle.Items[0].RenderedKey,
					handle.Items[1].RenderedKey,
					handle.Items[2].RenderedKey,
				},
				Hits: []bool{false, false, false},
			},
		}, store.ops)
	})

	t.Run("positional entries: a mixed batch serves exactly the primed positions", func(t *testing.T) {
		store := newTestStore()
		cfg := entityConfig(t, time.Minute)
		// Buckets 0 and 2 are primed with DISTINCT values; bucket 1 is not, so a
		// misaligned answer could not pass unnoticed.
		key1 := writeThrough(t, NewController(store, nil).BeginRequest(nil), cfg, productItem(t, "1"), table)
		key3 := writeThrough(t, NewController(store, nil).BeginRequest(nil), cfg, productItem(t, "3"), desk)
		store.ops = nil // the mixed batch's ops assert in isolation

		rc := NewController(store, nil).BeginRequest(nil)
		buckets := [][]*astjson.Value{
			{productItem(t, "1")},
			{productItem(t, "2")},
			{productItem(t, "3")},
		}
		decision, handle := rc.PrepareFetch(batchInput(cfg, buckets...))
		// Full-batch semantics: ANY uncovered bucket refetches everything.
		require.Equal(t, resolve.DecisionFetch, decision)
		require.Len(t, handle.Items, 3)

		require.NotNil(t, handle.Items[0].FromCache)
		assert.Equal(t, table, string(handle.Items[0].FromCache.MarshalTo(nil)))
		assert.Nil(t, handle.Items[1].FromCache)
		require.NotNil(t, handle.Items[2].FromCache)
		assert.Equal(t, desk, string(handle.Items[2].FromCache.MarshalTo(nil)))

		assert.Equal(t, []testStoreOp{
			{
				Kind: "GetMany",
				Keys: []string{key1, handle.Items[1].RenderedKey, key3},
				Hits: []bool{true, false, true},
			},
		}, store.ops)
	})

	t.Run("a single-entity fetch reads exactly one key", func(t *testing.T) {
		store := newTestStore()
		cfg := entityConfig(t, time.Minute)
		rc := NewController(store, nil).BeginRequest(nil)

		decision, handle := prepare(t, rc, cfg, productItem(t, "1"))
		require.Equal(t, resolve.DecisionFetch, decision)
		require.Len(t, handle.Items, 1)
		assert.Equal(t, []testStoreOp{
			{
				Kind: "GetMany",
				Keys: []string{handle.Items[0].RenderedKey},
				Hits: []bool{false},
			},
		}, store.ops)
	})

	t.Run("a root-field fetch reads exactly one key", func(t *testing.T) {
		store := newTestStore()
		cfg := rootFieldConfig(time.Minute)
		rc := NewController(store, nil).BeginRequest(nil)

		decision, handle := prepare(t, rc, cfg, rootItem())
		require.Equal(t, resolve.DecisionFetch, decision)
		require.Len(t, handle.Items, 1)
		assert.Equal(t, []testStoreOp{
			{
				Kind: "GetMany",
				Keys: []string{handle.Items[0].RenderedKey},
				Hits: []bool{false},
			},
		}, store.ops)
	})

	t.Run("keys already served from L1 stay out of the batch read", func(t *testing.T) {
		store := newTestStore()
		cfg := entityConfig(t, time.Minute) // L1 + L2
		rc := NewController(store, nil).BeginRequest(nil)

		// Fetch A populates L1 (and defers its L2 write) for upc 1.
		item := productItem(t, "1")
		_, handleA := prepare(t, rc, cfg, item)
		require.NoError(t, rc.OnFetchResult(handleA, resolve.MergeInput{
			Items:        []*astjson.Value{item},
			ResponseData: astjson.MustParseBytes([]byte(table)),
			Arena:        beginner(),
		}))

		// The batch needs upc 1 and upc 2, but only upc 2 reaches the store.
		buckets := [][]*astjson.Value{{productItem(t, "1")}, {productItem(t, "2")}}
		_, handleB := rc.PrepareFetch(batchInput(cfg, buckets...))
		require.Len(t, handleB.Items, 2)
		require.NotNil(t, handleB.Items[0].FromCache)
		assert.Equal(t, "l1", handleB.Items[0].ServedFromLayer)
		assert.Nil(t, handleB.Items[1].FromCache)

		assert.Equal(t, []testStoreOp{
			{
				Kind: "GetMany",
				Keys: []string{handleA.Items[0].RenderedKey},
				Hits: []bool{false},
			},
			{
				Kind: "GetMany",
				Keys: []string{handleB.Items[1].RenderedKey},
				Hits: []bool{false},
			},
		}, store.ops)
	})

	t.Run("EndRequest writes every deferred item in ONE call with its own TTL", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			store := newTestStore()
			minuteCfg := entityConfig(t, time.Minute)
			halfMinuteCfg := entityConfig(t, 30*time.Second)
			negativeCfg := entityConfig(t, time.Minute)
			negativeCfg.NegativeCacheTTL = 5 * time.Second
			rc := NewController(store, nil).BeginRequest(nil)

			itemA := productItem(t, "1")
			_, handleA := rc.PrepareFetch(prepareInput(minuteCfg, itemA))
			require.NoError(t, rc.OnFetchResult(handleA, resolve.MergeInput{
				Items:        []*astjson.Value{itemA},
				ResponseData: astjson.MustParseBytes([]byte(table)),
				Arena:        beginner(),
			}))

			itemB := productItem(t, "2")
			_, handleB := rc.PrepareFetch(prepareInput(halfMinuteCfg, itemB))
			require.NoError(t, rc.OnFetchResult(handleB, resolve.MergeInput{
				Items:        []*astjson.Value{itemB},
				ResponseData: astjson.MustParseBytes([]byte(desk)),
				Arena:        beginner(),
			}))

			itemC := productItem(t, "404")
			_, handleC := rc.PrepareFetch(prepareInput(negativeCfg, itemC))
			require.NoError(t, rc.OnFetchResult(handleC, resolve.MergeInput{
				Items:        []*astjson.Value{itemC},
				ResponseData: astjson.MustParseBytes([]byte(`null`)),
				EmptyEntity:  true,
				Arena:        beginner(),
			}))

			rc.EndRequest()
			assert.Equal(t, []testStoreOp{
				{
					Kind: "GetMany",
					Keys: []string{handleA.Items[0].RenderedKey},
					Hits: []bool{false},
				},
				{
					Kind: "GetMany",
					Keys: []string{handleB.Items[0].RenderedKey},
					Hits: []bool{false},
				},
				{
					Kind: "GetMany",
					Keys: []string{handleC.Items[0].RenderedKey},
					Hits: []bool{false},
				},
				{
					Kind: "SetMany",
					Items: []testStoreItem{
						{
							Key:   handleA.Items[0].RenderedKey,
							Value: `{"data":{"__typename":"Product","name":"Table","price":100},"cc":{"ttl":60,"created":946684800,"scope":"public"}}`,
							TTL:   time.Minute,
							Tags: []string{
								"subgraph:products",
								"type:products:Product",
								"entity:products:Product:d3cc039c7a9789e7", // upc "1"
							},
						},
						{
							Key:   handleB.Items[0].RenderedKey,
							Value: `{"data":{"__typename":"Product","name":"Desk","price":80},"cc":{"ttl":30,"created":946684800,"scope":"public"}}`,
							TTL:   30 * time.Second,
							Tags: []string{
								"subgraph:products",
								"type:products:Product",
								"entity:products:Product:4f93140518d68e67", // upc "2"
							},
						},
						{
							Key:   handleC.Items[0].RenderedKey,
							Value: `{"data":null,"cc":{"ttl":5,"created":946684800,"scope":"public"}}`,
							TTL:   5 * time.Second,
							Tags: []string{
								"subgraph:products",
								"type:products:Product",
								"entity:products:Product:0256cf786a3c1be7", // upc "404"
							},
						},
					},
				},
			}, store.ops)
		})
	})

	t.Run("both store calls run under the request's own context", func(t *testing.T) {
		store := &ctxCapturingStore{}
		cfg := entityConfig(t, time.Minute)
		ctx := resolve.NewContext(t.Context())
		rc := NewController(store, nil).BeginRequest(ctx)

		item := productItem(t, "1")
		_, handle := prepare(t, rc, cfg, item)
		require.NoError(t, rc.OnFetchResult(handle, resolve.MergeInput{
			Items:        []*astjson.Value{item},
			ResponseData: astjson.MustParseBytes([]byte(table)),
			Arena:        beginner(),
		}))
		rc.EndRequest()

		assert.Equal(t, t.Context(), store.getCtx)
		assert.Equal(t, t.Context(), store.setCtx)
	})

	t.Run("a controller without a request context falls back to the background context", func(t *testing.T) {
		store := &ctxCapturingStore{}
		cfg := entityConfig(t, time.Minute)
		rc := NewController(store, nil).BeginRequest(nil)

		item := productItem(t, "1")
		_, handle := prepare(t, rc, cfg, item)
		require.NoError(t, rc.OnFetchResult(handle, resolve.MergeInput{
			Items:        []*astjson.Value{item},
			ResponseData: astjson.MustParseBytes([]byte(table)),
			Arena:        beginner(),
		}))
		rc.EndRequest()

		assert.Equal(t, context.Background(), store.getCtx)
		assert.Equal(t, context.Background(), store.setCtx)
	})

	t.Run("a fetch with nothing to look up issues no read at all", func(t *testing.T) {
		store := newTestStore()
		cfg := entityConfig(t, time.Minute)
		rc := NewController(store, nil).BeginRequest(nil)

		// An unrenderable key (no upc) leaves the batch read empty.
		unrenderable := astjson.MustParseBytes([]byte(`{"__typename":"Product"}`))
		_, handle := prepare(t, rc, cfg, unrenderable)
		require.NotNil(t, handle)
		assert.Equal(t, "", handle.Items[0].RenderedKey)
		assert.Empty(t, store.ops)

		// A batch whose every bucket is already served by L1 does the same.
		item := productItem(t, "1")
		_, warmup := prepare(t, rc, cfg, item)
		require.NoError(t, rc.OnFetchResult(warmup, resolve.MergeInput{
			Items:        []*astjson.Value{item},
			ResponseData: astjson.MustParseBytes([]byte(table)),
			Arena:        beginner(),
		}))
		store.ops = nil // the all-L1 batch's ops assert in isolation

		buckets := [][]*astjson.Value{{productItem(t, "1")}, {productItem(t, "1")}}
		decision, l1Handle := rc.PrepareFetch(batchInput(cfg, buckets...))
		assert.Equal(t, resolve.DecisionSkipFullHit, decision)
		assert.Equal(t, "l1", l1Handle.Items[0].ServedFromLayer)
		assert.Equal(t, "l1", l1Handle.Items[1].ServedFromLayer)
		assert.Empty(t, store.ops)
	})
}

// TestControllerStoreFailures pins the degradation contract: a failed read is
// a miss for every key it carried, a failed write is dropped, neither fails the
// request, and each failure is reported to the observer exactly once.
func TestControllerStoreFailures(t *testing.T) {
	table := `{"__typename":"Product","name":"Table","price":100}`

	t.Run("a failed read misses every key, refetches, and still writes back", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			store := newTestStore()
			cfg := entityConfig(t, time.Minute)
			// Both entities ARE cached: only the injected failure hides them.
			key1 := writeThrough(t, NewController(store, nil).BeginRequest(nil), cfg, productItem(t, "1"), table)
			key2 := writeThrough(t, NewController(store, nil).BeginRequest(nil), cfg, productItem(t, "2"), `{"__typename":"Product","name":"Desk","price":80}`)
			store.ops = nil // the failing request's ops assert in isolation

			readErr := errors.New("store read down")
			store.failGetMany, store.failGetManyErr = 1, readErr
			obs := &storeErrorObserver{}
			rc := NewController(store, obs).BeginRequest(nil)

			buckets := [][]*astjson.Value{{productItem(t, "1")}, {productItem(t, "2")}}
			in := batchInput(cfg, buckets...)
			in.Item = &resolve.FetchItem{
				Fetch: &resolve.EntityFetch{Info: &resolve.FetchInfo{DataSourceName: "inventory"}},
			}
			decision, handle := rc.PrepareFetch(in)
			// Every key of the failed call is a miss, so the fetch proceeds.
			assert.Equal(t, resolve.DecisionFetch, decision)
			assert.False(t, handle.WasHit)
			assert.Nil(t, handle.Items[0].FromCache)
			assert.Nil(t, handle.Items[1].FromCache)

			// ONE event for the whole failed call, naming the fetch's subgraph.
			assert.Equal(t, []StoreError{
				{Op: "GetMany", Subgraph: "inventory", KeyCount: 2, Err: readErr},
			}, obs.storeErrors)

			// The refetch's write-back is unaffected by the failed read.
			require.NoError(t, rc.OnFetchResult(handle, resolve.MergeInput{
				BatchStats:   buckets,
				ResponseData: astjson.MustParseBytes([]byte(`[` + table + `,{"__typename":"Product","name":"Desk","price":80}]`)),
				Arena:        beginner(),
			}))
			rc.EndRequest()
			assert.Equal(t, []testStoreOp{
				{
					Kind:   "GetMany",
					Keys:   []string{key1, key2},
					Failed: true,
				},
				{
					Kind: "SetMany",
					Items: []testStoreItem{
						{
							Key:   key1,
							Value: `{"data":{"__typename":"Product","name":"Table","price":100},"cc":{"ttl":60,"created":946684800,"scope":"public"}}`,
							TTL:   time.Minute,
							Tags: []string{
								"subgraph:products",
								"type:products:Product",
								"entity:products:Product:d3cc039c7a9789e7", // upc "1"
							},
						},
						{
							Key:   key2,
							Value: `{"data":{"__typename":"Product","name":"Desk","price":80},"cc":{"ttl":60,"created":946684800,"scope":"public"}}`,
							TTL:   time.Minute,
							Tags: []string{
								"subgraph:products",
								"type:products:Product",
								"entity:products:Product:4f93140518d68e67", // upc "2"
							},
						},
					},
				},
			}, store.ops)
		})
	})

	t.Run("an answer that does not align with the keys is refused wholesale", func(t *testing.T) {
		// The store answers ONE entry for TWO keys: accepting it would serve
		// entity 1's value for entity 2.
		store := &misalignedStore{
			entries: []Entry{
				{Value: []byte(table), OK: true},
			},
		}
		cfg := entityConfig(t, time.Minute)
		obs := &storeErrorObserver{}
		rc := NewController(store, obs).BeginRequest(nil)

		buckets := [][]*astjson.Value{{productItem(t, "1")}, {productItem(t, "2")}}
		decision, handle := rc.PrepareFetch(batchInput(cfg, buckets...))
		assert.Equal(t, resolve.DecisionFetch, decision)
		assert.Nil(t, handle.Items[0].FromCache)
		assert.Nil(t, handle.Items[1].FromCache)
		assert.Equal(t, []StoreError{
			{
				Op:       "GetMany",
				Subgraph: "",
				KeyCount: 2,
				Err:      errors.New("store returned 1 entries for 2 keys"),
			},
		}, obs.storeErrors)
	})

	t.Run("a failed write is dropped without failing the request", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			store := newTestStore()
			cfg := entityConfig(t, time.Minute)
			writeErr := errors.New("store write down")
			store.failSetMany, store.failSetManyErr = 1, writeErr
			obs := &storeErrorObserver{}
			rc := NewController(store, obs).BeginRequest(nil)

			item := productItem(t, "1")
			_, handle := rc.PrepareFetch(storeFetchInput(cfg, item))
			require.NoError(t, rc.OnFetchResult(handle, resolve.MergeInput{
				Items:        []*astjson.Value{item},
				ResponseData: astjson.MustParseBytes([]byte(table)),
				Arena:        beginner(),
			}))
			// The item is served from the response either way; the merge target
			// carries the fetched value untouched by the store failure.
			rc.EndRequest()

			key := handle.Items[0].RenderedKey
			_, stored := store.value(key)
			assert.False(t, stored)
			// The flush spans every fetch of the request, so it names no subgraph.
			assert.Equal(t, []StoreError{
				{Op: "SetMany", Subgraph: "", KeyCount: 1, Err: writeErr},
			}, obs.storeErrors)
			assert.Equal(t, []testStoreOp{
				{
					Kind: "GetMany",
					Keys: []string{key},
					Hits: []bool{false},
				},
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
					Failed: true,
				},
			}, store.ops)
		})
	})

	t.Run("a nil observer swallows failures at zero cost", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			store := newTestStore()
			cfg := entityConfig(t, time.Minute)
			store.failGetMany, store.failGetManyErr = 1, errors.New("store read down")
			store.failSetMany, store.failSetManyErr = 1, errors.New("store write down")
			rc := NewController(store, nil).BeginRequest(nil)

			item := productItem(t, "1")
			decision, handle := prepare(t, rc, cfg, item)
			assert.Equal(t, resolve.DecisionFetch, decision)
			require.NoError(t, rc.OnFetchResult(handle, resolve.MergeInput{
				Items:        []*astjson.Value{item},
				ResponseData: astjson.MustParseBytes([]byte(table)),
				Arena:        beginner(),
			}))
			rc.EndRequest()

			key := handle.Items[0].RenderedKey
			assert.Equal(t, []testStoreOp{
				{
					Kind:   "GetMany",
					Keys:   []string{key},
					Failed: true,
				},
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
					Failed: true,
				},
			}, store.ops)
		})
	})
}
