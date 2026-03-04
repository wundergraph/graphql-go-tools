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

// TestL1Cache tests the L1 (per-request, in-memory) entity cache functionality.
// L1 cache stores pointers to entities in the jsonArena, allowing reuse within a single request.
// It only applies to entity fetches (not root fetches) since root fields have no prior entity data.

func TestL1Cache(t *testing.T) {
	t.Run("L1 hit - same entity fetched twice in same request", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		// Root datasource - returns initial data
		rootDS := NewMockDataSource(ctrl)
		rootDS.EXPECT().
			Load(gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, headers any, input []byte) ([]byte, error) {
				return []byte(`{"data":{"product":{"__typename":"Product","id":"prod-1"}}}`), nil
			}).Times(1)

		// First entity fetch - should be called
		entityDS1 := NewMockDataSource(ctrl)
		entityDS1.EXPECT().
			Load(gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, headers any, input []byte) ([]byte, error) {
				return []byte(`{"data":{"_entities":[{"__typename":"Product","id":"prod-1","name":"Product One"}]}}`), nil
			}).Times(1)

		// Second entity fetch - should NOT be called (L1 hit)
		entityDS2 := NewMockDataSource(ctrl)
		entityDS2.EXPECT().
			Load(gomock.Any(), gomock.Any(), gomock.Any()).
			Times(0) // L1 should prevent this call

		productCacheKeyTemplate := &EntityQueryCacheKeyTemplate{
			Keys: NewResolvableObjectVariable(&Object{
				Fields: []*Field{
					{
						Name: []byte("__typename"),
						Value: &String{
							Path: []string{"__typename"},
						},
					},
					{
						Name: []byte("id"),
						Value: &String{
							Path: []string{"id"},
						},
					},
				},
			}),
		}

		providesData := &Object{
			Fields: []*Field{
				{
					Name: []byte("id"),
					Value: &Scalar{
						Path:     []string{"id"},
						Nullable: false,
					},
				},
				{
					Name: []byte("name"),
					Value: &Scalar{
						Path:     []string{"name"},
						Nullable: false,
					},
				},
			},
		}

		response := &GraphQLResponse{
			Info: &GraphQLResponseInfo{
				OperationType: ast.OperationTypeQuery,
			},
			Fetches: Sequence(
				// Root fetch
				SingleWithPath(&SingleFetch{
					FetchConfiguration: FetchConfiguration{
						DataSource: rootDS,
						PostProcessing: PostProcessingConfiguration{
							SelectResponseDataPath: []string{"data"},
						},
					},
					InputTemplate: InputTemplate{
						Segments: []TemplateSegment{
							{
								Data:        []byte(`{"method":"POST","url":"http://root.service","body":{"query":"{product {__typename id}}"}}`),
								SegmentType: StaticSegmentType,
							},
						},
					},
					DataSourceIdentifier: []byte("graphql_datasource.Source"),
				}, "query"),

				// First entity fetch - populates L1 cache
				SingleWithPath(&SingleFetch{
					FetchConfiguration: FetchConfiguration{
						DataSource: entityDS1,
						PostProcessing: PostProcessingConfiguration{
							SelectResponseDataPath: []string{"data", "_entities", "0"},
						},
						Caching: FetchCacheConfiguration{
							Enabled:          true,
							CacheName:        "default",
							TTL:              30 * time.Second,
							CacheKeyTemplate: productCacheKeyTemplate,
							UseL1Cache:       true,
						},
					},
					InputTemplate: InputTemplate{
						Segments: []TemplateSegment{
							{
								Data:        []byte(`{"method":"POST","url":"http://products.service","body":{"query":"...","variables":{"representations":[`),
								SegmentType: StaticSegmentType,
							},
							{
								SegmentType:  VariableSegmentType,
								VariableKind: ResolvableObjectVariableKind,
								Renderer: NewGraphQLVariableResolveRenderer(&Object{
									Fields: []*Field{
										{Name: []byte("__typename"), Value: &String{Path: []string{"__typename"}}},
										{Name: []byte("id"), Value: &String{Path: []string{"id"}}},
									},
								}),
							},
							{
								Data:        []byte(`]}}}`),
								SegmentType: StaticSegmentType,
							},
						},
					},
					Info: &FetchInfo{
						DataSourceID:   "products",
						DataSourceName: "products",
						OperationType:  ast.OperationTypeQuery,
						ProvidesData:   providesData,
					},
					DataSourceIdentifier: []byte("graphql_datasource.Source"),
				}, "query.product", ObjectPath("product")),

				// Second entity fetch for SAME entity - should hit L1 cache
				SingleWithPath(&SingleFetch{
					FetchConfiguration: FetchConfiguration{
						DataSource: entityDS2,
						PostProcessing: PostProcessingConfiguration{
							SelectResponseDataPath: []string{"data", "_entities", "0"},
						},
						Caching: FetchCacheConfiguration{
							Enabled:          true,
							CacheName:        "default",
							TTL:              30 * time.Second,
							CacheKeyTemplate: productCacheKeyTemplate,
							UseL1Cache:       true,
						},
					},
					InputTemplate: InputTemplate{
						Segments: []TemplateSegment{
							{
								Data:        []byte(`{"method":"POST","url":"http://products.service","body":{"query":"...","variables":{"representations":[`),
								SegmentType: StaticSegmentType,
							},
							{
								SegmentType:  VariableSegmentType,
								VariableKind: ResolvableObjectVariableKind,
								Renderer: NewGraphQLVariableResolveRenderer(&Object{
									Fields: []*Field{
										{Name: []byte("__typename"), Value: &String{Path: []string{"__typename"}}},
										{Name: []byte("id"), Value: &String{Path: []string{"id"}}},
									},
								}),
							},
							{
								Data:        []byte(`]}}}`),
								SegmentType: StaticSegmentType,
							},
						},
					},
					Info: &FetchInfo{
						DataSourceID:   "products",
						DataSourceName: "products",
						OperationType:  ast.OperationTypeQuery,
						ProvidesData:   providesData,
					},
					DataSourceIdentifier: []byte("graphql_datasource.Source"),
				}, "query.product", ObjectPath("product")),
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
							},
						},
					},
				},
			},
		}

		// Create loader WITHOUT L2 cache - only L1
		loader := &Loader{}

		ctx := NewContext(context.Background())
		ctx.ExecutionOptions.DisableSubgraphRequestDeduplication = true
		ctx.ExecutionOptions.Caching.EnableL1Cache = true
		// L2 disabled - testing L1 only

		ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
		resolvable := NewResolvable(ar, ResolvableOptions{})
		err := resolvable.Init(ctx, nil, ast.OperationTypeQuery)
		require.NoError(t, err)

		err = loader.LoadGraphQLResponseData(ctx, response, resolvable)
		require.NoError(t, err)

		out := fastjsonext.PrintGraphQLResponse(resolvable.data, resolvable.errors)
		assert.Equal(t, `{"data":{"product":{"__typename":"Product","id":"prod-1","name":"Product One"}}}`, out)
	})

	t.Run("L1 disabled - each entity fetch goes to subgraph", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		// Root datasource
		rootDS := NewMockDataSource(ctrl)
		rootDS.EXPECT().
			Load(gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, headers any, input []byte) ([]byte, error) {
				return []byte(`{"data":{"product":{"__typename":"Product","id":"prod-1"}}}`), nil
			}).Times(1)

		// Entity fetch - should be called TWICE (no L1 cache)
		entityDS := NewMockDataSource(ctrl)
		entityDS.EXPECT().
			Load(gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, headers any, input []byte) ([]byte, error) {
				return []byte(`{"data":{"_entities":[{"__typename":"Product","id":"prod-1","name":"Product One"}]}}`), nil
			}).Times(2) // Called twice because L1 is disabled

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
			Info: &GraphQLResponseInfo{
				OperationType: ast.OperationTypeQuery,
			},
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
							{Data: []byte(`{"method":"POST"}`), SegmentType: StaticSegmentType},
						},
					},
					DataSourceIdentifier: []byte("graphql_datasource.Source"),
				}, "query"),

				// First entity fetch
				SingleWithPath(&SingleFetch{
					FetchConfiguration: FetchConfiguration{
						DataSource: entityDS,
						PostProcessing: PostProcessingConfiguration{
							SelectResponseDataPath: []string{"data", "_entities", "0"},
						},
						Caching: FetchCacheConfiguration{
							Enabled:          true,
							CacheName:        "default",
							TTL:              30 * time.Second,
							CacheKeyTemplate: productCacheKeyTemplate,
						},
					},
					InputTemplate: InputTemplate{
						Segments: []TemplateSegment{
							{Data: []byte(`{"method":"POST"}`), SegmentType: StaticSegmentType},
						},
					},
					Info: &FetchInfo{
						DataSourceID:   "products",
						DataSourceName: "products",
						OperationType:  ast.OperationTypeQuery,
						ProvidesData:   providesData,
					},
					DataSourceIdentifier: []byte("graphql_datasource.Source"),
				}, "query.product", ObjectPath("product")),

				// Second entity fetch - should also be called (L1 disabled)
				SingleWithPath(&SingleFetch{
					FetchConfiguration: FetchConfiguration{
						DataSource: entityDS,
						PostProcessing: PostProcessingConfiguration{
							SelectResponseDataPath: []string{"data", "_entities", "0"},
						},
						Caching: FetchCacheConfiguration{
							Enabled:          true,
							CacheName:        "default",
							TTL:              30 * time.Second,
							CacheKeyTemplate: productCacheKeyTemplate,
						},
					},
					InputTemplate: InputTemplate{
						Segments: []TemplateSegment{
							{Data: []byte(`{"method":"POST"}`), SegmentType: StaticSegmentType},
						},
					},
					Info: &FetchInfo{
						DataSourceID:   "products",
						DataSourceName: "products",
						OperationType:  ast.OperationTypeQuery,
						ProvidesData:   providesData,
					},
					DataSourceIdentifier: []byte("graphql_datasource.Source"),
				}, "query.product", ObjectPath("product")),
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
							},
						},
					},
				},
			},
		}

		loader := &Loader{}

		ctx := NewContext(context.Background())
		ctx.ExecutionOptions.DisableSubgraphRequestDeduplication = true
		ctx.ExecutionOptions.Caching.EnableL1Cache = false // L1 DISABLED

		ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
		resolvable := NewResolvable(ar, ResolvableOptions{})
		err := resolvable.Init(ctx, nil, ast.OperationTypeQuery)
		require.NoError(t, err)

		err = loader.LoadGraphQLResponseData(ctx, response, resolvable)
		require.NoError(t, err)

		out := fastjsonext.PrintGraphQLResponse(resolvable.data, resolvable.errors)
		assert.Equal(t, `{"data":{"product":{"__typename":"Product","id":"prod-1","name":"Product One"}}}`, out)
	})

	t.Run("L1 partial data - fetch needed when missing required fields", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		// Root datasource
		rootDS := NewMockDataSource(ctrl)
		rootDS.EXPECT().
			Load(gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, headers any, input []byte) ([]byte, error) {
				return []byte(`{"data":{"product":{"__typename":"Product","id":"prod-1"}}}`), nil
			}).Times(1)

		// First entity fetch - only returns id and name
		entityDS1 := NewMockDataSource(ctrl)
		entityDS1.EXPECT().
			Load(gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, headers any, input []byte) ([]byte, error) {
				return []byte(`{"data":{"_entities":[{"__typename":"Product","id":"prod-1","name":"Product One"}]}}`), nil
			}).Times(1)

		// Second entity fetch needs price field - L1 has partial data, so fetch is needed
		entityDS2 := NewMockDataSource(ctrl)
		entityDS2.EXPECT().
			Load(gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, headers any, input []byte) ([]byte, error) {
				return []byte(`{"data":{"_entities":[{"__typename":"Product","id":"prod-1","name":"Product One","price":99.99}]}}`), nil
			}).Times(1) // Should be called because L1 doesn't have price field

		productCacheKeyTemplate := &EntityQueryCacheKeyTemplate{
			Keys: NewResolvableObjectVariable(&Object{
				Fields: []*Field{
					{Name: []byte("__typename"), Value: &String{Path: []string{"__typename"}}},
					{Name: []byte("id"), Value: &String{Path: []string{"id"}}},
				},
			}),
		}

		providesDataIdName := &Object{
			Fields: []*Field{
				{Name: []byte("id"), Value: &Scalar{Path: []string{"id"}, Nullable: false}},
				{Name: []byte("name"), Value: &Scalar{Path: []string{"name"}, Nullable: false}},
			},
		}

		providesDataIdNamePrice := &Object{
			Fields: []*Field{
				{Name: []byte("id"), Value: &Scalar{Path: []string{"id"}, Nullable: false}},
				{Name: []byte("name"), Value: &Scalar{Path: []string{"name"}, Nullable: false}},
				{Name: []byte("price"), Value: &Scalar{Path: []string{"price"}, Nullable: false}},
			},
		}

		response := &GraphQLResponse{
			Info: &GraphQLResponseInfo{
				OperationType: ast.OperationTypeQuery,
			},
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
							{Data: []byte(`{"method":"POST"}`), SegmentType: StaticSegmentType},
						},
					},
					DataSourceIdentifier: []byte("graphql_datasource.Source"),
				}, "query"),

				// First entity fetch - provides id, name
				SingleWithPath(&SingleFetch{
					FetchConfiguration: FetchConfiguration{
						DataSource: entityDS1,
						PostProcessing: PostProcessingConfiguration{
							SelectResponseDataPath: []string{"data", "_entities", "0"},
						},
						Caching: FetchCacheConfiguration{
							Enabled:          true,
							CacheName:        "default",
							TTL:              30 * time.Second,
							CacheKeyTemplate: productCacheKeyTemplate,
						},
					},
					InputTemplate: InputTemplate{
						Segments: []TemplateSegment{
							{Data: []byte(`{"method":"POST"}`), SegmentType: StaticSegmentType},
						},
					},
					Info: &FetchInfo{
						DataSourceID:   "products",
						DataSourceName: "products",
						OperationType:  ast.OperationTypeQuery,
						ProvidesData:   providesDataIdName,
					},
					DataSourceIdentifier: []byte("graphql_datasource.Source"),
				}, "query.product", ObjectPath("product")),

				// Second entity fetch - needs id, name, price (partial miss)
				SingleWithPath(&SingleFetch{
					FetchConfiguration: FetchConfiguration{
						DataSource: entityDS2,
						PostProcessing: PostProcessingConfiguration{
							SelectResponseDataPath: []string{"data", "_entities", "0"},
						},
						Caching: FetchCacheConfiguration{
							Enabled:          true,
							CacheName:        "default",
							TTL:              30 * time.Second,
							CacheKeyTemplate: productCacheKeyTemplate,
						},
					},
					InputTemplate: InputTemplate{
						Segments: []TemplateSegment{
							{Data: []byte(`{"method":"POST"}`), SegmentType: StaticSegmentType},
						},
					},
					Info: &FetchInfo{
						DataSourceID:   "products",
						DataSourceName: "products",
						OperationType:  ast.OperationTypeQuery,
						ProvidesData:   providesDataIdNamePrice, // Needs price field
					},
					DataSourceIdentifier: []byte("graphql_datasource.Source"),
				}, "query.product", ObjectPath("product")),
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

		ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
		resolvable := NewResolvable(ar, ResolvableOptions{})
		err := resolvable.Init(ctx, nil, ast.OperationTypeQuery)
		require.NoError(t, err)

		err = loader.LoadGraphQLResponseData(ctx, response, resolvable)
		require.NoError(t, err)

		out := fastjsonext.PrintGraphQLResponse(resolvable.data, resolvable.errors)
		assert.Equal(t, `{"data":{"product":{"__typename":"Product","id":"prod-1","name":"Product One","price":99.99}}}`, out)
	})
}

