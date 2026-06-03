// Benchmarks for the 4 cache-hit merge sites that currently StructuralCopy
// from the cache before merging into the response tree. Matched with the
// invariant tests in loader_cache_copy_invariant_test.go:
//
//   - loader.go:1220 — mergeBatchCacheHit             → BenchmarkMergeBatchCacheHit
//   - loader.go:1372 — mergeBatchPartialResponse      → BenchmarkMergeBatchPartialResponse
//   - loader.go:1472 — mergeResult cacheSkipFetch     → BenchmarkMergeResultCacheSkipFetch
//   - loader.go:1491 — mergeResult partialCacheEnabled → BenchmarkMergeResultPartialCache
//
// Each benchmark runs with entity counts {1, 10, 100} to expose how per-copy
// cost scales with batch size. Uses b.ReportAllocs() so ns/op, allocs/op, B/op
// are captured.
//
// Usage:
//
//	go test -run=^$ -bench BenchmarkMerge -benchmem ./v2/pkg/engine/resolve/...
package resolve

import (
	"context"
	"strconv"
	"testing"

	"github.com/wundergraph/astjson"
	"github.com/wundergraph/go-arena"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
)

// benchCopyEntityJSON is a realistic nested entity shape used across all
// cache-copy benches. Matches the shape used in the invariant tests.
func benchCopyEntityJSON(id string) []byte {
	return []byte(`{"__typename":"User","id":"` + id + `","name":"User ` + id + `","profile":{"email":"` + id + `@example.com","age":30,"bio":"Lorem ipsum dolor sit amet"},"tags":["a","b","c"]}`)
}

var benchCopyEntityCounts = []int{1, 10, 100}

func newBenchCopyLoader() (*Loader, arena.Arena) {
	ar := arena.NewMonotonicArena(arena.WithMinBufferSize(64 * 1024))
	ctx := NewContext(context.Background())
	ctx.ExecutionOptions.Caching.EnableL2Cache = true
	resolvable := NewResolvable(ar, ResolvableOptions{})
	if err := resolvable.Init(ctx, nil, ast.OperationTypeQuery); err != nil {
		panic(err)
	}
	return &Loader{
		jsonArena:  ar,
		resolvable: resolvable,
		ctx:        ctx,
	}, ar
}

