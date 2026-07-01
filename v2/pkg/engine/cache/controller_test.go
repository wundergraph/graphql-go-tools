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

// testStore is the minimal in-memory Store for controller unit tests, with an
// ordered op log mirroring cachetesting.FakeStore (which cannot be imported
// here: cachetesting imports this package).
type testStore struct {
	data map[string]testStoreEntry
	ops  []testStoreOp
}

type testStoreEntry struct {
	value     []byte
	expiresAt time.Time
}

type testStoreOp struct {
	Kind  string
	Key   string
	Value string
	TTL   time.Duration
}

func newTestStore() *testStore {
	return &testStore{data: map[string]testStoreEntry{}}
}

func (s *testStore) Get(key string) ([]byte, time.Duration, bool) {
	s.ops = append(s.ops, testStoreOp{Kind: "Get", Key: key})
	entry, ok := s.data[key]
	if !ok || !time.Now().Before(entry.expiresAt) {
		return nil, 0, false
	}
	return append([]byte(nil), entry.value...), time.Until(entry.expiresAt), true
}

func (s *testStore) Set(key string, value []byte, ttl time.Duration) {
	s.data[key] = testStoreEntry{value: append([]byte(nil), value...), expiresAt: time.Now().Add(ttl)}
	s.ops = append(s.ops, testStoreOp{Kind: "Set", Key: key, Value: string(value), TTL: ttl})
}

// productProvidesData is the coverage tree used across the controller rows:
// name non-nullable, price nullable.
func productProvidesData() *resolve.Object {
	return &resolve.Object{
		Fields: []*resolve.Field{
			{
				Name:        []byte("name"),
				Value:       &resolve.Scalar{Nullable: false, Path: []string{"name"}},
				OnTypeNames: [][]byte{[]byte("Product")},
			},
			{
				Name:        []byte("price"),
				Value:       &resolve.Scalar{Nullable: true, Path: []string{"price"}},
				OnTypeNames: [][]byte{[]byte("Product")},
			},
		},
	}
}

// entityConfig builds a single-candidate ("upc") entity config over the shared
// key-builder fixture.
func entityConfig(t *testing.T, ttl time.Duration) *resolve.FetchCacheConfig {
	t.Helper()
	builder := newKeyBuilder(t, parseKeyBuilderDefinition(t), newKeyBuilderFederation(t, "upc"))
	spec, ok := builder.buildEntitySpec(productEntityInfo())
	require.True(t, ok)
	return &resolve.FetchCacheConfig{
		L1:           true,
		L2:           ttl > 0,
		CacheName:    "entities",
		TTL:          ttl,
		KeySpec:      spec,
		ProvidesData: productProvidesData(),
	}
}

func beginner() resolve.TransactionBeginner {
	return resolve.NewTransactionBeginner(nil, &resolve.DataBuffer{})
}

func prepareInput(cfg *resolve.FetchCacheConfig, items ...*astjson.Value) resolve.PrepareFetchInput {
	return resolve.PrepareFetchInput{
		Items:  items,
		Config: cfg,
		Input:  []byte(`{"body":{"query":"..."}}`),
		Arena:  beginner(),
	}
}

func productItem(t *testing.T, upc string) *astjson.Value {
	t.Helper()
	return astjson.MustParseBytes([]byte(`{"__typename":"Product","upc":"` + upc + `"}`))
}

// prepare runs one PrepareFetch and returns decision + handle.
func prepare(t *testing.T, rc resolve.RequestCache, cfg *resolve.FetchCacheConfig, items ...*astjson.Value) (resolve.Decision, *resolve.FetchCacheHandle) {
	t.Helper()
	return rc.PrepareFetch(prepareInput(cfg, items...))
}

// writeThrough runs the miss → fetch-result → flush cycle for one item and
// returns the key it wrote under.
func writeThrough(t *testing.T, rc resolve.RequestCache, cfg *resolve.FetchCacheConfig, item *astjson.Value, responseData string) string {
	t.Helper()
	decision, handle := prepare(t, rc, cfg, item)
	require.Equal(t, resolve.DecisionFetch, decision)
	require.NotNil(t, handle)
	require.Len(t, handle.Items, 1)
	require.Len(t, handle.Items[0].RenderedKeys, 1)
	require.NoError(t, rc.OnFetchResult(handle, resolve.MergeInput{
		Items:        []*astjson.Value{item},
		ResponseData: astjson.MustParseBytes([]byte(responseData)),
		Arena:        beginner(),
	}))
	rc.EndRequest()
	return handle.Items[0].RenderedKeys[0]
}