// TestL1CachePartialLoading tests the partial cache loading feature.
// When EnablePartialCacheLoad is true, only cache-missed entities are fetched from the subgraph.
// This test uses the L2 cache to pre-populate data, simulating a scenario where some entities
// are cached and others are not.
func TestL1CachePartialLoading(t *testing.T) {
	t.Run("partial cache loading with L2 - only missing entities fetched", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		cache := NewFakeLoaderCache()

		// Pre-populate cache with prod-1 only (prod-2 and prod-3 are NOT cached)
		prod1Data := `{"__typename":"Product","id":"prod-1","name":"Cached Product One"}`
		err := cache.Set(context.Background(), []*CacheEntry{
			{Key: `{"__typename":"Product","key":{"id":"prod-1"}}`, Value: []byte(prod1Data)},
		}, 30*time.Second)
		require.NoError(t, err)
		cache.ClearLog()

		// Root datasource - returns 3 products
		rootDS := NewMockDataSource(ctrl)
		rootDS.EXPECT().
			Load(gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, headers any, input []byte) ([]byte, error) {
				return []byte(`{"data":{"topProducts":[{"__typename":"Product","id":"prod-1"},{"__typename":"Product","id":"prod-2"},{"__typename":"Product","id":"prod-3"}]}}`), nil
			}).Times(1)

		// Batch entity fetch - WITH partial cache loading enabled
		// Only prod-2 and prod-3 should be fetched (prod-1 is in L2 cache)
		entityDS := NewMockDataSource(ctrl)
		entityDS.EXPECT().
			Load(gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, headers any, input []byte) ([]byte, error) {
				// Verify exact input - only prod-2 and prod-3, NOT prod-1 (cached)
				expectedInput := `{"method":"POST","body":{"query":"...","variables":{"representations":[{"__typename":"Product","id":"prod-2"},{"__typename":"Product","id":"prod-3"}]}}}`
				assert.Equal(t, expectedInput, string(input))
				return []byte(`{"data":{"_entities":[{"__typename":"Product","id":"prod-2","name":"Fetched Product Two"},{"__typename":"Product","id":"prod-3","name":"Fetched Product Three"}]}}`), nil
			}).Times(1)

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
			Info: &GraphQLResponseInfo{
				OperationType: ast.OperationTypeQuery,
			},
			Fetches: Sequence(
				// Root fetch
				SingleWithPath(&SingleFetch{
					FetchConfiguration: FetchConfiguration{
						DataSource: rootDS,
						PostProcessing: PostProcessingConfiguration{
							SelectResponseDataPath: []string{"data"},
						},
					},
					InputTemplate: InputTemplate{
						Segments: []TemplateSegment{
							{Data: []byte(`{"method":"POST"}`), SegmentType: StaticSegmentType},
						},
					},
					DataSourceIdentifier: []byte("graphql_datasource.Source"),
				}, "query"),

				// Batch entity fetch - WITH EnablePartialCacheLoad
				// Should only fetch prod-2 and prod-3 (prod-1 is in cache)
				SingleWithPath(&BatchEntityFetch{
					Input: BatchInput{
						Header: InputTemplate{
							Segments: []TemplateSegment{
								{Data: []byte(`{"method":"POST","body":{"query":"...","variables":{"representations":[`), SegmentType: StaticSegmentType},
							},
						},
						Items: []InputTemplate{
							{
								Segments: []TemplateSegment{
									{
										SegmentType:  VariableSegmentType,
										VariableKind: ResolvableObjectVariableKind,
										Renderer: NewGraphQLVariableResolveRenderer(&Object{
											Fields: []*Field{
												{Name: []byte("__typename"), Value: &String{Path: []string{"__typename"}}},
												{Name: []byte("id"), Value: &String{Path: []string{"id"}}},
											},
										}),
									},
								},
							},
						},
						Separator: InputTemplate{
							Segments: []TemplateSegment{{Data: []byte(`,`), SegmentType: StaticSegmentType}},
						},
						Footer: InputTemplate{
							Segments: []TemplateSegment{{Data: []byte(`]}}}`), SegmentType: StaticSegmentType}},
						},
					},
					DataSource: entityDS,
					PostProcessing: PostProcessingConfiguration{
						SelectResponseDataPath: []string{"data", "_entities"},
					},
					Info: &FetchInfo{
						DataSourceID:   "products",
						DataSourceName: "products",
						OperationType:  ast.OperationTypeQuery,
						ProvidesData:   providesData,
					},
					Caching: FetchCacheConfiguration{
						Enabled:                true,
						CacheName:              "default",
						TTL:                    30 * time.Second,
						CacheKeyTemplate:       productCacheKeyTemplate,
						UseL1Cache:             true,
						EnablePartialCacheLoad: true, // KEY: Enable partial loading
					},
					DataSourceIdentifier: []byte("graphql_datasource.Source"),
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
								},
							},
						},
					},
				},
			},
		}

		loader := &Loader{
			caches: map[string]LoaderCache{
				"default": cache,
			},
		}

		ctx := NewContext(context.Background())
		ctx.ExecutionOptions.DisableSubgraphRequestDeduplication = true
		ctx.ExecutionOptions.Caching.EnableL1Cache = true
		ctx.ExecutionOptions.Caching.EnableL2Cache = true

		ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
		resolvable := NewResolvable(ar, ResolvableOptions{})
		err = resolvable.Init(ctx, nil, ast.OperationTypeQuery)
		require.NoError(t, err)

		err = loader.LoadGraphQLResponseData(ctx, response, resolvable)
		require.NoError(t, err)

		out := fastjsonext.PrintGraphQLResponse(resolvable.data, resolvable.errors)

		// All 3 products should be in the result
		// prod-1 should have the cached name, prod-2 and prod-3 should have fetched names
		expectedOutput := `{"data":{"topProducts":[{"__typename":"Product","id":"prod-1","name":"Cached Product One"},{"__typename":"Product","id":"prod-2","name":"Fetched Product Two"},{"__typename":"Product","id":"prod-3","name":"Fetched Product Three"}]}}`
		assert.Equal(t, expectedOutput, out)
	})

	t.Run("partial cache loading disabled with L2 - all entities fetched", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		cache := NewFakeLoaderCache()

		// Pre-populate cache with prod-1 only
		prod1Data := `{"__typename":"Product","id":"prod-1","name":"Cached Product One"}`
		err := cache.Set(context.Background(), []*CacheEntry{
			{Key: `{"__typename":"Product","key":{"id":"prod-1"}}`, Value: []byte(prod1Data)},
		}, 30*time.Second)
		require.NoError(t, err)
		cache.ClearLog()

		// Root datasource - returns 3 products
		rootDS := NewMockDataSource(ctrl)
		rootDS.EXPECT().
			Load(gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, headers any, input []byte) ([]byte, error) {
				return []byte(`{"data":{"topProducts":[{"__typename":"Product","id":"prod-1"},{"__typename":"Product","id":"prod-2"},{"__typename":"Product","id":"prod-3"}]}}`), nil
			}).Times(1)

		// Batch entity fetch - WITHOUT partial cache loading (default)
		// ALL 3 entities should be fetched
		entityDS := NewMockDataSource(ctrl)
		entityDS.EXPECT().
			Load(gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, headers any, input []byte) ([]byte, error) {
				// Verify exact input - all 3 entities (partial loading disabled)
				expectedInput := `{"method":"POST","body":{"query":"...","variables":{"representations":[{"__typename":"Product","id":"prod-1"},{"__typename":"Product","id":"prod-2"},{"__typename":"Product","id":"prod-3"}]}}}`
				assert.Equal(t, expectedInput, string(input))
				return []byte(`{"data":{"_entities":[{"__typename":"Product","id":"prod-1","name":"Fetched Product One"},{"__typename":"Product","id":"prod-2","name":"Fetched Product Two"},{"__typename":"Product","id":"prod-3","name":"Fetched Product Three"}]}}`), nil
			}).Times(1)

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
			Info: &GraphQLResponseInfo{
				OperationType: ast.OperationTypeQuery,
			},
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
							{Data: []byte(`{"method":"POST"}`), SegmentType: StaticSegmentType},
						},
					},
					DataSourceIdentifier: []byte("graphql_datasource.Source"),
				}, "query"),

				SingleWithPath(&BatchEntityFetch{
					Input: BatchInput{
						Header: InputTemplate{
							Segments: []TemplateSegment{
								{Data: []byte(`{"method":"POST","body":{"query":"...","variables":{"representations":[`), SegmentType: StaticSegmentType},
							},
						},
						Items: []InputTemplate{
							{
								Segments: []TemplateSegment{
									{
										SegmentType:  VariableSegmentType,
										VariableKind: ResolvableObjectVariableKind,
										Renderer: NewGraphQLVariableResolveRenderer(&Object{
											Fields: []*Field{
												{Name: []byte("__typename"), Value: &String{Path: []string{"__typename"}}},
												{Name: []byte("id"), Value: &String{Path: []string{"id"}}},
											},
										}),
									},
								},
							},
						},
						Separator: InputTemplate{
							Segments: []TemplateSegment{{Data: []byte(`,`), SegmentType: StaticSegmentType}},
						},
						Footer: InputTemplate{
							Segments: []TemplateSegment{{Data: []byte(`]}}}`), SegmentType: StaticSegmentType}},
						},
					},
					DataSource: entityDS,
					PostProcessing: PostProcessingConfiguration{
						SelectResponseDataPath: []string{"data", "_entities"},
					},
					Info: &FetchInfo{
						DataSourceID:   "products",
						DataSourceName: "products",
						OperationType:  ast.OperationTypeQuery,
						ProvidesData:   providesData,
					},
					Caching: FetchCacheConfiguration{
						Enabled:                true,
						CacheName:              "default",
						TTL:                    30 * time.Second,
						CacheKeyTemplate:       productCacheKeyTemplate,
						UseL1Cache:             true,
						EnablePartialCacheLoad: false, // KEY: Partial loading DISABLED (default)
					},
					DataSourceIdentifier: []byte("graphql_datasource.Source"),
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
								},
							},
						},
					},
				},
			},
		}

		loader := &Loader{
			caches: map[string]LoaderCache{
				"default": cache,
			},
		}

		ctx := NewContext(context.Background())
		ctx.ExecutionOptions.DisableSubgraphRequestDeduplication = true
		ctx.ExecutionOptions.Caching.EnableL1Cache = true
		ctx.ExecutionOptions.Caching.EnableL2Cache = true

		ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
		resolvable := NewResolvable(ar, ResolvableOptions{})
		err = resolvable.Init(ctx, nil, ast.OperationTypeQuery)
		require.NoError(t, err)

		err = loader.LoadGraphQLResponseData(ctx, response, resolvable)
		require.NoError(t, err)

		out := fastjsonext.PrintGraphQLResponse(resolvable.data, resolvable.errors)

		// All 3 products should be in the result with fetched names (not cached)
		expectedOutput := `{"data":{"topProducts":[{"__typename":"Product","id":"prod-1","name":"Fetched Product One"},{"__typename":"Product","id":"prod-2","name":"Fetched Product Two"},{"__typename":"Product","id":"prod-3","name":"Fetched Product Three"}]}}`
		assert.Equal(t, expectedOutput, out)
	})
}

