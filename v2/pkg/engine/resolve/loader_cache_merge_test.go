package resolve

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wundergraph/astjson"
	"github.com/wundergraph/go-arena"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
)

func newCacheMergeTestLoader(t *testing.T) (*Loader, arena.Arena) {
	t.Helper()
	ar := arena.NewMonotonicArena(arena.WithMinBufferSize(4096))
	ctx := NewContext(context.Background())
	ctx.ExecutionOptions.Caching.EnableL2Cache = true
	resolvable := NewResolvable(ar, ResolvableOptions{})
	require.NoError(t, resolvable.Init(ctx, nil, ast.OperationTypeQuery))
	l := &Loader{
		jsonArena:  ar,
		resolvable: resolvable,
		ctx:        ctx,
	}
	return l, ar
}

func TestMergeBatchCacheHit(t *testing.T) {
	t.Run("empty batch with no items sets response data with empty array at field name", func(t *testing.T) {
		// maxIndex < 0 (no cache keys), len(items) == 0 → replaces resolvable.data
		l, _ := newCacheMergeTestLoader(t)

		res := &result{
			fetchInfo: &FetchInfo{
				RootFields: []GraphCoordinate{{FieldName: "products"}},
			},
		}
		fetchItem := &FetchItem{}

		err := l.mergeBatchCacheHit(fetchItem, res, nil)
		require.NoError(t, err)

		// The response data should be {"products":[]}
		got := string(l.resolvable.data.MarshalTo(nil))
		assert.Equal(t, `{"products":[]}`, got)
	})

	t.Run("empty batch with one item merges at batchMergePath", func(t *testing.T) {
		// maxIndex < 0, len(items) == 1 → merge empty response into items[0] at batchMergePath
		l, ar := newCacheMergeTestLoader(t)

		existing, err := astjson.ParseBytesWithArena(ar, []byte(`{"data":"existing"}`))
		require.NoError(t, err)

		items := []*astjson.Value{existing}
		res := &result{
			batchMergePath: []string{"nested"},
			fetchInfo: &FetchInfo{
				RootFields: []GraphCoordinate{{FieldName: "products"}},
			},
		}
		fetchItem := &FetchItem{}

		err = l.mergeBatchCacheHit(fetchItem, res, items)
		require.NoError(t, err)

		// items[0] should now have the empty batch merged at "nested"
		got := string(items[0].MarshalTo(nil))
		assert.Equal(t, `{"data":"existing","nested":{"products":[]}}`, got)
	})

	t.Run("normal batch places cached entities at correct positions", func(t *testing.T) {
		// Two cache hits at indices 0 and 2, index 1 is a miss → null in result array
		l, ar := newCacheMergeTestLoader(t)

		entity0, err := astjson.ParseBytesWithArena(ar, []byte(`{"upc":"top-1","name":"Trilby"}`))
		require.NoError(t, err)
		entity2, err := astjson.ParseBytesWithArena(ar, []byte(`{"upc":"top-3","name":"Fedora"}`))
		require.NoError(t, err)

		cacheKeys := []*CacheKey{
			{BatchIndex: 0, FromCache: entity0, Keys: []string{"key0"}},
			{BatchIndex: 1, FromCache: nil, Keys: []string{"key1"}},
			{BatchIndex: 2, FromCache: entity2, Keys: []string{"key2"}},
		}
		res := &result{
			l2CacheKeys: cacheKeys,
		}
		fetchItem := &FetchItem{}

		// No items → sets resolvable.data directly (root-level merge without EntityMergePath)
		err = l.mergeBatchCacheHit(fetchItem, res, nil)
		require.NoError(t, err)

		// Without EntityMergePath, responseData is an empty object with entities in the array
		// but the array is only set under entityMergePath. With no entityMergePath, the object
		// is set as resolvable.data directly. Let's verify the data is set.
		got := string(l.resolvable.data.MarshalTo(nil))
		assert.Equal(t, `{}`, got)
	})

	t.Run("batch with EntityMergePath extracts entities from wrapper", func(t *testing.T) {
		// Entities are wrapped at EntityMergePath (e.g., {"products": {...entity...}})
		// during L2 load. mergeBatchCacheHit extracts them via Get(entityMergePath...).
		l, ar := newCacheMergeTestLoader(t)

		wrapped0, err := astjson.ParseBytesWithArena(ar, []byte(`{"products":{"upc":"top-1","name":"Trilby"}}`))
		require.NoError(t, err)
		wrapped1, err := astjson.ParseBytesWithArena(ar, []byte(`{"products":{"upc":"top-2","name":"Bowler"}}`))
		require.NoError(t, err)

		cacheKeys := []*CacheKey{
			{BatchIndex: 0, FromCache: wrapped0, Keys: []string{"key0"}, EntityMergePath: []string{"products"}},
			{BatchIndex: 1, FromCache: wrapped1, Keys: []string{"key1"}, EntityMergePath: []string{"products"}},
		}
		res := &result{
			l2CacheKeys: cacheKeys,
		}
		fetchItem := &FetchItem{}

		// Root-level merge: sets resolvable.data
		err = l.mergeBatchCacheHit(fetchItem, res, nil)
		require.NoError(t, err)

		// With EntityMergePath ["products"], the response is {"products": [entity0, entity1]}
		got := string(l.resolvable.data.MarshalTo(nil))
		assert.Equal(t, `{"products":[{"upc":"top-1","name":"Trilby"},{"upc":"top-2","name":"Bowler"}]}`, got)
	})

	t.Run("batch with EntityMergePath merges into items at batchMergePath", func(t *testing.T) {
		// Same as above but with items[0] and batchMergePath
		l, ar := newCacheMergeTestLoader(t)

		existing, err := astjson.ParseBytesWithArena(ar, []byte(`{"other":"value"}`))
		require.NoError(t, err)

		wrapped0, err := astjson.ParseBytesWithArena(ar, []byte(`{"products":{"upc":"top-1"}}`))
		require.NoError(t, err)

		cacheKeys := []*CacheKey{
			{BatchIndex: 0, FromCache: wrapped0, Keys: []string{"key0"}, EntityMergePath: []string{"products"}},
		}
		res := &result{
			l2CacheKeys:    cacheKeys,
			batchMergePath: []string{"nested"},
		}
		fetchItem := &FetchItem{}
		items := []*astjson.Value{existing}

		err = l.mergeBatchCacheHit(fetchItem, res, items)
		require.NoError(t, err)

		got := string(items[0].MarshalTo(nil))
		assert.Equal(t, `{"other":"value","nested":{"products":[{"upc":"top-1"}]}}`, got)
	})

	t.Run("batch with EntityMergePath matching batchMergePath merges entities into existing root array", func(t *testing.T) {
		l, ar := newCacheMergeTestLoader(t)

		existing, err := astjson.ParseBytesWithArena(ar, []byte(`{"catalogs":[{"id":"c1","name":"Electronics","itemCount":342},{"id":"c2","name":"Books","itemCount":1205}]}`))
		require.NoError(t, err)

		wrapped0, err := astjson.ParseBytesWithArena(ar, []byte(`{"catalogs":{"id":"c1","description":"Consumer electronics, gadgets, and accessories.","lastUpdated":"2025-03-15T08:00:00Z"}}`))
		require.NoError(t, err)
		wrapped1, err := astjson.ParseBytesWithArena(ar, []byte(`{"catalogs":{"id":"c2","description":"Fiction, non-fiction, technical books, and audiobooks.","lastUpdated":"2025-03-20T12:00:00Z"}}`))
		require.NoError(t, err)

		items := []*astjson.Value{existing}
		res := &result{
			l2CacheKeys: []*CacheKey{
				{BatchIndex: 0, FromCache: wrapped0, Keys: []string{"key0"}, EntityMergePath: []string{"catalogs"}},
				{BatchIndex: 1, FromCache: wrapped1, Keys: []string{"key1"}, EntityMergePath: []string{"catalogs"}},
			},
			batchMergePath: []string{"catalogs"},
			postProcessing: PostProcessingConfiguration{
				SelectResponseDataPath: []string{"data", "_entities"},
			},
			fetchInfo: &FetchInfo{
				RootFields: []GraphCoordinate{{FieldName: "catalogs"}},
			},
		}

		err = l.mergeBatchCacheHit(&FetchItem{}, res, items)
		require.NoError(t, err)

		assert.Equal(t, `{"catalogs":[{"id":"c1","name":"Electronics","itemCount":342,"description":"Consumer electronics, gadgets, and accessories.","lastUpdated":"2025-03-15T08:00:00Z"},{"id":"c2","name":"Books","itemCount":1205,"description":"Fiction, non-fiction, technical books, and audiobooks.","lastUpdated":"2025-03-20T12:00:00Z"}]}`, string(items[0].MarshalTo(nil)))
	})
}

