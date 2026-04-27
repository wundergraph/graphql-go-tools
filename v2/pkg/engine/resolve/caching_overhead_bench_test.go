package resolve

import (
	"bytes"
	"context"
	"net/http"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/wundergraph/go-arena"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/datasource/httpclient"
)

// benchDataSource returns a fixed response with no allocations beyond the copy.
type benchDataSource struct {
	data []byte
}

func (d *benchDataSource) Load(_ context.Context, _ http.Header, _ []byte) ([]byte, error) {
	out := make([]byte, len(d.data))
	copy(out, d.data)
	return out, nil
}

func (d *benchDataSource) LoadWithFiles(_ context.Context, _ http.Header, _ []byte, _ []*httpclient.FileUpload) ([]byte, error) {
	return d.Load(context.TODO(), nil, nil)
}

// benchCache is a zero-latency in-memory cache for benchmarking L2 overhead.
type benchCache struct {
	mu      sync.RWMutex
	storage map[string][]byte
}

func newBenchCache() *benchCache {
	return &benchCache{storage: make(map[string][]byte)}
}

func (c *benchCache) Get(_ context.Context, keys []string) ([]*CacheEntry, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	result := make([]*CacheEntry, len(keys))
	for i, key := range keys {
		if v, ok := c.storage[key]; ok {
			result[i] = &CacheEntry{Key: key, Value: v, RemainingTTL: 30 * time.Second}
		}
	}
	return result, nil
}

func (c *benchCache) Set(_ context.Context, entries []*CacheEntry) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, e := range entries {
		if e == nil {
			continue
		}
		c.storage[e.Key] = e.Value
	}
	return nil
}

func (c *benchCache) Delete(_ context.Context, keys []string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, key := range keys {
		delete(c.storage, key)
	}
	return nil
}

// buildBenchResponse constructs a GraphQLResponse representing a typical federated query:
//
//	query { topProducts { id name price } }
//
// Root fetch returns 10 products with __typename+id, then a batch entity fetch resolves name+price.
func buildBenchResponse(rootDS, entityDS DataSource, caching FetchCacheConfiguration) *GraphQLResponse {
	entityRepRenderer := NewGraphQLVariableResolveRenderer(&Object{
		Fields: []*Field{
			{Name: []byte("__typename"), Value: &String{Path: []string{"__typename"}}},
			{Name: []byte("id"), Value: &String{Path: []string{"id"}}},
		},
	})

	return &GraphQLResponse{
		Info: &GraphQLResponseInfo{OperationType: ast.OperationTypeQuery},
		Fetches: Sequence(
			SingleWithPath(&SingleFetch{
				FetchConfiguration: FetchConfiguration{
					DataSource: rootDS,
					PostProcessing: PostProcessingConfiguration{
						SelectResponseDataPath: []string{"data"},
					},
				},
				InputTemplate: InputTemplate{
					Segments: []TemplateSegment{
						{Data: []byte(`{"method":"POST","url":"http://products","body":{"query":"{topProducts{__typename id}}"}}`), SegmentType: StaticSegmentType},
					},
				},
				DataSourceIdentifier: []byte("graphql_datasource.Source"),
				Info: &FetchInfo{
					DataSourceID:   "products",
					DataSourceName: "products",
					OperationType:  ast.OperationTypeQuery,
					RootFields: []GraphCoordinate{
						{TypeName: "Query", FieldName: "topProducts"},
					},
				},
			}, "query"),
			SingleWithPath(&BatchEntityFetch{
				Input: BatchInput{
					Header: InputTemplate{
						Segments: []TemplateSegment{
							{Data: []byte(`{"method":"POST","url":"http://products","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Product{name price}}}","variables":{"representations":[`), SegmentType: StaticSegmentType},
						},
					},
					Items: []InputTemplate{
						{Segments: []TemplateSegment{
							{SegmentType: VariableSegmentType, VariableKind: ResolvableObjectVariableKind, Renderer: entityRepRenderer},
						}},
					},
					Separator: InputTemplate{
						Segments: []TemplateSegment{
							{Data: []byte(`,`), SegmentType: StaticSegmentType},
						},
					},
					Footer: InputTemplate{
						Segments: []TemplateSegment{
							{Data: []byte(`]}}}`), SegmentType: StaticSegmentType},
						},
					},
				},
				DataSource: entityDS,
				PostProcessing: PostProcessingConfiguration{
					SelectResponseDataPath: []string{"data", "_entities"},
				},
				DataSourceIdentifier: []byte("graphql_datasource.Source"),
				Info: &FetchInfo{
					DataSourceID:   "products",
					DataSourceName: "products",
					OperationType:  ast.OperationTypeQuery,
					RootFields: []GraphCoordinate{
						{TypeName: "Product", FieldName: "_entities"},
					},
					ProvidesData: &Object{
						Fields: []*Field{
							{Name: []byte("id"), Value: &Scalar{Path: []string{"id"}}},
							{Name: []byte("name"), Value: &Scalar{Path: []string{"name"}}},
							{Name: []byte("price"), Value: &Scalar{Path: []string{"price"}}},
						},
					},
				},
				Caching: caching,
			}, "query.topProducts", ArrayPath("topProducts")),
		),
		Data: &Object{
			Fields: []*Field{
				{
					Name: []byte("topProducts"),
					Value: &Array{
						Path: []string{"topProducts"},
						Item: &Object{
							Fields: []*Field{
								{Name: []byte("id"), Value: &String{Path: []string{"id"}}},
								{Name: []byte("name"), Value: &String{Path: []string{"name"}}},
								{Name: []byte("price"), Value: &Float{Path: []string{"price"}}},
							},
						},
					},
				},
			},
		},
	}
}

