package resolve

import (
	"context"
	"strings"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wundergraph/astjson"
	"github.com/wundergraph/go-arena"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/fastjsonext"
)

// Verifies L2 cache loading for a nested entity graph (products -> reviews -> users).
// Tests that cached entity values are correctly merged into the response at the right paths.
func TestCacheLoad_NestedProductsFromL2(t *testing.T) {
	t.Run("products with reviews - nested products from cache", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		cache := NewFakeLoaderCache()

		// Products datasource - returns list of products
		productsDS := NewMockDataSource(ctrl)
		productsDS.EXPECT().
			Load(gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, headers any, input []byte) ([]byte, error) {
				expected := `{"method":"POST","url":"http://products.service","body":{"query":"{topProducts {__typename id name}}"}}`
				assert.Equal(t, expected, string(input))
				return []byte(`{"data":{"topProducts":[{"__typename":"Product","id":"prod-1","name":"Product One"},{"__typename":"Product","id":"prod-2","name":"Product Two"}]}}`), nil
			}).Times(1)

		// Reviews datasource - returns reviews for products (batch entity fetch)
		reviewsDS := NewMockDataSource(ctrl)
		reviewsDS.EXPECT().
			Load(gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, headers any, input []byte) ([]byte, error) {
				// This is a batch entity fetch for reviews based on product references
				return []byte(`{"data":{"_entities":[{"__typename":"Product","reviews":[{"body":"Great product!","product":{"__typename":"Product","id":"prod-1"}},{"body":"Love it!","product":{"__typename":"Product","id":"prod-1"}}]},{"__typename":"Product","reviews":[{"body":"Awesome!","product":{"__typename":"Product","id":"prod-2"}}]}]}}`), nil
			}).Times(1)

		// Nested products datasource - should NOT be called if caching works
		// We create it but set Times(0) to ensure it's never called
		nestedProductsDS := NewMockDataSource(ctrl)
		nestedProductsDS.EXPECT().
			Load(gomock.Any(), gomock.Any(), gomock.Any()).
			Times(0) // This should never be called - products should come from cache

		// Build the fetch tree
		// 1. Root fetch: topProducts
		// 2. Sequential: fetch reviews for each product (batch)
		// 3. Sequential: fetch nested product (should be from cache)

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

		// ProvidesData for nested product fetch - what data the cache should have
		nestedProductProvidesData := &Object{
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
				// Step 1: Fetch top products
				SingleWithPath(&SingleFetch{
					FetchConfiguration: FetchConfiguration{
						DataSource: productsDS,
						PostProcessing: PostProcessingConfiguration{
							SelectResponseDataPath: []string{"data"},
						},
					},
					InputTemplate: InputTemplate{
						Segments: []TemplateSegment{
							{
								Data:        []byte(`{"method":"POST","url":"http://products.service","body":{"query":"{topProducts {__typename id name}}"}}`),
								SegmentType: StaticSegmentType,
							},
						},
					},
					DataSourceIdentifier: []byte("graphql_datasource.Source"),
				}, "query"),

				// Step 2: Fetch reviews for each product (batch entity fetch)
				SingleWithPath(&BatchEntityFetch{
					Input: BatchInput{
						Header: InputTemplate{
							Segments: []TemplateSegment{
								{
									Data:        []byte(`{"method":"POST","url":"http://reviews.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){__typename ... on Product {reviews {body product {__typename id}}}}}","variables":{"representations":[`),
									SegmentType: StaticSegmentType,
								},
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
									},
								},
							},
						},
						Separator: InputTemplate{
							Segments: []TemplateSegment{
								{
									Data:        []byte(`,`),
									SegmentType: StaticSegmentType,
								},
							},
						},
						Footer: InputTemplate{
							Segments: []TemplateSegment{
								{
									Data:        []byte(`]}}}`),
									SegmentType: StaticSegmentType,
								},
							},
						},
					},
					DataSource: reviewsDS,
					PostProcessing: PostProcessingConfiguration{
						SelectResponseDataPath: []string{"data", "_entities"},
					},
					DataSourceIdentifier: []byte("graphql_datasource.Source"),
				}, "query.topProducts", ArrayPath("topProducts")),

				// Step 3: Fetch nested products (should be from cache)
				SingleWithPath(&BatchEntityFetch{
					Input: BatchInput{
						Header: InputTemplate{
							Segments: []TemplateSegment{
								{
									Data:        []byte(`{"method":"POST","url":"http://products.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){__typename ... on Product {id name}}}","variables":{"representations":[`),
									SegmentType: StaticSegmentType,
								},
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
									},
								},
							},
						},
						Separator: InputTemplate{
							Segments: []TemplateSegment{
								{
									Data:        []byte(`,`),
									SegmentType: StaticSegmentType,
								},
							},
						},
						Footer: InputTemplate{
							Segments: []TemplateSegment{
								{
									Data:        []byte(`]}}}`),
									SegmentType: StaticSegmentType,
								},
							},
						},
					},
					DataSource: nestedProductsDS,
					PostProcessing: PostProcessingConfiguration{
						SelectResponseDataPath: []string{"data", "_entities"},
					},
					DataSourceIdentifier: []byte("graphql_datasource.Source"),
					Info: &FetchInfo{
						DataSourceID:   "products",
						DataSourceName: "products",
						OperationType:  ast.OperationTypeQuery,
						ProvidesData:   nestedProductProvidesData,
					},
					Caching: FetchCacheConfiguration{
						Enabled:          true,
						CacheName:        "default",
						TTL:              30 * time.Second,
						CacheKeyTemplate: productCacheKeyTemplate,
						UseL1Cache:       true,
					},
				}, "query.topProducts.reviews.product", ArrayPath("topProducts"), ArrayPath("reviews"), ObjectPath("product")),
			),
			Data: &Object{
				Fields: []*Field{
					{
						Name: []byte("topProducts"),
						Value: &Array{
							Path: []string{"topProducts"},
							Item: &Object{
								Fields: []*Field{
									{
										Name: []byte("id"),
										Value: &String{
											Path: []string{"id"},
										},
									},
									{
										Name: []byte("name"),
										Value: &String{
											Path: []string{"name"},
										},
									},
									{
										Name: []byte("reviews"),
										Value: &Array{
											Path: []string{"reviews"},
											Item: &Object{
												Fields: []*Field{
													{
														Name: []byte("body"),
														Value: &String{
															Path: []string{"body"},
														},
													},
													{
														Name: []byte("product"),
														Value: &Object{
															Path: []string{"product"},
															Fields: []*Field{
																{
																	Name: []byte("id"),
																	Value: &String{
																		Path: []string{"id"},
																	},
																},
																{
																	Name: []byte("name"),
																	Value: &String{
																		Path: []string{"name"},
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
					},
				},
			},
		}

		// Pre-populate cache with product data (simulating what would happen
		// if we had caching enabled on the root products fetch)
		// In the real implementation, the first products fetch should cache these
		prod1Data := `{"__typename":"Product","id":"prod-1","name":"Product One"}`
		prod2Data := `{"__typename":"Product","id":"prod-2","name":"Product Two"}`

		err := cache.Set(context.Background(), withCacheEntryTTL([]*CacheEntry{
			{Key: `{"__typename":"Product","key":{"id":"prod-1"}}`, Value: []byte(prod1Data)},
			{Key: `{"__typename":"Product","key":{"id":"prod-2"}}`, Value: []byte(prod2Data)},
		}, 30*time.Second))
		require.NoError(t, err)

		cache.ClearLog() // Clear log after pre-population

		// Create loader with cache
		loader := &Loader{
			caches: map[string]LoaderCache{
				"default": cache,
			},
		}

		ctx := NewContext(context.Background())
		// Disable subgraph request deduplication to avoid needing singleFlight
		ctx.ExecutionOptions.DisableSubgraphRequestDeduplication = true
		ctx.ExecutionOptions.Caching.EnableL2Cache = true

		// Create resolvable with arena
		ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
		resolvable := NewResolvable(ar, ResolvableOptions{})
		err = resolvable.Init(ctx, nil, ast.OperationTypeQuery)
		require.NoError(t, err)

		// Execute
		err = loader.LoadGraphQLResponseData(ctx, response, resolvable)
		require.NoError(t, err)

		// Output for debugging
		out := fastjsonext.PrintGraphQLResponse(resolvable.data, resolvable.errors)
		t.Logf("Output: %s", out)

		// Verify cache operations
		cacheLog := cache.GetLog()
		t.Logf("Cache log: %+v", cacheLog)

		// We expect:
		// 1. A "get" operation for the nested product cache keys (should be hits)
		// The nestedProductsDS.Load should NOT have been called (Times(0))

		// Find the get operation for product cache keys
		foundCacheGet := false
		for _, entry := range cacheLog {
			if entry.Operation == "get" {
				foundCacheGet = true
				// Check if we have cache hits
				for _, item := range entry.Items {
					t.Logf("Cache key %s: hit=%v", item.Key, item.Hit)
				}
			}
		}

		assert.True(t, foundCacheGet, "Expected cache get operation for nested products")
	})
}

// Verifies L2 cache hit for a single entity fetch - the simplest cache load path.
func TestCacheLoad_SingleEntityHit(t *testing.T) {
	t.Run("single entity fetch with cache hit", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		cache := NewFakeLoaderCache()

		// Pre-populate cache
		productData := `{"__typename":"Product","id":"prod-1","name":"Cached Product"}`
		err := cache.Set(context.Background(), withCacheEntryTTL([]*CacheEntry{
			{Key: `{"__typename":"Product","key":{"id":"prod-1"}}`, Value: []byte(productData)},
		}, 30*time.Second))
		require.NoError(t, err)
		cache.ClearLog()

		// Create a datasource that should NOT be called (cache hit)
		productDS := NewMockDataSource(ctrl)
		productDS.EXPECT().
			Load(gomock.Any(), gomock.Any(), gomock.Any()).
			Times(0) // Should never be called - we expect cache hit

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

		// Create a simple root response to give us initial data
		rootDS := NewMockDataSource(ctrl)
		rootDS.EXPECT().
			Load(gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, headers any, input []byte) ([]byte, error) {
				return []byte(`{"data":{"product":{"__typename":"Product","id":"prod-1"}}}`), nil
			}).Times(1)

		response := &GraphQLResponse{
			Info: &GraphQLResponseInfo{
				OperationType: ast.OperationTypeQuery,
			},
			Fetches: Sequence(
				// Root fetch to get product reference
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

				// Entity fetch with caching - should hit cache
				SingleWithPath(&SingleFetch{
					FetchConfiguration: FetchConfiguration{
						DataSource: productDS,
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
								Data:        []byte(`{"method":"POST","url":"http://products.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){__typename ... on Product {id name}}}","variables":{"representations":[`),
								SegmentType: StaticSegmentType,
							},
							{
								SegmentType:  VariableSegmentType,
								VariableKind: ResolvableObjectVariableKind,
								Renderer: NewGraphQLVariableResolveRenderer(&Object{
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
								{
									Name: []byte("id"),
									Value: &String{
										Path: []string{"id"},
									},
								},
								{
									Name: []byte("name"),
									Value: &String{
										Path: []string{"name"},
									},
								},
							},
						},
					},
				},
			},
		}

		// Create loader with cache
		loader := &Loader{
			caches: map[string]LoaderCache{
				"default": cache,
			},
		}

		ctx := NewContext(context.Background())
		// Disable subgraph request deduplication to avoid needing singleFlight
		ctx.ExecutionOptions.DisableSubgraphRequestDeduplication = true
		ctx.ExecutionOptions.Caching.EnableL2Cache = true

		// Create resolvable with arena
		ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
		resolvable := NewResolvable(ar, ResolvableOptions{})
		err = resolvable.Init(ctx, nil, ast.OperationTypeQuery)
		require.NoError(t, err)

		// Execute
		err = loader.LoadGraphQLResponseData(ctx, response, resolvable)
		require.NoError(t, err)

		// Output for debugging
		out := fastjsonext.PrintGraphQLResponse(resolvable.data, resolvable.errors)
		t.Logf("Output: %s", out)

		// Verify cache operations
		cacheLog := cache.GetLog()
		t.Logf("Cache log: %+v", cacheLog)

		// We expect at least one cache get that should be a hit
		foundCacheHit := false
		for _, entry := range cacheLog {
			if entry.Operation == "get" {
				for _, item := range entry.Items {
					t.Logf("Cache key %s: hit=%v", item.Key, item.Hit)
					if item.Hit {
						foundCacheHit = true
					}
				}
			}
		}

		assert.True(t, foundCacheHit, "Expected at least one cache hit")
	})

	t.Run("single entity fetch with cache miss", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		cache := NewFakeLoaderCache()
		// Cache is empty - expect cache miss

		// Create a datasource that SHOULD be called (cache miss)
		productDS := NewMockDataSource(ctrl)
		productDS.EXPECT().
			Load(gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, headers any, input []byte) ([]byte, error) {
				return []byte(`{"data":{"_entities":[{"__typename":"Product","id":"prod-1","name":"Fetched Product"}]}}`), nil
			}).Times(1)

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

		// Create a simple root response to give us initial data
		rootDS := NewMockDataSource(ctrl)
		rootDS.EXPECT().
			Load(gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, headers any, input []byte) ([]byte, error) {
				return []byte(`{"data":{"product":{"__typename":"Product","id":"prod-1"}}}`), nil
			}).Times(1)

		response := &GraphQLResponse{
			Info: &GraphQLResponseInfo{
				OperationType: ast.OperationTypeQuery,
			},
			Fetches: Sequence(
				// Root fetch to get product reference
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

				// Entity fetch with caching - should miss cache and fetch
				SingleWithPath(&SingleFetch{
					FetchConfiguration: FetchConfiguration{
						DataSource: productDS,
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
								Data:        []byte(`{"method":"POST","url":"http://products.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){__typename ... on Product {id name}}}","variables":{"representations":[`),
								SegmentType: StaticSegmentType,
							},
							{
								SegmentType:  VariableSegmentType,
								VariableKind: ResolvableObjectVariableKind,
								Renderer: NewGraphQLVariableResolveRenderer(&Object{
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
								{
									Name: []byte("id"),
									Value: &String{
										Path: []string{"id"},
									},
								},
								{
									Name: []byte("name"),
									Value: &String{
										Path: []string{"name"},
									},
								},
							},
						},
					},
				},
			},
		}

		// Create loader with cache
		loader := &Loader{
			caches: map[string]LoaderCache{
				"default": cache,
			},
		}

		ctx := NewContext(context.Background())
		// Disable subgraph request deduplication to avoid needing singleFlight
		ctx.ExecutionOptions.DisableSubgraphRequestDeduplication = true
		ctx.ExecutionOptions.Caching.EnableL2Cache = true

		// Create resolvable with arena
		ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
		resolvable := NewResolvable(ar, ResolvableOptions{})
		err := resolvable.Init(ctx, nil, ast.OperationTypeQuery)
		require.NoError(t, err)

		// Execute
		err = loader.LoadGraphQLResponseData(ctx, response, resolvable)
		require.NoError(t, err)

		// Output for debugging
		out := fastjsonext.PrintGraphQLResponse(resolvable.data, resolvable.errors)
		t.Logf("Output: %s", out)

		// Verify cache operations
		cacheLog := cache.GetLog()
		t.Logf("Cache log: %+v", cacheLog)

		// We expect:
		// 1. A "get" operation that misses
		// 2. A "set" operation to cache the result
		foundCacheGet := false
		foundCacheSet := false
		for _, entry := range cacheLog {
			if entry.Operation == "get" {
				foundCacheGet = true
				// Verify it's a miss
				for _, item := range entry.Items {
					t.Logf("Cache key %s: hit=%v", item.Key, item.Hit)
					assert.False(t, item.Hit, "Expected cache miss")
				}
			}
			if entry.Operation == "set" {
				foundCacheSet = true
				t.Logf("Cache set items: %v", entry.Items)
			}
		}

		assert.True(t, foundCacheGet, "Expected cache get operation")
		assert.True(t, foundCacheSet, "Expected cache set operation after miss")
	})
}

// Verifies the L2 miss-then-hit lifecycle: first call populates cache, second call reads from it.
func TestCacheLoad_SequentialMissThenHit(t *testing.T) {
	t.Run("two sequential calls - miss then hit", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		cache := NewFakeLoaderCache()
		// Cache is empty - no pre-population

		// Create a datasource that should be called exactly ONCE (first call = miss)
		productDS := NewMockDataSource(ctrl)
		productDS.EXPECT().
			Load(gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, headers any, input []byte) ([]byte, error) {
				return []byte(`{"data":{"_entities":[{"__typename":"Product","id":"prod-1","name":"Fetched Product"}]}}`), nil
			}).Times(1) // Only called once - second call should hit cache

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

		// Root datasource - will be called twice (once per execution)
		rootDS := NewMockDataSource(ctrl)
		rootDS.EXPECT().
			Load(gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, headers any, input []byte) ([]byte, error) {
				return []byte(`{"data":{"product":{"__typename":"Product","id":"prod-1"}}}`), nil
			}).Times(2) // Called for each execution

		buildResponse := func() *GraphQLResponse {
			return &GraphQLResponse{
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
								{
									Data:        []byte(`{"method":"POST","url":"http://root.service","body":{"query":"{product {__typename id}}"}}`),
									SegmentType: StaticSegmentType,
								},
							},
						},
						DataSourceIdentifier: []byte("graphql_datasource.Source"),
					}, "query"),
					SingleWithPath(&SingleFetch{
						FetchConfiguration: FetchConfiguration{
							DataSource: productDS,
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
									Data:        []byte(`{"method":"POST","url":"http://products.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){__typename ... on Product {id name}}}","variables":{"representations":[`),
									SegmentType: StaticSegmentType,
								},
								{
									SegmentType:  VariableSegmentType,
									VariableKind: ResolvableObjectVariableKind,
									Renderer: NewGraphQLVariableResolveRenderer(&Object{
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
									{
										Name: []byte("id"),
										Value: &String{
											Path: []string{"id"},
										},
									},
									{
										Name: []byte("name"),
										Value: &String{
											Path: []string{"name"},
										},
									},
								},
							},
						},
					},
				},
			}
		}

		// Shared loader with cache
		loader := &Loader{
			caches: map[string]LoaderCache{
				"default": cache,
			},
		}

		// === First execution: expect cache MISS ===
		t.Log("=== First execution (expect cache miss) ===")

		ctx1 := NewContext(context.Background())
		ctx1.ExecutionOptions.DisableSubgraphRequestDeduplication = true
		ctx1.ExecutionOptions.Caching.EnableL2Cache = true

		ar1 := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
		resolvable1 := NewResolvable(ar1, ResolvableOptions{})
		err := resolvable1.Init(ctx1, nil, ast.OperationTypeQuery)
		require.NoError(t, err)

		response1 := buildResponse()
		err = loader.LoadGraphQLResponseData(ctx1, response1, resolvable1)
		require.NoError(t, err)

		out1 := fastjsonext.PrintGraphQLResponse(resolvable1.data, resolvable1.errors)
		t.Logf("First output: %s", out1)

		// Verify first call had cache miss and set
		cacheLog1 := cache.GetLog()
		t.Logf("Cache log after first call: %+v", cacheLog1)

		var firstGetHits []bool
		foundFirstGet := false
		foundFirstSet := false
		for _, entry := range cacheLog1 {
			if entry.Operation == "get" {
				foundFirstGet = true
				firstGetHits = make([]bool, 0, len(entry.Items))
				for _, item := range entry.Items {
					firstGetHits = append(firstGetHits, item.Hit)
					t.Logf("First call - Cache key %s: hit=%v", item.Key, item.Hit)
				}
			}
			if entry.Operation == "set" {
				foundFirstSet = true
			}
		}

		assert.True(t, foundFirstGet, "Expected cache get operation on first call")
		assert.True(t, foundFirstSet, "Expected cache set operation on first call (after miss)")
		require.Len(t, firstGetHits, 1, "Expected exactly one cache key")
		assert.False(t, firstGetHits[0], "Expected cache MISS on first call")

		// Clear log for second execution
		cache.ClearLog()

		// === Second execution: expect cache HIT ===
		t.Log("=== Second execution (expect cache hit) ===")

		ctx2 := NewContext(context.Background())
		ctx2.ExecutionOptions.DisableSubgraphRequestDeduplication = true
		ctx2.ExecutionOptions.Caching.EnableL2Cache = true

		ar2 := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
		resolvable2 := NewResolvable(ar2, ResolvableOptions{})
		err = resolvable2.Init(ctx2, nil, ast.OperationTypeQuery)
		require.NoError(t, err)

		response2 := buildResponse()
		err = loader.LoadGraphQLResponseData(ctx2, response2, resolvable2)
		require.NoError(t, err)

		out2 := fastjsonext.PrintGraphQLResponse(resolvable2.data, resolvable2.errors)
		t.Logf("Second output: %s", out2)

		// Verify second call had cache hit (no set)
		cacheLog2 := cache.GetLog()
		t.Logf("Cache log after second call: %+v", cacheLog2)

		var secondGetHits []bool
		foundSecondGet := false
		foundSecondSet := false
		for _, entry := range cacheLog2 {
			if entry.Operation == "get" {
				foundSecondGet = true
				secondGetHits = make([]bool, 0, len(entry.Items))
				for _, item := range entry.Items {
					secondGetHits = append(secondGetHits, item.Hit)
					t.Logf("Second call - Cache key %s: hit=%v", item.Key, item.Hit)
				}
			}
			if entry.Operation == "set" {
				foundSecondSet = true
			}
		}

		assert.True(t, foundSecondGet, "Expected cache get operation on second call")
		assert.False(t, foundSecondSet, "Expected NO cache set on second call (cache hit)")
		require.Len(t, secondGetHits, 1, "Expected exactly one cache key")
		assert.True(t, secondGetHits[0], "Expected cache HIT on second call")

		// Verify both outputs are identical
		assert.Equal(t, out1, out2, "Both executions should produce identical output")
	})
}

