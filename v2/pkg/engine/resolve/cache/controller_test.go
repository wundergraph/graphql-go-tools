package cache

import (
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wundergraph/astjson"
	"github.com/wundergraph/go-arena"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

type storeOp struct {
	Kind  string
	Key   string
	Value string
	TTL   time.Duration
}

type storedEntry struct {
	value     []byte
	expiresAt time.Time
}

type testStore struct {
	data map[string]storedEntry
	ops  []storeOp
}

func newTestStore() *testStore {
	return &testStore{data: make(map[string]storedEntry)}
}

func (s *testStore) Seed(key string, value []byte, ttl time.Duration) {
	s.data[key] = storedEntry{
		value:     append([]byte(nil), value...),
		expiresAt: time.Now().Add(ttl),
	}
}

func (s *testStore) Get(key string) ([]byte, time.Duration, bool) {
	s.ops = append(s.ops, storeOp{Kind: "Get", Key: key})
	entry, ok := s.data[key]
	if !ok {
		return nil, 0, false
	}
	remaining := time.Until(entry.expiresAt)
	if remaining <= 0 {
		return nil, 0, false
	}
	return append([]byte(nil), entry.value...), remaining, true
}

func (s *testStore) Set(key string, value []byte, ttl time.Duration) {
	s.ops = append(s.ops, storeOp{Kind: "Set", Key: key, Value: string(value), TTL: ttl})
	s.data[key] = storedEntry{
		value:     append([]byte(nil), value...),
		expiresAt: time.Now().Add(ttl),
	}
}

type testMergeArena struct {
	a arena.Arena
}

func newTestMergeArena() *testMergeArena {
	return &testMergeArena{a: arena.NewMonotonicArena(arena.WithMinBufferSize(1024))}
}

func (m *testMergeArena) Begin() resolve.MergeSession {
	return &testMergeSession{a: m.a}
}

type testMergeSession struct {
	a arena.Arena
}

func (s *testMergeSession) ParseBytes(b []byte) (*astjson.Value, error) {
	return astjson.ParseBytesWithArena(s.a, b)
}

func (s *testMergeSession) StructuralCopy(v *astjson.Value) *astjson.Value {
	return astjson.DeepCopy(s.a, v)
}

func (s *testMergeSession) MergeValues(dst, src *astjson.Value) (*astjson.Value, error) {
	merged, _, err := astjson.MergeValues(s.a, dst, src)
	return merged, err
}

func (s *testMergeSession) MergeValuesWithPath(dst, src *astjson.Value, path ...string) (*astjson.Value, error) {
	merged, _, err := astjson.MergeValuesWithPath(s.a, dst, src, path...)
	return merged, err
}

func (s *testMergeSession) NewObject() *astjson.Value {
	return astjson.ObjectValue(s.a)
}

func (s *testMergeSession) NewArray() *astjson.Value {
	return astjson.ArrayValue(s.a)
}

func (s *testMergeSession) String(value string) *astjson.Value {
	return astjson.StringValue(s.a, value)
}

func (s *testMergeSession) Null() *astjson.Value {
	return astjson.NullValue
}

func (s *testMergeSession) Close() {}

func TestControllerPrepareFetch_SingleCandidateL2Hit(t *testing.T) {
	cfg := productConfig(30 * time.Second)
	item := parseValue(t, `{"__typename":"Product","upc":"1"}`)
	key := renderedKey(t, cfg, item)
	store := newTestStore()
	store.Seed(key, []byte(`{"upc":"1","name":"Table"}`), time.Minute)

	rc := NewController(store, ModeL2, nil).BeginRequest(resolve.NewContext(t.Context()))
	decision, handle := rc.PrepareFetch(prepareInput(t, cfg, item))
	require.NotNil(t, handle)

	assert.Equal(t, resolve.DecisionSkipFullHit, decision)
	assert.Equal(t, &resolve.FetchCacheHandle{
		Decision: resolve.DecisionSkipFullHit,
		WasHit:   true,
		Items: []resolve.ItemCacheState{
			{
				Item:                 item,
				FromCache:            handle.Items[0].FromCache,
				RenderedKeys:         []string{key},
				SelectedRemainingTTL: handle.Items[0].SelectedRemainingTTL,
			},
		},
	}, handle)
	require.NoError(t, rc.OnFetchSkipped(handle, mergeInput(item, nil, false, false)))
	assert.Equal(t, `{"__typename":"Product","upc":"1","name":"Table"}`, valueString(item))
	assert.Equal(t, []storeOp{{Kind: "Get", Key: key}}, store.ops)
}

func TestControllerPrepareFetch_NoOpMode(t *testing.T) {
	cfg := productConfig(time.Minute)
	item := parseValue(t, `{"__typename":"Product","upc":"1"}`)
	store := newTestStore()

	rc := NewController(store, ModeNoop, nil).BeginRequest(resolve.NewContext(t.Context()))
	decision, handle := rc.PrepareFetch(prepareInput(t, cfg, item))

	assert.Equal(t, resolve.DecisionFetch, decision)
	assert.Equal(t, (*resolve.FetchCacheHandle)(nil), handle)
	assert.Equal(t, []storeOp(nil), store.ops)
}

func TestControllerPrepareFetch_SingleCandidateL2MissWritesOnFetchResult(t *testing.T) {
	cfg := productConfig(45 * time.Second)
	item := parseValue(t, `{"__typename":"Product","upc":"1"}`)
	key := renderedKey(t, cfg, item)
	store := newTestStore()

	rc := NewController(store, ModeL2, nil).BeginRequest(resolve.NewContext(t.Context()))
	decision, handle := rc.PrepareFetch(prepareInput(t, cfg, item))
	require.NotNil(t, handle)

	assert.Equal(t, resolve.DecisionFetch, decision)
	assert.Equal(t, &resolve.FetchCacheHandle{
		Decision: resolve.DecisionFetch,
		Items: []resolve.ItemCacheState{
			{Item: item, RenderedKeys: []string{key}},
		},
	}, handle)
	require.NoError(t, rc.OnFetchResult(handle, mergeInput(item, parseValue(t, `{"upc":"1","name":"Table"}`), false, false)))
	assert.Equal(t, []storeOp{{Kind: "Get", Key: key}}, store.ops)

	rc.EndRequest()
	assert.Equal(t, []storeOp{
		{Kind: "Get", Key: key},
		{Kind: "Set", Key: key, Value: `{"upc":"1","name":"Table"}`, TTL: 45 * time.Second},
	}, store.ops)
}

func TestControllerPrepareFetch_CoverageValidation(t *testing.T) {
	tests := []struct {
		name     string
		cached   string
		provides *resolve.Object
		decision resolve.Decision
	}{
		{
			name:     "D3 missing required field is miss",
			cached:   `{"upc":"1"}`,
			provides: productProvides(false),
			decision: resolve.DecisionFetch,
		},
		{
			name:     "D4 null nullable field is hit",
			cached:   `{"upc":"1","name":null}`,
			provides: productProvides(true),
			decision: resolve.DecisionSkipFullHit,
		},
		{
			name:     "D5 null non nullable field is miss",
			cached:   `{"upc":"1","name":null}`,
			provides: productProvides(false),
			decision: resolve.DecisionFetch,
		},
		{
			name:     "D14 nil ProvidesData is miss",
			cached:   `{"upc":"1","name":"Table"}`,
			provides: nil,
			decision: resolve.DecisionFetch,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := productConfig(time.Minute)
			cfg.ProvidesData = tt.provides
			item := parseValue(t, `{"__typename":"Product","upc":"1"}`)
			key := renderedKey(t, cfg, item)
			store := newTestStore()
			store.Seed(key, []byte(tt.cached), time.Minute)

			rc := NewController(store, ModeL2, nil).BeginRequest(resolve.NewContext(t.Context()))
			decision, handle := rc.PrepareFetch(prepareInput(t, cfg, item))
			require.NotNil(t, handle)

			assert.Equal(t, tt.decision, decision)
			assert.Equal(t, `{"__typename":"Product","upc":"1"}`, valueString(item))
			assert.Equal(t, []storeOp{{Kind: "Get", Key: key}}, store.ops)
		})
	}
}