// buildParallelBenchResponse constructs a GraphQLResponse with parallel entity fetches
// to exercise the 4-phase parallel execution path.
//
//	query { topProducts { id name price } reviews { id body rating } }
//
// Root fetch returns products+reviews, then two parallel batch entity fetches resolve details.
func buildParallelBenchResponse(rootDS, productDS, reviewDS DataSource, productCaching, reviewCaching FetchCacheConfiguration) *GraphQLResponse {
	productRepRenderer := NewGraphQLVariableResolveRenderer(&Object{
		Fields: []*Field{
			{Name: []byte("__typename"), Value: &String{Path: []string{"__typename"}}},
			{Name: []byte("id"), Value: &String{Path: []string{"id"}}},
		},
	})
	reviewRepRenderer := NewGraphQLVariableResolveRenderer(&Object{
		Fields: []*Field{
			{Name: []byte("__typename"), Value: &String{Path: []string{"__typename"}}},
			{Name: []byte("id"), Value: &String{Path: []string{"id"}}},
		},
	})

	return &GraphQLResponse{
		Info: &GraphQLResponseInfo{OperationType: ast.OperationTypeQuery},
		Fetches: Sequence(
			SingleWithPath(&SingleFetch{
				FetchConfiguration: FetchConfiguration{
					DataSource: rootDS,
					PostProcessing: PostProcessingConfiguration{
						SelectResponseDataPath: []string{"data"},
					},
				},
				InputTemplate: InputTemplate{
					Segments: []TemplateSegment{
						{Data: []byte(`{"method":"POST","url":"http://root","body":{"query":"{topProducts{__typename id} reviews{__typename id}}"}}`), SegmentType: StaticSegmentType},
					},
				},
				DataSourceIdentifier: []byte("graphql_datasource.Source"),
				Info: &FetchInfo{
					DataSourceID:   "root",
					DataSourceName: "root",
					OperationType:  ast.OperationTypeQuery,
					RootFields: []GraphCoordinate{
						{TypeName: "Query", FieldName: "topProducts"},
						{TypeName: "Query", FieldName: "reviews"},
					},
				},
			}, "query"),
			Parallel(
				SingleWithPath(&BatchEntityFetch{
					Input: BatchInput{
						Header: InputTemplate{Segments: []TemplateSegment{
							{Data: []byte(`{"method":"POST","url":"http://products","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Product{name price}}}","variables":{"representations":[`), SegmentType: StaticSegmentType},
						}},
						Items: []InputTemplate{{Segments: []TemplateSegment{
							{SegmentType: VariableSegmentType, VariableKind: ResolvableObjectVariableKind, Renderer: productRepRenderer},
						}}},
						Separator: InputTemplate{Segments: []TemplateSegment{{Data: []byte(`,`), SegmentType: StaticSegmentType}}},
						Footer:    InputTemplate{Segments: []TemplateSegment{{Data: []byte(`]}}}`), SegmentType: StaticSegmentType}}},
					},
					DataSource: productDS,
					PostProcessing: PostProcessingConfiguration{
						SelectResponseDataPath: []string{"data", "_entities"},
					},
					DataSourceIdentifier: []byte("graphql_datasource.Source"),
					Info: &FetchInfo{
						DataSourceID:   "products",
						DataSourceName: "products",
						OperationType:  ast.OperationTypeQuery,
						RootFields:     []GraphCoordinate{{TypeName: "Product", FieldName: "_entities"}},
						ProvidesData: &Object{Fields: []*Field{
							{Name: []byte("id"), Value: &Scalar{Path: []string{"id"}}},
							{Name: []byte("name"), Value: &Scalar{Path: []string{"name"}}},
							{Name: []byte("price"), Value: &Scalar{Path: []string{"price"}}},
						}},
					},
					Caching: productCaching,
				}, "query.topProducts", ArrayPath("topProducts")),
				SingleWithPath(&BatchEntityFetch{
					Input: BatchInput{
						Header: InputTemplate{Segments: []TemplateSegment{
							{Data: []byte(`{"method":"POST","url":"http://reviews","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Review{body rating}}}","variables":{"representations":[`), SegmentType: StaticSegmentType},
						}},
						Items: []InputTemplate{{Segments: []TemplateSegment{
							{SegmentType: VariableSegmentType, VariableKind: ResolvableObjectVariableKind, Renderer: reviewRepRenderer},
						}}},
						Separator: InputTemplate{Segments: []TemplateSegment{{Data: []byte(`,`), SegmentType: StaticSegmentType}}},
						Footer:    InputTemplate{Segments: []TemplateSegment{{Data: []byte(`]}}}`), SegmentType: StaticSegmentType}}},
					},
					DataSource: reviewDS,
					PostProcessing: PostProcessingConfiguration{
						SelectResponseDataPath: []string{"data", "_entities"},
					},
					DataSourceIdentifier: []byte("graphql_datasource.Source"),
					Info: &FetchInfo{
						DataSourceID:   "reviews",
						DataSourceName: "reviews",
						OperationType:  ast.OperationTypeQuery,
						RootFields:     []GraphCoordinate{{TypeName: "Review", FieldName: "_entities"}},
						ProvidesData: &Object{Fields: []*Field{
							{Name: []byte("id"), Value: &Scalar{Path: []string{"id"}}},
							{Name: []byte("body"), Value: &Scalar{Path: []string{"body"}}},
							{Name: []byte("rating"), Value: &Scalar{Path: []string{"rating"}}},
						}},
					},
					Caching: reviewCaching,
				}, "query.reviews", ArrayPath("reviews")),
			),
		),
		Data: &Object{
			Fields: []*Field{
				{
					Name: []byte("topProducts"),
					Value: &Array{
						Path: []string{"topProducts"},
						Item: &Object{Fields: []*Field{
							{Name: []byte("id"), Value: &String{Path: []string{"id"}}},
							{Name: []byte("name"), Value: &String{Path: []string{"name"}}},
							{Name: []byte("price"), Value: &Float{Path: []string{"price"}}},
						}},
					},
				},
				{
					Name: []byte("reviews"),
					Value: &Array{
						Path: []string{"reviews"},
						Item: &Object{Fields: []*Field{
							{Name: []byte("id"), Value: &String{Path: []string{"id"}}},
							{Name: []byte("body"), Value: &String{Path: []string{"body"}}},
							{Name: []byte("rating"), Value: &Integer{Path: []string{"rating"}}},
						}},
					},
				},
			},
		},
	}
}