// TestL1CachePartialLoadingL1Only tests partial cache loading using only L1 cache (no L2).
// This tests a realistic scenario where a batch entity fetch for nested entities
// encounters some entities that are already in L1 cache from a previous fetch.
//
// Scenario: Products with reviews, where each review has an author.
// - First batch fetch: Get reviews for products (returns author references)
// - Second batch fetch: Get author details - some authors are duplicated across reviews
// - With L1 cache and partial loading, duplicate authors should come from cache
func TestL1CachePartialLoadingL1Only(t *testing.T) {
	t.Run("L1 partial cache loading - duplicate entities from nested fetch", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		// Root datasource - returns products with reviews
		// Each review has an author reference, some authors appear multiple times
		rootDS := NewMockDataSource(ctrl)
		rootDS.EXPECT().
			Load(gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, headers any, input []byte) ([]byte, error) {
				// Product has 3 reviews: 2 by author-1, 1 by author-2
				return []byte(`{"data":{"product":{"__typename":"Product","id":"prod-1","reviews":[{"body":"Great!","author":{"__typename":"User","id":"author-1"}},{"body":"Love it!","author":{"__typename":"User","id":"author-1"}},{"body":"Nice!","author":{"__typename":"User","id":"author-2"}}]}}}`), nil
			}).Times(1)

		// First batch entity fetch - fetches ALL authors (author-1, author-1, author-2)
		// This populates L1 cache with author-1 and author-2
		// Note: Due to deduplication in batch, author-1 appears once in the actual request
		entityDS1 := NewMockDataSource(ctrl)
		entityDS1.EXPECT().
			Load(gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, headers any, input []byte) ([]byte, error) {
				// Verify exact input - deduplicated to 2 unique authors
				expectedInput := `{"method":"POST","body":{"query":"first author fetch","variables":{"representations":[{"__typename":"User","id":"author-1"},{"__typename":"User","id":"author-2"}]}}}`
				assert.Equal(t, expectedInput, string(input))
				// Response for unique authors (deduplicated)
				return []byte(`{"data":{"_entities":[{"__typename":"User","id":"author-1","username":"user1"},{"__typename":"User","id":"author-2","username":"user2"}]}}`), nil
			}).Times(1)

		// Second batch entity fetch - WITH partial cache loading enabled
		// This fetch requests all 3 author references again
		// With partial loading: author-1 and author-2 are in L1 cache, no fetch needed
		// Since ALL are cached, the fetch should be skipped entirely
		entityDS2 := NewMockDataSource(ctrl)
		entityDS2.EXPECT().
			Load(gomock.Any(), gomock.Any(), gomock.Any()).
			Times(0) // Should NOT be called - all authors are in L1 cache

		userCacheKeyTemplate := &EntityQueryCacheKeyTemplate{
			Keys: NewResolvableObjectVariable(&Object{
				Fields: []*Field{
					{Name: []byte("__typename"), Value: &String{Path: []string{"__typename"}}},
					{Name: []byte("id"), Value: &String{Path: []string{"id"}}},
				},
			}),
		}

		userProvidesData := &Object{
			Fields: []*Field{
				{Name: []byte("id"), Value: &Scalar{Path: []string{"id"}, Nullable: false}},
				{Name: []byte("username"), Value: &Scalar{Path: []string{"username"}, Nullable: false}},
			},
		}

		response := &GraphQLResponse{
			Info: &GraphQLResponseInfo{
				OperationType: ast.OperationTypeQuery,
			},
			Fetches: Sequence(
				// Root fetch - gets product with reviews and author references
				SingleWithPath(&SingleFetch{
					FetchConfiguration: FetchConfiguration{
						DataSource: rootDS,
						PostProcessing: PostProcessingConfiguration{
							SelectResponseDataPath: []string{"data"},
						},
					},
					InputTemplate: InputTemplate{
						Segments: []TemplateSegment{
							{Data: []byte(`{"method":"POST"}`), SegmentType: StaticSegmentType},
						},
					},
					DataSourceIdentifier: []byte("graphql_datasource.Source"),
				}, "query"),

				// First batch entity fetch - for authors (populates L1 cache)
				SingleWithPath(&BatchEntityFetch{
					Input: BatchInput{
						Header: InputTemplate{
							Segments: []TemplateSegment{
								{Data: []byte(`{"method":"POST","body":{"query":"first author fetch","variables":{"representations":[`), SegmentType: StaticSegmentType},
							},
						},
						Items: []InputTemplate{
							{
								Segments: []TemplateSegment{
									{
										SegmentType:  VariableSegmentType,
										VariableKind: ResolvableObjectVariableKind,
										Renderer: NewGraphQLVariableResolveRenderer(&Object{
											Fields: []*Field{
												{Name: []byte("__typename"), Value: &String{Path: []string{"__typename"}}},
												{Name: []byte("id"), Value: &String{Path: []string{"id"}}},
											},
										}),
									},
								},
							},
						},
						Separator: InputTemplate{
							Segments: []TemplateSegment{{Data: []byte(`,`), SegmentType: StaticSegmentType}},
						},
						Footer: InputTemplate{
							Segments: []TemplateSegment{{Data: []byte(`]}}}`), SegmentType: StaticSegmentType}},
						},
					},
					DataSource: entityDS1,
					PostProcessing: PostProcessingConfiguration{
						SelectResponseDataPath: []string{"data", "_entities"},
					},
					Info: &FetchInfo{
						DataSourceID:   "users",
						DataSourceName: "users",
						OperationType:  ast.OperationTypeQuery,
						ProvidesData:   userProvidesData,
					},
					Caching: FetchCacheConfiguration{
						Enabled:          true,
						CacheName:        "default",
						TTL:              30 * time.Second,
						CacheKeyTemplate: userCacheKeyTemplate,
						UseL1Cache:       true,
						// First fetch does NOT have partial loading - fetches all
					},
					DataSourceIdentifier: []byte("graphql_datasource.Source"),
				}, "query.product.reviews.author", ObjectPath("product"), ArrayPath("reviews"), ObjectPath("author")),

				// Second batch entity fetch - WITH EnablePartialCacheLoad
				// Should skip fetch entirely (all authors already in L1 cache)
				SingleWithPath(&BatchEntityFetch{
					Input: BatchInput{
						Header: InputTemplate{
							Segments: []TemplateSegment{
								{Data: []byte(`{"method":"POST","body":{"query":"second author fetch","variables":{"representations":[`), SegmentType: StaticSegmentType},
							},
						},
						Items: []InputTemplate{
							{
								Segments: []TemplateSegment{
									{
										SegmentType:  VariableSegmentType,
										VariableKind: ResolvableObjectVariableKind,
										Renderer: NewGraphQLVariableResolveRenderer(&Object{
											Fields: []*Field{
												{Name: []byte("__typename"), Value: &String{Path: []string{"__typename"}}},
												{Name: []byte("id"), Value: &String{Path: []string{"id"}}},
											},
										}),
									},
								},
							},
						},
						Separator: InputTemplate{
							Segments: []TemplateSegment{{Data: []byte(`,`), SegmentType: StaticSegmentType}},
						},
						Footer: InputTemplate{
							Segments: []TemplateSegment{{Data: []byte(`]}}}`), SegmentType: StaticSegmentType}},
						},
					},
					DataSource: entityDS2,
					PostProcessing: PostProcessingConfiguration{
						SelectResponseDataPath: []string{"data", "_entities"},
					},
					Info: &FetchInfo{
						DataSourceID:   "users",
						DataSourceName: "users",
						OperationType:  ast.OperationTypeQuery,
						ProvidesData:   userProvidesData,
					},
					Caching: FetchCacheConfiguration{
						Enabled:                true,
						CacheName:              "default",
						TTL:                    30 * time.Second,
						CacheKeyTemplate:       userCacheKeyTemplate,
						UseL1Cache:             true,
						EnablePartialCacheLoad: true, // KEY: Enable partial loading
					},
					DataSourceIdentifier: []byte("graphql_datasource.Source"),
				}, "query.product.reviews.author", ObjectPath("product"), ArrayPath("reviews"), ObjectPath("author")),
			),
			Data: &Object{
				Fields: []*Field{
					{
						Name: []byte("product"),
						Value: &Object{
							Path: []string{"product"},
							Fields: []*Field{
								{Name: []byte("id"), Value: &String{Path: []string{"id"}}},
								{
									Name: []byte("reviews"),
									Value: &Array{
										Path: []string{"reviews"},
										Item: &Object{
											Fields: []*Field{
												{Name: []byte("body"), Value: &String{Path: []string{"body"}}},
												{
													Name: []byte("author"),
													Value: &Object{
														Path: []string{"author"},
														Fields: []*Field{
															{Name: []byte("id"), Value: &String{Path: []string{"id"}}},
															{Name: []byte("username"), Value: &String{Path: []string{"username"}}},
														},
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		}

		// NO L2 cache - testing L1 only
		loader := &Loader{}

		ctx := NewContext(context.Background())
		ctx.ExecutionOptions.DisableSubgraphRequestDeduplication = true
		ctx.ExecutionOptions.Caching.EnableL1Cache = true
		ctx.ExecutionOptions.Caching.EnableL2Cache = false // L2 disabled

		ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
		resolvable := NewResolvable(ar, ResolvableOptions{})
		err := resolvable.Init(ctx, nil, ast.OperationTypeQuery)
		require.NoError(t, err)

		err = loader.LoadGraphQLResponseData(ctx, response, resolvable)
		require.NoError(t, err)

		out := fastjsonext.PrintGraphQLResponse(resolvable.data, resolvable.errors)

		// All authors should be in the result with usernames from first fetch
		expectedOutput := `{"data":{"product":{"__typename":"Product","id":"prod-1","reviews":[{"body":"Great!","author":{"__typename":"User","id":"author-1","username":"user1"}},{"body":"Love it!","author":{"__typename":"User","id":"author-1","username":"user1"}},{"body":"Nice!","author":{"__typename":"User","id":"author-2","username":"user2"}}]}}}`
		assert.Equal(t, expectedOutput, out)
	})
}

func TestL1CacheNestedEntitiesInFetchResponse(t *testing.T) {
	t.Run("nested entities in entity fetch response are not populated in L1", func(t *testing.T) {
		// When entity fetch 1 returns User u1 whose response contains a nested User u3
		// (via bestFriend), only u1 is stored in L1. The nested u3 is NOT extracted and
		// cached separately. A subsequent entity fetch 2 for u3 must call the subgraph.
		//
		// If nested entity L1 population were implemented, entityDS2 would be Times(0)
		// because u3 would already be in L1 from fetch 1's response.
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		// Root fetch - returns two user references at different paths
		rootDS := NewMockDataSource(ctrl)
		rootDS.EXPECT().
			Load(gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, headers any, input []byte) ([]byte, error) {
				return []byte(`{"data":{"firstUser":{"__typename":"User","id":"u1"},"secondUser":{"__typename":"User","id":"u3"}}}`), nil
			}).Times(1)

		// Entity fetch 1 - resolves User u1, response includes nested User u3 (bestFriend)
		entityDS1 := NewMockDataSource(ctrl)
		entityDS1.EXPECT().
			Load(gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, headers any, input []byte) ([]byte, error) {
				return []byte(`{"data":{"_entities":[{"__typename":"User","id":"u1","name":"Alice","bestFriend":{"__typename":"User","id":"u3","name":"Charlie"}}]}}`), nil
			}).Times(1)

		// Entity fetch 2 - resolves User u3
		// Called because u3 is NOT in L1 (only u1 was cached from fetch 1)
		entityDS2 := NewMockDataSource(ctrl)
		entityDS2.EXPECT().
			Load(gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, headers any, input []byte) ([]byte, error) {
				return []byte(`{"data":{"_entities":[{"__typename":"User","id":"u3","name":"Charlie"}]}}`), nil
			}).Times(1) // Would be Times(0) if nested entity L1 population were implemented

		userCacheKeyTemplate := &EntityQueryCacheKeyTemplate{
			Keys: NewResolvableObjectVariable(&Object{
				Fields: []*Field{
					{Name: []byte("__typename"), Value: &String{Path: []string{"__typename"}}},
					{Name: []byte("id"), Value: &String{Path: []string{"id"}}},
				},
			}),
		}

		userProvidesData := &Object{
			Fields: []*Field{
				{Name: []byte("id"), Value: &Scalar{Path: []string{"id"}, Nullable: false}},
				{Name: []byte("name"), Value: &Scalar{Path: []string{"name"}, Nullable: false}},
			},
		}

		response := &GraphQLResponse{
			Info: &GraphQLResponseInfo{
				OperationType: ast.OperationTypeQuery,
			},
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
							{Data: []byte(`{"method":"POST"}`), SegmentType: StaticSegmentType},
						},
					},
					DataSourceIdentifier: []byte("graphql_datasource.Source"),
				}, "query"),

				// Entity fetch 1: resolves u1 at firstUser path
				// Response includes nested u3 as bestFriend, but only u1 is cached in L1
				SingleWithPath(&BatchEntityFetch{
					Input: BatchInput{
						Header: InputTemplate{
							Segments: []TemplateSegment{
								{Data: []byte(`{"method":"POST","body":{"query":"first fetch","variables":{"representations":[`), SegmentType: StaticSegmentType},
							},
						},
						Items: []InputTemplate{
							{
								Segments: []TemplateSegment{
									{
										SegmentType:  VariableSegmentType,
										VariableKind: ResolvableObjectVariableKind,
										Renderer: NewGraphQLVariableResolveRenderer(&Object{
											Fields: []*Field{
												{Name: []byte("__typename"), Value: &String{Path: []string{"__typename"}}},
												{Name: []byte("id"), Value: &String{Path: []string{"id"}}},
											},
										}),
									},
								},
							},
						},
						Separator: InputTemplate{
							Segments: []TemplateSegment{{Data: []byte(`,`), SegmentType: StaticSegmentType}},
						},
						Footer: InputTemplate{
							Segments: []TemplateSegment{{Data: []byte(`]}}}`), SegmentType: StaticSegmentType}},
						},
					},
					DataSource: entityDS1,
					PostProcessing: PostProcessingConfiguration{
						SelectResponseDataPath: []string{"data", "_entities"},
					},
					Info: &FetchInfo{
						DataSourceID:   "users",
						DataSourceName: "users",
						OperationType:  ast.OperationTypeQuery,
						ProvidesData:   userProvidesData,
					},
					Caching: FetchCacheConfiguration{
						Enabled:          true,
						CacheName:        "default",
						TTL:              30 * time.Second,
						CacheKeyTemplate: userCacheKeyTemplate,
						UseL1Cache:       true,
					},
					DataSourceIdentifier: []byte("graphql_datasource.Source"),
				}, "query.firstUser", ObjectPath("firstUser")),

				// Entity fetch 2: resolves u3 at secondUser path
				// u3 appeared as nested entity in fetch 1's response but is NOT in L1
				SingleWithPath(&BatchEntityFetch{
					Input: BatchInput{
						Header: InputTemplate{
							Segments: []TemplateSegment{
								{Data: []byte(`{"method":"POST","body":{"query":"second fetch","variables":{"representations":[`), SegmentType: StaticSegmentType},
							},
						},
						Items: []InputTemplate{
							{
								Segments: []TemplateSegment{
									{
										SegmentType:  VariableSegmentType,
										VariableKind: ResolvableObjectVariableKind,
										Renderer: NewGraphQLVariableResolveRenderer(&Object{
											Fields: []*Field{
												{Name: []byte("__typename"), Value: &String{Path: []string{"__typename"}}},
												{Name: []byte("id"), Value: &String{Path: []string{"id"}}},
											},
										}),
									},
								},
							},
						},
						Separator: InputTemplate{
							Segments: []TemplateSegment{{Data: []byte(`,`), SegmentType: StaticSegmentType}},
						},
						Footer: InputTemplate{
							Segments: []TemplateSegment{{Data: []byte(`]}}}`), SegmentType: StaticSegmentType}},
						},
					},
					DataSource: entityDS2,
					PostProcessing: PostProcessingConfiguration{
						SelectResponseDataPath: []string{"data", "_entities"},
					},
					Info: &FetchInfo{
						DataSourceID:   "users",
						DataSourceName: "users",
						OperationType:  ast.OperationTypeQuery,
						ProvidesData:   userProvidesData,
					},
					Caching: FetchCacheConfiguration{
						Enabled:          true,
						CacheName:        "default",
						TTL:              30 * time.Second,
						CacheKeyTemplate: userCacheKeyTemplate,
						UseL1Cache:       true,
					},
					DataSourceIdentifier: []byte("graphql_datasource.Source"),
				}, "query.secondUser", ObjectPath("secondUser")),
			),
			Data: &Object{
				Fields: []*Field{
					{
						Name: []byte("firstUser"),
						Value: &Object{
							Path: []string{"firstUser"},
							Fields: []*Field{
								{Name: []byte("id"), Value: &String{Path: []string{"id"}}},
								{Name: []byte("name"), Value: &String{Path: []string{"name"}}},
								{
									Name: []byte("bestFriend"),
									Value: &Object{
										Path: []string{"bestFriend"},
										Fields: []*Field{
											{Name: []byte("id"), Value: &String{Path: []string{"id"}}},
											{Name: []byte("name"), Value: &String{Path: []string{"name"}}},
										},
									},
								},
							},
						},
					},
					{
						Name: []byte("secondUser"),
						Value: &Object{
							Path: []string{"secondUser"},
							Fields: []*Field{
								{Name: []byte("id"), Value: &String{Path: []string{"id"}}},
								{Name: []byte("name"), Value: &String{Path: []string{"name"}}},
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
		ctx.ExecutionOptions.Caching.EnableL2Cache = false

		ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
		resolvable := NewResolvable(ar, ResolvableOptions{})
		err := resolvable.Init(ctx, nil, ast.OperationTypeQuery)
		require.NoError(t, err)

		err = loader.LoadGraphQLResponseData(ctx, response, resolvable)
		require.NoError(t, err)

		out := fastjsonext.PrintGraphQLResponse(resolvable.data, resolvable.errors)
		expectedOutput := `{"data":{"firstUser":{"__typename":"User","id":"u1","name":"Alice","bestFriend":{"__typename":"User","id":"u3","name":"Charlie"}},"secondUser":{"__typename":"User","id":"u3","name":"Charlie"}}}`
		assert.Equal(t, expectedOutput, out)

		// gomock verifies: entityDS1.Times(1) and entityDS2.Times(1)
		// entityDS2 being called proves u3 (nested in fetch 1's response) was NOT cached in L1
	})
}

func TestL1CacheUseL1CacheFlagDisabled(t *testing.T) {
	t.Run("UseL1Cache=false bypasses L1 even when globally enabled", func(t *testing.T) {
		// This test verifies that when UseL1Cache=false is set on a fetch,
		// the L1 cache is bypassed even though L1 is globally enabled.
		// This is the behavior set by the optimizeL1Cache postprocessor when
		// a fetch cannot benefit from L1 caching.
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		// Root datasource
		rootDS := NewMockDataSource(ctrl)
		rootDS.EXPECT().
			Load(gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, headers any, input []byte) ([]byte, error) {
				return []byte(`{"data":{"product":{"__typename":"Product","id":"prod-1"}}}`), nil
			}).Times(1)

		// Entity fetch - should be called TWICE because UseL1Cache=false
		// even though L1 is globally enabled
		entityDS := NewMockDataSource(ctrl)
		entityDS.EXPECT().
			Load(gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, headers any, input []byte) ([]byte, error) {
				return []byte(`{"data":{"_entities":[{"__typename":"Product","id":"prod-1","name":"Product One"}]}}`), nil
			}).Times(2) // Called twice because UseL1Cache=false bypasses L1

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
			Info: &GraphQLResponseInfo{
				OperationType: ast.OperationTypeQuery,
			},
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
							{Data: []byte(`{"method":"POST"}`), SegmentType: StaticSegmentType},
						},
					},
					DataSourceIdentifier: []byte("graphql_datasource.Source"),
				}, "query"),

				// First entity fetch - UseL1Cache=false
				SingleWithPath(&SingleFetch{
					FetchConfiguration: FetchConfiguration{
						DataSource: entityDS,
						PostProcessing: PostProcessingConfiguration{
							SelectResponseDataPath: []string{"data", "_entities", "0"},
						},
						Caching: FetchCacheConfiguration{
							Enabled:          true,
							CacheName:        "default",
							TTL:              30 * time.Second,
							CacheKeyTemplate: productCacheKeyTemplate,
							UseL1Cache:       false, // Explicitly disabled
						},
					},
					InputTemplate: InputTemplate{
						Segments: []TemplateSegment{
							{Data: []byte(`{"method":"POST"}`), SegmentType: StaticSegmentType},
						},
					},
					Info: &FetchInfo{
						DataSourceID:   "products",
						DataSourceName: "products",
						OperationType:  ast.OperationTypeQuery,
						ProvidesData:   providesData,
					},
					DataSourceIdentifier: []byte("graphql_datasource.Source"),
				}, "query.product", ObjectPath("product")),

				// Second entity fetch - UseL1Cache=false, should NOT hit L1
				SingleWithPath(&SingleFetch{
					FetchConfiguration: FetchConfiguration{
						DataSource: entityDS,
						PostProcessing: PostProcessingConfiguration{
							SelectResponseDataPath: []string{"data", "_entities", "0"},
						},
						Caching: FetchCacheConfiguration{
							Enabled:          true,
							CacheName:        "default",
							TTL:              30 * time.Second,
							CacheKeyTemplate: productCacheKeyTemplate,
							UseL1Cache:       false, // Explicitly disabled
						},
					},
					InputTemplate: InputTemplate{
						Segments: []TemplateSegment{
							{Data: []byte(`{"method":"POST"}`), SegmentType: StaticSegmentType},
						},
					},
					Info: &FetchInfo{
						DataSourceID:   "products",
						DataSourceName: "products",
						OperationType:  ast.OperationTypeQuery,
						ProvidesData:   providesData,
					},
					DataSourceIdentifier: []byte("graphql_datasource.Source"),
				}, "query.product", ObjectPath("product")),
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
							},
						},
					},
				},
			},
		}

		loader := &Loader{}

		ctx := NewContext(context.Background())
		ctx.ExecutionOptions.DisableSubgraphRequestDeduplication = true
		ctx.ExecutionOptions.Caching.EnableL1Cache = true // L1 globally ENABLED
		ctx.ExecutionOptions.Caching.EnableCacheAnalytics = true

		ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
		resolvable := NewResolvable(ar, ResolvableOptions{})
		err := resolvable.Init(ctx, nil, ast.OperationTypeQuery)
		require.NoError(t, err)

		err = loader.LoadGraphQLResponseData(ctx, response, resolvable)
		require.NoError(t, err)

		out := fastjsonext.PrintGraphQLResponse(resolvable.data, resolvable.errors)
		assert.Equal(t, `{"data":{"product":{"__typename":"Product","id":"prod-1","name":"Product One"}}}`, out)

		// Verify L1 cache stats show no hits (both fetches went to subgraph)
		stats := ctx.GetCacheStats()
		assert.Equal(t, 0, len(stats.L1Reads), "should have 0 L1 reads when UseL1Cache=false")
	})
}

