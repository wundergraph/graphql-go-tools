package resolve

import (
	"bytes"
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

// newNegativeCacheProductProvidesData returns a ProvidesData object for negative cache tests.
// Uses only "name" since that's what the entity fetch requests (unlike the interceptor
// helper which includes "id" + "name").
func newNegativeCacheProductProvidesData() *Object {
	return &Object{
		Fields: []*Field{
			{
				Name: []byte("name"),
				Value: &Scalar{
					Path:     []string{"name"},
					Nullable: false,
				},
			},
		},
	}
}

// newNegativeCacheEntitySegments returns input template segments for negative cache entity fetches.
func newNegativeCacheEntitySegments() []TemplateSegment {
	return []TemplateSegment{
		{
			Data:        []byte(`{"method":"POST","url":"http://products.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Product {name}}}","variables":{"representations":[`),
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
	}
}

// TestNegativeCache_NullEntityBehavior verifies the negative cache lifecycle: storing
// null entity results as sentinels, serving them on subsequent requests, TTL behavior,
// mutation interaction, and overwriting sentinels with real data after TTL expiry.
func TestNegativeCache_NullEntityBehavior(t *testing.T) {
	t.Run("null entity stored as negative sentinel and served on second request", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		cache := NewFakeLoaderCache()

		// Root fetch provides the product reference
		rootDS := NewMockDataSource(ctrl)
		rootDS.EXPECT().
			Load(gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, headers any, input []byte) ([]byte, error) {
				return []byte(`{"data":{"product":{"__typename":"Product","id":"prod-1"}}}`), nil
			}).AnyTimes()

		// Entity fetch returns null (entity not found in this subgraph)
		productDS := NewMockDataSource(ctrl)
		productDS.EXPECT().
			Load(gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, headers any, input []byte) ([]byte, error) {
				return []byte(`{"data":{"_entities":[null]}}`), nil
			}).Times(1) // Only called ONCE — second request uses negative cache

		cacheKeyTemplate := newProductCacheKeyTemplate()
		providesData := newNegativeCacheProductProvidesData()

		buildResponse := func() *GraphQLResponse {
			return &GraphQLResponse{
				Info: &GraphQLResponseInfo{
					OperationType: ast.OperationTypeQuery,
				},
				Fetches: Sequence(
					// Root fetch to populate product reference
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

					// Entity fetch that returns null
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
								CacheKeyTemplate: cacheKeyTemplate,
								NegativeCacheTTL: 10 * time.Second,
							},
						},
						InputTemplate: InputTemplate{
							Segments: newNegativeCacheEntitySegments(),
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
								Path:     []string{"product"},
								Nullable: true,
								Fields: []*Field{
									{
										Name: []byte("name"),
										Value: &String{
											Path:     []string{"name"},
											Nullable: true,
										},
									},
								},
							},
						},
					},
				},
			}
		}

		execute := func() string {
			loader := &Loader{
				caches: map[string]LoaderCache{
					"default": cache,
				},
			}
			ctx := NewContext(context.Background())
			ctx.ExecutionOptions.DisableSubgraphRequestDeduplication = true
			ctx.ExecutionOptions.Caching.EnableL2Cache = true

			ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
			resolvable := NewResolvable(ar, ResolvableOptions{})
			err := resolvable.Init(ctx, nil, ast.OperationTypeQuery)
			require.NoError(t, err)

			err = loader.LoadGraphQLResponseData(ctx, buildResponse(), resolvable)
			require.NoError(t, err)

			return string(fastjsonext.PrintGraphQLResponse(resolvable.data, resolvable.errors))
		}

		// First execution: subgraph is called, returns null
		out1 := execute()
		t.Logf("First output: %s", out1)

		// Verify the null sentinel was stored in L2
		cacheLog := cache.GetLog()
		var setFound bool
		for _, entry := range cacheLog {
			if entry.Operation == "set" {
				for _, item := range entry.Items {
					t.Logf("Stored cache key: %s", item.Key)
				}
				setFound = true
			}
		}
		assert.True(t, setFound)

		// Find the last set operation's first key and verify stored value is "null"
		for i := len(cacheLog) - 1; i >= 0; i-- {
			if cacheLog[i].Operation == "set" && len(cacheLog[i].Items) > 0 {
				storedValue := cache.GetValue(cacheLog[i].Items[0].Key)
				assert.Equal(t, "null", string(storedValue))
				break
			}
		}

		cache.ClearLog()

		// Second execution: should NOT call the subgraph (negative cache hit)
		out2 := execute()
		t.Logf("Second output: %s", out2)

		// Verify L2 cache was read (GET) and returned a hit
		cacheLog2 := cache.GetLog()
		var getFound bool
		for _, entry := range cacheLog2 {
			if entry.Operation == "get" {
				for _, item := range entry.Items {
					t.Logf("Cache key %s: hit=%v", item.Key, item.Hit)
					if item.Hit {
						getFound = true
					}
				}
			}
		}
		assert.True(t, getFound)
	})

	t.Run("negative caching disabled when NegativeCacheTTL is 0", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		cache := NewFakeLoaderCache()

		// Root fetch provides the product reference
		rootDS := NewMockDataSource(ctrl)
		rootDS.EXPECT().
			Load(gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, headers any, input []byte) ([]byte, error) {
				return []byte(`{"data":{"product":{"__typename":"Product","id":"prod-1"}}}`), nil
			}).AnyTimes()

		// Subgraph returns null both times — no negative caching
		productDS := NewMockDataSource(ctrl)
		productDS.EXPECT().
			Load(gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, headers any, input []byte) ([]byte, error) {
				return []byte(`{"data":{"_entities":[null]}}`), nil
			}).Times(2) // Called TWICE because negative caching is disabled

		cacheKeyTemplate := newProductCacheKeyTemplate()
		providesData := newNegativeCacheProductProvidesData()

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
								CacheKeyTemplate: cacheKeyTemplate,
								NegativeCacheTTL: 0, // Negative caching disabled
							},
						},
						InputTemplate: InputTemplate{
							Segments: newNegativeCacheEntitySegments(),
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
								Path:     []string{"product"},
								Nullable: true,
								Fields: []*Field{
									{
										Name: []byte("name"),
										Value: &String{
											Path:     []string{"name"},
											Nullable: true,
										},
									},
								},
							},
						},
					},
				},
			}
		}

		execute := func() {
			loader := &Loader{
				caches: map[string]LoaderCache{
					"default": cache,
				},
			}
			ctx := NewContext(context.Background())
			ctx.ExecutionOptions.DisableSubgraphRequestDeduplication = true
			ctx.ExecutionOptions.Caching.EnableL2Cache = true

			ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
			resolvable := NewResolvable(ar, ResolvableOptions{})
			err := resolvable.Init(ctx, nil, ast.OperationTypeQuery)
			require.NoError(t, err)

			err = loader.LoadGraphQLResponseData(ctx, buildResponse(), resolvable)
			require.NoError(t, err)
		}

		// Both calls should hit the subgraph (no negative caching)
		execute()
		cache.ClearLog()
		execute()
		// gomock verifies Times(2) — both calls went to subgraph
	})

	t.Run("negative cache sentinel uses NegativeCacheTTL not regular TTL", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		cache := NewFakeLoaderCache()

		// Root fetch provides the product reference
		rootDS := NewMockDataSource(ctrl)
		rootDS.EXPECT().
			Load(gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, headers any, input []byte) ([]byte, error) {
				return []byte(`{"data":{"product":{"__typename":"Product","id":"prod-1"}}}`), nil
			}).Times(1)

		// Entity fetch returns null
		productDS := NewMockDataSource(ctrl)
		productDS.EXPECT().
			Load(gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, headers any, input []byte) ([]byte, error) {
				return []byte(`{"data":{"_entities":[null]}}`), nil
			}).Times(1)

		cacheKeyTemplate := newProductCacheKeyTemplate()
		providesData := newNegativeCacheProductProvidesData()

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
							TTL:              60 * time.Second,
							CacheKeyTemplate: cacheKeyTemplate,
							NegativeCacheTTL: 5 * time.Second, // Much shorter than regular TTL
						},
					},
					InputTemplate: InputTemplate{
						Segments: newNegativeCacheEntitySegments(),
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
							Path:     []string{"product"},
							Nullable: true,
							Fields: []*Field{
								{
									Name: []byte("name"),
									Value: &String{
										Path:     []string{"name"},
										Nullable: true,
									},
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
		ctx.ExecutionOptions.Caching.EnableL2Cache = true

		ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
		resolvable := NewResolvable(ar, ResolvableOptions{})
		err := resolvable.Init(ctx, nil, ast.OperationTypeQuery)
		require.NoError(t, err)

		err = loader.LoadGraphQLResponseData(ctx, response, resolvable)
		require.NoError(t, err)

		// Verify the TTL used for the negative sentinel
		cacheLog := cache.GetLog()
		for _, entry := range cacheLog {
			if entry.Operation == "set" {
				t.Logf("Set: items=%v", entry.Items)
				// The negative sentinel should use NegativeCacheTTL (5s), not regular TTL (60s)
				// Negative sentinel should use NegativeCacheTTL (5s), not regular TTL (60s)
				assert.Equal(t, 5*time.Second, entry.Items[0].TTL)
			}
		}
	})

	t.Run("negative cache with mutation population stores sentinel with NegativeCacheTTL", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		cache := NewFakeLoaderCache()

		// Root mutation fetch
		mutationDS := NewMockDataSource(ctrl)
		mutationDS.EXPECT().
			Load(gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, headers any, input []byte) ([]byte, error) {
				return []byte(`{"data":{"createProduct":{"__typename":"Product","id":"prod-new"}}}`), nil
			}).Times(1)

		// Entity fetch returns null (entity not found after creation — edge case)
		productDS := NewMockDataSource(ctrl)
		productDS.EXPECT().
			Load(gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, headers any, input []byte) ([]byte, error) {
				return []byte(`{"data":{"_entities":[null]}}`), nil
			}).Times(1)

		cacheKeyTemplate := newProductCacheKeyTemplate()
		providesData := newNegativeCacheProductProvidesData()

		response := &GraphQLResponse{
			Info: &GraphQLResponseInfo{
				OperationType: ast.OperationTypeMutation,
			},
			Fetches: Sequence(
				SingleWithPath(&SingleFetch{
					FetchConfiguration: FetchConfiguration{
						DataSource: mutationDS,
						PostProcessing: PostProcessingConfiguration{
							SelectResponseDataPath: []string{"data"},
						},
						Caching: FetchCacheConfiguration{
							EnableMutationL2CachePopulation: true,
						},
					},
					InputTemplate: InputTemplate{
						Segments: []TemplateSegment{
							{
								Data:        []byte(`{"method":"POST","url":"http://mutation.service","body":{"query":"mutation{createProduct{__typename id}}"}}`),
								SegmentType: StaticSegmentType,
							},
						},
					},
					Info: &FetchInfo{
						DataSourceID:   "mutations",
						DataSourceName: "mutations",
						OperationType:  ast.OperationTypeMutation,
					},
					DataSourceIdentifier: []byte("graphql_datasource.Source"),
				}, "mutation"),

				SingleWithPath(&SingleFetch{
					FetchConfiguration: FetchConfiguration{
						DataSource: productDS,
						PostProcessing: PostProcessingConfiguration{
							SelectResponseDataPath: []string{"data", "_entities", "0"},
						},
						Caching: FetchCacheConfiguration{
							Enabled:          true,
							CacheName:        "default",
							TTL:              60 * time.Second,
							CacheKeyTemplate: cacheKeyTemplate,
							NegativeCacheTTL: 10 * time.Second,
						},
					},
					InputTemplate: InputTemplate{
						Segments: newNegativeCacheEntitySegments(),
					},
					Info: &FetchInfo{
						DataSourceID:   "products",
						DataSourceName: "products",
						OperationType:  ast.OperationTypeQuery, // Entity fetch within mutation gets Query type
						ProvidesData:   providesData,
					},
					DataSourceIdentifier: []byte("graphql_datasource.Source"),
				}, "mutation.createProduct", ObjectPath("createProduct")),
			),
			Data: &Object{
				Fields: []*Field{
					{
						Name: []byte("createProduct"),
						Value: &Object{
							Path:     []string{"createProduct"},
							Nullable: true,
							Fields: []*Field{
								{
									Name: []byte("name"),
									Value: &String{
										Path:     []string{"name"},
										Nullable: true,
									},
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
		ctx.ExecutionOptions.Caching.EnableL2Cache = true

		ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
		resolvable := NewResolvable(ar, ResolvableOptions{})
		err := resolvable.Init(ctx, nil, ast.OperationTypeMutation)
		require.NoError(t, err)

		err = loader.LoadGraphQLResponseData(ctx, response, resolvable)
		require.NoError(t, err)

		// Verify the full cache log: no L2 read (mutations skip L2 reads per AC-MUT-01),
		// only the negative sentinel write with NegativeCacheTTL (10s)
		cacheLog := cache.GetLog()
		assert.Equal(t, []CacheLogEntry{
			{Operation: "set", Items: []CacheLogItem{{Key: `{"__typename":"Product","key":{"id":"prod-new"}}`, TTL: 10 * time.Second}}}, // Negative sentinel stored with NegativeCacheTTL (10s), not entity TTL (60s); no prior "get" because mutations skip L2 reads
		}, cacheLog)

		// Verify the stored value is the null sentinel
		storedValue := cache.GetValue(`{"__typename":"Product","key":{"id":"prod-new"}}`)
		assert.Equal(t, "null", string(storedValue))
	})

	t.Run("negative cache entry overwritten by real data on subsequent fetch", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		cache := NewFakeLoaderCache()

		// Root fetch provides the product reference
		rootDS := NewMockDataSource(ctrl)
		rootDS.EXPECT().
			Load(gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, headers any, input []byte) ([]byte, error) {
				return []byte(`{"data":{"product":{"__typename":"Product","id":"prod-1"}}}`), nil
			}).AnyTimes()

		callCount := 0
		// Entity fetch: first call returns null, second returns real data
		productDS := NewMockDataSource(ctrl)
		productDS.EXPECT().
			Load(gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, headers any, input []byte) ([]byte, error) {
				callCount++
				if callCount == 1 {
					return []byte(`{"data":{"_entities":[null]}}`), nil
				}
				return []byte(`{"data":{"_entities":[{"name":"Widget"}]}}`), nil
			}).Times(2) // Called twice: first stores null, second after cache eviction stores real data

		cacheKeyTemplate := newProductCacheKeyTemplate()
		providesData := newNegativeCacheProductProvidesData()

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
								CacheKeyTemplate: cacheKeyTemplate,
								NegativeCacheTTL: 5 * time.Second,
							},
						},
						InputTemplate: InputTemplate{
							Segments: newNegativeCacheEntitySegments(),
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
								Path:     []string{"product"},
								Nullable: true,
								Fields: []*Field{
									{
										Name: []byte("name"),
										Value: &String{
											Path:     []string{"name"},
											Nullable: true,
										},
									},
								},
							},
						},
					},
				},
			}
		}

		execute := func() string {
			loader := &Loader{
				caches: map[string]LoaderCache{
					"default": cache,
				},
			}
			ctx := NewContext(context.Background())
			ctx.ExecutionOptions.DisableSubgraphRequestDeduplication = true
			ctx.ExecutionOptions.Caching.EnableL2Cache = true

			ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
			resolvable := NewResolvable(ar, ResolvableOptions{})
			err := resolvable.Init(ctx, nil, ast.OperationTypeQuery)
			require.NoError(t, err)

			err = loader.LoadGraphQLResponseData(ctx, buildResponse(), resolvable)
			require.NoError(t, err)

			return string(fastjsonext.PrintGraphQLResponse(resolvable.data, resolvable.errors))
		}

		// Request 1: returns null for the entity fetch → product has __typename/id from root but no "name"
		out1 := execute()
		// First request: only root fields, no entity data (null entity)
		assert.Equal(t, `{"data":{"product":{"__typename":"Product","id":"prod-1"}}}`, out1)

		productKey := `{"__typename":"Product","key":{"id":"prod-1"}}`

		// Verify request 1 cache log: L2 miss → negative sentinel stored
		cacheLog := cache.GetLog()
		assert.Equal(t, []CacheLogEntry{
			{Operation: "get", Items: []CacheLogItem{{Key: productKey, Hit: false}}},           // L2 miss: cache empty on first request
			{Operation: "set", Items: []CacheLogItem{{Key: productKey, TTL: 5 * time.Second}}}, // Negative sentinel stored with NegativeCacheTTL (5s)
		}, cacheLog)

		// Evict the negative sentinel to simulate TTL expiry
		_ = cache.Delete(context.Background(), []string{productKey})
		cache.ClearLog()

		// Request 2: negative sentinel evicted, subgraph called again, returns real data
		out2 := execute()
		// Second request: real product data after negative cache eviction
		assert.Equal(t, `{"data":{"product":{"__typename":"Product","id":"prod-1","name":"Widget"}}}`, out2)

		// Verify request 2 cache log: L2 miss (sentinel evicted) → real data stored with entity TTL
		cacheLog2 := cache.GetLog()
		assert.Equal(t, []CacheLogEntry{
			{Operation: "get", Items: []CacheLogItem{{Key: productKey, Hit: false}}},            // L2 miss: negative sentinel was evicted (TTL expiry simulated)
			{Operation: "set", Items: []CacheLogItem{{Key: productKey, TTL: 30 * time.Second}}}, // Real entity data stored with regular TTL (30s), replacing the evicted sentinel
		}, cacheLog2)

		// Verify the cache now holds real data, not the null sentinel
		// Cache now holds real data, not the null sentinel
		storedValue := cache.GetValue(productKey)
		assert.Equal(t, `{"__typename":"Product","id":"prod-1","name":"Widget"}`, string(storedValue))
	})
}