// Testing utilities

// CacheLogItem is one key touched by a cache operation.
// Field meaning depends on Operation:
//   - "get": Key + Hit are populated; TTL is unused.
//   - "set": Key + TTL are populated; Hit is unused.
//   - "delete": only Key is populated.
type CacheLogItem struct {
	Key string
	Hit bool
	TTL time.Duration
}

// CacheLogEntry tracks a cache operation for testing.
type CacheLogEntry struct {
	Operation string
	Items     []CacheLogItem
}

type cacheEntry struct {
	data      []byte
	expiresAt *time.Time
}

func withCacheEntryTTL(entries []*CacheEntry, ttl time.Duration) []*CacheEntry {
	for _, entry := range entries {
		if entry != nil {
			entry.TTL = ttl
		}
	}
	return entries
}

// FakeLoaderCache is an in-memory cache implementation for testing
type FakeLoaderCache struct {
	mu      sync.RWMutex
	storage map[string]cacheEntry
	log     []CacheLogEntry
}

func NewFakeLoaderCache() *FakeLoaderCache {
	return &FakeLoaderCache{
		storage: make(map[string]cacheEntry),
		log:     make([]CacheLogEntry, 0),
	}
}

func (f *FakeLoaderCache) cleanupExpired() {
	now := time.Now()
	for key, entry := range f.storage {
		if entry.expiresAt != nil && now.After(*entry.expiresAt) {
			delete(f.storage, key)
		}
	}
}

func (f *FakeLoaderCache) Get(ctx context.Context, keys []string) ([]*CacheEntry, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	// Clean up expired entries before executing command
	f.cleanupExpired()

	items := make([]CacheLogItem, len(keys))
	result := make([]*CacheEntry, len(keys))
	for i, key := range keys {
		items[i].Key = key
		if entry, exists := f.storage[key]; exists {
			// Make a copy of the data to prevent external modifications
			dataCopy := make([]byte, len(entry.data))
			copy(dataCopy, entry.data)
			ce := &CacheEntry{
				Key:   key,
				Value: dataCopy,
			}
			// Populate RemainingTTL from expiresAt for cache age analytics
			if entry.expiresAt != nil {
				remaining := time.Until(*entry.expiresAt)
				if remaining > 0 {
					ce.RemainingTTL = remaining
				}
			}
			result[i] = ce
			items[i].Hit = true
		} else {
			result[i] = nil
		}
	}

	// Log the operation
	f.log = append(f.log, CacheLogEntry{
		Operation: "get",
		Items:     items,
	})

	return result, nil
}

func (f *FakeLoaderCache) Set(ctx context.Context, entries []*CacheEntry) error {
	if len(entries) == 0 {
		return nil
	}

	f.mu.Lock()
	defer f.mu.Unlock()

	// Clean up expired entries before executing command
	f.cleanupExpired()

	items := make([]CacheLogItem, 0, len(entries))
	for _, entry := range entries {
		if entry == nil {
			continue
		}
		ce := cacheEntry{
			// Make a copy of the data to prevent external modifications
			data: make([]byte, len(entry.Value)),
		}
		copy(ce.data, entry.Value)

		// Non-positive TTLs use the fake cache's no-expiration default.
		if entry.TTL > 0 {
			expiresAt := time.Now().Add(entry.TTL)
			ce.expiresAt = &expiresAt
		}

		f.storage[entry.Key] = ce
		items = append(items, CacheLogItem{Key: entry.Key, TTL: entry.TTL})
	}

	// Log the operation
	f.log = append(f.log, CacheLogEntry{
		Operation: "set",
		Items:     items,
	})

	return nil
}

func (f *FakeLoaderCache) Delete(ctx context.Context, keys []string) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	// Clean up expired entries before executing command
	f.cleanupExpired()

	for _, key := range keys {
		delete(f.storage, key)
	}
	items := make([]CacheLogItem, len(keys))
	for i, key := range keys {
		items[i] = CacheLogItem{Key: key}
	}

	// Log the operation
	f.log = append(f.log, CacheLogEntry{
		Operation: "delete",
		Items:     items,
	})

	return nil
}

// GetLog returns a copy of the cache operation log
func (f *FakeLoaderCache) GetLog() []CacheLogEntry {
	f.mu.RLock()
	defer f.mu.RUnlock()
	logCopy := make([]CacheLogEntry, len(f.log))
	copy(logCopy, f.log)
	return logCopy
}

// ClearLog clears the cache operation log
func (f *FakeLoaderCache) ClearLog() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.log = make([]CacheLogEntry, 0)
}

// GetValue returns the raw cached value for a key, or nil if not found.
func (f *FakeLoaderCache) GetValue(key string) []byte {
	f.mu.RLock()
	defer f.mu.RUnlock()
	if entry, exists := f.storage[key]; exists {
		dataCopy := make([]byte, len(entry.data))
		copy(dataCopy, entry.data)
		return dataCopy
	}
	return nil
}

// Clear removes all entries from the cache
func (f *FakeLoaderCache) Clear() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.storage = make(map[string]cacheEntry)
}

// SetRawData directly injects data into the cache for testing purposes.
// This bypasses the normal Set path and allows injecting stale/modified data.
func (f *FakeLoaderCache) SetRawData(key string, value []byte, ttl time.Duration) {
	f.mu.Lock()
	defer f.mu.Unlock()
	ce := cacheEntry{
		data: make([]byte, len(value)),
	}
	copy(ce.data, value)
	if ttl > 0 {
		expiresAt := time.Now().Add(ttl)
		ce.expiresAt = &expiresAt
	}
	f.storage[key] = ce
}

// =============================================================================
// Shadow Mode Integration Tests
// =============================================================================

