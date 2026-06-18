package resolve

import (
	"context"
	"fmt"
	"testing"

	"github.com/wundergraph/astjson"
)

var loaderCacheCopyBenchSink *astjson.Value

func BenchmarkLoaderCacheCopy_MergeBatchCacheHit(b *testing.B) {
	for _, count := range []int{1, 10, 100} {
		b.Run(fmt.Sprintf("Entities%d", count), func(b *testing.B) {
			benchmarkLoaderCacheCopyMergeBatchCacheHit(b, count)
		})
	}
}

func BenchmarkLoaderCacheCopy_MergeBatchPartialResponse(b *testing.B) {
	for _, count := range []int{1, 10, 100} {
		b.Run(fmt.Sprintf("Entities%d", count), func(b *testing.B) {
			benchmarkLoaderCacheCopyMergeBatchPartialResponse(b, count)
		})
	}
}

func BenchmarkLoaderCacheCopy_MergeResultCacheSkipFetch(b *testing.B) {
	for _, count := range []int{1, 10, 100} {
		b.Run(fmt.Sprintf("Entities%d", count), func(b *testing.B) {
			benchmarkLoaderCacheCopyMergeResultCacheSkipFetch(b, count)
		})
	}
}

func BenchmarkLoaderCacheCopy_MergeResultPartialCache(b *testing.B) {
	for _, count := range []int{1, 10, 100} {
		b.Run(fmt.Sprintf("Entities%d", count), func(b *testing.B) {
			benchmarkLoaderCacheCopyMergeResultPartialCache(b, count)
		})
	}
}

func benchmarkLoaderCacheCopyMergeBatchCacheHit(b *testing.B, count int) {
	sourceLoader, releaseSource := newLoaderCacheTransformTestLoader()
	defer releaseSource()
	loader, releaseLoader := newLoaderCacheBenchLoader()
	defer releaseLoader()

	cached := make([]*astjson.Value, count)
	for i := range count {
		cached[i] = parseLoaderCacheBenchValue(b, sourceLoader, benchUserValueJSON(i, "Ada"))
	}

	cacheKeys := make([]*CacheKey, count)
	batchStats := make([][]*astjson.Value, count)

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		loader.jsonArena.Reset()
		for i := range count {
			batchStats[i] = []*astjson.Value{parseLoaderCacheBenchValue(b, loader, benchUserStubJSON(i))}
			cacheKeys[i] = &CacheKey{FromCache: cached[i]}
		}
		res := &result{
			batchStats: batchStats,
			cacheKeys:  cacheKeys,
		}
		if err := loader.mergeBatchCacheHits(&FetchItem{}, res); err != nil {
			b.Fatal(err)
		}
		loaderCacheCopyBenchSink = batchStats[count-1][0]
	}
}

func benchmarkLoaderCacheCopyMergeBatchPartialResponse(b *testing.B, count int) {
	sourceLoader, releaseSource := newLoaderCacheTransformTestLoader()
	defer releaseSource()
	loader, releaseLoader := newLoaderCacheBenchLoader()
	defer releaseLoader()

	cached := make([]*astjson.Value, count)
	fetched := make([]*astjson.Value, count)
	for i := range count {
		cached[i] = parseLoaderCacheBenchValue(b, sourceLoader, benchUserValueJSON(i, "Ada"))
		fetched[i] = parseLoaderCacheBenchValue(b, sourceLoader, benchUserValueJSON(i, "Grace"))
	}

	cacheKeys := make([]*CacheKey, count)
	batchStats := make([][]*astjson.Value, count)

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		loader.jsonArena.Reset()
		for i := range count {
			batchStats[i] = []*astjson.Value{parseLoaderCacheBenchValue(b, loader, benchUserStubJSON(i))}
			if i%2 == 0 {
				cacheKeys[i] = &CacheKey{FromCache: cached[i]}
			} else {
				cacheKeys[i] = &CacheKey{}
			}
		}
		res := &result{
			batchStats: batchStats,
			cacheKeys:  cacheKeys,
		}
		if err := loader.mergeBatchCacheHits(&FetchItem{}, res); err != nil {
			b.Fatal(err)
		}
		for i := range count {
			if i%2 == 0 {
				continue
			}
			if err := loader.mergeBatchFetchedValue(&FetchItem{}, res, i, fetched[i]); err != nil {
				b.Fatal(err)
			}
		}
		loaderCacheCopyBenchSink = batchStats[count-1][0]
	}
}

