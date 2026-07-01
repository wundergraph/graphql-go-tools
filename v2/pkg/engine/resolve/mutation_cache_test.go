package resolve

import (
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

// ---------------------------------------------------------------------------
// navigateProvidesDataToField
// ---------------------------------------------------------------------------

// TestNavigateProvidesDataToField verifies the ProvidesData tree navigation used
// by mutation cache impact detection to find the entity object under a root field.
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
// buildEntityKeyValue (Loader method)
// ---------------------------------------------------------------------------

// testBuildEntityKeyValue is a test helper that creates a minimal Loader
// to call the buildEntityKeyValue method.
func testBuildEntityKeyValue(ar arena.Arena, data *astjson.Value, keyFields []KeyField) *astjson.Value {
	l := &Loader{jsonArena: ar}
	return l.buildEntityKeyValue(data, keyFields)
}

// TestBuildEntityKeyValue verifies that entity key construction from response data
// handles simple, composite, and nested @key fields correctly.
func TestBuildEntityKeyValue(t *testing.T) {
	t.Run("simple key", func(t *testing.T) {
		ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
		data, err := astjson.ParseWithArena(ar, `{"id":"123","name":"Alice"}`)
		require.NoError(t, err)

		result := testBuildEntityKeyValue(ar, data, []KeyField{{Name: "id"}})
		got := string(result.MarshalTo(nil))

		assert.Equal(t, `{"id":"123"}`, got)
	})

	t.Run("composite key", func(t *testing.T) {
		ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
		data, err := astjson.ParseWithArena(ar, `{"id":"1","orgId":"acme","name":"Bob"}`)
		require.NoError(t, err)

		result := testBuildEntityKeyValue(ar, data, []KeyField{{Name: "id"}, {Name: "orgId"}})
		got := string(result.MarshalTo(nil))

		assert.Equal(t, `{"id":"1","orgId":"acme"}`, got)
	})

	t.Run("nested key", func(t *testing.T) {
		ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
		data, err := astjson.ParseWithArena(ar, `{"key":{"subId":"x"},"name":"Carol"}`)
		require.NoError(t, err)

		result := testBuildEntityKeyValue(ar, data, []KeyField{
			{Name: "key", Children: []KeyField{{Name: "subId"}}},
		})
		got := string(result.MarshalTo(nil))

		assert.Equal(t, `{"key":{"subId":"x"}}`, got)
	})

	t.Run("missing field in data omits field from output", func(t *testing.T) {
		ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
		data, err := astjson.ParseWithArena(ar, `{"name":"Dave"}`)
		require.NoError(t, err)

		result := testBuildEntityKeyValue(ar, data, []KeyField{{Name: "id"}})
		got := string(result.MarshalTo(nil))

		// "id" is missing in data, so it is omitted from the result
		assert.Equal(t, `{}`, got)
	})

	t.Run("numeric key coerced to string to match read-path key", func(t *testing.T) {
		// A mutation returning an Int @key (e.g. Employee.id) yields {"id":1} in the
		// response data. The entity-fetch (read) key builder coerces numbers to strings,
		// so the write/invalidate key must too — otherwise {"id":1} never invalidates the
		// cached read {"id":"1"}.
		ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
		data, err := astjson.ParseWithArena(ar, `{"id":1,"__typename":"Employee","isAvailable":false}`)
		require.NoError(t, err)

		result := testBuildEntityKeyValue(ar, data, []KeyField{{Name: "id"}})
		got := string(result.MarshalTo(nil))

		assert.Equal(t, `{"id":"1"}`, got)
	})

	t.Run("nested numeric key coerced to string", func(t *testing.T) {
		ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
		data, err := astjson.ParseWithArena(ar, `{"key":{"id":42},"name":"Eve"}`)
		require.NoError(t, err)

		result := testBuildEntityKeyValue(ar, data, []KeyField{
			{Name: "key", Children: []KeyField{{Name: "id"}}},
		})
		got := string(result.MarshalTo(nil))

		assert.Equal(t, `{"key":{"id":"42"}}`, got)
	})
}

// ---------------------------------------------------------------------------
// buildMutationEntityCacheKey
// ---------------------------------------------------------------------------

// TestBuildMutationEntityCacheKey verifies that mutation cache key construction
// applies header prefix, global prefix, and L2 interceptor transformations correctly.
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
// detectMutationEntityImpact
// ---------------------------------------------------------------------------

// TestDetectMutationEntityImpact verifies that after a mutation completes, the resolver
// correctly detects impacted entities and invalidates/records analytics for them.
// Without this, stale cached entities would persist after mutations.
func TestDetectMutationEntityImpact(t *testing.T) {
	// Helper: builds a Loader with minimal fields for detectMutationEntityImpact.
	makeLoader := func(ctx *Context, cache LoaderCache, cacheName string) *Loader {
		ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
		return &Loader{
			jsonArena: ar,
			ctx:       ctx,
			caches:    map[string]LoaderCache{cacheName: cache},
			l1Cache:   map[string]*astjson.Value{},
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
		_ = cache.Set(context.Background(), withCacheEntryTTL([]*CacheEntry{
			{Key: cacheKey, Value: []byte(`{"id":"1234","username":"OldMe"}`)},
		}, 0))
		cache.ClearLog()

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

	t.Run("PopulateCache true writes mutation response payload to L2", func(t *testing.T) {
		// Single-subgraph mutations annotated with @cachePopulate have no follow-up
		// entity fetch to inherit EnableMutationL2CachePopulation. The populate path
		// inside detectSingleMutationEntityImpact must write the entity payload to L2
		// directly so a subsequent read by the same key hits cache.
		cache := NewFakeLoaderCache()
		cacheKey := `{"__typename":"User","key":{"id":"u-pop"}}`

		ctx := NewContext(context.Background())
		ctx.ExecutionOptions.Caching.EnableL2Cache = true
		l := makeLoader(ctx, cache, "default")

		cfg := &MutationEntityImpactConfig{
			EntityTypeName: "User",
			KeyFields:      []KeyField{{Name: "id"}},
			CacheName:      "default",
			PopulateCache:  true,
			PopulateTTL:    60 * time.Second,
		}
		info := makeMutationInfo("updateUsername", mutationProvidesData)
		res := makeResult(cfg)

		responseData, err := astjson.ParseWithArena(l.jsonArena,
			`{"updateUsername":{"id":"u-pop","username":"PopMe"}}`)
		require.NoError(t, err)

		_ = l.detectMutationEntityImpact(res, info, responseData)

		// Verify the entity payload was written to L2 under the entity cache key.
		entries, err := cache.Get(context.Background(), []string{cacheKey})
		require.NoError(t, err)
		require.NotNil(t, entries[0], "PopulateCache should write the entity to L2")
		assert.Equal(t, `{"id":"u-pop","username":"PopMe"}`, string(entries[0].Value),
			"cached payload must equal the entity projection through ProvidesData")
	})

	t.Run("PopulateCache true does not write to L2 when L2 is disabled", func(t *testing.T) {
		cache := NewFakeLoaderCache()
		cacheKey := `{"__typename":"User","key":{"id":"u-pop-disabled"}}`

		ctx := NewContext(context.Background())
		ctx.ExecutionOptions.Caching.EnableL2Cache = false
		l := makeLoader(ctx, cache, "default")

		cfg := &MutationEntityImpactConfig{
			EntityTypeName: "User",
			KeyFields:      []KeyField{{Name: "id"}},
			CacheName:      "default",
			PopulateCache:  true,
			PopulateTTL:    60 * time.Second,
		}
		info := makeMutationInfo("updateUsername", mutationProvidesData)
		res := makeResult(cfg)

		responseData, err := astjson.ParseWithArena(l.jsonArena,
			`{"updateUsername":{"id":"u-pop-disabled","username":"PopMe"}}`)
		require.NoError(t, err)

		_ = l.detectMutationEntityImpact(res, info, responseData)

		entries, err := cache.Get(context.Background(), []string{cacheKey})
		require.NoError(t, err)
		assert.Nil(t, entries[0], "PopulateCache must respect EnableL2Cache=false")
	})

	t.Run("PopulateCache false does not write to L2", func(t *testing.T) {
		// Defensive: when neither PopulateCache nor InvalidateCache is set and
		// analytics is off, detectMutationEntityImpact must not touch the cache.
		cache := NewFakeLoaderCache()

		ctx := NewContext(context.Background())
		l := makeLoader(ctx, cache, "default")

		cfg := &MutationEntityImpactConfig{
			EntityTypeName: "User",
			KeyFields:      []KeyField{{Name: "id"}},
			CacheName:      "default",
			// PopulateCache: false, InvalidateCache: false, no analytics
		}
		info := makeMutationInfo("updateUsername", mutationProvidesData)
		res := makeResult(cfg)

		responseData, err := astjson.ParseWithArena(l.jsonArena,
			`{"updateUsername":{"id":"u1","username":"NoPop"}}`)
		require.NoError(t, err)

		_ = l.detectMutationEntityImpact(res, info, responseData)

		// Cache must be untouched.
		assert.Empty(t, cache.GetLog(), "with no impact config flags set, cache must not be touched")
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

	t.Run("analytics enabled still avoids mutation-time cache reads for stale entries", func(t *testing.T) {
		cache := NewFakeLoaderCache()
		cacheKey := `{"__typename":"User","key":{"id":"1234"}}`
		// Cached value has username="OldMe" (differs from mutation response)
		_ = cache.Set(context.Background(), withCacheEntryTTL([]*CacheEntry{
			{Key: cacheKey, Value: []byte(`{"id":"1234","username":"OldMe"}`)},
		}, 0))
		cache.ClearLog()

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
		assert.Equal(t, false, event.HadCachedValue)
		assert.Equal(t, false, event.IsStale)
		assert.Equal(t, uint64(0), event.CachedHash)
		assert.NotEqual(t, uint64(0), event.FreshHash)
		assert.Equal(t, 0, event.CachedBytes)
		assert.NotEqual(t, 0, event.FreshBytes)
		assert.Equal(t, []CacheLogEntry{{Operation: "delete", Items: []CacheLogItem{{Key: cacheKey}}}}, cache.GetLog())
	})

	t.Run("analytics enabled still avoids mutation-time cache reads for fresh entries", func(t *testing.T) {
		cache := NewFakeLoaderCache()
		cacheKey := `{"__typename":"User","key":{"id":"1234"}}`
		// Cached value matches the mutation response exactly
		_ = cache.Set(context.Background(), withCacheEntryTTL([]*CacheEntry{
			{Key: cacheKey, Value: []byte(`{"id":"1234","username":"NewMe"}`)},
		}, 0))
		cache.ClearLog()

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
		assert.Equal(t, false, event.HadCachedValue)
		assert.Equal(t, false, event.IsStale)
		assert.Equal(t, uint64(0), event.CachedHash)
		assert.Equal(t, 0, event.CachedBytes)
		assert.NotEqual(t, 0, event.FreshBytes)
		assert.Equal(t, []CacheLogEntry{{Operation: "delete", Items: []CacheLogItem{{Key: cacheKey}}}}, cache.GetLog())
	})

	t.Run("InvalidateCache false with analytics records event but no Delete", func(t *testing.T) {
		cache := NewFakeLoaderCache()
		cacheKey := `{"__typename":"User","key":{"id":"1234"}}`
		_ = cache.Set(context.Background(), withCacheEntryTTL([]*CacheEntry{
			{Key: cacheKey, Value: []byte(`{"id":"1234","username":"OldMe"}`)},
		}, 0))
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

		// Verify mutation analytics does not issue a cache read.
		log := cache.GetLog()
		require.Len(t, log, 0, "mutation impact analytics must not read from cache")

		// Verify cache entry still exists
		entries, _ := cache.Get(context.Background(), []string{cacheKey})
		assert.NotNil(t, entries[0], "cache entry should still exist")

		// Verify MutationEvent was recorded
		stats := ctx.GetCacheStats()
		require.Len(t, stats.MutationEvents, 1)
		assert.Equal(t, false, stats.MutationEvents[0].HadCachedValue)
		assert.Equal(t, false, stats.MutationEvents[0].IsStale)
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
		_ = cache.Set(context.Background(), withCacheEntryTTL([]*CacheEntry{
			{Key: cacheKey1, Value: []byte(`{"id":"1","username":"Alice"}`)},
			{Key: cacheKey2, Value: []byte(`{"id":"2","username":"Bob"}`)},
		}, 0))

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
		assert.Equal(t, false, stats.MutationEvents[0].HadCachedValue)
		assert.Equal(t, cacheKey2, stats.MutationEvents[1].EntityCacheKey)
		assert.Equal(t, false, stats.MutationEvents[1].HadCachedValue)
	})

	t.Run("array response with non-object items skips them", func(t *testing.T) {
		cache := NewFakeLoaderCache()
		cacheKey := `{"__typename":"User","key":{"id":"1"}}`
		_ = cache.Set(context.Background(), withCacheEntryTTL([]*CacheEntry{
			{Key: cacheKey, Value: []byte(`{"id":"1","username":"Alice"}`)},
		}, 0))

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

// ---------------------------------------------------------------------------
// MutationCacheTTLOverride
// ---------------------------------------------------------------------------

// TestMutationCacheTTLOverride verifies that MutationCacheTTLOverride takes precedence
// over the entity's default TTL when mutations populate L2 cache.
// Without this, mutation-written cache entries could have inappropriately long TTLs.
func TestMutationCacheTTLOverride(t *testing.T) {
	t.Run("mutation with TTL override uses override value", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		cache := NewFakeLoaderCache()

		rootDS := NewMockDataSource(ctrl)
		rootDS.EXPECT().
			Load(gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, headers any, input []byte) ([]byte, error) {
				return []byte(`{"data":{"updateUser":{"__typename":"User","id":"u1"}}}`), nil
			}).Times(1)

		entityDS := NewMockDataSource(ctrl)
		entityDS.EXPECT().
			Load(gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, headers any, input []byte) ([]byte, error) {
				return []byte(`{"data":{"_entities":[{"name":"Alice"}]}}`), nil
			}).Times(1)

		response := buildMutationTTLResponse(
			rootDS, entityDS,
			newMutationUserCacheKeyTemplate(), newMutationUserProvidesData(),
			true,            // enableL2Population
			60*time.Second,  // mutationTTLOverride
			300*time.Second, // entityTTL (entity default)
		)

		loader := &Loader{caches: map[string]LoaderCache{"default": cache}}
		ctx := NewContext(context.Background())
		ctx.ExecutionOptions.DisableSubgraphRequestDeduplication = true
		ctx.ExecutionOptions.Caching.EnableL2Cache = true

		ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
		resolvable := NewResolvable(ar, ResolvableOptions{})
		err := resolvable.Init(ctx, nil, ast.OperationTypeMutation)
		require.NoError(t, err)

		err = loader.LoadGraphQLResponseData(ctx, response, resolvable)
		require.NoError(t, err)

		out := string(fastjsonext.PrintGraphQLResponse(resolvable.data, resolvable.errors))
		assert.Equal(t, `{"data":{"updateUser":{"__typename":"User","id":"u1","name":"Alice"}}}`, out)

		// No L2 "get" because mutations skip L2 reads (AC-MUT-01).
		// L2 Set uses override TTL (60s), not entity default (300s),
		// because EnableMutationL2CachePopulation=true and MutationCacheTTLOverride=60s.
		cacheLog := cache.GetLog()
		assert.Equal(t, []CacheLogEntry{
			{Operation: "set", Items: []CacheLogItem{{Key: `{"__typename":"User","key":{"id":"u1"}}`, TTL: 60 * time.Second}}}, // L2 write uses mutation TTL override (60s), not entity default (300s); no prior "get" because mutations skip L2 reads
		}, cacheLog)
	})

	t.Run("mutation without TTL override uses entity default", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		cache := NewFakeLoaderCache()

		rootDS := NewMockDataSource(ctrl)
		rootDS.EXPECT().
			Load(gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, headers any, input []byte) ([]byte, error) {
				return []byte(`{"data":{"updateUser":{"__typename":"User","id":"u1"}}}`), nil
			}).Times(1)

		entityDS := NewMockDataSource(ctrl)
		entityDS.EXPECT().
			Load(gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, headers any, input []byte) ([]byte, error) {
				return []byte(`{"data":{"_entities":[{"name":"Bob"}]}}`), nil
			}).Times(1)

		response := buildMutationTTLResponse(
			rootDS, entityDS,
			newMutationUserCacheKeyTemplate(), newMutationUserProvidesData(),
			true,            // enableL2Population
			0,               // mutationTTLOverride=0 means no override
			300*time.Second, // entityTTL (entity default)
		)

		loader := &Loader{caches: map[string]LoaderCache{"default": cache}}
		ctx := NewContext(context.Background())
		ctx.ExecutionOptions.DisableSubgraphRequestDeduplication = true
		ctx.ExecutionOptions.Caching.EnableL2Cache = true

		ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
		resolvable := NewResolvable(ar, ResolvableOptions{})
		err := resolvable.Init(ctx, nil, ast.OperationTypeMutation)
		require.NoError(t, err)

		err = loader.LoadGraphQLResponseData(ctx, response, resolvable)
		require.NoError(t, err)

		out := string(fastjsonext.PrintGraphQLResponse(resolvable.data, resolvable.errors))
		assert.Equal(t, `{"data":{"updateUser":{"__typename":"User","id":"u1","name":"Bob"}}}`, out)

		// No L2 "get" because mutations skip L2 reads (AC-MUT-01).
		// L2 Set uses entity default TTL (300s) because MutationCacheTTLOverride=0.
		cacheLog := cache.GetLog()
		assert.Equal(t, []CacheLogEntry{
			{Operation: "set", Items: []CacheLogItem{{Key: `{"__typename":"User","key":{"id":"u1"}}`, TTL: 300 * time.Second}}}, // L2 write uses entity default TTL (300s); no mutation override (MutationCacheTTLOverride=0)
		}, cacheLog)
	})

	t.Run("TTL override not applied when mutation L2 population disabled", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		cache := NewFakeLoaderCache()

		rootDS := NewMockDataSource(ctrl)
		rootDS.EXPECT().
			Load(gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, headers any, input []byte) ([]byte, error) {
				return []byte(`{"data":{"updateUser":{"__typename":"User","id":"u1"}}}`), nil
			}).Times(1)

		entityDS := NewMockDataSource(ctrl)
		entityDS.EXPECT().
			Load(gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, headers any, input []byte) ([]byte, error) {
				return []byte(`{"data":{"_entities":[{"name":"Carol"}]}}`), nil
			}).Times(1)

		response := buildMutationTTLResponse(
			rootDS, entityDS,
			newMutationUserCacheKeyTemplate(), newMutationUserProvidesData(),
			false,           // enableL2Population=false — mutations do NOT write to L2
			60*time.Second,  // mutationTTLOverride is set but irrelevant since L2 writes are disabled
			300*time.Second, // entityTTL
		)

		loader := &Loader{caches: map[string]LoaderCache{"default": cache}}
		ctx := NewContext(context.Background())
		ctx.ExecutionOptions.DisableSubgraphRequestDeduplication = true
		ctx.ExecutionOptions.Caching.EnableL2Cache = true

		ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
		resolvable := NewResolvable(ar, ResolvableOptions{})
		err := resolvable.Init(ctx, nil, ast.OperationTypeMutation)
		require.NoError(t, err)

		err = loader.LoadGraphQLResponseData(ctx, response, resolvable)
		require.NoError(t, err)

		out := string(fastjsonext.PrintGraphQLResponse(resolvable.data, resolvable.errors))
		assert.Equal(t, `{"data":{"updateUser":{"__typename":"User","id":"u1","name":"Carol"}}}`, out)

		// No L2 operations at all — mutations skip L2 entirely when EnableMutationL2CachePopulation=false
		cacheLog := cache.GetLog()
		assert.Equal(t, []CacheLogEntry{}, cacheLog)
	})
}