func TestControllerOnFetchResult_WriteGate(t *testing.T) {
	tests := []struct {
		name        string
		response    *astjson.Value
		fetchFailed bool
		hasErrors   bool
	}{
		{
			name:     "F1 clean success writes",
			response: parseValue(t, `{"upc":"1","name":"Table"}`),
		},
		{
			name:        "F2 transport failure writes nothing",
			fetchFailed: true,
		},
		{
			name:      "F5 GraphQL errors write nothing",
			response:  parseValue(t, `{"upc":"1","name":"Table"}`),
			hasErrors: true,
		},
		{
			name:     "F6 JSON null writes nothing",
			response: parseValue(t, `null`),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := productConfig(20 * time.Second)
			item := parseValue(t, `{"__typename":"Product","upc":"1"}`)
			key := renderedKey(t, cfg, item)
			store := newTestStore()

			rc := NewController(store, ModeL2, nil).BeginRequest(resolve.NewContext(t.Context()))
			_, handle := rc.PrepareFetch(prepareInput(t, cfg, item))
			require.NoError(t, rc.OnFetchResult(handle, mergeInput(item, tt.response, tt.fetchFailed, tt.hasErrors)))
			rc.EndRequest()

			expected := []storeOp{{Kind: "Get", Key: key}}
			if tt.name == "F1 clean success writes" {
				expected = append(expected, storeOp{Kind: "Set", Key: key, Value: `{"upc":"1","name":"Table"}`, TTL: 20 * time.Second})
			}
			assert.Equal(t, expected, store.ops)
		})
	}
}

