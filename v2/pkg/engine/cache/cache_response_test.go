package cache

import (
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wundergraph/astjson"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan/cacheconfig"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

// clientAnswerRequest begins ONE request of a controller carrying the given
// global knobs, and returns both sides of the client cache answer: the Context
// the integrator reads it from and the cache surface that folds it.
func clientAnswerRequest(t *testing.T, store Store, global cacheconfig.GlobalCacheConfig) (*resolve.Context, resolve.RequestCache) {
	t.Helper()
	ctx := resolve.NewContext(t.Context())
	return ctx, NewController(store, nil, WithGlobalConfig(global)).BeginRequest(ctx)
}

// fetchPart runs one miss -> fresh-result cycle for an item on an already begun
// request, with the Cache-Control the subgraph answered with ("" = none). It
// does NOT end the request: a response is folded from several parts.
func fetchPart(t *testing.T, rc resolve.RequestCache, cfg *resolve.FetchCacheConfig, item *astjson.Value, responseData, cacheControlValue string) {
	t.Helper()
	decision, handle := prepare(t, rc, cfg, item)
	require.Equal(t, resolve.DecisionFetch, decision)
	require.NoError(t, rc.OnFetchResult(handle, resolve.MergeInput{
		Items:           []*astjson.Value{item},
		ResponseData:    astjson.MustParseBytes([]byte(responseData)),
		ResponseHeaders: cacheControlHeader(cacheControlValue),
		Arena:           beginner(),
	}))
}

// servePart runs one full-hit cycle for an item: the lookup that covers it and
// the splice hook that folds what is left of the entry's freshness.
func servePart(t *testing.T, rc resolve.RequestCache, cfg *resolve.FetchCacheConfig, item *astjson.Value) {
	t.Helper()
	decision, handle := prepare(t, rc, cfg, item)
	require.Equal(t, resolve.DecisionSkipFullHit, decision)
	require.NoError(t, rc.OnFetchSkipped(handle, resolve.MergeInput{
		Items: []*astjson.Value{item},
		Arena: beginner(),
	}))
}

// TestClientCacheAnswerFreshness pins the freshness fold: which part
// contributes what, which part makes the whole response unstorable, and the
// difference between "nothing to say" and "do not store this".
func TestClientCacheAnswerFreshness(t *testing.T) {
	fresh := `{"__typename":"Product","name":"Table","price":100}`

	t.Run("a fresh result contributes the lifetime its entry was written under", func(t *testing.T) {
		store := newTestStore()
		ctx, rc := clientAnswerRequest(t, store, cacheconfig.GlobalCacheConfig{})
		fetchPart(t, rc, entityConfig(t, time.Minute), productItem(t, "1"), fresh, "")
		rc.EndRequest()

		assert.Equal(t, resolve.CacheResponseInfo{
			HasPolicy: true,
			MaxAge:    time.Minute,
		}, ctx.CacheResponseInfo())
	})

	t.Run("the header-driven lifetime is what a fresh part contributes", func(t *testing.T) {
		store := newTestStore()
		ctx, rc := clientAnswerRequest(t, store, cacheconfig.GlobalCacheConfig{})
		// The origin's max-age beats the configured 5m for the entry AND for the
		// answer: one resolved lifetime feeds both.
		fetchPart(t, rc, entityConfig(t, 5*time.Minute), productItem(t, "1"), fresh, "max-age=30")
		rc.EndRequest()

		assert.Equal(t, resolve.CacheResponseInfo{
			HasPolicy: true,
			MaxAge:    30 * time.Second,
		}, ctx.CacheResponseInfo())
	})

	t.Run("the shortest-lived part decides the answer", func(t *testing.T) {
		store := newTestStore()
		ctx, rc := clientAnswerRequest(t, store, cacheconfig.GlobalCacheConfig{})
		fetchPart(t, rc, entityConfig(t, 5*time.Minute), productItem(t, "1"), fresh, "")
		fetchPart(t, rc, entityConfig(t, time.Minute), productItem(t, "2"), fresh, "")
		rc.EndRequest()

		assert.Equal(t, resolve.CacheResponseInfo{
			HasPolicy: true,
			MaxAge:    time.Minute,
		}, ctx.CacheResponseInfo())
	})

	t.Run("a served entry contributes what is LEFT of its lifetime, and counts down", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			store := newTestStore()
			cfg := entityConfig(t, time.Minute)

			// The request that writes the entry answers with its full lifetime.
			writing, writingCache := clientAnswerRequest(t, store, cacheconfig.GlobalCacheConfig{})
			fetchPart(t, writingCache, cfg, productItem(t, "1"), fresh, "")
			writingCache.EndRequest()
			assert.Equal(t, resolve.CacheResponseInfo{
				HasPolicy: true,
				MaxAge:    time.Minute,
			}, writing.CacheResponseInfo())

			time.Sleep(20 * time.Second)
			served, servedCache := clientAnswerRequest(t, store, cacheconfig.GlobalCacheConfig{})
			servePart(t, servedCache, cfg, productItem(t, "1"))
			servedCache.EndRequest()
			assert.Equal(t, resolve.CacheResponseInfo{
				HasPolicy: true,
				MaxAge:    40 * time.Second,
			}, served.CacheResponseInfo())

			// Two seconds later the SAME entry answers two seconds less: the
			// answer counts down with the entry instead of restarting at its
			// written lifetime.
			time.Sleep(2 * time.Second)
			later, laterCache := clientAnswerRequest(t, store, cacheconfig.GlobalCacheConfig{})
			servePart(t, laterCache, cfg, productItem(t, "1"))
			laterCache.EndRequest()
			assert.Equal(t, resolve.CacheResponseInfo{
				HasPolicy: true,
				MaxAge:    38 * time.Second,
			}, later.CacheResponseInfo())
		})
	})

	t.Run("a served entry bounds a fresher part of the same response", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			store := newTestStore()
			warm := entityConfig(t, time.Minute)
			_, warming := clientAnswerRequest(t, store, cacheconfig.GlobalCacheConfig{})
			fetchPart(t, warming, warm, productItem(t, "1"), fresh, "")
			warming.EndRequest()

			time.Sleep(45 * time.Second)
			ctx, rc := clientAnswerRequest(t, store, cacheconfig.GlobalCacheConfig{})
			servePart(t, rc, warm, productItem(t, "1"))
			fetchPart(t, rc, entityConfig(t, 5*time.Minute), productItem(t, "2"), fresh, "")
			rc.EndRequest()

			// The 15s left on the served entry, not the 5m the fresh part was
			// just written under.
			assert.Equal(t, resolve.CacheResponseInfo{
				HasPolicy: true,
				MaxAge:    15 * time.Second,
			}, ctx.CacheResponseInfo())
		})
	})

	t.Run("a part served from the request-lifetime layer adds no constraint of its own", func(t *testing.T) {
		store := newTestStore()
		cfg := entityConfig(t, time.Minute)
		ctx, rc := clientAnswerRequest(t, store, cacheconfig.GlobalCacheConfig{})
		fetchPart(t, rc, cfg, productItem(t, "1"), fresh, "")
		// The second fetch of the same entity rides the value the first one put
		// in the request-lifetime layer. Counting that as uncacheable would make
		// a fully cacheable response no-store.
		servePart(t, rc, cfg, productItem(t, "1"))
		rc.EndRequest()

		assert.Equal(t, resolve.CacheResponseInfo{
			HasPolicy: true,
			MaxAge:    time.Minute,
		}, ctx.CacheResponseInfo())
	})

	t.Run("a part the origin marked no-store makes the whole response no-store", func(t *testing.T) {
		store := newTestStore()
		ctx, rc := clientAnswerRequest(t, store, cacheconfig.GlobalCacheConfig{})
		fetchPart(t, rc, entityConfig(t, time.Minute), productItem(t, "1"), fresh, "")
		fetchPart(t, rc, entityConfig(t, time.Minute), productItem(t, "2"), fresh, "no-store")
		rc.EndRequest()

		// The most restrictive part wins outright: the minute the first part is
		// good for is not reported alongside it.
		assert.Equal(t, resolve.CacheResponseInfo{
			HasPolicy: true,
			NoStore:   true,
		}, ctx.CacheResponseInfo())
	})

	t.Run("a part with a non-positive resolved lifetime makes the response no-store", func(t *testing.T) {
		store := newTestStore()
		ctx, rc := clientAnswerRequest(t, store, cacheconfig.GlobalCacheConfig{})
		fetchPart(t, rc, entityConfig(t, time.Minute), productItem(t, "1"), fresh, "max-age=0")
		rc.EndRequest()

		assert.Equal(t, resolve.CacheResponseInfo{
			HasPolicy: true,
			NoStore:   true,
		}, ctx.CacheResponseInfo())
	})

	t.Run("a part that reaches no store layer makes the response no-store", func(t *testing.T) {
		store := newTestStore()
		ctx, rc := clientAnswerRequest(t, store, cacheconfig.GlobalCacheConfig{})
		// TTL 0 leaves the fetch on the request-lifetime layer alone: nothing a
		// later request — or a CDN — could serve from.
		fetchPart(t, rc, entityConfig(t, 0), productItem(t, "1"), fresh, "")
		rc.EndRequest()

		assert.Equal(t, resolve.CacheResponseInfo{
			HasPolicy: true,
			NoStore:   true,
		}, ctx.CacheResponseInfo())
	})

	t.Run("a failed fetch makes the response no-store", func(t *testing.T) {
		store := newTestStore()
		cfg := entityConfig(t, time.Minute)
		ctx, rc := clientAnswerRequest(t, store, cacheconfig.GlobalCacheConfig{})
		item := productItem(t, "1")
		_, handle := prepare(t, rc, cfg, item)
		require.NoError(t, rc.OnFetchResult(handle, resolve.MergeInput{
			Items:       []*astjson.Value{item},
			FetchFailed: true,
			Arena:       beginner(),
		}))
		rc.EndRequest()

		assert.Equal(t, resolve.CacheResponseInfo{
			HasPolicy: true,
			NoStore:   true,
		}, ctx.CacheResponseInfo())
	})

	t.Run("a fetch that ran outside the cache makes the response no-store", func(t *testing.T) {
		store := newTestStore()
		ctx, rc := clientAnswerRequest(t, store, cacheconfig.GlobalCacheConfig{})
		fetchPart(t, rc, entityConfig(t, time.Minute), productItem(t, "1"), fresh, "")
		rc.OnUncachedFetch()
		rc.EndRequest()

		// The cached part's minute is not advertised for a response that also
		// carries data no policy covers.
		assert.Equal(t, resolve.CacheResponseInfo{
			HasPolicy: true,
			NoStore:   true,
		}, ctx.CacheResponseInfo())
	})

	t.Run("an uncached fetch counts however it is ordered against the cached one", func(t *testing.T) {
		store := newTestStore()
		ctx, rc := clientAnswerRequest(t, store, cacheconfig.GlobalCacheConfig{})
		// The uncached fetch runs FIRST here — the order the loader produces
		// whenever a cached entity hangs off an uncached root field.
		rc.OnUncachedFetch()
		fetchPart(t, rc, entityConfig(t, time.Minute), productItem(t, "1"), fresh, "")
		rc.EndRequest()

		assert.Equal(t, resolve.CacheResponseInfo{
			HasPolicy: true,
			NoStore:   true,
		}, ctx.CacheResponseInfo())
	})

	t.Run("uncached fetches alone answer nothing", func(t *testing.T) {
		store := newTestStore()
		ctx, rc := clientAnswerRequest(t, store, cacheconfig.GlobalCacheConfig{})
		rc.OnUncachedFetch()
		rc.OnUncachedFetch()
		rc.EndRequest()

		// The zero contribution that cannot stand alone: with nothing cached to
		// restrict, the answer stays empty and the integrator emits no header —
		// a graph without caching never grows a no-store header.
		assert.Equal(t, resolve.CacheResponseInfo{}, ctx.CacheResponseInfo())
	})

	t.Run("a request whose fetches are none of the cache's business answers nothing", func(t *testing.T) {
		store := newTestStore()
		ctx, rc := clientAnswerRequest(t, store, cacheconfig.GlobalCacheConfig{})
		rc.EndRequest()

		// An EMPTY answer, not a no-store one: the integrator emits no header at
		// all rather than forbidding a cache it never consulted.
		assert.Equal(t, resolve.CacheResponseInfo{}, ctx.CacheResponseInfo())
	})

	t.Run("a request without a cache controller answers nothing", func(t *testing.T) {
		assert.Equal(t, resolve.CacheResponseInfo{}, resolve.NewContext(t.Context()).CacheResponseInfo())
	})
}

