package resolve

import (
	"context"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wundergraph/astjson"
	"github.com/wundergraph/go-arena"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/fastjsonext"
)

// Helpers to build batch entity cache test fixtures.
// These mirror the integration test scenario: products(upcs: ["top-1","top-2","top-3"])
// with EntityKeyMappings using ArgumentIsEntityKey=true.

func newBatchProductsCacheKeyTemplate() *RootQueryCacheKeyTemplate {
	return NewRootQueryCacheKeyTemplate(
		[]QueryField{
			{
				Coordinate: GraphCoordinate{TypeName: "Query", FieldName: "products"},
				Args: []FieldArgument{
					{
						Name: "upcs",
						Variable: &ContextVariable{
							Path:     []string{"upcs"},
							Renderer: NewCacheKeyVariableRenderer(),
						},
					},
				},
			},
		},
		[]EntityKeyMappingConfig{
			{
				EntityTypeName: "Product",
				FieldMappings: []EntityFieldMappingConfig{
					{EntityKeyField: "upc", ArgumentPath: []string{"upcs"}, ArgumentIsEntityKey: true},
				},
			},
		},
	)
}

func newBatchProductsProvidesData() *Object {
	return &Object{
		Fields: []*Field{
			{Name: []byte("upc"), Value: &Scalar{Path: []string{"upc"}, Nullable: false}},
			{Name: []byte("name"), Value: &Scalar{Path: []string{"name"}, Nullable: false}},
			{Name: []byte("price"), Value: &Scalar{Path: []string{"price"}, Nullable: false}},
		},
	}
}

func newBatchProductsResponse(rootDS DataSource, cacheKeyTemplate CacheKeyTemplate, providesData *Object) *GraphQLResponse {
	var rootProvidesData *Object
	if providesData != nil {
		rootProvidesData = &Object{
			Fields: []*Field{
				{
					Name: []byte("products"),
					Value: &Array{
						Item: &Object{
							Fields: providesData.Fields,
						},
					},
				},
			},
		}
	}

	return &GraphQLResponse{
		Info: &GraphQLResponseInfo{OperationType: ast.OperationTypeQuery},
		Fetches: Sequence(
			SingleWithPath(&SingleFetch{
				FetchConfiguration: FetchConfiguration{
					DataSource: rootDS,
					PostProcessing: PostProcessingConfiguration{
						SelectResponseDataPath: []string{"data"},
						// No MergePath for root field fetches - data is merged at root level
					},
					Caching: FetchCacheConfiguration{
						Enabled:          true,
						CacheName:        "default",
						TTL:              30 * time.Second,
						CacheKeyTemplate: cacheKeyTemplate,
					},
				},
				InputTemplate: InputTemplate{
					Segments: []TemplateSegment{
						{Data: []byte(`{"method":"POST","url":"http://products","body":{"query":"{ products(upcs: $upcs) { upc name price } }"}}`), SegmentType: StaticSegmentType},
					},
				},
				Info: &FetchInfo{
					DataSourceID:   "products",
					DataSourceName: "products",
					RootFields:     []GraphCoordinate{{TypeName: "Query", FieldName: "products"}},
					OperationType:  ast.OperationTypeQuery,
					ProvidesData:   rootProvidesData,
				},
				DataSourceIdentifier: []byte("graphql_datasource.Source"),
			}, "query"),
		),
		Data: &Object{
			Fields: []*Field{
				{
					Name: []byte("products"),
					Value: &Array{
						Path: []string{"products"},
						Item: &Object{
							Fields: []*Field{
								{Name: []byte("upc"), Value: &String{Path: []string{"upc"}}},
								{Name: []byte("name"), Value: &String{Path: []string{"name"}}},
								{Name: []byte("price"), Value: &Scalar{Path: []string{"price"}}},
							},
						},
					},
				},
			},
		},
	}
}

