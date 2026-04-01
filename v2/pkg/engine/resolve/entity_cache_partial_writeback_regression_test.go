package resolve

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wundergraph/astjson"
	"github.com/wundergraph/go-arena"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/fastjsonext"
)

func TestEntityFetchWritebackPreservesExistingCachedFields(t *testing.T) {
	cache := NewFakeLoaderCache()
	productKey := `{"__typename":"Product","key":{"id":"prod-1"}}`

	// Seed the shared Product entity key with one partial projection.
	out1 := runSingleProductEntityFieldRequest(t, cache, []productFieldSpec{
		{name: "title", value: "Alpha Widget"},
	})
	assert.Equal(t, `{"data":{"product":{"__typename":"Product","id":"prod-1","title":"Alpha Widget"}}}`, out1)
	assert.Equal(t, `{"__typename":"Product","id":"prod-1","title":"Alpha Widget"}`, string(cache.GetValue(productKey)))

	cache.ClearLog()

	// Re-fetch the same entity through the same cache key, but with a narrower projection.
	// The response should still only contain `brand`, while the cache writeback must merge
	// that fresh field into the previously cached `title` payload instead of replacing it.
	out2 := runSingleProductEntityFieldRequest(t, cache, []productFieldSpec{
		{name: "brand", value: "Acme Corp"},
	})
	assert.Equal(t, `{"data":{"product":{"__typename":"Product","id":"prod-1","brand":"Acme Corp"}}}`, out2)
	assert.Equal(t, []CacheLogEntry{
		// L2 hit on the existing entity entry.
		{Operation: "get", Keys: []string{productKey}, Hits: []bool{true}},
		// Writeback merges the new projection into the cached object under the same key.
		{Operation: "set", Keys: []string{productKey}, TTL: 30 * time.Second},
	}, cache.GetLog())
	assert.Equal(t, `{"__typename":"Product","id":"prod-1","title":"Alpha Widget","brand":"Acme Corp"}`, string(cache.GetValue(productKey)))

	cache.ClearLog()

	// A later request for both fields should now be a pure cache hit. If the previous
	// writeback had overwritten `title`, this request would have to fetch again.
	out3 := runSingleProductEntityFieldRequest(t, cache, []productFieldSpec{
		{name: "title", value: "Alpha Widget"},
		{name: "brand", value: "Acme Corp"},
	})
	assert.Equal(t, `{"data":{"product":{"__typename":"Product","id":"prod-1","title":"Alpha Widget","brand":"Acme Corp"}}}`, out3)
	assert.Equal(t, []CacheLogEntry{
		// No writeback on the final request: the merged cache entry is already complete.
		{Operation: "get", Keys: []string{productKey}, Hits: []bool{true}},
	}, cache.GetLog())
}

func TestRootFieldEntityCacheEntrySurvivesLaterPartialEntityFetch(t *testing.T) {
	cache := NewFakeLoaderCache()
	productKey := `{"__typename":"Product","key":{"id":"prod-1"}}`

	// First populate the shared Product entity key from a root-field cache write.
	out1 := runProductByIDRootRequest(t, cache)
	assert.Equal(t, `{"data":{"productById":{"__typename":"Product","id":"prod-1","sku":"ABC","title":"Alpha Widget"}}}`, out1)
	assert.Equal(t, `{"__typename":"Product","id":"prod-1","sku":"ABC","title":"Alpha Widget"}`, string(cache.GetValue(productKey)))

	cache.ClearLog()

	// Then resolve the same entity through a different root field that only asks the entity
	// subgraph for `brand`. This reproduces the cross-path regression: the narrower entity
	// fetch must extend the existing shared entry instead of wiping out `sku` and `title`.
	out2 := runProductBySKUWithBrandRequest(t, cache)
	assert.Equal(t, `{"data":{"productBySku":{"__typename":"Product","id":"prod-1","brand":"Acme Corp"}}}`, out2)
	assert.Equal(t, []CacheLogEntry{
		// Read the shared entity key created by the first root-field request.
		{Operation: "get", Keys: []string{productKey}, Hits: []bool{true}},
		// Rewrite that same key with the merged view of old root-field data plus new entity data.
		{Operation: "set", Keys: []string{productKey}, TTL: 30 * time.Second},
	}, cache.GetLog())
	assert.Equal(t, `{"__typename":"Product","id":"prod-1","sku":"ABC","title":"Alpha Widget","brand":"Acme Corp"}`, string(cache.GetValue(productKey)))
}

