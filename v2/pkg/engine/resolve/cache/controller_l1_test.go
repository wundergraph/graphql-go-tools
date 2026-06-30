package cache

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wundergraph/astjson"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

func TestControllerL1_ModeMatrix(t *testing.T) {
	cfg := productL1L2Config(45 * time.Second)
	l1Key := renderedL1Key(t, cfg, parseValue(t, `{"__typename":"Product","upc":"1"}`))
	l2Key := renderedKey(t, cfg, parseValue(t, `{"__typename":"Product","upc":"1"}`))
	fresh := []byte(`{"upc":"1","name":"Table"}`)

	tests := []struct {
		name               string
		mode               Mode
		seedL2             bool
		wantFirstDecision  resolve.Decision
		wantFirstHandle    bool
		wantFirstItem      string
		wantStoreOps       []storeOp
		wantSecondDecision resolve.Decision
		wantSecondItem     string
		wantSecondStoreOps []storeOp
	}{
		{
			name:               "J1 noop returns fetch without a handle",
			mode:               ModeNoop,
			wantFirstDecision:  resolve.DecisionFetch,
			wantFirstHandle:    false,
			wantFirstItem:      `{"__typename":"Product","upc":"1"}`,
			wantStoreOps:       []storeOp(nil),
			wantSecondDecision: resolve.DecisionFetch,
			wantSecondItem:     `{"__typename":"Product","upc":"1"}`,
			wantSecondStoreOps: []storeOp(nil),
		},
		{
			name:               "J2 J3 l1 hit short-circuits l2",
			mode:               ModeL1,
			wantFirstDecision:  resolve.DecisionFetch,
			wantFirstHandle:    true,
			wantFirstItem:      `{"__typename":"Product","upc":"1"}`,
			wantStoreOps:       []storeOp(nil),
			wantSecondDecision: resolve.DecisionSkipFullHit,
			wantSecondItem:     `{"__typename":"Product","upc":"1","name":"Table"}`,
			wantSecondStoreOps: []storeOp(nil),
		},
		{
			name:               "J4 l2 hit serves data",
			mode:               ModeL2,
			seedL2:             true,
			wantFirstDecision:  resolve.DecisionSkipFullHit,
			wantFirstHandle:    true,
			wantFirstItem:      `{"__typename":"Product","upc":"1","name":"Table"}`,
			wantStoreOps:       []storeOp{{Kind: "Get", Key: l2Key}},
			wantSecondDecision: resolve.DecisionSkipFullHit,
			wantSecondItem:     `{"__typename":"Product","upc":"1","name":"Table"}`,
			wantSecondStoreOps: []storeOp{{Kind: "Get", Key: l2Key}, {Kind: "Get", Key: l2Key}},
		},
		{
			name:               "J5 J6 J7 l1l2 l2 hit populates l1 for same request",
			mode:               ModeL1L2,
			seedL2:             true,
			wantFirstDecision:  resolve.DecisionSkipFullHit,
			wantFirstHandle:    true,
			wantFirstItem:      `{"__typename":"Product","upc":"1","name":"Table"}`,
			wantStoreOps:       []storeOp{{Kind: "Get", Key: l2Key}},
			wantSecondDecision: resolve.DecisionSkipFullHit,
			wantSecondItem:     `{"__typename":"Product","upc":"1","name":"Table"}`,
			wantSecondStoreOps: []storeOp{{Kind: "Get", Key: l2Key}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := newTestStore()
			if tt.seedL2 {
				store.Seed(l2Key, fresh, time.Minute)
			}
			rc := NewController(store, tt.mode, nil).BeginRequest(resolve.NewContext(t.Context()))

			firstItem := parseValue(t, `{"__typename":"Product","upc":"1"}`)
			firstDecision, firstHandle := rc.PrepareFetch(prepareInput(t, cfg, firstItem))
			assert.Equal(t, tt.wantFirstDecision, firstDecision)
			if tt.wantFirstHandle {
				require.NotNil(t, firstHandle)
			} else {
				assert.Equal(t, (*resolve.FetchCacheHandle)(nil), firstHandle)
			}
			if tt.wantFirstDecision == resolve.DecisionFetch && firstHandle != nil {
				require.NoError(t, rc.OnFetchResult(firstHandle, mergeInput(firstItem, parseValue(t, string(fresh)), false, false)))
			}
			if tt.wantFirstDecision == resolve.DecisionSkipFullHit {
				require.NoError(t, rc.OnFetchSkipped(firstHandle, mergeInput(firstItem, nil, false, false)))
			}
			assert.Equal(t, tt.wantFirstItem, valueString(firstItem))
			assert.Equal(t, tt.wantStoreOps, store.ops)

			secondItem := parseValue(t, `{"__typename":"Product","upc":"1"}`)
			secondDecision, secondHandle := rc.PrepareFetch(prepareInput(t, cfg, secondItem))
			assert.Equal(t, tt.wantSecondDecision, secondDecision)
			if tt.wantSecondDecision == resolve.DecisionSkipFullHit {
				require.NotNil(t, secondHandle)
				require.NoError(t, rc.OnFetchSkipped(secondHandle, mergeInput(secondItem, nil, false, false)))
			}
			assert.Equal(t, tt.wantSecondItem, valueString(secondItem))
			assert.Equal(t, tt.wantSecondStoreOps, store.ops)
		})
	}

	assert.Equal(t, false, l1Key == l2Key)
	assert.Equal(t, "entity:products:", l2Key[:len("entity:products:")])
}

