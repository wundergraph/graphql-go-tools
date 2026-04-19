// Package resolve tests.
//
// This file contains "copy invariant" tests that exercise the four
// StructuralCopy call sites in loader.go which sit on the cache-hit merge
// paths:
//
//   - loader.go:1220 — mergeBatchCacheHit: entityArray.SetArrayItem(..., StructuralCopy(entity))
//   - loader.go:1372 — mergeBatchPartialResponse: completeArray.SetArrayItem(..., StructuralCopy(entity))
//   - loader.go:1472 — mergeResult cacheSkipFetch: MergeValues(..., Item, StructuralCopy(FromCache))
//   - loader.go:1491 — mergeResult partialCacheEnabled: MergeValues(..., Item, StructuralCopy(FromCache))
//
// The invariant under test: after the merge runs, mutations to the resulting
// response tree MUST NOT mutate the source `FromCache` values that were read
// from the cache. StructuralCopy is what provides that isolation today.
//
// These tests are designed to:
//   1. Pass on current master (proving the invariant holds today).
//   2. Fail if a candidate StructuralCopy is removed AND it was load-bearing
//      (i.e., mutations to the merged tree would corrupt a shared container
//      node inside FromCache).
//
// If a test still passes after a removal, the copy is provably redundant at
// that site, given how MergeValues and the response tree interact today.
//
// Mutation strategy: we deliberately mutate a NESTED object under the merged
// tree (not a top-level field), because MergeValues only aliases nested
// containers — top-level fields are always rewritten by the merge itself.
// Mutating a nested container is the real-world corruption risk.
package resolve

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wundergraph/astjson"
)

// copyInvariantEntityJSON is a nested entity shape. The `profile` object is
// the nested container whose aliasing is the corruption risk.
const copyInvariantEntityJSON = `{"__typename":"User","id":"u1","name":"Alice","profile":{"email":"alice@example.com","age":30}}`

// assertFromCacheUnchanged reparses the original JSON to produce a fresh
// reference value and compares against FromCache.MarshalTo. Using a fresh
// parse avoids any chance of the reference itself being mutated.
func assertFromCacheUnchanged(t *testing.T, fromCache *astjson.Value, originalJSON string) {
	t.Helper()
	require.NotNil(t, fromCache)
	assert.Equal(t, originalJSON, string(fromCache.MarshalTo(nil)),
		"FromCache was mutated by downstream merge / response-tree mutation — StructuralCopy at this site is load-bearing")
}

// TestCopyInvariant_MergeBatchCacheHit targets loader.go:1220.
//
// Scenario: batch entity fetch with EntityMergePath — cached entities are
// spliced into a response array via entityArray.SetArrayItem(..., StructuralCopy(entity)),
// then MergeValuesWithPath merges that response into items[0].
//
// Adversarial mutation: after the merge, reach into the merged response tree
// and mutate the nested `profile` object. If StructuralCopy is removed, the
// merged response's profile node may alias back into FromCache.
func TestCopyInvariant_MergeBatchCacheHit(t *testing.T) {
	l, ar := newCacheMergeTestLoader(t)

	wrapped0, err := astjson.ParseBytesWithArena(ar, []byte(`{"users":`+copyInvariantEntityJSON+`}`))
	require.NoError(t, err)
	wrapped1, err := astjson.ParseBytesWithArena(ar, []byte(`{"users":`+copyInvariantEntityJSON+`}`))
	require.NoError(t, err)

	// FromCache points at the entity inside the wrapper (as bulk L2 lookup
	// produces, before mergeBatchCacheHit unwraps via EntityMergePath).
	fromCache0 := wrapped0
	fromCache1 := wrapped1

	cacheKeys := []*CacheKey{
		{BatchIndex: 0, FromCache: fromCache0, Keys: []string{"key0"}, EntityMergePath: []string{"users"}},
		{BatchIndex: 1, FromCache: fromCache1, Keys: []string{"key1"}, EntityMergePath: []string{"users"}},
	}
	res := &result{l2CacheKeys: cacheKeys}

	// Root-level merge: resolvable.data will be set.
	err = l.mergeBatchCacheHit(&FetchItem{}, res, nil)
	require.NoError(t, err)

	// Sanity: merge produced the expected shape.
	got := string(l.resolvable.data.MarshalTo(nil))
	assert.Equal(t,
		`{"users":[`+copyInvariantEntityJSON+`,`+copyInvariantEntityJSON+`]}`,
		got)

	// Adversarial mutation: reach into the merged response array's FIRST
	// entity's nested profile and mutate it. If the copy is redundant,
	// fromCache0's nested profile is independent and survives. If the copy
	// was load-bearing, fromCache0's profile is now corrupted.
	mergedArray := l.resolvable.data.Get("users").GetArray()
	require.Len(t, mergedArray, 2)
	profile0 := mergedArray[0].Get("profile")
	require.NotNil(t, profile0)
	profile0.Set(ar, "email", astjson.StringValue(ar, "CORRUPTED"))
	profile0.Set(ar, "age", astjson.NumberValue(ar, "999"))

	// Also mutate the SECOND entity's profile via Del + Set to exercise
	// multiple mutation kinds.
	profile1 := mergedArray[1].Get("profile")
	require.NotNil(t, profile1)
	profile1.Del("age")
	profile1.Set(ar, "email", astjson.StringValue(ar, "ALSO_CORRUPTED"))

	// Invariant: both FromCache pointers must still produce the original JSON.
	// Note: FromCache here is the wrapper value; the entity is at FromCache.users.
	assertFromCacheUnchanged(t, fromCache0, `{"users":`+copyInvariantEntityJSON+`}`)
	assertFromCacheUnchanged(t, fromCache1, `{"users":`+copyInvariantEntityJSON+`}`)
}