// TestClientCacheAnswerPrivacy pins the privacy fold: both sources mark the
// response private, whether or not anything was stored.
func TestClientCacheAnswerPrivacy(t *testing.T) {
	fresh := `{"__typename":"Product","name":"Table","price":100}`

	t.Run("a statically-private part marks the response private", func(t *testing.T) {
		store := newTestStore()
		ctx := resolve.NewContext(t.Context())
		ctx.SetPrivatePartitionProvider(&partitionProvider{identities: map[string]string{"products": "user-a"}})
		rc := NewController(store, nil).BeginRequest(ctx)
		fetchPart(t, rc, privateEntityConfig(t, time.Minute), productItem(t, "1"), fresh, "")
		rc.EndRequest()

		// The entry WAS stored — in the requester's partition — so the response
		// stays fresh for a minute, for that requester alone.
		assert.Equal(t, resolve.CacheResponseInfo{
			HasPolicy: true,
			MaxAge:    time.Minute,
			Private:   true,
		}, ctx.CacheResponseInfo())
	})

	t.Run("a private response header marks the response private and stores nothing", func(t *testing.T) {
		store := newTestStore()
		ctx, rc := clientAnswerRequest(t, store, cacheconfig.GlobalCacheConfig{})
		fetchPart(t, rc, entityConfig(t, time.Minute), productItem(t, "1"), fresh, "private, max-age=600")
		rc.EndRequest()

		// A statically-public fetch answered private has nowhere to put the
		// value: no entry, hence no freshness to promise.
		assert.Equal(t, resolve.CacheResponseInfo{
			HasPolicy: true,
			Private:   true,
			NoStore:   true,
		}, ctx.CacheResponseInfo())
	})

	t.Run("a statically-private part without a requester identity makes the response no-store", func(t *testing.T) {
		store := newTestStore()
		ctx := resolve.NewContext(t.Context())
		rc := NewController(store, nil).BeginRequest(ctx)
		fetchPart(t, rc, privateEntityConfig(t, time.Minute), productItem(t, "1"), fresh, "")
		rc.EndRequest()

		assert.Equal(t, resolve.CacheResponseInfo{
			HasPolicy: true,
			Private:   true,
			NoStore:   true,
		}, ctx.CacheResponseInfo())
	})
}