// TestBatchEntityCache_AllMissThenAllHit mirrors the integration test
// TestBatchEntityCacheLookup_FullFetch_AllMiss + TestBatchEntityCacheLookup_FullFetch_AllHit.
// Verifies the complete batch entity cache lifecycle at the resolve layer:
// 1. First request: all L2 misses → subgraph fetch → entities written to L2 individually
// 2. Second request: all L2 hits → no subgraph call → entities served from cache
func TestBatchEntityCache_AllMissThenAllHit(t *testing.T) {
	ctrl := gomock.NewController(t)

	cache := NewFakeLoaderCache()

	// First request: subgraph returns 3 products
	rootDS := NewMockDataSource(ctrl)
	rootDS.EXPECT().Load(gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, headers any, input []byte) ([]byte, error) {
			return []byte(`{"data":{"products":[{"upc":"top-1","name":"Trilby","price":11},{"upc":"top-2","name":"Fedora","price":22},{"upc":"top-3","name":"Boater","price":33}]}}`), nil
		}).Times(1) // Only called once across both requests

	response := newBatchProductsResponse(
		rootDS,
		newBatchProductsCacheKeyTemplate(),
		newBatchProductsProvidesData(),
	)

	loader := &Loader{caches: map[string]LoaderCache{"default": cache}}
	ctx := NewContext(context.Background())
	ctx.Variables = astjson.MustParseBytes([]byte(`{"upcs":["top-1","top-2","top-3"]}`))
	ctx.ExecutionOptions.DisableSubgraphRequestDeduplication = true
	ctx.ExecutionOptions.Caching.EnableL1Cache = false
	ctx.ExecutionOptions.Caching.EnableL2Cache = true

	ar := arena.NewMonotonicArena(arena.WithMinBufferSize(4096))
	resolvable := NewResolvable(ar, ResolvableOptions{})

	// Request 1: cold cache → fetch from subgraph, write entities to L2
	err := resolvable.Init(ctx, nil, ast.OperationTypeQuery)
	require.NoError(t, err)
	err = loader.LoadGraphQLResponseData(ctx, response, resolvable)
	require.NoError(t, err)

	out1 := fastjsonext.PrintGraphQLResponse(resolvable.data, resolvable.errors)
	assert.Equal(t, `{"data":{"products":[{"upc":"top-1","name":"Trilby","price":11},{"upc":"top-2","name":"Fedora","price":22},{"upc":"top-3","name":"Boater","price":33}]}}`, out1)

	// Cache log: 1 batch get (3 misses) + 1 batch set (3 entries)
	log := cache.GetLog()
	require.Equal(t, 2, len(log))
	assert.Equal(t, "get", log[0].Operation)
	assert.Equal(t, []CacheLogItem{
		{Key: `{"__typename":"Product","key":{"upc":"top-1"}}`, Hit: false},
		{Key: `{"__typename":"Product","key":{"upc":"top-2"}}`, Hit: false},
		{Key: `{"__typename":"Product","key":{"upc":"top-3"}}`, Hit: false},
	}, log[0].Items)
	assert.Equal(t, "set", log[1].Operation)
	assert.Equal(t, []CacheLogItem{
		{Key: `{"__typename":"Product","key":{"upc":"top-1"}}`, TTL: 30 * time.Second},
		{Key: `{"__typename":"Product","key":{"upc":"top-2"}}`, TTL: 30 * time.Second},
		{Key: `{"__typename":"Product","key":{"upc":"top-3"}}`, TTL: 30 * time.Second},
	}, log[1].Items)
	cache.ClearLog()

	// Verify each entity was stored individually
	assert.Equal(t, `{"upc":"top-1","name":"Trilby","price":11}`, string(cache.GetValue(`{"__typename":"Product","key":{"upc":"top-1"}}`)))
	assert.Equal(t, `{"upc":"top-2","name":"Fedora","price":22}`, string(cache.GetValue(`{"__typename":"Product","key":{"upc":"top-2"}}`)))
	assert.Equal(t, `{"upc":"top-3","name":"Boater","price":33}`, string(cache.GetValue(`{"__typename":"Product","key":{"upc":"top-3"}}`)))

	// Request 2: warm cache → all hits, no subgraph call
	ar2 := arena.NewMonotonicArena(arena.WithMinBufferSize(4096))
	resolvable2 := NewResolvable(ar2, ResolvableOptions{})
	ctx2 := NewContext(context.Background())
	ctx2.Variables = astjson.MustParseBytes([]byte(`{"upcs":["top-1","top-2","top-3"]}`))
	ctx2.ExecutionOptions.DisableSubgraphRequestDeduplication = true
	ctx2.ExecutionOptions.Caching.EnableL1Cache = false
	ctx2.ExecutionOptions.Caching.EnableL2Cache = true

	err = resolvable2.Init(ctx2, nil, ast.OperationTypeQuery)
	require.NoError(t, err)
	loader2 := &Loader{caches: map[string]LoaderCache{"default": cache}}
	err = loader2.LoadGraphQLResponseData(ctx2, response, resolvable2)
	require.NoError(t, err)

	out2 := fastjsonext.PrintGraphQLResponse(resolvable2.data, resolvable2.errors)
	assert.Equal(t, out1, out2)

	// Cache log: 1 batch get (3 hits), no set
	log2 := cache.GetLog()
	require.Equal(t, 1, len(log2))
	assert.Equal(t, "get", log2[0].Operation)
	assert.Equal(t, []CacheLogItem{
		{Key: `{"__typename":"Product","key":{"upc":"top-1"}}`, Hit: true},
		{Key: `{"__typename":"Product","key":{"upc":"top-2"}}`, Hit: true},
		{Key: `{"__typename":"Product","key":{"upc":"top-3"}}`, Hit: true},
	}, log2[0].Items)
}