// TestCopyInvariant_MergeBatchPartialResponse targets loader.go:1372.
//
// Scenario: partial batch fetch — some entities are cache hits (interleaved
// into the result via StructuralCopy), others come from the fresh subgraph
// response. completeArray.SetArrayItem(..., StructuralCopy(entity)) is the
// site under test.
func TestCopyInvariant_MergeBatchPartialResponse(t *testing.T) {
	l, ar := newCacheMergeTestLoader(t)

	// Cached entity for index 0 (indices 1 and 2 will come from fresh response).
	cachedEntity, err := astjson.ParseBytesWithArena(ar, []byte(copyInvariantEntityJSON))
	require.NoError(t, err)
	fromCache := cachedEntity

	// Fresh subgraph response already merged into items[0]: contains entities
	// at indices 1 and 2. mergeBatchPartialResponse reads from
	// items[0].Get(arrayPath...) and rebuilds the full array.
	freshResponse, err := astjson.ParseBytesWithArena(ar, []byte(
		`{"users":[`+
			`{"__typename":"User","id":"u2","name":"Bob","profile":{"email":"bob@example.com","age":25}},`+
			`{"__typename":"User","id":"u3","name":"Cara","profile":{"email":"cara@example.com","age":40}}`+
			`]}`))
	require.NoError(t, err)

	items := []*astjson.Value{freshResponse}

	res := &result{
		l2CacheKeys: []*CacheKey{
			{BatchIndex: 0, FromCache: fromCache, Keys: []string{"key0"}},
		},
		batchCachedIndices: []int{0},
		batchMissedIndices: []int{1, 2},
	}

	info := &FetchInfo{RootFields: []GraphCoordinate{{FieldName: "users"}}}

	l.mergeBatchPartialResponse(res, items, info)

	// Sanity: the interleaved array has three elements, with the cached
	// entity at index 0.
	got := string(items[0].MarshalTo(nil))
	assert.Equal(t,
		`{"users":[{"__typename":"User","id":"u1","name":"Alice","profile":{"email":"alice@example.com","age":30}},{"__typename":"User","id":"u2","name":"Bob","profile":{"email":"bob@example.com","age":25}},{"__typename":"User","id":"u3","name":"Cara","profile":{"email":"cara@example.com","age":40}}]}`,
		got)

	// Adversarial mutation: mutate the nested profile of the entity that
	// was spliced from the cache (index 0 in the rebuilt array).
	mergedArray := items[0].Get("users").GetArray()
	require.GreaterOrEqual(t, len(mergedArray), 1)
	profile0 := mergedArray[0].Get("profile")
	require.NotNil(t, profile0)
	profile0.Set(ar, "email", astjson.StringValue(ar, "CORRUPTED"))
	profile0.Del("age")

	// Invariant: the cached entity must still produce the original JSON.
	assertFromCacheUnchanged(t, fromCache, copyInvariantEntityJSON)
}

