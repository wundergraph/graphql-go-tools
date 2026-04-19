package resolve

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/wundergraph/astjson"
	"github.com/wundergraph/go-arena"
)

// TestRootFieldL2CachePrefix verifies that rootFieldL2CachePrefix correctly
// combines the global prefix and header hash into an L2 cache key prefix.
//
// The `includeHeaderPrefix` flag is the source of truth for whether header
// partitioning is active for this fetch — it's set in tryL2CacheLoad alongside
// headerHash whenever `IncludeSubgraphHeaderPrefix && SubgraphHeadersBuilder != nil`.
// The flag matters for the empty-headers case: hash == 0 from "no headers
// forwarded" must still produce a "0:" prefix so the WRITE key matches the
// READ key (which always builds the prefix when partitioning is active).
func TestRootFieldL2CachePrefix(t *testing.T) {
	tests := []struct {
		name                string
		globalPrefix        string
		headerHash          uint64
		includeHeaderPrefix bool
		expected            string
	}{
		{
			name:                "both globalPrefix and headerHash present",
			globalPrefix:        "tenant123",
			headerHash:          12345,
			includeHeaderPrefix: true,
			expected:            "tenant123:12345",
		},
		{
			name:                "headerHash only",
			globalPrefix:        "",
			headerHash:          12345,
			includeHeaderPrefix: true,
			expected:            "12345",
		},
		{
			name:                "globalPrefix only, no header partitioning",
			globalPrefix:        "tenant123",
			headerHash:          0,
			includeHeaderPrefix: false,
			expected:            "tenant123",
		},
		{
			name:                "neither present, no header partitioning",
			globalPrefix:        "",
			headerHash:          0,
			includeHeaderPrefix: false,
			expected:            "",
		},
		// REGRESSION: includeHeaders=true with no headers forwarded (hash=0).
		// Previously the WRITE path dropped the prefix because hash==0,
		// while the READ path built "0:..." — every read missed.
		{
			name:                "includeHeaders=true, hash=0 (no headers forwarded), no globalPrefix",
			globalPrefix:        "",
			headerHash:          0,
			includeHeaderPrefix: true,
			expected:            "0",
		},
		{
			name:                "includeHeaders=true, hash=0 (no headers forwarded), with globalPrefix",
			globalPrefix:        "tenant123",
			headerHash:          0,
			includeHeaderPrefix: true,
			expected:            "tenant123:0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := NewContext(context.Background())
			ctx.ExecutionOptions.Caching.GlobalCacheKeyPrefix = tt.globalPrefix

			l := &Loader{ctx: ctx}
			res := &result{
				headerHash:          tt.headerHash,
				includeHeaderPrefix: tt.includeHeaderPrefix,
			}

			got := l.rootFieldL2CachePrefix(res)
			assert.Equal(t, tt.expected, got)
		})
	}
}

// TestApplyL2CacheKeyInterceptor verifies that applyL2CacheKeyInterceptor
// returns the key unchanged when no interceptor is set, and applies the
// interceptor function correctly when one is configured.
func TestApplyL2CacheKeyInterceptor(t *testing.T) {
	t.Run("nil interceptor returns key unchanged", func(t *testing.T) {
		ctx := NewContext(context.Background())
		// No interceptor set (nil by default)

		l := &Loader{ctx: ctx}
		res := &result{
			ds:          DataSourceInfo{Name: "accounts"},
			cacheConfig: FetchCacheConfiguration{CacheName: "default"},
		}

		got := l.applyL2CacheKeyInterceptor("entity:user:1", res)
		assert.Equal(t, "entity:user:1", got)
	})

	t.Run("interceptor that prepends tenant", func(t *testing.T) {
		ctx := NewContext(context.Background())
		ctx.ExecutionOptions.Caching.L2CacheKeyInterceptor = func(_ context.Context, key string, _ L2CacheKeyInterceptorInfo) string {
			return "tenantX:" + key
		}

		l := &Loader{ctx: ctx}
		res := &result{
			ds:          DataSourceInfo{Name: "accounts"},
			cacheConfig: FetchCacheConfiguration{CacheName: "default"},
		}

		got := l.applyL2CacheKeyInterceptor("entity:user:1", res)
		assert.Equal(t, "tenantX:entity:user:1", got)
	})

	t.Run("interceptor uses fetchInfo DataSourceName", func(t *testing.T) {
		ctx := NewContext(context.Background())
		var capturedInfo L2CacheKeyInterceptorInfo
		ctx.ExecutionOptions.Caching.L2CacheKeyInterceptor = func(_ context.Context, key string, info L2CacheKeyInterceptorInfo) string {
			capturedInfo = info
			return key
		}

		l := &Loader{ctx: ctx}
		res := &result{
			ds:          DataSourceInfo{Name: "accounts"},
			cacheConfig: FetchCacheConfiguration{CacheName: "myCache"},
			fetchInfo:   &FetchInfo{DataSourceName: "overridden-accounts"},
		}

		l.applyL2CacheKeyInterceptor("key", res)
		// fetchInfo.DataSourceName overrides ds.Name
		assert.Equal(t, L2CacheKeyInterceptorInfo{
			SubgraphName: "overridden-accounts",
			CacheName:    "myCache",
		}, capturedInfo)
	})
}

