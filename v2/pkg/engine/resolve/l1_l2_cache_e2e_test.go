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

// TestL1L2CacheEndToEnd provides comprehensive end-to-end tests for the L1/L2 caching system.
//
// L1 Cache (Per-Request, In-Memory):
//   - Stored in Loader as sync.Map
//   - Lifecycle: Single GraphQL request
//   - Only used for entity fetches (not root fetches)
//   - Purpose: Prevents redundant fetches for same entity at different paths
//
// L2 Cache (External, Cross-Request):
//   - Uses LoaderCache interface implementations
//   - Lifecycle: Configured TTL, shared across requests
//   - Applies to both root fetches and entity fetches
//
// Lookup Order (entity fetches): L1 -> L2 -> Subgraph Fetch
// Lookup Order (root fetches): L2 -> Subgraph Fetch (no L1)

func TestL1L2CacheEndToEnd(t *testing.T) {
	// =============================================================================
	// L1 CACHE ONLY TESTS
	// =============================================================================

	t.Run("L1 Only - entity reuse within same request", func(t *testing.T) {
		// This test verifies that L1 cache prevents redundant entity fetches
		// within a single request when the same entity appears at multiple paths.
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		// Root fetch - get product with minimal data
		rootDS := NewMockDataSource(ctrl)
		rootDS.EXPECT().
			Load(gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, headers any, input []byte) ([]byte, error) {
				return []byte(`{"data":{"product":{"__typename":"Product","id":"prod-1"}}}`), nil
			}).Times(1)

		// First entity fetch - should be called (L1 miss)
		entityDS := NewMockDataSource(ctrl)
		entityDS.EXPECT().
			Load(gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, headers any, input []byte) ([]byte, error) {
				return []byte(`{"data":{"_entities":[{"__typename":"Product","id":"prod-1","name":"Product One","price":99.99}]}}`), nil
			}).Times(1)

		// Second entity fetch for same entity - should NOT be called (L1 hit)
		entityDS2 := NewMockDataSource(ctrl)
		entityDS2.EXPECT().
			Load(gomock.Any(), gomock.Any(), gomock.Any()).
			Times(0) // L1 should prevent this call

		productCacheKeyTemplate := &EntityQueryCacheKeyTemplate{
			Keys: NewResolvableObjectVariable(&Object{
				Fields: []*Field{
					{Name: []byte("__typename"), Value: &String{Path: []string{"__typename"}}},
					{Name: []byte("id"), Value: &String{Path: []string{"id"}}},
				},
			}),
		}

		providesData := &Object{
			Fields: []*Field{
				{Name: []byte("id"), Value: &Scalar{Path: []string{"id"}, Nullable: false}},
				{Name: []byte("name"), Value: &Scalar{Path: []string{"name"}, Nullable: false}},
				{Name: []byte("price"), Value: &Scalar{Path: []string{"price"}, Nullable: false}},
			},
		}

		response := &GraphQLResponse{
			Info: &GraphQLResponseInfo{OperationType: ast.OperationTypeQuery},
			Fetches: Sequence(
				// Root fetch
				SingleWithPath(&SingleFetch{
					FetchConfiguration: FetchConfiguration{
						DataSource:     rootDS,
						PostProcessing: PostProcessingConfiguration{SelectResponseDataPath: []string{"data"}},
					},
					InputTemplate: InputTemplate{
						Segments: []TemplateSegment{{Data: []byte(`{"method":"POST","url":"http://products"}`), SegmentType: StaticSegmentType}},
					},
					DataSourceIdentifier: []byte("graphql_datasource.Source"),
				}, "query"),
				// First entity fetch
				SingleWithPath(&BatchEntityFetch{
					Input: BatchInput{
						Header: InputTemplate{Segments: []TemplateSegment{{Data: []byte(`{"method":"POST","url":"http://products","body":{"query":"entity1","variables":{"representations":[`), SegmentType: StaticSegmentType}}},
						Items:  []InputTemplate{{Segments: []TemplateSegment{{SegmentType: VariableSegmentType, VariableKind: ResolvableObjectVariableKind, Renderer: NewGraphQLVariableResolveRenderer(&Object{Fields: []*Field{{Name: []byte("__typename"), Value: &String{Path: []string{"__typename"}}}, {Name: []byte("id"), Value: &String{Path: []string{"id"}}}}})}}}},
						Footer: InputTemplate{Segments: []TemplateSegment{{Data: []byte(`]}}}`), SegmentType: StaticSegmentType}}},
					},
					DataSource:     entityDS,
					PostProcessing: PostProcessingConfiguration{SelectResponseDataPath: []string{"data", "_entities"}},
					Info: &FetchInfo{
						DataSourceName: "products",
						OperationType:  ast.OperationTypeQuery,
						ProvidesData:   providesData,
					},
					Caching: FetchCacheConfiguration{
						Enabled:          true,
						CacheName:        "default",
						CacheKeyTemplate: productCacheKeyTemplate,
						UseL1Cache:       true,
					},
				}, "query.product", ObjectPath("product")),
				// Second entity fetch (same entity at different path)
				SingleWithPath(&BatchEntityFetch{
					Input: BatchInput{
						Header: InputTemplate{Segments: []TemplateSegment{{Data: []byte(`{"method":"POST","url":"http://products","body":{"query":"entity2","variables":{"representations":[`), SegmentType: StaticSegmentType}}},
						Items:  []InputTemplate{{Segments: []TemplateSegment{{SegmentType: VariableSegmentType, VariableKind: ResolvableObjectVariableKind, Renderer: NewGraphQLVariableResolveRenderer(&Object{Fields: []*Field{{Name: []byte("__typename"), Value: &String{Path: []string{"__typename"}}}, {Name: []byte("id"), Value: &String{Path: []string{"id"}}}}})}}}},
						Footer: InputTemplate{Segments: []TemplateSegment{{Data: []byte(`]}}}`), SegmentType: StaticSegmentType}}},
					},
					DataSource:     entityDS2,
					PostProcessing: PostProcessingConfiguration{SelectResponseDataPath: []string{"data", "_entities"}},
					Info: &FetchInfo{
						DataSourceName: "products",
						OperationType:  ast.OperationTypeQuery,
						ProvidesData:   providesData,
					},
					Caching: FetchCacheConfiguration{
						Enabled:          true,
						CacheName:        "default",
						CacheKeyTemplate: productCacheKeyTemplate,
						UseL1Cache:       true,
					},
				}, "query.product.related", ObjectPath("product")),
			),
			Data: &Object{
				Fields: []*Field{
					{
						Name: []byte("product"),
						Value: &Object{
							Path: []string{"product"},
							Fields: []*Field{
								{Name: []byte("id"), Value: &String{Path: []string{"id"}}},
								{Name: []byte("name"), Value: &String{Path: []string{"name"}}},
								{Name: []byte("price"), Value: &Float{Path: []string{"price"}}},
							},
						},
					},
				},
			},
		}

		loader := &Loader{}

		ctx := NewContext(context.Background())
		ctx.ExecutionOptions.DisableSubgraphRequestDeduplication = true
		ctx.ExecutionOptions.Caching.EnableL1Cache = true
		ctx.ExecutionOptions.Caching.EnableL2Cache = false // L1 only

		ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
		resolvable := NewResolvable(ar, ResolvableOptions{})
		err := resolvable.Init(ctx, nil, ast.OperationTypeQuery)
		require.NoError(t, err)

		err = loader.LoadGraphQLResponseData(ctx, response, resolvable)
		require.NoError(t, err)

		out := fastjsonext.PrintGraphQLResponse(resolvable.data, resolvable.errors)
		assert.Equal(t, `{"data":{"product":{"__typename":"Product","id":"prod-1","name":"Product One","price":99.99}}}`, out)
	})

	t.Run("L1 Only - disabled means separate fetches", func(t *testing.T) {
		// When L1 is disabled, same entity at different paths should trigger separate fetches
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		rootDS := NewMockDataSource(ctrl)
		rootDS.EXPECT().
			Load(gomock.Any(), gomock.Any(), gomock.Any()).
			Return([]byte(`{"data":{"product":{"__typename":"Product","id":"prod-1"}}}`), nil).Times(1)

		// Both entity fetches should be called when L1 is disabled
		entityDS := NewMockDataSource(ctrl)
		entityDS.EXPECT().
			Load(gomock.Any(), gomock.Any(), gomock.Any()).
			Return([]byte(`{"data":{"_entities":[{"__typename":"Product","id":"prod-1","name":"Product One"}]}}`), nil).Times(2)

		productCacheKeyTemplate := &EntityQueryCacheKeyTemplate{
			Keys: NewResolvableObjectVariable(&Object{
				Fields: []*Field{
					{Name: []byte("__typename"), Value: &String{Path: []string{"__typename"}}},
					{Name: []byte("id"), Value: &String{Path: []string{"id"}}},
				},
			}),
		}

		providesData := &Object{
			Fields: []*Field{
				{Name: []byte("id"), Value: &Scalar{Path: []string{"id"}, Nullable: false}},
				{Name: []byte("name"), Value: &Scalar{Path: []string{"name"}, Nullable: false}},
			},
		}

		response := &GraphQLResponse{
			Info: &GraphQLResponseInfo{OperationType: ast.OperationTypeQuery},
			Fetches: Sequence(
				SingleWithPath(&SingleFetch{
					FetchConfiguration: FetchConfiguration{
						DataSource:     rootDS,
						PostProcessing: PostProcessingConfiguration{SelectResponseDataPath: []string{"data"}},
					},
					InputTemplate:        InputTemplate{Segments: []TemplateSegment{{Data: []byte(`{}`), SegmentType: StaticSegmentType}}},
					DataSourceIdentifier: []byte("graphql_datasource.Source"),
				}, "query"),
				SingleWithPath(&BatchEntityFetch{
					Input: BatchInput{
						Header: InputTemplate{Segments: []TemplateSegment{{Data: []byte(`{"body":{"representations":[`), SegmentType: StaticSegmentType}}},
						Items:  []InputTemplate{{Segments: []TemplateSegment{{SegmentType: VariableSegmentType, VariableKind: ResolvableObjectVariableKind, Renderer: NewGraphQLVariableResolveRenderer(&Object{Fields: []*Field{{Name: []byte("__typename"), Value: &String{Path: []string{"__typename"}}}, {Name: []byte("id"), Value: &String{Path: []string{"id"}}}}})}}}},
						Footer: InputTemplate{Segments: []TemplateSegment{{Data: []byte(`]}}}`), SegmentType: StaticSegmentType}}},
					},
					DataSource:     entityDS,
					PostProcessing: PostProcessingConfiguration{SelectResponseDataPath: []string{"data", "_entities"}},
					Info:           &FetchInfo{DataSourceName: "products", OperationType: ast.OperationTypeQuery, ProvidesData: providesData},
					Caching:        FetchCacheConfiguration{Enabled: true, CacheName: "default", CacheKeyTemplate: productCacheKeyTemplate, UseL1Cache: true},
				}, "query.product", ObjectPath("product")),
				SingleWithPath(&BatchEntityFetch{
					Input: BatchInput{
						Header: InputTemplate{Segments: []TemplateSegment{{Data: []byte(`{"body":{"representations":[`), SegmentType: StaticSegmentType}}},
						Items:  []InputTemplate{{Segments: []TemplateSegment{{SegmentType: VariableSegmentType, VariableKind: ResolvableObjectVariableKind, Renderer: NewGraphQLVariableResolveRenderer(&Object{Fields: []*Field{{Name: []byte("__typename"), Value: &String{Path: []string{"__typename"}}}, {Name: []byte("id"), Value: &String{Path: []string{"id"}}}}})}}}},
						Footer: InputTemplate{Segments: []TemplateSegment{{Data: []byte(`]}}}`), SegmentType: StaticSegmentType}}},
					},
					DataSource:     entityDS,
					PostProcessing: PostProcessingConfiguration{SelectResponseDataPath: []string{"data", "_entities"}},
					Info:           &FetchInfo{DataSourceName: "products", OperationType: ast.OperationTypeQuery, ProvidesData: providesData},
					Caching:        FetchCacheConfiguration{Enabled: true, CacheName: "default", CacheKeyTemplate: productCacheKeyTemplate, UseL1Cache: true},
				}, "query.product.related", ObjectPath("product")),
			),
			Data: &Object{
				Fields: []*Field{
					{Name: []byte("product"), Value: &Object{Path: []string{"product"}, Fields: []*Field{
						{Name: []byte("id"), Value: &String{Path: []string{"id"}}},
						{Name: []byte("name"), Value: &String{Path: []string{"name"}}},
					}}},
				},
			},
		}

		loader := &Loader{}

		ctx := NewContext(context.Background())
		ctx.ExecutionOptions.DisableSubgraphRequestDeduplication = true
		ctx.ExecutionOptions.Caching.EnableL1Cache = false // Disabled
		ctx.ExecutionOptions.Caching.EnableL2Cache = false

		ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
		resolvable := NewResolvable(ar, ResolvableOptions{})
		err := resolvable.Init(ctx, nil, ast.OperationTypeQuery)
		require.NoError(t, err)

		err = loader.LoadGraphQLResponseData(ctx, response, resolvable)
		require.NoError(t, err)

	})

	// =============================================================================
	// L2 CACHE ONLY TESTS
	// =============================================================================

	t.Run("L2 Only - miss then hit across requests", func(t *testing.T) {
		// This test verifies L2 cache works for cross-request caching
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		cache := NewFakeLoaderCache()

		// Root DS for first request
		rootDS1 := NewMockDataSource(ctrl)
		rootDS1.EXPECT().
			Load(gomock.Any(), gomock.Any(), gomock.Any()).
			Return([]byte(`{"data":{"product":{"__typename":"Product","id":"prod-1"}}}`), nil).Times(1)

		// First request: entity DS called (cache miss)
		entityDS1 := NewMockDataSource(ctrl)
		entityDS1.EXPECT().
			Load(gomock.Any(), gomock.Any(), gomock.Any()).
			Return([]byte(`{"data":{"_entities":[{"__typename":"Product","id":"prod-1","name":"Cached Product"}]}}`), nil).Times(1)

		// Root DS for second request
		rootDS2 := NewMockDataSource(ctrl)
		rootDS2.EXPECT().
			Load(gomock.Any(), gomock.Any(), gomock.Any()).
			Return([]byte(`{"data":{"product":{"__typename":"Product","id":"prod-1"}}}`), nil).Times(1)

		// Second request: entity DS NOT called (cache hit)
		entityDS2 := NewMockDataSource(ctrl)
		entityDS2.EXPECT().
			Load(gomock.Any(), gomock.Any(), gomock.Any()).
			Times(0) // Cache hit

		productCacheKeyTemplate := &EntityQueryCacheKeyTemplate{
			Keys: NewResolvableObjectVariable(&Object{
				Fields: []*Field{
					{Name: []byte("__typename"), Value: &String{Path: []string{"__typename"}}},
					{Name: []byte("id"), Value: &String{Path: []string{"id"}}},
				},
			}),
		}

		providesData := &Object{
			Fields: []*Field{
				{Name: []byte("id"), Value: &Scalar{Path: []string{"id"}, Nullable: false}},
				{Name: []byte("name"), Value: &Scalar{Path: []string{"name"}, Nullable: false}},
			},
		}

		createResponse := func(rootDS, entityDS DataSource) *GraphQLResponse {
			return &GraphQLResponse{
				Info: &GraphQLResponseInfo{OperationType: ast.OperationTypeQuery},
				Fetches: Sequence(
					SingleWithPath(&SingleFetch{
						FetchConfiguration: FetchConfiguration{
							DataSource:     rootDS,
							PostProcessing: PostProcessingConfiguration{SelectResponseDataPath: []string{"data"}},
						},
						InputTemplate:        InputTemplate{Segments: []TemplateSegment{{Data: []byte(`{}`), SegmentType: StaticSegmentType}}},
						DataSourceIdentifier: []byte("graphql_datasource.Source"),
					}, "query"),
					SingleWithPath(&BatchEntityFetch{
						Input: BatchInput{
							Header: InputTemplate{Segments: []TemplateSegment{{Data: []byte(`{"representations":[`), SegmentType: StaticSegmentType}}},
							Items:  []InputTemplate{{Segments: []TemplateSegment{{SegmentType: VariableSegmentType, VariableKind: ResolvableObjectVariableKind, Renderer: NewGraphQLVariableResolveRenderer(&Object{Fields: []*Field{{Name: []byte("__typename"), Value: &String{Path: []string{"__typename"}}}, {Name: []byte("id"), Value: &String{Path: []string{"id"}}}}})}}}},
							Footer: InputTemplate{Segments: []TemplateSegment{{Data: []byte(`]}`), SegmentType: StaticSegmentType}}},
						},
						DataSource:     entityDS,
						PostProcessing: PostProcessingConfiguration{SelectResponseDataPath: []string{"data", "_entities"}},
						Info:           &FetchInfo{DataSourceName: "products", OperationType: ast.OperationTypeQuery, ProvidesData: providesData},
						Caching:        FetchCacheConfiguration{Enabled: true, CacheName: "default", CacheKeyTemplate: productCacheKeyTemplate, TTL: time.Minute, UseL1Cache: true},
					}, "query.product", ObjectPath("product")),
				),
				Data: &Object{
					Fields: []*Field{
						{Name: []byte("product"), Value: &Object{Path: []string{"product"}, Fields: []*Field{
							{Name: []byte("id"), Value: &String{Path: []string{"id"}}},
							{Name: []byte("name"), Value: &String{Path: []string{"name"}}},
						}}},
					},
				},
			}
		}

		// First request (cache miss)
		ctx1 := NewContext(context.Background())
		ctx1.ExecutionOptions.DisableSubgraphRequestDeduplication = true
		ctx1.ExecutionOptions.Caching.EnableL1Cache = false
		ctx1.ExecutionOptions.Caching.EnableL2Cache = true

		loader1 := &Loader{caches: map[string]LoaderCache{"default": cache}}

		ar1 := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
		resolvable1 := NewResolvable(ar1, ResolvableOptions{})
		err := resolvable1.Init(ctx1, nil, ast.OperationTypeQuery)
		require.NoError(t, err)

		err = loader1.LoadGraphQLResponseData(ctx1, createResponse(rootDS1, entityDS1), resolvable1)
		require.NoError(t, err)

		productKey := []string{`{"__typename":"Product","key":{"id":"prod-1"}}`}

		log := cache.GetLog()
		wantFirstLog := []CacheLogEntry{
			// _entities(Product) — L2 miss, product not yet cached
			{Operation: "get", Keys: productKey, Hits: []bool{false}},
			// _entities(Product) — store fetched product data in L2
			{Operation: "set", Keys: productKey, TTL: time.Minute},
		}
		assert.Equal(t, wantFirstLog, log, "First request: L2 miss then set")

		// Second request (cache hit) — new loader but same L2 cache instance
		cache.ClearLog()
		ctx2 := NewContext(context.Background())
		ctx2.ExecutionOptions.DisableSubgraphRequestDeduplication = true
		ctx2.ExecutionOptions.Caching.EnableL1Cache = false
		ctx2.ExecutionOptions.Caching.EnableL2Cache = true

		loader2 := &Loader{caches: map[string]LoaderCache{"default": cache}}

		ar2 := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
		resolvable2 := NewResolvable(ar2, ResolvableOptions{})
		err = resolvable2.Init(ctx2, nil, ast.OperationTypeQuery)
		require.NoError(t, err)

		err = loader2.LoadGraphQLResponseData(ctx2, createResponse(rootDS2, entityDS2), resolvable2)
		require.NoError(t, err)

		log2 := cache.GetLog()
		wantSecondLog := []CacheLogEntry{
			// _entities(Product) — L2 hit, product cached from first request; no DS call needed
			{Operation: "get", Keys: productKey, Hits: []bool{true}},
		}
		assert.Equal(t, wantSecondLog, log2, "Second request: L2 hit only")
	})

	t.Run("L2 Only - disabled means no cache operations", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		cache := NewFakeLoaderCache()

		// Root DS for both requests
		rootDS := NewMockDataSource(ctrl)
		rootDS.EXPECT().
			Load(gomock.Any(), gomock.Any(), gomock.Any()).
			Return([]byte(`{"data":{"product":{"__typename":"Product","id":"prod-1"}}}`), nil).Times(2)

		// Entity DS called both times (no cache)
		entityDS := NewMockDataSource(ctrl)
		entityDS.EXPECT().
			Load(gomock.Any(), gomock.Any(), gomock.Any()).
			Return([]byte(`{"data":{"_entities":[{"__typename":"Product","id":"prod-1","name":"Product"}]}}`), nil).Times(2) // Called both times

		productCacheKeyTemplate := &EntityQueryCacheKeyTemplate{
			Keys: NewResolvableObjectVariable(&Object{
				Fields: []*Field{
					{Name: []byte("__typename"), Value: &String{Path: []string{"__typename"}}},
					{Name: []byte("id"), Value: &String{Path: []string{"id"}}},
				},
			}),
		}

		providesData := &Object{Fields: []*Field{
			{Name: []byte("id"), Value: &Scalar{Path: []string{"id"}, Nullable: false}},
			{Name: []byte("name"), Value: &Scalar{Path: []string{"name"}, Nullable: false}},
		}}

		response := &GraphQLResponse{
			Info: &GraphQLResponseInfo{OperationType: ast.OperationTypeQuery},
			Fetches: Sequence(
				SingleWithPath(&SingleFetch{
					FetchConfiguration: FetchConfiguration{
						DataSource:     rootDS,
						PostProcessing: PostProcessingConfiguration{SelectResponseDataPath: []string{"data"}},
					},
					InputTemplate:        InputTemplate{Segments: []TemplateSegment{{Data: []byte(`{}`), SegmentType: StaticSegmentType}}},
					DataSourceIdentifier: []byte("graphql_datasource.Source"),
				}, "query"),
				SingleWithPath(&BatchEntityFetch{
					Input: BatchInput{
						Header: InputTemplate{Segments: []TemplateSegment{{Data: []byte(`{"representations":[`), SegmentType: StaticSegmentType}}},
						Items:  []InputTemplate{{Segments: []TemplateSegment{{SegmentType: VariableSegmentType, VariableKind: ResolvableObjectVariableKind, Renderer: NewGraphQLVariableResolveRenderer(&Object{Fields: []*Field{{Name: []byte("__typename"), Value: &String{Path: []string{"__typename"}}}, {Name: []byte("id"), Value: &String{Path: []string{"id"}}}}})}}}},
						Footer: InputTemplate{Segments: []TemplateSegment{{Data: []byte(`]}`), SegmentType: StaticSegmentType}}},
					},
					DataSource:     entityDS,
					PostProcessing: PostProcessingConfiguration{SelectResponseDataPath: []string{"data", "_entities"}},
					Info:           &FetchInfo{DataSourceName: "products", OperationType: ast.OperationTypeQuery, ProvidesData: providesData},
					Caching:        FetchCacheConfiguration{Enabled: true, CacheName: "default", CacheKeyTemplate: productCacheKeyTemplate, UseL1Cache: true},
				}, "query.product", ObjectPath("product")),
			),
			Data: &Object{Fields: []*Field{{Name: []byte("product"), Value: &Object{Path: []string{"product"}, Fields: []*Field{
				{Name: []byte("id"), Value: &String{Path: []string{"id"}}},
				{Name: []byte("name"), Value: &String{Path: []string{"name"}}},
			}}}}},
		}

		// Run twice with L2 disabled
		for range 2 {
			ctx := NewContext(context.Background())
			ctx.ExecutionOptions.DisableSubgraphRequestDeduplication = true
			ctx.ExecutionOptions.Caching.EnableL1Cache = false
			ctx.ExecutionOptions.Caching.EnableL2Cache = false // Disabled

			loader := &Loader{caches: map[string]LoaderCache{"default": cache}}

			ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
			resolvable := NewResolvable(ar, ResolvableOptions{})
			err := resolvable.Init(ctx, nil, ast.OperationTypeQuery)
			require.NoError(t, err)

			err = loader.LoadGraphQLResponseData(ctx, response, resolvable)
			require.NoError(t, err)
		}

		// Verify no cache operations occurred
		log := cache.GetLog()
		assert.Empty(t, log, "No cache operations should occur when L2 is disabled")
	})

	// =============================================================================
	// L1 + L2 COMBINED TESTS
	// =============================================================================

	t.Run("L1+L2 - L1 hit prevents L2 lookup", func(t *testing.T) {
		// When L1 has the data, L2 should not be consulted for entity fetches
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		cache := NewFakeLoaderCache()

		rootDS := NewMockDataSource(ctrl)
		rootDS.EXPECT().
			Load(gomock.Any(), gomock.Any(), gomock.Any()).
			Return([]byte(`{"data":{"product":{"__typename":"Product","id":"prod-1"}}}`), nil).Times(1)

		// First entity fetch populates both L1 and L2
		entityDS1 := NewMockDataSource(ctrl)
		entityDS1.EXPECT().
			Load(gomock.Any(), gomock.Any(), gomock.Any()).
			Return([]byte(`{"data":{"_entities":[{"__typename":"Product","id":"prod-1","name":"Product One"}]}}`), nil).Times(1)

		// Second entity fetch should hit L1 (no DS call, no L2 lookup needed)
		entityDS2 := NewMockDataSource(ctrl)
		entityDS2.EXPECT().
			Load(gomock.Any(), gomock.Any(), gomock.Any()).
			Times(0)

		productCacheKeyTemplate := &EntityQueryCacheKeyTemplate{
			Keys: NewResolvableObjectVariable(&Object{
				Fields: []*Field{
					{Name: []byte("__typename"), Value: &String{Path: []string{"__typename"}}},
					{Name: []byte("id"), Value: &String{Path: []string{"id"}}},
				},
			}),
		}

		providesData := &Object{Fields: []*Field{
			{Name: []byte("id"), Value: &Scalar{Path: []string{"id"}, Nullable: false}},
			{Name: []byte("name"), Value: &Scalar{Path: []string{"name"}, Nullable: false}},
		}}

		response := &GraphQLResponse{
			Info: &GraphQLResponseInfo{OperationType: ast.OperationTypeQuery},
			Fetches: Sequence(
				SingleWithPath(&SingleFetch{
					FetchConfiguration: FetchConfiguration{
						DataSource:     rootDS,
						PostProcessing: PostProcessingConfiguration{SelectResponseDataPath: []string{"data"}},
					},
					InputTemplate:        InputTemplate{Segments: []TemplateSegment{{Data: []byte(`{}`), SegmentType: StaticSegmentType}}},
					DataSourceIdentifier: []byte("graphql_datasource.Source"),
				}, "query"),
				SingleWithPath(&BatchEntityFetch{
					Input: BatchInput{
						Header: InputTemplate{Segments: []TemplateSegment{{Data: []byte(`{"representations":[`), SegmentType: StaticSegmentType}}},
						Items:  []InputTemplate{{Segments: []TemplateSegment{{SegmentType: VariableSegmentType, VariableKind: ResolvableObjectVariableKind, Renderer: NewGraphQLVariableResolveRenderer(&Object{Fields: []*Field{{Name: []byte("__typename"), Value: &String{Path: []string{"__typename"}}}, {Name: []byte("id"), Value: &String{Path: []string{"id"}}}}})}}}},
						Footer: InputTemplate{Segments: []TemplateSegment{{Data: []byte(`]}`), SegmentType: StaticSegmentType}}},
					},
					DataSource:     entityDS1,
					PostProcessing: PostProcessingConfiguration{SelectResponseDataPath: []string{"data", "_entities"}},
					Info:           &FetchInfo{DataSourceName: "products", OperationType: ast.OperationTypeQuery, ProvidesData: providesData},
					Caching:        FetchCacheConfiguration{Enabled: true, CacheName: "default", CacheKeyTemplate: productCacheKeyTemplate, TTL: time.Minute, UseL1Cache: true},
				}, "query.product", ObjectPath("product")),
				SingleWithPath(&BatchEntityFetch{
					Input: BatchInput{
						Header: InputTemplate{Segments: []TemplateSegment{{Data: []byte(`{"representations":[`), SegmentType: StaticSegmentType}}},
						Items:  []InputTemplate{{Segments: []TemplateSegment{{SegmentType: VariableSegmentType, VariableKind: ResolvableObjectVariableKind, Renderer: NewGraphQLVariableResolveRenderer(&Object{Fields: []*Field{{Name: []byte("__typename"), Value: &String{Path: []string{"__typename"}}}, {Name: []byte("id"), Value: &String{Path: []string{"id"}}}}})}}}},
						Footer: InputTemplate{Segments: []TemplateSegment{{Data: []byte(`]}`), SegmentType: StaticSegmentType}}},
					},
					DataSource:     entityDS2,
					PostProcessing: PostProcessingConfiguration{SelectResponseDataPath: []string{"data", "_entities"}},
					Info:           &FetchInfo{DataSourceName: "products", OperationType: ast.OperationTypeQuery, ProvidesData: providesData},
					Caching:        FetchCacheConfiguration{Enabled: true, CacheName: "default", CacheKeyTemplate: productCacheKeyTemplate, TTL: time.Minute, UseL1Cache: true},
				}, "query.product.related", ObjectPath("product")),
			),
			Data: &Object{Fields: []*Field{{Name: []byte("product"), Value: &Object{Path: []string{"product"}, Fields: []*Field{
				{Name: []byte("id"), Value: &String{Path: []string{"id"}}},
				{Name: []byte("name"), Value: &String{Path: []string{"name"}}},
			}}}}},
		}

		ctx := NewContext(context.Background())
		ctx.ExecutionOptions.DisableSubgraphRequestDeduplication = true
		ctx.ExecutionOptions.Caching.EnableL1Cache = true
		ctx.ExecutionOptions.Caching.EnableL2Cache = true

		loader := &Loader{caches: map[string]LoaderCache{"default": cache}}

		ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
		resolvable := NewResolvable(ar, ResolvableOptions{})
		err := resolvable.Init(ctx, nil, ast.OperationTypeQuery)
		require.NoError(t, err)

		err = loader.LoadGraphQLResponseData(ctx, response, resolvable)
		require.NoError(t, err)

		// Two sequential entity fetches for the same product (prod-1):
		// 1st fetch: L1 miss -> L2 miss -> DS call -> populate L1 + L2
		// 2nd fetch: L1 hit -> skip L2 and DS entirely
		// So L2 only sees operations from the 1st fetch
		productKey := []string{`{"__typename":"Product","key":{"id":"prod-1"}}`}
		log := cache.GetLog()
		wantLog := []CacheLogEntry{
			// 1st _entities(Product) — L1 miss, L2 miss
			{Operation: "get", Keys: productKey, Hits: []bool{false}},
			// 1st _entities(Product) — store fetched data in L2 (L1 also populated in-memory)
			{Operation: "set", Keys: productKey, TTL: time.Minute},
			// 2nd _entities(Product) — no L2 operations: L1 hit short-circuits
		}
		assert.Equal(t, wantLog, log, "L1 hit should prevent second L2 lookup")
	})

	t.Run("L1+L2 - L1 miss, L2 hit provides data", func(t *testing.T) {
		// When L1 misses but L2 has data, data should come from L2
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		cache := NewFakeLoaderCache()

		// Pre-populate L2 cache with correct key format: {"__typename":"Product","key":{"id":"prod-1"}}
		_ = cache.Set(context.Background(), []*CacheEntry{
			{Key: `{"__typename":"Product","key":{"id":"prod-1"}}`, Value: []byte(`{"__typename":"Product","id":"prod-1","name":"L2 Cached Product"}`)},
		}, time.Minute)
		cache.ClearLog() // Clear the set log

		rootDS := NewMockDataSource(ctrl)
		rootDS.EXPECT().
			Load(gomock.Any(), gomock.Any(), gomock.Any()).
			Return([]byte(`{"data":{"product":{"__typename":"Product","id":"prod-1"}}}`), nil).Times(1)

		// Entity DS should NOT be called (L2 hit)
		entityDS := NewMockDataSource(ctrl)
		entityDS.EXPECT().
			Load(gomock.Any(), gomock.Any(), gomock.Any()).
			Times(0)

		productCacheKeyTemplate := &EntityQueryCacheKeyTemplate{
			Keys: NewResolvableObjectVariable(&Object{
				Fields: []*Field{
					{Name: []byte("__typename"), Value: &String{Path: []string{"__typename"}}},
					{Name: []byte("id"), Value: &String{Path: []string{"id"}}},
				},
			}),
		}

		providesData := &Object{Fields: []*Field{
			{Name: []byte("id"), Value: &Scalar{Path: []string{"id"}, Nullable: false}},
			{Name: []byte("name"), Value: &Scalar{Path: []string{"name"}, Nullable: false}},
		}}

		response := &GraphQLResponse{
			Info: &GraphQLResponseInfo{OperationType: ast.OperationTypeQuery},
			Fetches: Sequence(
				SingleWithPath(&SingleFetch{
					FetchConfiguration: FetchConfiguration{
						DataSource:     rootDS,
						PostProcessing: PostProcessingConfiguration{SelectResponseDataPath: []string{"data"}},
					},
					InputTemplate:        InputTemplate{Segments: []TemplateSegment{{Data: []byte(`{}`), SegmentType: StaticSegmentType}}},
					DataSourceIdentifier: []byte("graphql_datasource.Source"),
				}, "query"),
				SingleWithPath(&BatchEntityFetch{
					Input: BatchInput{
						Header: InputTemplate{Segments: []TemplateSegment{{Data: []byte(`{"representations":[`), SegmentType: StaticSegmentType}}},
						Items:  []InputTemplate{{Segments: []TemplateSegment{{SegmentType: VariableSegmentType, VariableKind: ResolvableObjectVariableKind, Renderer: NewGraphQLVariableResolveRenderer(&Object{Fields: []*Field{{Name: []byte("__typename"), Value: &String{Path: []string{"__typename"}}}, {Name: []byte("id"), Value: &String{Path: []string{"id"}}}}})}}}},
						Footer: InputTemplate{Segments: []TemplateSegment{{Data: []byte(`]}`), SegmentType: StaticSegmentType}}},
					},
					DataSource:     entityDS,
					PostProcessing: PostProcessingConfiguration{SelectResponseDataPath: []string{"data", "_entities"}},
					Info:           &FetchInfo{DataSourceName: "products", OperationType: ast.OperationTypeQuery, ProvidesData: providesData},
					Caching:        FetchCacheConfiguration{Enabled: true, CacheName: "default", CacheKeyTemplate: productCacheKeyTemplate, UseL1Cache: true},
				}, "query.product", ObjectPath("product")),
			),
			Data: &Object{Fields: []*Field{{Name: []byte("product"), Value: &Object{Path: []string{"product"}, Fields: []*Field{
				{Name: []byte("id"), Value: &String{Path: []string{"id"}}},
				{Name: []byte("name"), Value: &String{Path: []string{"name"}}},
			}}}}},
		}

		ctx := NewContext(context.Background())
		ctx.ExecutionOptions.DisableSubgraphRequestDeduplication = true
		ctx.ExecutionOptions.Caching.EnableL1Cache = true
		ctx.ExecutionOptions.Caching.EnableL2Cache = true

		loader := &Loader{caches: map[string]LoaderCache{"default": cache}}

		ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
		resolvable := NewResolvable(ar, ResolvableOptions{})
		err := resolvable.Init(ctx, nil, ast.OperationTypeQuery)
		require.NoError(t, err)

		err = loader.LoadGraphQLResponseData(ctx, response, resolvable)
		require.NoError(t, err)

		out := fastjsonext.PrintGraphQLResponse(resolvable.data, resolvable.errors)
		assert.Equal(t, `{"data":{"product":{"__typename":"Product","id":"prod-1","name":"L2 Cached Product"}}}`, out)

		log := cache.GetLog()
		wantLog := []CacheLogEntry{
			// _entities(Product) — L1 miss (empty), L2 hit from pre-populated cache; no DS call needed
			{Operation: "get", Keys: []string{`{"__typename":"Product","key":{"id":"prod-1"}}`}, Hits: []bool{true}},
		}
		assert.Equal(t, wantLog, log, "L2 hit: single get operation with hit")
	})

	t.Run("L1+L2 - cross-request: L1 isolated, L2 shared", func(t *testing.T) {
		// L1 is per-request, L2 is shared across requests
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		cache := NewFakeLoaderCache()

		// Root DS for request 1
		rootDS1 := NewMockDataSource(ctrl)
		rootDS1.EXPECT().
			Load(gomock.Any(), gomock.Any(), gomock.Any()).
			Return([]byte(`{"data":{"product":{"__typename":"Product","id":"prod-1"}}}`), nil).Times(1)

		// Request 1: Cache miss, fetches from DS
		entityDS1 := NewMockDataSource(ctrl)
		entityDS1.EXPECT().
			Load(gomock.Any(), gomock.Any(), gomock.Any()).
			Return([]byte(`{"data":{"_entities":[{"__typename":"Product","id":"prod-1","name":"Product One"}]}}`), nil).Times(1)

		// Root DS for request 2
		rootDS2 := NewMockDataSource(ctrl)
		rootDS2.EXPECT().
			Load(gomock.Any(), gomock.Any(), gomock.Any()).
			Return([]byte(`{"data":{"product":{"__typename":"Product","id":"prod-1"}}}`), nil).Times(1)

		// Request 2: L2 hit (L1 is fresh/empty for new request)
		entityDS2 := NewMockDataSource(ctrl)
		entityDS2.EXPECT().
			Load(gomock.Any(), gomock.Any(), gomock.Any()).
			Times(0) // L2 hit

		productCacheKeyTemplate := &EntityQueryCacheKeyTemplate{
			Keys: NewResolvableObjectVariable(&Object{
				Fields: []*Field{
					{Name: []byte("__typename"), Value: &String{Path: []string{"__typename"}}},
					{Name: []byte("id"), Value: &String{Path: []string{"id"}}},
				},
			}),
		}

		providesData := &Object{Fields: []*Field{
			{Name: []byte("id"), Value: &Scalar{Path: []string{"id"}, Nullable: false}},
			{Name: []byte("name"), Value: &Scalar{Path: []string{"name"}, Nullable: false}},
		}}

		createResponse := func(rootDS, entityDS DataSource) *GraphQLResponse {
			return &GraphQLResponse{
				Info: &GraphQLResponseInfo{OperationType: ast.OperationTypeQuery},
				Fetches: Sequence(
					SingleWithPath(&SingleFetch{
						FetchConfiguration: FetchConfiguration{
							DataSource:     rootDS,
							PostProcessing: PostProcessingConfiguration{SelectResponseDataPath: []string{"data"}},
						},
						InputTemplate:        InputTemplate{Segments: []TemplateSegment{{Data: []byte(`{}`), SegmentType: StaticSegmentType}}},
						DataSourceIdentifier: []byte("graphql_datasource.Source"),
					}, "query"),
					SingleWithPath(&BatchEntityFetch{
						Input: BatchInput{
							Header: InputTemplate{Segments: []TemplateSegment{{Data: []byte(`{"representations":[`), SegmentType: StaticSegmentType}}},
							Items:  []InputTemplate{{Segments: []TemplateSegment{{SegmentType: VariableSegmentType, VariableKind: ResolvableObjectVariableKind, Renderer: NewGraphQLVariableResolveRenderer(&Object{Fields: []*Field{{Name: []byte("__typename"), Value: &String{Path: []string{"__typename"}}}, {Name: []byte("id"), Value: &String{Path: []string{"id"}}}}})}}}},
							Footer: InputTemplate{Segments: []TemplateSegment{{Data: []byte(`]}`), SegmentType: StaticSegmentType}}},
						},
						DataSource:     entityDS,
						PostProcessing: PostProcessingConfiguration{SelectResponseDataPath: []string{"data", "_entities"}},
						Info:           &FetchInfo{DataSourceName: "products", OperationType: ast.OperationTypeQuery, ProvidesData: providesData},
						Caching:        FetchCacheConfiguration{Enabled: true, CacheName: "default", CacheKeyTemplate: productCacheKeyTemplate, TTL: time.Minute, UseL1Cache: true},
					}, "query.product", ObjectPath("product")),
				),
				Data: &Object{Fields: []*Field{{Name: []byte("product"), Value: &Object{Path: []string{"product"}, Fields: []*Field{
					{Name: []byte("id"), Value: &String{Path: []string{"id"}}},
					{Name: []byte("name"), Value: &String{Path: []string{"name"}}},
				}}}}},
			}
		}

		// Request 1
		ctx1 := NewContext(context.Background())
		ctx1.ExecutionOptions.DisableSubgraphRequestDeduplication = true
		ctx1.ExecutionOptions.Caching.EnableL1Cache = true
		ctx1.ExecutionOptions.Caching.EnableL2Cache = true

		loader1 := &Loader{caches: map[string]LoaderCache{"default": cache}}

		ar1 := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
		resolvable1 := NewResolvable(ar1, ResolvableOptions{})
		err := resolvable1.Init(ctx1, nil, ast.OperationTypeQuery)
		require.NoError(t, err)

		err = loader1.LoadGraphQLResponseData(ctx1, createResponse(rootDS1, entityDS1), resolvable1)
		require.NoError(t, err)

		// Request 2 (new context = new L1, but same L2)
		cache.ClearLog()
		ctx2 := NewContext(context.Background())
		ctx2.ExecutionOptions.DisableSubgraphRequestDeduplication = true
		ctx2.ExecutionOptions.Caching.EnableL1Cache = true
		ctx2.ExecutionOptions.Caching.EnableL2Cache = true

		loader2 := &Loader{caches: map[string]LoaderCache{"default": cache}}

		ar2 := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
		resolvable2 := NewResolvable(ar2, ResolvableOptions{})
		err = resolvable2.Init(ctx2, nil, ast.OperationTypeQuery)
		require.NoError(t, err)

		err = loader2.LoadGraphQLResponseData(ctx2, createResponse(rootDS2, entityDS2), resolvable2)
		require.NoError(t, err)

		// Request 2 uses a new Loader (new L1) but same L2 cache instance
		log := cache.GetLog()
		wantLog := []CacheLogEntry{
			// _entities(Product) — L1 miss (new request, empty L1), L2 hit from request 1; no DS call
			{Operation: "get", Keys: []string{`{"__typename":"Product","key":{"id":"prod-1"}}`}, Hits: []bool{true}},
		}
		assert.Equal(t, wantLog, log, "Request 2: L2 hit (L1 is fresh/empty)")
	})

	t.Run("Both disabled - no cache operations", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		cache := NewFakeLoaderCache()

		rootDS := NewMockDataSource(ctrl)
		rootDS.EXPECT().
			Load(gomock.Any(), gomock.Any(), gomock.Any()).
			Return([]byte(`{"data":{"product":{"__typename":"Product","id":"prod-1"}}}`), nil).Times(1)

		entityDS := NewMockDataSource(ctrl)
		entityDS.EXPECT().
			Load(gomock.Any(), gomock.Any(), gomock.Any()).
			Return([]byte(`{"data":{"_entities":[{"__typename":"Product","id":"prod-1","name":"Product"}]}}`), nil).Times(1)

		productCacheKeyTemplate := &EntityQueryCacheKeyTemplate{
			Keys: NewResolvableObjectVariable(&Object{
				Fields: []*Field{
					{Name: []byte("__typename"), Value: &String{Path: []string{"__typename"}}},
					{Name: []byte("id"), Value: &String{Path: []string{"id"}}},
				},
			}),
		}

		providesData := &Object{Fields: []*Field{
			{Name: []byte("id"), Value: &Scalar{Path: []string{"id"}, Nullable: false}},
			{Name: []byte("name"), Value: &Scalar{Path: []string{"name"}, Nullable: false}},
		}}

		response := &GraphQLResponse{
			Info: &GraphQLResponseInfo{OperationType: ast.OperationTypeQuery},
			Fetches: Sequence(
				SingleWithPath(&SingleFetch{
					FetchConfiguration: FetchConfiguration{
						DataSource:     rootDS,
						PostProcessing: PostProcessingConfiguration{SelectResponseDataPath: []string{"data"}},
					},
					InputTemplate:        InputTemplate{Segments: []TemplateSegment{{Data: []byte(`{}`), SegmentType: StaticSegmentType}}},
					DataSourceIdentifier: []byte("graphql_datasource.Source"),
				}, "query"),
				SingleWithPath(&BatchEntityFetch{
					Input: BatchInput{
						Header: InputTemplate{Segments: []TemplateSegment{{Data: []byte(`{"representations":[`), SegmentType: StaticSegmentType}}},
						Items:  []InputTemplate{{Segments: []TemplateSegment{{SegmentType: VariableSegmentType, VariableKind: ResolvableObjectVariableKind, Renderer: NewGraphQLVariableResolveRenderer(&Object{Fields: []*Field{{Name: []byte("__typename"), Value: &String{Path: []string{"__typename"}}}, {Name: []byte("id"), Value: &String{Path: []string{"id"}}}}})}}}},
						Footer: InputTemplate{Segments: []TemplateSegment{{Data: []byte(`]}`), SegmentType: StaticSegmentType}}},
					},
					DataSource:     entityDS,
					PostProcessing: PostProcessingConfiguration{SelectResponseDataPath: []string{"data", "_entities"}},
					Info:           &FetchInfo{DataSourceName: "products", OperationType: ast.OperationTypeQuery, ProvidesData: providesData},
					Caching:        FetchCacheConfiguration{Enabled: true, CacheName: "default", CacheKeyTemplate: productCacheKeyTemplate, UseL1Cache: true},
				}, "query.product", ObjectPath("product")),
			),
			Data: &Object{Fields: []*Field{{Name: []byte("product"), Value: &Object{Path: []string{"product"}, Fields: []*Field{
				{Name: []byte("id"), Value: &String{Path: []string{"id"}}},
				{Name: []byte("name"), Value: &String{Path: []string{"name"}}},
			}}}}},
		}

		ctx := NewContext(context.Background())
		ctx.ExecutionOptions.DisableSubgraphRequestDeduplication = true
		ctx.ExecutionOptions.Caching.EnableL1Cache = false
		ctx.ExecutionOptions.Caching.EnableL2Cache = false

		loader := &Loader{caches: map[string]LoaderCache{"default": cache}}

		ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
		resolvable := NewResolvable(ar, ResolvableOptions{})
		err := resolvable.Init(ctx, nil, ast.OperationTypeQuery)
		require.NoError(t, err)

		err = loader.LoadGraphQLResponseData(ctx, response, resolvable)
		require.NoError(t, err)

		// Verify no cache operations
		log := cache.GetLog()
		assert.Empty(t, log, "No cache operations should occur when both L1 and L2 are disabled")
	})
}