// TestControllerDecisionRows covers the D core rows: full hit, miss, coverage
// failure (stale partial NOT served), nullability, and ProvidesData == nil.
func TestControllerDecisionRows(t *testing.T) {
	newRC := func(store Store) resolve.RequestCache {
		return NewController(store, nil).BeginRequest(nil)
	}

	t.Run("[D1] full hit: covering value skips the fetch", func(t *testing.T) {
		store := newTestStore()
		cfg := entityConfig(t, time.Minute)
		key := writeThrough(t, newRC(store), cfg, productItem(t, "1"), `{"__typename":"Product","name":"Table","price":100}`)

		decision, handle := prepare(t, newRC(store), cfg, productItem(t, "1"))
		require.NotNil(t, handle)
		assert.Equal(t, resolve.DecisionSkipFullHit, decision)
		assert.Equal(t, resolve.DecisionSkipFullHit, handle.Decision)
		assert.True(t, handle.WasHit)
		require.Len(t, handle.Items, 1)
		// Key fidelity (O row): the read key IS the write key.
		assert.Equal(t, []string{key}, handle.Items[0].RenderedKeys)
		require.NotNil(t, handle.Items[0].FromCache)
		assert.Equal(t, `{"__typename":"Product","name":"Table","price":100}`, string(handle.Items[0].FromCache.MarshalTo(nil)))
	})

	t.Run("[D2] miss: empty store fetches", func(t *testing.T) {
		store := newTestStore()
		cfg := entityConfig(t, time.Minute)
		decision, handle := prepare(t, newRC(store), cfg, productItem(t, "1"))
		require.NotNil(t, handle)
		assert.Equal(t, resolve.DecisionFetch, decision)
		assert.False(t, handle.WasHit)
		require.Len(t, handle.Items, 1)
		assert.Nil(t, handle.Items[0].FromCache)
		assert.Nil(t, handle.Items[0].FromCacheCandidates)
	})

	t.Run("[D3] coverage failure: stale partial value is NOT served", func(t *testing.T) {
		store := newTestStore()
		cfg := entityConfig(t, time.Minute)
		// The cached value lacks the non-nullable "name" field.
		key := writeThrough(t, newRC(store), cfg, productItem(t, "1"), `{"__typename":"Product","price":100}`)

		decision, handle := prepare(t, newRC(store), cfg, productItem(t, "1"))
		require.NotNil(t, handle)
		assert.Equal(t, resolve.DecisionFetch, decision)
		require.Len(t, handle.Items, 1)
		assert.Nil(t, handle.Items[0].FromCache)
		// The candidate WAS found — coverage rejected it.
		assert.Equal(t, []resolve.CacheCandidate{{
			Value:        []byte(`{"__typename":"Product","price":100}`),
			RemainingTTL: handle.Items[0].FromCacheCandidates[0].RemainingTTL,
		}}, handle.Items[0].FromCacheCandidates)
		assert.Equal(t, []string{key}, handle.Items[0].RenderedKeys)
	})

	t.Run("[D4] null accepted where nullable", func(t *testing.T) {
		store := newTestStore()
		cfg := entityConfig(t, time.Minute)
		writeThrough(t, newRC(store), cfg, productItem(t, "1"), `{"__typename":"Product","name":"Table","price":null}`)

		decision, handle := prepare(t, newRC(store), cfg, productItem(t, "1"))
		require.NotNil(t, handle)
		assert.Equal(t, resolve.DecisionSkipFullHit, decision)
	})

	t.Run("[D5] null rejected where non-nullable", func(t *testing.T) {
		store := newTestStore()
		cfg := entityConfig(t, time.Minute)
		writeThrough(t, newRC(store), cfg, productItem(t, "1"), `{"__typename":"Product","name":null,"price":100}`)

		decision, handle := prepare(t, newRC(store), cfg, productItem(t, "1"))
		require.NotNil(t, handle)
		assert.Equal(t, resolve.DecisionFetch, decision)
		assert.Nil(t, handle.Items[0].FromCache)
	})

	t.Run("[D14] ProvidesData nil disables the coverage walk: always a miss", func(t *testing.T) {
		store := newTestStore()
		cfg := entityConfig(t, time.Minute)
		writeThrough(t, newRC(store), cfg, productItem(t, "1"), `{"__typename":"Product","name":"Table","price":100}`)

		noProvides := entityConfig(t, time.Minute)
		noProvides.ProvidesData = nil
		decision, handle := prepare(t, newRC(store), noProvides, productItem(t, "1"))
		require.NotNil(t, handle)
		assert.Equal(t, resolve.DecisionFetch, decision)
		assert.Nil(t, handle.Items[0].FromCache)
	})

	t.Run("[D] partial hit degrades to fetch (AND-reduction)", func(t *testing.T) {
		store := newTestStore()
		cfg := entityConfig(t, time.Minute)
		writeThrough(t, newRC(store), cfg, productItem(t, "1"), `{"__typename":"Product","name":"Table","price":100}`)

		decision, handle := prepare(t, newRC(store), cfg, productItem(t, "1"), productItem(t, "2"))
		require.NotNil(t, handle)
		assert.Equal(t, resolve.DecisionFetch, decision)
		require.Len(t, handle.Items, 2)
		assert.NotNil(t, handle.Items[0].FromCache)
		assert.Nil(t, handle.Items[1].FromCache)
	})

	t.Run("[L] malformed cached bytes are a miss, never a panic", func(t *testing.T) {
		store := newTestStore()
		cfg := entityConfig(t, time.Minute)
		key := writeThrough(t, newRC(store), cfg, productItem(t, "1"), `{"__typename":"Product","name":"Table","price":100}`)
		store.data[key] = testStoreEntry{value: []byte(`{not json`), expiresAt: time.Now().Add(time.Hour)}

		decision, handle := prepare(t, newRC(store), cfg, productItem(t, "1"))
		require.NotNil(t, handle)
		assert.Equal(t, resolve.DecisionFetch, decision)
		assert.Nil(t, handle.Items[0].FromCache)
		assert.Nil(t, handle.Items[0].FromCacheCandidates)
	})

	t.Run("[F/TTL] a written value expires exactly after its TTL", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			store := newTestStore()
			cfg := entityConfig(t, time.Minute)
			writeThrough(t, newRC(store), cfg, productItem(t, "1"), `{"__typename":"Product","name":"Table","price":100}`)

			decision, _ := prepare(t, newRC(store), cfg, productItem(t, "1"))
			assert.Equal(t, resolve.DecisionSkipFullHit, decision)

			time.Sleep(time.Minute + time.Second)
			decision, _ = prepare(t, newRC(store), cfg, productItem(t, "1"))
			assert.Equal(t, resolve.DecisionFetch, decision)
		})
	})
}

