package planbytecode

import (
	"testing"

	"github.com/wundergraph/astjson"
	arena "github.com/wundergraph/go-arena"
)

var benchmarkSink int

func BenchmarkJSONMergeASTJSON(b *testing.B) {
	src := []byte(`{"id":"1","name":"Ada","reviews":[{"body":"ok","meta":{"score":10}},{"body":"fine","meta":{"score":8}}],"extra":true}`)
	targetBytes := []byte(`{"id":"1"}`)
	a := arena.NewMonotonicArena(arena.WithMinBufferSize(64 * 1024))
	defer a.Release()

	b.ReportAllocs()
	b.SetBytes(int64(len(src)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		a.Reset()
		target, err := astjson.ParseBytesWithArena(a, targetBytes)
		if err != nil {
			b.Fatal(err)
		}
		response, err := astjson.ParseBytesWithArena(a, src)
		if err != nil {
			b.Fatal(err)
		}
		merged, _, err := astjson.MergeValuesWithPath(a, target, response)
		if err != nil {
			b.Fatal(err)
		}
		benchmarkSink += int(merged.Type())
	}
}

func BenchmarkJSONPasteByteRanges(b *testing.B) {
	src := []byte(`{"id":"1","name":"Ada","reviews":[{"body":"ok","meta":{"score":10}},{"body":"fine","meta":{"score":8}}],"extra":true}`)
	fields := []string{"id", "name", "reviews"}
	rangesScratch := make([]ByteRange, 0, len(fields))
	out := make([]byte, 0, len(src))

	b.ReportAllocs()
	b.SetBytes(int64(len(src)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ranges, ok := ScanObjectFieldRanges(src, fields, rangesScratch[:0])
		if !ok {
			b.Fatal("scan failed")
		}
		out = AppendObjectFromRanges(out[:0], src, ranges)
		benchmarkSink += len(out)
	}
}
