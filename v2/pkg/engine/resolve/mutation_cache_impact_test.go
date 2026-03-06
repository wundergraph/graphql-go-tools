package resolve

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wundergraph/astjson"
	"github.com/wundergraph/go-arena"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
)

// ---------------------------------------------------------------------------
// navigateProvidesDataToField
// ---------------------------------------------------------------------------

func TestNavigateProvidesDataToField(t *testing.T) {
	t.Run("valid field name returns inner Object", func(t *testing.T) {
		inner := &Object{
			Fields: []*Field{
				{Name: []byte("id"), Value: &Scalar{Path: []string{"id"}}},
				{Name: []byte("username"), Value: &Scalar{Path: []string{"username"}}},
			},
		}
		provides := &Object{
			Fields: []*Field{
				{Name: []byte("updateUsername"), Value: inner},
			},
		}

		got := navigateProvidesDataToField(provides, "updateUsername")
		assert.Equal(t, inner, got)
	})

	t.Run("missing field name returns nil", func(t *testing.T) {
		provides := &Object{
			Fields: []*Field{
				{Name: []byte("updateUsername"), Value: &Object{}},
			},
		}

		got := navigateProvidesDataToField(provides, "deleteUser")
		assert.Nil(t, got)
	})

	t.Run("nil providesData returns nil", func(t *testing.T) {
		got := navigateProvidesDataToField(nil, "anything")
		assert.Nil(t, got)
	})

	t.Run("field value is not Object returns nil", func(t *testing.T) {
		provides := &Object{
			Fields: []*Field{
				{Name: []byte("scalarField"), Value: &Scalar{Path: []string{"scalarField"}}},
			},
		}

		got := navigateProvidesDataToField(provides, "scalarField")
		assert.Nil(t, got)
	})
}

// ---------------------------------------------------------------------------
// buildEntityKeyValue
// ---------------------------------------------------------------------------

func TestBuildEntityKeyValue(t *testing.T) {
	t.Run("simple key", func(t *testing.T) {
		ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
		data, err := astjson.ParseWithArena(ar, `{"id":"123","name":"Alice"}`)
		require.NoError(t, err)

		keyFields := []KeyField{{Name: "id"}}
		result := buildEntityKeyValue(ar, data, keyFields)
		got := string(result.MarshalTo(nil))

		assert.Equal(t, `{"id":"123"}`, got)
	})

	t.Run("composite key", func(t *testing.T) {
		ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
		data, err := astjson.ParseWithArena(ar, `{"id":"1","orgId":"acme","name":"Bob"}`)
		require.NoError(t, err)

		keyFields := []KeyField{{Name: "id"}, {Name: "orgId"}}
		result := buildEntityKeyValue(ar, data, keyFields)
		got := string(result.MarshalTo(nil))

		assert.Equal(t, `{"id":"1","orgId":"acme"}`, got)
	})

	t.Run("nested key", func(t *testing.T) {
		ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
		data, err := astjson.ParseWithArena(ar, `{"key":{"subId":"x"},"name":"Carol"}`)
		require.NoError(t, err)

		keyFields := []KeyField{
			{Name: "key", Children: []KeyField{{Name: "subId"}}},
		}
		result := buildEntityKeyValue(ar, data, keyFields)
		got := string(result.MarshalTo(nil))

		assert.Equal(t, `{"key":{"subId":"x"}}`, got)
	})

	t.Run("missing field in data omits field from output", func(t *testing.T) {
		ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
		data, err := astjson.ParseWithArena(ar, `{"name":"Dave"}`)
		require.NoError(t, err)

		keyFields := []KeyField{{Name: "id"}}
		result := buildEntityKeyValue(ar, data, keyFields)
		got := string(result.MarshalTo(nil))

		// "id" is missing in data, so it is omitted from the result
		assert.Equal(t, `{}`, got)
	})
}

// ---------------------------------------------------------------------------
// buildMutationEntityCacheKey
// ---------------------------------------------------------------------------