// TestBatchEntityCache_PartialHitFetchesMissing mirrors
// TestBatchEntityCacheLookup_PartialFetch_SomeCached.
// Verifies that when partial batch loading is enabled, only missing entities
// are fetched from the subgraph while cached entities are served from L2.
func TestBatchEntityCache_PartialHitFetchesMissing(t *testing.T) {
	ctrl := gomock.NewController(t)

	cache := NewFakeLoaderCache()

	// Seed cache with 2 of 3 products
	err := cache.Set(context.Background(), withCacheEntryTTL([]*CacheEntry{
		{Key: `{"__typename":"Product","key":{"upc":"top-1"}}`, Value: []byte(`{"upc":"top-1","name":"Trilby","price":11}`)},
		{Key: `{"__typename":"Product","key":{"upc":"top-2"}}`, Value: []byte(`{"upc":"top-2","name":"Fedora","price":22}`)},
	}, 30*time.Second))
	require.NoError(t, err)
	cache.ClearLog()

	// Subgraph should only be called for the missing product (top-3)
	rootDS := NewMockDataSource(ctrl)
	rootDS.EXPECT().Load(gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, headers any, input []byte) ([]byte, error) {
			return []byte(`{"data":{"products":[{"upc":"top-3","name":"Boater","price":33}]}}`), nil
		}).Times(1)

	tmpl := newBatchProductsCacheKeyTemplate()
	provides := newBatchProductsProvidesData()

	var rootProvidesData *Object
	if provides != nil {
		rootProvidesData = &Object{
			Fields: []*Field{
				{
					Name: []byte("products"),
					Value: &Array{
						Item: &Object{
							Fields: provides.Fields,
						},
					},
				},
			},
		}
	}

	response := &GraphQLResponse{
		Info: &GraphQLResponseInfo{OperationType: ast.OperationTypeQuery},
		Fetches: Sequence(
			SingleWithPath(&SingleFetch{
				FetchConfiguration: FetchConfiguration{
					DataSource: rootDS,
					PostProcessing: PostProcessingConfiguration{
						SelectResponseDataPath: []string{"data"},
						// No MergePath for root field fetches - data is merged at root level
					},
					Caching: FetchCacheConfiguration{
						Enabled:                true,
						CacheName:              "default",
						TTL:                    30 * time.Second,
						CacheKeyTemplate:       tmpl,
						EnablePartialCacheLoad: true,
						PartialBatchLoad:       true,
					},
				},
				InputTemplate: InputTemplate{
					Segments: []TemplateSegment{
						{Data: []byte(`{"method":"POST","url":"http://products","body":{"query":"{ products(upcs: $upcs) { upc name price } }"}}`), SegmentType: StaticSegmentType},
					},
				},
				Info: &FetchInfo{
					DataSourceID:   "products",
					DataSourceName: "products",
					RootFields:     []GraphCoordinate{{TypeName: "Query", FieldName: "products"}},
					OperationType:  ast.OperationTypeQuery,
					ProvidesData:   rootProvidesData,
				},
				DataSourceIdentifier: []byte("graphql_datasource.Source"),
			}, "query"),
		),
		Data: &Object{
			Fields: []*Field{
				{
					Name: []byte("products"),
					Value: &Array{
						Path: []string{"products"},
						Item: &Object{
							Fields: []*Field{
								{Name: []byte("upc"), Value: &String{Path: []string{"upc"}}},
								{Name: []byte("name"), Value: &String{Path: []string{"name"}}},
								{Name: []byte("price"), Value: &Scalar{Path: []string{"price"}}},
							},
						},
					},
				},
			},
		},
	}

	loader := &Loader{caches: map[string]LoaderCache{"default": cache}}
	ctx := NewContext(context.Background())
	ctx.Variables = astjson.MustParseBytes([]byte(`{"upcs":["top-1","top-2","top-3"]}`))
	ctx.ExecutionOptions.DisableSubgraphRequestDeduplication = true
	ctx.ExecutionOptions.Caching.EnableL1Cache = false
	ctx.ExecutionOptions.Caching.EnableL2Cache = true

	ar := arena.NewMonotonicArena(arena.WithMinBufferSize(4096))
	resolvable := NewResolvable(ar, ResolvableOptions{})
	err = resolvable.Init(ctx, nil, ast.OperationTypeQuery)
	require.NoError(t, err)

	err = loader.LoadGraphQLResponseData(ctx, response, resolvable)
	require.NoError(t, err)

	out := fastjsonext.PrintGraphQLResponse(resolvable.data, resolvable.errors)
	assert.Equal(t, `{"data":{"products":[{"upc":"top-1","name":"Trilby","price":11},{"upc":"top-2","name":"Fedora","price":22},{"upc":"top-3","name":"Boater","price":33}]}}`, out)

	// Cache log: 1 get (2 hits, 1 miss) + 1 set (missing entity written)
	log := cache.GetLog()
	require.Equal(t, 2, len(log))
	assert.Equal(t, "get", log[0].Operation)
	assert.Equal(t, []CacheLogItem{
		{Key: `{"__typename":"Product","key":{"upc":"top-1"}}`, Hit: true},
		{Key: `{"__typename":"Product","key":{"upc":"top-2"}}`, Hit: true},
		{Key: `{"__typename":"Product","key":{"upc":"top-3"}}`, Hit: false},
	}, log[0].Items)
	assert.Equal(t, "set", log[1].Operation)
	assert.Equal(t, []CacheLogItem{
		{Key: `{"__typename":"Product","key":{"upc":"top-3"}}`, TTL: 30 * time.Second},
	}, log[1].Items)
}