// TestNegativeCachingResolveRegression_PreservesParentObjectForNullableField guards
// against a regression where a null entity fetch would drop the parent object entirely.
// The parent object with its already-known fields (e.g., id) must survive the null merge.
func TestNegativeCachingResolveRegression_PreservesParentObjectForNullableField(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	cache := NewFakeLoaderCache()

	// The root fetch discovers the Product identity and creates the parent object that the
	// entity fetch will later extend. It does not provide `name`.
	rootDS := NewMockDataSource(ctrl)
	rootDS.EXPECT().
		Load(gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, headers any, input []byte) ([]byte, error) {
			return []byte(`{"data":{"product":{"__typename":"Product","id":"prod-1"}}}`), nil
		}).Times(1)

	// The entity fetch comes back as `null`, which triggers negative caching for this Product key.
	// The regression here was that resolve could lose the already-built parent object and return
	// `product: null` instead of preserving `product.id` and filling the nullable child as `null`.
	entityDS := NewMockDataSource(ctrl)
	entityDS.EXPECT().
		Load(gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, headers any, input []byte) ([]byte, error) {
			return []byte(`{"data":{"_entities":[null]}}`), nil
		}).Times(1)

	response := &GraphQLResponse{
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
					Segments: []TemplateSegment{{
						Data:        []byte(`{"method":"POST","url":"http://root.service","body":{"query":"{product {__typename id}}"}}`),
						SegmentType: StaticSegmentType,
					}},
				},
				DataSourceIdentifier: []byte("graphql_datasource.Source"),
			}, "query"),
			SingleWithPath(&SingleFetch{
				// This entity fetch asks only for the nullable `name` field. Negative caching is enabled
				// so the resolver has to merge a negative-cache result back into the existing `product` object.
				FetchConfiguration: FetchConfiguration{
					DataSource: entityDS,
					PostProcessing: PostProcessingConfiguration{
						SelectResponseDataPath: []string{"data", "_entities", "0"},
					},
					Caching: FetchCacheConfiguration{
						Enabled:          true,
						CacheName:        "default",
						TTL:              30 * time.Second,
						CacheKeyTemplate: newProductCacheKeyTemplate(),
						NegativeCacheTTL: 10 * time.Second,
					},
				},
				InputTemplate: InputTemplate{Segments: newNegativeCacheEntitySegments()},
				Info: &FetchInfo{
					DataSourceID:   "products",
					DataSourceName: "products",
					OperationType:  ast.OperationTypeQuery,
					ProvidesData: &Object{Fields: []*Field{{
						Name:  []byte("name"),
						Value: &String{Path: []string{"name"}, Nullable: true},
					}}},
				},
				DataSourceIdentifier: []byte("graphql_datasource.Source"),
			}, "query.product", ObjectPath("product")),
		),
		Data: &Object{Fields: []*Field{{
			Name: []byte("product"),
			Value: &Object{
				Path:     []string{"product"},
				Nullable: true,
				Fields: []*Field{
					{Name: []byte("id"), Value: &String{Path: []string{"id"}, Nullable: false}},
					// `name` is nullable, so a negative-cache hit should materialize it as `null`
					// while still preserving the parent object and its non-null `id`.
					{Name: []byte("name"), Value: &String{Path: []string{"name"}, Nullable: true}},
				},
			},
		}}},
	}

	loader := &Loader{caches: map[string]LoaderCache{"default": cache}}
	ctx := NewContext(context.Background())
	ctx.ExecutionOptions.DisableSubgraphRequestDeduplication = true
	ctx.ExecutionOptions.Caching.EnableL2Cache = true

	ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
	resolvable := NewResolvable(ar, ResolvableOptions{})
	err := resolvable.Init(ctx, nil, ast.OperationTypeQuery)
	require.NoError(t, err)

	err = loader.LoadGraphQLResponseData(ctx, response, resolvable)
	require.NoError(t, err)

	buf := &bytes.Buffer{}
	err = resolvable.Resolve(context.Background(), response.Data, response.Fetches, buf)
	require.NoError(t, err)
	// The parent object must survive the negative entity result. The regression would have
	// dropped the object entirely instead of returning the already-known `id` plus `name: null`.
	assert.Equal(t, `{"data":{"product":{"id":"prod-1","name":null}}}`, buf.String())
}