func entityCacheKeyTemplate() *EntityQueryCacheKeyTemplate {
	return &EntityQueryCacheKeyTemplate{
		Keys: NewResolvableObjectVariable(&Object{
			Fields: []*Field{
				{Name: []byte("__typename"), Value: &String{Path: []string{"__typename"}}},
				{Name: []byte("id"), Value: &String{Path: []string{"id"}}},
			},
		}),
	}
}

// --- Sequential benchmarks (root fetch → batch entity fetch) ---

// BenchmarkCachingOverhead_Sequential measures the full Loader.LoadGraphQLResponseData path
// for a sequential fetch tree (root → batch entity) under different caching configurations.
//
// Sub-benchmarks:
//   - Disabled: L1=off, L2=off, no CacheKeyTemplate — measures true zero-overhead baseline
//   - ConfiguredButDisabled: L1=off, L2=off, but CacheKeyTemplate IS set — detects any
//     work done even when caching flags are off
//   - L1Only: L1=on, L2=off — measures L1 overhead (sync.Map, key rendering)
//   - L1L2_Miss: L1=on, L2=on, empty cache — measures L2 miss overhead (Get call, key prefix)
//   - L1L2_Hit: L1=on, L2=on, pre-populated cache — measures L2 hit path (Get, parse, merge)
func BenchmarkCachingOverhead_Sequential(b *testing.B) {
	rootData := []byte(`{"data":{"topProducts":[` +
		`{"__typename":"Product","id":"p1"},` +
		`{"__typename":"Product","id":"p2"},` +
		`{"__typename":"Product","id":"p3"},` +
		`{"__typename":"Product","id":"p4"},` +
		`{"__typename":"Product","id":"p5"},` +
		`{"__typename":"Product","id":"p6"},` +
		`{"__typename":"Product","id":"p7"},` +
		`{"__typename":"Product","id":"p8"},` +
		`{"__typename":"Product","id":"p9"},` +
		`{"__typename":"Product","id":"p10"}` +
		`]}}`)

	entityData := []byte(`{"data":{"_entities":[` +
		`{"__typename":"Product","id":"p1","name":"Product 1","price":10.00},` +
		`{"__typename":"Product","id":"p2","name":"Product 2","price":20.00},` +
		`{"__typename":"Product","id":"p3","name":"Product 3","price":30.00},` +
		`{"__typename":"Product","id":"p4","name":"Product 4","price":40.00},` +
		`{"__typename":"Product","id":"p5","name":"Product 5","price":50.00},` +
		`{"__typename":"Product","id":"p6","name":"Product 6","price":60.00},` +
		`{"__typename":"Product","id":"p7","name":"Product 7","price":70.00},` +
		`{"__typename":"Product","id":"p8","name":"Product 8","price":80.00},` +
		`{"__typename":"Product","id":"p9","name":"Product 9","price":90.00},` +
		`{"__typename":"Product","id":"p10","name":"Product 10","price":100.00}` +
		`]}}`)

	rootDS := &benchDataSource{data: rootData}
	entityDS := &benchDataSource{data: entityData}

	b.Run("Disabled", func(b *testing.B) {
		// No CacheKeyTemplate, L1=off, L2=off — true baseline
		response := buildBenchResponse(rootDS, entityDS, FetchCacheConfiguration{})
		benchResolveSequential(b, response, false, false, nil)
	})

	b.Run("ConfiguredButDisabled", func(b *testing.B) {
		// CacheKeyTemplate IS set but L1=off, L2=off — detects leaky guard checks
		caching := FetchCacheConfiguration{
			Enabled:          true,
			CacheName:        "default",
			TTL:              30 * time.Second,
			CacheKeyTemplate: entityCacheKeyTemplate(),
			UseL1Cache:       true,
		}
		response := buildBenchResponse(rootDS, entityDS, caching)
		benchResolveSequential(b, response, false, false, nil)
	})

	b.Run("L1Only", func(b *testing.B) {
		caching := FetchCacheConfiguration{
			Enabled:          true,
			CacheName:        "default",
			TTL:              30 * time.Second,
			CacheKeyTemplate: entityCacheKeyTemplate(),
			UseL1Cache:       true,
		}
		response := buildBenchResponse(rootDS, entityDS, caching)
		benchResolveSequential(b, response, true, false, nil)
	})

	b.Run("L1L2_Miss", func(b *testing.B) {
		cache := newBenchCache()
		caching := FetchCacheConfiguration{
			Enabled:          true,
			CacheName:        "default",
			TTL:              30 * time.Second,
			CacheKeyTemplate: entityCacheKeyTemplate(),
			UseL1Cache:       true,
		}
		response := buildBenchResponse(rootDS, entityDS, caching)
		benchResolveSequential(b, response, true, true, cache)
	})

	b.Run("L1L2_Hit", func(b *testing.B) {
		cache := newBenchCache()
		// Pre-populate cache with all 10 entities
		for i := range 10 {
			id := "p" + itoa(i+1)
			key := `{"__typename":"Product","key":{"id":"` + id + `"}}`
			val := []byte(`{"__typename":"Product","id":"` + id + `","name":"Product ` + itoa(i+1) + `","price":` + itoa((i+1)*10) + `}`)
			cache.storage[key] = val
		}
		caching := FetchCacheConfiguration{
			Enabled:          true,
			CacheName:        "default",
			TTL:              30 * time.Second,
			CacheKeyTemplate: entityCacheKeyTemplate(),
			UseL1Cache:       true,
		}
		response := buildBenchResponse(rootDS, entityDS, caching)
		benchResolveSequential(b, response, true, true, cache)
	})
}