// TestCompareCacheCandidateFreshness verifies the ordering logic that selects
// the freshest cache candidate when multiple L2 entries exist for the same key.
func TestCompareCacheCandidateFreshness(t *testing.T) {
	tests := []struct {
		name     string
		a, b     time.Duration
		expected int
	}{
		{
			name:     "both unknown (0, 0) — equal",
			a:        0,
			b:        0,
			expected: 0,
		},
		{
			name:     "only a known — a is fresher",
			a:        100 * time.Millisecond,
			b:        0,
			expected: -1,
		},
		{
			name:     "only b known — b is fresher",
			a:        0,
			b:        100 * time.Millisecond,
			expected: 1,
		},
		{
			name:     "both known, b has more remaining TTL — b is fresher",
			a:        100 * time.Millisecond,
			b:        200 * time.Millisecond,
			expected: 1,
		},
		{
			name:     "both known, equal TTL",
			a:        100 * time.Millisecond,
			b:        100 * time.Millisecond,
			expected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := compareCacheCandidateFreshness(tt.a, tt.b)
			assert.Equal(t, tt.expected, got)
		})
	}
}

// TestMergeCachedValueForWrite verifies that mergeCachedValueForWrite preserves
// older cached fields while letting fresh fields win on overlap.
func TestMergeCachedValueForWrite(t *testing.T) {
	a := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))

	t.Run("cachedValue nil returns freshValue", func(t *testing.T) {
		fresh := astjson.MustParse(`{"name":"Alice"}`)
		got := mergeCachedValueForWrite(a, nil, fresh)
		assert.Equal(t, `{"name":"Alice"}`, string(got.MarshalTo(nil)))
	})

	t.Run("freshValue nil returns nil", func(t *testing.T) {
		cached := astjson.MustParse(`{"name":"Alice"}`)
		got := mergeCachedValueForWrite(a, cached, nil)
		assert.Nil(t, got)
	})

	t.Run("cachedValue not object returns freshValue", func(t *testing.T) {
		cached := astjson.MustParse(`[1,2,3]`)
		fresh := astjson.MustParse(`{"name":"Bob"}`)
		got := mergeCachedValueForWrite(a, cached, fresh)
		assert.Equal(t, `{"name":"Bob"}`, string(got.MarshalTo(nil)))
	})

	t.Run("freshValue not object returns freshValue", func(t *testing.T) {
		cached := astjson.MustParse(`{"name":"Alice"}`)
		fresh := astjson.MustParse(`"just a string"`)
		got := mergeCachedValueForWrite(a, cached, fresh)
		assert.Equal(t, `"just a string"`, string(got.MarshalTo(nil)))
	})

	t.Run("both objects merge succeeds with fresh winning on overlap", func(t *testing.T) {
		cached := astjson.MustParse(`{"name":"Alice","email":"alice@old.com"}`)
		fresh := astjson.MustParse(`{"name":"Bob"}`)
		got := mergeCachedValueForWrite(a, cached, fresh)
		result := string(got.MarshalTo(nil))
		// Fresh "name" wins over cached "name", cached "email" is preserved
		assert.Equal(t, `{"name":"Bob","email":"alice@old.com"}`, result)
	})

	t.Run("both objects fresh has new fields merged contains both", func(t *testing.T) {
		cached := astjson.MustParse(`{"id":"1"}`)
		fresh := astjson.MustParse(`{"id":"1","age":30}`)
		got := mergeCachedValueForWrite(a, cached, fresh)
		result := string(got.MarshalTo(nil))
		assert.Equal(t, `{"id":"1","age":30}`, result)
	})
}