func TestBuildMutationEntityCacheKey(t *testing.T) {
	t.Run("basic key without prefix", func(t *testing.T) {
		ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
		ctx := NewContext(context.Background())

		l := &Loader{
			jsonArena: ar,
			ctx:       ctx,
		}

		entityData, err := astjson.ParseWithArena(ar, `{"id":"1234","username":"Alice"}`)
		require.NoError(t, err)

		cfg := &MutationEntityImpactConfig{
			EntityTypeName: "User",
			KeyFields:      []KeyField{{Name: "id"}},
			CacheName:      "default",
		}
		info := &FetchInfo{
			DataSourceName: "accounts",
		}

		got := l.buildMutationEntityCacheKey(cfg, entityData, info)
		assert.Equal(t, `{"__typename":"User","key":{"id":"1234"}}`, got)
	})

	t.Run("with header prefix", func(t *testing.T) {
		ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
		ctx := NewContext(context.Background())
		ctx.SubgraphHeadersBuilder = &mockSubgraphHeadersBuilder{
			hashes: map[string]uint64{"accounts": 99887766},
		}

		l := &Loader{
			jsonArena: ar,
			ctx:       ctx,
		}

		entityData, err := astjson.ParseWithArena(ar, `{"id":"1234","username":"Alice"}`)
		require.NoError(t, err)

		cfg := &MutationEntityImpactConfig{
			EntityTypeName:              "User",
			KeyFields:                   []KeyField{{Name: "id"}},
			CacheName:                   "default",
			IncludeSubgraphHeaderPrefix: true,
		}
		info := &FetchInfo{
			DataSourceName: "accounts",
		}

		got := l.buildMutationEntityCacheKey(cfg, entityData, info)
		assert.Equal(t, `99887766:{"__typename":"User","key":{"id":"1234"}}`, got)
	})

	t.Run("with interceptor", func(t *testing.T) {
		ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
		ctx := NewContext(context.Background())
		ctx.ExecutionOptions.Caching.L2CacheKeyInterceptor = func(_ context.Context, key string, info L2CacheKeyInterceptorInfo) string {
			return "tenant-42:" + key
		}

		l := &Loader{
			jsonArena: ar,
			ctx:       ctx,
		}

		entityData, err := astjson.ParseWithArena(ar, `{"id":"1234"}`)
		require.NoError(t, err)

		cfg := &MutationEntityImpactConfig{
			EntityTypeName: "User",
			KeyFields:      []KeyField{{Name: "id"}},
			CacheName:      "default",
		}
		info := &FetchInfo{
			DataSourceName: "accounts",
		}

		got := l.buildMutationEntityCacheKey(cfg, entityData, info)
		assert.Equal(t, `tenant-42:{"__typename":"User","key":{"id":"1234"}}`, got)
	})
}

// ---------------------------------------------------------------------------
// buildMutationEntityDisplayKey
// ---------------------------------------------------------------------------

func TestBuildMutationEntityDisplayKey(t *testing.T) {
	t.Run("display key always without prefix", func(t *testing.T) {
		ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
		ctx := NewContext(context.Background())
		// Even with a SubgraphHeadersBuilder, display key has no prefix
		ctx.SubgraphHeadersBuilder = &mockSubgraphHeadersBuilder{
			hashes: map[string]uint64{"accounts": 99887766},
		}

		l := &Loader{
			jsonArena: ar,
			ctx:       ctx,
		}

		entityData, err := astjson.ParseWithArena(ar, `{"id":"1234","username":"Alice"}`)
		require.NoError(t, err)

		cfg := &MutationEntityImpactConfig{
			EntityTypeName:              "User",
			KeyFields:                   []KeyField{{Name: "id"}},
			CacheName:                   "default",
			IncludeSubgraphHeaderPrefix: true,
		}

		got := l.buildMutationEntityDisplayKey(cfg, entityData)
		assert.Equal(t, `{"__typename":"User","key":{"id":"1234"}}`, got)
	})
}

// ---------------------------------------------------------------------------
// detectMutationEntityImpact
// ---------------------------------------------------------------------------