// ---------------------------------------------------------------------------
// Helpers for mutation cache tests
// ---------------------------------------------------------------------------

// buildMutationTTLResponse creates a GraphQLResponse for testing mutation TTL override.
// The root fetch is a mutation that sets EnableMutationL2CachePopulation and MutationCacheTTLOverride
// on the Loader. The entity fetch that follows inherits these flags via resolveSingle propagation.
func buildMutationTTLResponse(
	rootDS, entityDS DataSource,
	cacheKeyTemplate CacheKeyTemplate,
	providesData *Object,
	enableL2Population bool,
	mutationTTLOverride time.Duration,
	entityTTL time.Duration,
) *GraphQLResponse {
	return &GraphQLResponse{
		Info: &GraphQLResponseInfo{OperationType: ast.OperationTypeMutation},
		Fetches: Sequence(
			// Root mutation fetch — propagates EnableMutationL2CachePopulation and MutationCacheTTLOverride to Loader
			SingleWithPath(&SingleFetch{
				FetchConfiguration: FetchConfiguration{
					DataSource:     rootDS,
					PostProcessing: PostProcessingConfiguration{SelectResponseDataPath: []string{"data"}},
					Caching: FetchCacheConfiguration{
						EnableMutationL2CachePopulation: enableL2Population,
						MutationCacheTTLOverride:        mutationTTLOverride,
					},
				},
				InputTemplate: InputTemplate{Segments: []TemplateSegment{
					{Data: []byte(`{"method":"POST","url":"http://accounts.service","body":{"query":"mutation{updateUser(id:\"u1\",name:\"Alice\"){__typename id}}"}}`), SegmentType: StaticSegmentType},
				}},
				DataSourceIdentifier: []byte("graphql_datasource.Source"),
				Info: &FetchInfo{
					DataSourceID: "accounts", DataSourceName: "accounts",
					RootFields:    []GraphCoordinate{{TypeName: "Mutation", FieldName: "updateUser"}},
					OperationType: ast.OperationTypeMutation,
				},
			}, "mutation"),

			// Entity fetch — inherits mutation L2 flags, uses caching config with entity TTL
			SingleWithPath(&SingleFetch{
				FetchConfiguration: FetchConfiguration{
					DataSource:     entityDS,
					PostProcessing: PostProcessingConfiguration{SelectResponseDataPath: []string{"data", "_entities", "0"}},
					Caching: FetchCacheConfiguration{
						Enabled:          true,
						CacheName:        "default",
						TTL:              entityTTL,
						CacheKeyTemplate: cacheKeyTemplate,
						UseL1Cache:       true,
					},
				},
				InputTemplate: InputTemplate{Segments: []TemplateSegment{
					{Data: []byte(`{"method":"POST","url":"http://accounts.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {name}}}","variables":{"representations":[`), SegmentType: StaticSegmentType},
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
					DataSourceID: "accounts", DataSourceName: "accounts",
					RootFields:    []GraphCoordinate{{TypeName: "User", FieldName: "name"}},
					OperationType: ast.OperationTypeQuery, // Entity fetches resolve from non-root types, so planner sets Query
					ProvidesData:  providesData,
				},
			}, "mutation.updateUser", ObjectPath("updateUser")),
		),
		Data: &Object{
			Fields: []*Field{{
				Name: []byte("updateUser"),
				Value: &Object{
					Path: []string{"updateUser"},
					Fields: []*Field{
						{Name: []byte("id"), Value: &String{Path: []string{"id"}}},
						{Name: []byte("name"), Value: &String{Path: []string{"name"}}},
					},
				},
			}},
		},
	}
}

// newMutationUserCacheKeyTemplate returns a cache key template for User entities in mutation tests.
func newMutationUserCacheKeyTemplate() CacheKeyTemplate {
	return &EntityQueryCacheKeyTemplate{
		Keys: NewResolvableObjectVariable(&Object{
			Fields: []*Field{
				{Name: []byte("__typename"), Value: &String{Path: []string{"__typename"}}},
				{Name: []byte("id"), Value: &String{Path: []string{"id"}}},
			},
		}),
	}
}

// newMutationUserProvidesData returns a ProvidesData for User entities in mutation tests.
func newMutationUserProvidesData() *Object {
	return &Object{
		Fields: []*Field{
			{Name: []byte("name"), Value: &Scalar{Path: []string{"name"}, Nullable: false}},
		},
	}
}