type productFieldSpec struct {
	name  string
	value string
}

func runSingleProductEntityFieldRequest(t *testing.T, cache LoaderCache, fields []productFieldSpec) string {
	t.Helper()

	// The root fetch only contributes the entity identity. The second fetch requests the
	// actual field projection and is the one that exercises partial entity-cache writeback.
	rootDS := &staticDataSource{data: []byte(`{"data":{"product":{"__typename":"Product","id":"prod-1"}}}`)}
	entityDS := &staticDataSource{data: productEntityResponse(fields)}
	response := buildSingleProductFieldResponse(rootDS, entityDS, fields)

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

	return fastjsonext.PrintGraphQLResponse(resolvable.data, resolvable.errors)
}

func buildSingleProductFieldResponse(rootDS, entityDS DataSource, fields []productFieldSpec) *GraphQLResponse {
	fieldInfos := make([]GraphCoordinate, 0, len(fields))
	responseFields := make([]*Field, 0, len(fields)+1)
	providesFields := make([]*Field, 0, len(fields))

	responseFields = append(responseFields, &Field{
		Name:  []byte("id"),
		Value: &String{Path: []string{"id"}},
	})

	for _, field := range fields {
		fieldInfos = append(fieldInfos, GraphCoordinate{TypeName: "Product", FieldName: field.name})
		providesFields = append(providesFields, &Field{
			Name:  []byte(field.name),
			Value: &Scalar{Path: []string{field.name}, Nullable: false},
		})
		responseFields = append(responseFields, &Field{
			Name:  []byte(field.name),
			Value: &String{Path: []string{field.name}},
		})
	}

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
				Info: &FetchInfo{
					DataSourceID:   "products",
					DataSourceName: "products",
					RootFields:     []GraphCoordinate{{TypeName: "Query", FieldName: "product"}},
					OperationType:  ast.OperationTypeQuery,
				},
			}, "query"),
			SingleWithPath(&SingleFetch{
				FetchConfiguration: FetchConfiguration{
					DataSource:     entityDS,
					PostProcessing: PostProcessingConfiguration{SelectResponseDataPath: []string{"data", "_entities", "0"}},
					Caching: FetchCacheConfiguration{
						Enabled:          true,
						CacheName:        "default",
						TTL:              30 * time.Second,
						CacheKeyTemplate: newProductCacheKeyTemplate(),
						UseL1Cache:       true,
					},
				},
				InputTemplate: InputTemplate{Segments: []TemplateSegment{
					{Data: []byte(`{"method":"POST","body":{"query":"..."}}`), SegmentType: StaticSegmentType},
				}},
				DataSourceIdentifier: []byte("graphql_datasource.Source"),
				Info: &FetchInfo{
					DataSourceID:   "details",
					DataSourceName: "details",
					RootFields:     fieldInfos,
					OperationType:  ast.OperationTypeQuery,
					ProvidesData:   &Object{Fields: providesFields},
				},
			}, "query.product", ObjectPath("product")),
		),
		Data: &Object{
			Fields: []*Field{
				{
					Name: []byte("product"),
					Value: &Object{
						Path:   []string{"product"},
						Fields: responseFields,
					},
				},
			},
		},
	}
}

func productEntityResponse(fields []productFieldSpec) []byte {
	payload := `{"data":{"_entities":[{"__typename":"Product","id":"prod-1"`
	for _, field := range fields {
		payload += `,"` + field.name + `":"` + field.value + `"`
	}
	payload += `}]}}`
	return []byte(payload)
}