// TestL1CacheEntityFieldArguments documents that entity cache keys are based solely on @key fields.
// Field arguments (e.g., friends(first:5) vs friends(first:20)) are invisible to the cache key.
// When the same entity is fetched twice in a request with different field arguments but the same
// ProvidesData field name, the L1 cache serves stale data from the first fetch.
func TestL1CacheEntityFieldArguments(t *testing.T) {
	// userCacheKeyTemplate creates the standard User @key(fields: "id") cache key template.
	// This produces cache keys like: {"__typename":"User","key":{"id":"user-1"}}
	// Field arguments are NOT included in this key.
	userCacheKeyTemplate := func() *EntityQueryCacheKeyTemplate {
		return &EntityQueryCacheKeyTemplate{
			Keys: NewResolvableObjectVariable(&Object{
				Fields: []*Field{
					{Name: []byte("__typename"), Value: &String{Path: []string{"__typename"}}},
					{Name: []byte("id"), Value: &String{Path: []string{"id"}}},
				},
			}),
		}
	}

	// buildL1CollisionResponse builds a GraphQLResponse with two sequential entity fetches
	// for the same User entity. Both fetches share the same cache key template and ProvidesData
	// field name, but return different data (simulating different field arguments).
	// The second fetch's datasource should have Times(0) because L1 will serve cached data.
	buildL1CollisionResponse := func(
		rootDS, entityDS1, entityDS2 DataSource,
		fieldName string,
		fieldValue Node,
	) *GraphQLResponse {
		cacheKeyTmpl := userCacheKeyTemplate()
		// ProvidesData uses Scalar for all non-key fields, matching real-world planner behavior.
		// The planner always generates Scalar nodes in ProvidesData regardless of actual field type.
		// Using Array/Object here would cause validateItemHasRequiredData() to do deep validation
		// that may fail, preventing L1 cache hits.
		providesData := &Object{
			Fields: []*Field{
				{Name: []byte("id"), Value: &Scalar{Path: []string{"id"}, Nullable: false}},
				{Name: []byte(fieldName), Value: &Scalar{Path: []string{fieldName}, Nullable: true}},
			},
		}

		return &GraphQLResponse{
			Info: &GraphQLResponseInfo{OperationType: ast.OperationTypeQuery},
			Fetches: Sequence(
				// Root fetch: returns user reference
				SingleWithPath(&SingleFetch{
					FetchConfiguration: FetchConfiguration{
						DataSource:     rootDS,
						PostProcessing: PostProcessingConfiguration{SelectResponseDataPath: []string{"data"}},
					},
					InputTemplate:        InputTemplate{Segments: []TemplateSegment{{Data: []byte(`{"method":"POST","url":"http://root.service","body":{"query":"{user {__typename id}}"}}`), SegmentType: StaticSegmentType}}},
					DataSourceIdentifier: []byte("graphql_datasource.Source"),
				}, "query"),

				// First entity fetch: e.g. friends(first:5) — populates L1 cache
				SingleWithPath(&SingleFetch{
					FetchConfiguration: FetchConfiguration{
						DataSource:     entityDS1,
						PostProcessing: PostProcessingConfiguration{SelectResponseDataPath: []string{"data", "_entities", "0"}},
						Caching: FetchCacheConfiguration{
							Enabled:          true,
							CacheName:        "default",
							TTL:              30 * time.Second,
							CacheKeyTemplate: cacheKeyTmpl,
							UseL1Cache:       true,
						},
					},
					InputTemplate: InputTemplate{Segments: []TemplateSegment{
						{Data: []byte(`{"method":"POST","url":"http://accounts.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {id ` + fieldName + `}}}","variables":{"representations":[`), SegmentType: StaticSegmentType},
						{SegmentType: VariableSegmentType, VariableKind: ResolvableObjectVariableKind, Renderer: NewGraphQLVariableResolveRenderer(&Object{
							Fields: []*Field{
								{Name: []byte("__typename"), Value: &String{Path: []string{"__typename"}}},
								{Name: []byte("id"), Value: &String{Path: []string{"id"}}},
							},
						})},
						{Data: []byte(`]}}}`), SegmentType: StaticSegmentType},
					}},
					Info: &FetchInfo{
						DataSourceID:   "accounts",
						DataSourceName: "accounts",
						OperationType:  ast.OperationTypeQuery,
						ProvidesData:   providesData,
					},
					DataSourceIdentifier: []byte("graphql_datasource.Source"),
				}, "query.user", ObjectPath("user")),

				// Second entity fetch: e.g. friends(first:20) — L1 hit, datasource NOT called
				SingleWithPath(&SingleFetch{
					FetchConfiguration: FetchConfiguration{
						DataSource:     entityDS2,
						PostProcessing: PostProcessingConfiguration{SelectResponseDataPath: []string{"data", "_entities", "0"}},
						Caching: FetchCacheConfiguration{
							Enabled:          true,
							CacheName:        "default",
							TTL:              30 * time.Second,
							CacheKeyTemplate: cacheKeyTmpl,
							UseL1Cache:       true,
						},
					},
					InputTemplate: InputTemplate{Segments: []TemplateSegment{
						{Data: []byte(`{"method":"POST","url":"http://accounts.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {id ` + fieldName + `}}}","variables":{"representations":[`), SegmentType: StaticSegmentType},
						{SegmentType: VariableSegmentType, VariableKind: ResolvableObjectVariableKind, Renderer: NewGraphQLVariableResolveRenderer(&Object{
							Fields: []*Field{
								{Name: []byte("__typename"), Value: &String{Path: []string{"__typename"}}},
								{Name: []byte("id"), Value: &String{Path: []string{"id"}}},
							},
						})},
						{Data: []byte(`]}}}`), SegmentType: StaticSegmentType},
					}},
					Info: &FetchInfo{
						DataSourceID:   "accounts",
						DataSourceName: "accounts",
						OperationType:  ast.OperationTypeQuery,
						ProvidesData:   providesData,
					},
					DataSourceIdentifier: []byte("graphql_datasource.Source"),
				}, "query.user", ObjectPath("user")),
			),
			Data: &Object{
				Fields: []*Field{
					{
						Name: []byte("user"),
						Value: &Object{
							Path: []string{"user"},
							Fields: []*Field{
								{Name: []byte("id"), Value: &String{Path: []string{"id"}}},
								{Name: []byte(fieldName), Value: fieldValue},
							},
						},
					},
				},
			},
		}
	}

	// runL1CollisionTest executes a test that documents L1 cache collision.
	runL1CollisionTest := func(t *testing.T, response *GraphQLResponse, expectedOutput string) {
		t.Helper()

		loader := &Loader{}
		ctx := NewContext(context.Background())
		ctx.ExecutionOptions.DisableSubgraphRequestDeduplication = true
		ctx.ExecutionOptions.Caching.EnableL1Cache = true

		ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
		resolvable := NewResolvable(ar, ResolvableOptions{})
		err := resolvable.Init(ctx, nil, ast.OperationTypeQuery)
		require.NoError(t, err)

		err = loader.LoadGraphQLResponseData(ctx, response, resolvable)
		require.NoError(t, err)

		out := fastjsonext.PrintGraphQLResponse(resolvable.data, resolvable.errors)
		assert.Equal(t, expectedOutput, out)
	}

	t.Run("same entity different Int arg - pagination collision", func(t *testing.T) {
		// Scenario: query { user(id: "user-1") { friends(first: 5) { id } } }
		// followed by a second fetch for the same user with friends(first: 20).
		// The L1 cache key is {"__typename":"User","key":{"id":"user-1"}} for both.
		// ProvidesData has "friends" field in both cases, so validation passes.
		// Result: second fetch gets 5 friends instead of 20 (stale data).

		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		rootDS := NewMockDataSource(ctrl)
		rootDS.EXPECT().Load(gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, headers any, input []byte) ([]byte, error) {
				return []byte(`{"data":{"user":{"__typename":"User","id":"user-1"}}}`), nil
			}).Times(1)

		// friends(first: 5) returns 5 items
		entityDS1 := NewMockDataSource(ctrl)
		entityDS1.EXPECT().Load(gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, headers any, input []byte) ([]byte, error) {
				return []byte(`{"data":{"_entities":[{"__typename":"User","id":"user-1","friends":[{"id":"f1"},{"id":"f2"},{"id":"f3"},{"id":"f4"},{"id":"f5"}]}]}}`), nil
			}).Times(1)

		// friends(first: 20) — never called because L1 cache hit
		entityDS2 := NewMockDataSource(ctrl)
		entityDS2.EXPECT().Load(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)

		friendsValue := &Array{Path: []string{"friends"}, Item: &Object{
			Fields: []*Field{{Name: []byte("id"), Value: &String{Path: []string{"id"}}}},
		}}

		response := buildL1CollisionResponse(rootDS, entityDS1, entityDS2, "friends", friendsValue)

		// Output shows 5 friends (from first fetch) — stale for the second fetch's perspective
		runL1CollisionTest(t, response, `{"data":{"user":{"__typename":"User","id":"user-1","friends":[{"id":"f1"},{"id":"f2"},{"id":"f3"},{"id":"f4"},{"id":"f5"}]}}}`)
	})

	t.Run("same entity different String arg - search collision", func(t *testing.T) {
		// Scenario: user.search(term: "foo") vs user.search(term: "bar")
		// Same entity key, same field name "search" → L1 hit with wrong results.

		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		rootDS := NewMockDataSource(ctrl)
		rootDS.EXPECT().Load(gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, headers any, input []byte) ([]byte, error) {
				return []byte(`{"data":{"user":{"__typename":"User","id":"user-1"}}}`), nil
			}).Times(1)

		// search(term: "foo") returns foo results
		entityDS1 := NewMockDataSource(ctrl)
		entityDS1.EXPECT().Load(gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, headers any, input []byte) ([]byte, error) {
				return []byte(`{"data":{"_entities":[{"__typename":"User","id":"user-1","search":[{"title":"Foo Result 1"},{"title":"Foo Result 2"}]}]}}`), nil
			}).Times(1)

		// search(term: "bar") — never called
		entityDS2 := NewMockDataSource(ctrl)
		entityDS2.EXPECT().Load(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)

		searchValue := &Array{Path: []string{"search"}, Item: &Object{
			Fields: []*Field{{Name: []byte("title"), Value: &String{Path: []string{"title"}}}},
		}}

		response := buildL1CollisionResponse(rootDS, entityDS1, entityDS2, "search", searchValue)

		// Output shows foo results — stale when second fetch wanted "bar" results
		runL1CollisionTest(t, response, `{"data":{"user":{"__typename":"User","id":"user-1","search":[{"title":"Foo Result 1"},{"title":"Foo Result 2"}]}}}`)
	})

	t.Run("same entity different Enum arg - status collision", func(t *testing.T) {
		// Scenario: user.friendsByStatus(status: "ACTIVE") vs user.friendsByStatus(status: "BLOCKED")
		// Enum values are normalized to quoted strings in variables JSON.
		// Same entity key, same field name "friendsByStatus" → L1 hit with wrong results.

		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		rootDS := NewMockDataSource(ctrl)
		rootDS.EXPECT().Load(gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, headers any, input []byte) ([]byte, error) {
				return []byte(`{"data":{"user":{"__typename":"User","id":"user-1"}}}`), nil
			}).Times(1)

		// friendsByStatus(status: ACTIVE) returns active friends
		entityDS1 := NewMockDataSource(ctrl)
		entityDS1.EXPECT().Load(gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, headers any, input []byte) ([]byte, error) {
				return []byte(`{"data":{"_entities":[{"__typename":"User","id":"user-1","friendsByStatus":[{"id":"active-1","status":"ACTIVE"},{"id":"active-2","status":"ACTIVE"}]}]}}`), nil
			}).Times(1)

		// friendsByStatus(status: BLOCKED) — never called
		entityDS2 := NewMockDataSource(ctrl)
		entityDS2.EXPECT().Load(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)

		statusValue := &Array{Path: []string{"friendsByStatus"}, Item: &Object{
			Fields: []*Field{
				{Name: []byte("id"), Value: &String{Path: []string{"id"}}},
				{Name: []byte("status"), Value: &String{Path: []string{"status"}}},
			},
		}}

		response := buildL1CollisionResponse(rootDS, entityDS1, entityDS2, "friendsByStatus", statusValue)

		// Output shows ACTIVE friends — stale when second fetch wanted BLOCKED friends
		runL1CollisionTest(t, response, `{"data":{"user":{"__typename":"User","id":"user-1","friendsByStatus":[{"id":"active-1","status":"ACTIVE"},{"id":"active-2","status":"ACTIVE"}]}}}`)
	})

	t.Run("same entity different input object arg - filter collision", func(t *testing.T) {
		// Scenario: user.filtered(filter: {status:"ACTIVE",minAge:18}) vs
		//           user.filtered(filter: {status:"INACTIVE",minAge:30})
		// Input objects are serialized as JSON in variables.
		// Same entity key, same field name "filtered" → L1 hit with wrong results.

		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		rootDS := NewMockDataSource(ctrl)
		rootDS.EXPECT().Load(gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, headers any, input []byte) ([]byte, error) {
				return []byte(`{"data":{"user":{"__typename":"User","id":"user-1"}}}`), nil
			}).Times(1)

		// filtered(filter: {status:"ACTIVE",minAge:18})
		entityDS1 := NewMockDataSource(ctrl)
		entityDS1.EXPECT().Load(gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, headers any, input []byte) ([]byte, error) {
				return []byte(`{"data":{"_entities":[{"__typename":"User","id":"user-1","filtered":[{"id":"u1","age":25}]}]}}`), nil
			}).Times(1)

		// filtered(filter: {status:"INACTIVE",minAge:30}) — never called
		entityDS2 := NewMockDataSource(ctrl)
		entityDS2.EXPECT().Load(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)

		filteredValue := &Array{Path: []string{"filtered"}, Item: &Object{
			Fields: []*Field{
				{Name: []byte("id"), Value: &String{Path: []string{"id"}}},
				{Name: []byte("age"), Value: &Integer{Path: []string{"age"}}},
			},
		}}

		response := buildL1CollisionResponse(rootDS, entityDS1, entityDS2, "filtered", filteredValue)

		// Output shows filter 1 results — stale when second fetch used different filter
		runL1CollisionTest(t, response, `{"data":{"user":{"__typename":"User","id":"user-1","filtered":[{"id":"u1","age":25}]}}}`)
	})

	t.Run("same entity different list arg - tags collision", func(t *testing.T) {
		// Scenario: user.byTags(tags: ["a","b"]) vs user.byTags(tags: ["x","y","z"])
		// List arguments are serialized as JSON arrays in variables.
		// Same entity key, same field name "byTags" → L1 hit with wrong results.

		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		rootDS := NewMockDataSource(ctrl)
		rootDS.EXPECT().Load(gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, headers any, input []byte) ([]byte, error) {
				return []byte(`{"data":{"user":{"__typename":"User","id":"user-1"}}}`), nil
			}).Times(1)

		// byTags(tags: ["a","b"])
		entityDS1 := NewMockDataSource(ctrl)
		entityDS1.EXPECT().Load(gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, headers any, input []byte) ([]byte, error) {
				return []byte(`{"data":{"_entities":[{"__typename":"User","id":"user-1","byTags":[{"id":"t1","tag":"a"},{"id":"t2","tag":"b"}]}]}}`), nil
			}).Times(1)

		// byTags(tags: ["x","y","z"]) — never called
		entityDS2 := NewMockDataSource(ctrl)
		entityDS2.EXPECT().Load(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)

		tagsValue := &Array{Path: []string{"byTags"}, Item: &Object{
			Fields: []*Field{
				{Name: []byte("id"), Value: &String{Path: []string{"id"}}},
				{Name: []byte("tag"), Value: &String{Path: []string{"tag"}}},
			},
		}}

		response := buildL1CollisionResponse(rootDS, entityDS1, entityDS2, "byTags", tagsValue)

		// Output shows tags ["a","b"] results — stale when second fetch wanted ["x","y","z"]
		runL1CollisionTest(t, response, `{"data":{"user":{"__typename":"User","id":"user-1","byTags":[{"id":"t1","tag":"a"},{"id":"t2","tag":"b"}]}}}`)
	})

	t.Run("same entity multiple args different values - connection collision", func(t *testing.T) {
		// Scenario: user.connection(first:5,after:"cursor1") vs user.connection(first:10,after:"cursor2")
		// Multiple arguments all invisible to entity cache key.
		// Same entity key, same field name "connection" → L1 hit with wrong results.

		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		rootDS := NewMockDataSource(ctrl)
		rootDS.EXPECT().Load(gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, headers any, input []byte) ([]byte, error) {
				return []byte(`{"data":{"user":{"__typename":"User","id":"user-1"}}}`), nil
			}).Times(1)

		// connection(first:5, after:"cursor1")
		entityDS1 := NewMockDataSource(ctrl)
		entityDS1.EXPECT().Load(gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, headers any, input []byte) ([]byte, error) {
				return []byte(`{"data":{"_entities":[{"__typename":"User","id":"user-1","connection":{"edges":[{"node":"n1"},{"node":"n2"}],"pageInfo":{"hasNextPage":true}}}]}}`), nil
			}).Times(1)

		// connection(first:10, after:"cursor2") — never called
		entityDS2 := NewMockDataSource(ctrl)
		entityDS2.EXPECT().Load(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)

		connectionValue := &Object{Path: []string{"connection"}, Fields: []*Field{
			{Name: []byte("edges"), Value: &Array{Path: []string{"edges"}, Item: &Object{
				Fields: []*Field{{Name: []byte("node"), Value: &String{Path: []string{"node"}}}},
			}}},
			{Name: []byte("pageInfo"), Value: &Object{Path: []string{"pageInfo"}, Fields: []*Field{
				{Name: []byte("hasNextPage"), Value: &Boolean{Path: []string{"hasNextPage"}}},
			}}},
		}}

		response := buildL1CollisionResponse(rootDS, entityDS1, entityDS2, "connection", connectionValue)

		// Output shows first page — stale when second fetch wanted different page
		runL1CollisionTest(t, response, `{"data":{"user":{"__typename":"User","id":"user-1","connection":{"edges":[{"node":"n1"},{"node":"n2"}],"pageInfo":{"hasNextPage":true}}}}}`)
	})

	t.Run("default arg vs explicit arg same value - correct L1 hit", func(t *testing.T) {
		// Scenario: user.friends (using default first=5) followed by user.friends(first: 5) explicit.
		// Both produce the same subgraph query and same response data.
		// After normalization, default values are injected, so both fetches are equivalent.
		// The L1 cache hit here is CORRECT behavior — same args = same data.

		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		rootDS := NewMockDataSource(ctrl)
		rootDS.EXPECT().Load(gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, headers any, input []byte) ([]byte, error) {
				return []byte(`{"data":{"user":{"__typename":"User","id":"user-1"}}}`), nil
			}).Times(1)

		// friends (default first=5) — same data as friends(first: 5)
		entityDS1 := NewMockDataSource(ctrl)
		entityDS1.EXPECT().Load(gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, headers any, input []byte) ([]byte, error) {
				return []byte(`{"data":{"_entities":[{"__typename":"User","id":"user-1","friends":[{"id":"f1"},{"id":"f2"},{"id":"f3"},{"id":"f4"},{"id":"f5"}]}]}}`), nil
			}).Times(1)

		// friends(first: 5) explicit — not called, L1 hit is correct
		entityDS2 := NewMockDataSource(ctrl)
		entityDS2.EXPECT().Load(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)

		friendsValue := &Array{Path: []string{"friends"}, Item: &Object{
			Fields: []*Field{{Name: []byte("id"), Value: &String{Path: []string{"id"}}}},
		}}

		response := buildL1CollisionResponse(rootDS, entityDS1, entityDS2, "friends", friendsValue)

		// L1 hit is correct here: same default value = same data
		runL1CollisionTest(t, response, `{"data":{"user":{"__typename":"User","id":"user-1","friends":[{"id":"f1"},{"id":"f2"},{"id":"f3"},{"id":"f4"},{"id":"f5"}]}}}`)
	})

	t.Run("batch entity fetch - same entities different args", func(t *testing.T) {
		// Scenario: batch fetch for 3 User entities with friends(first:5),
		// followed by another batch fetch for the same 3 users with friends(first:20).
		// All 3 entities get L1 hits, returning stale data from the first fetch.

		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		rootDS := NewMockDataSource(ctrl)
		rootDS.EXPECT().Load(gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, headers any, input []byte) ([]byte, error) {
				return []byte(`{"data":{"users":[{"__typename":"User","id":"u1"},{"__typename":"User","id":"u2"},{"__typename":"User","id":"u3"}]}}`), nil
			}).Times(1)

		// Batch entity fetch 1: friends(first:5) for all 3 users
		batchDS1 := NewMockDataSource(ctrl)
		batchDS1.EXPECT().Load(gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, headers any, input []byte) ([]byte, error) {
				return []byte(`{"data":{"_entities":[{"__typename":"User","id":"u1","friends":[{"id":"f1"}]},{"__typename":"User","id":"u2","friends":[{"id":"f2"}]},{"__typename":"User","id":"u3","friends":[{"id":"f3"}]}]}}`), nil
			}).Times(1)

		// Batch entity fetch 2: friends(first:20) — never called, all L1 hits
		batchDS2 := NewMockDataSource(ctrl)
		batchDS2.EXPECT().Load(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)

		cacheKeyTmpl := userCacheKeyTemplate()
		providesData := &Object{
			Fields: []*Field{
				{Name: []byte("id"), Value: &Scalar{Path: []string{"id"}, Nullable: false}},
				{Name: []byte("friends"), Value: &Scalar{Path: []string{"friends"}, Nullable: true}},
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
					InputTemplate:        InputTemplate{Segments: []TemplateSegment{{Data: []byte(`{"method":"POST","url":"http://root.service","body":{"query":"{users {__typename id}}"}}`), SegmentType: StaticSegmentType}}},
					DataSourceIdentifier: []byte("graphql_datasource.Source"),
				}, "query"),

				// Batch fetch 1: friends(first:5)
				SingleWithPath(&BatchEntityFetch{
					Input: BatchInput{
						Header: InputTemplate{Segments: []TemplateSegment{
							{Data: []byte(`{"method":"POST","url":"http://accounts.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {id friends(first:5) {id}}}}","variables":{"representations":[`), SegmentType: StaticSegmentType},
						}},
						Items: []InputTemplate{{Segments: []TemplateSegment{
							{SegmentType: VariableSegmentType, VariableKind: ResolvableObjectVariableKind, Renderer: NewGraphQLVariableResolveRenderer(&Object{
								Fields: []*Field{
									{Name: []byte("__typename"), Value: &String{Path: []string{"__typename"}}},
									{Name: []byte("id"), Value: &String{Path: []string{"id"}}},
								},
							})},
						}}},
						Separator: InputTemplate{Segments: []TemplateSegment{{Data: []byte(`,`), SegmentType: StaticSegmentType}}},
						Footer:    InputTemplate{Segments: []TemplateSegment{{Data: []byte(`]}}}`), SegmentType: StaticSegmentType}}},
					},
					DataSource: batchDS1,
					PostProcessing: PostProcessingConfiguration{
						SelectResponseDataPath: []string{"data", "_entities"},
					},
					Caching: FetchCacheConfiguration{
						Enabled:          true,
						CacheName:        "default",
						TTL:              30 * time.Second,
						CacheKeyTemplate: cacheKeyTmpl,
						UseL1Cache:       true,
					},
					Info: &FetchInfo{
						DataSourceID:   "accounts",
						DataSourceName: "accounts",
						OperationType:  ast.OperationTypeQuery,
						ProvidesData:   providesData,
					},
					DataSourceIdentifier: []byte("graphql_datasource.Source"),
				}, "query.users", ArrayPath("users")),

				// Batch fetch 2: friends(first:20) — all L1 hits, never called
				SingleWithPath(&BatchEntityFetch{
					Input: BatchInput{
						Header: InputTemplate{Segments: []TemplateSegment{
							{Data: []byte(`{"method":"POST","url":"http://accounts.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {id friends(first:20) {id}}}}","variables":{"representations":[`), SegmentType: StaticSegmentType},
						}},
						Items: []InputTemplate{{Segments: []TemplateSegment{
							{SegmentType: VariableSegmentType, VariableKind: ResolvableObjectVariableKind, Renderer: NewGraphQLVariableResolveRenderer(&Object{
								Fields: []*Field{
									{Name: []byte("__typename"), Value: &String{Path: []string{"__typename"}}},
									{Name: []byte("id"), Value: &String{Path: []string{"id"}}},
								},
							})},
						}}},
						Separator: InputTemplate{Segments: []TemplateSegment{{Data: []byte(`,`), SegmentType: StaticSegmentType}}},
						Footer:    InputTemplate{Segments: []TemplateSegment{{Data: []byte(`]}}}`), SegmentType: StaticSegmentType}}},
					},
					DataSource: batchDS2,
					PostProcessing: PostProcessingConfiguration{
						SelectResponseDataPath: []string{"data", "_entities"},
					},
					Caching: FetchCacheConfiguration{
						Enabled:          true,
						CacheName:        "default",
						TTL:              30 * time.Second,
						CacheKeyTemplate: cacheKeyTmpl,
						UseL1Cache:       true,
					},
					Info: &FetchInfo{
						DataSourceID:   "accounts",
						DataSourceName: "accounts",
						OperationType:  ast.OperationTypeQuery,
						ProvidesData:   providesData,
					},
					DataSourceIdentifier: []byte("graphql_datasource.Source"),
				}, "query.users", ArrayPath("users")),
			),
			Data: &Object{
				Fields: []*Field{
					{
						Name: []byte("users"),
						Value: &Array{
							Path: []string{"users"},
							Item: &Object{
								Fields: []*Field{
									{Name: []byte("id"), Value: &String{Path: []string{"id"}}},
									{Name: []byte("friends"), Value: &Array{Path: []string{"friends"}, Item: &Object{
										Fields: []*Field{{Name: []byte("id"), Value: &String{Path: []string{"id"}}}},
									}}},
								},
							},
						},
					},
				},
			},
		}

		// All 3 users get stale data from batch fetch 1 (friends(first:5))
		runL1CollisionTest(t, response, `{"data":{"users":[{"__typename":"User","id":"u1","friends":[{"id":"f1"}]},{"__typename":"User","id":"u2","friends":[{"id":"f2"}]},{"__typename":"User","id":"u3","friends":[{"id":"f3"}]}]}}`)
	})
}