// TestClientCacheAnswerTags pins the CDN tag union: which entries contribute,
// how duplicates collapse, and that the order is the sorted one.
func TestClientCacheAnswerTags(t *testing.T) {
	fresh := `{"__typename":"Product","name":"Table","price":100}`
	rootResponse := `{"products":[{"name":"Table"}]}`

	t.Run("the union collects the written entries, sorted and deduplicated", func(t *testing.T) {
		store := newTestStore()
		ctx, rc := clientAnswerRequest(t, store, cacheconfig.GlobalCacheConfig{EmitCdnTags: true})
		fetchPart(t, rc, entityConfig(t, time.Minute), productItem(t, "1"), fresh, "")
		fetchPart(t, rc, entityConfig(t, time.Minute), productItem(t, "2"), fresh, "")
		fetchPart(t, rc, rootFieldConfig(time.Minute), rootItem(), rootResponse, "")
		rc.EndRequest()

		// The two entities share their subgraph and type tags — one address
		// each — while their entity tags keep them separately purgeable, and
		// the root-field entry adds the coordinate's Query type tag.
		assert.Equal(t, resolve.CacheResponseInfo{
			HasPolicy: true,
			MaxAge:    time.Minute,
			Tags: []string{
				"entity:products:Product:4f93140518d68e67", // upc "2"
				"entity:products:Product:d3cc039c7a9789e7", // upc "1"
				"subgraph:products",
				"type:products:Product",
				"type:products:Query",
			},
		}, ctx.CacheResponseInfo())
	})

	t.Run("a served entry's tags join the union: the CDN caches what the client sees", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			store := newTestStore()
			cfg := entityConfig(t, time.Minute)
			_, warming := clientAnswerRequest(t, store, cacheconfig.GlobalCacheConfig{EmitCdnTags: true})
			fetchPart(t, warming, cfg, productItem(t, "1"), fresh, "")
			warming.EndRequest()

			ctx, rc := clientAnswerRequest(t, store, cacheconfig.GlobalCacheConfig{EmitCdnTags: true})
			servePart(t, rc, cfg, productItem(t, "1"))
			rc.EndRequest()

			// This response wrote nothing at all and is still fully purgeable.
			assert.Equal(t, resolve.CacheResponseInfo{
				HasPolicy: true,
				MaxAge:    time.Minute,
				Tags: []string{
					"entity:products:Product:d3cc039c7a9789e7", // upc "1"
					"subgraph:products",
					"type:products:Product",
				},
			}, ctx.CacheResponseInfo())
		})
	})

	t.Run("no tags are accumulated while emission is off", func(t *testing.T) {
		store := newTestStore()
		ctx, rc := clientAnswerRequest(t, store, cacheconfig.GlobalCacheConfig{})
		fetchPart(t, rc, entityConfig(t, time.Minute), productItem(t, "1"), fresh, "")
		rc.EndRequest()

		assert.Equal(t, resolve.CacheResponseInfo{
			HasPolicy: true,
			MaxAge:    time.Minute,
		}, ctx.CacheResponseInfo())
	})
}