func runProductByIDRootRequest(t *testing.T, cache LoaderCache) string {
	t.Helper()

	// This root query caches a full Product object and maps it onto the shared Product
	// entity key, which lets later entity fetches hit and update the same cache entry.
	rootDS := &staticDataSource{data: []byte(`{"data":{"productById":{"__typename":"Product","id":"prod-1","sku":"ABC","title":"Alpha Widget"}}}`)}
	response := &GraphQLResponse{
		Info: &GraphQLResponseInfo{OperationType: ast.OperationTypeQuery},
		Fetches: Sequence(
			SingleWithPath(&SingleFetch{
				FetchConfiguration: FetchConfiguration{
					DataSource:     rootDS,
					PostProcessing: PostProcessingConfiguration{SelectResponseDataPath: []string{"data"}},
					Caching: FetchCacheConfiguration{
						Enabled:   true,
						CacheName: "default",
						TTL:       30 * time.Second,
						CacheKeyTemplate: NewRootQueryCacheKeyTemplate(
							[]QueryField{{
								Coordinate: GraphCoordinate{TypeName: "Query", FieldName: "productById"},
								Args: []FieldArgument{{
									Name: "id",
									Variable: &ContextVariable{
										Path:     []string{"id"},
										Renderer: NewPlainVariableRenderer(),
									},
								}},
							}},
							[]EntityKeyMappingConfig{{
								EntityTypeName: "Product",
								FieldMappings: []EntityFieldMappingConfig{{
									EntityKeyField: "id",
									ArgumentPath:   []string{"id"},
								}},
							}},
						),
					},
				},
				InputTemplate: InputTemplate{Segments: []TemplateSegment{
					{Data: []byte(`{"method":"POST"}`), SegmentType: StaticSegmentType},
				}},
				DataSourceIdentifier: []byte("graphql_datasource.Source"),
				Info: &FetchInfo{
					DataSourceID:   "items",
					DataSourceName: "items",
					RootFields:     []GraphCoordinate{{TypeName: "Query", FieldName: "productById"}},
					OperationType:  ast.OperationTypeQuery,
				},
			}, "query"),
		),
		Data: &Object{
			Fields: []*Field{{
				Name: []byte("productById"),
				Value: &Object{
					Path: []string{"productById"},
					Fields: []*Field{
						{Name: []byte("__typename"), Value: &String{Path: []string{"__typename"}}},
						{Name: []byte("id"), Value: &String{Path: []string{"id"}}},
						{Name: []byte("sku"), Value: &String{Path: []string{"sku"}}},
						{Name: []byte("title"), Value: &String{Path: []string{"title"}}},
					},
				},
			}},
		},
	}

	loader := &Loader{caches: map[string]LoaderCache{"default": cache}}
	ctx := NewContext(t.Context())
	ctx.Variables = astjson.MustParseBytes([]byte(`{"id":"prod-1"}`))
	ctx.ExecutionOptions.DisableSubgraphRequestDeduplication = true
	ctx.ExecutionOptions.Caching.EnableL1Cache = true
	ctx.ExecutionOptions.Caching.EnableL2Cache = true

	ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
	resolvable := NewResolvable(ar, ResolvableOptions{})
	err := resolvable.Init(ctx, nil, ast.OperationTypeQuery)
	require.NoError(t, err)

	err = loader.LoadGraphQLResponseData(ctx, response, resolvable)
	require.NoError(t, err)

	return fastjsonext.PrintGraphQLResponse(resolvable.data, resolvable.errors)
}