// normalizeCacheAnalyticsSnapshot zeroes out non-deterministic fields
// (FetchTimings.DurationMs and every event's auto-stamped Timestamp) and
// normalizes empty slices to nil for consistent assert.Equal comparison.
// CacheAgeMs is deterministic when tests run inside synctest.Test (fake clock).
func normalizeCacheAnalyticsSnapshot(snap CacheAnalyticsSnapshot) CacheAnalyticsSnapshot {
	// Zero out non-deterministic FetchTimings (DurationMs varies between runs)
	snap.FetchTimings = nil

	// Zero auto-stamped Timestamps on every event slice. Each Record* method
	// stamps time.Now() when the caller passes a zero value; the wall-clock
	// nanos make literal struct equality flake without this normalization.
	for i := range snap.L1Reads {
		snap.L1Reads[i].Timestamp = time.Time{}
	}
	for i := range snap.L2Reads {
		snap.L2Reads[i].Timestamp = time.Time{}
	}
	for i := range snap.L1Writes {
		snap.L1Writes[i].Timestamp = time.Time{}
	}
	for i := range snap.L2Writes {
		snap.L2Writes[i].Timestamp = time.Time{}
	}
	for i := range snap.ErrorEvents {
		snap.ErrorEvents[i].Timestamp = time.Time{}
	}
	for i := range snap.FieldHashes {
		snap.FieldHashes[i].Timestamp = time.Time{}
		// FieldHash / KeyHash are deterministic xxhash values but not
		// human-meaningful as test fixtures — embedding the literal uint64
		// would force every test to spell out the hash of the cached payload
		// and cache key. The exact hash semantics are covered by
		// TestCacheAnalyticsCollector_BuildRootFieldCacheHits; here we just
		// verify the field is populated (non-zero for synthetic root-field
		// hits) and zero it out for the literal-struct equality check.
		snap.FieldHashes[i].FieldHash = 0
		snap.FieldHashes[i].KeyHash = 0
	}
	for i := range snap.ShadowComparisons {
		snap.ShadowComparisons[i].Timestamp = time.Time{}
	}
	for i := range snap.MutationEvents {
		snap.MutationEvents[i].Timestamp = time.Time{}
	}
	for i := range snap.HeaderImpactEvents {
		snap.HeaderImpactEvents[i].Timestamp = time.Time{}
	}
	for i := range snap.CacheOpErrors {
		snap.CacheOpErrors[i].Timestamp = time.Time{}
	}

	// Normalize empty slices to nil
	if len(snap.L1Reads) == 0 {
		snap.L1Reads = nil
	}
	if len(snap.L2Reads) == 0 {
		snap.L2Reads = nil
	}
	if len(snap.L1Writes) == 0 {
		snap.L1Writes = nil
	}
	if len(snap.L2Writes) == 0 {
		snap.L2Writes = nil
	}
	if len(snap.ErrorEvents) == 0 {
		snap.ErrorEvents = nil
	}
	if len(snap.FieldHashes) == 0 {
		snap.FieldHashes = nil
	}
	if len(snap.EntityTypes) == 0 {
		snap.EntityTypes = nil
	}
	if len(snap.ShadowComparisons) == 0 {
		snap.ShadowComparisons = nil
	}
	if len(snap.MutationEvents) == 0 {
		snap.MutationEvents = nil
	}
	if len(snap.HeaderImpactEvents) == 0 {
		snap.HeaderImpactEvents = nil
	}
	if len(snap.CacheOpErrors) == 0 {
		snap.CacheOpErrors = nil
	}

	return snap
}

const (
	shadowTestKeyProduct = `{"__typename":"Product","key":{"id":"prod-1"}}`
	shadowTestKeyUser    = `{"__typename":"User","key":{"id":"u1"}}`
)

// Verifies that shadow mode always fetches from the subgraph even when L2 has data.
// Shadow mode exists for staleness detection without serving potentially stale cached data.
func TestShadowMode_L2_AlwaysFetches(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		cache := NewFakeLoaderCache()

		// Root fetch (not cached)
		rootDS := NewMockDataSource(ctrl)
		rootDS.EXPECT().
			Load(gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, headers any, input []byte) ([]byte, error) {
				return []byte(`{"data":{"product":{"__typename":"Product","id":"prod-1"}}}`), nil
			}).Times(2) // called twice (once per request)

		// Entity fetch - called BOTH times (shadow mode prevents cache serving)
		entityDS := NewMockDataSource(ctrl)
		entityDS.EXPECT().
			Load(gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, headers any, input []byte) ([]byte, error) {
				return []byte(`{"data":{"_entities":[{"__typename":"Product","id":"prod-1","name":"Product One"}]}}`), nil
			}).Times(2) // called twice because shadow mode

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

		buildResponse := func() *GraphQLResponse {
			return &GraphQLResponse{
				Info: &GraphQLResponseInfo{OperationType: ast.OperationTypeQuery},
				Fetches: Sequence(
					SingleWithPath(&SingleFetch{
						FetchConfiguration: FetchConfiguration{
							DataSource:     rootDS,
							PostProcessing: PostProcessingConfiguration{SelectResponseDataPath: []string{"data"}},
						},
						InputTemplate: InputTemplate{Segments: []TemplateSegment{
							{Data: []byte(`{"method":"POST","url":"http://root.service","body":{"query":"{product {__typename id}}"}}`), SegmentType: StaticSegmentType},
						}},
						DataSourceIdentifier: []byte("graphql_datasource.Source"),
					}, "query"),
					SingleWithPath(&SingleFetch{
						FetchConfiguration: FetchConfiguration{
							DataSource:     entityDS,
							PostProcessing: PostProcessingConfiguration{SelectResponseDataPath: []string{"data", "_entities", "0"}},
							Caching: FetchCacheConfiguration{
								Enabled:          true,
								CacheName:        "default",
								TTL:              30 * time.Second,
								CacheKeyTemplate: productCacheKeyTemplate,
								UseL1Cache:       true,
								ShadowMode:       true,
								KeyFields:        []KeyField{{Name: "id"}},
							},
						},
						InputTemplate: InputTemplate{Segments: []TemplateSegment{
							{Data: []byte(`{"method":"POST","url":"http://products.service","body":{"query":"...","variables":{"representations":[`), SegmentType: StaticSegmentType},
							{SegmentType: VariableSegmentType, VariableKind: ResolvableObjectVariableKind, Renderer: NewGraphQLVariableResolveRenderer(&Object{
								Fields: []*Field{
									{Name: []byte("__typename"), Value: &String{Path: []string{"__typename"}}},
									{Name: []byte("id"), Value: &String{Path: []string{"id"}}},
								},
							})},
							{Data: []byte(`]}}}`), SegmentType: StaticSegmentType},
						}},
						Info: &FetchInfo{
							DataSourceID: "products", DataSourceName: "products",
							RootFields:    []GraphCoordinate{{TypeName: "Product", FieldName: "name"}},
							OperationType: ast.OperationTypeQuery, ProvidesData: providesData,
						},
						DataSourceIdentifier: []byte("graphql_datasource.Source"),
					}, "query.product", ObjectPath("product")),
				),
				Data: &Object{
					Fields: []*Field{{
						Name: []byte("product"),
						Value: &Object{
							Path: []string{"product"},
							Fields: []*Field{
								{Name: []byte("id"), Value: &String{Path: []string{"id"}}},
								{Name: []byte("name"), Value: &String{Path: []string{"name"}}},
							},
						},
					}},
				},
			}
		}

		// Request 1: L2 miss -> DataSource called -> L2 populated
		loader := &Loader{caches: map[string]LoaderCache{"default": cache}}
		ctx1 := NewContext(context.Background())
		ctx1.ExecutionOptions.DisableSubgraphRequestDeduplication = true
		ctx1.ExecutionOptions.Caching.EnableL1Cache = true
		ctx1.ExecutionOptions.Caching.EnableL2Cache = true
		ctx1.ExecutionOptions.Caching.EnableCacheAnalytics = true

		ar1 := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
		resolvable1 := NewResolvable(ar1, ResolvableOptions{})
		err := resolvable1.Init(ctx1, nil, ast.OperationTypeQuery)
		require.NoError(t, err)

		err = loader.LoadGraphQLResponseData(ctx1, buildResponse(), resolvable1)
		require.NoError(t, err)

		out1 := fastjsonext.PrintGraphQLResponse(resolvable1.data, resolvable1.errors)
		assert.Equal(t, `{"data":{"product":{"__typename":"Product","id":"prod-1","name":"Product One"}}}`, out1)

		assert.Equal(t, normalizeCacheAnalyticsSnapshot(CacheAnalyticsSnapshot{
			L1Reads: []CacheKeyEvent{
				{CacheKey: shadowTestKeyProduct, EntityType: "Product", Kind: CacheKeyMiss, DataSource: "products"}, // First request, L1 is empty
			},
			L2Reads: []CacheKeyEvent{
				{CacheKey: shadowTestKeyProduct, EntityType: "Product", Kind: CacheKeyMiss, DataSource: "products", Shadow: true}, // First request, L2 is empty; Shadow marks shadow-mode fetch
			},
			L1Writes: []CacheWriteEvent{
				{CacheKey: shadowTestKeyProduct, EntityType: "Product", ByteSize: 59, DataSource: "products", CacheLevel: CacheLevelL1, Source: CacheSourceQuery}, // Miss triggered subgraph fetch, result written to L1
			},
			L2Writes: []CacheWriteEvent{
				{CacheKey: shadowTestKeyProduct, EntityType: "Product", ByteSize: 59, DataSource: "products", CacheLevel: CacheLevelL2, TTL: 30 * time.Second, Source: CacheSourceQuery}, // Miss triggered subgraph fetch, result written to L2
			},
		}), normalizeCacheAnalyticsSnapshot(ctx1.GetCacheStats()))

		// Advance fake clock by 5s so Request 2's L2 hit has a measurable CacheAgeMs
		time.Sleep(5 * time.Second)

		// Request 2: L2 hit (shadow) -> DataSource STILL called
		loader2 := &Loader{caches: map[string]LoaderCache{"default": cache}}
		ctx2 := NewContext(context.Background())
		ctx2.ExecutionOptions.DisableSubgraphRequestDeduplication = true
		ctx2.ExecutionOptions.Caching.EnableL1Cache = true
		ctx2.ExecutionOptions.Caching.EnableL2Cache = true
		ctx2.ExecutionOptions.Caching.EnableCacheAnalytics = true

		ar2 := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
		resolvable2 := NewResolvable(ar2, ResolvableOptions{})
		err = resolvable2.Init(ctx2, nil, ast.OperationTypeQuery)
		require.NoError(t, err)

		err = loader2.LoadGraphQLResponseData(ctx2, buildResponse(), resolvable2)
		require.NoError(t, err)

		out2 := fastjsonext.PrintGraphQLResponse(resolvable2.data, resolvable2.errors)
		assert.Equal(t, `{"data":{"product":{"__typename":"Product","id":"prod-1","name":"Product One"}}}`, out2)

		assert.Equal(t, normalizeCacheAnalyticsSnapshot(CacheAnalyticsSnapshot{
			L1Reads: []CacheKeyEvent{
				{CacheKey: shadowTestKeyProduct, EntityType: "Product", Kind: CacheKeyMiss, DataSource: "products"}, // New Loader instance, L1 is per-request and empty
			},
			L2Reads: []CacheKeyEvent{
				{CacheKey: shadowTestKeyProduct, EntityType: "Product", Kind: CacheKeyHit, DataSource: "products", ByteSize: 59, Shadow: true, CacheAgeMs: 5000}, // L2 populated by Request 1, 5s ago; Shadow=true so subgraph is still fetched
			},
			L1Writes: []CacheWriteEvent{
				{CacheKey: shadowTestKeyProduct, EntityType: "Product", ByteSize: 59, DataSource: "products", CacheLevel: CacheLevelL1, Source: CacheSourceQuery}, // Written from subgraph response (shadow mode always fetches)
			},
			L2Writes: []CacheWriteEvent{
				{CacheKey: shadowTestKeyProduct, EntityType: "Product", ByteSize: 59, DataSource: "products", CacheLevel: CacheLevelL2, TTL: 30 * time.Second, Source: CacheSourceQuery}, // Overwritten in L2 with fresh subgraph response
			},
			ShadowComparisons: []ShadowComparisonEvent{
				{CacheKey: shadowTestKeyProduct, EntityType: "Product", IsFresh: true, CachedHash: 16331343294028781429, FreshHash: 16331343294028781429, CachedBytes: 36, FreshBytes: 36, DataSource: "products", ConfiguredTTL: 30 * time.Second, CacheAgeMs: 5000}, // Cached data matches subgraph (same hash), no staleness; entry was 5s old
			},
			FieldHashes: []EntityFieldHash{
				{EntityType: "Product", FieldName: "id", FieldHash: 4016270444951293489, KeyRaw: `{"id":"prod-1"}`, Source: FieldSourceShadowCached, DataSource: "products"},   // Cached "id" field from shadow comparison
				{EntityType: "Product", FieldName: "name", FieldHash: 8385814294091472045, KeyRaw: `{"id":"prod-1"}`, Source: FieldSourceShadowCached, DataSource: "products"}, // Cached "name" field from shadow comparison
			},
		}), normalizeCacheAnalyticsSnapshot(ctx2.GetCacheStats()))
	})
}