// TestL1CacheSkipsParallelFetch verifies that parallel fetches are skipped when L1 cache has complete hits.
// This tests the optimization at loader.go:296 where goroutines are not spawned for parallel fetch nodes
// that have all entities already in L1 cache from a previous sequential fetch.
func TestL1CacheSkipsParallelFetch(t *testing.T) {
	t.Run("parallel fetches skipped on L1 hit from previous fetch", func(t *testing.T) {
		// This test sets up a sequence where:
		// 1. Root fetch returns products
		// 2. First entity fetch runs and populates L1 cache with all needed data
		// 3. Parallel group runs - the fetch for same entities should be SKIPPED (L1 hit)
		//
		// The key behavior being tested: when L1 cache has a complete hit for all entities
		// in a parallel fetch node, the goroutine is not spawned (line 295-296 in loader.go)
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		rootDS := NewMockDataSource(ctrl)
		rootDS.EXPECT().
			Load(gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, headers any, input []byte) ([]byte, error) {
				return []byte(`{"data":{"products":[{"__typename":"Product","id":"prod-1"},{"__typename":"Product","id":"prod-2"}]}}`), nil
			}).Times(1)

		// First entity fetch (sequential) - populates L1 with all fields including price
		entityDS1 := NewMockDataSource(ctrl)
		entityDS1.EXPECT().
			Load(gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, headers any, input []byte) ([]byte, error) {
				return []byte(`{"data":{"_entities":[{"__typename":"Product","id":"prod-1","name":"Product One","price":99.99},{"__typename":"Product","id":"prod-2","name":"Product Two","price":49.99}]}}`), nil
			}).Times(1)

		// Second entity fetch (in parallel group) - should NOT be called (L1 hit from entityDS1)
		entityDS2 := NewMockDataSource(ctrl)
		entityDS2.EXPECT().
			Load(gomock.Any(), gomock.Any(), gomock.Any()).
			Times(0) // L1 cache hit should skip this fetch entirely - THIS IS THE KEY ASSERTION

		productCacheKeyTemplate := &EntityQueryCacheKeyTemplate{
			Keys: NewResolvableObjectVariable(&Object{
				Fields: []*Field{
					{Name: []byte("__typename"), Value: &String{Path: []string{"__typename"}}},
					{Name: []byte("id"), Value: &String{Path: []string{"id"}}},
				},
			}),
		}

		// First fetch provides both name AND price so L1 can satisfy second fetch
		providesDataFull := &Object{
			Fields: []*Field{
				{Name: []byte("id"), Value: &Scalar{Path: []string{"id"}, Nullable: false}},
				{Name: []byte("name"), Value: &Scalar{Path: []string{"name"}, Nullable: false}},
				{Name: []byte("price"), Value: &Scalar{Path: []string{"price"}, Nullable: false}},
			},
		}

		// Second fetch only needs price (subset of what first fetch provides)
		providesDataPrice := &Object{
			Fields: []*Field{
				{Name: []byte("id"), Value: &Scalar{Path: []string{"id"}, Nullable: false}},
				{Name: []byte("price"), Value: &Scalar{Path: []string{"price"}, Nullable: false}},
			},
		}

		response := &GraphQLResponse{
			Info: &GraphQLResponseInfo{OperationType: ast.OperationTypeQuery},
			Fetches: Sequence(
				// Root fetch - get products
				SingleWithPath(&SingleFetch{
					FetchConfiguration: FetchConfiguration{
						DataSource:     rootDS,
						PostProcessing: PostProcessingConfiguration{SelectResponseDataPath: []string{"data"}},
					},
					InputTemplate: InputTemplate{
						Segments: []TemplateSegment{{Data: []byte(`{"method":"POST","url":"http://products"}`), SegmentType: StaticSegmentType}},
					},
					DataSourceIdentifier: []byte("graphql_datasource.Source"),
				}, "query"),
				// First entity fetch - populates L1 with product entities (includes price)
				SingleWithPath(&BatchEntityFetch{
					Input: BatchInput{
						Header: InputTemplate{Segments: []TemplateSegment{{Data: []byte(`{"method":"POST","url":"http://products","body":{"query":"names","variables":{"representations":[`), SegmentType: StaticSegmentType}}},
						Items:  []InputTemplate{{Segments: []TemplateSegment{{SegmentType: VariableSegmentType, VariableKind: ResolvableObjectVariableKind, Renderer: NewGraphQLVariableResolveRenderer(&Object{Fields: []*Field{{Name: []byte("__typename"), Value: &String{Path: []string{"__typename"}}}, {Name: []byte("id"), Value: &String{Path: []string{"id"}}}}})}}}},
						Footer: InputTemplate{Segments: []TemplateSegment{{Data: []byte(`]}}}`), SegmentType: StaticSegmentType}}},
					},
					DataSource:     entityDS1,
					PostProcessing: PostProcessingConfiguration{SelectResponseDataPath: []string{"data", "_entities"}},
					Info: &FetchInfo{
						DataSourceName: "products-names",
						OperationType:  ast.OperationTypeQuery,
						ProvidesData:   providesDataFull,
					},
					Caching: FetchCacheConfiguration{
						Enabled:          true,
						CacheName:        "default",
						CacheKeyTemplate: productCacheKeyTemplate,
						UseL1Cache:       true,
					},
				}, "query.products", ArrayPath("products")),
				// Parallel group with single fetch - should skip because L1 has all data
				Parallel(
					SingleWithPath(&BatchEntityFetch{
						Input: BatchInput{
							Header: InputTemplate{Segments: []TemplateSegment{{Data: []byte(`{"method":"POST","url":"http://pricing","body":{"query":"prices","variables":{"representations":[`), SegmentType: StaticSegmentType}}},
							Items:  []InputTemplate{{Segments: []TemplateSegment{{SegmentType: VariableSegmentType, VariableKind: ResolvableObjectVariableKind, Renderer: NewGraphQLVariableResolveRenderer(&Object{Fields: []*Field{{Name: []byte("__typename"), Value: &String{Path: []string{"__typename"}}}, {Name: []byte("id"), Value: &String{Path: []string{"id"}}}}})}}}},
							Footer: InputTemplate{Segments: []TemplateSegment{{Data: []byte(`]}}}`), SegmentType: StaticSegmentType}}},
						},
						DataSource:     entityDS2,
						PostProcessing: PostProcessingConfiguration{SelectResponseDataPath: []string{"data", "_entities"}},
						Info: &FetchInfo{
							DataSourceName: "pricing",
							OperationType:  ast.OperationTypeQuery,
							ProvidesData:   providesDataPrice,
						},
						Caching: FetchCacheConfiguration{
							Enabled:          true,
							CacheName:        "default",
							CacheKeyTemplate: productCacheKeyTemplate,
							UseL1Cache:       true,
						},
					}, "query.products", ArrayPath("products")),
				),
			),
			Data: &Object{
				Fields: []*Field{
					{
						Name: []byte("products"),
						Value: &Array{
							Path: []string{"products"},
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

		loader := &Loader{}

		ctx := NewContext(context.Background())
		ctx.ExecutionOptions.DisableSubgraphRequestDeduplication = true
		ctx.ExecutionOptions.Caching.EnableL1Cache = true
		ctx.ExecutionOptions.Caching.EnableL2Cache = false // L1 only for this test
		ctx.ExecutionOptions.Caching.EnableCacheAnalytics = true

		ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
		resolvable := NewResolvable(ar, ResolvableOptions{})
		err := resolvable.Init(ctx, nil, ast.OperationTypeQuery)
		require.NoError(t, err)

		err = loader.LoadGraphQLResponseData(ctx, response, resolvable)
		require.NoError(t, err)

		out := fastjsonext.PrintGraphQLResponse(resolvable.data, resolvable.errors)
		// Output includes all data from L1 cache (merged from first fetch)
		// __typename is included because the entity data from L1 cache includes it
		assert.Equal(t, `{"data":{"products":[{"__typename":"Product","id":"prod-1","name":"Product One","price":99.99},{"__typename":"Product","id":"prod-2","name":"Product Two","price":49.99}]}}`, out)

		// Verify L1 stats:
		// - 2 misses from first entity fetch (sequential, populates L1)
		// - 2 hits from second entity fetch in parallel (same products, skipped via L1)
		stats := ctx.GetCacheStats()
		var l1Hits, l1Misses int
		for _, ev := range stats.L1Reads {
			if ev.Kind == CacheKeyHit {
				l1Hits++
			} else {
				l1Misses++
			}
		}
		assert.Equal(t, 2, l1Hits, "L1 should have 2 hits (parallel fetch for same entities skipped)")
		assert.Equal(t, 2, l1Misses, "L1 should have 2 misses (first entity fetch)")
	})

}

func TestL1CacheFieldAccumulation(t *testing.T) {
	t.Run("fields from fetch 1 survive fetch 2 merge and are available for fetch 3", func(t *testing.T) {
		// Scenario: 3 sequential entity fetches for the same entity (User:1),
		// each with different ProvidesData (different field sets).
		//
		// Fetch 1: ProvidesData = {name}
		//   → L1 MISS, calls subgraph, stores {__typename, id, name} in L1
		//
		// Fetch 2: ProvidesData = {email}
		//   → L1 HIT but widening check fails (cached value lacks "email")
		//   → Calls subgraph, gets {__typename, id, email}
		//   → Merges into L1: {__typename, id, name, email}
		//
		// Fetch 3: ProvidesData = {name}
		//   → L1 HIT, widening check passes ("name" is in L1 from fetch 1)
		//   → Skips subgraph call
		//
		// This proves:
		// 1. L1 passthrough write preserves all fields (including @key "id")
		// 2. L1 merge accumulates fields across fetches
		// 3. Fetch 1's "name" survives fetch 2's merge and is available for fetch 3
		// 4. Fetch 3 consumes a field that fetch 2 did NOT provide

		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		rootDS := NewMockDataSource(ctrl)
		rootDS.EXPECT().
			Load(gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, headers any, input []byte) ([]byte, error) {
				return []byte(`{"data":{"user":{"__typename":"User","id":"1"}}}`), nil
			}).Times(1)

		// Fetch 1: returns name only
		entityDS1 := NewMockDataSource(ctrl)
		entityDS1.EXPECT().
			Load(gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, headers any, input []byte) ([]byte, error) {
				return []byte(`{"data":{"_entities":[{"__typename":"User","id":"1","name":"Alice"}]}}`), nil
			}).Times(1)

		// Fetch 2: returns email only (NOT name — fetch 2's subgraph doesn't provide name)
		entityDS2 := NewMockDataSource(ctrl)
		entityDS2.EXPECT().
			Load(gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, headers any, input []byte) ([]byte, error) {
				return []byte(`{"data":{"_entities":[{"__typename":"User","id":"1","email":"alice@example.com"}]}}`), nil
			}).Times(1)

		// Fetch 3: should NOT be called — "name" is in L1 from fetch 1
		entityDS3 := NewMockDataSource(ctrl)
		entityDS3.EXPECT().
			Load(gomock.Any(), gomock.Any(), gomock.Any()).
			Times(0)

		userCacheKeyTemplate := &EntityQueryCacheKeyTemplate{
			Keys: NewResolvableObjectVariable(&Object{
				Fields: []*Field{
					{Name: []byte("__typename"), Value: &String{Path: []string{"__typename"}}},
					{Name: []byte("id"), Value: &String{Path: []string{"id"}}},
				},
			}),
		}

		providesData1 := &Object{
			Fields: []*Field{
				{Name: []byte("name"), Value: &Scalar{}},
			},
		}
		ComputeHasAliases(providesData1)

		providesData2 := &Object{
			Fields: []*Field{
				{Name: []byte("email"), Value: &Scalar{}},
			},
		}
		ComputeHasAliases(providesData2)

		// Fetch 3 wants "name" — a field from fetch 1, NOT from fetch 2.
		providesData3 := &Object{
			Fields: []*Field{
				{Name: []byte("name"), Value: &Scalar{}},
			},
		}
		ComputeHasAliases(providesData3)

		entityInput := BatchInput{
			Header: InputTemplate{Segments: []TemplateSegment{{Data: []byte(`{"method":"POST","url":"http://users","body":{"query":"q","variables":{"representations":[`), SegmentType: StaticSegmentType}}},
			Items: []InputTemplate{{Segments: []TemplateSegment{{
				SegmentType:  VariableSegmentType,
				VariableKind: ResolvableObjectVariableKind,
				Renderer: NewGraphQLVariableResolveRenderer(&Object{Fields: []*Field{
					{Name: []byte("__typename"), Value: &String{Path: []string{"__typename"}}},
					{Name: []byte("id"), Value: &String{Path: []string{"id"}}},
				}}),
			}}}},
			Footer: InputTemplate{Segments: []TemplateSegment{{Data: []byte(`]}}}`), SegmentType: StaticSegmentType}}},
		}

		response := &GraphQLResponse{
			Info: &GraphQLResponseInfo{OperationType: ast.OperationTypeQuery},
			Fetches: Sequence(
				SingleWithPath(&SingleFetch{
					FetchConfiguration: FetchConfiguration{
						DataSource:     rootDS,
						PostProcessing: PostProcessingConfiguration{SelectResponseDataPath: []string{"data"}},
					},
					InputTemplate: InputTemplate{
						Segments: []TemplateSegment{{Data: []byte(`{"method":"POST","url":"http://users"}`), SegmentType: StaticSegmentType}},
					},
					DataSourceIdentifier: []byte("graphql_datasource.Source"),
				}, "query"),
				SingleWithPath(&BatchEntityFetch{
					Input:          entityInput,
					DataSource:     entityDS1,
					PostProcessing: PostProcessingConfiguration{SelectResponseDataPath: []string{"data", "_entities"}},
					Info: &FetchInfo{
						DataSourceName: "users",
						OperationType:  ast.OperationTypeQuery,
						ProvidesData:   providesData1,
					},
					Caching: FetchCacheConfiguration{
						Enabled:          true,
						CacheName:        "default",
						CacheKeyTemplate: userCacheKeyTemplate,
						UseL1Cache:       true,
					},
				}, "query.user", ObjectPath("user")),
				SingleWithPath(&BatchEntityFetch{
					Input:          entityInput,
					DataSource:     entityDS2,
					PostProcessing: PostProcessingConfiguration{SelectResponseDataPath: []string{"data", "_entities"}},
					Info: &FetchInfo{
						DataSourceName: "users",
						OperationType:  ast.OperationTypeQuery,
						ProvidesData:   providesData2,
					},
					Caching: FetchCacheConfiguration{
						Enabled:          true,
						CacheName:        "default",
						CacheKeyTemplate: userCacheKeyTemplate,
						UseL1Cache:       true,
					},
				}, "query.user", ObjectPath("user")),
				SingleWithPath(&BatchEntityFetch{
					Input:          entityInput,
					DataSource:     entityDS3,
					PostProcessing: PostProcessingConfiguration{SelectResponseDataPath: []string{"data", "_entities"}},
					Info: &FetchInfo{
						DataSourceName: "users",
						OperationType:  ast.OperationTypeQuery,
						ProvidesData:   providesData3,
					},
					Caching: FetchCacheConfiguration{
						Enabled:          true,
						CacheName:        "default",
						CacheKeyTemplate: userCacheKeyTemplate,
						UseL1Cache:       true,
					},
				}, "query.user", ObjectPath("user")),
			),
			Data: &Object{
				Fields: []*Field{
					{
						Name: []byte("user"),
						Value: &Object{
							Path: []string{"user"},
							Fields: []*Field{
								{Name: []byte("name"), Value: &String{Path: []string{"name"}}},
								{Name: []byte("email"), Value: &String{Path: []string{"email"}}},
							},
						},
					},
				},
			},
		}

		ar := arena.NewMonotonicArena(arena.WithMinBufferSize(4096))
		ctx := NewContext(context.Background())
		ctx.ExecutionOptions.Caching.EnableL1Cache = true
		ctx.ExecutionOptions.Caching.EnableCacheAnalytics = true
		ctx.ExecutionOptions.DisableSubgraphRequestDeduplication = true

		resolvable := NewResolvable(ar, ResolvableOptions{})
		err := resolvable.Init(ctx, nil, ast.OperationTypeQuery)
		require.NoError(t, err)

		loader := &Loader{
			ctx:        ctx,
			jsonArena:  ar,
			l1Cache:    map[string]*astjson.Value{},
			resolvable: resolvable,
			caches:     map[string]LoaderCache{"default": NewFakeLoaderCache()},
		}

		err = loader.LoadGraphQLResponseData(ctx, response, resolvable)
		require.NoError(t, err)

		out := fastjsonext.PrintGraphQLResponse(resolvable.data, resolvable.errors)
		// Extra fields (__typename, id) from L1 passthrough are present in the
		// merged data tree but harmless — the render walk only outputs fields
		// listed in the response plan.
		assert.Equal(t, `{"data":{"user":{"__typename":"User","id":"1","name":"Alice","email":"alice@example.com"}}}`, string(out))

		stats := ctx.GetCacheStats()
		// Fetch 1: L1 miss → subgraph call → stores {name, id, __typename}
		// Fetch 2: L1 hit but widening fails (no email) → subgraph call → merges email into L1
		// Fetch 3: L1 hit, widening passes (name present from fetch 1) → no subgraph call
		var l1Hits, l1Misses int
		for _, ev := range stats.L1Reads {
			if ev.Kind == CacheKeyHit {
				l1Hits++
			} else {
				l1Misses++
			}
		}
		assert.Equal(t, 1, l1Hits, "Fetch 3 should hit L1 (name from fetch 1 survived fetch 2's merge)")
		assert.Equal(t, 1, l1Misses, "Fetch 1 should miss L1 (cache empty)")

		// Verify the L1 cache entry contains ALL accumulated fields.
		const cacheKey = `{"__typename":"User","key":{"id":"1"}}`
		cached, ok := loader.l1Cache[cacheKey]
		require.True(t, ok, "L1 should have User:1 entry")
		cachedJSON := string(cached.MarshalTo(nil))
		assert.Equal(t, `{"__typename":"User","id":"1","name":"Alice","email":"alice@example.com"}`, cachedJSON,
			"L1 entry must contain name (fetch 1), email (fetch 2 merge), and key fields (id, __typename) via passthrough")
	})

	t.Run("different aliases for same field across fetches", func(t *testing.T) {
		// Fetch 1: ProvidesData = {nickname: name} (alias "nickname" for field "name")
		//   → L1 MISS, calls subgraph, stores {__typename, id, name} in L1 (normalized)
		//
		// Fetch 2: ProvidesData = {email}
		//   → L1 widening miss (no email), calls subgraph
		//
		// Fetch 3: ProvidesData = {displayName: name} (different alias for same field)
		//   → L1 HIT: L1 stores schema-name "name", denormalize maps it to "displayName"

		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		rootDS := NewMockDataSource(ctrl)
		rootDS.EXPECT().
			Load(gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, headers any, input []byte) ([]byte, error) {
				return []byte(`{"data":{"user":{"__typename":"User","id":"1"}}}`), nil
			}).Times(1)

		entityDS1 := NewMockDataSource(ctrl)
		entityDS1.EXPECT().
			Load(gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, headers any, input []byte) ([]byte, error) {
				// Subgraph returns schema field name "name", response has alias "nickname"
				return []byte(`{"data":{"_entities":[{"__typename":"User","id":"1","nickname":"Alice"}]}}`), nil
			}).Times(1)

		entityDS2 := NewMockDataSource(ctrl)
		entityDS2.EXPECT().
			Load(gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, headers any, input []byte) ([]byte, error) {
				return []byte(`{"data":{"_entities":[{"__typename":"User","id":"1","email":"alice@example.com"}]}}`), nil
			}).Times(1)

		// Fetch 3 should NOT call subgraph — "name" is in L1 from fetch 1
		entityDS3 := NewMockDataSource(ctrl)
		entityDS3.EXPECT().
			Load(gomock.Any(), gomock.Any(), gomock.Any()).
			Times(0)

		userCacheKeyTemplate := &EntityQueryCacheKeyTemplate{
			Keys: NewResolvableObjectVariable(&Object{
				Fields: []*Field{
					{Name: []byte("__typename"), Value: &String{Path: []string{"__typename"}}},
					{Name: []byte("id"), Value: &String{Path: []string{"id"}}},
				},
			}),
		}

		// Fetch 1: alias "nickname" → schema "name"
		providesData1 := &Object{
			HasAliases: true,
			Fields: []*Field{
				{Name: []byte("nickname"), OriginalName: []byte("name"), Value: &Scalar{}},
			},
		}

		providesData2 := &Object{
			Fields: []*Field{
				{Name: []byte("email"), Value: &Scalar{}},
			},
		}
		ComputeHasAliases(providesData2)

		// Fetch 3: alias "displayName" → schema "name"
		providesData3 := &Object{
			HasAliases: true,
			Fields: []*Field{
				{Name: []byte("displayName"), OriginalName: []byte("name"), Value: &Scalar{}},
			},
		}

		entityInput := BatchInput{
			Header: InputTemplate{Segments: []TemplateSegment{{Data: []byte(`{"method":"POST","url":"http://users","body":{"query":"q","variables":{"representations":[`), SegmentType: StaticSegmentType}}},
			Items: []InputTemplate{{Segments: []TemplateSegment{{
				SegmentType:  VariableSegmentType,
				VariableKind: ResolvableObjectVariableKind,
				Renderer: NewGraphQLVariableResolveRenderer(&Object{Fields: []*Field{
					{Name: []byte("__typename"), Value: &String{Path: []string{"__typename"}}},
					{Name: []byte("id"), Value: &String{Path: []string{"id"}}},
				}}),
			}}}},
			Footer: InputTemplate{Segments: []TemplateSegment{{Data: []byte(`]}}}`), SegmentType: StaticSegmentType}}},
		}

		response := &GraphQLResponse{
			Info: &GraphQLResponseInfo{OperationType: ast.OperationTypeQuery},
			Fetches: Sequence(
				SingleWithPath(&SingleFetch{
					FetchConfiguration: FetchConfiguration{
						DataSource:     rootDS,
						PostProcessing: PostProcessingConfiguration{SelectResponseDataPath: []string{"data"}},
					},
					InputTemplate: InputTemplate{
						Segments: []TemplateSegment{{Data: []byte(`{"method":"POST","url":"http://users"}`), SegmentType: StaticSegmentType}},
					},
					DataSourceIdentifier: []byte("graphql_datasource.Source"),
				}, "query"),
				SingleWithPath(&BatchEntityFetch{
					Input:          entityInput,
					DataSource:     entityDS1,
					PostProcessing: PostProcessingConfiguration{SelectResponseDataPath: []string{"data", "_entities"}},
					Info:           &FetchInfo{DataSourceName: "users", OperationType: ast.OperationTypeQuery, ProvidesData: providesData1},
					Caching:        FetchCacheConfiguration{Enabled: true, CacheName: "default", CacheKeyTemplate: userCacheKeyTemplate, UseL1Cache: true},
				}, "query.user", ObjectPath("user")),
				SingleWithPath(&BatchEntityFetch{
					Input:          entityInput,
					DataSource:     entityDS2,
					PostProcessing: PostProcessingConfiguration{SelectResponseDataPath: []string{"data", "_entities"}},
					Info:           &FetchInfo{DataSourceName: "users", OperationType: ast.OperationTypeQuery, ProvidesData: providesData2},
					Caching:        FetchCacheConfiguration{Enabled: true, CacheName: "default", CacheKeyTemplate: userCacheKeyTemplate, UseL1Cache: true},
				}, "query.user", ObjectPath("user")),
				SingleWithPath(&BatchEntityFetch{
					Input:          entityInput,
					DataSource:     entityDS3,
					PostProcessing: PostProcessingConfiguration{SelectResponseDataPath: []string{"data", "_entities"}},
					Info:           &FetchInfo{DataSourceName: "users", OperationType: ast.OperationTypeQuery, ProvidesData: providesData3},
					Caching:        FetchCacheConfiguration{Enabled: true, CacheName: "default", CacheKeyTemplate: userCacheKeyTemplate, UseL1Cache: true},
				}, "query.user", ObjectPath("user")),
			),
			Data: &Object{
				Fields: []*Field{
					{Name: []byte("user"), Value: &Object{
						Path: []string{"user"},
						Fields: []*Field{
							{Name: []byte("displayName"), Value: &String{Path: []string{"displayName"}}},
							{Name: []byte("email"), Value: &String{Path: []string{"email"}}},
						},
					}},
				},
			},
		}

		ar := arena.NewMonotonicArena(arena.WithMinBufferSize(4096))
		ctx := NewContext(context.Background())
		ctx.ExecutionOptions.Caching.EnableL1Cache = true
		ctx.ExecutionOptions.Caching.EnableCacheAnalytics = true
		ctx.ExecutionOptions.DisableSubgraphRequestDeduplication = true

		resolvable := NewResolvable(ar, ResolvableOptions{})
		err := resolvable.Init(ctx, nil, ast.OperationTypeQuery)
		require.NoError(t, err)

		loader := &Loader{
			ctx:        ctx,
			jsonArena:  ar,
			l1Cache:    map[string]*astjson.Value{},
			resolvable: resolvable,
			caches:     map[string]LoaderCache{"default": NewFakeLoaderCache()},
		}

		err = loader.LoadGraphQLResponseData(ctx, response, resolvable)
		require.NoError(t, err)

		out := fastjsonext.PrintGraphQLResponse(resolvable.data, resolvable.errors)
		assert.Equal(t, `{"data":{"user":{"__typename":"User","id":"1","nickname":"Alice","email":"alice@example.com","displayName":"Alice"}}}`, string(out),
			"fetch 3 should get name via different alias (displayName)")

		stats := ctx.GetCacheStats()
		var l1Hits int
		for _, ev := range stats.L1Reads {
			if ev.Kind == CacheKeyHit {
				l1Hits++
			}
		}
		assert.Equal(t, 1, l1Hits, "Fetch 3 should hit L1 (schema name 'name' stored by fetch 1, denormalized to 'displayName')")
	})

	t.Run("alias then no alias for same field", func(t *testing.T) {
		// Fetch 1: ProvidesData = {nickname: name} (alias)
		//   → L1 MISS, stores normalized "name" in L1
		//
		// Fetch 2: ProvidesData = {email}
		//   → L1 widening miss
		//
		// Fetch 3: ProvidesData = {name} (no alias, schema name)
		//   → L1 HIT: "name" is in L1 from fetch 1's normalized write

		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		rootDS := NewMockDataSource(ctrl)
		rootDS.EXPECT().
			Load(gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, headers any, input []byte) ([]byte, error) {
				return []byte(`{"data":{"user":{"__typename":"User","id":"1"}}}`), nil
			}).Times(1)

		entityDS1 := NewMockDataSource(ctrl)
		entityDS1.EXPECT().
			Load(gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, headers any, input []byte) ([]byte, error) {
				return []byte(`{"data":{"_entities":[{"__typename":"User","id":"1","nickname":"Alice"}]}}`), nil
			}).Times(1)

		entityDS2 := NewMockDataSource(ctrl)
		entityDS2.EXPECT().
			Load(gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, headers any, input []byte) ([]byte, error) {
				return []byte(`{"data":{"_entities":[{"__typename":"User","id":"1","email":"alice@example.com"}]}}`), nil
			}).Times(1)

		entityDS3 := NewMockDataSource(ctrl)
		entityDS3.EXPECT().
			Load(gomock.Any(), gomock.Any(), gomock.Any()).
			Times(0) // L1 hit

		userCacheKeyTemplate := &EntityQueryCacheKeyTemplate{
			Keys: NewResolvableObjectVariable(&Object{
				Fields: []*Field{
					{Name: []byte("__typename"), Value: &String{Path: []string{"__typename"}}},
					{Name: []byte("id"), Value: &String{Path: []string{"id"}}},
				},
			}),
		}

		// Fetch 1: alias "nickname" → schema "name"
		providesData1 := &Object{
			HasAliases: true,
			Fields: []*Field{
				{Name: []byte("nickname"), OriginalName: []byte("name"), Value: &Scalar{}},
			},
		}

		providesData2 := &Object{
			Fields: []*Field{
				{Name: []byte("email"), Value: &Scalar{}},
			},
		}
		ComputeHasAliases(providesData2)

		// Fetch 3: no alias, uses schema name directly
		providesData3 := &Object{
			Fields: []*Field{
				{Name: []byte("name"), Value: &Scalar{}},
			},
		}
		ComputeHasAliases(providesData3)

		entityInput := BatchInput{
			Header: InputTemplate{Segments: []TemplateSegment{{Data: []byte(`{"method":"POST","url":"http://users","body":{"query":"q","variables":{"representations":[`), SegmentType: StaticSegmentType}}},
			Items: []InputTemplate{{Segments: []TemplateSegment{{
				SegmentType:  VariableSegmentType,
				VariableKind: ResolvableObjectVariableKind,
				Renderer: NewGraphQLVariableResolveRenderer(&Object{Fields: []*Field{
					{Name: []byte("__typename"), Value: &String{Path: []string{"__typename"}}},
					{Name: []byte("id"), Value: &String{Path: []string{"id"}}},
				}}),
			}}}},
			Footer: InputTemplate{Segments: []TemplateSegment{{Data: []byte(`]}}}`), SegmentType: StaticSegmentType}}},
		}

		response := &GraphQLResponse{
			Info: &GraphQLResponseInfo{OperationType: ast.OperationTypeQuery},
			Fetches: Sequence(
				SingleWithPath(&SingleFetch{
					FetchConfiguration: FetchConfiguration{
						DataSource:     rootDS,
						PostProcessing: PostProcessingConfiguration{SelectResponseDataPath: []string{"data"}},
					},
					InputTemplate: InputTemplate{
						Segments: []TemplateSegment{{Data: []byte(`{"method":"POST","url":"http://users"}`), SegmentType: StaticSegmentType}},
					},
					DataSourceIdentifier: []byte("graphql_datasource.Source"),
				}, "query"),
				SingleWithPath(&BatchEntityFetch{
					Input:          entityInput,
					DataSource:     entityDS1,
					PostProcessing: PostProcessingConfiguration{SelectResponseDataPath: []string{"data", "_entities"}},
					Info:           &FetchInfo{DataSourceName: "users", OperationType: ast.OperationTypeQuery, ProvidesData: providesData1},
					Caching:        FetchCacheConfiguration{Enabled: true, CacheName: "default", CacheKeyTemplate: userCacheKeyTemplate, UseL1Cache: true},
				}, "query.user", ObjectPath("user")),
				SingleWithPath(&BatchEntityFetch{
					Input:          entityInput,
					DataSource:     entityDS2,
					PostProcessing: PostProcessingConfiguration{SelectResponseDataPath: []string{"data", "_entities"}},
					Info:           &FetchInfo{DataSourceName: "users", OperationType: ast.OperationTypeQuery, ProvidesData: providesData2},
					Caching:        FetchCacheConfiguration{Enabled: true, CacheName: "default", CacheKeyTemplate: userCacheKeyTemplate, UseL1Cache: true},
				}, "query.user", ObjectPath("user")),
				SingleWithPath(&BatchEntityFetch{
					Input:          entityInput,
					DataSource:     entityDS3,
					PostProcessing: PostProcessingConfiguration{SelectResponseDataPath: []string{"data", "_entities"}},
					Info:           &FetchInfo{DataSourceName: "users", OperationType: ast.OperationTypeQuery, ProvidesData: providesData3},
					Caching:        FetchCacheConfiguration{Enabled: true, CacheName: "default", CacheKeyTemplate: userCacheKeyTemplate, UseL1Cache: true},
				}, "query.user", ObjectPath("user")),
			),
			Data: &Object{
				Fields: []*Field{
					{Name: []byte("user"), Value: &Object{
						Path: []string{"user"},
						Fields: []*Field{
							{Name: []byte("name"), Value: &String{Path: []string{"name"}}},
							{Name: []byte("email"), Value: &String{Path: []string{"email"}}},
						},
					}},
				},
			},
		}

		ar := arena.NewMonotonicArena(arena.WithMinBufferSize(4096))
		ctx := NewContext(context.Background())
		ctx.ExecutionOptions.Caching.EnableL1Cache = true
		ctx.ExecutionOptions.Caching.EnableCacheAnalytics = true
		ctx.ExecutionOptions.DisableSubgraphRequestDeduplication = true

		resolvable := NewResolvable(ar, ResolvableOptions{})
		err := resolvable.Init(ctx, nil, ast.OperationTypeQuery)
		require.NoError(t, err)

		loader := &Loader{
			ctx:        ctx,
			jsonArena:  ar,
			l1Cache:    map[string]*astjson.Value{},
			resolvable: resolvable,
			caches:     map[string]LoaderCache{"default": NewFakeLoaderCache()},
		}

		err = loader.LoadGraphQLResponseData(ctx, response, resolvable)
		require.NoError(t, err)

		out := fastjsonext.PrintGraphQLResponse(resolvable.data, resolvable.errors)
		assert.Equal(t, `{"data":{"user":{"__typename":"User","id":"1","nickname":"Alice","email":"alice@example.com","name":"Alice"}}}`, string(out),
			"fetch 3 should get name (no alias) from L1")

		stats := ctx.GetCacheStats()
		var l1Hits int
		for _, ev := range stats.L1Reads {
			if ev.Kind == CacheKeyHit {
				l1Hits++
			}
		}
		assert.Equal(t, 1, l1Hits, "Fetch 3 should hit L1 (schema name 'name' stored by fetch 1's alias normalize)")
	})
}