func runProductBySKUWithBrandRequest(t *testing.T, cache LoaderCache) string {
	t.Helper()

	// The root fetch finds the entity identity by SKU. The follow-up entity fetch asks only
	// for `brand`, which is enough to reproduce the bug if writeback overwrites the cache.
	rootDS := &staticDataSource{data: []byte(`{"data":{"productBySku":{"__typename":"Product","id":"prod-1"}}}`)}
	entityDS := &staticDataSource{data: []byte(`{"data":{"_entities":[{"__typename":"Product","id":"prod-1","brand":"Acme Corp"}]}}`)}
	rootFieldEntityTemplate := &EntityQueryCacheKeyTemplate{
		Keys: NewResolvableObjectVariable(&Object{
			Path: []string{"productBySku"},
			Fields: []*Field{
				{Name: []byte("__typename"), Value: &String{Path: []string{"__typename"}}},
				{Name: []byte("id"), Value: &String{Path: []string{"id"}}},
			},
		}),
	}
	response := &GraphQLResponse{
		Info: &GraphQLResponseInfo{OperationType: ast.OperationTypeQuery},
		Fetches: Sequence(
			SingleWithPath(&SingleFetch{
				FetchConfiguration: FetchConfiguration{
					DataSource:     rootDS,
					PostProcessing: PostProcessingConfiguration{SelectResponseDataPath: []string{"data"}},
					Caching: FetchCacheConfiguration{
						Enabled:    true,
						UseL1Cache: true,
						RootFieldL1EntityCacheKeyTemplates: map[string]CacheKeyTemplate{
							"productBySku:Product": rootFieldEntityTemplate,
						},
					},
				},
				InputTemplate: InputTemplate{Segments: []TemplateSegment{
					{Data: []byte(`{"method":"POST"}`), SegmentType: StaticSegmentType},
				}},
				DataSourceIdentifier: []byte("graphql_datasource.Source"),
				Info: &FetchInfo{
					DataSourceID:   "items",
					DataSourceName: "items",
					RootFields:     []GraphCoordinate{{TypeName: "Query", FieldName: "productBySku"}},
					OperationType:  ast.OperationTypeQuery,
				},
			}, "query"),
			SingleWithPath(&EntityFetch{
				Input: EntityInput{
					Header: InputTemplate{Segments: []TemplateSegment{
						{Data: []byte(`{"method":"POST","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Product {brand}}}","variables":{"representations":[`), SegmentType: StaticSegmentType},
					}},
					Item: InputTemplate{Segments: []TemplateSegment{
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
					}},
					Footer: InputTemplate{Segments: []TemplateSegment{
						{Data: []byte(`]}}}`), SegmentType: StaticSegmentType},
					}},
					SkipErrItem: true,
				},
				DataSource:     entityDS,
				PostProcessing: PostProcessingConfiguration{SelectResponseDataPath: []string{"data", "_entities", "0"}},
				Caching: FetchCacheConfiguration{
					Enabled:          true,
					CacheName:        "default",
					TTL:              30 * time.Second,
					CacheKeyTemplate: newProductCacheKeyTemplate(),
					UseL1Cache:       true,
				},
				DataSourceIdentifier: []byte("graphql_datasource.Source"),
				Info: &FetchInfo{
					DataSourceID:   "details",
					DataSourceName: "details",
					RootFields:     []GraphCoordinate{{TypeName: "Product", FieldName: "brand"}},
					OperationType:  ast.OperationTypeQuery,
					ProvidesData: &Object{
						Fields: []*Field{
							{Name: []byte("brand"), Value: &Scalar{Path: []string{"brand"}, Nullable: false}},
						},
					},
				},
			}, "query.productBySku", ObjectPath("productBySku")),
		),
		Data: &Object{
			Fields: []*Field{{
				Name: []byte("productBySku"),
				Value: &Object{
					Path: []string{"productBySku"},
					Fields: []*Field{
						{Name: []byte("__typename"), Value: &String{Path: []string{"__typename"}}},
						{Name: []byte("id"), Value: &String{Path: []string{"id"}}},
						{Name: []byte("brand"), Value: &String{Path: []string{"brand"}}},
					},
				},
			}},
		},
	}

	loader := &Loader{caches: map[string]LoaderCache{"default": cache}}
	ctx := NewContext(t.Context())
	ctx.Variables = astjson.MustParseBytes([]byte(`{"sku":"ABC","region":"US"}`))
	ctx.ExecutionOptions.DisableSubgraphRequestDeduplication = true
	ctx.ExecutionOptions.Caching.EnableL1Cache = true
	ctx.ExecutionOptions.Caching.EnableL2Cache = true

	ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
	resolvable := NewResolvable(ar, ResolvableOptions{})
	err := resolvable.Init(ctx, nil, ast.OperationTypeQuery)
	require.NoError(t, err)

	err = loader.LoadGraphQLResponseData(ctx, response, resolvable)
	require.NoError(t, err)

	return fastjsonext.PrintGraphQLResponse(resolvable.data, resolvable.errors)
}