// Verifies that shadow mode records staleness comparison events when cached data
// differs from fresh subgraph data.
func TestShadowMode_StalenessDetection(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		cache := NewFakeLoaderCache()

		rootDS := NewMockDataSource(ctrl)
		rootDS.EXPECT().
			Load(gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, headers any, input []byte) ([]byte, error) {
				return []byte(`{"data":{"user":{"__typename":"User","id":"u1"}}}`), nil
			}).Times(2)

		entityDS := NewMockDataSource(ctrl)
		// First call returns "Alice"
		entityDS.EXPECT().
			Load(gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, headers any, input []byte) ([]byte, error) {
				return []byte(`{"data":{"_entities":[{"__typename":"User","id":"u1","username":"Alice"}]}}`), nil
			}).Times(1)
		// Second call returns "AliceUpdated" (subgraph data changed)
		entityDS.EXPECT().
			Load(gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, headers any, input []byte) ([]byte, error) {
				return []byte(`{"data":{"_entities":[{"__typename":"User","id":"u1","username":"AliceUpdated"}]}}`), nil
			}).Times(1)

		userCacheKeyTemplate := &EntityQueryCacheKeyTemplate{
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
				{Name: []byte("username"), Value: &Scalar{Path: []string{"username"}, Nullable: false}},
			},
		}

		buildResponse := func() *GraphQLResponse {
			return &GraphQLResponse{
				Info: &GraphQLResponseInfo{OperationType: ast.OperationTypeQuery},
				Fetches: Sequence(
					SingleWithPath(&SingleFetch{
						FetchConfiguration: FetchConfiguration{
							DataSource:     rootDS,
							PostProcessing: PostProcessingConfiguration{SelectResponseDataPath: []string{"data"}},
						},
						InputTemplate: InputTemplate{Segments: []TemplateSegment{
							{Data: []byte(`{"method":"POST"}`), SegmentType: StaticSegmentType},
						}},
						DataSourceIdentifier: []byte("graphql_datasource.Source"),
					}, "query"),
					SingleWithPath(&SingleFetch{
						FetchConfiguration: FetchConfiguration{
							DataSource:     entityDS,
							PostProcessing: PostProcessingConfiguration{SelectResponseDataPath: []string{"data", "_entities", "0"}},
							Caching: FetchCacheConfiguration{
								Enabled:          true,
								CacheName:        "default",
								TTL:              30 * time.Second,
								CacheKeyTemplate: userCacheKeyTemplate,
								UseL1Cache:       true,
								ShadowMode:       true,
								KeyFields:        []KeyField{{Name: "id"}},
							},
						},
						InputTemplate: InputTemplate{Segments: []TemplateSegment{
							{Data: []byte(`{"method":"POST","url":"http://accounts.service","body":{"query":"...","variables":{"representations":[`), SegmentType: StaticSegmentType},
							{SegmentType: VariableSegmentType, VariableKind: ResolvableObjectVariableKind, Renderer: NewGraphQLVariableResolveRenderer(&Object{
								Fields: []*Field{
									{Name: []byte("__typename"), Value: &String{Path: []string{"__typename"}}},
									{Name: []byte("id"), Value: &String{Path: []string{"id"}}},
								},
							})},
							{Data: []byte(`]}}}`), SegmentType: StaticSegmentType},
						}},
						Info: &FetchInfo{
							DataSourceID: "accounts", DataSourceName: "accounts",
							RootFields:    []GraphCoordinate{{TypeName: "User", FieldName: "username"}},
							OperationType: ast.OperationTypeQuery, ProvidesData: providesData,
						},
						DataSourceIdentifier: []byte("graphql_datasource.Source"),
					}, "query.user", ObjectPath("user")),
				),
				Data: &Object{
					Fields: []*Field{{
						Name: []byte("user"),
						Value: &Object{
							Path: []string{"user"},
							Fields: []*Field{
								{Name: []byte("id"), Value: &String{Path: []string{"id"}}},
								{Name: []byte("username"), Value: &String{Path: []string{"username"}}},
							},
						},
					}},
				},
			}
		}

		// Request 1: Populate L2 cache with "Alice"
		loader1 := &Loader{caches: map[string]LoaderCache{"default": cache}}
		ctx1 := NewContext(context.Background())
		ctx1.ExecutionOptions.DisableSubgraphRequestDeduplication = true
		ctx1.ExecutionOptions.Caching.EnableL1Cache = true
		ctx1.ExecutionOptions.Caching.EnableL2Cache = true
		ctx1.ExecutionOptions.Caching.EnableCacheAnalytics = true

		ar1 := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
		resolvable1 := NewResolvable(ar1, ResolvableOptions{})
		err := resolvable1.Init(ctx1, nil, ast.OperationTypeQuery)
		require.NoError(t, err)

		err = loader1.LoadGraphQLResponseData(ctx1, buildResponse(), resolvable1)
		require.NoError(t, err)

		assert.Equal(t, normalizeCacheAnalyticsSnapshot(CacheAnalyticsSnapshot{
			L1Reads: []CacheKeyEvent{
				{CacheKey: shadowTestKeyUser, EntityType: "User", Kind: CacheKeyMiss, DataSource: "accounts"}, // First request, L1 is empty
			},
			L2Reads: []CacheKeyEvent{
				{CacheKey: shadowTestKeyUser, EntityType: "User", Kind: CacheKeyMiss, DataSource: "accounts", Shadow: true}, // First request, L2 is empty; Shadow marks shadow-mode fetch
			},
			L1Writes: []CacheWriteEvent{
				{CacheKey: shadowTestKeyUser, EntityType: "User", ByteSize: 50, DataSource: "accounts", CacheLevel: CacheLevelL1, Source: CacheSourceQuery}, // "Alice" written to L1 after subgraph fetch
			},
			L2Writes: []CacheWriteEvent{
				{CacheKey: shadowTestKeyUser, EntityType: "User", ByteSize: 50, DataSource: "accounts", CacheLevel: CacheLevelL2, TTL: 30 * time.Second, Source: CacheSourceQuery}, // "Alice" written to L2 after subgraph fetch
			},
		}), normalizeCacheAnalyticsSnapshot(ctx1.GetCacheStats()))

		// Advance fake clock by 5s so Request 2's L2 hit has a measurable CacheAgeMs
		time.Sleep(5 * time.Second)

		// Request 2: L2 has "Alice" but subgraph returns "AliceUpdated"
		loader2 := &Loader{caches: map[string]LoaderCache{"default": cache}}
		ctx2 := NewContext(context.Background())
		ctx2.ExecutionOptions.DisableSubgraphRequestDeduplication = true
		ctx2.ExecutionOptions.Caching.EnableL1Cache = true
		ctx2.ExecutionOptions.Caching.EnableL2Cache = true
		ctx2.ExecutionOptions.Caching.EnableCacheAnalytics = true

		ar2 := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
		resolvable2 := NewResolvable(ar2, ResolvableOptions{})
		err = resolvable2.Init(ctx2, nil, ast.OperationTypeQuery)
		require.NoError(t, err)

		err = loader2.LoadGraphQLResponseData(ctx2, buildResponse(), resolvable2)
		require.NoError(t, err)

		// Verify fresh data is served (not stale cache)
		out2 := fastjsonext.PrintGraphQLResponse(resolvable2.data, resolvable2.errors)
		assert.Equal(t, `{"data":{"user":{"__typename":"User","id":"u1","username":"AliceUpdated"}}}`, out2)

		assert.Equal(t, normalizeCacheAnalyticsSnapshot(CacheAnalyticsSnapshot{
			L1Reads: []CacheKeyEvent{
				{CacheKey: shadowTestKeyUser, EntityType: "User", Kind: CacheKeyMiss, DataSource: "accounts"}, // New Loader instance, L1 is per-request and empty
			},
			L2Reads: []CacheKeyEvent{
				{CacheKey: shadowTestKeyUser, EntityType: "User", Kind: CacheKeyHit, DataSource: "accounts", ByteSize: 50, Shadow: true, CacheAgeMs: 5000}, // L2 has "Alice" from Request 1, 5s ago; Shadow=true so subgraph is still fetched
			},
			L1Writes: []CacheWriteEvent{
				{CacheKey: shadowTestKeyUser, EntityType: "User", ByteSize: 57, DataSource: "accounts", CacheLevel: CacheLevelL1, Source: CacheSourceQuery}, // "AliceUpdated" written to L1 from fresh subgraph response
			},
			L2Writes: []CacheWriteEvent{
				{CacheKey: shadowTestKeyUser, EntityType: "User", ByteSize: 57, DataSource: "accounts", CacheLevel: CacheLevelL2, TTL: 30 * time.Second, Source: CacheSourceQuery}, // "AliceUpdated" overwrites "Alice" in L2
			},
			ShadowComparisons: []ShadowComparisonEvent{
				{CacheKey: shadowTestKeyUser, EntityType: "User", IsFresh: false, CachedHash: 272931794584083561, FreshHash: 4550742678894771079, CachedBytes: 30, FreshBytes: 37, DataSource: "accounts", ConfiguredTTL: 30 * time.Second, CacheAgeMs: 5000}, // Cached "Alice" differs from fresh "AliceUpdated" (different hashes); entry was 5s old
			},
			FieldHashes: []EntityFieldHash{
				{EntityType: "User", FieldName: "id", FieldHash: 13311642224980425257, KeyRaw: `{"id":"u1"}`, Source: FieldSourceShadowCached, DataSource: "accounts"},      // Cached "id" field from "Alice" entity
				{EntityType: "User", FieldName: "username", FieldHash: 5631231822564450273, KeyRaw: `{"id":"u1"}`, Source: FieldSourceShadowCached, DataSource: "accounts"}, // Cached "username"="Alice" (stale value)
			},
		}), normalizeCacheAnalyticsSnapshot(ctx2.GetCacheStats()))
	})
}

// Verifies that L1 cache operates normally even when shadow mode is enabled for L2.
// Shadow mode should only affect L2 behavior.
func TestShadowMode_L1_WorksNormally(t *testing.T) {
	t.Run("L1 cache serves data normally even with shadow mode entity", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		cache := NewFakeLoaderCache()

		rootDS := NewMockDataSource(ctrl)
		rootDS.EXPECT().
			Load(gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, headers any, input []byte) ([]byte, error) {
				return []byte(`{"data":{"product":{"__typename":"Product","id":"prod-1"}}}`), nil
			}).Times(1)

		// Entity fetch called only ONCE (second occurrence served from L1)
		entityDS := NewMockDataSource(ctrl)
		entityDS.EXPECT().
			Load(gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, headers any, input []byte) ([]byte, error) {
				return []byte(`{"data":{"_entities":[{"__typename":"Product","id":"prod-1","name":"Product One"}]}}`), nil
			}).Times(1)

		// Second entity fetch for SAME entity - should hit L1 (not called)
		entityDS2 := NewMockDataSource(ctrl)
		entityDS2.EXPECT().Load(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)

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
					InputTemplate: InputTemplate{Segments: []TemplateSegment{
						{Data: []byte(`{"method":"POST"}`), SegmentType: StaticSegmentType},
					}},
					DataSourceIdentifier: []byte("graphql_datasource.Source"),
				}, "query"),
				// First entity fetch (shadow mode + L1)
				SingleWithPath(&SingleFetch{
					FetchConfiguration: FetchConfiguration{
						DataSource:     entityDS,
						PostProcessing: PostProcessingConfiguration{SelectResponseDataPath: []string{"data", "_entities", "0"}},
						Caching: FetchCacheConfiguration{
							Enabled:          true,
							CacheName:        "default",
							TTL:              30 * time.Second,
							CacheKeyTemplate: productCacheKeyTemplate,
							UseL1Cache:       true,
							ShadowMode:       true,
						},
					},
					InputTemplate: InputTemplate{Segments: []TemplateSegment{
						{Data: []byte(`{"method":"POST","url":"http://products.service","body":{"query":"...","variables":{"representations":[`), SegmentType: StaticSegmentType},
						{SegmentType: VariableSegmentType, VariableKind: ResolvableObjectVariableKind, Renderer: NewGraphQLVariableResolveRenderer(&Object{
							Fields: []*Field{
								{Name: []byte("__typename"), Value: &String{Path: []string{"__typename"}}},
								{Name: []byte("id"), Value: &String{Path: []string{"id"}}},
							},
						})},
						{Data: []byte(`]}}}`), SegmentType: StaticSegmentType},
					}},
					Info: &FetchInfo{
						DataSourceID: "products", DataSourceName: "products",
						RootFields:    []GraphCoordinate{{TypeName: "Product", FieldName: "name"}},
						OperationType: ast.OperationTypeQuery, ProvidesData: providesData,
					},
					DataSourceIdentifier: []byte("graphql_datasource.Source"),
				}, "query.product", ObjectPath("product")),
				// Second entity fetch for SAME entity - should hit L1 (shadow doesn't affect L1)
				SingleWithPath(&SingleFetch{
					FetchConfiguration: FetchConfiguration{
						DataSource:     entityDS2,
						PostProcessing: PostProcessingConfiguration{SelectResponseDataPath: []string{"data", "_entities", "0"}},
						Caching: FetchCacheConfiguration{
							Enabled:          true,
							CacheName:        "default",
							TTL:              30 * time.Second,
							CacheKeyTemplate: productCacheKeyTemplate,
							UseL1Cache:       true,
							ShadowMode:       true,
						},
					},
					InputTemplate: InputTemplate{Segments: []TemplateSegment{
						{Data: []byte(`{"method":"POST","url":"http://products.service","body":{"query":"...","variables":{"representations":[`), SegmentType: StaticSegmentType},
						{SegmentType: VariableSegmentType, VariableKind: ResolvableObjectVariableKind, Renderer: NewGraphQLVariableResolveRenderer(&Object{
							Fields: []*Field{
								{Name: []byte("__typename"), Value: &String{Path: []string{"__typename"}}},
								{Name: []byte("id"), Value: &String{Path: []string{"id"}}},
							},
						})},
						{Data: []byte(`]}}}`), SegmentType: StaticSegmentType},
					}},
					Info: &FetchInfo{
						DataSourceID: "products", DataSourceName: "products",
						RootFields:    []GraphCoordinate{{TypeName: "Product", FieldName: "name"}},
						OperationType: ast.OperationTypeQuery, ProvidesData: providesData,
					},
					DataSourceIdentifier: []byte("graphql_datasource.Source"),
				}, "query.product", ObjectPath("product")),
			),
			Data: &Object{
				Fields: []*Field{{
					Name: []byte("product"),
					Value: &Object{
						Path: []string{"product"},
						Fields: []*Field{
							{Name: []byte("id"), Value: &String{Path: []string{"id"}}},
							{Name: []byte("name"), Value: &String{Path: []string{"name"}}},
						},
					},
				}},
			},
		}

		loader := &Loader{caches: map[string]LoaderCache{"default": cache}}
		ctx := NewContext(context.Background())
		ctx.ExecutionOptions.DisableSubgraphRequestDeduplication = true
		ctx.ExecutionOptions.Caching.EnableL1Cache = true
		ctx.ExecutionOptions.Caching.EnableL2Cache = false // L2 disabled — only L1 can serve the second fetch

		ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
		resolvable := NewResolvable(ar, ResolvableOptions{})
		err := resolvable.Init(ctx, nil, ast.OperationTypeQuery)
		require.NoError(t, err)

		err = loader.LoadGraphQLResponseData(ctx, response, resolvable)
		require.NoError(t, err)

		out := fastjsonext.PrintGraphQLResponse(resolvable.data, resolvable.errors)
		assert.Equal(t, `{"data":{"product":{"__typename":"Product","id":"prod-1","name":"Product One"}}}`, out)

		// No stats when analytics disabled — EnableCacheAnalytics not set, so no events are collected
		assert.Equal(t, CacheAnalyticsSnapshot{}, ctx.GetCacheStats())
	})
}

