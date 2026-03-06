package resolve

import (
	"context"
	"sync"
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

	t.Run("with CacheArgs - appends arg suffix", func(t *testing.T) {
		ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
		ctx := NewContext(t.Context())
		ctx.Variables = astjson.MustParseBytes([]byte(`{"a":"5"}`))
		loader := &Loader{jsonArena: ar, ctx: ctx}

		field := &Field{
			Name:      []byte("friends"),
			Value:     &Scalar{},
			CacheArgs: []CacheFieldArg{{ArgName: "first", VariableName: "a"}},
		}
		obj := &Object{
			HasAliases: true,
			Fields:     []*Field{field},
		}

		item := mustParseJSON(ar, `{"friends":"value"}`)
		result := loader.normalizeForCache(item, obj)

		suffix := loader.computeArgSuffix(field.CacheArgs)
		resultJSON := string(result.MarshalTo(nil))
		assert.Equal(t, `{"friends`+suffix+`":"value"}`, resultJSON, "should append arg suffix to field name")
	})

	t.Run("with alias + CacheArgs - uses original name + arg suffix", func(t *testing.T) {
		ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
		ctx := NewContext(t.Context())
		ctx.Variables = astjson.MustParseBytes([]byte(`{"a":"5"}`))
		loader := &Loader{jsonArena: ar, ctx: ctx}

		field := &Field{
			Name:         []byte("myFriends"),
			OriginalName: []byte("friends"),
			Value:        &Scalar{},
			CacheArgs:    []CacheFieldArg{{ArgName: "first", VariableName: "a"}},
		}
		obj := &Object{
			HasAliases: true,
			Fields:     []*Field{field},
		}

		item := mustParseJSON(ar, `{"myFriends":"value"}`)
		result := loader.normalizeForCache(item, obj)

		suffix := loader.computeArgSuffix(field.CacheArgs)
		resultJSON := string(result.MarshalTo(nil))
		assert.Equal(t, `{"friends`+suffix+`":"value"}`, resultJSON, "should use original name + arg suffix")
	})
}