// TestCacheFieldName tests the cacheFieldName and computeArgSuffix methods directly.
// Verifies that different argument values produce different suffixed field names,
// and that fields without arguments return the plain SchemaFieldName.
func TestCacheFieldName(t *testing.T) {
	t.Run("no CacheArgs returns SchemaFieldName", func(t *testing.T) {
		ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
		ctx := NewContext(context.Background())
		loader := &Loader{ctx: ctx, jsonArena: ar}

		field := &Field{Name: []byte("friends")}
		assert.Equal(t, "friends", loader.cacheFieldName(field))
	})

	t.Run("with CacheArgs appends suffix", func(t *testing.T) {
		ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
		ctx := NewContext(context.Background())
		ctx.Variables = astjson.MustParseBytes([]byte(`{"a":5}`))
		loader := &Loader{ctx: ctx, jsonArena: ar}

		field := &Field{
			Name:      []byte("friends"),
			CacheArgs: []CacheFieldArg{{ArgName: "first", VariablePath: []string{"a"}}},
		}
		name := loader.cacheFieldName(field)
		assert.Equal(t, "friends", name[:7], "prefix should be field name")
		assert.Equal(t, "_xxh", name[7:11], "suffix should start with _xxh")
		assert.Equal(t, 27, len(name), "friends(7) + _xxh(4) + 16 hex chars = 27")
	})

	t.Run("different arg values produce different suffixes", func(t *testing.T) {
		ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
		ctx := NewContext(context.Background())
		ctx.Variables = astjson.MustParseBytes([]byte(`{"a":5,"b":20}`))
		loader := &Loader{ctx: ctx, jsonArena: ar}

		field1 := &Field{
			Name:      []byte("friends"),
			CacheArgs: []CacheFieldArg{{ArgName: "first", VariablePath: []string{"a"}}},
		}
		field2 := &Field{
			Name:      []byte("friends"),
			CacheArgs: []CacheFieldArg{{ArgName: "first", VariablePath: []string{"b"}}},
		}
		name1 := loader.cacheFieldName(field1)
		name2 := loader.cacheFieldName(field2)
		assert.NotEqual(t, name1, name2, "different arg values should produce different suffixes")
	})

	t.Run("same arg values produce same suffix", func(t *testing.T) {
		ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
		ctx := NewContext(context.Background())
		ctx.Variables = astjson.MustParseBytes([]byte(`{"a":5}`))
		loader := &Loader{ctx: ctx, jsonArena: ar}

		field1 := &Field{
			Name:      []byte("friends"),
			CacheArgs: []CacheFieldArg{{ArgName: "first", VariablePath: []string{"a"}}},
		}
		field2 := &Field{
			Name:      []byte("friends"),
			CacheArgs: []CacheFieldArg{{ArgName: "first", VariablePath: []string{"a"}}},
		}
		assert.Equal(t, loader.cacheFieldName(field1), loader.cacheFieldName(field2))
	})

	t.Run("multiple args produce deterministic suffix regardless of order", func(t *testing.T) {
		ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
		ctx := NewContext(context.Background())
		ctx.Variables = astjson.MustParseBytes([]byte(`{"a":"cursor1","b":5}`))
		loader := &Loader{ctx: ctx, jsonArena: ar}

		// Args in order: after, first
		field1 := &Field{
			Name: []byte("connection"),
			CacheArgs: []CacheFieldArg{
				{ArgName: "after", VariablePath: []string{"a"}},
				{ArgName: "first", VariablePath: []string{"b"}},
			},
		}
		// Args in reverse order: first, after
		field2 := &Field{
			Name: []byte("connection"),
			CacheArgs: []CacheFieldArg{
				{ArgName: "first", VariablePath: []string{"b"}},
				{ArgName: "after", VariablePath: []string{"a"}},
			},
		}
		assert.Equal(t, loader.cacheFieldName(field1), loader.cacheFieldName(field2),
			"arg order should not affect suffix — both are sorted by ArgName")
	})

	t.Run("with alias uses SchemaFieldName for suffix base", func(t *testing.T) {
		ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
		ctx := NewContext(context.Background())
		ctx.Variables = astjson.MustParseBytes([]byte(`{"a":5}`))
		loader := &Loader{ctx: ctx, jsonArena: ar}

		field := &Field{
			Name:         []byte("myFriends"), // alias
			OriginalName: []byte("friends"),   // schema name
			CacheArgs:    []CacheFieldArg{{ArgName: "first", VariablePath: []string{"a"}}},
		}
		name := loader.cacheFieldName(field)
		assert.True(t, len(name) > len("friends"), "should have suffix")
		assert.Equal(t, "friends", name[:7], "should use SchemaFieldName as base")
	})

	t.Run("RemapVariables applied to variable path", func(t *testing.T) {
		ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
		ctx := NewContext(context.Background())
		ctx.Variables = astjson.MustParseBytes([]byte(`{"remapped_a":5}`))
		ctx.RemapVariables = map[string]string{"a": "remapped_a"}
		loader := &Loader{ctx: ctx, jsonArena: ar}

		field := &Field{
			Name:      []byte("friends"),
			CacheArgs: []CacheFieldArg{{ArgName: "first", VariablePath: []string{"a"}}},
		}
		// Without RemapVariables, "a" wouldn't resolve. With it, "a" → "remapped_a" → 5
		name := loader.cacheFieldName(field)
		assert.True(t, len(name) > len("friends"), "should have suffix even with remapped variable")

		// Compare with direct path to remapped_a — should produce same suffix
		ctx2 := NewContext(context.Background())
		ctx2.Variables = astjson.MustParseBytes([]byte(`{"remapped_a":5}`))
		loader2 := &Loader{ctx: ctx2, jsonArena: ar}
		field2 := &Field{
			Name:      []byte("friends"),
			CacheArgs: []CacheFieldArg{{ArgName: "first", VariablePath: []string{"remapped_a"}}},
		}
		assert.Equal(t, name, loader2.cacheFieldName(field2))
	})
}