func TestControllerEndRequest_FlushesDeferredSets(t *testing.T) {
	cfg := productConfig(time.Minute)
	store := newTestStore()
	rc := NewController(store, ModeL2, nil).BeginRequest(resolve.NewContext(t.Context()))

	itemA := parseValue(t, `{"__typename":"Product","upc":"1"}`)
	keyA := renderedKey(t, cfg, itemA)
	_, handleA := rc.PrepareFetch(prepareInput(t, cfg, itemA))
	require.NoError(t, rc.OnFetchResult(handleA, mergeInput(itemA, parseValue(t, `{"upc":"1","name":"Table"}`), false, false)))

	itemB := parseValue(t, `{"__typename":"Product","upc":"2"}`)
	keyB := renderedKey(t, cfg, itemB)
	_, handleB := rc.PrepareFetch(prepareInput(t, cfg, itemB))
	require.NoError(t, rc.OnFetchResult(handleB, mergeInput(itemB, parseValue(t, `{"upc":"2","name":"Chair"}`), false, false)))

	assert.Equal(t, []storeOp{{Kind: "Get", Key: keyA}, {Kind: "Get", Key: keyB}}, store.ops)
	rc.EndRequest()
	assert.Equal(t, []storeOp{
		{Kind: "Get", Key: keyA},
		{Kind: "Get", Key: keyB},
		{Kind: "Set", Key: keyA, Value: `{"upc":"1","name":"Table"}`, TTL: time.Minute},
		{Kind: "Set", Key: keyB, Value: `{"upc":"2","name":"Chair"}`, TTL: time.Minute},
	}, store.ops)
	rc.EndRequest()
	assert.Equal(t, []storeOp{
		{Kind: "Get", Key: keyA},
		{Kind: "Get", Key: keyB},
		{Kind: "Set", Key: keyA, Value: `{"upc":"1","name":"Table"}`, TTL: time.Minute},
		{Kind: "Set", Key: keyB, Value: `{"upc":"2","name":"Chair"}`, TTL: time.Minute},
	}, store.ops)
}

