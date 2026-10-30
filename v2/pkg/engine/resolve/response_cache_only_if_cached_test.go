package resolve

import (
	"bytes"
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestResponseCacheOnlyIfCached(t *testing.T) {
	resolveOnce := func(t *testing.T, ds DataSource, opts ResponseCacheOptions) (*Context, string) {
		t.Helper()
		ctx := NewContext(context.Background())
		ctx.SetResponseCache(opts)
		out := &bytes.Buffer{}
		_, err := newTestResolver(t, baseResolverOpts()).ResolveGraphQLResponse(ctx, singleFetchResponse(ds), nil, out)
		require.NoError(t, err)
		return ctx, out.String()
	}

	t.Run("a miss is not fetched", func(t *testing.T) {
		calls := &atomic.Int32{}
		ds := privateDataSource{cacheControl: "public, max-age=60", calls: calls}
		ctx, out := resolveOnce(t, ds, ResponseCacheOptions{Store: newTestCache(), DefaultTTL: time.Minute, OnlyIfCached: true})
		require.Zero(t, calls.Load())
		require.True(t, ctx.ResponseCacheUnsatisfied())
		require.Contains(t, out, "only-if-cached")
	})

	t.Run("a hit is served", func(t *testing.T) {
		calls := &atomic.Int32{}
		ds := privateDataSource{cacheControl: "public, max-age=60", calls: calls}
		store := newTestCache()
		_, _ = resolveOnce(t, ds, ResponseCacheOptions{Store: store, DefaultTTL: time.Minute})
		require.EqualValues(t, 1, calls.Load())

		ctx, out := resolveOnce(t, ds, ResponseCacheOptions{Store: store, DefaultTTL: time.Minute, OnlyIfCached: true})
		require.EqualValues(t, 1, calls.Load())
		require.False(t, ctx.ResponseCacheUnsatisfied())
		require.Equal(t, `{"data":{"me":"anonymous"}}`, out)
	})

	t.Run("an uncached subgraph is not fetched", func(t *testing.T) {
		calls := &atomic.Int32{}
		ds := privateDataSource{cacheControl: "public, max-age=60", calls: calls}
		ctx, _ := resolveOnce(t, ds, ResponseCacheOptions{
			Store: newTestCache(), DefaultTTL: time.Minute, OnlyIfCached: true,
			Subgraphs: map[string]ResponseCacheSubgraphOptions{"accounts": {Disabled: true}},
		})
		require.Zero(t, calls.Load())
		require.True(t, ctx.ResponseCacheUnsatisfied())
	})
}