// TestControllerWriteGateRows covers the F rows: only a clean fetch writes;
// transport/empty/parse failures, GraphQL errors, null data, status fallback,
// and empty entities all produce ZERO writes.
func TestControllerWriteGateRows(t *testing.T) {
	prepareMiss := func(t *testing.T, store *testStore, cfg *resolve.FetchCacheConfig, item *astjson.Value) (resolve.RequestCache, *resolve.FetchCacheHandle) {
		t.Helper()
		rc := NewController(store, nil).BeginRequest(nil)
		decision, handle := prepare(t, rc, cfg, item)
		require.Equal(t, resolve.DecisionFetch, decision)
		require.NotNil(t, handle)
		return rc, handle
	}

	t.Run("[F1] clean fetch writes exactly once with the exact TTL", func(t *testing.T) {
		store := newTestStore()
		cfg := entityConfig(t, time.Minute)
		item := productItem(t, "1")
		rc, handle := prepareMiss(t, store, cfg, item)
		require.NoError(t, rc.OnFetchResult(handle, resolve.MergeInput{
			Items:        []*astjson.Value{item},
			ResponseData: astjson.MustParseBytes([]byte(`{"__typename":"Product","name":"Table","price":100}`)),
			StatusCode:   200,
			Arena:        beginner(),
		}))
		key := handle.Items[0].RenderedKeys[0]
		rc.EndRequest()
		assert.Equal(t, []testStoreOp{
			{Kind: "Get", Key: key},
			{Kind: "Set", Key: key, Value: `{"__typename":"Product","name":"Table","price":100}`, TTL: time.Minute},
		}, store.ops)
	})

	gateRows := []struct {
		name string
		in   func(item *astjson.Value) resolve.MergeInput
	}{
		{
			// The historical blocking bug: transport failures arrive with
			// HasErrors == false — the gate must key off FetchFailed.
			name: "[F2] transport failure writes nothing",
			in: func(item *astjson.Value) resolve.MergeInput {
				return resolve.MergeInput{Items: []*astjson.Value{item}, FetchFailed: true, Arena: beginner()}
			},
		},
		{
			name: "[F3] empty body writes nothing",
			in: func(item *astjson.Value) resolve.MergeInput {
				return resolve.MergeInput{Items: []*astjson.Value{item}, FetchFailed: true, StatusCode: 200, Arena: beginner()}
			},
		},
		{
			name: "[F4] parse failure writes nothing",
			in: func(item *astjson.Value) resolve.MergeInput {
				return resolve.MergeInput{Items: []*astjson.Value{item}, FetchFailed: true, StatusCode: 200, Arena: beginner()}
			},
		},
		{
			name: "[F5] GraphQL errors write nothing even with data present",
			in: func(item *astjson.Value) resolve.MergeInput {
				return resolve.MergeInput{
					Items:        []*astjson.Value{item},
					ResponseData: astjson.MustParseBytes([]byte(`{"__typename":"Product","name":"Table","price":100}`)),
					HasErrors:    true,
					Arena:        beginner(),
				}
			},
		},
		{
			name: "[F6] JSON-null data writes nothing",
			in: func(item *astjson.Value) resolve.MergeInput {
				return resolve.MergeInput{Items: []*astjson.Value{item}, ResponseData: astjson.MustParseBytes([]byte(`null`)), Arena: beginner()}
			},
		},
		{
			name: "[F7] status fallback (failed fetch with 500) writes nothing",
			in: func(item *astjson.Value) resolve.MergeInput {
				return resolve.MergeInput{Items: []*astjson.Value{item}, FetchFailed: true, StatusCode: 500, Arena: beginner()}
			},
		},
		{
			name: "[F8] successful-but-empty entity writes nothing (negative caching is task 11)",
			in: func(item *astjson.Value) resolve.MergeInput {
				return resolve.MergeInput{
					Items:        []*astjson.Value{item},
					ResponseData: astjson.MustParseBytes([]byte(`null`)),
					EmptyEntity:  true,
					StatusCode:   200,
					Arena:        beginner(),
				}
			},
		},
	}
	for _, row := range gateRows {
		t.Run(row.name, func(t *testing.T) {
			store := newTestStore()
			cfg := entityConfig(t, time.Minute)
			item := productItem(t, "1")
			rc, handle := prepareMiss(t, store, cfg, item)
			key := handle.Items[0].RenderedKeys[0]
			require.NoError(t, rc.OnFetchResult(handle, row.in(item)))
			rc.EndRequest()
			// Only the lookup Get; ZERO Sets.
			assert.Equal(t, []testStoreOp{{Kind: "Get", Key: key}}, store.ops)
		})
	}
}