// TestValidateFieldDataWithCacheArgs tests that validateFieldData uses suffixed field names.
func TestValidateFieldDataWithCacheArgs(t *testing.T) {
	t.Run("validates against suffixed field name", func(t *testing.T) {
		ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
		ctx := NewContext(context.Background())
		ctx.Variables = astjson.MustParseBytes([]byte(`{"a":5}`))
		loader := &Loader{ctx: ctx, jsonArena: ar}

		field := &Field{
			Name:      []byte("friends"),
			CacheArgs: []CacheFieldArg{{ArgName: "first", VariablePath: []string{"a"}}},
			Value:     &Scalar{Path: []string{"friends"}, Nullable: true},
		}

		suffixedName := loader.cacheFieldName(field)

		// Item has the suffixed field name — should validate
		itemWithSuffix := astjson.MustParseBytes([]byte(`{"id":"1","` + suffixedName + `":[{"id":"f1"}]}`))
		assert.True(t, loader.validateFieldData(itemWithSuffix, field))

		// Item has the plain field name (no suffix) — should NOT validate
		itemPlain := astjson.MustParseBytes([]byte(`{"id":"1","friends":[{"id":"f1"}]}`))
		assert.False(t, loader.validateFieldData(itemPlain, field))
	})

	t.Run("different arg values miss on each other", func(t *testing.T) {
		ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
		ctx := NewContext(context.Background())
		ctx.Variables = astjson.MustParseBytes([]byte(`{"a":5,"b":20}`))
		loader := &Loader{ctx: ctx, jsonArena: ar}

		fieldA := &Field{
			Name:      []byte("friends"),
			CacheArgs: []CacheFieldArg{{ArgName: "first", VariablePath: []string{"a"}}},
			Value:     &Scalar{Path: []string{"friends"}, Nullable: true},
		}
		fieldB := &Field{
			Name:      []byte("friends"),
			CacheArgs: []CacheFieldArg{{ArgName: "first", VariablePath: []string{"b"}}},
			Value:     &Scalar{Path: []string{"friends"}, Nullable: true},
		}

		suffixA := loader.cacheFieldName(fieldA)
		suffixB := loader.cacheFieldName(fieldB)

		// Item has suffix for first=5
		item := astjson.MustParseBytes([]byte(`{"id":"1","` + suffixA + `":[{"id":"f1"}]}`))

		// fieldA (first=5) validates — suffix matches
		assert.True(t, loader.validateFieldData(item, fieldA))
		// fieldB (first=20) does NOT validate — different suffix
		assert.False(t, loader.validateFieldData(item, fieldB))

		// Item with BOTH suffixes — both validate
		itemBoth := astjson.MustParseBytes([]byte(`{"id":"1","` + suffixA + `":[{"id":"f1"}],"` + suffixB + `":[{"id":"g1"}]}`))
		assert.True(t, loader.validateFieldData(itemBoth, fieldA))
		assert.True(t, loader.validateFieldData(itemBoth, fieldB))
	})
}

