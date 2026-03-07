package resolve

import (
	"context"
	"net/http"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wundergraph/go-arena"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/datasource/httpclient"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/fastjsonext"
)

// TestResolveParallel_NoConcurrentArenaRace verifies that parallel entity fetches
// with L2 caching do not race on the arena. This test exercises the goroutine code
// paths in resolveParallel Phase 2 (extractCacheKeysStrings, populateFromCache,
// denormalizeFromCache) which allocate from per-goroutine arenas.
//
// Run with: go test -race -run TestResolveParallel_NoConcurrentArenaRace ./v2/pkg/engine/resolve/... -v -count=1
func TestResolveParallel_NoConcurrentArenaRace(t *testing.T) {
	t.Run("parallel batch entity fetches with L2 cache miss", func(t *testing.T) {
		// Scenario: Root fetch → Parallel(
		//     BatchEntityFetch (products subgraph, L2 miss → subgraph fetch),
		//     BatchEntityFetch (inventory subgraph, L2 miss → subgraph fetch),
		// )
		// Both fetches run as goroutines in Phase 2, exercising arena allocations concurrently.
		// With -race, this would detect if goroutines accidentally share l.jsonArena.

		productsDS := &staticDataSource{data: []byte(`{"data":{"_entities":[{"__typename":"Product","id":"prod-1","name":"Widget"},{"__typename":"Product","id":"prod-2","name":"Gadget"}]}}`)}
		inventoryDS := &staticDataSource{data: []byte(`{"data":{"_entities":[{"__typename":"Product","id":"prod-1","inStock":true},{"__typename":"Product","id":"prod-2","inStock":false}]}}`)}

		productCacheKeyTemplate := &EntityQueryCacheKeyTemplate{
			Keys: NewResolvableObjectVariable(&Object{
				Fields: []*Field{
					{Name: []byte("__typename"), Value: &String{Path: []string{"__typename"}}},
					{Name: []byte("id"), Value: &String{Path: []string{"id"}}},
				},
			}),
		}

		inventoryCacheKeyTemplate := &EntityQueryCacheKeyTemplate{
			Keys: NewResolvableObjectVariable(&Object{
				Fields: []*Field{
					{Name: []byte("__typename"), Value: &String{Path: []string{"__typename"}}},
					{Name: []byte("id"), Value: &String{Path: []string{"id"}}},
				},
			}),
		}

		// Run 100 iterations to increase the race window probability
		for range 100 {
			cache := NewFakeLoaderCache()

			rootDS := &staticDataSource{data: []byte(`{"data":{"products":[{"__typename":"Product","id":"prod-1"},{"__typename":"Product","id":"prod-2"}]}}`)}

			response := &GraphQLResponse{
				Info: &GraphQLResponseInfo{OperationType: ast.OperationTypeQuery},
				Fetches: Sequence(
					SingleWithPath(&SingleFetch{
						FetchConfiguration: FetchConfiguration{
							DataSource:     rootDS,
							PostProcessing: PostProcessingConfiguration{SelectResponseDataPath: []string{"data"}},
						},
						InputTemplate: InputTemplate{
							Segments: []TemplateSegment{{Data: []byte(`{"method":"POST","url":"http://products"}`), SegmentType: StaticSegmentType}},
						},
					}, "query"),
					Parallel(
						SingleWithPath(&BatchEntityFetch{
							Input: BatchInput{
								Header: InputTemplate{Segments: []TemplateSegment{{Data: []byte(`{"method":"POST","url":"http://products","body":{"query":"names","variables":{"representations":[`), SegmentType: StaticSegmentType}}},
								Items: []InputTemplate{{Segments: []TemplateSegment{{
									SegmentType:  VariableSegmentType,
									VariableKind: ResolvableObjectVariableKind,
									Renderer: NewGraphQLVariableResolveRenderer(&Object{Fields: []*Field{
										{Name: []byte("__typename"), Value: &String{Path: []string{"__typename"}}},
										{Name: []byte("id"), Value: &String{Path: []string{"id"}}},
									}}),
								}}}},
								Separator: InputTemplate{Segments: []TemplateSegment{{Data: []byte(`,`), SegmentType: StaticSegmentType}}},
								Footer:    InputTemplate{Segments: []TemplateSegment{{Data: []byte(`]}}}`), SegmentType: StaticSegmentType}}},
							},
							DataSource:     productsDS,
							PostProcessing: PostProcessingConfiguration{SelectResponseDataPath: []string{"data", "_entities"}},
							Info: &FetchInfo{
								DataSourceName: "products",
								OperationType:  ast.OperationTypeQuery,
								RootFields:     []GraphCoordinate{{TypeName: "Product"}},
								ProvidesData: &Object{
									Fields: []*Field{
										{Name: []byte("id"), Value: &Scalar{Path: []string{"id"}}},
										{Name: []byte("name"), Value: &Scalar{Path: []string{"name"}}},
									},
								},
							},
							Caching: FetchCacheConfiguration{
								Enabled:          true,
								CacheName:        "default",
								CacheKeyTemplate: productCacheKeyTemplate,
								UseL1Cache:       true,
								TTL:              60_000_000_000, // 60s
							},
						}, "query.products", ArrayPath("products")),
						SingleWithPath(&BatchEntityFetch{
							Input: BatchInput{
								Header: InputTemplate{Segments: []TemplateSegment{{Data: []byte(`{"method":"POST","url":"http://inventory","body":{"query":"stock","variables":{"representations":[`), SegmentType: StaticSegmentType}}},
								Items: []InputTemplate{{Segments: []TemplateSegment{{
									SegmentType:  VariableSegmentType,
									VariableKind: ResolvableObjectVariableKind,
									Renderer: NewGraphQLVariableResolveRenderer(&Object{Fields: []*Field{
										{Name: []byte("__typename"), Value: &String{Path: []string{"__typename"}}},
										{Name: []byte("id"), Value: &String{Path: []string{"id"}}},
									}}),
								}}}},
								Separator: InputTemplate{Segments: []TemplateSegment{{Data: []byte(`,`), SegmentType: StaticSegmentType}}},
								Footer:    InputTemplate{Segments: []TemplateSegment{{Data: []byte(`]}}}`), SegmentType: StaticSegmentType}}},
							},
							DataSource:     inventoryDS,
							PostProcessing: PostProcessingConfiguration{SelectResponseDataPath: []string{"data", "_entities"}},
							Info: &FetchInfo{
								DataSourceName: "inventory",
								OperationType:  ast.OperationTypeQuery,
								RootFields:     []GraphCoordinate{{TypeName: "Product"}},
								ProvidesData: &Object{
									Fields: []*Field{
										{Name: []byte("id"), Value: &Scalar{Path: []string{"id"}}},
										{Name: []byte("inStock"), Value: &Scalar{Path: []string{"inStock"}}},
									},
								},
							},
							Caching: FetchCacheConfiguration{
								Enabled:          true,
								CacheName:        "inventory",
								CacheKeyTemplate: inventoryCacheKeyTemplate,
								UseL1Cache:       true,
								TTL:              60_000_000_000,
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
										{Name: []byte("inStock"), Value: &Boolean{Path: []string{"inStock"}}},
									},
								},
							},
						},
					},
				},
			}

			ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
			loader := &Loader{
				jsonArena:          ar,
				caches:             map[string]LoaderCache{"default": cache, "inventory": cache},
				entityCacheConfigs: map[string]map[string]*EntityCacheInvalidationConfig{},
			}

			ctx := NewContext(context.Background())
			ctx.ExecutionOptions.DisableSubgraphRequestDeduplication = true
			ctx.ExecutionOptions.Caching.EnableL1Cache = true
			ctx.ExecutionOptions.Caching.EnableL2Cache = true

			resolvable := NewResolvable(ar, ResolvableOptions{})
			err := resolvable.Init(ctx, nil, ast.OperationTypeQuery)
			require.NoError(t, err)

			err = loader.LoadGraphQLResponseData(ctx, response, resolvable)
			require.NoError(t, err)

			out := fastjsonext.PrintGraphQLResponse(resolvable.data, resolvable.errors)
			assert.Contains(t, out, `"id":"prod-1"`)
			assert.Contains(t, out, `"id":"prod-2"`)

			loader.Free()
			ar.Reset()
		}
	})

	t.Run("parallel batch entity fetches with partial L2 cache hit", func(t *testing.T) {
		// Scenario: Root fetch → Parallel(
		//     BatchEntityFetch (products subgraph, L2 hit → populateFromCache),
		//     BatchEntityFetch (inventory subgraph, L2 miss → subgraph fetch),
		// )
		// Products fetch exercises populateFromCache (parsing cached JSON on goroutine arena).
		// Inventory fetch exercises concurrent subgraph fetch alongside cache path.

		cache := NewFakeLoaderCache()
		// Pre-populate L2 cache with product entities only; inventory entities are NOT cached
		cache.SetRawData(`{"__typename":"Product","key":{"id":"prod-1"}}`, []byte(`{"__typename":"Product","id":"prod-1","name":"Widget"}`), 60_000_000_000)
		cache.SetRawData(`{"__typename":"Product","key":{"id":"prod-2"}}`, []byte(`{"__typename":"Product","id":"prod-2","name":"Gadget"}`), 60_000_000_000)

		productCacheKeyTemplate := &EntityQueryCacheKeyTemplate{
			Keys: NewResolvableObjectVariable(&Object{
				Fields: []*Field{
					{Name: []byte("__typename"), Value: &String{Path: []string{"__typename"}}},
					{Name: []byte("id"), Value: &String{Path: []string{"id"}}},
				},
			}),
		}

		productsDS := &staticDataSource{data: []byte(`{"data":{"_entities":[{"__typename":"Product","id":"prod-1","name":"Widget"},{"__typename":"Product","id":"prod-2","name":"Gadget"}]}}`)}
		inventoryDS := &staticDataSource{data: []byte(`{"data":{"_entities":[{"__typename":"Product","id":"prod-1","inStock":true},{"__typename":"Product","id":"prod-2","inStock":false}]}}`)}

		for range 100 {
			rootDS := &staticDataSource{data: []byte(`{"data":{"products":[{"__typename":"Product","id":"prod-1"},{"__typename":"Product","id":"prod-2"}]}}`)}

			response := &GraphQLResponse{
				Info: &GraphQLResponseInfo{OperationType: ast.OperationTypeQuery},
				Fetches: Sequence(
					SingleWithPath(&SingleFetch{
						FetchConfiguration: FetchConfiguration{
							DataSource:     rootDS,
							PostProcessing: PostProcessingConfiguration{SelectResponseDataPath: []string{"data"}},
						},
						InputTemplate: InputTemplate{
							Segments: []TemplateSegment{{Data: []byte(`{"method":"POST","url":"http://products"}`), SegmentType: StaticSegmentType}},
						},
					}, "query"),
					Parallel(
						SingleWithPath(&BatchEntityFetch{
							Input: BatchInput{
								Header: InputTemplate{Segments: []TemplateSegment{{Data: []byte(`{"method":"POST","url":"http://products","body":{"query":"names","variables":{"representations":[`), SegmentType: StaticSegmentType}}},
								Items: []InputTemplate{{Segments: []TemplateSegment{{
									SegmentType:  VariableSegmentType,
									VariableKind: ResolvableObjectVariableKind,
									Renderer: NewGraphQLVariableResolveRenderer(&Object{Fields: []*Field{
										{Name: []byte("__typename"), Value: &String{Path: []string{"__typename"}}},
										{Name: []byte("id"), Value: &String{Path: []string{"id"}}},
									}}),
								}}}},
								Separator: InputTemplate{Segments: []TemplateSegment{{Data: []byte(`,`), SegmentType: StaticSegmentType}}},
								Footer:    InputTemplate{Segments: []TemplateSegment{{Data: []byte(`]}}}`), SegmentType: StaticSegmentType}}},
							},
							DataSource:     productsDS,
							PostProcessing: PostProcessingConfiguration{SelectResponseDataPath: []string{"data", "_entities"}},
							Info: &FetchInfo{
								DataSourceName: "products",
								OperationType:  ast.OperationTypeQuery,
								RootFields:     []GraphCoordinate{{TypeName: "Product"}},
								ProvidesData: &Object{
									Fields: []*Field{
										{Name: []byte("id"), Value: &Scalar{Path: []string{"id"}}},
										{Name: []byte("name"), Value: &Scalar{Path: []string{"name"}}},
									},
								},
							},
							Caching: FetchCacheConfiguration{
								Enabled:          true,
								CacheName:        "default",
								CacheKeyTemplate: productCacheKeyTemplate,
								UseL1Cache:       true,
								TTL:              60_000_000_000,
							},
						}, "query.products", ArrayPath("products")),
						SingleWithPath(&BatchEntityFetch{
							Input: BatchInput{
								Header: InputTemplate{Segments: []TemplateSegment{{Data: []byte(`{"method":"POST","url":"http://inventory","body":{"query":"stock","variables":{"representations":[`), SegmentType: StaticSegmentType}}},
								Items: []InputTemplate{{Segments: []TemplateSegment{{
									SegmentType:  VariableSegmentType,
									VariableKind: ResolvableObjectVariableKind,
									Renderer: NewGraphQLVariableResolveRenderer(&Object{Fields: []*Field{
										{Name: []byte("__typename"), Value: &String{Path: []string{"__typename"}}},
										{Name: []byte("id"), Value: &String{Path: []string{"id"}}},
									}}),
								}}}},
								Separator: InputTemplate{Segments: []TemplateSegment{{Data: []byte(`,`), SegmentType: StaticSegmentType}}},
								Footer:    InputTemplate{Segments: []TemplateSegment{{Data: []byte(`]}}}`), SegmentType: StaticSegmentType}}},
							},
							DataSource:     inventoryDS,
							PostProcessing: PostProcessingConfiguration{SelectResponseDataPath: []string{"data", "_entities"}},
							Info: &FetchInfo{
								DataSourceName: "inventory",
								OperationType:  ast.OperationTypeQuery,
								RootFields:     []GraphCoordinate{{TypeName: "Product"}},
								ProvidesData: &Object{
									Fields: []*Field{
										{Name: []byte("id"), Value: &Scalar{Path: []string{"id"}}},
										{Name: []byte("inStock"), Value: &Scalar{Path: []string{"inStock"}}},
									},
								},
							},
							Caching: FetchCacheConfiguration{
								Enabled:          true,
								CacheName:        "default",
								CacheKeyTemplate: productCacheKeyTemplate,
								UseL1Cache:       true,
								TTL:              60_000_000_000,
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
										{Name: []byte("inStock"), Value: &Boolean{Path: []string{"inStock"}}},
									},
								},
							},
						},
					},
				},
			}

			ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
			loader := &Loader{
				jsonArena:          ar,
				caches:             map[string]LoaderCache{"default": cache},
				entityCacheConfigs: map[string]map[string]*EntityCacheInvalidationConfig{},
			}

			ctx := NewContext(context.Background())
			ctx.ExecutionOptions.DisableSubgraphRequestDeduplication = true
			ctx.ExecutionOptions.Caching.EnableL1Cache = true
			ctx.ExecutionOptions.Caching.EnableL2Cache = true

			resolvable := NewResolvable(ar, ResolvableOptions{})
			err := resolvable.Init(ctx, nil, ast.OperationTypeQuery)
			require.NoError(t, err)

			err = loader.LoadGraphQLResponseData(ctx, response, resolvable)
			require.NoError(t, err)

			out := fastjsonext.PrintGraphQLResponse(resolvable.data, resolvable.errors)
			assert.Contains(t, out, `"id":"prod-1"`)
			assert.Contains(t, out, `"id":"prod-2"`)

			loader.Free()
			ar.Reset()
		}
	})
}

// staticDataSource returns static data for every Load call. Thread-safe.
type staticDataSource struct {
	data []byte
	mu   sync.Mutex
}

func (s *staticDataSource) Load(ctx context.Context, headers http.Header, input []byte) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]byte, len(s.data))
	copy(out, s.data)
	return out, nil
}

func (s *staticDataSource) LoadWithFiles(ctx context.Context, headers http.Header, input []byte, files []*httpclient.FileUpload) ([]byte, error) {
	return s.Load(ctx, headers, input)
}
