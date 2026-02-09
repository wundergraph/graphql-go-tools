package resolve

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wundergraph/astjson"
	"github.com/wundergraph/go-arena"
)

// TestEntityMergePath tests the EntityMergePath mechanism, which enables cache
// sharing between root field fetches and entity fetches.
//
// Problem: A root field fetch (e.g. Query.user(id:"1234")) returns response-level
// data like {"user":{"id":"1234","username":"Me"}}. An entity fetch for the same
// entity returns entity-level data like {"id":"1234","username":"Me"} (no wrapper).
// When both use the same cache key (derived entity key), the stored format must be
// consistent so either fetch type can read the other's cache entries.
//
// Solution: EntityMergePath records the JSON path (e.g. ["user"]) at which the
// entity data is nested in the root field response. On store, cacheKeysToEntries
// strips the wrapper. On load, tryL2CacheLoad re-wraps the entity data.
func TestEntityMergePath(t *testing.T) {

	// Group 1: prepareCacheKeys — EntityMergePath assignment

	t.Run("prepareCacheKeys", func(t *testing.T) {
		t.Run("root field with EntityKeyMappings single field sets EntityMergePath from field name", func(t *testing.T) {
			ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
			ctx := NewContext(context.Background())
			ctx.ExecutionOptions.Caching.EnableL1Cache = true
			ctx.Variables = astjson.MustParseBytes([]byte(`{"id":"1234"}`))

			loader := &Loader{
				ctx:       ctx,
				jsonArena: ar,
			}

			cfg := FetchCacheConfiguration{
				CacheKeyTemplate: &RootQueryCacheKeyTemplate{
					RootFields: []QueryField{
						{
							Coordinate: GraphCoordinate{TypeName: "Query", FieldName: "user"},
							Args: []FieldArgument{
								{
									Name: "id",
									Variable: &ContextVariable{
										Path:     []string{"id"},
										Renderer: NewPlainVariableRenderer(),
									},
								},
							},
						},
					},
					EntityKeyMappings: []EntityKeyMappingConfig{
						{
							EntityTypeName: "User",
							FieldMappings: []EntityFieldMappingConfig{
								{EntityKeyField: "id", ArgumentPath: []string{"id"}},
							},
						},
					},
				},
			}

			item := astjson.MustParseBytes([]byte(`{"user":{"id":"1234","username":"Me"}}`))
			inputItems := []*astjson.Value{item}
			res := &result{}

			isEntity, err := loader.prepareCacheKeys(&FetchInfo{}, cfg, inputItems, res)
			require.NoError(t, err)
			assert.False(t, isEntity)
			require.Equal(t, 1, len(res.l1CacheKeys))
			assert.Equal(t, []string{"user"}, res.l1CacheKeys[0].EntityMergePath)
		})

		t.Run("root field with EntityKeyMappings sets EntityMergePath from explicit MergePath", func(t *testing.T) {
			ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
			ctx := NewContext(context.Background())
			ctx.ExecutionOptions.Caching.EnableL1Cache = true
			ctx.Variables = astjson.MustParseBytes([]byte(`{"id":"1234"}`))

			loader := &Loader{
				ctx:       ctx,
				jsonArena: ar,
			}

			cfg := FetchCacheConfiguration{
				CacheKeyTemplate: &RootQueryCacheKeyTemplate{
					RootFields: []QueryField{
						{
							Coordinate: GraphCoordinate{TypeName: "Query", FieldName: "user"},
							Args: []FieldArgument{
								{
									Name: "id",
									Variable: &ContextVariable{
										Path:     []string{"id"},
										Renderer: NewPlainVariableRenderer(),
									},
								},
							},
						},
					},
					EntityKeyMappings: []EntityKeyMappingConfig{
						{
							EntityTypeName: "User",
							FieldMappings: []EntityFieldMappingConfig{
								{EntityKeyField: "id", ArgumentPath: []string{"id"}},
							},
						},
					},
				},
			}

			item := astjson.MustParseBytes([]byte(`{"data":{"user":{"id":"1234"}}}`))
			inputItems := []*astjson.Value{item}
			res := &result{
				postProcessing: PostProcessingConfiguration{
					MergePath: []string{"data", "user"},
				},
			}

			isEntity, err := loader.prepareCacheKeys(&FetchInfo{}, cfg, inputItems, res)
			require.NoError(t, err)
			assert.False(t, isEntity)
			require.Equal(t, 1, len(res.l1CacheKeys))
			assert.Equal(t, []string{"data", "user"}, res.l1CacheKeys[0].EntityMergePath)
		})

		t.Run("root field without EntityKeyMappings does not set EntityMergePath", func(t *testing.T) {
			ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
			ctx := NewContext(context.Background())
			ctx.ExecutionOptions.Caching.EnableL1Cache = true
			ctx.Variables = astjson.MustParseBytes([]byte(`{"id":"1234"}`))

			loader := &Loader{
				ctx:       ctx,
				jsonArena: ar,
			}

			cfg := FetchCacheConfiguration{
				CacheKeyTemplate: &RootQueryCacheKeyTemplate{
					RootFields: []QueryField{
						{
							Coordinate: GraphCoordinate{TypeName: "Query", FieldName: "user"},
							Args: []FieldArgument{
								{
									Name: "id",
									Variable: &ContextVariable{
										Path:     []string{"id"},
										Renderer: NewPlainVariableRenderer(),
									},
								},
							},
						},
					},
					// No EntityKeyMappings
				},
			}

			item := astjson.MustParseBytes([]byte(`{"user":{"id":"1234"}}`))
			inputItems := []*astjson.Value{item}
			res := &result{}

			_, err := loader.prepareCacheKeys(&FetchInfo{}, cfg, inputItems, res)
			require.NoError(t, err)
			require.Equal(t, 1, len(res.l1CacheKeys))
			assert.Equal(t, []string(nil), res.l1CacheKeys[0].EntityMergePath)
		})

		t.Run("entity fetch template does not set EntityMergePath", func(t *testing.T) {
			ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
			ctx := NewContext(context.Background())
			ctx.ExecutionOptions.Caching.EnableL1Cache = true

			loader := &Loader{
				ctx:       ctx,
				jsonArena: ar,
			}

			cfg := FetchCacheConfiguration{
				CacheKeyTemplate: &EntityQueryCacheKeyTemplate{
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
				},
			}

			item := astjson.MustParseBytes([]byte(`{"__typename":"User","id":"1234"}`))
			inputItems := []*astjson.Value{item}
			res := &result{}

			isEntity, err := loader.prepareCacheKeys(&FetchInfo{}, cfg, inputItems, res)
			require.NoError(t, err)
			assert.True(t, isEntity)
			require.Equal(t, 1, len(res.l1CacheKeys))
			assert.Equal(t, []string(nil), res.l1CacheKeys[0].EntityMergePath)
		})

		// When there are multiple root fields, EntityMergePath cannot be derived from a single
		// field name (ambiguous), so it falls back to res.postProcessing.MergePath if available.
		t.Run("multiple root fields without MergePath does not set EntityMergePath", func(t *testing.T) {
			ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
			ctx := NewContext(context.Background())
			ctx.ExecutionOptions.Caching.EnableL1Cache = true
			ctx.Variables = astjson.MustParseBytes([]byte(`{"id":"1234"}`))

			loader := &Loader{
				ctx:       ctx,
				jsonArena: ar,
			}

			cfg := FetchCacheConfiguration{
				CacheKeyTemplate: &RootQueryCacheKeyTemplate{
					RootFields: []QueryField{
						{
							Coordinate: GraphCoordinate{TypeName: "Query", FieldName: "user"},
							Args: []FieldArgument{
								{
									Name: "id",
									Variable: &ContextVariable{
										Path:     []string{"id"},
										Renderer: NewPlainVariableRenderer(),
									},
								},
							},
						},
						{
							Coordinate: GraphCoordinate{TypeName: "Query", FieldName: "account"},
							Args: []FieldArgument{
								{
									Name: "id",
									Variable: &ContextVariable{
										Path:     []string{"id"},
										Renderer: NewPlainVariableRenderer(),
									},
								},
							},
						},
					},
					EntityKeyMappings: []EntityKeyMappingConfig{
						{
							EntityTypeName: "User",
							FieldMappings: []EntityFieldMappingConfig{
								{EntityKeyField: "id", ArgumentPath: []string{"id"}},
							},
						},
					},
				},
			}

			item := astjson.MustParseBytes([]byte(`{"user":{"id":"1234"}}`))
			inputItems := []*astjson.Value{item}
			res := &result{}

			_, err := loader.prepareCacheKeys(&FetchInfo{}, cfg, inputItems, res)
			require.NoError(t, err)
			require.Equal(t, 1, len(res.l1CacheKeys))
			assert.Equal(t, []string(nil), res.l1CacheKeys[0].EntityMergePath)
		})

		t.Run("multiple root fields with MergePath sets EntityMergePath", func(t *testing.T) {
			ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
			ctx := NewContext(context.Background())
			ctx.ExecutionOptions.Caching.EnableL1Cache = true
			ctx.Variables = astjson.MustParseBytes([]byte(`{"id":"1234"}`))

			loader := &Loader{
				ctx:       ctx,
				jsonArena: ar,
			}

			cfg := FetchCacheConfiguration{
				CacheKeyTemplate: &RootQueryCacheKeyTemplate{
					RootFields: []QueryField{
						{
							Coordinate: GraphCoordinate{TypeName: "Query", FieldName: "user"},
							Args: []FieldArgument{
								{
									Name: "id",
									Variable: &ContextVariable{
										Path:     []string{"id"},
										Renderer: NewPlainVariableRenderer(),
									},
								},
							},
						},
						{
							Coordinate: GraphCoordinate{TypeName: "Query", FieldName: "account"},
							Args: []FieldArgument{
								{
									Name: "id",
									Variable: &ContextVariable{
										Path:     []string{"id"},
										Renderer: NewPlainVariableRenderer(),
									},
								},
							},
						},
					},
					EntityKeyMappings: []EntityKeyMappingConfig{
						{
							EntityTypeName: "User",
							FieldMappings: []EntityFieldMappingConfig{
								{EntityKeyField: "id", ArgumentPath: []string{"id"}},
							},
						},
					},
				},
			}

			item := astjson.MustParseBytes([]byte(`{"user":{"id":"1234"}}`))
			inputItems := []*astjson.Value{item}
			res := &result{
				postProcessing: PostProcessingConfiguration{
					MergePath: []string{"user"},
				},
			}

			_, err := loader.prepareCacheKeys(&FetchInfo{}, cfg, inputItems, res)
			require.NoError(t, err)
			require.Equal(t, 1, len(res.l1CacheKeys))
			assert.Equal(t, []string{"user"}, res.l1CacheKeys[0].EntityMergePath)
		})
	})

	// Group 2: cacheKeysToEntries — Extract entity data for storage

	t.Run("cacheKeysToEntries", func(t *testing.T) {
		t.Run("EntityMergePath set extracts entity data only", func(t *testing.T) {
			ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
			loader := &Loader{
				jsonArena: ar,
			}

			item := astjson.MustParseBytes([]byte(`{"user":{"id":"1234","username":"Me"}}`))
			cacheKeys := []*CacheKey{
				{
					Item:            item,
					Keys:            []string{`{"__typename":"User","key":{"id":"1234"}}`},
					EntityMergePath: []string{"user"},
				},
			}

			entries, err := loader.cacheKeysToEntries(ar, cacheKeys)
			require.NoError(t, err)
			require.Equal(t, 1, len(entries))
			assert.Equal(t, `{"__typename":"User","key":{"id":"1234"}}`, entries[0].Key)
			assert.Equal(t, `{"id":"1234","username":"Me"}`, string(entries[0].Value))
		})

		t.Run("EntityMergePath not set stores full response", func(t *testing.T) {
			ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
			loader := &Loader{
				jsonArena: ar,
			}

			item := astjson.MustParseBytes([]byte(`{"user":{"id":"1234","username":"Me"}}`))
			cacheKeys := []*CacheKey{
				{
					Item: item,
					Keys: []string{`root:user:1234`},
				},
			}

			entries, err := loader.cacheKeysToEntries(ar, cacheKeys)
			require.NoError(t, err)
			require.Equal(t, 1, len(entries))
			assert.Equal(t, `root:user:1234`, entries[0].Key)
			assert.Equal(t, `{"user":{"id":"1234","username":"Me"}}`, string(entries[0].Value))
		})

		t.Run("EntityMergePath set but data not found at path stores full response", func(t *testing.T) {
			ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
			loader := &Loader{
				jsonArena: ar,
			}

			item := astjson.MustParseBytes([]byte(`{"user":{"id":"1234"}}`))
			cacheKeys := []*CacheKey{
				{
					Item:            item,
					Keys:            []string{`key1`},
					EntityMergePath: []string{"nonexistent"},
				},
			}

			entries, err := loader.cacheKeysToEntries(ar, cacheKeys)
			require.NoError(t, err)
			require.Equal(t, 1, len(entries))
			assert.Equal(t, `{"user":{"id":"1234"}}`, string(entries[0].Value))
		})

		t.Run("multi-segment EntityMergePath extracts at nested path", func(t *testing.T) {
			ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
			loader := &Loader{
				jsonArena: ar,
			}

			item := astjson.MustParseBytes([]byte(`{"data":{"user":{"id":"1234"}}}`))
			cacheKeys := []*CacheKey{
				{
					Item:            item,
					Keys:            []string{`key1`},
					EntityMergePath: []string{"data", "user"},
				},
			}

			entries, err := loader.cacheKeysToEntries(ar, cacheKeys)
			require.NoError(t, err)
			require.Equal(t, 1, len(entries))
			assert.Equal(t, `{"id":"1234"}`, string(entries[0].Value))
		})
	})

	// Group 3: tryL2CacheLoad — Wrap cached entity data on load

	t.Run("tryL2CacheLoad wrapping", func(t *testing.T) {
		t.Run("EntityMergePath set and cache hit wraps entity data", func(t *testing.T) {
			ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
			cache := NewFakeLoaderCache()

			ctx := NewContext(context.Background())
			ctx.ExecutionOptions.Caching.EnableL1Cache = true
			ctx.ExecutionOptions.Caching.EnableL2Cache = true

			loader := &Loader{
				ctx:       ctx,
				jsonArena: ar,
				caches:    map[string]LoaderCache{"default": cache},
			}

			// Pre-populate cache with entity-level data (as stored by cacheKeysToEntries with EntityMergePath)
			cacheKey := `{"__typename":"User","key":{"id":"1234"}}`
			err := cache.Set(context.Background(), []*CacheEntry{
				{Key: cacheKey, Value: []byte(`{"id":"1234","username":"Me"}`)},
			}, 30*time.Second)
			require.NoError(t, err)

			// Set up result with L2 cache keys that have EntityMergePath
			res := &result{
				cache: cache,
				l2CacheKeys: []*CacheKey{
					{
						Keys:            []string{cacheKey},
						EntityMergePath: []string{"user"},
					},
				},
				l1CacheKeys: []*CacheKey{
					{
						Keys:            []string{cacheKey},
						EntityMergePath: []string{"user"},
					},
				},
			}

			// Call tryL2CacheLoad
			// ProvidesData must match the wrapped response shape for validation to pass
			skipFetch, err := loader.tryL2CacheLoad(context.Background(), &FetchInfo{
				ProvidesData: &Object{
					Fields: []*Field{
						{Name: []byte("user"), Value: &Object{
							Path: []string{"user"},
							Fields: []*Field{
								{Name: []byte("id"), Value: &Scalar{Path: []string{"id"}}},
								{Name: []byte("username"), Value: &Scalar{Path: []string{"username"}}},
							},
						}},
					},
				},
			}, res)
			require.NoError(t, err)
			assert.True(t, skipFetch, "all items cached, should skip fetch")

			// Verify the L2 cache key's FromCache was wrapped
			require.NotNil(t, res.l2CacheKeys[0].FromCache)
			wrapped := string(res.l2CacheKeys[0].FromCache.MarshalTo(nil))
			assert.Equal(t, `{"user":{"id":"1234","username":"Me"}}`, wrapped)

			// Verify L1 cache key also received the wrapped value (L2-to-L1 copy)
			require.NotNil(t, res.l1CacheKeys[0].FromCache)
			l1Wrapped := string(res.l1CacheKeys[0].FromCache.MarshalTo(nil))
			assert.Equal(t, `{"user":{"id":"1234","username":"Me"}}`, l1Wrapped)

			// Verify L2 stats: 1 hit, 0 misses
			stats := ctx.GetCacheStats()
			assert.Equal(t, int64(1), stats.L2Hits)
			assert.Equal(t, int64(0), stats.L2Misses)
		})

		t.Run("EntityMergePath not set and cache hit returns data as-is", func(t *testing.T) {
			ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
			cache := NewFakeLoaderCache()

			ctx := NewContext(context.Background())
			ctx.ExecutionOptions.Caching.EnableL1Cache = true
			ctx.ExecutionOptions.Caching.EnableL2Cache = true

			loader := &Loader{
				ctx:       ctx,
				jsonArena: ar,
				caches:    map[string]LoaderCache{"default": cache},
			}

			cacheKey := `root:user:1234`
			err := cache.Set(context.Background(), []*CacheEntry{
				{Key: cacheKey, Value: []byte(`{"user":{"id":"1234","username":"Me"}}`)},
			}, 30*time.Second)
			require.NoError(t, err)

			res := &result{
				cache: cache,
				l2CacheKeys: []*CacheKey{
					{
						Keys: []string{cacheKey},
						// No EntityMergePath
					},
				},
				l1CacheKeys: []*CacheKey{
					{
						Keys: []string{cacheKey},
					},
				},
			}

			skipFetch, err := loader.tryL2CacheLoad(context.Background(), &FetchInfo{
				ProvidesData: &Object{
					Fields: []*Field{
						{Name: []byte("user"), Value: &Object{
							Path: []string{"user"},
							Fields: []*Field{
								{Name: []byte("id"), Value: &Scalar{Path: []string{"id"}}},
								{Name: []byte("username"), Value: &Scalar{Path: []string{"username"}}},
							},
						}},
					},
				},
			}, res)
			require.NoError(t, err)
			assert.True(t, skipFetch, "all items cached, should skip fetch")

			require.NotNil(t, res.l2CacheKeys[0].FromCache)
			unwrapped := string(res.l2CacheKeys[0].FromCache.MarshalTo(nil))
			assert.Equal(t, `{"user":{"id":"1234","username":"Me"}}`, unwrapped)

			// Verify L1 cache key also received the value (L2-to-L1 copy)
			require.NotNil(t, res.l1CacheKeys[0].FromCache)
			l1Value := string(res.l1CacheKeys[0].FromCache.MarshalTo(nil))
			assert.Equal(t, `{"user":{"id":"1234","username":"Me"}}`, l1Value)

			// Verify L2 stats: 1 hit, 0 misses
			stats := ctx.GetCacheStats()
			assert.Equal(t, int64(1), stats.L2Hits)
			assert.Equal(t, int64(0), stats.L2Misses)
		})

		t.Run("EntityMergePath set but cache miss stays nil", func(t *testing.T) {
			ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
			cache := NewFakeLoaderCache()

			ctx := NewContext(context.Background())
			ctx.ExecutionOptions.Caching.EnableL1Cache = true
			ctx.ExecutionOptions.Caching.EnableL2Cache = true

			loader := &Loader{
				ctx:       ctx,
				jsonArena: ar,
				caches:    map[string]LoaderCache{"default": cache},
			}

			// Don't populate cache — miss

			res := &result{
				cache: cache,
				l2CacheKeys: []*CacheKey{
					{
						Keys:            []string{`{"__typename":"User","key":{"id":"9999"}}`},
						EntityMergePath: []string{"user"},
					},
				},
				l1CacheKeys: []*CacheKey{
					{
						Keys:            []string{`{"__typename":"User","key":{"id":"9999"}}`},
						EntityMergePath: []string{"user"},
					},
				},
			}

			skipFetch, err := loader.tryL2CacheLoad(context.Background(), &FetchInfo{
				ProvidesData: &Object{
					Fields: []*Field{
						{Name: []byte("id"), Value: &Scalar{Path: []string{"id"}}},
					},
				},
			}, res)
			require.NoError(t, err)
			assert.False(t, skipFetch, "cache miss, should not skip fetch")

			assert.Nil(t, res.l2CacheKeys[0].FromCache)

			// Verify L2 stats: 0 hits, 1 miss
			stats := ctx.GetCacheStats()
			assert.Equal(t, int64(0), stats.L2Hits)
			assert.Equal(t, int64(1), stats.L2Misses)
		})

		t.Run("multi-segment EntityMergePath wraps at each level", func(t *testing.T) {
			ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
			cache := NewFakeLoaderCache()

			ctx := NewContext(context.Background())
			ctx.ExecutionOptions.Caching.EnableL1Cache = true
			ctx.ExecutionOptions.Caching.EnableL2Cache = true

			loader := &Loader{
				ctx:       ctx,
				jsonArena: ar,
				caches:    map[string]LoaderCache{"default": cache},
			}

			cacheKey := `key1`
			err := cache.Set(context.Background(), []*CacheEntry{
				{Key: cacheKey, Value: []byte(`{"id":"1234"}`)},
			}, 30*time.Second)
			require.NoError(t, err)

			res := &result{
				cache: cache,
				l2CacheKeys: []*CacheKey{
					{
						Keys:            []string{cacheKey},
						EntityMergePath: []string{"data", "user"},
					},
				},
				l1CacheKeys: []*CacheKey{
					{
						Keys:            []string{cacheKey},
						EntityMergePath: []string{"data", "user"},
					},
				},
			}

			skipFetch, err := loader.tryL2CacheLoad(context.Background(), &FetchInfo{
				ProvidesData: &Object{
					Fields: []*Field{
						{Name: []byte("data"), Value: &Object{
							Path: []string{"data"},
							Fields: []*Field{
								{Name: []byte("user"), Value: &Object{
									Path: []string{"user"},
									Fields: []*Field{
										{Name: []byte("id"), Value: &Scalar{Path: []string{"id"}}},
									},
								}},
							},
						}},
					},
				},
			}, res)
			require.NoError(t, err)
			assert.True(t, skipFetch, "all items cached, should skip fetch")

			require.NotNil(t, res.l2CacheKeys[0].FromCache)
			wrapped := string(res.l2CacheKeys[0].FromCache.MarshalTo(nil))
			assert.Equal(t, `{"data":{"user":{"id":"1234"}}}`, wrapped)

			// Verify L1 cache key also received the wrapped value (L2-to-L1 copy)
			require.NotNil(t, res.l1CacheKeys[0].FromCache)
			l1Wrapped := string(res.l1CacheKeys[0].FromCache.MarshalTo(nil))
			assert.Equal(t, `{"data":{"user":{"id":"1234"}}}`, l1Wrapped)

			// Verify L2 stats: 1 hit, 0 misses
			stats := ctx.GetCacheStats()
			assert.Equal(t, int64(1), stats.L2Hits)
			assert.Equal(t, int64(0), stats.L2Misses)
		})
	})

	// Group 4: Roundtrip consistency

	t.Run("roundtrip", func(t *testing.T) {
		t.Run("store then load via EntityMergePath produces original data", func(t *testing.T) {
			ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
			cache := NewFakeLoaderCache()

			ctx := NewContext(context.Background())
			ctx.ExecutionOptions.Caching.EnableL1Cache = true
			ctx.ExecutionOptions.Caching.EnableL2Cache = true

			loader := &Loader{
				ctx:       ctx,
				jsonArena: ar,
				caches:    map[string]LoaderCache{"default": cache},
			}

			originalJSON := `{"user":{"id":"1234","username":"Me"}}`
			item := astjson.MustParseBytes([]byte(originalJSON))

			// Step 1: Create cache keys with EntityMergePath and convert to entries (store)
			cacheKey := `{"__typename":"User","key":{"id":"1234"}}`
			storeKeys := []*CacheKey{
				{
					Item:            item,
					Keys:            []string{cacheKey},
					EntityMergePath: []string{"user"},
				},
			}

			entries, err := loader.cacheKeysToEntries(ar, storeKeys)
			require.NoError(t, err)
			require.Equal(t, 1, len(entries))
			// Verify it stored entity-level data
			assert.Equal(t, `{"id":"1234","username":"Me"}`, string(entries[0].Value))

			// Step 2: Store in L2 cache
			err = cache.Set(context.Background(), entries, 30*time.Second)
			require.NoError(t, err)

			// Step 3: Load from L2 cache with EntityMergePath wrapping
			loadRes := &result{
				cache: cache,
				l2CacheKeys: []*CacheKey{
					{
						Keys:            []string{cacheKey},
						EntityMergePath: []string{"user"},
					},
				},
				l1CacheKeys: []*CacheKey{
					{
						Keys:            []string{cacheKey},
						EntityMergePath: []string{"user"},
					},
				},
			}

			_, err = loader.tryL2CacheLoad(context.Background(), &FetchInfo{
				ProvidesData: &Object{
					Fields: []*Field{
						{Name: []byte("id"), Value: &Scalar{Path: []string{"id"}}},
						{Name: []byte("username"), Value: &Scalar{Path: []string{"username"}}},
					},
				},
			}, loadRes)
			require.NoError(t, err)

			// Verify roundtrip: loaded data should match original
			require.NotNil(t, loadRes.l2CacheKeys[0].FromCache)
			loaded := string(loadRes.l2CacheKeys[0].FromCache.MarshalTo(nil))
			assert.Equal(t, originalJSON, loaded)
		})

		t.Run("root field store is loadable by entity fetch using same derived key", func(t *testing.T) {
			ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
			cache := NewFakeLoaderCache()

			ctx := NewContext(context.Background())
			ctx.ExecutionOptions.Caching.EnableL1Cache = true
			ctx.ExecutionOptions.Caching.EnableL2Cache = true
			ctx.Variables = astjson.MustParseBytes([]byte(`{"id":"1234"}`))

			loader := &Loader{
				ctx:       ctx,
				jsonArena: ar,
				caches:    map[string]LoaderCache{"default": cache},
			}

			// Step 1: Root field fetch produces response with wrapper
			rootItem := astjson.MustParseBytes([]byte(`{"user":{"__typename":"User","id":"1234","username":"Me"}}`))

			// prepareCacheKeys for root field with EntityKeyMappings
			rootCfg := FetchCacheConfiguration{
				Enabled:   true,
				CacheName: "default",
				TTL:       30 * time.Second,
				CacheKeyTemplate: &RootQueryCacheKeyTemplate{
					RootFields: []QueryField{
						{
							Coordinate: GraphCoordinate{TypeName: "Query", FieldName: "user"},
							Args: []FieldArgument{
								{
									Name: "id",
									Variable: &ContextVariable{
										Path:     []string{"id"},
										Renderer: NewPlainVariableRenderer(),
									},
								},
							},
						},
					},
					EntityKeyMappings: []EntityKeyMappingConfig{
						{
							EntityTypeName: "User",
							FieldMappings: []EntityFieldMappingConfig{
								{EntityKeyField: "id", ArgumentPath: []string{"id"}},
							},
						},
					},
				},
			}

			rootRes := &result{}
			_, err := loader.prepareCacheKeys(&FetchInfo{}, rootCfg, []*astjson.Value{rootItem}, rootRes)
			require.NoError(t, err)
			require.Equal(t, 1, len(rootRes.l1CacheKeys))
			assert.Equal(t, []string{"user"}, rootRes.l1CacheKeys[0].EntityMergePath)

			// Store: cacheKeysToEntries should extract entity-level data
			entries, err := loader.cacheKeysToEntries(ar, rootRes.l1CacheKeys)
			require.NoError(t, err)
			require.Equal(t, 1, len(entries))
			// Entity-level data (stripped of the "user" wrapper)
			assert.Equal(t, `{"__typename":"User","id":"1234","username":"Me"}`, string(entries[0].Value))

			// Store in L2
			err = cache.Set(context.Background(), entries, 30*time.Second)
			require.NoError(t, err)

			// Step 2: Entity fetch tries to load from cache using same key format
			// Entity fetches use EntityQueryCacheKeyTemplate which produces the same key
			entityItem := astjson.MustParseBytes([]byte(`{"__typename":"User","id":"1234"}`))
			entityCfg := FetchCacheConfiguration{
				Enabled:   true,
				CacheName: "default",
				TTL:       30 * time.Second,
				CacheKeyTemplate: &EntityQueryCacheKeyTemplate{
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
				},
			}

			entityRes := &result{}
			isEntity, err := loader.prepareCacheKeys(&FetchInfo{}, entityCfg, []*astjson.Value{entityItem}, entityRes)
			require.NoError(t, err)
			assert.True(t, isEntity)
			require.Equal(t, 1, len(entityRes.l1CacheKeys))
			// Entity fetch should NOT have EntityMergePath
			assert.Equal(t, []string(nil), entityRes.l1CacheKeys[0].EntityMergePath)

			// Verify key format matches between root (derived entity key) and entity fetch
			rootKeyStr := rootRes.l1CacheKeys[0].Keys[0]
			entityKeyStr := entityRes.l1CacheKeys[0].Keys[0]
			assert.Equal(t, rootKeyStr, entityKeyStr, "root field derived entity key should match entity fetch key")

			// The entity fetch can now find the cache entry stored by the root field
			cacheEntries, err := cache.Get(context.Background(), []string{entityKeyStr})
			require.NoError(t, err)
			require.Equal(t, 1, len(cacheEntries))
			require.NotNil(t, cacheEntries[0])
			assert.Equal(t, `{"__typename":"User","id":"1234","username":"Me"}`, string(cacheEntries[0].Value))
		})
	})
}