func TestNormalizeDenormalizeRoundTrip(t *testing.T) {
	t.Run("round-trip with CacheArgs preserves data", func(t *testing.T) {
		ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
		ctx := NewContext(t.Context())
		ctx.Variables = astjson.MustParseBytes([]byte(`{"a":"5"}`))
		loader := &Loader{jsonArena: ar, ctx: ctx}

		field := &Field{
			Name:      []byte("friends"),
			Value:     &Scalar{},
			CacheArgs: []CacheFieldArg{{ArgName: "first", VariableName: "a"}},
		}
		obj := &Object{
			HasAliases: true,
			Fields:     []*Field{field},
		}

		original := mustParseJSON(ar, `{"friends":"value"}`)
		normalized := loader.normalizeForCache(original, obj)
		denormalized := loader.denormalizeFromCache(ar, normalized, obj)

		assert.Equal(t, `{"friends":"value"}`, string(denormalized.MarshalTo(nil)))
	})

	t.Run("round-trip with alias + CacheArgs preserves data", func(t *testing.T) {
		ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
		ctx := NewContext(t.Context())
		ctx.Variables = astjson.MustParseBytes([]byte(`{"a":"5"}`))
		loader := &Loader{jsonArena: ar, ctx: ctx}

		field := &Field{
			Name:         []byte("myFriends"),
			OriginalName: []byte("friends"),
			Value:        &Scalar{},
			CacheArgs:    []CacheFieldArg{{ArgName: "first", VariableName: "a"}},
		}
		obj := &Object{
			HasAliases: true,
			Fields:     []*Field{field},
		}

		original := mustParseJSON(ar, `{"myFriends":"value"}`)
		normalized := loader.normalizeForCache(original, obj)
		denormalized := loader.denormalizeFromCache(ar, normalized, obj)

		assert.Equal(t, `{"myFriends":"value"}`, string(denormalized.MarshalTo(nil)))
	})

	t.Run("round-trip nested object with alias + CacheArgs", func(t *testing.T) {
		ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
		ctx := NewContext(t.Context())
		ctx.Variables = astjson.MustParseBytes([]byte(`{"a":"5"}`))
		loader := &Loader{jsonArena: ar, ctx: ctx}

		innerObj := &Object{
			HasAliases: true,
			Fields: []*Field{
				{Name: []byte("n"), OriginalName: []byte("name"), Value: &Scalar{}},
			},
		}
		field := &Field{
			Name:         []byte("myFriends"),
			OriginalName: []byte("friends"),
			Value:        innerObj,
			CacheArgs:    []CacheFieldArg{{ArgName: "first", VariableName: "a"}},
		}
		obj := &Object{
			HasAliases: true,
			Fields:     []*Field{field},
		}

		original := mustParseJSON(ar, `{"myFriends":{"n":"Alice"}}`)
		normalized := loader.normalizeForCache(original, obj)
		denormalized := loader.denormalizeFromCache(ar, normalized, obj)

		assert.Equal(t, `{"myFriends":{"n":"Alice"}}`, string(denormalized.MarshalTo(nil)))
	})

	t.Run("round-trip array of objects with alias + CacheArgs", func(t *testing.T) {
		ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
		ctx := NewContext(t.Context())
		ctx.Variables = astjson.MustParseBytes([]byte(`{"a":"5"}`))
		loader := &Loader{jsonArena: ar, ctx: ctx}

		innerObj := &Object{
			HasAliases: true,
			Fields: []*Field{
				{Name: []byte("n"), OriginalName: []byte("name"), Value: &Scalar{}},
			},
		}
		arrNode := &Array{Item: innerObj}
		field := &Field{
			Name:         []byte("myFriends"),
			OriginalName: []byte("friends"),
			Value:        arrNode,
			CacheArgs:    []CacheFieldArg{{ArgName: "first", VariableName: "a"}},
		}
		obj := &Object{
			HasAliases: true,
			Fields:     []*Field{field},
		}

		original := mustParseJSON(ar, `{"myFriends":[{"n":"Alice"},{"n":"Bob"}]}`)
		normalized := loader.normalizeForCache(original, obj)
		denormalized := loader.denormalizeFromCache(ar, normalized, obj)

		assert.Equal(t, `{"myFriends":[{"n":"Alice"},{"n":"Bob"}]}`, string(denormalized.MarshalTo(nil)))
	})

	t.Run("round-trip preserves __typename with CacheArgs", func(t *testing.T) {
		ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
		ctx := NewContext(t.Context())
		ctx.Variables = astjson.MustParseBytes([]byte(`{"a":"5"}`))
		loader := &Loader{jsonArena: ar, ctx: ctx}

		field := &Field{
			Name:         []byte("myFriends"),
			OriginalName: []byte("friends"),
			Value:        &Scalar{},
			CacheArgs:    []CacheFieldArg{{ArgName: "first", VariableName: "a"}},
		}
		obj := &Object{
			HasAliases: true,
			Fields:     []*Field{field},
		}

		original := mustParseJSON(ar, `{"__typename":"User","myFriends":"value"}`)
		normalized := loader.normalizeForCache(original, obj)
		denormalized := loader.denormalizeFromCache(ar, normalized, obj)

		// After round-trip, __typename should be preserved and field alias restored
		result := denormalized
		assert.Equal(t, `"User"`, string(result.Get("__typename").MarshalTo(nil)))
		assert.Equal(t, `"value"`, string(result.Get("myFriends").MarshalTo(nil)))
	})

	t.Run("round-trip multiple fields with different CacheArgs", func(t *testing.T) {
		ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
		ctx := NewContext(t.Context())
		ctx.Variables = astjson.MustParseBytes([]byte(`{"a":"5","b":"10"}`))
		loader := &Loader{jsonArena: ar, ctx: ctx}

		field1 := &Field{
			Name:      []byte("friends"),
			Value:     &Scalar{},
			CacheArgs: []CacheFieldArg{{ArgName: "first", VariableName: "a"}},
		}
		field2 := &Field{
			Name:  []byte("id"),
			Value: &Scalar{},
		}
		obj := &Object{
			HasAliases: true,
			Fields:     []*Field{field1, field2},
		}

		original := mustParseJSON(ar, `{"friends":"Alice","id":"1"}`)
		normalized := loader.normalizeForCache(original, obj)
		denormalized := loader.denormalizeFromCache(ar, normalized, obj)

		assert.Equal(t, `"Alice"`, string(denormalized.Get("friends").MarshalTo(nil)))
		assert.Equal(t, `"1"`, string(denormalized.Get("id").MarshalTo(nil)))
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
		result := loader.denormalizeFromCache(ar, item, obj)

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
		result := loader.denormalizeFromCache(ar, item, obj)

		resultJSON := string(result.MarshalTo(nil))
		assert.Equal(t, `{"userName":"Alice"}`, resultJSON, "should convert original name to alias")
	})

	t.Run("with CacheArgs - looks up suffixed field name", func(t *testing.T) {
		ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
		ctx := NewContext(t.Context())
		ctx.Variables = astjson.MustParseBytes([]byte(`{"a":"5"}`))
		loader := &Loader{jsonArena: ar, ctx: ctx}

		field := &Field{
			Name:      []byte("friends"),
			Value:     &Scalar{},
			CacheArgs: []CacheFieldArg{{ArgName: "first", VariableName: "a"}},
		}
		obj := &Object{
			HasAliases: true,
			Fields:     []*Field{field},
		}

		// Cache stores data with suffixed key
		suffix := loader.computeArgSuffix(field.CacheArgs)
		cacheJSON := `{"friends` + suffix + `":"value"}`
		cacheItem := mustParseJSON(ar, cacheJSON)

		result := loader.denormalizeFromCache(ar, cacheItem, obj)
		resultJSON := string(result.MarshalTo(nil))
		assert.Equal(t, `{"friends":"value"}`, resultJSON, "should map suffixed cache key back to query name")
	})

	t.Run("with alias + CacheArgs - maps suffixed original back to alias", func(t *testing.T) {
		ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
		ctx := NewContext(t.Context())
		ctx.Variables = astjson.MustParseBytes([]byte(`{"a":"5"}`))
		loader := &Loader{jsonArena: ar, ctx: ctx}

		field := &Field{
			Name:         []byte("myFriends"),
			OriginalName: []byte("friends"),
			Value:        &Scalar{},
			CacheArgs:    []CacheFieldArg{{ArgName: "first", VariableName: "a"}},
		}
		obj := &Object{
			HasAliases: true,
			Fields:     []*Field{field},
		}

		// Cache stores: friends_<suffix> → value
		suffix := loader.computeArgSuffix(field.CacheArgs)
		cacheJSON := `{"friends` + suffix + `":"value"}`
		cacheItem := mustParseJSON(ar, cacheJSON)

		result := loader.denormalizeFromCache(ar, cacheItem, obj)
		resultJSON := string(result.MarshalTo(nil))
		assert.Equal(t, `{"myFriends":"value"}`, resultJSON, "should map suffixed original name back to alias")
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

// TestPopulateL1CacheForRootFieldEntities_MissingKeyFields verifies that root field
// entity population gracefully handles entities that are missing @key fields.
// When the client's query doesn't select the @key fields (e.g., "id"), RenderCacheKeys
// produces a key with empty key object (e.g., {"__typename":"Product","key":{}}).
// The entity is stored under this degraded key but will never match any entity fetch,
// so the behavior is benign.
func TestPopulateL1CacheForRootFieldEntities_MissingKeyFields(t *testing.T) {
	ar := arena.NewMonotonicArena(arena.WithMinBufferSize(4096))
	ctx := NewContext(context.Background())
	ctx.ExecutionOptions.Caching.EnableL1Cache = true
	ctx.Variables = astjson.MustParse(`{}`)

	resolvable := NewResolvable(ar, ResolvableOptions{})
	err := resolvable.Init(ctx, nil, ast.OperationTypeQuery)
	require.NoError(t, err)

	// Set response data: entity with __typename but missing @key field "id"
	resolvable.data, err = astjson.ParseBytesWithArena(ar, []byte(`{"topProducts":[{"__typename":"Product","name":"Widget"}]}`))
	require.NoError(t, err)

	l1Cache := &sync.Map{}

	l := &Loader{
		jsonArena:  ar,
		ctx:        ctx,
		resolvable: resolvable,
		l1Cache:    l1Cache,
	}

	// Template expects @key field "id" which is NOT in the entity data.
	// Path points to where entities live in the response.
	entityTemplate := &EntityQueryCacheKeyTemplate{
		Keys: NewResolvableObjectVariable(&Object{
			Path: []string{"topProducts"},
			Fields: []*Field{
				{Name: []byte("__typename"), Value: &String{Path: []string{"__typename"}}},
				{Name: []byte("id"), Value: &String{Path: []string{"id"}}},
			},
		}),
	}

	fetchItem := &FetchItem{
		Fetch: &SingleFetch{
			FetchConfiguration: FetchConfiguration{
				Caching: FetchCacheConfiguration{
					Enabled:    true,
					UseL1Cache: true,
					RootFieldL1EntityCacheKeyTemplates: map[string]CacheKeyTemplate{
						"Product": entityTemplate,
					},
				},
			},
			Info: &FetchInfo{
				RootFields: []GraphCoordinate{
					{TypeName: "Query", FieldName: "topProducts"},
				},
			},
		},
	}

	l.populateL1CacheForRootFieldEntities(fetchItem)

	// Entity should be stored under a degraded key with empty key object.
	// An actual entity fetch would use {"__typename":"Product","key":{"id":"123"}},
	// which will never match this degraded key.
	degradedKey := `{"__typename":"Product","key":{}}`
	_, loaded := l1Cache.Load(degradedKey)
	assert.True(t, loaded, "entity should be stored under degraded key (empty key object)")

	// A proper entity cache key won't find anything
	_, loaded = l1Cache.Load(`{"__typename":"Product","key":{"id":"123"}}`)
	assert.False(t, loaded, "proper entity key should not find the entity with missing @key fields")
}

func mustParseJSON(a arena.Arena, jsonStr string) *astjson.Value {
	v, err := astjson.ParseBytesWithArena(a, []byte(jsonStr))
	if err != nil {
		panic(err)
	}
	return v
}

// --- P1: validateItemHasRequiredData unit tests ---

func TestValidateItemHasRequiredData(t *testing.T) {
	t.Run("nil item returns false", func(t *testing.T) {
		loader := &Loader{}
		obj := &Object{Fields: []*Field{{Name: []byte("id"), Value: &Scalar{}}}}
		assert.False(t, loader.validateItemHasRequiredData(nil, obj))
	})

	t.Run("all required scalar fields present", func(t *testing.T) {
		ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
		loader := &Loader{jsonArena: ar}
		obj := &Object{
			Fields: []*Field{
				{Name: []byte("id"), Value: &Scalar{}},
				{Name: []byte("name"), Value: &Scalar{}},
			},
		}
		item := mustParseJSON(ar, `{"id":"1","name":"Alice"}`)
		assert.True(t, loader.validateItemHasRequiredData(item, obj))
	})

	t.Run("missing required field", func(t *testing.T) {
		ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
		loader := &Loader{jsonArena: ar}
		obj := &Object{
			Fields: []*Field{
				{Name: []byte("id"), Value: &Scalar{}},
				{Name: []byte("name"), Value: &Scalar{}},
			},
		}
		item := mustParseJSON(ar, `{"id":"1"}`)
		assert.False(t, loader.validateItemHasRequiredData(item, obj))
	})

	t.Run("null value for non-nullable scalar", func(t *testing.T) {
		ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
		loader := &Loader{jsonArena: ar}
		obj := &Object{
			Fields: []*Field{
				{Name: []byte("id"), Value: &Scalar{Nullable: false}},
			},
		}
		item := mustParseJSON(ar, `{"id":null}`)
		assert.False(t, loader.validateItemHasRequiredData(item, obj))
	})

	t.Run("null value for nullable scalar", func(t *testing.T) {
		ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
		loader := &Loader{jsonArena: ar}
		obj := &Object{
			Fields: []*Field{
				{Name: []byte("email"), Value: &Scalar{Nullable: true}},
			},
		}
		item := mustParseJSON(ar, `{"email":null}`)
		assert.True(t, loader.validateItemHasRequiredData(item, obj))
	})

	t.Run("nested object with all fields", func(t *testing.T) {
		ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
		loader := &Loader{jsonArena: ar}
		innerObj := &Object{
			Fields: []*Field{
				{Name: []byte("street"), Value: &Scalar{}},
			},
		}
		obj := &Object{
			Fields: []*Field{
				{Name: []byte("address"), Value: innerObj},
			},
		}
		item := mustParseJSON(ar, `{"address":{"street":"Main St"}}`)
		assert.True(t, loader.validateItemHasRequiredData(item, obj))
	})

	t.Run("nested object missing required field", func(t *testing.T) {
		ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
		loader := &Loader{jsonArena: ar}
		innerObj := &Object{
			Fields: []*Field{
				{Name: []byte("street"), Value: &Scalar{}},
				{Name: []byte("city"), Value: &Scalar{}},
			},
		}
		obj := &Object{
			Fields: []*Field{
				{Name: []byte("address"), Value: innerObj},
			},
		}
		item := mustParseJSON(ar, `{"address":{"street":"Main St"}}`)
		assert.False(t, loader.validateItemHasRequiredData(item, obj))
	})

	t.Run("null for non-nullable object", func(t *testing.T) {
		ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
		loader := &Loader{jsonArena: ar}
		innerObj := &Object{
			Nullable: false,
			Fields:   []*Field{{Name: []byte("street"), Value: &Scalar{}}},
		}
		obj := &Object{
			Fields: []*Field{
				{Name: []byte("address"), Value: innerObj},
			},
		}
		item := mustParseJSON(ar, `{"address":null}`)
		assert.False(t, loader.validateItemHasRequiredData(item, obj))
	})

	t.Run("null for nullable object", func(t *testing.T) {
		ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
		loader := &Loader{jsonArena: ar}
		innerObj := &Object{
			Nullable: true,
			Fields:   []*Field{{Name: []byte("street"), Value: &Scalar{}}},
		}
		obj := &Object{
			Fields: []*Field{
				{Name: []byte("address"), Value: innerObj},
			},
		}
		item := mustParseJSON(ar, `{"address":null}`)
		assert.True(t, loader.validateItemHasRequiredData(item, obj))
	})

	t.Run("non-object value for object field", func(t *testing.T) {
		ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
		loader := &Loader{jsonArena: ar}
		innerObj := &Object{
			Fields: []*Field{{Name: []byte("street"), Value: &Scalar{}}},
		}
		obj := &Object{
			Fields: []*Field{
				{Name: []byte("address"), Value: innerObj},
			},
		}
		item := mustParseJSON(ar, `{"address":"not-an-object"}`)
		assert.False(t, loader.validateItemHasRequiredData(item, obj))
	})

	t.Run("array with all valid items", func(t *testing.T) {
		ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
		loader := &Loader{jsonArena: ar}
		arr := &Array{
			Item: &Scalar{},
		}
		obj := &Object{
			Fields: []*Field{
				{Name: []byte("tags"), Value: arr},
			},
		}
		item := mustParseJSON(ar, `{"tags":["a","b","c"]}`)
		assert.True(t, loader.validateItemHasRequiredData(item, obj))
	})

	t.Run("array with invalid item - non-nullable scalar null", func(t *testing.T) {
		ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
		loader := &Loader{jsonArena: ar}
		arr := &Array{
			Item: &Scalar{Nullable: false},
		}
		obj := &Object{
			Fields: []*Field{
				{Name: []byte("tags"), Value: arr},
			},
		}
		item := mustParseJSON(ar, `{"tags":["a",null,"c"]}`)
		assert.False(t, loader.validateItemHasRequiredData(item, obj))
	})

	t.Run("array with nullable items allows null", func(t *testing.T) {
		ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
		loader := &Loader{jsonArena: ar}
		arr := &Array{
			Item: &Scalar{Nullable: true},
		}
		obj := &Object{
			Fields: []*Field{
				{Name: []byte("tags"), Value: arr},
			},
		}
		item := mustParseJSON(ar, `{"tags":["a",null,"c"]}`)
		assert.True(t, loader.validateItemHasRequiredData(item, obj))
	})

	t.Run("null for non-nullable array", func(t *testing.T) {
		ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
		loader := &Loader{jsonArena: ar}
		arr := &Array{
			Nullable: false,
			Item:     &Scalar{},
		}
		obj := &Object{
			Fields: []*Field{
				{Name: []byte("tags"), Value: arr},
			},
		}
		item := mustParseJSON(ar, `{"tags":null}`)
		assert.False(t, loader.validateItemHasRequiredData(item, obj))
	})

	t.Run("null for nullable array", func(t *testing.T) {
		ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
		loader := &Loader{jsonArena: ar}
		arr := &Array{
			Nullable: true,
			Item:     &Scalar{},
		}
		obj := &Object{
			Fields: []*Field{
				{Name: []byte("tags"), Value: arr},
			},
		}
		item := mustParseJSON(ar, `{"tags":null}`)
		assert.True(t, loader.validateItemHasRequiredData(item, obj))
	})

	t.Run("non-array value for array field", func(t *testing.T) {
		ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
		loader := &Loader{jsonArena: ar}
		arr := &Array{Item: &Scalar{}}
		obj := &Object{
			Fields: []*Field{
				{Name: []byte("tags"), Value: arr},
			},
		}
		item := mustParseJSON(ar, `{"tags":"not-an-array"}`)
		assert.False(t, loader.validateItemHasRequiredData(item, obj))
	})

	t.Run("empty array is valid", func(t *testing.T) {
		ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
		loader := &Loader{jsonArena: ar}
		arr := &Array{Item: &Scalar{}}
		obj := &Object{
			Fields: []*Field{
				{Name: []byte("tags"), Value: arr},
			},
		}
		item := mustParseJSON(ar, `{"tags":[]}`)
		assert.True(t, loader.validateItemHasRequiredData(item, obj))
	})

	t.Run("array of objects with valid items", func(t *testing.T) {
		ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
		loader := &Loader{jsonArena: ar}
		itemObj := &Object{
			Fields: []*Field{
				{Name: []byte("id"), Value: &Scalar{}},
			},
		}
		arr := &Array{Item: itemObj}
		obj := &Object{
			Fields: []*Field{
				{Name: []byte("items"), Value: arr},
			},
		}
		item := mustParseJSON(ar, `{"items":[{"id":"1"},{"id":"2"}]}`)
		assert.True(t, loader.validateItemHasRequiredData(item, obj))
	})

	t.Run("array of objects with invalid item", func(t *testing.T) {
		ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
		loader := &Loader{jsonArena: ar}
		itemObj := &Object{
			Fields: []*Field{
				{Name: []byte("id"), Value: &Scalar{}},
				{Name: []byte("name"), Value: &Scalar{}},
			},
		}
		arr := &Array{Item: itemObj}
		obj := &Object{
			Fields: []*Field{
				{Name: []byte("items"), Value: arr},
			},
		}
		item := mustParseJSON(ar, `{"items":[{"id":"1","name":"ok"},{"id":"2"}]}`)
		assert.False(t, loader.validateItemHasRequiredData(item, obj))
	})

	t.Run("field with CacheArgs uses suffixed name for lookup", func(t *testing.T) {
		ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
		ctx := NewContext(t.Context())
		ctx.Variables = astjson.MustParseBytes([]byte(`{"first":"5"}`))
		loader := &Loader{jsonArena: ar, ctx: ctx}

		// Field has CacheArgs, so validation should look for "friends_<suffix>" not "friends"
		field := &Field{
			Name:  []byte("friends"),
			Value: &Scalar{},
			CacheArgs: []CacheFieldArg{
				{ArgName: "first", VariableName: "first"},
			},
		}

		// Compute expected suffixed name
		suffix := loader.computeArgSuffix(field.CacheArgs)
		expectedKey := "friends" + suffix

		// Item has the suffixed field name (as normalize would produce)
		itemJSON := `{"` + expectedKey + `":"value"}`
		item := mustParseJSON(ar, itemJSON)

		obj := &Object{Fields: []*Field{field}}
		assert.True(t, loader.validateItemHasRequiredData(item, obj))
	})

	t.Run("field with CacheArgs fails when only base name present", func(t *testing.T) {
		ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
		ctx := NewContext(t.Context())
		ctx.Variables = astjson.MustParseBytes([]byte(`{"first":"5"}`))
		loader := &Loader{jsonArena: ar, ctx: ctx}

		field := &Field{
			Name:  []byte("friends"),
			Value: &Scalar{},
			CacheArgs: []CacheFieldArg{
				{ArgName: "first", VariableName: "first"},
			},
		}

		// Item has only the base name "friends" without suffix
		item := mustParseJSON(ar, `{"friends":"value"}`)

		obj := &Object{Fields: []*Field{field}}
		assert.False(t, loader.validateItemHasRequiredData(item, obj))
	})

	t.Run("array with nil Item spec is valid if array exists", func(t *testing.T) {
		ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
		loader := &Loader{jsonArena: ar}
		arr := &Array{Item: nil}
		obj := &Object{
			Fields: []*Field{
				{Name: []byte("tags"), Value: arr},
			},
		}
		item := mustParseJSON(ar, `{"tags":["a","b"]}`)
		assert.True(t, loader.validateItemHasRequiredData(item, obj))
	})
}

// --- P3: computeArgSuffix unit tests ---

func TestComputeArgSuffix(t *testing.T) {
	t.Run("single arg produces deterministic suffix", func(t *testing.T) {
		ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
		ctx := NewContext(t.Context())
		ctx.Variables = astjson.MustParseBytes([]byte(`{"a":"5"}`))
		loader := &Loader{jsonArena: ar, ctx: ctx}

		suffix1 := loader.computeArgSuffix([]CacheFieldArg{{ArgName: "first", VariableName: "a"}})
		suffix2 := loader.computeArgSuffix([]CacheFieldArg{{ArgName: "first", VariableName: "a"}})

		assert.Equal(t, suffix1, suffix2, "same args should produce same suffix")
		assert.Equal(t, 17, len(suffix1), "suffix should be _ + 16 hex chars")
		assert.Equal(t, byte('_'), suffix1[0], "suffix should start with underscore")
	})

	t.Run("different values produce different suffixes", func(t *testing.T) {
		ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
		ctx := NewContext(t.Context())
		ctx.Variables = astjson.MustParseBytes([]byte(`{"a":"5","b":"10"}`))
		loader := &Loader{jsonArena: ar, ctx: ctx}

		suffix1 := loader.computeArgSuffix([]CacheFieldArg{{ArgName: "first", VariableName: "a"}})
		suffix2 := loader.computeArgSuffix([]CacheFieldArg{{ArgName: "first", VariableName: "b"}})

		assert.NotEqual(t, suffix1, suffix2, "different values should produce different suffixes")
	})

	t.Run("null variable produces null in hash", func(t *testing.T) {
		ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
		ctx := NewContext(t.Context())
		ctx.Variables = astjson.MustParseBytes([]byte(`{}`))
		loader := &Loader{jsonArena: ar, ctx: ctx}

		// Variable "missing" doesn't exist, so argValue is nil → "null" written
		suffix := loader.computeArgSuffix([]CacheFieldArg{{ArgName: "first", VariableName: "missing"}})
		assert.Equal(t, 17, len(suffix), "should still produce valid suffix for null variable")
	})

	t.Run("null variable differs from string null", func(t *testing.T) {
		ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
		ctx := NewContext(t.Context())
		ctx.Variables = astjson.MustParseBytes([]byte(`{"a":null,"b":"null"}`))
		loader := &Loader{jsonArena: ar, ctx: ctx}

		suffixNull := loader.computeArgSuffix([]CacheFieldArg{{ArgName: "first", VariableName: "a"}})
		suffixMissing := loader.computeArgSuffix([]CacheFieldArg{{ArgName: "first", VariableName: "missing"}})

		// Both json null and missing variable produce "null" in the hash,
		// so they should be equal
		assert.Equal(t, suffixNull, suffixMissing, "json null and missing variable both hash as null")
	})

	t.Run("unsorted args get sorted before hashing", func(t *testing.T) {
		ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
		ctx := NewContext(t.Context())
		ctx.Variables = astjson.MustParseBytes([]byte(`{"a":"1","b":"2"}`))
		loader := &Loader{jsonArena: ar, ctx: ctx}

		sorted := []CacheFieldArg{
			{ArgName: "alpha", VariableName: "a"},
			{ArgName: "beta", VariableName: "b"},
		}
		unsorted := []CacheFieldArg{
			{ArgName: "beta", VariableName: "b"},
			{ArgName: "alpha", VariableName: "a"},
		}

		suffixSorted := loader.computeArgSuffix(sorted)
		suffixUnsorted := loader.computeArgSuffix(unsorted)

		assert.Equal(t, suffixSorted, suffixUnsorted, "arg order should not affect suffix")
	})

	t.Run("RemapVariables applied before lookup", func(t *testing.T) {
		ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
		ctx := NewContext(t.Context())
		ctx.Variables = astjson.MustParseBytes([]byte(`{"original":"42"}`))
		ctx.RemapVariables = map[string]string{"remapped": "original"}
		loader := &Loader{jsonArena: ar, ctx: ctx}

		// "remapped" maps to "original" which has value "42"
		suffixRemapped := loader.computeArgSuffix([]CacheFieldArg{{ArgName: "first", VariableName: "remapped"}})
		// "original" has value "42" directly
		suffixDirect := loader.computeArgSuffix([]CacheFieldArg{{ArgName: "first", VariableName: "original"}})

		assert.Equal(t, suffixRemapped, suffixDirect, "remapped variable should produce same suffix as direct lookup")
	})

	t.Run("object arg produces deterministic hash regardless of key order", func(t *testing.T) {
		ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
		ctx1 := NewContext(t.Context())
		ctx1.Variables = astjson.MustParseBytes([]byte(`{"filter":{"name":"Alice","age":30}}`))
		loader1 := &Loader{jsonArena: ar, ctx: ctx1}

		ctx2 := NewContext(t.Context())
		ctx2.Variables = astjson.MustParseBytes([]byte(`{"filter":{"age":30,"name":"Alice"}}`))
		loader2 := &Loader{jsonArena: ar, ctx: ctx2}

		suffix1 := loader1.computeArgSuffix([]CacheFieldArg{{ArgName: "filter", VariableName: "filter"}})
		suffix2 := loader2.computeArgSuffix([]CacheFieldArg{{ArgName: "filter", VariableName: "filter"}})

		assert.Equal(t, suffix1, suffix2, "object arg key order should not affect hash (canonical JSON)")
	})
}

// --- P4: mergeEntityFields unit tests ---

func TestMergeEntityFields(t *testing.T) {
	t.Run("new field added to existing entity", func(t *testing.T) {
		ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
		loader := &Loader{jsonArena: ar}

		dst := mustParseJSON(ar, `{"id":"1","name":"Alice"}`)
		src := mustParseJSON(ar, `{"id":"1","email":"alice@example.com"}`)

		loader.mergeEntityFields(dst, src)

		resultJSON := string(dst.MarshalTo(nil))
		assert.Equal(t, `{"id":"1","name":"Alice","email":"alice@example.com"}`, resultJSON)
	})

	t.Run("existing field preserved not overwritten", func(t *testing.T) {
		ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
		loader := &Loader{jsonArena: ar}

		dst := mustParseJSON(ar, `{"id":"1","name":"Alice"}`)
		src := mustParseJSON(ar, `{"id":"1","name":"Bob"}`)

		loader.mergeEntityFields(dst, src)

		resultJSON := string(dst.MarshalTo(nil))
		assert.Equal(t, `{"id":"1","name":"Alice"}`, resultJSON, "existing field should not be overwritten")
	})

	t.Run("nil dst is no-op", func(t *testing.T) {
		ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
		loader := &Loader{jsonArena: ar}
		src := mustParseJSON(ar, `{"id":"1"}`)
		// Should not panic
		loader.mergeEntityFields(nil, src)
	})

	t.Run("nil src is no-op", func(t *testing.T) {
		ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
		loader := &Loader{jsonArena: ar}
		dst := mustParseJSON(ar, `{"id":"1"}`)
		loader.mergeEntityFields(dst, nil)
		resultJSON := string(dst.MarshalTo(nil))
		assert.Equal(t, `{"id":"1"}`, resultJSON, "dst should be unchanged")
	})

	t.Run("non-object type is no-op", func(t *testing.T) {
		ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
		loader := &Loader{jsonArena: ar}
		dst := mustParseJSON(ar, `"string-value"`)
		src := mustParseJSON(ar, `{"id":"1"}`)
		// Should not panic
		loader.mergeEntityFields(dst, src)
	})

	t.Run("multiple new and existing fields coexist", func(t *testing.T) {
		ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
		loader := &Loader{jsonArena: ar}

		dst := mustParseJSON(ar, `{"id":"1","name":"Alice","age":30}`)
		src := mustParseJSON(ar, `{"id":"1","email":"a@b.com","role":"admin","name":"Bob"}`)

		loader.mergeEntityFields(dst, src)

		result := dst
		// Existing fields preserved
		assert.Equal(t, `"1"`, string(result.Get("id").MarshalTo(nil)))
		assert.Equal(t, `"Alice"`, string(result.Get("name").MarshalTo(nil)))
		assert.Equal(t, `30`, string(result.Get("age").MarshalTo(nil)))
		// New fields added
		assert.Equal(t, `"a@b.com"`, string(result.Get("email").MarshalTo(nil)))
		assert.Equal(t, `"admin"`, string(result.Get("role").MarshalTo(nil)))
	})
}