// TestNormalizeAndDenormalizeWithCacheArgs tests that normalize/denormalize use suffixed field names.
func TestNormalizeAndDenormalizeWithCacheArgs(t *testing.T) {
	t.Run("normalize writes suffixed key, denormalize reads it back", func(t *testing.T) {
		ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
		ctx := NewContext(context.Background())
		ctx.Variables = astjson.MustParseBytes([]byte(`{"a":5}`))
		loader := &Loader{ctx: ctx, jsonArena: ar}

		cacheArgs := []CacheFieldArg{{ArgName: "first", VariablePath: []string{"a"}}}
		obj := &Object{
			HasAliases: true,
			Fields: []*Field{
				{Name: []byte("myFriends"), OriginalName: []byte("friends"), Value: &Scalar{}, CacheArgs: cacheArgs},
			},
		}

		// Input uses alias name
		input := astjson.MustParseBytes([]byte(`{"myFriends":[{"id":"f1"}]}`))

		// Normalize: alias → suffixed schema name
		normalized := loader.normalizeForCache(input, obj)
		suffixedName := loader.cacheFieldName(obj.Fields[0])

		// Verify the normalized JSON uses the suffixed name
		normalizedJSON := string(normalized.MarshalTo(nil))
		assert.Contains(t, normalizedJSON, suffixedName)
		assert.NotContains(t, normalizedJSON, "myFriends")

		// Denormalize: suffixed schema name → alias
		denormalized := loader.denormalizeFromCache(normalized, obj)
		denormalizedJSON := string(denormalized.MarshalTo(nil))
		assert.Contains(t, denormalizedJSON, "myFriends")
		assert.NotContains(t, denormalizedJSON, suffixedName)
	})
}

