package resolve

// Benchmarks for the L1/L2 cache copy primitives.
//
// These target the four StructuralCopy helpers in loader_cache_transform.go
// plus the L2 wire-format MarshalTo path, both with and without an alias
// Transform, to isolate the overhead of alias/arg-suffix normalization
// from the plain structural copy.
//
// Mapping to production call sites (loader_cache.go):
//   L1Write     -> structuralCopyNormalizedPassthrough (populateL1Cache)
//   L1Read      -> structuralCopyDenormalizedPassthrough (tryL1CacheLoad)
//   L2Read      -> ParseBytesWithArena + structuralCopyDenormalized (applyEntityFetchL2Results)
//   L2Write     -> MarshalTo (cacheKeysToEntriesBatch) — no transform in prod, since the
//                  L1-stored value is already schema-shape. The "WithTransform" variant models
//                  the hypothetical "normalize-and-serialize" cost of writing an aliased
//                  response value directly to L2.

import (
	"testing"

	"github.com/wundergraph/astjson"
	"github.com/wundergraph/go-arena"
)

// Representative entity payload: 10 fields, 4 aliased, mix of scalars + small nested array.
// Response shape (with aliases) — what the subgraph returned verbatim.
const benchEntityResponseShape = `{` +
	`"__typename":"Product",` +
	`"id":"p-00000001",` +
	`"n":"Wireless Headphones Pro X",` +
	`"p":249.99,` +
	`"in_stock":true,` +
	`"category":"electronics",` +
	`"desc":"Premium noise-cancelling wireless headphones with 40h battery life.",` +
	`"created_at":"2024-01-15T10:30:00Z",` +
	`"updated_at":"2024-03-22T14:05:12Z",` +
	`"tag_list":["audio","wireless","premium","bestseller"]` +
	`}`

// Schema shape — what's stored in L1/L2 after normalization.
const benchEntitySchemaShape = `{` +
	`"__typename":"Product",` +
	`"id":"p-00000001",` +
	`"name":"Wireless Headphones Pro X",` +
	`"price":249.99,` +
	`"in_stock":true,` +
	`"category":"electronics",` +
	`"description":"Premium noise-cancelling wireless headphones with 40h battery life.",` +
	`"created_at":"2024-01-15T10:30:00Z",` +
	`"updated_at":"2024-03-22T14:05:12Z",` +
	`"tags":["audio","wireless","premium","bestseller"]` +
	`}`

// benchAliasedObject describes the entity for the Transform builder.
// 4 of the 10 fields are aliased: n/p/desc/tag_list.
func benchAliasedObject() *Object {
	return &Object{
		HasAliases: true,
		Fields: []*Field{
			{Name: []byte("__typename"), Value: &String{}},
			{Name: []byte("id"), Value: &String{}},
			{Name: []byte("n"), OriginalName: []byte("name"), Value: &String{}},
			{Name: []byte("p"), OriginalName: []byte("price"), Value: &Float{}},
			{Name: []byte("in_stock"), Value: &Boolean{}},
			{Name: []byte("category"), Value: &String{}},
			{Name: []byte("desc"), OriginalName: []byte("description"), Value: &String{}},
			{Name: []byte("created_at"), Value: &String{}},
			{Name: []byte("updated_at"), Value: &String{}},
			{Name: []byte("tag_list"), OriginalName: []byte("tags"), Value: &Array{Item: &String{}}},
		},
	}
}

// benchNoAliasObject — same fields with no aliases. HasAliases=false routes
// all helpers to plain StructuralCopy (no Transform built).
func benchNoAliasObject() *Object {
	return &Object{
		HasAliases: false,
		Fields: []*Field{
			{Name: []byte("__typename"), Value: &String{}},
			{Name: []byte("id"), Value: &String{}},
			{Name: []byte("name"), Value: &String{}},
			{Name: []byte("price"), Value: &Float{}},
			{Name: []byte("in_stock"), Value: &Boolean{}},
			{Name: []byte("category"), Value: &String{}},
			{Name: []byte("description"), Value: &String{}},
			{Name: []byte("created_at"), Value: &String{}},
			{Name: []byte("updated_at"), Value: &String{}},
			{Name: []byte("tags"), Value: &Array{Item: &String{}}},
		},
	}
}

// newBenchLoader builds a Loader with a fresh target arena. The parser's
// scratch slabs and transform slabs amortize across iterations, mirroring prod.
func newBenchLoader() (*Loader, arena.Arena) {
	ar := arena.NewMonotonicArena(arena.WithMinBufferSize(4096))
	return &Loader{jsonArena: ar}, ar
}

// parseOnto parses src onto a's arena using a fresh parser (one-shot).
func parseOnto(a arena.Arena, src []byte) *astjson.Value {
	v, err := astjson.ParseBytesWithArena(a, src)
	if err != nil {
		panic(err)
	}
	return v
}

// ---------- L1 Write ----------