// TestMultiCandidateCacheValue_MergeCandidatesForWiderProjection exercises
// resolveMultiCandidateCacheValue's merge logic directly.
// Scenario: two EntityKeyMappings produce two cache entries for the same entity.
// Candidate A has {id, name}, candidate B has {id, email}. The request needs
// {id, name, email}. Neither candidate alone validates, but merging them does.
func TestMultiCandidateCacheValue_MergeCandidatesForWiderProjection(t *testing.T) {
	cache := NewFakeLoaderCache()

	// Seed cache with two entries for same user via different key mappings
	idKey := `{"__typename":"User","key":{"id":"u1"}}`
	emailKey := `{"__typename":"User","key":{"email":"a@example.com"}}`
	err := cache.Set(context.Background(), withCacheEntryTTL([]*CacheEntry{
		{Key: idKey, Value: []byte(`{"id":"u1","name":"Alice"}`), RemainingTTL: 20 * time.Second},
		{Key: emailKey, Value: []byte(`{"id":"u1","email":"a@example.com"}`), RemainingTTL: 10 * time.Second},
	}, 30*time.Second))
	require.NoError(t, err)
	cache.ClearLog()

	ctrl := gomock.NewController(t)
	// Subgraph should NOT be called — merged candidates satisfy the request
	rootDS := NewMockDataSource(ctrl)
	rootDS.EXPECT().Load(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)

	// ProvidesData requires all three fields: id, name, email
	providesData := &Object{
		Fields: []*Field{
			{Name: []byte("id"), Value: &Scalar{Path: []string{"id"}, Nullable: false}},
			{Name: []byte("name"), Value: &Scalar{Path: []string{"name"}, Nullable: false}},
			{Name: []byte("email"), Value: &Scalar{Path: []string{"email"}, Nullable: false}},
		},
	}

	response := newUserRootQueryResponse(
		rootDS,
		newUserRootQueryTemplate([]string{"id", "email"}, []string{"id", "email"}),
		providesData,
	)

	loader := &Loader{caches: map[string]LoaderCache{"default": cache}}
	ctx := NewContext(context.Background())
	ctx.Variables = astjson.MustParseBytes([]byte(`{"id":"u1","email":"a@example.com"}`))
	ctx.ExecutionOptions.DisableSubgraphRequestDeduplication = true
	ctx.ExecutionOptions.Caching.EnableL1Cache = true
	ctx.ExecutionOptions.Caching.EnableL2Cache = true

	ar := arena.NewMonotonicArena(arena.WithMinBufferSize(4096))
	resolvable := NewResolvable(ar, ResolvableOptions{})
	err = resolvable.Init(ctx, nil, ast.OperationTypeQuery)
	require.NoError(t, err)

	err = loader.LoadGraphQLResponseData(ctx, response, resolvable)
	require.NoError(t, err)

	out := fastjsonext.PrintGraphQLResponse(resolvable.data, resolvable.errors)
	// Merged result should contain all three fields
	assert.Equal(t, `{"data":{"user":{"id":"u1","name":"Alice","email":"a@example.com"}}}`, out)

	// Cache log: 1 get (both keys hit) + 1 set (writeback of merged value)
	log := cache.GetLog()
	require.GreaterOrEqual(t, len(log), 1)
	assert.Equal(t, "get", log[0].Operation)
	assert.Equal(t, []CacheLogItem{
		{Key: `{"__typename":"User","key":{"id":"u1"}}`, Hit: true},
		{Key: `{"__typename":"User","key":{"email":"a@example.com"}}`, Hit: true},
	}, log[0].Items)
}