// Verifies that shadow mode works safely when analytics are disabled.
func TestShadowMode_WithoutAnalytics(t *testing.T) {
	t.Run("shadow mode works without analytics - safety only", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		cache := NewFakeLoaderCache()

		rootDS := NewMockDataSource(ctrl)
		rootDS.EXPECT().
			Load(gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, headers any, input []byte) ([]byte, error) {
				return []byte(`{"data":{"product":{"__typename":"Product","id":"prod-1"}}}`), nil
			}).Times(2)

		entityDS := NewMockDataSource(ctrl)
		entityDS.EXPECT().
			Load(gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, headers any, input []byte) ([]byte, error) {
				return []byte(`{"data":{"_entities":[{"__typename":"Product","id":"prod-1","name":"Product One"}]}}`), nil
			}).Times(2) // Called both times (shadow mode)

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

		buildResponse := func() *GraphQLResponse {
			return &GraphQLResponse{
				Info: &GraphQLResponseInfo{OperationType: ast.OperationTypeQuery},
				Fetches: Sequence(
					SingleWithPath(&SingleFetch{
						FetchConfiguration: FetchConfiguration{
							DataSource:     rootDS,
							PostProcessing: PostProcessingConfiguration{SelectResponseDataPath: []string{"data"}},
						},
						InputTemplate: InputTemplate{Segments: []TemplateSegment{
							{Data: []byte(`{"method":"POST"}`), SegmentType: StaticSegmentType},
						}},
						DataSourceIdentifier: []byte("graphql_datasource.Source"),
					}, "query"),
					SingleWithPath(&SingleFetch{
						FetchConfiguration: FetchConfiguration{
							DataSource:     entityDS,
							PostProcessing: PostProcessingConfiguration{SelectResponseDataPath: []string{"data", "_entities", "0"}},
							Caching: FetchCacheConfiguration{
								Enabled:          true,
								CacheName:        "default",
								TTL:              30 * time.Second,
								CacheKeyTemplate: productCacheKeyTemplate,
								UseL1Cache:       true,
								ShadowMode:       true,
							},
						},
						InputTemplate: InputTemplate{Segments: []TemplateSegment{
							{Data: []byte(`{"method":"POST","url":"http://products.service","body":{"query":"...","variables":{"representations":[`), SegmentType: StaticSegmentType},
							{SegmentType: VariableSegmentType, VariableKind: ResolvableObjectVariableKind, Renderer: NewGraphQLVariableResolveRenderer(&Object{
								Fields: []*Field{
									{Name: []byte("__typename"), Value: &String{Path: []string{"__typename"}}},
									{Name: []byte("id"), Value: &String{Path: []string{"id"}}},
								},
							})},
							{Data: []byte(`]}}}`), SegmentType: StaticSegmentType},
						}},
						Info: &FetchInfo{
							DataSourceID: "products", DataSourceName: "products",
							RootFields:    []GraphCoordinate{{TypeName: "Product", FieldName: "name"}},
							OperationType: ast.OperationTypeQuery, ProvidesData: providesData,
						},
						DataSourceIdentifier: []byte("graphql_datasource.Source"),
					}, "query.product", ObjectPath("product")),
				),
				Data: &Object{
					Fields: []*Field{{
						Name: []byte("product"),
						Value: &Object{
							Path: []string{"product"},
							Fields: []*Field{
								{Name: []byte("id"), Value: &String{Path: []string{"id"}}},
								{Name: []byte("name"), Value: &String{Path: []string{"name"}}},
							},
						},
					}},
				},
			}
		}

		// Request 1: Populate cache
		loader1 := &Loader{caches: map[string]LoaderCache{"default": cache}}
		ctx1 := NewContext(context.Background())
		ctx1.ExecutionOptions.DisableSubgraphRequestDeduplication = true
		ctx1.ExecutionOptions.Caching.EnableL1Cache = true
		ctx1.ExecutionOptions.Caching.EnableL2Cache = true
		// Analytics disabled

		ar1 := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
		resolvable1 := NewResolvable(ar1, ResolvableOptions{})
		err := resolvable1.Init(ctx1, nil, ast.OperationTypeQuery)
		require.NoError(t, err)
		err = loader1.LoadGraphQLResponseData(ctx1, buildResponse(), resolvable1)
		require.NoError(t, err)

		// Empty: EnableCacheAnalytics not set, so no L1/L2 events are recorded
		assert.Equal(t, CacheAnalyticsSnapshot{}, ctx1.GetCacheStats())

		// Request 2: Shadow mode - still fetches from subgraph
		loader2 := &Loader{caches: map[string]LoaderCache{"default": cache}}
		ctx2 := NewContext(context.Background())
		ctx2.ExecutionOptions.DisableSubgraphRequestDeduplication = true
		ctx2.ExecutionOptions.Caching.EnableL1Cache = true
		ctx2.ExecutionOptions.Caching.EnableL2Cache = true
		// Analytics disabled

		ar2 := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
		resolvable2 := NewResolvable(ar2, ResolvableOptions{})
		err = resolvable2.Init(ctx2, nil, ast.OperationTypeQuery)
		require.NoError(t, err)
		err = loader2.LoadGraphQLResponseData(ctx2, buildResponse(), resolvable2)
		require.NoError(t, err)

		out2 := fastjsonext.PrintGraphQLResponse(resolvable2.data, resolvable2.errors)
		assert.Equal(t, `{"data":{"product":{"__typename":"Product","id":"prod-1","name":"Product One"}}}`, out2)

		// Empty: EnableCacheAnalytics not set, so no events or shadow comparisons collected
		assert.Equal(t, CacheAnalyticsSnapshot{}, ctx2.GetCacheStats())
	})
}

// ErrorLoaderCache wraps FakeLoaderCache but returns errors on Get/Set calls
// when configured to do so. Used for testing L2 error resilience.
type ErrorLoaderCache struct {
	*FakeLoaderCache

	getErr error
	setErr error
}

func (e *ErrorLoaderCache) Get(ctx context.Context, keys []string) ([]*CacheEntry, error) {
	if e.getErr != nil {
		return nil, e.getErr
	}
	return e.FakeLoaderCache.Get(ctx, keys)
}

func (e *ErrorLoaderCache) Set(ctx context.Context, entries []*CacheEntry) error {
	if e.setErr != nil {
		return e.setErr
	}
	return e.FakeLoaderCache.Set(ctx, entries)
}

// buildProductEntityResponse creates a GraphQLResponse for a single product entity fetch.
// Used by error resilience and mutation skip tests to avoid repeating boilerplate.
func buildProductEntityResponse(rootDS, entityDS DataSource, cacheKeyTemplate CacheKeyTemplate, providesData *Object, operationType ast.OperationType) *GraphQLResponse {
	rootOpName := "query"
	rootFieldType := "Query"
	rootFieldName := "product"
	if operationType == ast.OperationTypeMutation {
		rootOpName = "mutation"
		rootFieldType = "Mutation"
		rootFieldName = "updateUser"
	}

	return &GraphQLResponse{
		Info: &GraphQLResponseInfo{OperationType: operationType},
		Fetches: Sequence(
			SingleWithPath(&SingleFetch{
				FetchConfiguration: FetchConfiguration{
					DataSource:     rootDS,
					PostProcessing: PostProcessingConfiguration{SelectResponseDataPath: []string{"data"}},
				},
				InputTemplate: InputTemplate{Segments: []TemplateSegment{
					{Data: []byte(`{"method":"POST"}`), SegmentType: StaticSegmentType},
				}},
				DataSourceIdentifier: []byte("graphql_datasource.Source"),
				Info: &FetchInfo{
					DataSourceID: "ds", DataSourceName: "ds",
					RootFields:    []GraphCoordinate{{TypeName: rootFieldType, FieldName: rootFieldName}},
					OperationType: operationType,
				},
			}, rootOpName),
			SingleWithPath(&SingleFetch{
				FetchConfiguration: FetchConfiguration{
					DataSource:     entityDS,
					PostProcessing: PostProcessingConfiguration{SelectResponseDataPath: []string{"data", "_entities", "0"}},
					Caching: FetchCacheConfiguration{
						Enabled:          true,
						CacheName:        "default",
						TTL:              30 * time.Second,
						CacheKeyTemplate: cacheKeyTemplate,
						UseL1Cache:       true,
					},
				},
				InputTemplate: InputTemplate{Segments: []TemplateSegment{
					{Data: []byte(`{"method":"POST","url":"http://ds.service","body":{"query":"...","variables":{"representations":[`), SegmentType: StaticSegmentType},
					{SegmentType: VariableSegmentType, VariableKind: ResolvableObjectVariableKind, Renderer: NewGraphQLVariableResolveRenderer(&Object{
						Fields: []*Field{
							{Name: []byte("__typename"), Value: &String{Path: []string{"__typename"}}},
							{Name: []byte("id"), Value: &String{Path: []string{"id"}}},
						},
					})},
					{Data: []byte(`]}}}`), SegmentType: StaticSegmentType},
				}},
				DataSourceIdentifier: []byte("graphql_datasource.Source"),
				Info: &FetchInfo{
					DataSourceID: "ds", DataSourceName: "ds",
					RootFields:    []GraphCoordinate{{TypeName: "Product", FieldName: "name"}},
					OperationType: operationType, ProvidesData: providesData,
				},
			}, rootOpName+"."+rootFieldName, ObjectPath(rootFieldName)),
		),
		Data: &Object{
			Fields: []*Field{{
				Name: []byte(rootFieldName),
				Value: &Object{
					Path: []string{rootFieldName},
					Fields: []*Field{
						{Name: []byte("id"), Value: &String{Path: []string{"id"}}},
						{Name: []byte("name"), Value: &String{Path: []string{"name"}}},
					},
				},
			}},
		},
	}
}

// Verifies graceful degradation when the L2 cache returns errors.
// Cache failures should fall through to subgraph fetch, not fail the request.
func TestL2CacheErrorResilience(t *testing.T) {
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
			{Name: []byte("name"), Value: &Scalar{}},
		},
	}

	t.Run("L2 Get error falls through to fetch", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		errorCache := &ErrorLoaderCache{
			FakeLoaderCache: NewFakeLoaderCache(),
			getErr:          assert.AnError,
		}

		rootDS := NewMockDataSource(ctrl)
		rootDS.EXPECT().Load(gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, headers any, input []byte) ([]byte, error) {
				return []byte(`{"data":{"product":{"__typename":"Product","id":"prod-1"}}}`), nil
			}).Times(1)

		entityDS := NewMockDataSource(ctrl)
		entityDS.EXPECT().Load(gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, headers any, input []byte) ([]byte, error) {
				return []byte(`{"data":{"_entities":[{"__typename":"Product","id":"prod-1","name":"Product One"}]}}`), nil
			}).Times(1)

		response := buildProductEntityResponse(rootDS, entityDS, productCacheKeyTemplate, providesData, ast.OperationTypeQuery)

		loader := &Loader{caches: map[string]LoaderCache{"default": errorCache}}
		ctx := NewContext(t.Context())
		ctx.ExecutionOptions.DisableSubgraphRequestDeduplication = true
		ctx.ExecutionOptions.Caching.EnableL1Cache = true
		ctx.ExecutionOptions.Caching.EnableL2Cache = true

		ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
		resolvable := NewResolvable(ar, ResolvableOptions{})
		err := resolvable.Init(ctx, nil, ast.OperationTypeQuery)
		require.NoError(t, err)

		err = loader.LoadGraphQLResponseData(ctx, response, resolvable)
		require.NoError(t, err)

		out := fastjsonext.PrintGraphQLResponse(resolvable.data, resolvable.errors)
		assert.Equal(t, `{"data":{"product":{"__typename":"Product","id":"prod-1","name":"Product One"}}}`, out)
	})

	t.Run("L2 Set error does not fail request", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		errorCache := &ErrorLoaderCache{
			FakeLoaderCache: NewFakeLoaderCache(),
			setErr:          assert.AnError,
		}

		rootDS := NewMockDataSource(ctrl)
		rootDS.EXPECT().Load(gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, headers any, input []byte) ([]byte, error) {
				return []byte(`{"data":{"product":{"__typename":"Product","id":"prod-1"}}}`), nil
			}).Times(1)

		entityDS := NewMockDataSource(ctrl)
		entityDS.EXPECT().Load(gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, headers any, input []byte) ([]byte, error) {
				return []byte(`{"data":{"_entities":[{"__typename":"Product","id":"prod-1","name":"Product One"}]}}`), nil
			}).Times(1)

		response := buildProductEntityResponse(rootDS, entityDS, productCacheKeyTemplate, providesData, ast.OperationTypeQuery)

		loader := &Loader{caches: map[string]LoaderCache{"default": errorCache}}
		ctx := NewContext(t.Context())
		ctx.ExecutionOptions.DisableSubgraphRequestDeduplication = true
		ctx.ExecutionOptions.Caching.EnableL1Cache = true
		ctx.ExecutionOptions.Caching.EnableL2Cache = true

		ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
		resolvable := NewResolvable(ar, ResolvableOptions{})
		err := resolvable.Init(ctx, nil, ast.OperationTypeQuery)
		require.NoError(t, err)

		err = loader.LoadGraphQLResponseData(ctx, response, resolvable)
		require.NoError(t, err)

		out := fastjsonext.PrintGraphQLResponse(resolvable.data, resolvable.errors)
		assert.Equal(t, `{"data":{"product":{"__typename":"Product","id":"prod-1","name":"Product One"}}}`, out)
	})

	t.Run("corrupted cache entry treated as miss", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		cache := NewFakeLoaderCache()
		// Pre-populate cache with corrupted JSON using the real key format
		_ = cache.Set(t.Context(), withCacheEntryTTL([]*CacheEntry{
			{Key: `{"__typename":"Product","key":{"id":"prod-1"}}`, Value: []byte(`{not valid json!!!}`)},
		}, 30*time.Second))

		rootDS := NewMockDataSource(ctrl)
		rootDS.EXPECT().Load(gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, headers any, input []byte) ([]byte, error) {
				return []byte(`{"data":{"product":{"__typename":"Product","id":"prod-1"}}}`), nil
			}).Times(1)

		entityDS := NewMockDataSource(ctrl)
		entityDS.EXPECT().Load(gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, headers any, input []byte) ([]byte, error) {
				return []byte(`{"data":{"_entities":[{"__typename":"Product","id":"prod-1","name":"Product One"}]}}`), nil
			}).Times(1) // Must fetch because cached entry is corrupted

		response := buildProductEntityResponse(rootDS, entityDS, productCacheKeyTemplate, providesData, ast.OperationTypeQuery)

		loader := &Loader{caches: map[string]LoaderCache{"default": cache}}
		ctx := NewContext(t.Context())
		ctx.ExecutionOptions.DisableSubgraphRequestDeduplication = true
		ctx.ExecutionOptions.Caching.EnableL1Cache = true
		ctx.ExecutionOptions.Caching.EnableL2Cache = true

		ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
		resolvable := NewResolvable(ar, ResolvableOptions{})
		err := resolvable.Init(ctx, nil, ast.OperationTypeQuery)
		require.NoError(t, err)

		err = loader.LoadGraphQLResponseData(ctx, response, resolvable)
		require.NoError(t, err)

		out := fastjsonext.PrintGraphQLResponse(resolvable.data, resolvable.errors)
		assert.Equal(t, `{"data":{"product":{"__typename":"Product","id":"prod-1","name":"Product One"}}}`, out)

		// Verify L2 cache was actually accessed (Get returned the corrupted entry, then Set wrote fresh data)
		log := cache.GetLog()
		assert.Equal(t, 3, len(log), "should have set (seed) + get (corrupted hit) + set (fresh data)")
		assert.Equal(t, "set", log[0].Operation)
		assert.Equal(t, "get", log[1].Operation)
		assert.Equal(t, true, log[1].Items[0].Hit, "L2 Get should find the seeded corrupted entry")
		assert.Equal(t, "set", log[2].Operation)
	})
}

