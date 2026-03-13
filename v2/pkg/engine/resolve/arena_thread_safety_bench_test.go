package resolve

import (
	"strconv"
	"sync"
	"testing"

	"github.com/wundergraph/astjson"
	"github.com/wundergraph/go-arena"
)

// cacheLoadAllocs simulates the allocation pattern of tryL2CacheLoad:
// parse cached JSON bytes, create wrapper objects, allocate slices.
func cacheLoadAllocs(a arena.Arena) {
	// 1. extractCacheKeysStrings: allocate slice + string bytes
	keys := arena.AllocateSlice[string](a, 0, 4)
	for range 4 {
		buf := arena.AllocateSlice[byte](a, 0, 64)
		buf = arena.SliceAppend(a, buf, []byte("cache:entity:Product:id:prod-1234")...)
		keys = arena.SliceAppend(a, keys, string(buf))
	}
	_ = keys

	// 2. populateFromCache: parse JSON bytes
	v, _ := astjson.ParseBytesWithArena(a, []byte(`{"__typename":"Product","id":"prod-1234","name":"Test Product","price":29.99}`))

	// 3. EntityMergePath wrapping: create wrapper objects
	obj := astjson.ObjectValue(a)
	obj.Set(a, "product", v)
	outer := astjson.ObjectValue(a)
	outer.Set(a, "data", obj)

	// 4. denormalizeFromCache: create new object tree
	result := astjson.ObjectValue(a)
	result.Set(a, "productName", v.Get("name"))
	result.Set(a, "productPrice", v.Get("price"))
}

// BenchmarkConcurrentArena measures Option A: single arena wrapped with NewConcurrentArena.
// All goroutines allocate from the same mutex-protected arena.
func BenchmarkConcurrentArena(b *testing.B) {
	for _, goroutines := range []int{1, 4, 8, 16} {
		b.Run(goroutineName(goroutines), func(b *testing.B) {
			a := arena.NewConcurrentArena(arena.NewMonotonicArena(arena.WithMinBufferSize(64 * 1024)))
			b.ResetTimer()
			for b.Loop() {
				var wg sync.WaitGroup
				for range goroutines {
					wg.Go(func() {
						cacheLoadAllocs(a)
					})
				}
				wg.Wait()
				a.Reset()
			}
		})
	}
}

// BenchmarkPerGoroutineArena measures Option B: each goroutine gets its own arena from sync.Pool.
// Zero lock contention on allocations.
func BenchmarkPerGoroutineArena(b *testing.B) {
	pool := sync.Pool{
		New: func() any {
			return arena.NewMonotonicArena(arena.WithMinBufferSize(4096))
		},
	}

	for _, goroutines := range []int{1, 4, 8, 16} {
		b.Run(goroutineName(goroutines), func(b *testing.B) {
			b.ResetTimer()
			for b.Loop() {
				arenas := make([]arena.Arena, goroutines)
				var wg sync.WaitGroup
				for i := range goroutines {
					ga := pool.Get().(arena.Arena)
					arenas[i] = ga
					wg.Go(func() {
						cacheLoadAllocs(ga)
					})
				}
				wg.Wait()
				for _, ga := range arenas {
					ga.Reset()
					pool.Put(ga)
				}
			}
		})
	}
}

func goroutineName(n int) string {
	return "goroutines=" + stringFromInt(n)
}

func stringFromInt(n int) string {
	return strconv.Itoa(n)
}