// TestMaterializeNullableFieldsAsNull verifies that missing nullable fields are
// set to null while non-nullable and already-present fields are left alone.
func TestMaterializeNullableFieldsAsNull(t *testing.T) {
	a := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
	ctx := NewContext(context.Background())
	l := &Loader{ctx: ctx}

	t.Run("nil entity is no-op", func(t *testing.T) {
		obj := &Object{
			Fields: []*Field{
				{Name: []byte("name"), Value: &String{Nullable: true}},
			},
		}
		// Should not panic
		l.materializeNullableFieldsAsNull(a, nil, obj)
	})

	t.Run("entity missing nullable field gets null", func(t *testing.T) {
		entity, err := astjson.ParseBytesWithArena(a, []byte(`{"id":"1"}`))
		assert.NoError(t, err)
		obj := &Object{
			Fields: []*Field{
				{Name: []byte("id"), Value: &String{Nullable: false}},
				{Name: []byte("email"), Value: &String{Nullable: true}},
			},
		}
		l.materializeNullableFieldsAsNull(a, entity, obj)
		assert.Equal(t, `{"id":"1","email":null}`, string(entity.MarshalTo(nil)))
	})

	t.Run("entity missing non-nullable field is not set", func(t *testing.T) {
		entity, err := astjson.ParseBytesWithArena(a, []byte(`{"id":"1"}`))
		assert.NoError(t, err)
		obj := &Object{
			Fields: []*Field{
				{Name: []byte("id"), Value: &String{Nullable: false}},
				{Name: []byte("name"), Value: &String{Nullable: false}},
			},
		}
		l.materializeNullableFieldsAsNull(a, entity, obj)
		// Non-nullable "name" must NOT be materialized
		assert.Equal(t, `{"id":"1"}`, string(entity.MarshalTo(nil)))
	})

	t.Run("entity has all fields no change", func(t *testing.T) {
		entity, err := astjson.ParseBytesWithArena(a, []byte(`{"id":"1","email":"a@b.com"}`))
		assert.NoError(t, err)
		obj := &Object{
			Fields: []*Field{
				{Name: []byte("id"), Value: &String{Nullable: false}},
				{Name: []byte("email"), Value: &String{Nullable: true}},
			},
		}
		l.materializeNullableFieldsAsNull(a, entity, obj)
		assert.Equal(t, `{"id":"1","email":"a@b.com"}`, string(entity.MarshalTo(nil)))
	})

	t.Run("nested object with missing nullable field is recursively materialized", func(t *testing.T) {
		entity, err := astjson.ParseBytesWithArena(a, []byte(`{"address":{"city":"NYC"}}`))
		assert.NoError(t, err)
		obj := &Object{
			Fields: []*Field{
				{
					Name: []byte("address"),
					Value: &Object{
						Nullable: true,
						Fields: []*Field{
							{Name: []byte("city"), Value: &String{Nullable: false}},
							{Name: []byte("zip"), Value: &String{Nullable: true}},
						},
					},
				},
			},
		}
		l.materializeNullableFieldsAsNull(a, entity, obj)
		assert.Equal(t, `{"address":{"city":"NYC","zip":null}}`, string(entity.MarshalTo(nil)))
	})
}

// TestCacheKeyHasPositiveEntityData verifies edge cases for detecting whether
// a CacheKey carries entity data beyond just the identity key fields.
func TestCacheKeyHasPositiveEntityData(t *testing.T) {
	t.Run("nil CacheKey returns false", func(t *testing.T) {
		assert.Equal(t, false, cacheKeyHasPositiveEntityData(nil))
	})

	t.Run("empty CacheKey no values returns false", func(t *testing.T) {
		ck := &CacheKey{}
		assert.Equal(t, false, cacheKeyHasPositiveEntityData(ck))
	})

	t.Run("key-only payload returns false", func(t *testing.T) {
		// Entity has only __typename and the key field "id" — no extra data
		ck := &CacheKey{
			Item: astjson.MustParse(`{"__typename":"User","id":"1"}`),
			Keys: []string{`prefix:{"__typename":"User","key":{"id":"1"}}`},
		}
		assert.Equal(t, false, cacheKeyHasPositiveEntityData(ck))
	})

	t.Run("payload with extra fields returns true", func(t *testing.T) {
		// Entity has "name" beyond the key fields
		ck := &CacheKey{
			Item: astjson.MustParse(`{"__typename":"User","id":"1","name":"Alice"}`),
			Keys: []string{`prefix:{"__typename":"User","key":{"id":"1"}}`},
		}
		assert.Equal(t, true, cacheKeyHasPositiveEntityData(ck))
	})

	t.Run("FromCache with extra fields returns true", func(t *testing.T) {
		ck := &CacheKey{
			FromCache: astjson.MustParse(`{"__typename":"User","id":"1","email":"a@b.com"}`),
			Keys:      []string{`prefix:{"__typename":"User","key":{"id":"1"}}`},
		}
		assert.Equal(t, true, cacheKeyHasPositiveEntityData(ck))
	})

	t.Run("with EntityMergePath extracts nested entity", func(t *testing.T) {
		// The entity is nested under "user" path; the inner object has extra fields
		ck := &CacheKey{
			Item:            astjson.MustParse(`{"user":{"__typename":"User","id":"1","name":"Alice"}}`),
			Keys:            []string{`prefix:{"__typename":"User","key":{"id":"1"}}`},
			EntityMergePath: []string{"user"},
		}
		assert.Equal(t, true, cacheKeyHasPositiveEntityData(ck))
	})

	t.Run("with EntityMergePath key-only nested entity returns false", func(t *testing.T) {
		ck := &CacheKey{
			Item:            astjson.MustParse(`{"user":{"__typename":"User","id":"1"}}`),
			Keys:            []string{`prefix:{"__typename":"User","key":{"id":"1"}}`},
			EntityMergePath: []string{"user"},
		}
		assert.Equal(t, false, cacheKeyHasPositiveEntityData(ck))
	})
}