// TestControllerFlushRows covers the K rows: writes accumulate as bytes and
// flush once per instance at EndRequest.
func TestControllerFlushRows(t *testing.T) {
	t.Run("[K1/K2] writes accumulate and flush as one batch at EndRequest", func(t *testing.T) {
		store := newTestStore()
		cfg := entityConfig(t, time.Minute)
		rc := NewController(store, nil).BeginRequest(nil)

		item1 := productItem(t, "1")
		item2 := productItem(t, "2")
		_, h1 := prepare(t, rc, cfg, item1)
		_, h2 := prepare(t, rc, cfg, item2)
		require.NoError(t, rc.OnFetchResult(h1, resolve.MergeInput{
			Items:        []*astjson.Value{item1},
			ResponseData: astjson.MustParseBytes([]byte(`{"__typename":"Product","name":"Table","price":100}`)),
			Arena:        beginner(),
		}))
		require.NoError(t, rc.OnFetchResult(h2, resolve.MergeInput{
			Items:        []*astjson.Value{item2},
			ResponseData: astjson.MustParseBytes([]byte(`{"__typename":"Product","name":"Chair","price":50}`)),
			Arena:        beginner(),
		}))

		var setsBeforeFlush int
		for _, op := range store.ops {
			if op.Kind == "Set" {
				setsBeforeFlush++
			}
		}
		assert.Equal(t, 0, setsBeforeFlush)

		rc.EndRequest()
		assert.Equal(t, []testStoreOp{
			{Kind: "Get", Key: h1.Items[0].RenderedKeys[0]},
			{Kind: "Get", Key: h2.Items[0].RenderedKeys[0]},
			{Kind: "Set", Key: h1.Items[0].RenderedKeys[0], Value: `{"__typename":"Product","name":"Table","price":100}`, TTL: time.Minute},
			{Kind: "Set", Key: h2.Items[0].RenderedKeys[0], Value: `{"__typename":"Product","name":"Chair","price":50}`, TTL: time.Minute},
		}, store.ops)
	})

	t.Run("[K3] the flush holds bytes: later value mutation cannot leak into the store", func(t *testing.T) {
		store := newTestStore()
		cfg := entityConfig(t, time.Minute)
		rc := NewController(store, nil).BeginRequest(nil)
		item := productItem(t, "1")
		_, handle := prepare(t, rc, cfg, item)
		responseData := astjson.MustParseBytes([]byte(`{"__typename":"Product","name":"Table","price":100}`))
		require.NoError(t, rc.OnFetchResult(handle, resolve.MergeInput{
			Items:        []*astjson.Value{item},
			ResponseData: responseData,
			Arena:        beginner(),
		}))
		// Mutate the response value AFTER the hook, BEFORE the flush.
		responseData.Set(nil, "name", astjson.StringValue(nil, "Mutated"))
		rc.EndRequest()

		value, _, ok := store.Get(handle.Items[0].RenderedKeys[0])
		require.True(t, ok)
		assert.Equal(t, `{"__typename":"Product","name":"Table","price":100}`, string(value))
	})

	t.Run("[K4] nothing to flush: EndRequest is a no-op on the store", func(t *testing.T) {
		store := newTestStore()
		rc := NewController(store, nil).BeginRequest(nil)
		rc.EndRequest()
		assert.Empty(t, store.ops)
	})
}