// TestLoader_cacheKeysToNegativeEntries_PreservesPositiveEntityDataWithNullableFields
// verifies that when an entity already has non-key fields from a prior fetch, the
// negative cache entry preserves them and adds the newly requested nullable field as null.
func TestLoader_cacheKeysToNegativeEntries_PreservesPositiveEntityDataWithNullableFields(t *testing.T) {
	t.Parallel()

	a := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))

	loader := &Loader{}
	// Start from an existing cached entity that already has non-key fields. This is the
	// branch where negative caching keeps an object-shaped payload instead of plain `null`.
	fromCache, err := astjson.ParseBytesWithArena(a, []byte(`{"__typename":"Item","id":"1","name":"Widget"}`))
	require.NoError(t, err)

	res := &result{
		providesData: &Object{
			Fields: []*Field{
				{
					Name: []byte("summary"),
					Value: &String{
						Path:     []string{"summary"},
						Nullable: true,
					},
				},
			},
		},
	}

	// Simulate a negative-cache write for the same entity key. The helper should preserve
	// the existing object shape and materialize the requested nullable field as explicit null.
	entries := loader.cacheKeysToNegativeEntries(a, res, []*CacheKey{{
		FromCache:        fromCache,
		Keys:             []string{`{"__typename":"Item","key":{"id":"1"}}`},
		NegativeCacheHit: true,
	}})

	require.Len(t, entries, 1)
	// `summary` was not present in the old payload, but because it is nullable in ProvidesData
	// the negative-cache value must include `"summary": null` so the same selection can validate from cache.
	require.Equal(t,
		compactJSONForAssert(t, `{"__typename":"Item","id":"1","name":"Widget","summary":null}`),
		compactJSONForAssert(t, string(entries[0].Value)),
	)
}

// TestLoader_cacheKeysToNegativeEntries_UsesNullSentinelWithoutPositiveEntityData
// verifies that with no prior entity data, the negative cache entry collapses to
// the literal "null" sentinel instead of storing key-only scaffolding.
func TestLoader_cacheKeysToNegativeEntries_UsesNullSentinelWithoutPositiveEntityData(t *testing.T) {
	t.Parallel()

	a := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))

	loader := &Loader{}
	// With no existing non-key entity data, negative caching must collapse to the literal
	// `null` sentinel rather than storing key-only scaffolding as if it were a real entity.
	entries := loader.cacheKeysToNegativeEntries(a, &result{}, []*CacheKey{{
		Keys:             []string{`{"__typename":"Item","key":{"id":"1"}}`},
		NegativeCacheHit: true,
	}})

	require.Len(t, entries, 1)
	require.Equal(t, "null", string(entries[0].Value))
}
