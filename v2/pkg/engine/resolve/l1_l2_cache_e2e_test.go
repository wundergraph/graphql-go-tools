package resolve

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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
					Caching:        FetchCacheConfiguration{Enabled: true, CacheName: "default", CacheKeyTemplate: productCacheKeyTemplate},
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
					Caching:        FetchCacheConfiguration{Enabled: true, CacheName: "default", CacheKeyTemplate: productCacheKeyTemplate},
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
						Caching:        FetchCacheConfiguration{Enabled: true, CacheName: "default", CacheKeyTemplate: productCacheKeyTemplate, TTL: time.Minute},
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

		// Verify cache log shows miss then set
		log := cache.GetLog()
		require.GreaterOrEqual(t, len(log), 1)
		assert.Equal(t, "get", log[0].Operation)
		assert.False(t, log[0].Hits[0], "First request should be cache miss")

		// Second request (cache hit)
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

		// Verify cache hit
		log2 := cache.GetLog()
		require.GreaterOrEqual(t, len(log2), 1)
		assert.Equal(t, "get", log2[0].Operation)
		assert.True(t, log2[0].Hits[0], "Second request should be cache hit")
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
					Caching:        FetchCacheConfiguration{Enabled: true, CacheName: "default", CacheKeyTemplate: productCacheKeyTemplate},
				}, "query.product", ObjectPath("product")),
			),
			Data: &Object{Fields: []*Field{{Name: []byte("product"), Value: &Object{Path: []string{"product"}, Fields: []*Field{
				{Name: []byte("id"), Value: &String{Path: []string{"id"}}},
				{Name: []byte("name"), Value: &String{Path: []string{"name"}}},
			}}}}},
		}

		// Run twice with L2 disabled
		for i := 0; i < 2; i++ {
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
					Caching:        FetchCacheConfiguration{Enabled: true, CacheName: "default", CacheKeyTemplate: productCacheKeyTemplate, TTL: time.Minute},
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
					Caching:        FetchCacheConfiguration{Enabled: true, CacheName: "default", CacheKeyTemplate: productCacheKeyTemplate, TTL: time.Minute},
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

		// First entity fetch: L1 miss -> L2 miss -> fetch -> populate both
		// Second entity fetch: L1 hit -> skip everything
		// So we should only see one L2 get operation (for the first entity fetch)
		log := cache.GetLog()

		getCount := 0
		for _, entry := range log {
			if entry.Operation == "get" {
				getCount++
			}
		}
		assert.Equal(t, 1, getCount, "L1 hit should prevent second L2 lookup")
	})

	t.Run("L1+L2 - L1 miss, L2 hit provides data", func(t *testing.T) {
		// When L1 misses but L2 has data, data should come from L2
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		cache := NewFakeLoaderCache()

		// Pre-populate L2 cache with correct key format: {"__typename":"Product","key":{"id":"prod-1"}}
		cache.Set(context.Background(), []*CacheEntry{
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
					Caching:        FetchCacheConfiguration{Enabled: true, CacheName: "default", CacheKeyTemplate: productCacheKeyTemplate},
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

		// Verify L2 was consulted and hit
		log := cache.GetLog()
		require.GreaterOrEqual(t, len(log), 1)
		assert.Equal(t, "get", log[0].Operation)
		assert.True(t, log[0].Hits[0], "L2 should have hit")
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
						Caching:        FetchCacheConfiguration{Enabled: true, CacheName: "default", CacheKeyTemplate: productCacheKeyTemplate, TTL: time.Minute},
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

		// Verify L2 hit on second request
		log := cache.GetLog()
		require.GreaterOrEqual(t, len(log), 1)
		assert.Equal(t, "get", log[0].Operation)
		assert.True(t, log[0].Hits[0], "Request 2 should hit L2 cache")
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
					Caching:        FetchCacheConfiguration{Enabled: true, CacheName: "default", CacheKeyTemplate: productCacheKeyTemplate},
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

// TestCacheStatsThreadSafety verifies that L2 cache stats are thread-safe.
// This test should be run with -race flag: go test -race -run TestCacheStatsThreadSafety
//
// The test demonstrates that:
// - L1 stats are only accessed from the main thread (non-atomic, but safe due to single-thread access)
// - L2 stats use atomic operations (safe for concurrent access from goroutines)
func TestCacheStatsThreadSafety(t *testing.T) {
	t.Run("L2 stats concurrent access", func(t *testing.T) {
		// This test verifies no race conditions when multiple goroutines update L2 stats
		ctx := NewContext(context.Background())
		ctx.ExecutionOptions.Caching.EnableL2Cache = true

		const numGoroutines = 100

		var wg sync.WaitGroup
		wg.Add(numGoroutines * 2) // Each goroutine does both hit and miss

		for i := 0; i < numGoroutines; i++ {
			go func() {
				defer wg.Done()
				ctx.trackL2Hit()
			}()
			go func() {
				defer wg.Done()
				ctx.trackL2Miss()
			}()
		}
		wg.Wait()

		stats := ctx.GetCacheStats()
		assert.Equal(t, int64(numGoroutines), stats.L2Hits, "All L2 hits should be counted")
		assert.Equal(t, int64(numGoroutines), stats.L2Misses, "All L2 misses should be counted")
	})

	t.Run("L1 and L2 stats isolation", func(t *testing.T) {
		// This test verifies that L1 stats (main thread) and L2 stats (goroutines) are properly isolated
		ctx := NewContext(context.Background())
		ctx.ExecutionOptions.Caching.EnableL1Cache = true
		ctx.ExecutionOptions.Caching.EnableL2Cache = true

		// L1 stats on main thread
		ctx.trackL1Hit()
		ctx.trackL1Hit()
		ctx.trackL1Miss()

		// L2 stats from goroutines
		var wg sync.WaitGroup
		wg.Add(2)
		go func() {
			defer wg.Done()
			ctx.trackL2Hit()
			ctx.trackL2Hit()
			ctx.trackL2Hit()
		}()
		go func() {
			defer wg.Done()
			ctx.trackL2Miss()
			ctx.trackL2Miss()
		}()
		wg.Wait()

		stats := ctx.GetCacheStats()
		assert.Equal(t, int64(2), stats.L1Hits, "L1 hits should be 2")
		assert.Equal(t, int64(1), stats.L1Misses, "L1 misses should be 1")
		assert.Equal(t, int64(3), stats.L2Hits, "L2 hits should be 3")
		assert.Equal(t, int64(2), stats.L2Misses, "L2 misses should be 2")
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
		assert.Equal(t, int64(2), stats.L1Hits, "L1 should have 2 hits (parallel fetch for same entities skipped)")
		assert.Equal(t, int64(2), stats.L1Misses, "L1 should have 2 misses (first entity fetch)")
	})
}