// TestControllerMergePath covers the D4 deviation: splice and write honor the
// surfaced non-root merge path.
func TestControllerMergePath(t *testing.T) {
	t.Run("write stores the entity BELOW the merge path", func(t *testing.T) {
		store := newTestStore()
		cfg := entityConfig(t, time.Minute)
		rc := NewController(store, nil).BeginRequest(nil)
		item := productItem(t, "1")
		_, handle := prepare(t, rc, cfg, item)
		require.NoError(t, rc.OnFetchResult(handle, resolve.MergeInput{
			Items:        []*astjson.Value{item},
			ResponseData: astjson.MustParseBytes([]byte(`{"wrapper":{"__typename":"Product","name":"Table","price":100}}`)),
			MergePath:    []string{"wrapper"},
			Arena:        beginner(),
		}))
		rc.EndRequest()
		value, _, ok := store.Get(handle.Items[0].RenderedKeys[0])
		require.True(t, ok)
		assert.Equal(t, `{"__typename":"Product","name":"Table","price":100}`, string(value))
	})

	t.Run("splice merges the cached value AT the merge path", func(t *testing.T) {
		store := newTestStore()
		cfg := entityConfig(t, time.Minute)
		writeThrough(t, NewController(store, nil).BeginRequest(nil), cfg, productItem(t, "1"), `{"__typename":"Product","name":"Table","price":100}`)

		rc := NewController(store, nil).BeginRequest(nil)
		item := productItem(t, "1")
		decision, handle := prepare(t, rc, cfg, item)
		require.Equal(t, resolve.DecisionSkipFullHit, decision)
		require.NoError(t, rc.OnFetchSkipped(handle, resolve.MergeInput{
			Items:     []*astjson.Value{item},
			MergePath: []string{"nested"},
			Arena:     beginner(),
		}))
		assert.Equal(t,
			`{"__typename":"Product","upc":"1","nested":{"__typename":"Product","name":"Table","price":100}}`,
			string(item.MarshalTo(nil)))
	})

	t.Run("splice merges at the item root when the merge path is empty", func(t *testing.T) {
		store := newTestStore()
		cfg := entityConfig(t, time.Minute)
		writeThrough(t, NewController(store, nil).BeginRequest(nil), cfg, productItem(t, "1"), `{"__typename":"Product","name":"Table","price":100}`)

		rc := NewController(store, nil).BeginRequest(nil)
		item := productItem(t, "1")
		decision, handle := prepare(t, rc, cfg, item)
		require.Equal(t, resolve.DecisionSkipFullHit, decision)
		require.NoError(t, rc.OnFetchSkipped(handle, resolve.MergeInput{
			Items: []*astjson.Value{item},
			Arena: beginner(),
		}))
		assert.Equal(t,
			`{"__typename":"Product","upc":"1","name":"Table","price":100}`,
			string(item.MarshalTo(nil)))
	})
}

