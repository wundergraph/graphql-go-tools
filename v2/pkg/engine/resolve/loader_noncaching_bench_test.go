package resolve

import (
	"fmt"
	"testing"

	"github.com/wundergraph/astjson"
)

var loaderNoncachingBenchSink *astjson.Value

func BenchmarkLoaderNoncaching_ParseMergeCore(b *testing.B) {
	for _, count := range []int{1, 10, 100} {
		b.Run(fmt.Sprintf("Entities%d", count), func(b *testing.B) {
			benchmarkLoaderNoncachingParseMergeCore(b, count)
		})
	}
}

func BenchmarkLoaderNoncaching_MergeResult(b *testing.B) {
	for _, count := range []int{1, 10, 100} {
		b.Run(fmt.Sprintf("Entities%d", count), func(b *testing.B) {
			benchmarkLoaderNoncachingMergeResult(b, count)
		})
	}
}

func benchmarkLoaderNoncachingParseMergeCore(b *testing.B, count int) {
	loader, release := newLoaderCacheBenchLoader()
	defer release()

	response := benchUserEntitiesResponse(count, "Ada")
	items := make([]*astjson.Value, count)

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		loader.jsonArena.Reset()
		value := parseLoaderCacheBenchValue(b, loader, response)
		batch := value.Get("data", "_entities").GetArray()
		for i := range count {
			items[i] = parseLoaderCacheBenchValue(b, loader, benchUserStubJSON(i))
			var err error
			items[i], err = astjson.MergeValuesWithPath(loader.jsonArena, items[i], batch[i])
			if err != nil {
				b.Fatal(err)
			}
		}
		loaderNoncachingBenchSink = items[count-1]
	}
}

func benchmarkLoaderNoncachingMergeResult(b *testing.B, count int) {
	loader, release := newLoaderCacheBenchLoader()
	defer release()

	response := benchUserEntitiesResponse(count, "Ada")
	batchStats := make([][]*astjson.Value, count)
	items := make([]*astjson.Value, count)

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		loader.jsonArena.Reset()
		for i := range count {
			items[i] = parseLoaderCacheBenchValue(b, loader, benchUserStubJSON(i))
			batchStats[i] = []*astjson.Value{items[i]}
		}
		res := &result{
			postProcessing: PostProcessingConfiguration{
				SelectResponseDataPath: []string{"data", "_entities"},
			},
			batchStats: batchStats,
			out:        response,
		}
		if err := loader.mergeResult(&FetchItem{}, res, items); err != nil {
			b.Fatal(err)
		}
		loaderNoncachingBenchSink = items[count-1]
	}
}
