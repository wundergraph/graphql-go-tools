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

func TestCacheLoad(t *testing.T) {
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

		err := cache.Set(context.Background(), []*CacheEntry{
			{Key: `{"__typename":"Product","key":{"id":"prod-1"}}`, Value: []byte(prod1Data)},
			{Key: `{"__typename":"Product","key":{"id":"prod-2"}}`, Value: []byte(prod2Data)},
		}, 30*time.Second)
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
				for i, hit := range entry.Hits {
					t.Logf("Cache key %s: hit=%v", entry.Keys[i], hit)
				}
			}
		}

		assert.True(t, foundCacheGet, "Expected cache get operation for nested products")
	})
}

func TestCacheLoadSimple(t *testing.T) {
	t.Run("single entity fetch with cache hit", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		cache := NewFakeLoaderCache()

		// Pre-populate cache
		productData := `{"__typename":"Product","id":"prod-1","name":"Cached Product"}`
		err := cache.Set(context.Background(), []*CacheEntry{
			{Key: `{"__typename":"Product","key":{"id":"prod-1"}}`, Value: []byte(productData)},
		}, 30*time.Second)
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
				for i, hit := range entry.Hits {
					t.Logf("Cache key %s: hit=%v", entry.Keys[i], hit)
					if hit {
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
				for i, hit := range entry.Hits {
					t.Logf("Cache key %s: hit=%v", entry.Keys[i], hit)
					assert.False(t, hit, "Expected cache miss")
				}
			}
			if entry.Operation == "set" {
				foundCacheSet = true
				t.Logf("Cache set keys: %v", entry.Keys)
			}
		}

		assert.True(t, foundCacheGet, "Expected cache get operation")
		assert.True(t, foundCacheSet, "Expected cache set operation after miss")
	})
}

func TestCacheLoadSequential(t *testing.T) {
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
				firstGetHits = entry.Hits
				for i, hit := range entry.Hits {
					t.Logf("First call - Cache key %s: hit=%v", entry.Keys[i], hit)
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
				secondGetHits = entry.Hits
				for i, hit := range entry.Hits {
					t.Logf("Second call - Cache key %s: hit=%v", entry.Keys[i], hit)
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

// CacheLogEntry tracks a cache operation for testing
type CacheLogEntry struct {
	Operation string   // "get", "set", "delete"
	Keys      []string // Keys involved in the operation
	Hits      []bool   // For Get: whether each key was a hit (true) or miss (false)
}

type cacheEntry struct {
	data      []byte
	expiresAt *time.Time
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

	hits := make([]bool, len(keys))
	result := make([]*CacheEntry, len(keys))
	for i, key := range keys {
		if entry, exists := f.storage[key]; exists {
			// Make a copy of the data to prevent external modifications
			dataCopy := make([]byte, len(entry.data))
			copy(dataCopy, entry.data)
			result[i] = &CacheEntry{
				Key:   key,
				Value: dataCopy,
			}
			hits[i] = true
		} else {
			result[i] = nil
			hits[i] = false
		}
	}

	// Log the operation
	f.log = append(f.log, CacheLogEntry{
		Operation: "get",
		Keys:      keys,
		Hits:      hits,
	})

	return result, nil
}

func (f *FakeLoaderCache) Set(ctx context.Context, entries []*CacheEntry, ttl time.Duration) error {
	if len(entries) == 0 {
		return nil
	}

	f.mu.Lock()
	defer f.mu.Unlock()

	// Clean up expired entries before executing command
	f.cleanupExpired()

	keys := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry == nil {
			continue
		}
		ce := cacheEntry{
			// Make a copy of the data to prevent external modifications
			data: make([]byte, len(entry.Value)),
		}
		copy(ce.data, entry.Value)

		// If ttl is 0, store without expiration
		if ttl > 0 {
			expiresAt := time.Now().Add(ttl)
			ce.expiresAt = &expiresAt
		}

		f.storage[entry.Key] = ce
		keys = append(keys, entry.Key)
	}

	// Log the operation
	f.log = append(f.log, CacheLogEntry{
		Operation: "set",
		Keys:      keys,
		Hits:      nil, // Set operations don't have hits/misses
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

	// Log the operation
	f.log = append(f.log, CacheLogEntry{
		Operation: "delete",
		Keys:      keys,
		Hits:      nil, // Delete operations don't have hits/misses
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

// Clear removes all entries from the cache
func (f *FakeLoaderCache) Clear() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.storage = make(map[string]cacheEntry)
}