// TestBatchEntityCache_NegativeCacheHit exercises the negative cache path in
// applyRootFetchL2Results (loader_cache.go ~line 1170-1194).
// When the L2 cache holds a null sentinel for an entity and NegativeCacheTTL > 0,
// the entity is served as null from the negative cache without calling the subgraph.
func TestBatchEntityCache_NegativeCacheHit(t *testing.T) {
	ctrl := gomock.NewController(t)

	cache := NewFakeLoaderCache()

	// Seed cache: top-1 → real data, top-2 → null sentinel, top-3 → real data
	err := cache.Set(context.Background(), withCacheEntryTTL([]*CacheEntry{
		{Key: `{"__typename":"Product","key":{"upc":"top-1"}}`, Value: []byte(`{"upc":"top-1","name":"Trilby","price":11}`)},
		{Key: `{"__typename":"Product","key":{"upc":"top-2"}}`, Value: []byte(`null`)},
		{Key: `{"__typename":"Product","key":{"upc":"top-3"}}`, Value: []byte(`{"upc":"top-3","name":"Boater","price":33}`)},
	}, 30*time.Second))
	require.NoError(t, err)
	cache.ClearLog()

	// Subgraph should NOT be called — all entities are cache hits (including negative)
	rootDS := NewMockDataSource(ctrl)
	rootDS.EXPECT().Load(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)

	tmpl := newBatchProductsCacheKeyTemplate()

	response := &GraphQLResponse{
		Info: &GraphQLResponseInfo{OperationType: ast.OperationTypeQuery},
		Fetches: Sequence(
			SingleWithPath(&SingleFetch{
				FetchConfiguration: FetchConfiguration{
					DataSource: rootDS,
					PostProcessing: PostProcessingConfiguration{
						SelectResponseDataPath: []string{"data"},
					},
					Caching: FetchCacheConfiguration{
						Enabled:          true,
						CacheName:        "default",
						TTL:              30 * time.Second,
						NegativeCacheTTL: 10 * time.Second,
						CacheKeyTemplate: tmpl,
					},
				},
				InputTemplate: InputTemplate{
					Segments: []TemplateSegment{
						{Data: []byte(`{"method":"POST","url":"http://products","body":{"query":"{ products(upcs: $upcs) { upc name price } }"}}`), SegmentType: StaticSegmentType},
					},
				},
				Info: &FetchInfo{
					DataSourceID:   "products",
					DataSourceName: "products",
					RootFields:     []GraphCoordinate{{TypeName: "Query", FieldName: "products"}},
					OperationType:  ast.OperationTypeQuery,
				},
				DataSourceIdentifier: []byte("graphql_datasource.Source"),
			}, "query"),
		),
		Data: &Object{
			Fields: []*Field{
				{
					Name: []byte("products"),
					Value: &Array{
						Path: []string{"products"},
						Item: &Object{
							Fields: []*Field{
								{Name: []byte("upc"), Value: &String{Path: []string{"upc"}}},
								{Name: []byte("name"), Value: &String{Path: []string{"name"}}},
								{Name: []byte("price"), Value: &Scalar{Path: []string{"price"}}},
							},
						},
					},
				},
			},
		},
	}

	loader := &Loader{caches: map[string]LoaderCache{"default": cache}}
	ctx := NewContext(context.Background())
	ctx.Variables = astjson.MustParseBytes([]byte(`{"upcs":["top-1","top-2","top-3"]}`))
	ctx.ExecutionOptions.DisableSubgraphRequestDeduplication = true
	ctx.ExecutionOptions.Caching.EnableL1Cache = false
	ctx.ExecutionOptions.Caching.EnableL2Cache = true

	ar := arena.NewMonotonicArena(arena.WithMinBufferSize(4096))
	resolvable := NewResolvable(ar, ResolvableOptions{})
	err = resolvable.Init(ctx, nil, ast.OperationTypeQuery)
	require.NoError(t, err)

	err = loader.LoadGraphQLResponseData(ctx, response, resolvable)
	require.NoError(t, err)

	out := fastjsonext.PrintGraphQLResponse(resolvable.data, resolvable.errors)
	// top-1 and top-3 have real data; top-2 is null from negative cache
	assert.Equal(t, `{"data":{"products":[{"upc":"top-1","name":"Trilby","price":11},null,{"upc":"top-3","name":"Boater","price":33}]}}`, out)

	// Cache log: 1 batch get (3 hits including negative), no set (nothing new to write)
	log := cache.GetLog()
	require.Equal(t, 1, len(log))
	assert.Equal(t, "get", log[0].Operation)
	assert.Equal(t, []CacheLogItem{
		{Key: `{"__typename":"Product","key":{"upc":"top-1"}}`, Hit: true},
		{Key: `{"__typename":"Product","key":{"upc":"top-2"}}`, Hit: true},
		{Key: `{"__typename":"Product","key":{"upc":"top-3"}}`, Hit: true},
	}, log[0].Items) // All 3 are cache hits (including null sentinel)
}