func benchmarkLoaderCacheCopyMergeResultCacheSkipFetch(b *testing.B, count int) {
	sourceLoader, releaseSource := newLoaderCacheTransformTestLoader()
	defer releaseSource()
	loader, releaseLoader := newLoaderCacheBenchLoader()
	defer releaseLoader()

	cached := make([]*astjson.Value, count)
	for i := range count {
		cached[i] = parseLoaderCacheBenchValue(b, sourceLoader, benchUserValueJSON(i, "Ada"))
	}

	cacheKeys := make([]*CacheKey, count)
	items := make([]*astjson.Value, count)

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		loader.jsonArena.Reset()
		for i := range count {
			items[i] = parseLoaderCacheBenchValue(b, loader, benchUserStubJSON(i))
			cacheKeys[i] = &CacheKey{FromCache: cached[i]}
		}
		res := &result{
			cacheSkipFetch: true,
			cacheKeys:      cacheKeys,
		}
		if err := loader.mergeResult(&FetchItem{}, res, items); err != nil {
			b.Fatal(err)
		}
		loaderCacheCopyBenchSink = items[count-1]
	}
}

func benchmarkLoaderCacheCopyMergeResultPartialCache(b *testing.B, count int) {
	sourceLoader, releaseSource := newLoaderCacheTransformTestLoader()
	defer releaseSource()
	loader, releaseLoader := newLoaderCacheBenchLoader()
	defer releaseLoader()
	loader.ctx.ExecutionOptions.Caching.EnableL1Cache = true

	l1Values := make([]*astjson.Value, count)
	for i := range count {
		l1Values[i] = parseLoaderCacheBenchValue(b, sourceLoader, benchUserValueJSON(i, "Ada"))
	}

	cache := &FetchCacheConfiguration{
		UseL1Cache:  true,
		KeyTemplate: cacheTestUserKeyTemplate(),
		ProvidesData: &Object{
			Fields: []*Field{
				{Name: []byte("__typename"), Value: &String{}},
				{Name: []byte("id"), Value: &String{}},
				{Name: []byte("name"), Value: &String{}},
				{Name: []byte("profile"), Value: &Object{Fields: []*Field{{Name: []byte("rank"), Value: &Integer{}}}}},
			},
		},
	}
	cacheKeys := make([]*CacheKey, count)
	batchStats := make([][]*astjson.Value, count)
	items := make([]*astjson.Value, count)

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		loader.jsonArena.Reset()
		loader.l1Cache = make(map[string]*astjson.Value, count)
		for i := range count {
			key := benchUserCacheKey(i)
			loader.l1Cache[key] = l1Values[i]
			items[i] = parseLoaderCacheBenchValue(b, loader, benchUserStubJSON(i))
			batchStats[i] = []*astjson.Value{items[i]}
			cacheKeys[i] = &CacheKey{Keys: []string{key}}
		}
		res := &result{
			postProcessing: PostProcessingConfiguration{
				SelectResponseDataPath: []string{"data", "_entities"},
			},
			batchStats: batchStats,
			out:        benchUserEntitiesResponse(count, "Grace"),
			cacheKeys:  cacheKeys,
		}
		if err := loader.mergeResult(&FetchItem{Fetch: &BatchEntityFetch{Cache: cache}}, res, items); err != nil {
			b.Fatal(err)
		}
		loaderCacheCopyBenchSink = items[count-1]
	}
}

func newLoaderCacheBenchLoader() (*Loader, func()) {
	loader, release := newLoaderCacheTransformTestLoader()
	loader.ctx = NewContext(context.Background())
	loader.ctx.ExecutionOptions.Caching = CachingOptions{
		EnableL1Cache: false,
		EnableL2Cache: false,
	}
	return loader, release
}

func parseLoaderCacheBenchValue(b *testing.B, loader *Loader, data []byte) *astjson.Value {
	b.Helper()

	value, err := astjson.ParseBytesWithArena(loader.jsonArena, data)
	if err != nil {
		b.Fatal(err)
	}
	return value
}

func benchUserStubJSON(i int) []byte {
	return []byte(fmt.Sprintf(`{"__typename":"User","id":"%d"}`, i))
}

func benchUserValueJSON(i int, name string) []byte {
	return []byte(fmt.Sprintf(`{"__typename":"User","id":"%d","name":"%s","profile":{"rank":%d}}`, i, name, i+1))
}

func benchUserCacheKey(i int) string {
	return fmt.Sprintf(`{"__typename":"User","key":{"id":"%d"}}`, i)
}

func benchUserEntitiesResponse(count int, name string) []byte {
	out := make([]byte, 0, 64+count*80)
	out = append(out, `{"data":{"_entities":[`...)
	for i := range count {
		if i != 0 {
			out = append(out, ',')
		}
		out = append(out, benchUserValueJSON(i, name)...)
	}
	out = append(out, `]}}`...)
	return out
}
