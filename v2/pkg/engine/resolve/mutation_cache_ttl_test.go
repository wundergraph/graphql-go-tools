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
			{Operation: "set", Keys: []string{`{"__typename":"User","key":{"id":"u1"}}`}, TTL: 60 * time.Second}, // L2 write uses mutation TTL override (60s), not entity default (300s); no prior "get" because mutations skip L2 reads
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
			{Operation: "set", Keys: []string{`{"__typename":"User","key":{"id":"u1"}}`}, TTL: 300 * time.Second}, // L2 write uses entity default TTL (300s); no mutation override (MutationCacheTTLOverride=0)
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
