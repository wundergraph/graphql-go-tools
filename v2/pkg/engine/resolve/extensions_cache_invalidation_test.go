package resolve

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wundergraph/go-arena"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/fastjsonext"
)

// newUserCacheKeyTemplate returns a cache key template for User entities with @key(fields: "id").
func newUserCacheKeyTemplate() *EntityQueryCacheKeyTemplate {
	return &EntityQueryCacheKeyTemplate{
		Keys: NewResolvableObjectVariable(&Object{
			Fields: []*Field{
				{Name: []byte("__typename"), Value: &String{Path: []string{"__typename"}}},
				{Name: []byte("id"), Value: &String{Path: []string{"id"}}},
			},
		}),
	}
}

func newUserProvidesData() *Object {
	return &Object{
		Fields: []*Field{
			{Name: []byte("id"), Value: &Scalar{Path: []string{"id"}, Nullable: false}},
			{Name: []byte("username"), Value: &Scalar{Path: []string{"username"}, Nullable: false}},
		},
	}
}

func newUserEntityFetchSegments() []TemplateSegment {
	return []TemplateSegment{
		{
			Data:        []byte(`{"method":"POST","url":"http://accounts.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){__typename ... on User {id username}}}","variables":{"representations":[`),
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

// setupExtInvalidationTest creates a standard test setup with a root DS and an entity DS for User entities.
// The entityResponse is returned by the entity DS and should contain extensions.cacheInvalidation if needed.
func setupExtInvalidationTest(t *testing.T, entityResponse string, entityCallCount int, opts ...func(*Loader, *Context)) (*Loader, *Context, *GraphQLResponse, *FakeLoaderCache) {
	t.Helper()
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	cache := NewFakeLoaderCache()

	rootDS := NewMockDataSource(ctrl)
	rootDS.EXPECT().
		Load(gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, headers any, input []byte) ([]byte, error) {
			return []byte(`{"data":{"user":{"__typename":"User","id":"1"}}}`), nil
		}).Times(1)

	entityDS := NewMockDataSource(ctrl)
	entityDS.EXPECT().
		Load(gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, headers any, input []byte) ([]byte, error) {
			return []byte(entityResponse), nil
		}).Times(entityCallCount)

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
					Segments: []TemplateSegment{
						{Data: []byte(`{"method":"POST","url":"http://root.service","body":{"query":"{user {__typename id}}"}}`), SegmentType: StaticSegmentType},
					},
				},
				DataSourceIdentifier: []byte("graphql_datasource.Source"),
			}, "query"),
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
						CacheKeyTemplate: newUserCacheKeyTemplate(),
						UseL1Cache:       true,
					},
				},
				InputTemplate: InputTemplate{Segments: newUserEntityFetchSegments()},
				Info: &FetchInfo{
					DataSourceID:   "accounts",
					DataSourceName: "accounts",
					OperationType:  ast.OperationTypeQuery,
					ProvidesData:   newUserProvidesData(),
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
							{Name: []byte("username"), Value: &String{Path: []string{"username"}}},
						},
					},
				},
			},
		},
	}

	loader := &Loader{
		caches: map[string]LoaderCache{"default": cache},
		entityCacheConfigs: map[string]map[string]*EntityCacheInvalidationConfig{
			"accounts": {
				"User": {CacheName: "default", IncludeSubgraphHeaderPrefix: false},
			},
		},
	}

	ctx := NewContext(t.Context())
	ctx.ExecutionOptions.DisableSubgraphRequestDeduplication = true
	ctx.ExecutionOptions.Caching.EnableL1Cache = true
	ctx.ExecutionOptions.Caching.EnableL2Cache = true

	for _, opt := range opts {
		opt(loader, ctx)
	}

	return loader, ctx, response, cache
}