func benchResolveSequential(b *testing.B, response *GraphQLResponse, enableL1, enableL2 bool, cache LoaderCache) {
	b.Helper()

	caches := map[string]LoaderCache{}
	if cache != nil {
		caches["default"] = cache
	}

	var buf bytes.Buffer
	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		ar := arena.NewMonotonicArena(arena.WithMinBufferSize(4096))
		resolvable := NewResolvable(ar, ResolvableOptions{})
		loader := &Loader{
			caches:    caches,
			jsonArena: ar,
		}

		ctx := NewContext(context.Background())
		ctx.ExecutionOptions.DisableSubgraphRequestDeduplication = true
		ctx.ExecutionOptions.Caching.EnableL1Cache = enableL1
		ctx.ExecutionOptions.Caching.EnableL2Cache = enableL2

		_ = resolvable.Init(ctx, nil, ast.OperationTypeQuery)
		_ = loader.LoadGraphQLResponseData(ctx, response, resolvable)

		buf.Reset()
		_ = resolvable.Resolve(ctx.ctx, response.Data, response.Fetches, &buf)

		loader.Free()
		ar.Reset()
	}
}

// --- Parallel benchmarks (root → 2 parallel entity fetches) ---

// BenchmarkCachingOverhead_Parallel measures the 4-phase parallel execution path under
// different caching configurations.
//
// The parallel path exercises Phase 1 (main thread L1 check), Phase 2 (goroutine L2+fetch),
// Phase 3 (analytics merge), and Phase 4 (result merge + cache population).
func BenchmarkCachingOverhead_Parallel(b *testing.B) {
	rootData := []byte(`{"data":{"topProducts":[` +
		`{"__typename":"Product","id":"p1"},` +
		`{"__typename":"Product","id":"p2"},` +
		`{"__typename":"Product","id":"p3"},` +
		`{"__typename":"Product","id":"p4"},` +
		`{"__typename":"Product","id":"p5"}` +
		`],"reviews":[` +
		`{"__typename":"Review","id":"r1"},` +
		`{"__typename":"Review","id":"r2"},` +
		`{"__typename":"Review","id":"r3"},` +
		`{"__typename":"Review","id":"r4"},` +
		`{"__typename":"Review","id":"r5"}` +
		`]}}`)

	productData := []byte(`{"data":{"_entities":[` +
		`{"__typename":"Product","id":"p1","name":"Product 1","price":10.00},` +
		`{"__typename":"Product","id":"p2","name":"Product 2","price":20.00},` +
		`{"__typename":"Product","id":"p3","name":"Product 3","price":30.00},` +
		`{"__typename":"Product","id":"p4","name":"Product 4","price":40.00},` +
		`{"__typename":"Product","id":"p5","name":"Product 5","price":50.00}` +
		`]}}`)

	reviewData := []byte(`{"data":{"_entities":[` +
		`{"__typename":"Review","id":"r1","body":"Great","rating":5},` +
		`{"__typename":"Review","id":"r2","body":"Good","rating":4},` +
		`{"__typename":"Review","id":"r3","body":"Okay","rating":3},` +
		`{"__typename":"Review","id":"r4","body":"Meh","rating":2},` +
		`{"__typename":"Review","id":"r5","body":"Bad","rating":1}` +
		`]}}`)

	rootDS := &benchDataSource{data: rootData}
	productDS := &benchDataSource{data: productData}
	reviewDS := &benchDataSource{data: reviewData}

	noCaching := FetchCacheConfiguration{}

	b.Run("Disabled", func(b *testing.B) {
		response := buildParallelBenchResponse(rootDS, productDS, reviewDS, noCaching, noCaching)
		benchResolveParallel(b, response, false, false, nil)
	})

	b.Run("L1Only", func(b *testing.B) {
		caching := FetchCacheConfiguration{
			Enabled:          true,
			CacheName:        "default",
			TTL:              30 * time.Second,
			CacheKeyTemplate: entityCacheKeyTemplate(),
			UseL1Cache:       true,
		}
		response := buildParallelBenchResponse(rootDS, productDS, reviewDS, caching, caching)
		benchResolveParallel(b, response, true, false, nil)
	})

	b.Run("L1L2_Miss", func(b *testing.B) {
		cache := newBenchCache()
		caching := FetchCacheConfiguration{
			Enabled:          true,
			CacheName:        "default",
			TTL:              30 * time.Second,
			CacheKeyTemplate: entityCacheKeyTemplate(),
			UseL1Cache:       true,
		}
		response := buildParallelBenchResponse(rootDS, productDS, reviewDS, caching, caching)
		benchResolveParallel(b, response, true, true, cache)
	})

	b.Run("L1L2_Hit", func(b *testing.B) {
		cache := newBenchCache()
		for i := range 5 {
			pid := "p" + itoa(i+1)
			pKey := `{"__typename":"Product","key":{"id":"` + pid + `"}}`
			pVal := []byte(`{"__typename":"Product","id":"` + pid + `","name":"Product ` + itoa(i+1) + `","price":` + itoa((i+1)*10) + `}`)
			cache.storage[pKey] = pVal

			rid := "r" + itoa(i+1)
			rKey := `{"__typename":"Review","key":{"id":"` + rid + `"}}`
			rVal := []byte(`{"__typename":"Review","id":"` + rid + `","body":"Review ` + itoa(i+1) + `","rating":` + itoa(i+1) + `}`)
			cache.storage[rKey] = rVal
		}
		caching := FetchCacheConfiguration{
			Enabled:          true,
			CacheName:        "default",
			TTL:              30 * time.Second,
			CacheKeyTemplate: entityCacheKeyTemplate(),
			UseL1Cache:       true,
		}
		response := buildParallelBenchResponse(rootDS, productDS, reviewDS, caching, caching)
		benchResolveParallel(b, response, true, true, cache)
	})
}

