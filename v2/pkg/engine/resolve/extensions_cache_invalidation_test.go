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
)

func TestExtensionsCacheInvalidation(t *testing.T) {
	// -------------------------------------------------------------------------
	// Delete-before-set optimization: when the invalidated entity is the SAME
	// entity being fetched, the L2 delete is skipped because updateL2Cache
	// will immediately set it with fresh data.
	// -------------------------------------------------------------------------

	t.Run("same entity fetched and invalidated — delete skipped", func(t *testing.T) {
		// User:1 is fetched AND invalidated in the same response.
		// updateL2Cache will set User:1, so the delete is redundant and skipped.
		env := newExtInvEnv(t,
			`{"data":{"_entities":[{"__typename":"User","id":"1","username":"Alice"}]},"extensions":{"cacheInvalidation":{"keys":[{"typename":"User","key":{"id":"1"}}]}}}`,
		)
		env.run()
		assert.False(t, env.hasDeletes(), "delete skipped — same key about to be set by updateL2Cache")
	})

	t.Run("same entity with header prefix — delete still skipped", func(t *testing.T) {
		// Same optimization applies even when keys are prefixed (e.g. "33333:User:1").
		// Both the invalidation key and the L2 set key go through the same prefix transform.
		env := newExtInvEnv(t,
			`{"data":{"_entities":[{"__typename":"User","id":"1","username":"Alice"}]},"extensions":{"cacheInvalidation":{"keys":[{"typename":"User","key":{"id":"1"}}]}}}`,
			withExtInvHeaderPrefix(33333),
		)
		env.run()
		assert.False(t, env.hasDeletes(), "delete skipped — prefixed key also about to be set")
	})

	t.Run("same entity with L2CacheKeyInterceptor — delete still skipped", func(t *testing.T) {
		// Same optimization applies when keys are transformed by an interceptor.
		env := newExtInvEnv(t,
			`{"data":{"_entities":[{"__typename":"User","id":"1","username":"Alice"}]},"extensions":{"cacheInvalidation":{"keys":[{"typename":"User","key":{"id":"1"}}]}}}`,
			withExtInvInterceptor(func(_ context.Context, key string, _ L2CacheKeyInterceptorInfo) string {
				return "tenant-X:" + key
			}),
		)
		env.run()
		assert.False(t, env.hasDeletes(), "delete skipped — intercepted key also about to be set")
	})

	t.Run("same entity with both prefix and interceptor — delete still skipped", func(t *testing.T) {
		// Both transforms applied: prefix + interceptor. Delete is still redundant.
		env := newExtInvEnv(t,
			`{"data":{"_entities":[{"__typename":"User","id":"1","username":"Alice"}]},"extensions":{"cacheInvalidation":{"keys":[{"typename":"User","key":{"id":"1"}}]}}}`,
			withExtInvHeaderPrefix(33333),
			withExtInvInterceptor(func(_ context.Context, key string, _ L2CacheKeyInterceptorInfo) string {
				return "tenant-X:" + key
			}),
		)
		env.run()
		assert.False(t, env.hasDeletes(), "delete skipped — both prefix and interceptor applied, key still about to be set")
	})

	// -------------------------------------------------------------------------
	// Different entity invalidated: the delete MUST happen because the key
	// being invalidated is NOT the same key being set by updateL2Cache.
	// -------------------------------------------------------------------------

	t.Run("different entity invalidated — only that entity deleted", func(t *testing.T) {
		// Invalidation targets User:1 (same as fetched → skipped) AND User:2 (different → deleted).
		// This proves the optimization is per-key, not all-or-nothing.
		env := newExtInvEnv(t,
			`{"data":{"_entities":[{"__typename":"User","id":"1","username":"Alice"}]},"extensions":{"cacheInvalidation":{"keys":[{"typename":"User","key":{"id":"1"}},{"typename":"User","key":{"id":"2"}}]}}}`,
		)
		env.run()

		deleteKeys := env.deleteKeys()
		require.Len(t, deleteKeys, 1, "User:1 skipped (about to be set), User:2 deleted")
		assert.Equal(t, `{"__typename":"User","key":{"id":"2"}}`, deleteKeys[0])
	})

	t.Run("composite key fields — different key shape is not skipped", func(t *testing.T) {
		// Invalidation key has composite fields {id:"1", orgId:"42"} which differs
		// from the fetched entity key {id:"1"}. No match → delete happens.
		env := newExtInvEnv(t,
			`{"data":{"_entities":[{"__typename":"User","id":"1","username":"Alice"}]},"extensions":{"cacheInvalidation":{"keys":[{"typename":"User","key":{"id":"1","orgId":"42"}}]}}}`,
		)
		env.run()

		deleteKeys := env.deleteKeys()
		require.Len(t, deleteKeys, 1, "composite key differs from fetch key — delete not skipped")
		assert.Equal(t, `{"__typename":"User","key":{"id":"1","orgId":"42"}}`, deleteKeys[0])
	})

	// -------------------------------------------------------------------------
	// No-op cases: various scenarios where no delete should happen.
	// -------------------------------------------------------------------------

	t.Run("no extensions in response — no delete", func(t *testing.T) {
		// Response has no extensions at all. Nothing to invalidate.
		env := newExtInvEnv(t,
			`{"data":{"_entities":[{"__typename":"User","id":"1","username":"Alice"}]}}`,
		)
		env.run()
		assert.False(t, env.hasDeletes(), "no extensions → no invalidation")
	})

	t.Run("extensions without cacheInvalidation key — no delete", func(t *testing.T) {
		// Extensions present but contain only tracing data, not cacheInvalidation.
		env := newExtInvEnv(t,
			`{"data":{"_entities":[{"__typename":"User","id":"1","username":"Alice"}]},"extensions":{"tracing":{"version":1}}}`,
		)
		env.run()
		assert.False(t, env.hasDeletes(), "no cacheInvalidation key → no invalidation")
	})

	t.Run("empty keys array — no delete", func(t *testing.T) {
		// cacheInvalidation present but keys array is empty.
		env := newExtInvEnv(t,
			`{"data":{"_entities":[{"__typename":"User","id":"1","username":"Alice"}]},"extensions":{"cacheInvalidation":{"keys":[]}}}`,
		)
		env.run()
		assert.False(t, env.hasDeletes(), "empty keys array → no invalidation")
	})

	t.Run("unknown typename — silently skipped, no delete", func(t *testing.T) {
		// Typename "UnknownType" has no entity cache config → skipped.
		env := newExtInvEnv(t,
			`{"data":{"_entities":[{"__typename":"User","id":"1","username":"Alice"}]},"extensions":{"cacheInvalidation":{"keys":[{"typename":"UnknownType","key":{"id":"1"}}]}}}`,
		)
		env.run()
		assert.False(t, env.hasDeletes(), "unknown typename has no cache config → skipped")
	})

	t.Run("L2 cache disabled — no delete", func(t *testing.T) {
		// With L2 disabled, processExtensionsCacheInvalidation returns early.
		env := newExtInvEnv(t,
			`{"data":{"_entities":[{"__typename":"User","id":"1","username":"Alice"}]},"extensions":{"cacheInvalidation":{"keys":[{"typename":"User","key":{"id":"1"}}]}}}`,
			withExtInvL2Disabled(),
		)
		env.run()
		assert.False(t, env.hasDeletes(), "L2 disabled → invalidation skipped entirely")
	})

	// -------------------------------------------------------------------------
	// Malformed extensions: gracefully handled, no panics, no deletes.
	// -------------------------------------------------------------------------

	t.Run("malformed — keys not an array", func(t *testing.T) {
		env := newExtInvEnv(t,
			`{"data":{"_entities":[{"__typename":"User","id":"1","username":"Alice"}]},"extensions":{"cacheInvalidation":{"keys":"invalid"}}}`,
		)
		env.run()
		assert.False(t, env.hasDeletes(), "malformed keys field → gracefully ignored")
	})

	t.Run("malformed — entry missing typename", func(t *testing.T) {
		env := newExtInvEnv(t,
			`{"data":{"_entities":[{"__typename":"User","id":"1","username":"Alice"}]},"extensions":{"cacheInvalidation":{"keys":[{"key":{"id":"1"}}]}}}`,
		)
		env.run()
		assert.False(t, env.hasDeletes(), "missing typename → entry skipped")
	})

	t.Run("malformed — entry missing key", func(t *testing.T) {
		env := newExtInvEnv(t,
			`{"data":{"_entities":[{"__typename":"User","id":"1","username":"Alice"}]},"extensions":{"cacheInvalidation":{"keys":[{"typename":"User"}]}}}`,
		)
		env.run()
		assert.False(t, env.hasDeletes(), "missing key → entry skipped")
	})

	// -------------------------------------------------------------------------
	// L1 cache eviction: invalidation evicts entries from the per-request L1 cache
	// to prevent stale reads within the same request.
	// -------------------------------------------------------------------------

	t.Run("L1 cache eviction — cross-entity invalidation within same request", func(t *testing.T) {
		// Two sequential entity fetches in one request:
		// 1. Friend fetch (User:2) → populates L1 with User:2
		// 2. User fetch (User:1) → response invalidates User:2 → evicts from L1
		//
		// This ensures that if a later part of the same request reads User:2,
		// it won't get stale data from L1.
		ctrl := gomock.NewController(t)
		t.Cleanup(ctrl.Finish)

		cache := NewFakeLoaderCache()

		rootDS := NewMockDataSource(ctrl)
		rootDS.EXPECT().Load(gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, _ any, _ []byte) ([]byte, error) {
				return []byte(`{"data":{"user":{"__typename":"User","id":"1"},"friend":{"__typename":"User","id":"2"}}}`), nil
			}).Times(1)

		friendDS := NewMockDataSource(ctrl)
		friendDS.EXPECT().Load(gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, _ any, _ []byte) ([]byte, error) {
				return []byte(`{"data":{"_entities":[{"__typename":"User","id":"2","username":"Bob"}]}}`), nil
			}).Times(1)

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
				// Friend fetch runs first → populates L1 with User:2
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
				// User fetch runs second → invalidates User:2 from L1
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
					"User": {CacheName: "default"},
				},
			},
		}

		ctx := NewContext(t.Context())
		ctx.ExecutionOptions.DisableSubgraphRequestDeduplication = true
		ctx.ExecutionOptions.Caching.EnableL1Cache = true
		ctx.ExecutionOptions.Caching.EnableL2Cache = true

		ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
		resolvable := NewResolvable(ar, ResolvableOptions{})
		require.NoError(t, resolvable.Init(ctx, nil, ast.OperationTypeQuery))
		require.NoError(t, loader.LoadGraphQLResponseData(ctx, response, resolvable))

		// User:2 was populated by friend fetch, then evicted by user fetch's invalidation.
		_, found := loader.l1Cache.Load(`{"__typename":"User","key":{"id":"2"}}`)
		assert.False(t, found, "User:2 should be evicted from L1 after invalidation")

		// User:1 should still be present — it was set by the user fetch, not invalidated.
		_, found = loader.l1Cache.Load(`{"__typename":"User","key":{"id":"1"}}`)
		assert.True(t, found, "User:1 should remain in L1 — not targeted by invalidation")
	})

	// -------------------------------------------------------------------------
	// Interceptor metadata: verify the L2CacheKeyInterceptor receives correct
	// SubgraphName and CacheName for both regular cache operations and
	// invalidation key construction.
	// -------------------------------------------------------------------------

	t.Run("interceptor receives correct SubgraphName and CacheName", func(t *testing.T) {
		// The interceptor is called twice: once for the L2 cache set (regular flow)
		// and once for the invalidation key construction.
		var capturedInfos []L2CacheKeyInterceptorInfo
		env := newExtInvEnv(t,
			`{"data":{"_entities":[{"__typename":"User","id":"1","username":"Alice"}]},"extensions":{"cacheInvalidation":{"keys":[{"typename":"User","key":{"id":"1"}}]}}}`,
			withExtInvInterceptor(func(_ context.Context, key string, info L2CacheKeyInterceptorInfo) string {
				capturedInfos = append(capturedInfos, info)
				return key
			}),
		)
		env.run()

		require.Len(t, capturedInfos, 2, "interceptor called for L2 set + invalidation key")
		assert.Equal(t, L2CacheKeyInterceptorInfo{SubgraphName: "accounts", CacheName: "default"}, capturedInfos[0])
		assert.Equal(t, L2CacheKeyInterceptorInfo{SubgraphName: "accounts", CacheName: "default"}, capturedInfos[1])
	})
}