func runLoader(t *testing.T, loader *Loader, ctx *Context, response *GraphQLResponse) string {
	t.Helper()
	ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
	resolvable := NewResolvable(ar, ResolvableOptions{})
	err := resolvable.Init(ctx, nil, ast.OperationTypeQuery)
	require.NoError(t, err)

	err = loader.LoadGraphQLResponseData(ctx, response, resolvable)
	require.NoError(t, err)

	return fastjsonext.PrintGraphQLResponse(resolvable.data, resolvable.errors)
}

// setEntityFetchHeaderPrefix modifies the entity fetch's FetchCacheConfiguration
// to enable or disable IncludeSubgraphHeaderPrefix. This must match the EntityCacheInvalidationConfig
// setting for the delete-before-set optimization to correctly compare L2 keys.
func setEntityFetchHeaderPrefix(response *GraphQLResponse, enabled bool) {
	entityFetch := response.Fetches.ChildNodes[1].Item.Fetch.(*SingleFetch)
	entityFetch.Caching.IncludeSubgraphHeaderPrefix = enabled
}

func TestExtensionsCacheInvalidation(t *testing.T) {
	t.Run("single entity invalidation - same entity skipped", func(t *testing.T) {
		// Invalidation targets User:1 which is the same entity being fetched.
		// Since updateL2Cache will set User:1 with fresh data, the delete is redundant and skipped.
		entityResponse := `{"data":{"_entities":[{"__typename":"User","id":"1","username":"Alice"}]},"extensions":{"cacheInvalidation":{"keys":[{"typename":"User","key":{"id":"1"}}]}}}`
		loader, ctx, response, cache := setupExtInvalidationTest(t, entityResponse, 1)

		_ = runLoader(t, loader, ctx, response)

		for _, entry := range cache.GetLog() {
			assert.NotEqual(t, "delete", entry.Operation, "delete should be skipped when same key is about to be set")
		}
	})

	t.Run("multiple entity invalidation - only different entity deleted", func(t *testing.T) {
		// Invalidation targets User:1 (same as fetched, skipped) and User:2 (different, deleted).
		entityResponse := `{"data":{"_entities":[{"__typename":"User","id":"1","username":"Alice"}]},"extensions":{"cacheInvalidation":{"keys":[{"typename":"User","key":{"id":"1"}},{"typename":"User","key":{"id":"2"}}]}}}`
		loader, ctx, response, cache := setupExtInvalidationTest(t, entityResponse, 1)

		_ = runLoader(t, loader, ctx, response)

		var deleteKeys []string
		for _, entry := range cache.GetLog() {
			if entry.Operation == "delete" {
				deleteKeys = append(deleteKeys, entry.Keys...)
			}
		}
		require.Len(t, deleteKeys, 1, "only User:2 should be deleted (User:1 skipped — about to be set)")
		assert.Equal(t, `{"__typename":"User","key":{"id":"2"}}`, deleteKeys[0])
	})

	t.Run("with subgraph header prefix - same entity skipped", func(t *testing.T) {
		// Same entity fetched and invalidated — delete skipped even with header prefix.
		entityResponse := `{"data":{"_entities":[{"__typename":"User","id":"1","username":"Alice"}]},"extensions":{"cacheInvalidation":{"keys":[{"typename":"User","key":{"id":"1"}}]}}}`

		loader, ctx, response, cache := setupExtInvalidationTest(t, entityResponse, 1, func(l *Loader, c *Context) {
			l.entityCacheConfigs["accounts"]["User"].IncludeSubgraphHeaderPrefix = true
			c.SubgraphHeadersBuilder = &mockSubgraphHeadersBuilder{
				hashes: map[string]uint64{"accounts": 33333},
			}
		})

		// Also enable prefix on the fetch caching config so L2 store keys match invalidation keys.
		setEntityFetchHeaderPrefix(response, true)

		_ = runLoader(t, loader, ctx, response)

		for _, entry := range cache.GetLog() {
			assert.NotEqual(t, "delete", entry.Operation, "delete should be skipped when same key is about to be set")
		}
	})

	t.Run("with L2CacheKeyInterceptor - same entity skipped", func(t *testing.T) {
		// Same entity fetched and invalidated — delete skipped even with interceptor.
		entityResponse := `{"data":{"_entities":[{"__typename":"User","id":"1","username":"Alice"}]},"extensions":{"cacheInvalidation":{"keys":[{"typename":"User","key":{"id":"1"}}]}}}`

		loader, ctx, response, cache := setupExtInvalidationTest(t, entityResponse, 1, func(_ *Loader, c *Context) {
			c.ExecutionOptions.Caching.L2CacheKeyInterceptor = func(_ context.Context, key string, _ L2CacheKeyInterceptorInfo) string {
				return "tenant-X:" + key
			}
		})

		_ = runLoader(t, loader, ctx, response)

		for _, entry := range cache.GetLog() {
			assert.NotEqual(t, "delete", entry.Operation, "delete should be skipped when same key is about to be set")
		}
	})

	t.Run("with both prefix and interceptor - same entity skipped", func(t *testing.T) {
		// Same entity fetched and invalidated — delete skipped even with both transforms.
		entityResponse := `{"data":{"_entities":[{"__typename":"User","id":"1","username":"Alice"}]},"extensions":{"cacheInvalidation":{"keys":[{"typename":"User","key":{"id":"1"}}]}}}`

		loader, ctx, response, cache := setupExtInvalidationTest(t, entityResponse, 1, func(l *Loader, c *Context) {
			l.entityCacheConfigs["accounts"]["User"].IncludeSubgraphHeaderPrefix = true
			c.SubgraphHeadersBuilder = &mockSubgraphHeadersBuilder{
				hashes: map[string]uint64{"accounts": 33333},
			}
			c.ExecutionOptions.Caching.L2CacheKeyInterceptor = func(_ context.Context, key string, _ L2CacheKeyInterceptorInfo) string {
				return "tenant-X:" + key
			}
		})

		// Also enable prefix on the fetch caching config so L2 store keys match invalidation keys.
		setEntityFetchHeaderPrefix(response, true)

		_ = runLoader(t, loader, ctx, response)

		for _, entry := range cache.GetLog() {
			assert.NotEqual(t, "delete", entry.Operation, "delete should be skipped when same key is about to be set")
		}
	})

	t.Run("query response with invalidation - same entity skipped", func(t *testing.T) {
		// Query response invalidates User:1 which is the same entity being fetched and set.
		// Delete is skipped because updateL2Cache will set the same key with fresh data.
		entityResponse := `{"data":{"_entities":[{"__typename":"User","id":"1","username":"Alice"}]},"extensions":{"cacheInvalidation":{"keys":[{"typename":"User","key":{"id":"1"}}]}}}`
		loader, ctx, response, cache := setupExtInvalidationTest(t, entityResponse, 1)

		assert.Equal(t, ast.OperationTypeQuery, response.Info.OperationType)

		_ = runLoader(t, loader, ctx, response)

		for _, entry := range cache.GetLog() {
			assert.NotEqual(t, "delete", entry.Operation, "delete should be skipped when same key is about to be set")
		}
	})

	t.Run("no extensions in response", func(t *testing.T) {
		entityResponse := `{"data":{"_entities":[{"__typename":"User","id":"1","username":"Alice"}]}}`
		loader, ctx, response, cache := setupExtInvalidationTest(t, entityResponse, 1)

		_ = runLoader(t, loader, ctx, response)

		for _, entry := range cache.GetLog() {
			assert.NotEqual(t, "delete", entry.Operation, "should have zero delete calls")
		}
	})

	t.Run("extensions present but no cacheInvalidation key", func(t *testing.T) {
		entityResponse := `{"data":{"_entities":[{"__typename":"User","id":"1","username":"Alice"}]},"extensions":{"tracing":{"version":1}}}`
		loader, ctx, response, cache := setupExtInvalidationTest(t, entityResponse, 1)

		_ = runLoader(t, loader, ctx, response)

		for _, entry := range cache.GetLog() {
			assert.NotEqual(t, "delete", entry.Operation, "should have zero delete calls")
		}
	})

	t.Run("empty keys array", func(t *testing.T) {
		entityResponse := `{"data":{"_entities":[{"__typename":"User","id":"1","username":"Alice"}]},"extensions":{"cacheInvalidation":{"keys":[]}}}`
		loader, ctx, response, cache := setupExtInvalidationTest(t, entityResponse, 1)

		_ = runLoader(t, loader, ctx, response)

		for _, entry := range cache.GetLog() {
			assert.NotEqual(t, "delete", entry.Operation, "should have zero delete calls")
		}
	})

	t.Run("unknown typename silently skipped", func(t *testing.T) {
		entityResponse := `{"data":{"_entities":[{"__typename":"User","id":"1","username":"Alice"}]},"extensions":{"cacheInvalidation":{"keys":[{"typename":"UnknownType","key":{"id":"1"}}]}}}`
		loader, ctx, response, cache := setupExtInvalidationTest(t, entityResponse, 1)

		_ = runLoader(t, loader, ctx, response)

		for _, entry := range cache.GetLog() {
			assert.NotEqual(t, "delete", entry.Operation, "should have zero delete calls for unknown typename")
		}
	})

	t.Run("L2 cache disabled", func(t *testing.T) {
		entityResponse := `{"data":{"_entities":[{"__typename":"User","id":"1","username":"Alice"}]},"extensions":{"cacheInvalidation":{"keys":[{"typename":"User","key":{"id":"1"}}]}}}`
		loader, ctx, response, cache := setupExtInvalidationTest(t, entityResponse, 1, func(_ *Loader, c *Context) {
			c.ExecutionOptions.Caching.EnableL2Cache = false
		})

		// Since L2 is disabled, the entity fetch won't use caching at all,
		// and processExtensionsCacheInvalidation should return early.
		_ = runLoader(t, loader, ctx, response)

		for _, entry := range cache.GetLog() {
			assert.NotEqual(t, "delete", entry.Operation, "should have zero delete calls when L2 disabled")
		}
	})

	t.Run("malformed extensions - keys not an array", func(t *testing.T) {
		entityResponse := `{"data":{"_entities":[{"__typename":"User","id":"1","username":"Alice"}]},"extensions":{"cacheInvalidation":{"keys":"invalid"}}}`
		loader, ctx, response, cache := setupExtInvalidationTest(t, entityResponse, 1)

		// Should not panic
		_ = runLoader(t, loader, ctx, response)

		for _, entry := range cache.GetLog() {
			assert.NotEqual(t, "delete", entry.Operation, "should have zero delete calls for malformed extensions")
		}
	})

	t.Run("malformed extensions - entry missing typename", func(t *testing.T) {
		entityResponse := `{"data":{"_entities":[{"__typename":"User","id":"1","username":"Alice"}]},"extensions":{"cacheInvalidation":{"keys":[{"key":{"id":"1"}}]}}}`
		loader, ctx, response, cache := setupExtInvalidationTest(t, entityResponse, 1)

		_ = runLoader(t, loader, ctx, response)

		for _, entry := range cache.GetLog() {
			assert.NotEqual(t, "delete", entry.Operation, "should have zero delete calls when typename is missing")
		}
	})

	t.Run("malformed extensions - entry missing key", func(t *testing.T) {
		entityResponse := `{"data":{"_entities":[{"__typename":"User","id":"1","username":"Alice"}]},"extensions":{"cacheInvalidation":{"keys":[{"typename":"User"}]}}}`
		loader, ctx, response, cache := setupExtInvalidationTest(t, entityResponse, 1)

		_ = runLoader(t, loader, ctx, response)

		for _, entry := range cache.GetLog() {
			assert.NotEqual(t, "delete", entry.Operation, "should have zero delete calls when key is missing")
		}
	})

	t.Run("composite key fields", func(t *testing.T) {
		entityResponse := `{"data":{"_entities":[{"__typename":"User","id":"1","username":"Alice"}]},"extensions":{"cacheInvalidation":{"keys":[{"typename":"User","key":{"id":"1","orgId":"42"}}]}}}`
		loader, ctx, response, cache := setupExtInvalidationTest(t, entityResponse, 1)

		_ = runLoader(t, loader, ctx, response)

		var deleteKeys []string
		for _, entry := range cache.GetLog() {
			if entry.Operation == "delete" {
				deleteKeys = append(deleteKeys, entry.Keys...)
			}
		}
		require.Len(t, deleteKeys, 1, "should have exactly 1 delete key")
		assert.Equal(t, `{"__typename":"User","key":{"id":"1","orgId":"42"}}`, deleteKeys[0])
	})

	t.Run("L1 cache eviction", func(t *testing.T) {
		// Two sequential entity fetches within one request:
		// 1. Friend entity fetch (User:2) → populates L1
		// 2. User entity fetch (User:1) → response includes invalidation for User:2 → evicts from L1
		ctrl := gomock.NewController(t)
		t.Cleanup(ctrl.Finish)

		cache := NewFakeLoaderCache()

		rootDS := NewMockDataSource(ctrl)
		rootDS.EXPECT().Load(gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, _ any, _ []byte) ([]byte, error) {
				return []byte(`{"data":{"user":{"__typename":"User","id":"1"},"friend":{"__typename":"User","id":"2"}}}`), nil
			}).Times(1)

		// Friend entity fetch: resolves User:2, no invalidation
		friendDS := NewMockDataSource(ctrl)
		friendDS.EXPECT().Load(gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, _ any, _ []byte) ([]byte, error) {
				return []byte(`{"data":{"_entities":[{"__typename":"User","id":"2","username":"Bob"}]}}`), nil
			}).Times(1)

		// User entity fetch: resolves User:1, invalidates User:2
		userDS := NewMockDataSource(ctrl)
		userDS.EXPECT().Load(gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, _ any, _ []byte) ([]byte, error) {
				return []byte(`{"data":{"_entities":[{"__typename":"User","id":"1","username":"Alice"}]},"extensions":{"cacheInvalidation":{"keys":[{"typename":"User","key":{"id":"2"}}]}}}`), nil
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
						Segments: []TemplateSegment{
							{Data: []byte(`{"method":"POST","url":"http://root.service","body":{"query":"{user {__typename id} friend {__typename id}}"}}`), SegmentType: StaticSegmentType},
						},
					},
					DataSourceIdentifier: []byte("graphql_datasource.Source"),
				}, "query"),
				// Friend entity fetch runs first → populates L1 with User:2
				SingleWithPath(&SingleFetch{
					FetchConfiguration: FetchConfiguration{
						DataSource: friendDS,
						PostProcessing: PostProcessingConfiguration{
							SelectResponseDataPath: []string{"data", "_entities", "0"},
						},
						Caching: FetchCacheConfiguration{
							Enabled:          true,
							CacheName:        "default",
							TTL:              30 * time.Second,
							CacheKeyTemplate: newUserCacheKeyTemplate(),
							UseL1Cache:       true,
						},
					},
					InputTemplate: InputTemplate{Segments: newUserEntityFetchSegments()},
					Info: &FetchInfo{
						DataSourceID:   "accounts",
						DataSourceName: "accounts",
						OperationType:  ast.OperationTypeQuery,
						ProvidesData:   newUserProvidesData(),
					},
					DataSourceIdentifier: []byte("graphql_datasource.Source"),
				}, "query.friend", ObjectPath("friend")),
				// User entity fetch runs second → invalidates User:2 from L1
				SingleWithPath(&SingleFetch{
					FetchConfiguration: FetchConfiguration{
						DataSource: userDS,
						PostProcessing: PostProcessingConfiguration{
							SelectResponseDataPath: []string{"data", "_entities", "0"},
						},
						Caching: FetchCacheConfiguration{
							Enabled:          true,
							CacheName:        "default",
							TTL:              30 * time.Second,
							CacheKeyTemplate: newUserCacheKeyTemplate(),
							UseL1Cache:       true,
						},
					},
					InputTemplate: InputTemplate{Segments: newUserEntityFetchSegments()},
					Info: &FetchInfo{
						DataSourceID:   "accounts",
						DataSourceName: "accounts",
						OperationType:  ast.OperationTypeQuery,
						ProvidesData:   newUserProvidesData(),
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
								{Name: []byte("username"), Value: &String{Path: []string{"username"}}},
							},
						},
					},
					{
						Name: []byte("friend"),
						Value: &Object{
							Path: []string{"friend"},
							Fields: []*Field{
								{Name: []byte("id"), Value: &String{Path: []string{"id"}}},
								{Name: []byte("username"), Value: &String{Path: []string{"username"}}},
							},
						},
					},
				},
			},
		}

		loader := &Loader{
			caches: map[string]LoaderCache{"default": cache},
			entityCacheConfigs: map[string]map[string]*EntityCacheInvalidationConfig{
				"accounts": {
					"User": {CacheName: "default", IncludeSubgraphHeaderPrefix: false},
				},
			},
		}

		ctx := NewContext(t.Context())
		ctx.ExecutionOptions.DisableSubgraphRequestDeduplication = true
		ctx.ExecutionOptions.Caching.EnableL1Cache = true
		ctx.ExecutionOptions.Caching.EnableL2Cache = true

		_ = runLoader(t, loader, ctx, response)

		// User:2 was populated by friend entity fetch, then evicted by user entity fetch's invalidation
		l1Key2 := `{"__typename":"User","key":{"id":"2"}}`
		_, found := loader.l1Cache.Load(l1Key2)
		assert.False(t, found, "L1 cache entry for User:2 should be evicted after invalidation")

		// User:1 should still be present (populated by user entity fetch)
		l1Key1 := `{"__typename":"User","key":{"id":"1"}}`
		_, found = loader.l1Cache.Load(l1Key1)
		assert.True(t, found, "L1 cache entry for User:1 should still be present")
	})

	t.Run("interceptor receives correct SubgraphName and CacheName", func(t *testing.T) {
		entityResponse := `{"data":{"_entities":[{"__typename":"User","id":"1","username":"Alice"}]},"extensions":{"cacheInvalidation":{"keys":[{"typename":"User","key":{"id":"1"}}]}}}`

		var capturedInfos []L2CacheKeyInterceptorInfo
		loader, ctx, response, _ := setupExtInvalidationTest(t, entityResponse, 1, func(_ *Loader, c *Context) {
			c.ExecutionOptions.Caching.L2CacheKeyInterceptor = func(_ context.Context, key string, info L2CacheKeyInterceptorInfo) string {
				capturedInfos = append(capturedInfos, info)
				return key
			}
		})

		_ = runLoader(t, loader, ctx, response)

		// The interceptor is called for the invalidation key (and possibly the regular cache key).
		// All calls should use the same subgraph name and cache name.
		require.Len(t, capturedInfos, 2, "interceptor should be called exactly twice (cache set + invalidation)")
		assert.Equal(t, L2CacheKeyInterceptorInfo{SubgraphName: "accounts", CacheName: "default"}, capturedInfos[0])
		assert.Equal(t, L2CacheKeyInterceptorInfo{SubgraphName: "accounts", CacheName: "default"}, capturedInfos[1])
	})
}

// mockSubgraphHeadersBuilder is a test mock for SubgraphHeadersBuilder.
type mockSubgraphHeadersBuilder struct {
	hashes map[string]uint64
}

func (m *mockSubgraphHeadersBuilder) HeadersForSubgraph(subgraphName string) (http.Header, uint64) {
	return nil, m.hashes[subgraphName]
}

func (m *mockSubgraphHeadersBuilder) HashAll() uint64 {
	return 0
}

// Ensure mockSubgraphHeadersBuilder satisfies the SubgraphHeadersBuilder interface.
var _ SubgraphHeadersBuilder = (*mockSubgraphHeadersBuilder)(nil)