// BenchmarkMergeBatchCacheHit exercises loader.go:1220.
// The loader splices N cached entities into a response array via
// entityArray.SetArrayItem(arena, idx, StructuralCopy(entity)).
func BenchmarkMergeBatchCacheHit(b *testing.B) {
	for _, n := range benchCopyEntityCounts {
		b.Run("entities="+strconv.Itoa(n), func(b *testing.B) {
			// Cache-backing arena: holds cached *astjson.Value across iterations.
			// Never Reset so pointers stay valid.
			cacheArena := arena.NewMonotonicArena(arena.WithMinBufferSize(64 * 1024))
			cached := make([]*astjson.Value, n)
			for i := range n {
				v, err := astjson.ParseBytesWithArena(cacheArena,
					[]byte(`{"users":`+string(benchCopyEntityJSON(strconv.Itoa(i)))+`}`))
				if err != nil {
					b.Fatal(err)
				}
				cached[i] = v
			}

			l, ar := newBenchCopyLoader()

			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				ar.Reset()
				// Rebuild cacheKeys each iteration (cheap, not the measurement target).
				cacheKeys := make([]*CacheKey, n)
				for i := range n {
					cacheKeys[i] = &CacheKey{
						BatchIndex:      i,
						FromCache:       cached[i],
						Keys:            []string{"key" + strconv.Itoa(i)},
						EntityMergePath: []string{"users"},
					}
				}
				res := &result{l2CacheKeys: cacheKeys}
				if err := l.mergeBatchCacheHit(&FetchItem{}, res, nil); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// BenchmarkMergeBatchPartialResponse exercises loader.go:1372.
// Half the entities are cache hits (spliced via StructuralCopy), half come
// from the fresh subgraph response (no copy).
func BenchmarkMergeBatchPartialResponse(b *testing.B) {
	for _, n := range benchCopyEntityCounts {
		b.Run("entities="+strconv.Itoa(n), func(b *testing.B) {
			cacheArena := arena.NewMonotonicArena(arena.WithMinBufferSize(64 * 1024))
			cachedCount := n / 2
			if cachedCount == 0 {
				cachedCount = 1
			}
			cached := make([]*astjson.Value, cachedCount)
			for i := range cachedCount {
				v, err := astjson.ParseBytesWithArena(cacheArena, benchCopyEntityJSON("c"+strconv.Itoa(i)))
				if err != nil {
					b.Fatal(err)
				}
				cached[i] = v
			}

			// Pre-build fresh-response JSON: entities at indices [cachedCount, n).
			freshJSON := []byte(`{"users":[`)
			for i := cachedCount; i < n; i++ {
				if i > cachedCount {
					freshJSON = append(freshJSON, ',')
				}
				freshJSON = append(freshJSON, benchCopyEntityJSON("f"+strconv.Itoa(i))...)
			}
			freshJSON = append(freshJSON, `]}`...)

			cachedIndices := make([]int, cachedCount)
			for i := range cachedCount {
				cachedIndices[i] = i
			}
			missedIndices := make([]int, 0, n-cachedCount)
			for i := cachedCount; i < n; i++ {
				missedIndices = append(missedIndices, i)
			}

			l, ar := newBenchCopyLoader()
			info := &FetchInfo{RootFields: []GraphCoordinate{{FieldName: "users"}}}

			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				ar.Reset()
				freshResp, err := astjson.ParseBytesWithArena(ar, freshJSON)
				if err != nil {
					b.Fatal(err)
				}
				cacheKeys := make([]*CacheKey, cachedCount)
				for i := range cachedCount {
					cacheKeys[i] = &CacheKey{
						BatchIndex: i,
						FromCache:  cached[i],
						Keys:       []string{"key" + strconv.Itoa(i)},
					}
				}
				res := &result{
					l2CacheKeys:        cacheKeys,
					batchCachedIndices: cachedIndices,
					batchMissedIndices: missedIndices,
				}
				l.mergeBatchPartialResponse(res, []*astjson.Value{freshResp}, info)
			}
		})
	}
}

// BenchmarkMergeResultCacheSkipFetch exercises loader.go:1472.
// N L1 hits, each StructuralCopy'd before MergeValues into the response item.
func BenchmarkMergeResultCacheSkipFetch(b *testing.B) {
	for _, n := range benchCopyEntityCounts {
		b.Run("entities="+strconv.Itoa(n), func(b *testing.B) {
			cacheArena := arena.NewMonotonicArena(arena.WithMinBufferSize(64 * 1024))
			cached := make([]*astjson.Value, n)
			for i := range n {
				v, err := astjson.ParseBytesWithArena(cacheArena, benchCopyEntityJSON(strconv.Itoa(i)))
				if err != nil {
					b.Fatal(err)
				}
				cached[i] = v
			}

			l, ar := newBenchCopyLoader()

			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				ar.Reset()
				// Fresh items per iteration — arena reset invalidates the previous ones.
				items := make([]*astjson.Value, n)
				l1Keys := make([]*CacheKey, n)
				for i := range n {
					item, err := astjson.ParseBytesWithArena(ar, []byte(`{"id":"`+strconv.Itoa(i)+`"}`))
					if err != nil {
						b.Fatal(err)
					}
					items[i] = item
					l1Keys[i] = &CacheKey{
						Item:      item,
						FromCache: cached[i],
						Keys:      []string{"key" + strconv.Itoa(i)},
					}
				}
				res := &result{
					cacheSkipFetch:     true,
					batchEntityKeyMode: false,
					l1CacheKeys:        l1Keys,
				}
				if err := l.mergeResult(&FetchItem{}, res, items); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// BenchmarkMergeResultPartialCache exercises loader.go:1491.
// N L1 hits, merged via the partialCacheEnabled branch (fetchSkipped=true to
// short-circuit the rest of mergeResult).
func BenchmarkMergeResultPartialCache(b *testing.B) {
	for _, n := range benchCopyEntityCounts {
		b.Run("entities="+strconv.Itoa(n), func(b *testing.B) {
			cacheArena := arena.NewMonotonicArena(arena.WithMinBufferSize(64 * 1024))
			cached := make([]*astjson.Value, n)
			for i := range n {
				v, err := astjson.ParseBytesWithArena(cacheArena, benchCopyEntityJSON(strconv.Itoa(i)))
				if err != nil {
					b.Fatal(err)
				}
				cached[i] = v
			}

			cachedIndices := make([]int, n)
			for i := range n {
				cachedIndices[i] = i
			}

			l, ar := newBenchCopyLoader()

			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				ar.Reset()
				items := make([]*astjson.Value, n)
				l1Keys := make([]*CacheKey, n)
				for i := range n {
					item, err := astjson.ParseBytesWithArena(ar, []byte(`{"id":"`+strconv.Itoa(i)+`"}`))
					if err != nil {
						b.Fatal(err)
					}
					items[i] = item
					l1Keys[i] = &CacheKey{
						Item:      item,
						FromCache: cached[i],
						Keys:      []string{"key" + strconv.Itoa(i)},
					}
				}
				res := &result{
					partialCacheEnabled: true,
					cachedItemIndices:   cachedIndices,
					l1CacheKeys:         l1Keys,
					fetchSkipped:        true,
				}
				if err := l.mergeResult(&FetchItem{}, res, items); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