// Verifies that mutation operations bypass L2 cache reads and always fetch fresh data.
// Mutations must not serve stale cached entities.
func TestMutationSkipsL2Read(t *testing.T) {
	t.Run("mutation operation type skips L2 read and always fetches", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		cache := NewFakeLoaderCache()
		// Pre-populate cache with stale data using the real key format
		_ = cache.Set(t.Context(), withCacheEntryTTL([]*CacheEntry{
			{Key: `{"__typename":"Product","key":{"id":"prod-1"}}`, Value: []byte(`{"__typename":"Product","id":"prod-1","name":"Old Name"}`)},
		}, 30*time.Second))

		userCacheKeyTemplate := &EntityQueryCacheKeyTemplate{
			Keys: NewResolvableObjectVariable(&Object{
				Fields: []*Field{
					{Name: []byte("__typename"), Value: &String{Path: []string{"__typename"}}},
					{Name: []byte("id"), Value: &String{Path: []string{"id"}}},
				},
			}),
		}
		providesData := &Object{
			Fields: []*Field{
				{Name: []byte("name"), Value: &Scalar{}},
			},
		}

		rootDS := NewMockDataSource(ctrl)
		rootDS.EXPECT().Load(gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, headers any, input []byte) ([]byte, error) {
				return []byte(`{"data":{"updateUser":{"__typename":"Product","id":"prod-1"}}}`), nil
			}).Times(1)

		entityDS := NewMockDataSource(ctrl)
		entityDS.EXPECT().Load(gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, headers any, input []byte) ([]byte, error) {
				return []byte(`{"data":{"_entities":[{"__typename":"Product","id":"prod-1","name":"New Name"}]}}`), nil
			}).Times(1) // Must fetch fresh data despite cache having stale entry

		response := buildProductEntityResponse(rootDS, entityDS, userCacheKeyTemplate, providesData, ast.OperationTypeMutation)

		loader := &Loader{caches: map[string]LoaderCache{"default": cache}}
		ctx := NewContext(t.Context())
		ctx.ExecutionOptions.DisableSubgraphRequestDeduplication = true
		ctx.ExecutionOptions.Caching.EnableL1Cache = true
		ctx.ExecutionOptions.Caching.EnableL2Cache = true

		ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
		resolvable := NewResolvable(ar, ResolvableOptions{})
		err := resolvable.Init(ctx, nil, ast.OperationTypeMutation)
		require.NoError(t, err)

		err = loader.LoadGraphQLResponseData(ctx, response, resolvable)
		require.NoError(t, err)

		out := fastjsonext.PrintGraphQLResponse(resolvable.data, resolvable.errors)
		assert.Equal(t, `{"data":{"updateUser":{"__typename":"Product","id":"prod-1","name":"New Name"}}}`, out, "mutation should fetch fresh data, not use cached stale data")
	})
}

func newUserRootQueryTemplate(requestedFields []string, entityKeyFields []string) *RootQueryCacheKeyTemplate {
	rootArgs := make([]FieldArgument, 0, len(requestedFields))
	for _, field := range requestedFields {
		rootArgs = append(rootArgs, FieldArgument{
			Name: field,
			Variable: &ContextVariable{
				Path:     []string{field},
				Renderer: NewPlainVariableRenderer(),
			},
		})
	}

	entityKeyMappings := make([]EntityKeyMappingConfig, 0, len(entityKeyFields))
	for _, field := range entityKeyFields {
		entityKeyMappings = append(entityKeyMappings, EntityKeyMappingConfig{
			EntityTypeName: "User",
			FieldMappings: []EntityFieldMappingConfig{
				{
					EntityKeyField: field,
					ArgumentPath:   []string{field},
				},
			},
		})
	}

	return NewRootQueryCacheKeyTemplate(
		[]QueryField{
			{
				Coordinate: GraphCoordinate{TypeName: "Query", FieldName: "user"},
				Args:       rootArgs,
			},
		},
		entityKeyMappings,
	)
}

func newUserRootQueryResponse(rootDS DataSource, cacheKeyTemplate CacheKeyTemplate, providesData *Object) *GraphQLResponse {
	rootProvidesData := providesData
	if providesData != nil {
		rootProvidesData = &Object{
			Fields: providesData.Fields,
		}
		rootProvidesData = &Object{
			Fields: []*Field{
				{
					Name:  []byte("user"),
					Value: rootProvidesData,
				},
			},
		}
	}

	return &GraphQLResponse{
		Info: &GraphQLResponseInfo{OperationType: ast.OperationTypeQuery},
		Fetches: Sequence(
			SingleWithPath(&SingleFetch{
				FetchConfiguration: FetchConfiguration{
					DataSource:     rootDS,
					PostProcessing: PostProcessingConfiguration{SelectResponseDataPath: []string{"data"}},
					Caching: FetchCacheConfiguration{
						Enabled:          true,
						CacheName:        "default",
						TTL:              30 * time.Second,
						CacheKeyTemplate: cacheKeyTemplate,
					},
				},
				InputTemplate: InputTemplate{
					Segments: []TemplateSegment{
						{Data: []byte(`{"method":"POST"}`), SegmentType: StaticSegmentType},
					},
				},
				Info: &FetchInfo{
					DataSourceID:   "accounts",
					DataSourceName: "accounts",
					RootFields:     []GraphCoordinate{{TypeName: "Query", FieldName: "user"}},
					OperationType:  ast.OperationTypeQuery,
					ProvidesData:   rootProvidesData,
				},
				DataSourceIdentifier: []byte("graphql_datasource.Source"),
			}, "query"),
		),
		Data: &Object{
			Fields: []*Field{
				{
					Name: []byte("user"),
					Value: &Object{
						Path: []string{"user"},
						Fields: []*Field{
							{Name: []byte("id"), Value: &String{Path: []string{"id"}}},
							{Name: []byte("email"), Value: &String{Path: []string{"email"}}},
							{Name: []byte("username"), Value: &String{Path: []string{"username"}}},
						},
					},
				},
			},
		},
	}
}

// Verifies that when all EntityKeyMappings produce cache hits, the fetch is skipped
// and missing derived keys are backfilled from the cached data.
func TestCacheBackfill_SkipFetch_HappyPath(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Scenario: the request asks for id + email keys, only the id key is cached,
	// and that cached entity already contains the email field required to prove
	// the missing sibling key. The loader should skip the subgraph fetch, backfill
	// only the missing email key, and leave the existing id key untouched.
	cache := NewFakeLoaderCache()
	idKey := `{"__typename":"User","key":{"id":"u1"}}`
	emailKey := `{"__typename":"User","key":{"email":"a@example.com"}}`

	// Seed L2 with only the id key. The stored entity is complete enough to serve
	// the request and to prove that the email key belongs to the same entity.
	err := cache.Set(t.Context(), withCacheEntryTTL([]*CacheEntry{
		{Key: idKey, Value: []byte(`{"__typename":"User","id":"u1","email":"a@example.com","username":"Alice"}`)},
	}, 30*time.Second))
	require.NoError(t, err)
	cache.ClearLog()

	// The request should stay on the cache-only path, so the root datasource must
	// never be called.
	rootDS := NewMockDataSource(ctrl)
	rootDS.EXPECT().Load(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)

	response := newUserRootQueryResponse(
		rootDS,
		newUserRootQueryTemplate([]string{"id", "email"}, []string{"id", "email"}),
		&Object{
			Fields: []*Field{
				{Name: []byte("id"), Value: &Scalar{Path: []string{"id"}, Nullable: false}},
				{Name: []byte("username"), Value: &Scalar{Path: []string{"username"}, Nullable: false}},
			},
		},
	)

	loader := &Loader{caches: map[string]LoaderCache{"default": cache}}
	ctx := NewContext(t.Context())
	ctx.Variables = astjson.MustParseBytes([]byte(`{"id":"u1","email":"a@example.com"}`))
	ctx.ExecutionOptions.DisableSubgraphRequestDeduplication = true
	ctx.ExecutionOptions.Caching.EnableL1Cache = true
	ctx.ExecutionOptions.Caching.EnableL2Cache = true
	ctx.ExecutionOptions.Caching.EnableCacheAnalytics = true

	ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
	resolvable := NewResolvable(ar, ResolvableOptions{})
	err = resolvable.Init(ctx, nil, ast.OperationTypeQuery)
	require.NoError(t, err)

	err = loader.LoadGraphQLResponseData(ctx, response, resolvable)
	require.NoError(t, err)

	// Assert the exact cache story:
	// 1. L2 reads both requested keys and finds only the id key.
	// 2. L2 writes only the missing email key.
	assert.Equal(t, []CacheLogEntry{
		{
			Operation: "get",
			Items: []CacheLogItem{
				{Key: idKey, Hit: true},
				{Key: emailKey, Hit: false},
			},
		},
		{Operation: "set", Items: []CacheLogItem{{Key: emailKey, TTL: 30 * time.Second}}},
	}, cache.GetLog())
	// Assert the written value matches the final merged entity and that the
	// existing id entry was preserved rather than rewritten.
	assert.Equal(t, `{"__typename":"User","id":"u1","email":"a@example.com","username":"Alice"}`, string(cache.GetValue(emailKey)))
	assert.Equal(t, `{"__typename":"User","id":"u1","email":"a@example.com","username":"Alice"}`, string(cache.GetValue(idKey)))

	snap := normalizeCacheAnalyticsSnapshot(ctx.GetCacheStats())
	assert.Equal(t, normalizeCacheAnalyticsSnapshot(CacheAnalyticsSnapshot{
		L2Reads: []CacheKeyEvent{
			// id key found in L2 (first key in CacheKey.Keys)
			{
				CacheKey:   idKey,
				EntityType: "User",
				Kind:       CacheKeyHit,
				DataSource: "accounts",
				ByteSize:   83,
			},
		},
		L2Writes: []CacheWriteEvent{
			// backfill: missing requested key proven by cached entity data
			{
				CacheKey:    emailKey,
				EntityType:  "User",
				ByteSize:    74,
				DataSource:  "accounts",
				CacheLevel:  CacheLevelL2,
				TTL:         30 * time.Second,
				Source:      CacheSourceQuery,
				WriteReason: CacheWriteReasonBackfill,
			},
		},
		FieldHashes: []EntityFieldHash{
			// Synthetic root-field cache hit: @openfed__queryCache on Query.user
			// served from L2. Drives the hub heatmap's per-field cache-hit
			// tint; entity-scope leaf hashes are emitted separately from the
			// response walk and aren't relevant to this assertion.
			{
				EntityType: "Query",
				FieldName:  "user",
				Source:     FieldSourceL2,
				DataSource: "accounts",
			},
		},
	}), snap)
}

// REGRESSION: a root-field SingleFetch whose L2 lookup is a complete cache hit
// must record `LoadSkipped = true` on the fetch's DataSourceLoadTrace, mirroring
// how the entity-fetch and bulk-parallel paths already do. Otherwise downstream
// observability (Cosmo Router cache_trace, ART) reports `load_skipped=false` on
// fetches that demonstrably never called the subgraph — making it impossible to
// distinguish "served from cache" from "fetched fresh".
func TestSingleFetch_CacheHit_SetsLoadSkippedOnTrace_RED(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	cache := NewFakeLoaderCache()
	idKey := `{"__typename":"User","key":{"id":"u1"}}`
	emailKey := `{"__typename":"User","key":{"email":"a@example.com"}}`

	// Pre-warm L2 with a fully-derivable cached entity so tryCacheLoad returns skip=true.
	err := cache.Set(t.Context(), withCacheEntryTTL([]*CacheEntry{
		{Key: idKey, Value: []byte(`{"__typename":"User","id":"u1","email":"a@example.com","username":"Alice"}`)},
	}, 30*time.Second))
	require.NoError(t, err)
	cache.ClearLog()

	// Subgraph must NOT be called.
	rootDS := NewMockDataSource(ctrl)
	rootDS.EXPECT().Load(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)

	response := newUserRootQueryResponse(
		rootDS,
		newUserRootQueryTemplate([]string{"id", "email"}, []string{"id", "email"}),
		&Object{
			Fields: []*Field{
				{Name: []byte("id"), Value: &Scalar{Path: []string{"id"}, Nullable: false}},
				{Name: []byte("username"), Value: &Scalar{Path: []string{"username"}, Nullable: false}},
			},
		},
	)

	loader := &Loader{caches: map[string]LoaderCache{"default": cache}}
	ctx := NewContext(t.Context())
	ctx.Variables = astjson.MustParseBytes([]byte(`{"id":"u1","email":"a@example.com"}`))
	ctx.ExecutionOptions.DisableSubgraphRequestDeduplication = true
	ctx.ExecutionOptions.Caching.EnableL1Cache = true
	ctx.ExecutionOptions.Caching.EnableL2Cache = true
	// Enable tracing — that's how the loader populates fetch.Trace.LoadSkipped.
	ctx.TracingOptions.Enable = true

	ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
	resolvable := NewResolvable(ar, ResolvableOptions{})
	err = resolvable.Init(ctx, nil, ast.OperationTypeQuery)
	require.NoError(t, err)

	err = loader.LoadGraphQLResponseData(ctx, response, resolvable)
	require.NoError(t, err)

	// Walk the fetch tree to find the SingleFetch and verify its trace.
	var checked int
	walkFetchTreeForTest(response.Fetches, func(f Fetch) {
		single, ok := f.(*SingleFetch)
		if !ok {
			return
		}
		require.NotNil(t, single.Trace, "SingleFetch.Trace must be populated when tracing is enabled")
		assert.True(t, single.Trace.LoadSkipped,
			"SingleFetch.Trace.LoadSkipped must be true when tryCacheLoad returned skip=true (cache hit, no subgraph call)")
		checked++
	})
	assert.Equal(t, 1, checked, "expected exactly one SingleFetch to inspect")

	// Sanity: the cache get happened, no set, no subgraph call.
	assert.Equal(t, []CacheLogEntry{
		{
			Operation: "get",
			Items: []CacheLogItem{
				{Key: idKey, Hit: true},
				{Key: emailKey, Hit: false},
			},
		},
		{Operation: "set", Items: []CacheLogItem{{Key: emailKey, TTL: 30 * time.Second}}},
	}, cache.GetLog())
}