func TestControllerL1_OnFetchResultWritesPrefixFreeL1Key(t *testing.T) {
	cfg := productL1L2Config(time.Minute)
	item := parseValue(t, `{"__typename":"Product","upc":"1"}`)
	l1Key := renderedL1Key(t, cfg, item)
	store := newTestStore()

	rc := NewController(store, ModeL1, nil).BeginRequest(resolve.NewContext(t.Context()))
	decision, handle := rc.PrepareFetch(prepareInput(t, cfg, item))
	require.NotNil(t, handle)

	assert.Equal(t, resolve.DecisionFetch, decision)
	assert.Equal(t, &resolve.FetchCacheHandle{
		Decision: resolve.DecisionFetch,
		Items: []resolve.ItemCacheState{
			{Item: item, RenderedKeys: []string{l1Key}},
		},
	}, handle)
	require.NoError(t, rc.OnFetchResult(handle, mergeInput(item, parseValue(t, `{"upc":"1","name":"Table"}`), false, false)))
	assert.Equal(t, []storeOp(nil), store.ops)

	secondItem := parseValue(t, `{"__typename":"Product","upc":"1"}`)
	secondDecision, secondHandle := rc.PrepareFetch(prepareInput(t, cfg, secondItem))
	require.NotNil(t, secondHandle)
	assert.Equal(t, resolve.DecisionSkipFullHit, secondDecision)
	assert.Equal(t, &resolve.FetchCacheHandle{
		Decision: resolve.DecisionSkipFullHit,
		WasHit:   true,
		Items: []resolve.ItemCacheState{
			{
				Item:                 secondItem,
				FromCache:            secondHandle.Items[0].FromCache,
				RenderedKeys:         []string{l1Key},
				FromCacheCandidates:  []resolve.CacheCandidate{{Value: []byte(`{"upc":"1","name":"Table"}`), RemainingTTL: time.Minute}},
				SelectedRemainingTTL: time.Minute,
			},
		},
	}, secondHandle)
	require.NoError(t, rc.OnFetchSkipped(secondHandle, mergeInput(secondItem, nil, false, false)))
	assert.Equal(t, `{"__typename":"Product","upc":"1","name":"Table"}`, valueString(secondItem))
	assert.Equal(t, []storeOp(nil), store.ops)
}

func productL1L2Config(ttl time.Duration) *resolve.FetchCacheConfig {
	cfg := productConfig(ttl)
	cfg.L1 = true
	return cfg
}

func renderedL1Key(t *testing.T, cfg *resolve.FetchCacheConfig, item *astjson.Value) string {
	t.Helper()
	arena := newTestMergeArena()
	session := arena.Begin()
	defer session.Close()
	key, ok := renderEntityKey(session, cfg.KeySpec.Candidates[0].Representation, item, "")
	require.Equal(t, true, ok)
	return key
}