func TestDetectMutationEntityImpact(t *testing.T) {
	// Helper: builds a Loader with minimal fields for detectMutationEntityImpact.
	makeLoader := func(ctx *Context, cache LoaderCache, cacheName string) *Loader {
		ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
		return &Loader{
			jsonArena: ar,
			ctx:       ctx,
			caches:    map[string]LoaderCache{cacheName: cache},
			l1Cache:   &sync.Map{},
		}
	}

	// Helper: builds a result with MutationEntityImpactConfig.
	makeResult := func(cfg *MutationEntityImpactConfig) *result {
		return &result{
			cacheConfig: FetchCacheConfiguration{
				MutationEntityImpactConfig: cfg,
			},
		}
	}

	// Helper: builds FetchInfo for a mutation.
	makeMutationInfo := func(rootFieldName string, providesData *Object) *FetchInfo {
		return &FetchInfo{
			OperationType:  ast.OperationTypeMutation,
			DataSourceName: "accounts",
			RootFields: []GraphCoordinate{
				{TypeName: "Mutation", FieldName: rootFieldName},
			},
			ProvidesData: providesData,
		}
	}

	// Common ProvidesData: mutation returns an object with id and username.
	entityProvidesData := &Object{
		Fields: []*Field{
			{Name: []byte("id"), Value: &Scalar{Path: []string{"id"}}},
			{Name: []byte("username"), Value: &Scalar{Path: []string{"username"}}},
		},
	}
	mutationProvidesData := &Object{
		Fields: []*Field{
			{Name: []byte("updateUsername"), Value: entityProvidesData},
		},
	}

	t.Run("non-mutation operation returns nil", func(t *testing.T) {
		ctx := NewContext(context.Background())
		l := makeLoader(ctx, NewFakeLoaderCache(), "default")

		info := &FetchInfo{
			OperationType: ast.OperationTypeQuery, // not a mutation
		}
		res := makeResult(&MutationEntityImpactConfig{
			EntityTypeName:  "User",
			KeyFields:       []KeyField{{Name: "id"}},
			CacheName:       "default",
			InvalidateCache: true,
		})
		responseData := astjson.MustParse(`{"updateUsername":{"id":"1234","username":"NewMe"}}`)

		got := l.detectMutationEntityImpact(res, info, responseData)
		assert.Nil(t, got)
	})

	t.Run("nil info returns nil", func(t *testing.T) {
		ctx := NewContext(context.Background())
		l := makeLoader(ctx, NewFakeLoaderCache(), "default")

		res := makeResult(&MutationEntityImpactConfig{
			EntityTypeName:  "User",
			KeyFields:       []KeyField{{Name: "id"}},
			CacheName:       "default",
			InvalidateCache: true,
		})
		responseData := astjson.MustParse(`{"updateUsername":{"id":"1234","username":"NewMe"}}`)

		got := l.detectMutationEntityImpact(res, nil, responseData)
		assert.Nil(t, got)
	})

	t.Run("no MutationEntityImpactConfig returns nil", func(t *testing.T) {
		ctx := NewContext(context.Background())
		l := makeLoader(ctx, NewFakeLoaderCache(), "default")

		info := makeMutationInfo("updateUsername", mutationProvidesData)
		res := makeResult(nil) // no config
		responseData := astjson.MustParse(`{"updateUsername":{"id":"1234","username":"NewMe"}}`)

		got := l.detectMutationEntityImpact(res, info, responseData)
		assert.Nil(t, got)
	})

	t.Run("InvalidateCache true deletes cache entry and returns deletedKeys", func(t *testing.T) {
		cache := NewFakeLoaderCache()
		// Pre-populate cache with the entity
		cacheKey := `{"__typename":"User","key":{"id":"1234"}}`
		_ = cache.Set(context.Background(), []*CacheEntry{
			{Key: cacheKey, Value: []byte(`{"id":"1234","username":"OldMe"}`)},
		}, 0)

		ctx := NewContext(context.Background())
		ctx.ExecutionOptions.Caching.EnableCacheAnalytics = true
		ctx.initCacheAnalytics()

		l := makeLoader(ctx, cache, "default")

		cfg := &MutationEntityImpactConfig{
			EntityTypeName:  "User",
			KeyFields:       []KeyField{{Name: "id"}},
			CacheName:       "default",
			InvalidateCache: true,
		}
		info := makeMutationInfo("updateUsername", mutationProvidesData)
		res := makeResult(cfg)

		responseData, err := astjson.ParseWithArena(l.jsonArena, `{"updateUsername":{"id":"1234","username":"NewMe"}}`)
		require.NoError(t, err)

		deletedKeys := l.detectMutationEntityImpact(res, info, responseData)

		// Should return the deleted key
		assert.Equal(t, map[string]struct{}{cacheKey: {}}, deletedKeys)

		// Verify cache entry was actually deleted
		entries, _ := cache.Get(context.Background(), []string{cacheKey})
		assert.Nil(t, entries[0], "cache entry should be deleted")
	})

	t.Run("analytics enabled, no cached value records MutationEvent with HadCachedValue=false", func(t *testing.T) {
		cache := NewFakeLoaderCache() // empty cache

		ctx := NewContext(context.Background())
		ctx.ExecutionOptions.Caching.EnableCacheAnalytics = true
		ctx.initCacheAnalytics()

		l := makeLoader(ctx, cache, "default")

		cfg := &MutationEntityImpactConfig{
			EntityTypeName:  "User",
			KeyFields:       []KeyField{{Name: "id"}},
			CacheName:       "default",
			InvalidateCache: true,
		}
		info := makeMutationInfo("updateUsername", mutationProvidesData)
		res := makeResult(cfg)

		responseData, err := astjson.ParseWithArena(l.jsonArena, `{"updateUsername":{"id":"1234","username":"NewMe"}}`)
		require.NoError(t, err)

		_ = l.detectMutationEntityImpact(res, info, responseData)

		stats := ctx.GetCacheStats()
		require.Len(t, stats.MutationEvents, 1)

		event := stats.MutationEvents[0]
		assert.Equal(t, "updateUsername", event.MutationRootField)
		assert.Equal(t, "User", event.EntityType)
		assert.Equal(t, `{"__typename":"User","key":{"id":"1234"}}`, event.EntityCacheKey) // display key (no prefix)
		assert.Equal(t, false, event.HadCachedValue)                                       // no cached value in empty cache
		assert.Equal(t, false, event.IsStale)
		assert.Equal(t, uint64(0), event.CachedHash) // zero because no cached value
		assert.NotEqual(t, uint64(0), event.FreshHash)
		assert.Equal(t, 0, event.CachedBytes)
		assert.NotEqual(t, 0, event.FreshBytes)
	})

	t.Run("analytics enabled, stale cached value records MutationEvent with IsStale=true", func(t *testing.T) {
		cache := NewFakeLoaderCache()
		cacheKey := `{"__typename":"User","key":{"id":"1234"}}`
		// Cached value has username="OldMe" (differs from mutation response)
		_ = cache.Set(context.Background(), []*CacheEntry{
			{Key: cacheKey, Value: []byte(`{"id":"1234","username":"OldMe"}`)},
		}, 0)

		ctx := NewContext(context.Background())
		ctx.ExecutionOptions.Caching.EnableCacheAnalytics = true
		ctx.initCacheAnalytics()

		l := makeLoader(ctx, cache, "default")

		cfg := &MutationEntityImpactConfig{
			EntityTypeName:  "User",
			KeyFields:       []KeyField{{Name: "id"}},
			CacheName:       "default",
			InvalidateCache: true,
		}
		info := makeMutationInfo("updateUsername", mutationProvidesData)
		res := makeResult(cfg)

		responseData, err := astjson.ParseWithArena(l.jsonArena, `{"updateUsername":{"id":"1234","username":"NewMe"}}`)
		require.NoError(t, err)

		_ = l.detectMutationEntityImpact(res, info, responseData)

		stats := ctx.GetCacheStats()
		require.Len(t, stats.MutationEvents, 1)

		event := stats.MutationEvents[0]
		assert.Equal(t, "updateUsername", event.MutationRootField)
		assert.Equal(t, "User", event.EntityType)
		assert.Equal(t, true, event.HadCachedValue)  // cache was populated
		assert.Equal(t, true, event.IsStale)          // username changed: OldMe -> NewMe
		assert.NotEqual(t, uint64(0), event.CachedHash)
		assert.NotEqual(t, uint64(0), event.FreshHash)
		assert.NotEqual(t, event.CachedHash, event.FreshHash) // hashes differ because content differs
		assert.NotEqual(t, 0, event.CachedBytes)
		assert.NotEqual(t, 0, event.FreshBytes)
	})

	t.Run("analytics enabled, fresh cached value records MutationEvent with IsStale=false", func(t *testing.T) {
		cache := NewFakeLoaderCache()
		cacheKey := `{"__typename":"User","key":{"id":"1234"}}`
		// Cached value matches the mutation response exactly
		_ = cache.Set(context.Background(), []*CacheEntry{
			{Key: cacheKey, Value: []byte(`{"id":"1234","username":"NewMe"}`)},
		}, 0)

		ctx := NewContext(context.Background())
		ctx.ExecutionOptions.Caching.EnableCacheAnalytics = true
		ctx.initCacheAnalytics()

		l := makeLoader(ctx, cache, "default")

		cfg := &MutationEntityImpactConfig{
			EntityTypeName:  "User",
			KeyFields:       []KeyField{{Name: "id"}},
			CacheName:       "default",
			InvalidateCache: true,
		}
		info := makeMutationInfo("updateUsername", mutationProvidesData)
		res := makeResult(cfg)

		responseData, err := astjson.ParseWithArena(l.jsonArena, `{"updateUsername":{"id":"1234","username":"NewMe"}}`)
		require.NoError(t, err)

		_ = l.detectMutationEntityImpact(res, info, responseData)

		stats := ctx.GetCacheStats()
		require.Len(t, stats.MutationEvents, 1)

		event := stats.MutationEvents[0]
		assert.Equal(t, "updateUsername", event.MutationRootField)
		assert.Equal(t, "User", event.EntityType)
		assert.Equal(t, true, event.HadCachedValue)            // cache was populated
		assert.Equal(t, false, event.IsStale)                   // cached value matches mutation response
		assert.Equal(t, event.CachedHash, event.FreshHash)     // hashes are equal
		assert.NotEqual(t, uint64(0), event.CachedHash)
		assert.NotEqual(t, 0, event.CachedBytes)
		assert.NotEqual(t, 0, event.FreshBytes)
	})

	t.Run("InvalidateCache false with analytics records event but no Delete", func(t *testing.T) {
		cache := NewFakeLoaderCache()
		cacheKey := `{"__typename":"User","key":{"id":"1234"}}`
		_ = cache.Set(context.Background(), []*CacheEntry{
			{Key: cacheKey, Value: []byte(`{"id":"1234","username":"OldMe"}`)},
		}, 0)
		cache.ClearLog()

		ctx := NewContext(context.Background())
		ctx.ExecutionOptions.Caching.EnableCacheAnalytics = true
		ctx.initCacheAnalytics()

		l := makeLoader(ctx, cache, "default")

		cfg := &MutationEntityImpactConfig{
			EntityTypeName:  "User",
			KeyFields:       []KeyField{{Name: "id"}},
			CacheName:       "default",
			InvalidateCache: false, // no deletion
		}
		info := makeMutationInfo("updateUsername", mutationProvidesData)
		res := makeResult(cfg)

		responseData, err := astjson.ParseWithArena(l.jsonArena, `{"updateUsername":{"id":"1234","username":"NewMe"}}`)
		require.NoError(t, err)

		deletedKeys := l.detectMutationEntityImpact(res, info, responseData)
		assert.Nil(t, deletedKeys, "no keys should be deleted when InvalidateCache=false")

		// Verify only a Get was logged (for analytics), no Delete
		log := cache.GetLog()
		require.Len(t, log, 1, "exactly 1 cache operation: Get for analytics comparison")
		assert.Equal(t, "get", log[0].Operation)

		// Verify cache entry still exists
		entries, _ := cache.Get(context.Background(), []string{cacheKey})
		assert.NotNil(t, entries[0], "cache entry should still exist")

		// Verify MutationEvent was recorded
		stats := ctx.GetCacheStats()
		require.Len(t, stats.MutationEvents, 1)
		assert.Equal(t, true, stats.MutationEvents[0].HadCachedValue)
		assert.Equal(t, true, stats.MutationEvents[0].IsStale) // username changed
	})

	t.Run("no caches map returns nil", func(t *testing.T) {
		ctx := NewContext(context.Background())
		ctx.ExecutionOptions.Caching.EnableCacheAnalytics = true
		ctx.initCacheAnalytics()

		l := &Loader{
			jsonArena: arena.NewMonotonicArena(arena.WithMinBufferSize(1024)),
			ctx:       ctx,
			caches:    nil, // no caches
		}

		cfg := &MutationEntityImpactConfig{
			EntityTypeName:  "User",
			KeyFields:       []KeyField{{Name: "id"}},
			CacheName:       "default",
			InvalidateCache: true,
		}
		info := makeMutationInfo("updateUsername", mutationProvidesData)
		res := makeResult(cfg)

		responseData := astjson.MustParse(`{"updateUsername":{"id":"1234","username":"NewMe"}}`)

		got := l.detectMutationEntityImpact(res, info, responseData)
		assert.Nil(t, got)
	})

	t.Run("nil ProvidesData returns nil", func(t *testing.T) {
		ctx := NewContext(context.Background())
		ctx.ExecutionOptions.Caching.EnableCacheAnalytics = true
		ctx.initCacheAnalytics()

		l := makeLoader(ctx, NewFakeLoaderCache(), "default")

		cfg := &MutationEntityImpactConfig{
			EntityTypeName:  "User",
			KeyFields:       []KeyField{{Name: "id"}},
			CacheName:       "default",
			InvalidateCache: true,
		}
		info := &FetchInfo{
			OperationType:  ast.OperationTypeMutation,
			DataSourceName: "accounts",
			RootFields: []GraphCoordinate{
				{TypeName: "Mutation", FieldName: "updateUsername"},
			},
			ProvidesData: nil, // no ProvidesData
		}
		res := makeResult(cfg)

		responseData := astjson.MustParse(`{"updateUsername":{"id":"1234","username":"NewMe"}}`)

		got := l.detectMutationEntityImpact(res, info, responseData)
		assert.Nil(t, got)
	})

	t.Run("response data not an object returns nil", func(t *testing.T) {
		ctx := NewContext(context.Background())
		ctx.ExecutionOptions.Caching.EnableCacheAnalytics = true
		ctx.initCacheAnalytics()

		l := makeLoader(ctx, NewFakeLoaderCache(), "default")

		cfg := &MutationEntityImpactConfig{
			EntityTypeName:  "User",
			KeyFields:       []KeyField{{Name: "id"}},
			CacheName:       "default",
			InvalidateCache: true,
		}
		info := makeMutationInfo("updateUsername", mutationProvidesData)
		res := makeResult(cfg)

		// Mutation returns a string instead of object
		responseData := astjson.MustParse(`{"updateUsername":"not-an-object"}`)

		got := l.detectMutationEntityImpact(res, info, responseData)
		assert.Nil(t, got)
	})

	t.Run("array response invalidates all entities in the list", func(t *testing.T) {
		cache := NewFakeLoaderCache()
		// Pre-populate cache with two entities
		cacheKey1 := `{"__typename":"User","key":{"id":"1"}}`
		cacheKey2 := `{"__typename":"User","key":{"id":"2"}}`
		_ = cache.Set(context.Background(), []*CacheEntry{
			{Key: cacheKey1, Value: []byte(`{"id":"1","username":"Alice"}`)},
			{Key: cacheKey2, Value: []byte(`{"id":"2","username":"Bob"}`)},
		}, 0)

		ctx := NewContext(context.Background())
		ctx.ExecutionOptions.Caching.EnableCacheAnalytics = true
		ctx.initCacheAnalytics()

		l := makeLoader(ctx, cache, "default")

		// ProvidesData for a list mutation: {deleteUsers: [{id, username}]}
		listEntityProvidesData := &Object{
			Fields: []*Field{
				{Name: []byte("id"), Value: &Scalar{Path: []string{"id"}}},
				{Name: []byte("username"), Value: &Scalar{Path: []string{"username"}}},
			},
		}
		listMutationProvidesData := &Object{
			Fields: []*Field{
				{Name: []byte("deleteUsers"), Value: listEntityProvidesData},
			},
		}

		cfg := &MutationEntityImpactConfig{
			EntityTypeName:  "User",
			KeyFields:       []KeyField{{Name: "id"}},
			CacheName:       "default",
			InvalidateCache: true,
		}
		info := makeMutationInfo("deleteUsers", listMutationProvidesData)
		res := makeResult(cfg)

		// Mutation returns an array of entities
		responseData, err := astjson.ParseWithArena(l.jsonArena, `{"deleteUsers":[{"id":"1","username":"Alice"},{"id":"2","username":"Bob"}]}`)
		require.NoError(t, err)

		deletedKeys := l.detectMutationEntityImpact(res, info, responseData)

		// Both entities should be invalidated
		assert.Equal(t, map[string]struct{}{cacheKey1: {}, cacheKey2: {}}, deletedKeys)

		// Verify both cache entries were deleted
		entries, _ := cache.Get(context.Background(), []string{cacheKey1, cacheKey2})
		assert.Nil(t, entries[0], "first entity should be deleted")
		assert.Nil(t, entries[1], "second entity should be deleted")

		// Verify analytics recorded events for both entities
		stats := ctx.GetCacheStats()
		require.Len(t, stats.MutationEvents, 2, "should record mutation event for each entity in the list")
		assert.Equal(t, cacheKey1, stats.MutationEvents[0].EntityCacheKey)
		assert.Equal(t, true, stats.MutationEvents[0].HadCachedValue)
		assert.Equal(t, cacheKey2, stats.MutationEvents[1].EntityCacheKey)
		assert.Equal(t, true, stats.MutationEvents[1].HadCachedValue)
	})

	t.Run("array response with non-object items skips them", func(t *testing.T) {
		cache := NewFakeLoaderCache()
		cacheKey := `{"__typename":"User","key":{"id":"1"}}`
		_ = cache.Set(context.Background(), []*CacheEntry{
			{Key: cacheKey, Value: []byte(`{"id":"1","username":"Alice"}`)},
		}, 0)

		ctx := NewContext(context.Background())
		l := makeLoader(ctx, cache, "default")

		listEntityProvidesData := &Object{
			Fields: []*Field{
				{Name: []byte("id"), Value: &Scalar{Path: []string{"id"}}},
				{Name: []byte("username"), Value: &Scalar{Path: []string{"username"}}},
			},
		}
		listMutationProvidesData := &Object{
			Fields: []*Field{
				{Name: []byte("deleteUsers"), Value: listEntityProvidesData},
			},
		}

		cfg := &MutationEntityImpactConfig{
			EntityTypeName:  "User",
			KeyFields:       []KeyField{{Name: "id"}},
			CacheName:       "default",
			InvalidateCache: true,
		}
		info := makeMutationInfo("deleteUsers", listMutationProvidesData)
		res := makeResult(cfg)

		// Array with mixed types: one valid object, one null, one string
		responseData, err := astjson.ParseWithArena(l.jsonArena, `{"deleteUsers":[{"id":"1","username":"Alice"},null,"invalid"]}`)
		require.NoError(t, err)

		deletedKeys := l.detectMutationEntityImpact(res, info, responseData)

		// Only the valid object entity should be invalidated
		assert.Equal(t, map[string]struct{}{cacheKey: {}}, deletedKeys)
	})
}

