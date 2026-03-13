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

func TestNegativeCaching(t *testing.T) {
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
				for _, key := range entry.Keys {
					t.Logf("Stored cache key: %s", key)
				}
				setFound = true
			}
		}
		assert.True(t, setFound, "Expected a cache set operation for the negative sentinel")

		// Find the last set operation's first key and verify stored value is "null"
		for i := len(cacheLog) - 1; i >= 0; i-- {
			if cacheLog[i].Operation == "set" && len(cacheLog[i].Keys) > 0 {
				storedValue := cache.GetValue(cacheLog[i].Keys[0])
				assert.Equal(t, "null", string(storedValue), "Negative cache sentinel should be 'null' bytes")
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
				for i, hit := range entry.Hits {
					t.Logf("Cache key %s: hit=%v", entry.Keys[i], hit)
					if hit {
						getFound = true
					}
				}
			}
		}
		assert.True(t, getFound, "Expected L2 cache hit for negative sentinel on second call")
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
				t.Logf("Set: keys=%v ttl=%v", entry.Keys, entry.TTL)
				// The negative sentinel should use NegativeCacheTTL (5s), not regular TTL (60s)
				assert.Equal(t, 5*time.Second, entry.TTL, "Negative cache sentinel should use NegativeCacheTTL")
			}
		}
	})
}