// TestControllerGates covers the prepare-side gates and nil-guards (O rows).
func TestControllerGates(t *testing.T) {
	rc := NewController(newTestStore(), nil).BeginRequest(nil)

	t.Run("nil config fetches", func(t *testing.T) {
		decision, handle := rc.PrepareFetch(prepareInput(nil, productItem(t, "1")))
		assert.Equal(t, resolve.DecisionFetch, decision)
		assert.Nil(t, handle)
	})

	t.Run("L1-only config is untouched by the L2 controller (task 17)", func(t *testing.T) {
		cfg := entityConfig(t, 0) // TTL 0 => L2 false, L1 true
		decision, handle := rc.PrepareFetch(prepareInput(cfg, productItem(t, "1")))
		assert.Equal(t, resolve.DecisionFetch, decision)
		assert.Nil(t, handle)
	})

	t.Run("shadow mode fetches until task 12", func(t *testing.T) {
		cfg := entityConfig(t, time.Minute)
		cfg.ShadowMode = true
		decision, handle := rc.PrepareFetch(prepareInput(cfg, productItem(t, "1")))
		assert.Equal(t, resolve.DecisionFetch, decision)
		assert.Nil(t, handle)
	})

	t.Run("root-field scope fetches until task 13", func(t *testing.T) {
		cfg := entityConfig(t, time.Minute)
		cfg.KeySpec.Scope = resolve.CacheScopeRootField
		decision, handle := rc.PrepareFetch(prepareInput(cfg, productItem(t, "1")))
		assert.Equal(t, resolve.DecisionFetch, decision)
		assert.Nil(t, handle)
	})

	t.Run("batch fetches until task 10", func(t *testing.T) {
		cfg := entityConfig(t, time.Minute)
		in := prepareInput(cfg, productItem(t, "1"))
		in.BatchStats = [][]*astjson.Value{{productItem(t, "1")}}
		decision, handle := rc.PrepareFetch(in)
		assert.Equal(t, resolve.DecisionFetch, decision)
		assert.Nil(t, handle)
	})

	t.Run("zero items fetch", func(t *testing.T) {
		cfg := entityConfig(t, time.Minute)
		decision, handle := rc.PrepareFetch(prepareInput(cfg))
		assert.Equal(t, resolve.DecisionFetch, decision)
		assert.Nil(t, handle)
	})

	t.Run("unrenderable candidate is a miss", func(t *testing.T) {
		cfg := entityConfig(t, time.Minute)
		item := astjson.MustParseBytes([]byte(`{"__typename":"Product"}`)) // no upc
		decision, handle := rc.PrepareFetch(prepareInput(cfg, item))
		assert.Equal(t, resolve.DecisionFetch, decision)
		require.NotNil(t, handle)
		assert.Nil(t, handle.Items[0].RenderedKeys)
	})

	t.Run("merge hooks tolerate nil handles and unknown handles", func(t *testing.T) {
		assert.NoError(t, rc.OnFetchSkipped(nil, resolve.MergeInput{Arena: beginner()}))
		assert.NoError(t, rc.OnFetchResult(nil, resolve.MergeInput{Arena: beginner()}))
		unknown := &resolve.FetchCacheHandle{}
		assert.NoError(t, rc.OnFetchSkipped(unknown, resolve.MergeInput{Arena: beginner()}))
		assert.NoError(t, rc.OnFetchResult(unknown, resolve.MergeInput{Arena: beginner()}))
	})
}