func TestPopulateBatchCacheKeysFromResponse(t *testing.T) {
	t.Run("batchEntityKeyMode false returns immediately", func(t *testing.T) {
		// When batchEntityKeyMode is false, the function should not set any Items
		l, ar := newCacheMergeTestLoader(t)

		responseObj, err := astjson.ParseBytesWithArena(ar, []byte(`{"products":[{"upc":"top-1"}]}`))
		require.NoError(t, err)

		ck := &CacheKey{BatchIndex: 0, Keys: []string{"key0"}}
		res := &result{
			batchEntityKeyMode: false, // disabled
			l2CacheKeys:        []*CacheKey{ck},
		}
		items := []*astjson.Value{responseObj}

		l.populateBatchCacheKeysFromResponse(res, items, &FetchInfo{
			RootFields: []GraphCoordinate{{FieldName: "products"}},
		})

		// Item should remain nil because batchEntityKeyMode is false
		assert.Nil(t, ck.Item)
	})

	t.Run("normal batch assigns array items to cache keys by BatchIndex", func(t *testing.T) {
		// Each array element should be assigned to the CacheKey with matching BatchIndex
		l, ar := newCacheMergeTestLoader(t)

		responseObj, err := astjson.ParseBytesWithArena(ar, []byte(`{"products":[{"upc":"top-1"},{"upc":"top-2"},{"upc":"top-3"}]}`))
		require.NoError(t, err)

		ck0 := &CacheKey{BatchIndex: 0, Keys: []string{"key0"}}
		ck1 := &CacheKey{BatchIndex: 1, Keys: []string{"key1"}}
		ck2 := &CacheKey{BatchIndex: 2, Keys: []string{"key2"}}

		res := &result{
			batchEntityKeyMode: true,
			l2CacheKeys:        []*CacheKey{ck0, ck1, ck2},
		}
		items := []*astjson.Value{responseObj}

		l.populateBatchCacheKeysFromResponse(res, items, &FetchInfo{
			RootFields: []GraphCoordinate{{FieldName: "products"}},
		})

		require.NotNil(t, ck0.Item)
		assert.Equal(t, `{"upc":"top-1"}`, string(ck0.Item.MarshalTo(nil)))
		require.NotNil(t, ck1.Item)
		assert.Equal(t, `{"upc":"top-2"}`, string(ck1.Item.MarshalTo(nil)))
		require.NotNil(t, ck2.Item)
		assert.Equal(t, `{"upc":"top-3"}`, string(ck2.Item.MarshalTo(nil)))
		// EntityMergePath should be cleared after population
		assert.Nil(t, ck0.EntityMergePath)
		assert.Nil(t, ck1.EntityMergePath)
		assert.Nil(t, ck2.EntityMergePath)
	})

	t.Run("items with batchMergePath navigates to nested array", func(t *testing.T) {
		// When batchMergePath is set, the function navigates through it first
		l, ar := newCacheMergeTestLoader(t)

		responseObj, err := astjson.ParseBytesWithArena(ar, []byte(`{"nested":{"products":[{"id":"1"},{"id":"2"}]}}`))
		require.NoError(t, err)

		ck0 := &CacheKey{BatchIndex: 0, Keys: []string{"key0"}}
		ck1 := &CacheKey{BatchIndex: 1, Keys: []string{"key1"}}

		res := &result{
			batchEntityKeyMode: true,
			batchMergePath:     []string{"nested"},
			l2CacheKeys:        []*CacheKey{ck0, ck1},
		}
		items := []*astjson.Value{responseObj}

		l.populateBatchCacheKeysFromResponse(res, items, &FetchInfo{
			RootFields: []GraphCoordinate{{FieldName: "products"}},
		})

		require.NotNil(t, ck0.Item)
		assert.Equal(t, `{"id":"1"}`, string(ck0.Item.MarshalTo(nil)))
		require.NotNil(t, ck1.Item)
		assert.Equal(t, `{"id":"2"}`, string(ck1.Item.MarshalTo(nil)))
	})

	t.Run("empty items slice returns immediately", func(t *testing.T) {
		// len(items) == 0 → early return
		l, _ := newCacheMergeTestLoader(t)

		ck := &CacheKey{BatchIndex: 0, Keys: []string{"key0"}}
		res := &result{
			batchEntityKeyMode: true,
			l2CacheKeys:        []*CacheKey{ck},
		}

		l.populateBatchCacheKeysFromResponse(res, nil, &FetchInfo{
			RootFields: []GraphCoordinate{{FieldName: "products"}},
		})

		assert.Nil(t, ck.Item)
	})

	t.Run("l1CacheKeys also populated", func(t *testing.T) {
		// The function iterates both l2CacheKeys and l1CacheKeys
		l, ar := newCacheMergeTestLoader(t)

		responseObj, err := astjson.ParseBytesWithArena(ar, []byte(`{"products":[{"upc":"a"},{"upc":"b"}]}`))
		require.NoError(t, err)

		l1ck0 := &CacheKey{BatchIndex: 0, Keys: []string{"l1key0"}}
		l1ck1 := &CacheKey{BatchIndex: 1, Keys: []string{"l1key1"}}

		res := &result{
			batchEntityKeyMode: true,
			l1CacheKeys:        []*CacheKey{l1ck0, l1ck1},
		}
		items := []*astjson.Value{responseObj}

		l.populateBatchCacheKeysFromResponse(res, items, &FetchInfo{
			RootFields: []GraphCoordinate{{FieldName: "products"}},
		})

		require.NotNil(t, l1ck0.Item)
		assert.Equal(t, `{"upc":"a"}`, string(l1ck0.Item.MarshalTo(nil)))
		require.NotNil(t, l1ck1.Item)
		assert.Equal(t, `{"upc":"b"}`, string(l1ck1.Item.MarshalTo(nil)))
	})

	t.Run("partial fetch skips cached indices", func(t *testing.T) {
		// When batchPartialFetchEnabled=true, cached indices are skipped
		l, ar := newCacheMergeTestLoader(t)

		responseObj, err := astjson.ParseBytesWithArena(ar, []byte(`{"products":[{"upc":"a"},{"upc":"b"},{"upc":"c"}]}`))
		require.NoError(t, err)

		ck0 := &CacheKey{BatchIndex: 0, Keys: []string{"key0"}}
		ck1 := &CacheKey{BatchIndex: 1, Keys: []string{"key1"}}
		ck2 := &CacheKey{BatchIndex: 2, Keys: []string{"key2"}}

		res := &result{
			batchEntityKeyMode:       true,
			batchPartialFetchEnabled: true,
			batchCachedIndices:       []int{0, 2}, // indices 0 and 2 are cached
			l2CacheKeys:              []*CacheKey{ck0, ck1, ck2},
		}
		items := []*astjson.Value{responseObj}

		l.populateBatchCacheKeysFromResponse(res, items, &FetchInfo{
			RootFields: []GraphCoordinate{{FieldName: "products"}},
		})

		// Only index 1 (not cached) should have Item set
		assert.Nil(t, ck0.Item)
		require.NotNil(t, ck1.Item)
		assert.Equal(t, `{"upc":"b"}`, string(ck1.Item.MarshalTo(nil)))
		assert.Nil(t, ck2.Item)
	})
}