// BenchmarkStructuralCopy_L1Write_NoTransform:
// populateL1Cache path when the response has no aliases —
// structuralCopyNormalizedPassthrough degenerates to plain StructuralCopy.
func BenchmarkStructuralCopy_L1Write_NoTransform(b *testing.B) {
	sourceAr := arena.NewMonotonicArena(arena.WithMinBufferSize(4096))
	src := parseOnto(sourceAr, []byte(benchEntitySchemaShape))
	obj := benchNoAliasObject()

	l, ar := newBenchLoader()
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_ = l.structuralCopyNormalizedPassthrough(src, obj)
		ar.Reset()
	}
}

// BenchmarkStructuralCopy_L1Write_WithTransform:
// populateL1Cache path with alias normalization — the hot path for any
// query that aliases entity fields.
func BenchmarkStructuralCopy_L1Write_WithTransform(b *testing.B) {
	sourceAr := arena.NewMonotonicArena(arena.WithMinBufferSize(4096))
	src := parseOnto(sourceAr, []byte(benchEntityResponseShape))
	obj := benchAliasedObject()

	l, ar := newBenchLoader()
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_ = l.structuralCopyNormalizedPassthrough(src, obj)
		ar.Reset()
	}
}

// ---------- L1 Read ----------

// BenchmarkStructuralCopy_L1Read_NoTransform:
// tryL1CacheLoad path with no aliases — plain StructuralCopy.
func BenchmarkStructuralCopy_L1Read_NoTransform(b *testing.B) {
	sourceAr := arena.NewMonotonicArena(arena.WithMinBufferSize(4096))
	src := parseOnto(sourceAr, []byte(benchEntitySchemaShape))
	obj := benchNoAliasObject()

	l, ar := newBenchLoader()
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_ = l.structuralCopyDenormalizedPassthrough(src, obj)
		ar.Reset()
	}
}

// BenchmarkStructuralCopy_L1Read_WithTransform:
// tryL1CacheLoad path with alias denormalization — re-applies the request's
// aliases to the schema-shape stored value.
func BenchmarkStructuralCopy_L1Read_WithTransform(b *testing.B) {
	sourceAr := arena.NewMonotonicArena(arena.WithMinBufferSize(4096))
	src := parseOnto(sourceAr, []byte(benchEntitySchemaShape))
	obj := benchAliasedObject()

	l, ar := newBenchLoader()
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_ = l.structuralCopyDenormalizedPassthrough(src, obj)
		ar.Reset()
	}
}

// ---------- L2 Read (parse + denormalize) ----------

// BenchmarkStructuralCopy_L2Read_NoTransform:
// applyEntityFetchL2Results path with no aliases — parse the wire bytes onto
// l.jsonArena then plain StructuralCopy to produce an isolated materialized value.
func BenchmarkStructuralCopy_L2Read_NoTransform(b *testing.B) {
	wire := []byte(benchEntitySchemaShape)
	obj := benchNoAliasObject()

	l, ar := newBenchLoader()
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		parsed, err := l.parser.ParseBytesWithArena(l.jsonArena, wire)
		if err != nil {
			b.Fatal(err)
		}
		_ = l.structuralCopyDenormalized(parsed, obj)
		ar.Reset()
	}
}

// BenchmarkStructuralCopy_L2Read_WithTransform:
// applyEntityFetchL2Results path with alias denormalization — parse + Transform.
func BenchmarkStructuralCopy_L2Read_WithTransform(b *testing.B) {
	wire := []byte(benchEntitySchemaShape)
	obj := benchAliasedObject()

	l, ar := newBenchLoader()
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		parsed, err := l.parser.ParseBytesWithArena(l.jsonArena, wire)
		if err != nil {
			b.Fatal(err)
		}
		_ = l.structuralCopyDenormalized(parsed, obj)
		ar.Reset()
	}
}

// ---------- L2 Write (serialize) ----------

// BenchmarkStructuralCopy_L2Write_NoTransform:
// cacheKeysToEntriesBatch path — MarshalTo on the already-normalized L1 entry.
// This is the ONLY path prod currently takes: the transform cost was paid on L1 write.
func BenchmarkStructuralCopy_L2Write_NoTransform(b *testing.B) {
	ar := arena.NewMonotonicArena(arena.WithMinBufferSize(4096))
	v := parseOnto(ar, []byte(benchEntitySchemaShape))

	var buf []byte
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		buf = v.MarshalTo(buf[:0])
	}
	_ = buf
}

// BenchmarkStructuralCopy_L2Write_WithTransform:
// Hypothetical "normalize + serialize" cost — models writing a still-aliased
// response value to L2 without an intermediate L1 entry. Not a live prod path,
// but measures the combined Transform + MarshalTo cost for comparison.
func BenchmarkStructuralCopy_L2Write_WithTransform(b *testing.B) {
	sourceAr := arena.NewMonotonicArena(arena.WithMinBufferSize(4096))
	src := parseOnto(sourceAr, []byte(benchEntityResponseShape))
	obj := benchAliasedObject()

	l, ar := newBenchLoader()
	var buf []byte
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		normalized := l.structuralCopyNormalized(src, obj)
		buf = normalized.MarshalTo(buf[:0])
		ar.Reset()
	}
	_ = buf
}
