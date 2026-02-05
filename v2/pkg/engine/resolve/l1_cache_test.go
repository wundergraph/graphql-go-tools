package resolve

import (
	"context"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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