func TestControllerEndRequest_NothingToFlush(t *testing.T) {
	store := newTestStore()
	rc := NewController(store, ModeL2, nil).BeginRequest(resolve.NewContext(t.Context()))
	rc.EndRequest()
	assert.Equal(t, []storeOp(nil), store.ops)
}

func TestControllerPrepareFetch_ExpiredEntryIsMiss(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		cfg := productConfig(time.Minute)
		item := parseValue(t, `{"__typename":"Product","upc":"1"}`)
		key := renderedKey(t, cfg, item)
		store := newTestStore()
		store.Seed(key, []byte(`{"upc":"1","name":"Table"}`), time.Second)

		time.Sleep(time.Second + time.Nanosecond)
		synctest.Wait()

		rc := NewController(store, ModeL2, nil).BeginRequest(resolve.NewContext(t.Context()))
		decision, handle := rc.PrepareFetch(prepareInput(t, cfg, item))
		require.NotNil(t, handle)

		assert.Equal(t, resolve.DecisionFetch, decision)
		assert.Equal(t, []storeOp{{Kind: "Get", Key: key}}, store.ops)
	})
}

func prepareInput(t *testing.T, cfg *resolve.FetchCacheConfig, item *astjson.Value) resolve.PrepareFetchInput {
	t.Helper()
	return resolve.PrepareFetchInput{
		Ctx:    resolve.NewContext(t.Context()),
		Items:  []*astjson.Value{item},
		Config: cfg,
		Arena:  newTestMergeArena(),
	}
}

func mergeInput(item, responseData *astjson.Value, fetchFailed, hasErrors bool) resolve.MergeInput {
	return resolve.MergeInput{
		Items:        []*astjson.Value{item},
		ResponseData: responseData,
		FetchFailed:  fetchFailed,
		HasErrors:    hasErrors,
		Arena:        newTestMergeArena(),
	}
}

func renderedKey(t *testing.T, cfg *resolve.FetchCacheConfig, item *astjson.Value) string {
	t.Helper()
	arena := newTestMergeArena()
	session := arena.Begin()
	defer session.Close()
	key, ok := renderEntityKey(session, cfg.KeySpec.Candidates[0].Representation, item, cacheKeyPrefix(cfg, 0))
	require.Equal(t, true, ok)
	return key
}

func productConfig(ttl time.Duration) *resolve.FetchCacheConfig {
	return &resolve.FetchCacheConfig{
		L2:        true,
		CacheName: "entity:products",
		TTL:       ttl,
		KeySpec: resolve.CacheKeySpec{
			Scope:    resolve.CacheScopeEntity,
			TypeName: "Product",
			Candidates: []resolve.CacheKeyCandidate{
				{Representation: productRepresentation()},
			},
		},
		ProvidesData: productProvides(false),
	}
}

func productRepresentation() *resolve.Object {
	return &resolve.Object{
		Fields: []*resolve.Field{
			{Name: []byte("__typename"), Value: &resolve.Scalar{}},
			{Name: []byte("upc"), Value: &resolve.Scalar{}},
		},
	}
}

func productProvides(nullableName bool) *resolve.Object {
	return &resolve.Object{
		Fields: []*resolve.Field{
			{Name: []byte("upc"), Value: &resolve.Scalar{}},
			{Name: []byte("name"), Value: &resolve.Scalar{Nullable: nullableName}},
		},
	}
}

func parseValue(t *testing.T, input string) *astjson.Value {
	t.Helper()
	value, err := astjson.ParseBytes([]byte(input))
	require.NoError(t, err)
	return value
}

func valueString(value *astjson.Value) string {
	if value == nil {
		return ""
	}
	return string(value.MarshalTo(nil))
}
