package engine

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

// The entity cache stores the answers to entity fetches, so every case here is
// built around one: me.reviews is a single entity fetch, and topProducts.reviews
// is a batched one. The two are separate code paths in the loader, and only the
// reviews subgraph's call count says whether either was served from the cache.

const (
	singleEntityQuery   = `{ me { id username reviews { body } } }`
	meAnswer            = `{"data":{"me":{"id":"1234","username":"Me","__typename":"User"}}}`
	cacheableForAMinute = "public, max-age=60"
)

// batchQuery asks for the reviews of the first n products, which reaches the
// reviews subgraph as one _entities request carrying n representations.
func batchQuery(n int) string {
	return fmt.Sprintf(`{ topProducts(first: %d) { upc reviews { body } } }`, n)
}

func TestEntityCacheExecution(t *testing.T) {
	t.Parallel()

	t.Run("a single entity fetch is cached", func(t *testing.T) {
		t.Parallel()

		h := newHarness(t)
		h.users.answers(meAnswer)
		h.reviews.answers(reviewsAnswer("A review"))

		cache := newMapCache()
		first := h.execute(t, singleEntityQuery, withEntityCache(t, cache))
		second := h.execute(t, singleEntityQuery, withEntityCache(t, cache))

		// The bodies, not just the counter: a cache that served the wrong bytes
		// would satisfy the counter on its own.
		require.Equal(t, first, second)
		require.Contains(t, first, `"body":"A review"`)
		require.EqualValues(t, 1, h.reviews.calls(),
			"the second execution must be served from the cache")
	})

	t.Run("a batch entity fetch is cached", func(t *testing.T) {
		t.Parallel()

		h := newHarness(t)
		h.products.answers(productsAnswer("1", "2", "3"))
		h.reviews.answers(reviewsAnswer("r1", "r2", "r3"))

		cache := newMapCache()
		first := h.execute(t, batchQuery(3), withEntityCache(t, cache))
		require.Equal(t, 3, h.reviews.representationCount(t),
			"the three products must reach reviews as one batched entity fetch")

		second := h.execute(t, batchQuery(3), withEntityCache(t, cache))

		require.Equal(t, first, second)
		require.EqualValues(t, 1, h.reviews.calls())
		require.Len(t, cache.keys(), 3, "each entity in the batch gets its own entry")
	})

	t.Run("the entities a batch answered are cached even when one of them is null", func(t *testing.T) {
		t.Parallel()

		h := newHarness(t)
		cache := newMapCache()

		h.products.answers(productsAnswer("1", "2", "3"))
		h.reviews.answers(reviewsAnswer("r1", "r2", ""))
		h.execute(t, batchQuery(3), withEntityCache(t, cache))
		require.EqualValues(t, 1, h.reviews.calls())
		require.Len(t, cache.keys(), 2, "the null entity must not be stored")

		h.products.answers(productsAnswer("1", "2"))
		h.execute(t, batchQuery(2), withEntityCache(t, cache))
		require.EqualValues(t, 1, h.reviews.calls(),
			"the entities the subgraph did answer must be cached even though one of their batch was null")

		// And product 3 is still not cached, which is what makes the hit above a
		// partial write rather than the null having been stored as an entity of
		// its own.
		h.products.answers(productsAnswer("1", "2", "3"))
		h.execute(t, batchQuery(3), withEntityCache(t, cache))
		require.EqualValues(t, 2, h.reviews.calls(),
			"a batch containing an entity that was never cached must go back to the subgraph")
	})

	t.Run("a batch containing one uncached entity is refetched whole", func(t *testing.T) {
		t.Parallel()

		h := newHarness(t)
		cache := newMapCache()

		// Caches product 1 alone.
		h.products.answers(productsAnswer("1"))
		h.reviews.answers(reviewsAnswer("r1"))
		h.execute(t, batchQuery(1), withEntityCache(t, cache))
		require.EqualValues(t, 1, h.reviews.calls())

		// Product 1 is cached and 2 and 3 are not. A batch is served from the
		// cache only when every entity in it is present, so all three go to the
		// subgraph, product 1 included.
		h.products.answers(productsAnswer("1", "2", "3"))
		h.reviews.answers(reviewsAnswer("r1", "r2", "r3"))
		h.execute(t, batchQuery(3), withEntityCache(t, cache))

		require.EqualValues(t, 2, h.reviews.calls(),
			"one miss in a batch must send the whole batch to the subgraph")
		require.Equal(t, 3, h.reviews.representationCount(t),
			"the already cached entity is refetched along with the rest, not trimmed out")
	})

	t.Run("the same entity twice in one batch is one representation and one entry", func(t *testing.T) {
		t.Parallel()

		h := newHarness(t)
		h.products.answers(productsAnswer("1", "1", "2"))
		h.reviews.answers(reviewsAnswer("r1", "r2"))

		cache := newMapCache()
		h.execute(t, batchQuery(3), withEntityCache(t, cache))

		// The engine deduplicates representations before it sends them, so the
		// repeated product must not be asked for twice and must not take a second
		// entry, which would be the same bytes under the same key.
		require.Equal(t, 2, h.reviews.representationCount(t))
		require.Len(t, cache.keys(), 2)
	})

	// --- every reason the engine declines to cache an answer it did get ---

	t.Run("a response with no cache-control is not cached", func(t *testing.T) {
		t.Parallel()

		h := newHarness(t)
		h.users.answers(meAnswer)
		h.reviews.answers(reviewsAnswer("A review"))
		h.reviews.cacheControl("")

		cache := newMapCache()
		h.execute(t, singleEntityQuery, withEntityCache(t, cache))
		h.execute(t, singleEntityQuery, withEntityCache(t, cache))

		require.Empty(t, cache.keys())
		require.EqualValues(t, 2, h.reviews.calls(),
			"a subgraph that says nothing about its response must not have it cached")
	})

	t.Run("a private response is not cached", func(t *testing.T) {
		t.Parallel()

		h := newHarness(t)
		h.users.answers(meAnswer)
		h.reviews.answers(reviewsAnswer("A review"))
		h.reviews.cacheControl("private, max-age=60")

		cache := newMapCache()
		h.execute(t, singleEntityQuery, withEntityCache(t, cache))
		h.execute(t, singleEntityQuery, withEntityCache(t, cache))

		require.Empty(t, cache.keys())
		require.EqualValues(t, 2, h.reviews.calls(),
			"private names a cache that belongs to one client, which this is not")
	})

	t.Run("a response carrying subgraph errors is not cached", func(t *testing.T) {
		t.Parallel()

		h := newHarness(t)
		h.users.answers(meAnswer)

		// A 200 with data and errors together: the partial success a subgraph
		// sends when it answered some of what it was asked for. Even the entity
		// that did arrive is not cached, because nothing here says which of them
		// the error belongs to.
		h.reviews.answers(`{"data":{"_entities":[{"reviews":[{"body":"A review"}]}]},` +
			`"errors":[{"message":"something went wrong"}]}`)

		cache := newMapCache()
		h.execute(t, singleEntityQuery, withEntityCache(t, cache))
		h.execute(t, singleEntityQuery, withEntityCache(t, cache))

		require.Empty(t, cache.keys())
		require.EqualValues(t, 2, h.reviews.calls(),
			"an answer that came with errors must not be cached however cacheable it claims to be")
	})

	t.Run("a failed subgraph response is not cached", func(t *testing.T) {
		t.Parallel()

		h := newHarness(t)
		h.users.answers(meAnswer)
		h.reviews.answers(reviewsAnswer("A review"))
		h.reviews.status(http.StatusInternalServerError)

		cache := newMapCache()
		h.execute(t, singleEntityQuery, withEntityCache(t, cache))
		h.execute(t, singleEntityQuery, withEntityCache(t, cache))

		require.Empty(t, cache.keys())
		require.EqualValues(t, 2, h.reviews.calls(),
			"a failed response must not be cached however cacheable it claims to be")
	})

	t.Run("a batch of nothing but nulls caches nothing and reports nothing", func(t *testing.T) {
		t.Parallel()

		h := newHarness(t)
		h.products.answers(productsAnswer("1", "2"))
		h.reviews.answers(reviewsAnswer("", ""))

		cache := newMapCache()
		// withEntityCache fails the test if the cache reports an error, which is
		// the half of this that matters: nothing to store is an ordinary outcome,
		// not a failure to be reported.
		h.execute(t, batchQuery(2), withEntityCache(t, cache))
		h.execute(t, batchQuery(2), withEntityCache(t, cache))

		require.Empty(t, cache.keys())
		require.EqualValues(t, 2, h.reviews.calls())
	})

	// --- controls, and the things a hit must and must not skip ---

	t.Run("without a cache every execution reaches the subgraph", func(t *testing.T) {
		t.Parallel()

		// The control for every case above. Without it a count of one could
		// equally be the plan cache or the engine coalescing two executions, and
		// none of this would prove anything about entity caching.
		h := newHarness(t)
		h.users.answers(meAnswer)
		h.reviews.answers(reviewsAnswer("A review"))

		h.execute(t, singleEntityQuery)
		h.execute(t, singleEntityQuery)

		require.EqualValues(t, 2, h.reviews.calls(),
			"with no entity cache on the context every execution must reach the subgraph")
	})

	t.Run("different selection sets do not share an entry", func(t *testing.T) {
		t.Parallel()

		h := newHarness(t)
		h.users.answers(meAnswer)
		cache := newMapCache()

		h.reviews.answers(reviewsAnswer("A review"))
		h.execute(t, singleEntityQuery, withEntityCache(t, cache))

		// The same entity, asked for more of. The stored entry answers the first
		// selection and cannot answer this one, so it must not be reused for it.
		h.reviews.answers(`{"data":{"_entities":[{"reviews":[{"body":"A review","product":{"upc":"1"}}]}]}}`)
		wider := h.execute(t, `{ me { id username reviews { body product { upc } } } }`, withEntityCache(t, cache))

		require.Contains(t, wider, `"upc":"1"`,
			"the wider selection must be answered in full, not from the narrower entry")
		require.EqualValues(t, 2, h.reviews.calls(),
			"the same entity asked for different fields must not share a cache entry")
		require.Len(t, cache.keys(), 2)
	})

	t.Run("a cache hit is still rate limited", func(t *testing.T) {
		t.Parallel()

		// A hit skips the fetch, but not the checks that run before one. Rate
		// limiting happens in the prepare phase, ahead of the cache lookup, so a
		// fetch served from the cache still spends budget for a request that never
		// leaves the process. Surprising, and worth pinning either way.
		h := newHarness(t)
		h.users.answers(meAnswer)
		h.reviews.answers(reviewsAnswer("A review"))

		limiter := &countingRateLimiter{}
		cache := newMapCache()

		h.execute(t, singleEntityQuery, withEntityCache(t, cache), withRateLimiter(limiter))
		uncached := limiter.preFetch.Load()
		require.EqualValues(t, 2, uncached, "both the users and the reviews fetch are checked")

		h.execute(t, singleEntityQuery, withEntityCache(t, cache), withRateLimiter(limiter))

		require.EqualValues(t, 1, h.reviews.calls(), "the second execution must be a hit")
		require.EqualValues(t, 4, limiter.preFetch.Load(),
			"a fetch served from the cache is checked exactly as the fetch it stands in for was")
	})

	t.Run("a cache hit skips the loader hooks", func(t *testing.T) {
		t.Parallel()

		// Pins today's behaviour rather than endorsing it. The hooks are where a
		// router hangs subgraph spans and metrics, and a hit reaches neither
		// OnLoad nor OnFinished, so a cached entity fetch is invisible to
		// subgraph telemetry. Should that ever be wanted, this test is the one
		// that has to be changed on purpose.
		h := newHarness(t)
		h.users.answers(meAnswer)
		h.reviews.answers(reviewsAnswer("A review"))

		hooks := &countingLoaderHooks{}
		cache := newMapCache()

		h.execute(t, singleEntityQuery, withEntityCache(t, cache), withLoaderHooks(hooks))
		require.EqualValues(t, 2, hooks.onLoad.Load(), "users and reviews are both fetched")
		require.EqualValues(t, 2, hooks.onFinished.Load())

		h.execute(t, singleEntityQuery, withEntityCache(t, cache), withLoaderHooks(hooks))

		require.EqualValues(t, 1, h.reviews.calls())
		require.EqualValues(t, 3, hooks.onLoad.Load(),
			"only the users fetch runs on the second execution; the cached reviews fetch reaches no hook")
		require.EqualValues(t, 3, hooks.onFinished.Load())
	})

	t.Run("a merged multi entity fetch is never cached", func(t *testing.T) {
		t.Parallel()

		// With multi fetch on, the planner collapses the two reviews fetches of one
		// parallel wave into a single aliased _entities request, and that shape
		// carries no cache keys and never reaches the collector: the merge phase
		// returns through mergeMultiEntityResult before it. So entity caching
		// silently turns itself off for every merged wave.
		//
		// This pins that as it stands today. It is the one shape where enabling a
		// planner optimisation costs you the cache without saying so.
		h := newHarness(t, func(c *Configuration) { c.EnableMultiFetch() })
		h.users.answers(meAnswer)
		h.products.answers(productsAnswer("1", "2"))
		// f1 and f2 are the aliases the planner gives the two merged fetches, in
		// fetch id order: f1 is the User fetch with its one representation, f2 the
		// Product fetch with its two.
		h.reviews.answers(`{"data":{"f1":[{"reviews":[{"body":"r1"}]}],` +
			`"f2":[{"reviews":[{"body":"r2"}]},{"reviews":[{"body":"r3"}]}]}}`)

		const bothShapes = `{ me { id username reviews { body } } topProducts(first: 2) { upc reviews { body } } }`

		cache := newMapCache()
		merged := h.execute(t, bothShapes, withEntityCache(t, cache))
		h.execute(t, bothShapes, withEntityCache(t, cache))

		// The answer is whole, so the emptiness below is the cache declining to
		// store a good response rather than a broken response being declined.
		require.Contains(t, merged, `"body":"r1"`)
		require.Contains(t, merged, `"body":"r2"`)
		require.Empty(t, cache.keys(), "a merged multi entity fetch stores nothing")
		require.EqualValues(t, 2, h.reviews.calls(),
			"one request per execution, so the two fetches really were merged into one")

		// That it really was one merged fetch and not two ordinary ones that each
		// happened to miss: the aliases only exist in the merged shape. Cases 1
		// and 2 above show unmerged entity fetches of both shapes do cache, so the
		// difference here is the merging.
		require.Contains(t, h.reviews.last.Load().(string), `f1: _entities`)
		require.Contains(t, h.reviews.last.Load().(string), `f2: _entities`)
	})
}