// TestBatchEntityCache_AnalyticsTracking exercises the analytics event recording
// in applyRootFetchL2Results (loader_cache.go ~lines 1150-1156 for misses,
// 1232-1242 for hits). Verifies that CacheKeyHit and CacheKeyMiss events are
// correctly recorded when analytics is enabled.
func TestBatchEntityCache_AnalyticsTracking(t *testing.T) {
	ctrl := gomock.NewController(t)

	cache := NewFakeLoaderCache()

	// Seed cache with 2 of 3 products (top-1 and top-3 cached, top-2 missing)
	err := cache.Set(context.Background(), withCacheEntryTTL([]*CacheEntry{
		{Key: `{"__typename":"Product","key":{"upc":"top-1"}}`, Value: []byte(`{"upc":"top-1","name":"Trilby","price":11}`)},
		{Key: `{"__typename":"Product","key":{"upc":"top-3"}}`, Value: []byte(`{"upc":"top-3","name":"Boater","price":33}`)},
	}, 30*time.Second))
	require.NoError(t, err)
	cache.ClearLog()

	// Subgraph called once for the missing product (top-2)
	rootDS := NewMockDataSource(ctrl)
	rootDS.EXPECT().Load(gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, headers any, input []byte) ([]byte, error) {
			return []byte(`{"data":{"products":[{"upc":"top-1","name":"Trilby","price":11},{"upc":"top-2","name":"Fedora","price":22},{"upc":"top-3","name":"Boater","price":33}]}}`), nil
		}).Times(1)

	tmpl := newBatchProductsCacheKeyTemplate()
	provides := newBatchProductsProvidesData()

	response := newBatchProductsResponse(rootDS, tmpl, provides)

	loader := &Loader{caches: map[string]LoaderCache{"default": cache}}
	ctx := NewContext(context.Background())
	ctx.Variables = astjson.MustParseBytes([]byte(`{"upcs":["top-1","top-2","top-3"]}`))
	ctx.ExecutionOptions.DisableSubgraphRequestDeduplication = true
	ctx.ExecutionOptions.Caching.EnableL1Cache = false
	ctx.ExecutionOptions.Caching.EnableL2Cache = true
	ctx.ExecutionOptions.Caching.EnableCacheAnalytics = true

	ar := arena.NewMonotonicArena(arena.WithMinBufferSize(4096))
	resolvable := NewResolvable(ar, ResolvableOptions{})
	err = resolvable.Init(ctx, nil, ast.OperationTypeQuery)
	require.NoError(t, err)

	err = loader.LoadGraphQLResponseData(ctx, response, resolvable)
	require.NoError(t, err)

	out := fastjsonext.PrintGraphQLResponse(resolvable.data, resolvable.errors)
	assert.Equal(t, `{"data":{"products":[{"upc":"top-1","name":"Trilby","price":11},{"upc":"top-2","name":"Fedora","price":22},{"upc":"top-3","name":"Boater","price":33}]}}`, out)

	// Verify analytics: 2 L2 hits (top-1, top-3) + 1 L2 miss (top-2)
	stats := ctx.GetCacheStats()
	require.Equal(t, 3, len(stats.L2Reads))
	assert.Equal(t, CacheKeyEvent{
		CacheKey:   `{"__typename":"Product","key":{"upc":"top-1"}}`,
		EntityType: "Query",     // Root field fetch uses the root type name
		Kind:       CacheKeyHit, // top-1 was seeded in L2 cache
		DataSource: "products",
		ByteSize:   len(`{"upc":"top-1","name":"Trilby","price":11}`),
		CacheAgeMs: stats.L2Reads[0].CacheAgeMs, // dynamic, just preserve actual
	}, stats.L2Reads[0])
	assert.Equal(t, CacheKeyEvent{
		CacheKey:   `{"__typename":"Product","key":{"upc":"top-2"}}`,
		EntityType: "Query",      // Root field fetch uses the root type name
		Kind:       CacheKeyMiss, // top-2 was not in L2 cache
		DataSource: "products",
		ByteSize:   0,
	}, stats.L2Reads[1])
	assert.Equal(t, CacheKeyEvent{
		CacheKey:   `{"__typename":"Product","key":{"upc":"top-3"}}`,
		EntityType: "Query",     // Root field fetch uses the root type name
		Kind:       CacheKeyHit, // top-3 was seeded in L2 cache
		DataSource: "products",
		ByteSize:   len(`{"upc":"top-3","name":"Boater","price":33}`),
		CacheAgeMs: stats.L2Reads[2].CacheAgeMs, // dynamic, just preserve actual
	}, stats.L2Reads[2])
}