// TestHasNonEmptyKey verifies the defensive guard used before issuing L2 Get.
// When extractCacheKeysStrings yields nothing but empty strings (e.g., a template
// missed a required variable), we must skip the L2 round-trip instead of asking
// the backend for entries keyed by "".
func TestHasNonEmptyKey(t *testing.T) {
	assert.Equal(t, false, hasNonEmptyKey(nil))
	assert.Equal(t, false, hasNonEmptyKey([]string{}))
	assert.Equal(t, false, hasNonEmptyKey([]string{""}))
	assert.Equal(t, false, hasNonEmptyKey([]string{"", "", ""}))
	assert.Equal(t, true, hasNonEmptyKey([]string{"", "a"}))
	assert.Equal(t, true, hasNonEmptyKey([]string{"a"}))
	assert.Equal(t, true, hasNonEmptyKey([]string{"a", "b"}))
}

// TestTryL2CacheLoad_AllEmptyKeysSkipsBackend verifies that a CacheKey whose
// Keys slice expands to only empty strings does not reach the L2 backend.
// Without the guard, the Loader would call cache.Get(ctx, []string{""}) — wasted
// round-trip and undefined backend semantics. Instead we short-circuit cleanly:
// skipFetch=false, cacheMustBeUpdated=true, inner cache untouched.
func TestTryL2CacheLoad_AllEmptyKeysSkipsBackend(t *testing.T) {
	inner := &failingCache{} // Get on this would bump getCalls — it must not be called.

	ctx := NewContext(context.Background())
	ctx.ExecutionOptions.Caching.EnableL2Cache = true

	l := &Loader{ctx: ctx}
	res := &result{
		cache:       inner,
		l2CacheKeys: []*CacheKey{{Keys: []string{"", ""}}},
		cacheConfig: FetchCacheConfiguration{CacheName: "default"},
	}

	skip, err := l.tryL2CacheLoad(t.Context(), &FetchInfo{DataSourceName: "users"}, res)
	assert.NoError(t, err)
	assert.Equal(t, false, skip)
	assert.Equal(t, true, res.cacheMustBeUpdated)
	assert.Equal(t, int64(0), inner.getCalls.Load())
}

// TestShouldWriteRequestedKey covers the request-key write decision matrix on the
// fetch path, with particular attention to the case where the response payload
// doesn't carry the entity's @key field — `renderedKey` is "" and the requested
// key (built from request arguments) must still be written. Previously this
// branch returned false, suppressing every cache write for queries that selected
// only non-key fields off a cached entity.
func TestShouldWriteRequestedKey(t *testing.T) {
	requested := `{"__typename":"Venue","key":{"address":{"id":"v1"}}}`
	missing := map[string]struct{}{requested: {}}

	tests := []struct {
		name           string
		cacheSkipFetch bool
		writeback      bool
		requested      string
		rendered       string
		missingKeys    map[string]struct{}
		want           bool
	}{
		{
			name:        "fetch path, key not previously requested → always write",
			requested:   requested,
			rendered:    requested,
			missingKeys: nil,
			want:        true,
		},
		{
			name:        "fetch path, key was missing on read, rendered matches requested → write",
			requested:   requested,
			rendered:    requested,
			missingKeys: missing,
			want:        true,
		},
		{
			name:        "fetch path, key was missing on read, response carries no key field → write requested key (REGRESSION)",
			requested:   requested,
			rendered:    "", // response payload didn't contain the @key field
			missingKeys: missing,
			want:        true,
		},
		{
			name:        "fetch path, key was missing on read, rendered disagrees → suppress (key skew)",
			requested:   requested,
			rendered:    `{"__typename":"Venue","key":{"address":{"id":"different"}}}`,
			missingKeys: missing,
			want:        false,
		},
		{
			name:           "skip-fetch path with writeback flag → write",
			cacheSkipFetch: true,
			writeback:      true,
			requested:      requested,
			rendered:       requested,
			missingKeys:    nil,
			want:           true,
		},
		{
			name:           "skip-fetch path without writeback flag → suppress",
			cacheSkipFetch: true,
			writeback:      false,
			requested:      requested,
			rendered:       requested,
			missingKeys:    nil,
			want:           false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := shouldWriteRequestedKey(tc.cacheSkipFetch, tc.writeback, tc.requested, tc.rendered, tc.missingKeys)
			assert.Equal(t, tc.want, got)
		})
	}
}