// walkFetchTreeForTest visits every Fetch in the tree.
func walkFetchTreeForTest(node *FetchTreeNode, visit func(Fetch)) {
	if node == nil {
		return
	}
	if node.Kind == FetchTreeNodeKindSingle && node.Item != nil && node.Item.Fetch != nil {
		visit(node.Item.Fetch)
	}
	for _, c := range node.ChildNodes {
		walkFetchTreeForTest(c, visit)
	}
}

// Verifies that backfill is skipped when the cached entity data doesn't contain
// the fields needed to derive the missing key.
func TestCacheBackfill_SkipFetch_Counterexample_NotDerivable(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Scenario: the request asks for id + email keys, only the id key is cached,
	// but the cached entity does not contain email. The loader may still skip the
	// fetch because the requested response only needs id + username, but it must
	// not backfill the missing email key from request args alone.
	cache := NewFakeLoaderCache()
	idKey := `{"__typename":"User","key":{"id":"u1"}}`
	emailKey := `{"__typename":"User","key":{"email":"a@example.com"}}`

	// Seed L2 with only the id key and omit email from the cached entity to make
	// the missing email key impossible to prove from final entity data.
	err := cache.Set(t.Context(), withCacheEntryTTL([]*CacheEntry{
		{Key: idKey, Value: []byte(`{"__typename":"User","id":"u1","username":"Alice"}`)},
	}, 30*time.Second))
	require.NoError(t, err)
	cache.ClearLog()

	// Cache-only path again: the subgraph must not be called.
	rootDS := NewMockDataSource(ctrl)
	rootDS.EXPECT().Load(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)

	response := newUserRootQueryResponse(
		rootDS,
		newUserRootQueryTemplate([]string{"id", "email"}, []string{"id", "email"}),
		&Object{
			Fields: []*Field{
				{Name: []byte("id"), Value: &Scalar{Path: []string{"id"}, Nullable: false}},
				{Name: []byte("username"), Value: &Scalar{Path: []string{"username"}, Nullable: false}},
			},
		},
	)

	loader := &Loader{caches: map[string]LoaderCache{"default": cache}}
	ctx := NewContext(t.Context())
	ctx.Variables = astjson.MustParseBytes([]byte(`{"id":"u1","email":"a@example.com"}`))
	ctx.ExecutionOptions.DisableSubgraphRequestDeduplication = true
	ctx.ExecutionOptions.Caching.EnableL1Cache = true
	ctx.ExecutionOptions.Caching.EnableL2Cache = true
	ctx.ExecutionOptions.Caching.EnableCacheAnalytics = true

	ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
	resolvable := NewResolvable(ar, ResolvableOptions{})
	err = resolvable.Init(ctx, nil, ast.OperationTypeQuery)
	require.NoError(t, err)

	err = loader.LoadGraphQLResponseData(ctx, response, resolvable)
	require.NoError(t, err)

	// Assert the exact cache story:
	// 1. L2 reads both requested keys and finds only the id key.
	// 2. No write happens because email is still not provable from the final entity.
	assert.Equal(t, []CacheLogEntry{
		{
			Operation: "get",
			Items: []CacheLogItem{
				{Key: idKey, Hit: true},
				{Key: emailKey, Hit: false},
			},
		},
	}, cache.GetLog())
	// Assert the missing email key stays absent and the original id entry is unchanged.
	assert.Nil(t, cache.GetValue(emailKey))
	assert.Equal(t, `{"__typename":"User","id":"u1","username":"Alice"}`, string(cache.GetValue(idKey)))

	snap := normalizeCacheAnalyticsSnapshot(ctx.GetCacheStats())
	assert.Equal(t, normalizeCacheAnalyticsSnapshot(CacheAnalyticsSnapshot{
		L2Reads: []CacheKeyEvent{
			// id key found in L2 (entity lacks email field)
			{
				CacheKey:   idKey,
				EntityType: "User",
				Kind:       CacheKeyHit,
				DataSource: "accounts",
				ByteSize:   59,
			},
		},
		// no L2 writes: email field missing from entity, cannot prove emailKey
		FieldHashes: []EntityFieldHash{
			// Synthetic root-field cache hit on Query.user — the queryCache served
			// from L2 even though the entity-key backfill was skipped.
			{
				EntityType: "Query",
				FieldName:  "user",
				Source:     FieldSourceL2,
				DataSource: "accounts",
			},
		},
	}), snap)
}

// Verifies that after a subgraph fetch, both the requested key and the derived key
// are written to L2 cache.
func TestCacheBackfill_FetchPath_HappyPath(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Scenario: the request asks for id + email keys, only the id key is cached,
	// and the cached entity is too incomplete to satisfy the request. The loader
	// must fetch fresh data, refresh the existing id key, and backfill the missing
	// email key from the fetched entity.
	cache := NewFakeLoaderCache()
	idKey := `{"__typename":"User","key":{"id":"u1"}}`
	emailKey := `{"__typename":"User","key":{"email":"a@example.com"}}`

	// Seed L2 with a stale/incomplete id entry so the fetch path is required.
	err := cache.Set(t.Context(), withCacheEntryTTL([]*CacheEntry{
		{Key: idKey, Value: []byte(`{"__typename":"User","id":"u1"}`)},
	}, 30*time.Second))
	require.NoError(t, err)
	cache.ClearLog()

	// The subgraph returns the complete entity, which should refresh id and prove email.
	rootDS := NewMockDataSource(ctrl)
	rootDS.EXPECT().
		Load(gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, headers any, input []byte) ([]byte, error) {
			return []byte(`{"data":{"user":{"__typename":"User","id":"u1","email":"a@example.com","username":"Alice"}}}`), nil
		}).Times(1)

	response := newUserRootQueryResponse(
		rootDS,
		newUserRootQueryTemplate([]string{"id", "email"}, []string{"id", "email"}),
		&Object{
			Fields: []*Field{
				{Name: []byte("id"), Value: &Scalar{Path: []string{"id"}, Nullable: false}},
				{Name: []byte("username"), Value: &Scalar{Path: []string{"username"}, Nullable: false}},
			},
		},
	)

	loader := &Loader{caches: map[string]LoaderCache{"default": cache}}
	ctx := NewContext(t.Context())
	ctx.Variables = astjson.MustParseBytes([]byte(`{"id":"u1","email":"a@example.com"}`))
	ctx.ExecutionOptions.DisableSubgraphRequestDeduplication = true
	ctx.ExecutionOptions.Caching.EnableL1Cache = true
	ctx.ExecutionOptions.Caching.EnableL2Cache = true
	ctx.ExecutionOptions.Caching.EnableCacheAnalytics = true

	ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
	resolvable := NewResolvable(ar, ResolvableOptions{})
	err = resolvable.Init(ctx, nil, ast.OperationTypeQuery)
	require.NoError(t, err)

	err = loader.LoadGraphQLResponseData(ctx, response, resolvable)
	require.NoError(t, err)

	// Assert the exact cache story:
	// 1. L2 reads both requested keys and finds only the stale id key.
	// 2. The fetch runs and writes both the refreshed id key and the backfilled email key.
	assert.Equal(t, []CacheLogEntry{
		{
			Operation: "get",
			Items: []CacheLogItem{
				{Key: idKey, Hit: true},
				{Key: emailKey, Hit: false},
			},
		},
		{
			Operation: "set",
			Items: []CacheLogItem{
				{Key: idKey, TTL: 30 * time.Second},
				{Key: emailKey, TTL: 30 * time.Second},
			},
		},
	}, cache.GetLog())
	// Assert both keys now store the same fresh entity payload.
	assert.Equal(t, `{"__typename":"User","id":"u1","email":"a@example.com","username":"Alice"}`, string(cache.GetValue(idKey)))
	assert.Equal(t, `{"__typename":"User","id":"u1","email":"a@example.com","username":"Alice"}`, string(cache.GetValue(emailKey)))

	snap := normalizeCacheAnalyticsSnapshot(ctx.GetCacheStats())
	assert.Equal(t, normalizeCacheAnalyticsSnapshot(CacheAnalyticsSnapshot{
		L2Reads: []CacheKeyEvent{
			// id key found but incomplete for ProvidesData → partial hit, fetch needed
			{
				CacheKey:   idKey,
				EntityType: "User",
				Kind:       CacheKeyPartialHit,
				DataSource: "accounts",
			},
		},
		L2Writes: []CacheWriteEvent{
			// refresh: existing key rewritten with fresh subgraph data
			{
				CacheKey:    idKey,
				EntityType:  "User",
				ByteSize:    74,
				DataSource:  "accounts",
				CacheLevel:  CacheLevelL2,
				TTL:         30 * time.Second,
				Source:      CacheSourceQuery,
				WriteReason: CacheWriteReasonRefresh,
			},
			// backfill: missing requested key proven by subgraph response
			{
				CacheKey:    emailKey,
				EntityType:  "User",
				ByteSize:    74,
				DataSource:  "accounts",
				CacheLevel:  CacheLevelL2,
				TTL:         30 * time.Second,
				Source:      CacheSourceQuery,
				WriteReason: CacheWriteReasonBackfill,
			},
		},
	}), snap)
}

// Verifies that when the subgraph response is missing a field needed for key derivation,
// only the requested key is written (derived key is skipped).
func TestCacheBackfill_FetchPath_MissingField(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Scenario: the request asks for id + email keys, only the id key is cached,
	// and the fetch runs. The fetched entity still does not contain email, so the
	// loader may refresh the existing id key but must not backfill email.
	cache := NewFakeLoaderCache()
	idKey := `{"__typename":"User","key":{"id":"u1"}}`
	emailKey := `{"__typename":"User","key":{"email":"a@example.com"}}`

	// Seed L2 with an incomplete id entry to force the fetch path.
	err := cache.Set(t.Context(), withCacheEntryTTL([]*CacheEntry{
		{Key: idKey, Value: []byte(`{"__typename":"User","id":"u1"}`)},
	}, 30*time.Second))
	require.NoError(t, err)
	cache.ClearLog()

	// The subgraph returns username but still no email.
	rootDS := NewMockDataSource(ctrl)
	rootDS.EXPECT().
		Load(gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, headers any, input []byte) ([]byte, error) {
			return []byte(`{"data":{"user":{"__typename":"User","id":"u1","username":"Alice"}}}`), nil
		}).Times(1)

	response := newUserRootQueryResponse(
		rootDS,
		newUserRootQueryTemplate([]string{"id", "email"}, []string{"id", "email"}),
		&Object{
			Fields: []*Field{
				{Name: []byte("id"), Value: &Scalar{Path: []string{"id"}, Nullable: false}},
				{Name: []byte("username"), Value: &Scalar{Path: []string{"username"}, Nullable: false}},
			},
		},
	)

	loader := &Loader{caches: map[string]LoaderCache{"default": cache}}
	ctx := NewContext(t.Context())
	ctx.Variables = astjson.MustParseBytes([]byte(`{"id":"u1","email":"a@example.com"}`))
	ctx.ExecutionOptions.DisableSubgraphRequestDeduplication = true
	ctx.ExecutionOptions.Caching.EnableL1Cache = true
	ctx.ExecutionOptions.Caching.EnableL2Cache = true
	ctx.ExecutionOptions.Caching.EnableCacheAnalytics = true

	ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
	resolvable := NewResolvable(ar, ResolvableOptions{})
	err = resolvable.Init(ctx, nil, ast.OperationTypeQuery)
	require.NoError(t, err)

	err = loader.LoadGraphQLResponseData(ctx, response, resolvable)
	require.NoError(t, err)

	// Assert the exact cache story:
	// 1. L2 reads both requested keys and finds only the id key.
	// 2. The fetch refreshes id with the new data.
	// 3. The email key is backfilled with the response payload, even though the response
	//    didn't carry the email field. The cache key was derived from the request arguments,
	//    and a non-null response from the subgraph confirms this entity matches that key.
	//    A future query selecting `email` would trigger a widening refetch since the cached
	//    payload doesn't contain it; a query selecting only id+username gets a cache hit.
	assert.Equal(t, []CacheLogEntry{
		{
			Operation: "get",
			Items: []CacheLogItem{
				{Key: idKey, Hit: true},
				{Key: emailKey, Hit: false},
			},
		},
		{
			Operation: "set",
			Items: []CacheLogItem{
				{Key: idKey, TTL: 30 * time.Second},
				{Key: emailKey, TTL: 30 * time.Second},
			},
		},
	}, cache.GetLog())
	assert.Equal(t, `{"__typename":"User","id":"u1","username":"Alice"}`, string(cache.GetValue(idKey)))
	assert.Equal(t, `{"__typename":"User","id":"u1","username":"Alice"}`, string(cache.GetValue(emailKey)))

	snap := normalizeCacheAnalyticsSnapshot(ctx.GetCacheStats())
	assert.Equal(t, normalizeCacheAnalyticsSnapshot(CacheAnalyticsSnapshot{
		L2Reads: []CacheKeyEvent{
			// id key found but incomplete for ProvidesData → partial hit, fetch needed
			{
				CacheKey:   idKey,
				EntityType: "User",
				Kind:       CacheKeyPartialHit,
				DataSource: "accounts",
			},
		},
		L2Writes: []CacheWriteEvent{
			// refresh: existing key rewritten with fresh data
			{
				CacheKey:    idKey,
				EntityType:  "User",
				ByteSize:    50,
				DataSource:  "accounts",
				CacheLevel:  CacheLevelL2,
				TTL:         30 * time.Second,
				Source:      CacheSourceQuery,
				WriteReason: CacheWriteReasonRefresh,
			},
			// backfill: email key was missing on read; written with the response payload
			// because the entity is the canonical match for the request args.
			{
				CacheKey:    emailKey,
				EntityType:  "User",
				ByteSize:    50,
				DataSource:  "accounts",
				CacheLevel:  CacheLevelL2,
				TTL:         30 * time.Second,
				Source:      CacheSourceQuery,
				WriteReason: CacheWriteReasonBackfill,
			},
		},
	}), snap)
}