// TestUpdateL2Cache_MutationSkipsWithoutFlag exercises the early return in
// updateL2Cache (loader_cache.go ~lines 1479-1482).
// When the operation is a mutation and enableMutationL2CachePopulation is false,
// updateL2Cache must return immediately without writing to the L2 cache.
func TestUpdateL2Cache_MutationSkipsWithoutFlag(t *testing.T) {
	ctrl := gomock.NewController(t)

	cache := NewFakeLoaderCache()

	// Subgraph returns a product (mutation result)
	rootDS := NewMockDataSource(ctrl)
	rootDS.EXPECT().Load(gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, headers any, input []byte) ([]byte, error) {
			return []byte(`{"data":{"createProduct":{"upc":"new-1","name":"NewHat","price":99}}}`), nil
		}).Times(1)

	tmpl := NewRootQueryCacheKeyTemplate(
		[]QueryField{
			{
				Coordinate: GraphCoordinate{TypeName: "Mutation", FieldName: "createProduct"},
				Args: []FieldArgument{
					{
						Name: "upc",
						Variable: &ContextVariable{
							Path:     []string{"upc"},
							Renderer: NewCacheKeyVariableRenderer(),
						},
					},
				},
			},
		},
		nil,
	)

	response := &GraphQLResponse{
		Info: &GraphQLResponseInfo{OperationType: ast.OperationTypeMutation},
		Fetches: Sequence(
			SingleWithPath(&SingleFetch{
				FetchConfiguration: FetchConfiguration{
					DataSource: rootDS,
					PostProcessing: PostProcessingConfiguration{
						SelectResponseDataPath: []string{"data"},
					},
					Caching: FetchCacheConfiguration{
						Enabled:          true,
						CacheName:        "default",
						TTL:              30 * time.Second,
						CacheKeyTemplate: tmpl,
					},
				},
				InputTemplate: InputTemplate{
					Segments: []TemplateSegment{
						{Data: []byte(`{"method":"POST","url":"http://products","body":{"query":"mutation { createProduct(upc: $upc) { upc name price } }"}}`), SegmentType: StaticSegmentType},
					},
				},
				Info: &FetchInfo{
					DataSourceID:   "products",
					DataSourceName: "products",
					RootFields:     []GraphCoordinate{{TypeName: "Mutation", FieldName: "createProduct"}},
					OperationType:  ast.OperationTypeMutation,
				},
				DataSourceIdentifier: []byte("graphql_datasource.Source"),
			}, "mutation"),
		),
		Data: &Object{
			Fields: []*Field{
				{
					Name: []byte("createProduct"),
					Value: &Object{
						Path: []string{"createProduct"},
						Fields: []*Field{
							{Name: []byte("upc"), Value: &String{Path: []string{"upc"}}},
							{Name: []byte("name"), Value: &String{Path: []string{"name"}}},
							{Name: []byte("price"), Value: &Scalar{Path: []string{"price"}}},
						},
					},
				},
			},
		},
	}

	loader := &Loader{caches: map[string]LoaderCache{"default": cache}}
	ctx := NewContext(context.Background())
	ctx.Variables = astjson.MustParseBytes([]byte(`{"upc":"new-1"}`))
	ctx.ExecutionOptions.DisableSubgraphRequestDeduplication = true
	ctx.ExecutionOptions.Caching.EnableL1Cache = false
	ctx.ExecutionOptions.Caching.EnableL2Cache = true

	ar := arena.NewMonotonicArena(arena.WithMinBufferSize(4096))
	resolvable := NewResolvable(ar, ResolvableOptions{})
	err := resolvable.Init(ctx, nil, ast.OperationTypeMutation)
	require.NoError(t, err)

	err = loader.LoadGraphQLResponseData(ctx, response, resolvable)
	require.NoError(t, err)

	out := fastjsonext.PrintGraphQLResponse(resolvable.data, resolvable.errors)
	assert.Equal(t, `{"data":{"createProduct":{"upc":"new-1","name":"NewHat","price":99}}}`, out)

	// Cache log: no set operations — mutation without enableMutationL2CachePopulation
	// skips L2 cache writes entirely
	log := cache.GetLog()
	for _, entry := range log {
		assert.NotEqual(t, "set", entry.Operation, "mutation without enableMutationL2CachePopulation should not write to L2 cache")
	}

	// Verify cache is empty — nothing was stored
	assert.Nil(t, cache.GetValue(`{"__typename":"Mutation","field":"createProduct","args":{"upc":"new-1"}}`))
}

// TestBatchEntityCache_TracingEnabled exercises the tracing code paths in
// applyRootFetchL2Results and updateL2Cache that record cache trace data
// (L2 miss/hit counts, duration, keys) when TracingOptions.Enable is true.
func TestBatchEntityCache_TracingEnabled(t *testing.T) {
	ctrl := gomock.NewController(t)

	cache := NewFakeLoaderCache()

	rootDS := NewMockDataSource(ctrl)
	rootDS.EXPECT().Load(gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, headers any, input []byte) ([]byte, error) {
			return []byte(`{"data":{"products":[{"upc":"top-1","name":"Trilby","price":11},{"upc":"top-2","name":"Fedora","price":22}]}}`), nil
		}).Times(1)

	response := newBatchProductsResponse(
		rootDS,
		newBatchProductsCacheKeyTemplate(),
		newBatchProductsProvidesData(),
	)

	loader := &Loader{caches: map[string]LoaderCache{"default": cache}}
	ctx := NewContext(context.Background())
	ctx.Variables = astjson.MustParseBytes([]byte(`{"upcs":["top-1","top-2"]}`))
	ctx.ExecutionOptions.DisableSubgraphRequestDeduplication = true
	ctx.ExecutionOptions.Caching.EnableL1Cache = false
	ctx.ExecutionOptions.Caching.EnableL2Cache = true
	// Enable tracing to exercise tracing branches in applyRootFetchL2Results + updateL2Cache
	ctx.TracingOptions = TraceOptions{
		Enable:                        true,
		EnablePredictableDebugTimings: true,
	}

	ar := arena.NewMonotonicArena(arena.WithMinBufferSize(4096))
	resolvable := NewResolvable(ar, ResolvableOptions{})
	err := resolvable.Init(ctx, nil, ast.OperationTypeQuery)
	require.NoError(t, err)

	err = loader.LoadGraphQLResponseData(ctx, response, resolvable)
	require.NoError(t, err)

	out := fastjsonext.PrintGraphQLResponse(resolvable.data, resolvable.errors)
	assert.Equal(t, `{"data":{"products":[{"upc":"top-1","name":"Trilby","price":11},{"upc":"top-2","name":"Fedora","price":22}]}}`, out)

	// Cache log: 1 get (2 misses) + 1 set (2 entries)
	log := cache.GetLog()
	require.Equal(t, 2, len(log))
	assert.Equal(t, "get", log[0].Operation)
	assert.Equal(t, []CacheLogItem{
		{Key: `{"__typename":"Product","key":{"upc":"top-1"}}`, Hit: false},
		{Key: `{"__typename":"Product","key":{"upc":"top-2"}}`, Hit: false},
	}, log[0].Items)
	assert.Equal(t, "set", log[1].Operation)
}