func benchResolveParallel(b *testing.B, response *GraphQLResponse, enableL1, enableL2 bool, cache LoaderCache) {
	b.Helper()

	caches := map[string]LoaderCache{}
	if cache != nil {
		caches["default"] = cache
	}

	var buf bytes.Buffer
	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		ar := arena.NewMonotonicArena(arena.WithMinBufferSize(4096))
		resolvable := NewResolvable(ar, ResolvableOptions{})
		loader := &Loader{
			caches:    caches,
			jsonArena: ar,
		}

		ctx := NewContext(context.Background())
		ctx.ExecutionOptions.DisableSubgraphRequestDeduplication = true
		ctx.ExecutionOptions.Caching.EnableL1Cache = enableL1
		ctx.ExecutionOptions.Caching.EnableL2Cache = enableL2

		_ = resolvable.Init(ctx, nil, ast.OperationTypeQuery)
		_ = loader.LoadGraphQLResponseData(ctx, response, resolvable)

		buf.Reset()
		_ = resolvable.Resolve(ctx.ctx, response.Data, response.Fetches, &buf)

		loader.Free()
		ar.Reset()
	}
}

// --- Analytics overhead benchmark ---

// BenchmarkCachingOverhead_Analytics measures the additional overhead of EnableCacheAnalytics
// on top of L1+L2 caching. Analytics collects per-entity events, field hashes, and timing data.
func BenchmarkCachingOverhead_Analytics(b *testing.B) {
	rootData := []byte(`{"data":{"topProducts":[` +
		`{"__typename":"Product","id":"p1"},` +
		`{"__typename":"Product","id":"p2"},` +
		`{"__typename":"Product","id":"p3"},` +
		`{"__typename":"Product","id":"p4"},` +
		`{"__typename":"Product","id":"p5"}` +
		`]}}`)

	entityData := []byte(`{"data":{"_entities":[` +
		`{"__typename":"Product","id":"p1","name":"Product 1","price":10.00},` +
		`{"__typename":"Product","id":"p2","name":"Product 2","price":20.00},` +
		`{"__typename":"Product","id":"p3","name":"Product 3","price":30.00},` +
		`{"__typename":"Product","id":"p4","name":"Product 4","price":40.00},` +
		`{"__typename":"Product","id":"p5","name":"Product 5","price":50.00}` +
		`]}}`)

	rootDS := &benchDataSource{data: rootData}
	entityDS := &benchDataSource{data: entityData}

	cache := newBenchCache()
	caching := FetchCacheConfiguration{
		Enabled:          true,
		CacheName:        "default",
		TTL:              30 * time.Second,
		CacheKeyTemplate: entityCacheKeyTemplate(),
		UseL1Cache:       true,
	}
	response := buildBenchResponse(rootDS, entityDS, caching)

	caches := map[string]LoaderCache{"default": cache}

	b.Run("AnalyticsOff", func(b *testing.B) {
		var buf bytes.Buffer
		b.ReportAllocs()
		b.ResetTimer()
		for b.Loop() {
			ar := arena.NewMonotonicArena(arena.WithMinBufferSize(4096))
			resolvable := NewResolvable(ar, ResolvableOptions{})
			loader := &Loader{caches: caches, jsonArena: ar}

			ctx := NewContext(context.Background())
			ctx.ExecutionOptions.DisableSubgraphRequestDeduplication = true
			ctx.ExecutionOptions.Caching.EnableL1Cache = true
			ctx.ExecutionOptions.Caching.EnableL2Cache = true
			ctx.ExecutionOptions.Caching.EnableCacheAnalytics = false

			_ = resolvable.Init(ctx, nil, ast.OperationTypeQuery)
			_ = loader.LoadGraphQLResponseData(ctx, response, resolvable)

			buf.Reset()
			_ = resolvable.Resolve(ctx.ctx, response.Data, response.Fetches, &buf)

			loader.Free()
			ar.Reset()
		}
	})

	b.Run("AnalyticsOn", func(b *testing.B) {
		var buf bytes.Buffer
		b.ReportAllocs()
		b.ResetTimer()
		for b.Loop() {
			ar := arena.NewMonotonicArena(arena.WithMinBufferSize(4096))
			resolvable := NewResolvable(ar, ResolvableOptions{})
			loader := &Loader{caches: caches, jsonArena: ar}

			ctx := NewContext(context.Background())
			ctx.ExecutionOptions.DisableSubgraphRequestDeduplication = true
			ctx.ExecutionOptions.Caching.EnableL1Cache = true
			ctx.ExecutionOptions.Caching.EnableL2Cache = true
			ctx.ExecutionOptions.Caching.EnableCacheAnalytics = true

			_ = resolvable.Init(ctx, nil, ast.OperationTypeQuery)
			_ = loader.LoadGraphQLResponseData(ctx, response, resolvable)

			buf.Reset()
			_ = resolvable.Resolve(ctx.ctx, response.Data, response.Fetches, &buf)

			loader.Free()
			ar.Reset()
		}
	})
}

func itoa(n int) string {
	return strconv.Itoa(n)
}
