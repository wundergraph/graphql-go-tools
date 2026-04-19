// Benchmarks for the non-caching fetch/merge path.
//
// The non-caching path has no StructuralCopy calls — the theoretical minimum
// is one parse (ParseBytesWithArena) + one merge (MergeValuesWithPath) per
// fetch. These benches measure that floor so we can identify hotspots in
// auxiliary work (res struct allocation, response buffer handling, merge
// pathology for large responses, etc.) separately from the caching work.
//
// Two shapes are measured:
//
//   - BenchmarkNonCachingParseMergeCore — raw ParseBytesWithArena +
//     MergeValuesWithPath, bypassing mergeResult's boilerplate. This is the
//     absolute lower bound.
//   - BenchmarkNonCachingMergeResult — the full mergeResult call with
//     caching disabled. This includes all the non-cache branches (rejected
//     check, response path extraction, error path, etc.) so the delta vs.
//     Core reveals how much overhead mergeResult itself adds on the hot
//     non-caching path.
//
// Usage:
//
//	go test -run=^$ -bench BenchmarkNonCaching -benchmem ./v2/pkg/engine/resolve/...
package resolve

import (
	"context"
	"strconv"
	"strings"
	"testing"

	"github.com/wundergraph/astjson"
	"github.com/wundergraph/go-arena"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
)

var benchNonCachingEntityCounts = []int{1, 10, 100}

// buildNonCachingResponse returns a realistic subgraph JSON response wrapping
// N entities under data.users.
func buildNonCachingResponse(n int) []byte {
	var sb strings.Builder
	sb.WriteString(`{"data":{"users":[`)
	for i := range n {
		if i > 0 {
			sb.WriteByte(',')
		}
		sb.Write(benchCopyEntityJSON(strconv.Itoa(i)))
	}
	sb.WriteString(`]}}`)
	return []byte(sb.String())
}

// BenchmarkNonCachingParseMergeCore measures the raw ParseBytesWithArena +
// MergeValuesWithPath hot loop. This is the floor — no caching, no mergeResult
// boilerplate, no error handling beyond the primitives themselves.
func BenchmarkNonCachingParseMergeCore(b *testing.B) {
	for _, n := range benchNonCachingEntityCounts {
		b.Run("entities="+strconv.Itoa(n), func(b *testing.B) {
			responseJSON := buildNonCachingResponse(n)
			ar := arena.NewMonotonicArena(arena.WithMinBufferSize(64 * 1024))

			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				ar.Reset()
				parsed, err := astjson.ParseBytesWithArena(ar, responseJSON)
				if err != nil {
					b.Fatal(err)
				}
				responseData := parsed.Get("data")
				// Root-level merge with no pre-existing items → set resolvable.data.
				// Mimic the real mergeResult behavior with an empty placeholder
				// to exercise MergeValuesWithPath identically to the fetch path.
				item, err := astjson.ParseBytesWithArena(ar, []byte(`{}`))
				if err != nil {
					b.Fatal(err)
				}
				_, err = astjson.MergeValuesWithPath(ar, item, responseData)
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// BenchmarkNonCachingMergeResult measures the full mergeResult path with
// caching disabled on the context. Compared to BenchmarkNonCachingParseMergeCore
// the delta reveals how much non-cache overhead mergeResult contributes.
func BenchmarkNonCachingMergeResult(b *testing.B) {
	for _, n := range benchNonCachingEntityCounts {
		b.Run("entities="+strconv.Itoa(n), func(b *testing.B) {
			responseJSON := buildNonCachingResponse(n)

			ar := arena.NewMonotonicArena(arena.WithMinBufferSize(64 * 1024))
			ctx := NewContext(context.Background())
			ctx.ExecutionOptions.Caching.EnableL1Cache = false
			ctx.ExecutionOptions.Caching.EnableL2Cache = false
			resolvable := NewResolvable(ar, ResolvableOptions{})
			if err := resolvable.Init(ctx, nil, ast.OperationTypeQuery); err != nil {
				b.Fatal(err)
			}
			l := &Loader{
				jsonArena:  ar,
				resolvable: resolvable,
				ctx:        ctx,
			}

			fetchItem := &FetchItem{
				Fetch: &SingleFetch{
					FetchConfiguration: FetchConfiguration{
						PostProcessing: PostProcessingConfiguration{
							SelectResponseDataPath: []string{"data"},
						},
					},
					Info: &FetchInfo{OperationType: ast.OperationTypeQuery},
				},
			}

			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				ar.Reset()
				item, err := astjson.ParseBytesWithArena(ar, []byte(`{}`))
				if err != nil {
					b.Fatal(err)
				}
				res := &result{
					out: responseJSON,
					postProcessing: PostProcessingConfiguration{
						SelectResponseDataPath: []string{"data"},
					},
				}
				if err := l.mergeResult(fetchItem, res, []*astjson.Value{item}); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
