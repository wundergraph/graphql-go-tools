package resolve

import (
	"runtime"
	"runtime/debug"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wundergraph/astjson"
	"github.com/wundergraph/go-arena"
)

// TestCrossArenaMergeValuesCreatesShallowReferences proves that MergeValues
// links *Value pointers from the source arena into the target arena's tree
// without deep-copying. Resetting the source arena makes the merged values stale.
//
// This is the foundational invariant for AC-THREAD-04: goroutine arenas that
// hold FromCache values must NOT be released before the response is fully rendered.
func TestCrossArenaMergeValuesCreatesShallowReferences(t *testing.T) {
	old := debug.SetGCPercent(1)
	defer debug.SetGCPercent(old)

	mainArena := arena.NewMonotonicArena(arena.WithMinBufferSize(4096))
	goroutineArena := arena.NewMonotonicArena(arena.WithMinBufferSize(4096))

	// Parse entity data on the "goroutine" arena (simulates populateFromCache)
	fromCache, err := astjson.ParseBytesWithArena(goroutineArena, []byte(`{"id":"prod-1","name":"Widget"}`))
	require.NoError(t, err)

	// Parse the target item on the main arena (simulates the response tree)
	item, err := astjson.ParseBytesWithArena(mainArena, []byte(`{"id":"prod-1"}`))
	require.NoError(t, err)

	// Merge: this splices FromCache nodes into item's object tree
	merged, _, err := astjson.MergeValues(mainArena, item, fromCache)
	require.NoError(t, err)

	// Verify merged result contains data from both arenas
	mergedJSON := string(merged.MarshalTo(nil))
	assert.Contains(t, mergedJSON, `"name":"Widget"`)
	assert.Contains(t, mergedJSON, `"id":"prod-1"`)

	// Force GC to stress-test pointer validity — goroutine arena is still alive
	runtime.GC()
	runtime.GC()

	// Values should still be valid since goroutine arena hasn't been reset
	postGCJSON := string(merged.MarshalTo(nil))
	assert.Equal(t, mergedJSON, postGCJSON,
		"merged values should survive GC when goroutine arena is still alive")

	// Now reset the goroutine arena — simulates premature release
	goroutineArena.Reset()

	// Overwrite the freed memory with different data
	_, _ = astjson.ParseBytesWithArena(goroutineArena, []byte(`{"id":"STALE","name":"CORRUPTED"}`))

	// The merged tree still holds pointers into the (now overwritten) goroutine arena.
	// This proves MergeValues is shallow — accessing the stale data may panic or
	// return corrupted values.
	staleOrPanicked := func() (result string, panicked bool) {
		defer func() {
			if r := recover(); r != nil {
				panicked = true
			}
		}()
		return string(merged.MarshalTo(nil)), false
	}
	staleJSON, panicked := staleOrPanicked()
	assert.True(t, panicked || staleJSON != mergedJSON,
		"merged values should be stale or inaccessible after goroutine arena reset — "+
			"this proves MergeValues creates cross-arena shallow references")

	runtime.KeepAlive(mainArena)
	runtime.KeepAlive(goroutineArena)
}

// TestGoroutineArenaLifetimeWithDeferredRelease verifies the correct pattern:
// goroutine arenas survive through the full resolve lifecycle and are only
// released in Free(). This matches the Loader.goroutineArenas design.
func TestGoroutineArenaLifetimeWithDeferredRelease(t *testing.T) {
	old := debug.SetGCPercent(1)
	defer debug.SetGCPercent(old)

	mainArena := arena.NewMonotonicArena(arena.WithMinBufferSize(4096))

	// Simulate multiple goroutines, each with their own arena
	const numGoroutines = 4
	goroutineArenas := make([]arena.Arena, numGoroutines)
	fromCacheValues := make([]*astjson.Value, numGoroutines)

	for i := range numGoroutines {
		goroutineArenas[i] = arena.NewMonotonicArena(arena.WithMinBufferSize(4096))
		var err error
		fromCacheValues[i], err = astjson.ParseBytesWithArena(
			goroutineArenas[i],
			[]byte(`{"id":"prod-`+stringFromInt(i+1)+`","name":"Product `+stringFromInt(i+1)+`"}`),
		)
		require.NoError(t, err)
	}

	// Phase 4: merge all FromCache values into main arena tree
	items := make([]*astjson.Value, numGoroutines)
	for i := range numGoroutines {
		items[i], _ = astjson.ParseBytesWithArena(mainArena, []byte(`{"id":"prod-`+stringFromInt(i+1)+`"}`))
		merged, _, err := astjson.MergeValues(mainArena, items[i], fromCacheValues[i])
		require.NoError(t, err)
		items[i] = merged
	}

	// GC pressure — all arenas still alive
	runtime.GC()
	runtime.GC()

	// Verify all merged values are still valid (simulates response rendering)
	for i := range numGoroutines {
		json := string(items[i].MarshalTo(nil))
		assert.Contains(t, json, `"name":"Product `+stringFromInt(i+1)+`"`,
			"merged value %d should be readable with goroutine arenas alive", i)
	}

	// Now release goroutine arenas (simulates Loader.Free())
	for _, a := range goroutineArenas {
		a.Reset()
	}

	runtime.KeepAlive(mainArena)
	runtime.KeepAlive(goroutineArenas)
}

// Benchmark_CrossArenaGCSafety exercises the goroutine arena pattern under GC
// pressure. Each iteration creates goroutine arenas, merges values, renders the
// result, then releases. runtime.GC() between iterations maximizes pressure on
// any dangling pointers.
func Benchmark_CrossArenaGCSafety(b *testing.B) {
	old := debug.SetGCPercent(1)
	defer debug.SetGCPercent(old)

	entityJSON := []byte(`{"__typename":"Product","id":"prod-1","name":"Widget","price":9.99}`)
	itemJSON := []byte(`{"__typename":"Product","id":"prod-1"}`)

	b.ResetTimer()
	for b.Loop() {
		mainArena := arena.NewMonotonicArena(arena.WithMinBufferSize(4096))
		goroutineArena := arena.NewMonotonicArena(arena.WithMinBufferSize(4096))

		// Simulate goroutine: parse cached entity
		fromCache, err := astjson.ParseBytesWithArena(goroutineArena, entityJSON)
		if err != nil {
			b.Fatal(err)
		}

		// Simulate Phase 4: merge into response tree
		item, err := astjson.ParseBytesWithArena(mainArena, itemJSON)
		if err != nil {
			b.Fatal(err)
		}
		merged, _, err := astjson.MergeValues(mainArena, item, fromCache)
		if err != nil {
			b.Fatal(err)
		}

		// Simulate response rendering
		buf := merged.MarshalTo(nil)
		if len(buf) == 0 {
			b.Fatal("empty output")
		}

		// Release (correct order: goroutine arena after rendering)
		goroutineArena.Reset()
		mainArena.Reset()

		// GC pressure between iterations
		runtime.GC()
	}
}