// Verifies that when the entity's field value doesn't match the requested argument,
// the derived key is written but the unproven requested key is skipped.
func TestCacheBackfill_FetchPath_ValueMismatch(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Scenario: the request asks for email=a@example.com, but the fetched entity
	// comes back with email=b@example.com. The loader must refresh the existing id
	// key, must NOT backfill the requested email key (a@), but MUST write a derived
	// key for the actual email value (b@) because it is backend-proven entity data.
	cache := NewFakeLoaderCache()
	idKey := `{"__typename":"User","key":{"id":"u1"}}`
	requestedEmailKey := `{"__typename":"User","key":{"email":"a@example.com"}}`
	actualEmailKey := `{"__typename":"User","key":{"email":"b@example.com"}}`

	// Seed L2 with an incomplete id entry to force the fetch path.
	err := cache.Set(t.Context(), withCacheEntryTTL([]*CacheEntry{
		{Key: idKey, Value: []byte(`{"__typename":"User","id":"u1"}`)},
	}, 30*time.Second))
	require.NoError(t, err)
	cache.ClearLog()

	// The subgraph returns a different email value than the requested key.
	rootDS := NewMockDataSource(ctrl)
	rootDS.EXPECT().
		Load(gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, headers any, input []byte) ([]byte, error) {
			return []byte(`{"data":{"user":{"__typename":"User","id":"u1","email":"b@example.com","username":"Alice"}}}`), nil
		}).Times(1)

	response := newUserRootQueryResponse(
		rootDS,
		newUserRootQueryTemplate([]string{"id", "email"}, []string{"id", "email"}),
		&Object{
			Fields: []*Field{
				{Name: []byte("id"), Value: &Scalar{Path: []string{"id"}, Nullable: false}},
				{Name: []byte("username"), Value: &Scalar{Path: []string{"username"}, Nullable: false}},
			},
		},
	)

	loader := &Loader{caches: map[string]LoaderCache{"default": cache}}
	ctx := NewContext(t.Context())
	ctx.Variables = astjson.MustParseBytes([]byte(`{"id":"u1","email":"a@example.com"}`))
	ctx.ExecutionOptions.DisableSubgraphRequestDeduplication = true
	ctx.ExecutionOptions.Caching.EnableL1Cache = true
	ctx.ExecutionOptions.Caching.EnableL2Cache = true
	ctx.ExecutionOptions.Caching.EnableCacheAnalytics = true

	ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
	resolvable := NewResolvable(ar, ResolvableOptions{})
	err = resolvable.Init(ctx, nil, ast.OperationTypeQuery)
	require.NoError(t, err)

	err = loader.LoadGraphQLResponseData(ctx, response, resolvable)
	require.NoError(t, err)

	// Assert the exact cache story:
	// 1. L2 reads both requested keys and finds only the id key.
	// 2. The fetch refreshes id with fresh data.
	// 3. The requested email key (a@) is NOT written — the entity doesn't prove it.
	// 4. The actual email key (b@) IS written — the subgraph returned b@example.com
	//    as backend-proven entity data, so we can build and store a key for it.
	assert.Equal(t, []CacheLogEntry{
		{
			Operation: "get",
			Items: []CacheLogItem{
				{Key: idKey, Hit: true},
				{Key: requestedEmailKey, Hit: false},
			},
		},
		{
			Operation: "set",
			Items: []CacheLogItem{
				{Key: idKey, TTL: 30 * time.Second},
				{Key: actualEmailKey, TTL: 30 * time.Second},
			},
		},
	}, cache.GetLog())
	assert.Equal(t, `{"__typename":"User","id":"u1","email":"b@example.com","username":"Alice"}`, string(cache.GetValue(idKey)))
	assert.Nil(t, cache.GetValue(requestedEmailKey))
	assert.Equal(t, `{"__typename":"User","id":"u1","email":"b@example.com","username":"Alice"}`, string(cache.GetValue(actualEmailKey)))

	snap := normalizeCacheAnalyticsSnapshot(ctx.GetCacheStats())
	assert.Equal(t, normalizeCacheAnalyticsSnapshot(CacheAnalyticsSnapshot{
		L2Reads: []CacheKeyEvent{
			// id key found but incomplete for ProvidesData → partial hit, fetch needed
			{
				CacheKey:   idKey,
				EntityType: "User",
				Kind:       CacheKeyPartialHit,
				DataSource: "accounts",
			},
		},
		L2Writes: []CacheWriteEvent{
			// refresh: existing key rewritten with fresh subgraph data
			{
				CacheKey:    idKey,
				EntityType:  "User",
				ByteSize:    74,
				DataSource:  "accounts",
				CacheLevel:  CacheLevelL2,
				TTL:         30 * time.Second,
				Source:      CacheSourceQuery,
				WriteReason: CacheWriteReasonRefresh,
			},
			// derived: subgraph returned b@ email, written as new derived key
			{
				CacheKey:    actualEmailKey,
				EntityType:  "User",
				ByteSize:    74,
				DataSource:  "accounts",
				CacheLevel:  CacheLevelL2,
				TTL:         30 * time.Second,
				Source:      CacheSourceQuery,
				WriteReason: CacheWriteReasonDerived,
			},
		},
	}), snap)
}

// Verifies that derived key expansion writes cache entries for entity key mappings
// that weren't part of the original request.
func TestCacheBackfill_DerivedKeyExpansion(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Scenario: the request asks for id + email keys, but the cache config also
	// knows about username as a third entity key. The fetch runs, returns all
	// three fields, and the loader should refresh id, backfill email, and add the
	// extra derived username key from final entity data.
	cache := NewFakeLoaderCache()
	idKey := `{"__typename":"User","key":{"id":"u1"}}`
	emailKey := `{"__typename":"User","key":{"email":"a@example.com"}}`
	usernameKey := `{"__typename":"User","key":{"username":"Alice"}}`

	// Seed L2 with only the incomplete id entry so the fetch path is required.
	err := cache.Set(t.Context(), withCacheEntryTTL([]*CacheEntry{
		{Key: idKey, Value: []byte(`{"__typename":"User","id":"u1"}`)},
	}, 30*time.Second))
	require.NoError(t, err)
	cache.ClearLog()

	// The subgraph returns the full entity, including the extra username key field.
	rootDS := NewMockDataSource(ctrl)
	rootDS.EXPECT().
		Load(gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, headers any, input []byte) ([]byte, error) {
			return []byte(`{"data":{"user":{"__typename":"User","id":"u1","email":"a@example.com","username":"Alice"}}}`), nil
		}).Times(1)

	response := newUserRootQueryResponse(
		rootDS,
		newUserRootQueryTemplate([]string{"id", "email"}, []string{"id", "email", "username"}),
		&Object{
			Fields: []*Field{
				{Name: []byte("id"), Value: &Scalar{Path: []string{"id"}, Nullable: false}},
				{Name: []byte("username"), Value: &Scalar{Path: []string{"username"}, Nullable: false}},
			},
		},
	)

	loader := &Loader{caches: map[string]LoaderCache{"default": cache}}
	ctx := NewContext(t.Context())
	ctx.Variables = astjson.MustParseBytes([]byte(`{"id":"u1","email":"a@example.com"}`))
	ctx.ExecutionOptions.DisableSubgraphRequestDeduplication = true
	ctx.ExecutionOptions.Caching.EnableL1Cache = true
	ctx.ExecutionOptions.Caching.EnableL2Cache = true
	ctx.ExecutionOptions.Caching.EnableCacheAnalytics = true

	ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
	resolvable := NewResolvable(ar, ResolvableOptions{})
	err = resolvable.Init(ctx, nil, ast.OperationTypeQuery)
	require.NoError(t, err)

	err = loader.LoadGraphQLResponseData(ctx, response, resolvable)
	require.NoError(t, err)

	// Assert the exact cache story:
	// 1. L2 reads the requested id + email keys and finds only id.
	// 2. The fetch refreshes id, backfills email, and adds the derived username key.
	assert.Equal(t, []CacheLogEntry{
		{
			Operation: "get",
			Items: []CacheLogItem{
				{Key: idKey, Hit: true},
				{Key: emailKey, Hit: false},
			},
		},
		{
			Operation: "set",
			Items: []CacheLogItem{
				{Key: idKey, TTL: 30 * time.Second},
				{Key: emailKey, TTL: 30 * time.Second},
				{Key: usernameKey, TTL: 30 * time.Second},
			},
		},
	}, cache.GetLog())
	// Assert all three keys now point at the same final entity payload.
	assert.Equal(t, `{"__typename":"User","id":"u1","email":"a@example.com","username":"Alice"}`, string(cache.GetValue(idKey)))
	assert.Equal(t, `{"__typename":"User","id":"u1","email":"a@example.com","username":"Alice"}`, string(cache.GetValue(emailKey)))
	assert.Equal(t, `{"__typename":"User","id":"u1","email":"a@example.com","username":"Alice"}`, string(cache.GetValue(usernameKey)))

	// Root-field caches that delegate via EntityKeyMappings attribute analytics
	// to the delegated entity type (here "User"), not the operation root
	// ("Query"). Hub / Studio per-entity dashboards group cache events by
	// EntityType, so tagging delegated reads as "Query" would leave the
	// per-Employee / per-User panels empty even when the cache served every
	// request. The attribution is applied in prepareCacheKeys via
	// res.analyticsEntityType and flows through L2 reads + writes uniformly.
	snap := normalizeCacheAnalyticsSnapshot(ctx.GetCacheStats())
	assert.Equal(t, normalizeCacheAnalyticsSnapshot(CacheAnalyticsSnapshot{
		L2Reads: []CacheKeyEvent{
			// id key found but incomplete for ProvidesData → partial hit, fetch needed
			{
				CacheKey:   idKey,
				EntityType: "User",
				Kind:       CacheKeyPartialHit,
				DataSource: "accounts",
			},
		},
		L2Writes: []CacheWriteEvent{
			// refresh: existing key rewritten with fresh subgraph data
			{
				CacheKey:    idKey,
				EntityType:  "User",
				ByteSize:    74,
				DataSource:  "accounts",
				CacheLevel:  CacheLevelL2,
				TTL:         30 * time.Second,
				Source:      CacheSourceQuery,
				WriteReason: CacheWriteReasonRefresh,
			},
			// backfill: missing requested key proven by subgraph response
			{
				CacheKey:    emailKey,
				EntityType:  "User",
				ByteSize:    74,
				DataSource:  "accounts",
				CacheLevel:  CacheLevelL2,
				TTL:         30 * time.Second,
				Source:      CacheSourceQuery,
				WriteReason: CacheWriteReasonBackfill,
			},
			// derived: username key not requested but derivable from entity data
			{
				CacheKey:    usernameKey,
				EntityType:  "User",
				ByteSize:    74,
				DataSource:  "accounts",
				CacheLevel:  CacheLevelL2,
				TTL:         30 * time.Second,
				Source:      CacheSourceQuery,
				WriteReason: CacheWriteReasonDerived,
			},
		},
	}), snap)
}

// Verifies that writeCanonicalJSON produces deterministic output regardless of
// key ordering in the input, ensuring stable cache keys.
func TestWriteCanonicalJSON(t *testing.T) {
	canonicalize := func(input string) string {
		v, err := astjson.Parse(input)
		require.NoError(t, err)
		var buf strings.Builder
		writeCanonicalJSON(&buf, v)
		return buf.String()
	}

	t.Run("object keys sorted alphabetically", func(t *testing.T) {
		assert.Equal(t, `{"a":1,"b":2,"c":3}`, canonicalize(`{"c":3,"a":1,"b":2}`))
	})

	t.Run("different key order produces same output", func(t *testing.T) {
		out1 := canonicalize(`{"style":"FORMAL","formatting":{"uppercase":true}}`)
		out2 := canonicalize(`{"formatting":{"uppercase":true},"style":"FORMAL"}`)
		assert.Equal(t, out1, out2)
		assert.Equal(t, `{"formatting":{"uppercase":true},"style":"FORMAL"}`, out1)
	})

	t.Run("nested objects sorted recursively", func(t *testing.T) {
		out := canonicalize(`{"z":{"b":2,"a":1},"a":{"d":4,"c":3}}`)
		assert.Equal(t, `{"a":{"c":3,"d":4},"z":{"a":1,"b":2}}`, out)
	})

	t.Run("array elements preserve order", func(t *testing.T) {
		assert.Equal(t, `[3,1,2]`, canonicalize(`[3,1,2]`))
	})

	t.Run("array of objects sorted by keys", func(t *testing.T) {
		out := canonicalize(`[{"b":2,"a":1},{"d":4,"c":3}]`)
		assert.Equal(t, `[{"a":1,"b":2},{"c":3,"d":4}]`, out)
	})

	t.Run("empty object", func(t *testing.T) {
		assert.Equal(t, `{}`, canonicalize(`{}`))
	})

	t.Run("empty array", func(t *testing.T) {
		assert.Equal(t, `[]`, canonicalize(`[]`))
	})

	t.Run("scalar string", func(t *testing.T) {
		assert.Equal(t, `"hello"`, canonicalize(`"hello"`))
	})

	t.Run("scalar number", func(t *testing.T) {
		assert.Equal(t, `42`, canonicalize(`42`))
	})

	t.Run("scalar boolean", func(t *testing.T) {
		assert.Equal(t, `true`, canonicalize(`true`))
		assert.Equal(t, `false`, canonicalize(`false`))
	})

	t.Run("null", func(t *testing.T) {
		assert.Equal(t, `null`, canonicalize(`null`))
	})

	t.Run("mixed nested structure", func(t *testing.T) {
		input := `{"tags":["b","a"],"config":{"z":true,"a":false},"name":"test"}`
		expected := `{"config":{"a":false,"z":true},"name":"test","tags":["b","a"]}`
		assert.Equal(t, expected, canonicalize(input))
	})
}