// TestBatchEntityCache_L2DisabledSkipsCache exercises the L2 disabled early return
// in tryCacheLoad. When EnableL2Cache is false, no cache operations should occur.
func TestBatchEntityCache_L2DisabledSkipsCache(t *testing.T) {
	ctrl := gomock.NewController(t)

	cache := NewFakeLoaderCache()
	// Seed cache - but it should never be read since L2 is disabled
	err := cache.Set(context.Background(), withCacheEntryTTL([]*CacheEntry{
		{Key: `{"__typename":"Product","key":{"upc":"top-1"}}`, Value: []byte(`{"upc":"top-1","name":"Trilby","price":11}`)},
	}, 30*time.Second))
	require.NoError(t, err)
	cache.ClearLog()

	rootDS := NewMockDataSource(ctrl)
	rootDS.EXPECT().Load(gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, headers any, input []byte) ([]byte, error) {
			return []byte(`{"data":{"products":[{"upc":"top-1","name":"Trilby","price":11}]}}`), nil
		}).Times(1)

	response := newBatchProductsResponse(
		rootDS,
		newBatchProductsCacheKeyTemplate(),
		newBatchProductsProvidesData(),
	)

	loader := &Loader{caches: map[string]LoaderCache{"default": cache}}
	ctx := NewContext(context.Background())
	ctx.Variables = astjson.MustParseBytes([]byte(`{"upcs":["top-1"]}`))
	ctx.ExecutionOptions.DisableSubgraphRequestDeduplication = true
	ctx.ExecutionOptions.Caching.EnableL1Cache = false
	ctx.ExecutionOptions.Caching.EnableL2Cache = false // L2 disabled

	ar := arena.NewMonotonicArena(arena.WithMinBufferSize(4096))
	resolvable := NewResolvable(ar, ResolvableOptions{})
	err = resolvable.Init(ctx, nil, ast.OperationTypeQuery)
	require.NoError(t, err)

	err = loader.LoadGraphQLResponseData(ctx, response, resolvable)
	require.NoError(t, err)

	out := fastjsonext.PrintGraphQLResponse(resolvable.data, resolvable.errors)
	assert.Equal(t, `{"data":{"products":[{"upc":"top-1","name":"Trilby","price":11}]}}`, out)

	// No cache operations should have occurred
	assert.Equal(t, 0, len(cache.GetLog()))
}

// TestBatchEntityCache_KeyInterceptorApplied exercises the L2CacheKeyInterceptor
// path. When an interceptor is set, it transforms the cache keys before L2 read/write.
func TestBatchEntityCache_KeyInterceptorApplied(t *testing.T) {
	ctrl := gomock.NewController(t)

	cache := NewFakeLoaderCache()

	rootDS := NewMockDataSource(ctrl)
	rootDS.EXPECT().Load(gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, headers any, input []byte) ([]byte, error) {
			return []byte(`{"data":{"products":[{"upc":"top-1","name":"Trilby","price":11}]}}`), nil
		}).Times(1)

	response := newBatchProductsResponse(
		rootDS,
		newBatchProductsCacheKeyTemplate(),
		newBatchProductsProvidesData(),
	)

	loader := &Loader{caches: map[string]LoaderCache{"default": cache}}
	ctx := NewContext(context.Background())
	ctx.Variables = astjson.MustParseBytes([]byte(`{"upcs":["top-1"]}`))
	ctx.ExecutionOptions.DisableSubgraphRequestDeduplication = true
	ctx.ExecutionOptions.Caching.EnableL1Cache = false
	ctx.ExecutionOptions.Caching.EnableL2Cache = true
	// Interceptor prepends "tenant42:" to every cache key
	ctx.ExecutionOptions.Caching.L2CacheKeyInterceptor = func(ctx context.Context, key string, info L2CacheKeyInterceptorInfo) string {
		return "tenant42:" + key
	}

	ar := arena.NewMonotonicArena(arena.WithMinBufferSize(4096))
	resolvable := NewResolvable(ar, ResolvableOptions{})
	err := resolvable.Init(ctx, nil, ast.OperationTypeQuery)
	require.NoError(t, err)

	err = loader.LoadGraphQLResponseData(ctx, response, resolvable)
	require.NoError(t, err)

	out := fastjsonext.PrintGraphQLResponse(resolvable.data, resolvable.errors)
	assert.Equal(t, `{"data":{"products":[{"upc":"top-1","name":"Trilby","price":11}]}}`, out)

	// Cache key should have been transformed by the interceptor
	log := cache.GetLog()
	require.GreaterOrEqual(t, len(log), 1)
	// The get operation should use the intercepted key
	assert.Equal(t, "get", log[0].Operation)
	assert.Equal(t, []CacheLogItem{
		{Key: `tenant42:{"__typename":"Product","key":{"upc":"top-1"}}`, Hit: false},
	}, log[0].Items)
}