// TestCopyInvariant_MergeResultCacheSkipFetch targets loader.go:1472.
//
// Scenario: all entities are full L1 hits — mergeResult takes the
// cacheSkipFetch branch and does MergeValues(Item, StructuralCopy(FromCache))
// for each key. The Item is the destination (a slot in the response tree);
// FromCache is the cached entity.
//
// Adversarial mutation: mutate the nested `profile` under Item after merge.
// If the copy is load-bearing, FromCache's profile container was aliased and
// is now corrupted.
func TestCopyInvariant_MergeResultCacheSkipFetch(t *testing.T) {
	l, ar := newCacheMergeTestLoader(t)

	fromCache, err := astjson.ParseBytesWithArena(ar, []byte(copyInvariantEntityJSON))
	require.NoError(t, err)

	// Item is the response-tree slot where the cached entity will be merged.
	// Empty object simulates the placeholder in the response items array.
	item, err := astjson.ParseBytesWithArena(ar, []byte(`{"id":"u1"}`))
	require.NoError(t, err)

	res := &result{
		cacheSkipFetch:     true,
		batchEntityKeyMode: false,
		l1CacheKeys: []*CacheKey{
			{Item: item, FromCache: fromCache, Keys: []string{"key0"}},
		},
	}

	err = l.mergeResult(&FetchItem{}, res, []*astjson.Value{item})
	require.NoError(t, err)

	// Sanity: item now has the cached fields merged in.
	assert.Equal(t,
		`{"id":"u1","__typename":"User","name":"Alice","profile":{"email":"alice@example.com","age":30}}`,
		string(item.MarshalTo(nil)))

	// Adversarial mutation: mutate nested profile.
	profile := item.Get("profile")
	require.NotNil(t, profile)
	profile.Set(ar, "email", astjson.StringValue(ar, "CORRUPTED"))
	profile.Del("age")

	// Invariant: fromCache must still produce the original JSON.
	assertFromCacheUnchanged(t, fromCache, copyInvariantEntityJSON)
}

// TestCopyInvariant_MergeResultPartialCache targets loader.go:1491.
//
// Scenario: partial cache loading — some items are L1 hits, others require
// fetch. mergeResult first merges cached entries (the loop at line 1484-1497)
// via MergeValues(Item, StructuralCopy(FromCache)), then returns early
// because fetchSkipped=true (we only want to exercise the partial-cache
// branch in this test).
func TestCopyInvariant_MergeResultPartialCache(t *testing.T) {
	l, ar := newCacheMergeTestLoader(t)

	fromCache, err := astjson.ParseBytesWithArena(ar, []byte(copyInvariantEntityJSON))
	require.NoError(t, err)

	item, err := astjson.ParseBytesWithArena(ar, []byte(`{"id":"u1"}`))
	require.NoError(t, err)

	res := &result{
		partialCacheEnabled: true,
		cachedItemIndices:   []int{0},
		l1CacheKeys: []*CacheKey{
			{Item: item, FromCache: fromCache, Keys: []string{"key0"}},
		},
		fetchSkipped: true, // short-circuit after the partial-cache merge loop
	}

	err = l.mergeResult(&FetchItem{}, res, []*astjson.Value{item})
	require.NoError(t, err)

	// Sanity: item now has the cached fields merged in.
	assert.Equal(t,
		`{"id":"u1","__typename":"User","name":"Alice","profile":{"email":"alice@example.com","age":30}}`,
		string(item.MarshalTo(nil)))

	// Adversarial mutation: mutate nested profile.
	profile := item.Get("profile")
	require.NotNil(t, profile)
	profile.Set(ar, "email", astjson.StringValue(ar, "CORRUPTED"))
	profile.Del("age")

	// Invariant: fromCache must still produce the original JSON.
	assertFromCacheUnchanged(t, fromCache, copyInvariantEntityJSON)
}