// TestClientCacheAnswerKnobs pins the two emission switches, including the one
// combination that must cost nothing at all.
func TestClientCacheAnswerKnobs(t *testing.T) {
	fresh := `{"__typename":"Product","name":"Table","price":100}`

	t.Run("both knobs off allocate no aggregation state", func(t *testing.T) {
		store := newTestStore()
		ctx, rc := clientAnswerRequest(t, store, cacheconfig.GlobalCacheConfig{EmitClientCacheControl: ptr(false)})
		fetchPart(t, rc, entityConfig(t, time.Minute), productItem(t, "1"), fresh, "")
		// The loader's per-fetch notification must stay free too: with nothing to
		// accumulate into, it allocates and locks nothing.
		rc.OnUncachedFetch()
		rc.EndRequest()

		require.Nil(t, rc.(*requestCache).aggregate)
		assert.Equal(t, resolve.CacheResponseInfo{}, ctx.CacheResponseInfo())
	})

	t.Run("tags alone accumulate without a freshness answer", func(t *testing.T) {
		store := newTestStore()
		ctx, rc := clientAnswerRequest(t, store, cacheconfig.GlobalCacheConfig{
			EmitClientCacheControl: ptr(false),
			EmitCdnTags:            true,
		})
		fetchPart(t, rc, entityConfig(t, time.Minute), productItem(t, "1"), fresh, "")
		// An uncached fetch restricts a freshness answer that is not being
		// computed here, so it leaves the tag union alone.
		rc.OnUncachedFetch()
		rc.EndRequest()

		// No policy and no freshness: the integrator emits the tag header alone.
		assert.Equal(t, resolve.CacheResponseInfo{
			Tags: []string{
				"entity:products:Product:d3cc039c7a9789e7", // upc "1"
				"subgraph:products",
				"type:products:Product",
			},
		}, ctx.CacheResponseInfo())
	})
}