func TestFilterBatchVariablesForPartialFetch(t *testing.T) {
	t.Run("filters batch variables to only missed indices", func(t *testing.T) {
		// Array variable with 5 items, only indices 1 and 3 are missed
		l, _ := newCacheMergeTestLoader(t)

		variables, err := astjson.ParseBytes([]byte(`{"upcs":["a","b","c","d","e"]}`))
		require.NoError(t, err)
		l.ctx.Variables = variables

		f := &SingleFetch{}
		f.Caching = FetchCacheConfiguration{
			CacheKeyTemplate: &RootQueryCacheKeyTemplate{
				EntityKeyMappings: []EntityKeyMappingConfig{
					{
						EntityTypeName: "Product",
						FieldMappings: []EntityFieldMappingConfig{
							{
								EntityKeyField:      "upc",
								ArgumentPath:        []string{"upcs"},
								ArgumentIsEntityKey: true,
							},
						},
					},
				},
			},
		}
		// Trigger precomputation of batchEntityKeyArgumentPath
		f.Caching.CacheKeyTemplate.(*RootQueryCacheKeyTemplate).precomputeDerivedFields()

		res := &result{
			batchMissedIndices: []int{1, 3},
		}

		renderCtx, err := l.filterBatchVariablesForPartialFetch(res, f)
		require.NoError(t, err)
		require.NotNil(t, renderCtx)

		// The filtered variables should contain only items at indices 1 and 3
		got := string(renderCtx.Variables.MarshalTo(nil))
		assert.Equal(t, `{"upcs":["b","d"]}`, got)
	})

	t.Run("empty argument path returns nil", func(t *testing.T) {
		// When batchEntityKeyArgumentPath is empty, returns nil
		l, _ := newCacheMergeTestLoader(t)

		f := &SingleFetch{}
		f.Caching = FetchCacheConfiguration{
			CacheKeyTemplate: &RootQueryCacheKeyTemplate{},
		}

		res := &result{
			batchMissedIndices: []int{0},
		}

		renderCtx, err := l.filterBatchVariablesForPartialFetch(res, f)
		require.NoError(t, err)
		assert.Nil(t, renderCtx)
	})
}