// TestMergeEntityFields tests that mergeEntityFields correctly accumulates fields.
func TestMergeEntityFields(t *testing.T) {
	t.Run("merge adds new fields without overwriting existing", func(t *testing.T) {
		ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
		loader := &Loader{jsonArena: ar}

		dst := astjson.MustParseBytes([]byte(`{"id":"1","friends_xxhAAAA":[{"id":"f1"}]}`))
		src := astjson.MustParseBytes([]byte(`{"id":"1","friends_xxhBBBB":[{"id":"g1"},{"id":"g2"}]}`))

		loader.mergeEntityFields(dst, src)

		result := string(dst.MarshalTo(nil))
		// Both suffixed fields should be present
		assert.Contains(t, result, `"friends_xxhAAAA"`)
		assert.Contains(t, result, `"friends_xxhBBBB"`)
		// Existing fields not overwritten
		assert.Contains(t, result, `"id":"1"`)
	})

	t.Run("merge does not overwrite existing fields", func(t *testing.T) {
		ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
		loader := &Loader{jsonArena: ar}

		dst := astjson.MustParseBytes([]byte(`{"id":"1","name":"original"}`))
		src := astjson.MustParseBytes([]byte(`{"id":"1","name":"overwritten","email":"test@test.com"}`))

		loader.mergeEntityFields(dst, src)

		result := string(dst.MarshalTo(nil))
		assert.Contains(t, result, `"name":"original"`, "existing field should not be overwritten")
		assert.Contains(t, result, `"email":"test@test.com"`, "new field should be added")
	})
}

func TestNormalizeForCache(t *testing.T) {
	t.Run("no aliases - fast path returns same value", func(t *testing.T) {
		ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
		loader := &Loader{
			jsonArena: ar,
		}

		obj := &Object{
			HasAliases: false,
			Fields: []*Field{
				{Name: []byte("username"), Value: &Scalar{}},
			},
		}

		item := mustParseJSON(ar, `{"username":"Alice"}`)
		result := loader.normalizeForCache(item, obj)

		// Fast path: should return the same pointer
		assert.Equal(t, item, result, "should return same pointer when no aliases")
	})

	t.Run("with aliases - normalizes to original names", func(t *testing.T) {
		ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
		loader := &Loader{
			jsonArena: ar,
		}

		obj := &Object{
			HasAliases: true,
			Fields: []*Field{
				{Name: []byte("userName"), OriginalName: []byte("username"), Value: &Scalar{}},
			},
		}

		item := mustParseJSON(ar, `{"userName":"Alice"}`)
		result := loader.normalizeForCache(item, obj)

		resultJSON := string(result.MarshalTo(nil))
		assert.Equal(t, `{"username":"Alice"}`, resultJSON, "should normalize alias to original name")
	})

	t.Run("mixed aliases and non-aliases", func(t *testing.T) {
		ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
		loader := &Loader{
			jsonArena: ar,
		}

		obj := &Object{
			HasAliases: true,
			Fields: []*Field{
				{Name: []byte("userName"), OriginalName: []byte("username"), Value: &Scalar{}},
				{Name: []byte("id"), Value: &Scalar{}},
			},
		}

		item := mustParseJSON(ar, `{"userName":"Alice","id":"123"}`)
		result := loader.normalizeForCache(item, obj)

		resultJSON := string(result.MarshalTo(nil))
		assert.Equal(t, `{"username":"Alice","id":"123"}`, resultJSON, "should normalize alias to original name and keep non-aliased fields")
	})

	t.Run("nested object with aliases", func(t *testing.T) {
		ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
		loader := &Loader{
			jsonArena: ar,
		}

		innerObj := &Object{
			HasAliases: true,
			Fields: []*Field{
				{Name: []byte("n"), OriginalName: []byte("name"), Value: &Scalar{}},
			},
		}
		obj := &Object{
			HasAliases: true,
			Fields: []*Field{
				{Name: []byte("p"), OriginalName: []byte("product"), Value: innerObj},
			},
		}

		item := mustParseJSON(ar, `{"p":{"n":"Widget"}}`)
		result := loader.normalizeForCache(item, obj)

		resultJSON := string(result.MarshalTo(nil))
		assert.Equal(t, `{"product":{"name":"Widget"}}`, resultJSON)
	})

	t.Run("preserves __typename", func(t *testing.T) {
		ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
		loader := &Loader{
			jsonArena: ar,
		}

		obj := &Object{
			HasAliases: true,
			Fields: []*Field{
				{Name: []byte("userName"), OriginalName: []byte("username"), Value: &Scalar{}},
			},
		}

		item := mustParseJSON(ar, `{"__typename":"User","userName":"Alice"}`)
		result := loader.normalizeForCache(item, obj)

		resultJSON := string(result.MarshalTo(nil))
		assert.Equal(t, `{"username":"Alice","__typename":"User"}`, resultJSON, "should normalize alias and preserve __typename")
	})
}

func TestDenormalizeFromCache(t *testing.T) {
	t.Run("no aliases - fast path returns same value", func(t *testing.T) {
		ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
		loader := &Loader{
			jsonArena: ar,
		}

		obj := &Object{
			HasAliases: false,
			Fields: []*Field{
				{Name: []byte("username"), Value: &Scalar{}},
			},
		}

		item := mustParseJSON(ar, `{"username":"Alice"}`)
		result := loader.denormalizeFromCache(item, obj)

		assert.Equal(t, item, result, "should return same pointer when no aliases")
	})

	t.Run("with aliases - converts original names to aliases", func(t *testing.T) {
		ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
		loader := &Loader{
			jsonArena: ar,
		}

		obj := &Object{
			HasAliases: true,
			Fields: []*Field{
				{Name: []byte("userName"), OriginalName: []byte("username"), Value: &Scalar{}},
			},
		}

		// Cache stores normalized data with original name "username"
		item := mustParseJSON(ar, `{"username":"Alice"}`)
		result := loader.denormalizeFromCache(item, obj)

		resultJSON := string(result.MarshalTo(nil))
		assert.Equal(t, `{"userName":"Alice"}`, resultJSON, "should convert original name to alias")
	})
}

func TestValidateFieldDataWithAliases(t *testing.T) {
	t.Run("validates using original name on normalized data", func(t *testing.T) {
		ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
		loader := &Loader{
			jsonArena: ar,
		}

		field := &Field{
			Name:         []byte("userName"),
			OriginalName: []byte("username"),
			Value:        &Scalar{},
		}

		// Cache data is normalized (uses original name "username")
		item := mustParseJSON(ar, `{"username":"Alice"}`)

		result := loader.validateFieldData(item, field)
		assert.True(t, result, "should validate using original name from normalized cache data")
	})

	t.Run("fails when original name missing from cached data", func(t *testing.T) {
		ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
		loader := &Loader{
			jsonArena: ar,
		}

		field := &Field{
			Name:         []byte("userName"),
			OriginalName: []byte("username"),
			Value:        &Scalar{},
		}

		// Cache data doesn't have "username"
		item := mustParseJSON(ar, `{"realName":"Alice"}`)

		result := loader.validateFieldData(item, field)
		assert.False(t, result, "should fail when original field name is missing from cache data")
	})
}

func TestShallowCopyWithAliases(t *testing.T) {
	t.Run("reads original name writes alias", func(t *testing.T) {
		ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
		loader := &Loader{
			jsonArena: ar,
		}

		obj := &Object{
			HasAliases: true,
			Fields: []*Field{
				{Name: []byte("userName"), OriginalName: []byte("username"), Value: &Scalar{}},
			},
		}

		// Cache stores data with original field name
		cached := mustParseJSON(ar, `{"username":"Alice"}`)
		result := loader.shallowCopyProvidedFields(cached, obj)

		resultJSON := string(result.MarshalTo(nil))
		assert.Equal(t, `{"userName":"Alice"}`, resultJSON,
			"should read 'username' from cache and write as 'userName' alias")
	})
}

func TestComputeHasAliases(t *testing.T) {
	t.Run("no aliases", func(t *testing.T) {
		obj := &Object{
			Fields: []*Field{
				{Name: []byte("id"), Value: &Scalar{}},
				{Name: []byte("name"), Value: &Scalar{}},
			},
		}
		result := ComputeHasAliases(obj)
		assert.False(t, result)
		assert.False(t, obj.HasAliases)
	})

	t.Run("direct alias", func(t *testing.T) {
		obj := &Object{
			Fields: []*Field{
				{Name: []byte("myId"), OriginalName: []byte("id"), Value: &Scalar{}},
			},
		}
		result := ComputeHasAliases(obj)
		assert.True(t, result)
		assert.True(t, obj.HasAliases)
	})

	t.Run("nested alias", func(t *testing.T) {
		innerObj := &Object{
			Fields: []*Field{
				{Name: []byte("n"), OriginalName: []byte("name"), Value: &Scalar{}},
			},
		}
		obj := &Object{
			Fields: []*Field{
				{Name: []byte("product"), Value: innerObj},
			},
		}
		result := ComputeHasAliases(obj)
		assert.True(t, result)
		assert.True(t, obj.HasAliases)
		assert.True(t, innerObj.HasAliases)
	})

	t.Run("alias in array item", func(t *testing.T) {
		innerObj := &Object{
			Fields: []*Field{
				{Name: []byte("n"), OriginalName: []byte("name"), Value: &Scalar{}},
			},
		}
		obj := &Object{
			Fields: []*Field{
				{Name: []byte("items"), Value: &Array{Item: innerObj}},
			},
		}
		result := ComputeHasAliases(obj)
		assert.True(t, result)
		assert.True(t, obj.HasAliases)
	})
}

func mustParseJSON(a arena.Arena, jsonStr string) *astjson.Value {
	v, err := astjson.ParseBytesWithArena(a, []byte(jsonStr))
	if err != nil {
		panic(err)
	}
	return v
}
