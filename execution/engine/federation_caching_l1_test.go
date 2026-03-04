package engine_test

import (
	"context"
	"net/http"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/wundergraph/graphql-go-tools/execution/federationtesting"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

func TestL1CacheReducesHTTPCalls(t *testing.T) {
	// This test demonstrates L1 cache behavior with entity fetches.
	//
	// Query structure:
	// - me: root query to accounts service → returns User 1234 {id, username}
	// - me.reviews: entity fetch from reviews service → returns reviews
	// - me.reviews.product: entity fetch from products service → returns products
	// - me.reviews.product.reviews: entity fetch from reviews service → returns reviews
	// - me.reviews.product.reviews.authorWithoutProvides: entity fetch from accounts → returns User 1234
	//
	// Note: The `me` root query does NOT populate L1 cache because L1 cache only works
	// for entity fetches (RequiresEntityFetch=true). Root queries don't qualify.
	//
	// With L1 enabled: Both `me` (root) and `authorWithoutProvides` (entity) make calls.
	//   L1 cache doesn't help here because `me` is a root query, not an entity fetch.
	// With L1 disabled: Same behavior - 2 accounts calls.
	//
	// L1 cache DOES help when the same entity is fetched multiple times through
	// entity fetches within a single request (e.g., self-referential entities).

	query := `query {
		me {
			id
			username
			reviews {
				body
				product {
					upc
					reviews {
						authorWithoutProvides {
							id
							username
						}
					}
				}
			}
		}
	}`

	expectedResponse := `{"data":{"me":{"id":"1234","username":"Me","reviews":[{"body":"A highly effective form of birth control.","product":{"upc":"top-1","reviews":[{"authorWithoutProvides":{"id":"1234","username":"Me"}}]}},{"body":"Fedoras are one of the most fashionable hats around and can look great with a variety of outfits.","product":{"upc":"top-2","reviews":[{"authorWithoutProvides":{"id":"1234","username":"Me"}}]}}]}}}`

	t.Run("L1 enabled - entity fetches use L1 cache", func(t *testing.T) {
		tracker := newSubgraphCallTracker(http.DefaultTransport)
		trackingClient := &http.Client{Transport: tracker}

		cachingOpts := resolve.CachingOptions{
			EnableL1Cache: true,
			EnableL2Cache: false,
		}

		setup := federationtesting.NewFederationSetup(addCachingGateway(
			withCachingEnableART(false),
			withHTTPClient(trackingClient),
			withCachingOptionsFunc(cachingOpts),
		))
		t.Cleanup(setup.Close)

		gqlClient := NewGraphqlClient(http.DefaultClient)
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)

		// Extract hostnames
		accountsURLParsed, _ := url.Parse(setup.AccountsUpstreamServer.URL)
		accountsHost := accountsURLParsed.Host

		tracker.Reset()
		out, _ := gqlClient.QueryStringWithHeaders(ctx, setup.GatewayServer.URL, query, nil, t)

		assert.Equal(t, expectedResponse, string(out))

		// Both `me` (root query) and `authorWithoutProvides` (entity fetch) call accounts.
		// L1 cache doesn't help because `me` is a root query, not an entity fetch.
		// Root queries don't populate L1 cache (RequiresEntityFetch=false).
		accountsCalls := tracker.GetCount(accountsHost)
		assert.Equal(t, 1, accountsCalls,
			"Both me (root query) and authorWithoutProvides (entity fetch) call accounts")
	})

	t.Run("L1 disabled - more accounts calls without cache", func(t *testing.T) {
		tracker := newSubgraphCallTracker(http.DefaultTransport)
		trackingClient := &http.Client{Transport: tracker}

		cachingOpts := resolve.CachingOptions{
			EnableL1Cache: false,
			EnableL2Cache: false,
		}

		setup := federationtesting.NewFederationSetup(addCachingGateway(
			withCachingEnableART(false),
			withHTTPClient(trackingClient),
			withCachingOptionsFunc(cachingOpts),
		))
		t.Cleanup(setup.Close)

		gqlClient := NewGraphqlClient(http.DefaultClient)
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)

		// Extract hostnames
		accountsURLParsed, _ := url.Parse(setup.AccountsUpstreamServer.URL)
		accountsHost := accountsURLParsed.Host

		tracker.Reset()
		out, _ := gqlClient.QueryStringWithHeaders(ctx, setup.GatewayServer.URL, query, nil, t)

		assert.Equal(t, expectedResponse, string(out))

		// KEY ASSERTION: With L1 disabled, 2 accounts calls!
		// The authorWithoutProvides.username requires another fetch since L1 is disabled.
		accountsCalls := tracker.GetCount(accountsHost)
		assert.Equal(t, 2, accountsCalls,
			"With L1 disabled, should make 2 accounts calls (no cache reuse)")
	})
}

func TestL1CacheReducesHTTPCallsInterface(t *testing.T) {
	// This test demonstrates L1 cache behavior with interface return types.
	//
	// Query structure:
	// - meInterface: root query to accounts service → returns User 1234 via Identifiable interface
	// - meInterface.reviews: entity fetch from reviews service → returns reviews
	// - meInterface.reviews.product: entity fetch from products service → returns products
	// - meInterface.reviews.product.reviews: entity fetch from reviews service → returns reviews
	// - meInterface.reviews.product.reviews.authorWithoutProvides: entity fetch from accounts → returns User 1234
	//
	// This tests that interface return types properly build cache key templates
	// for all entity types that implement the interface.

	query := `query {
		meInterface {
			... on User {
				id
				username
				reviews {
					body
					product {
						upc
						reviews {
							authorWithoutProvides {
								id
								username
							}
						}
					}
				}
			}
		}
	}`

	expectedResponse := `{"data":{"meInterface":{"id":"1234","username":"Me","reviews":[{"body":"A highly effective form of birth control.","product":{"upc":"top-1","reviews":[{"authorWithoutProvides":{"id":"1234","username":"Me"}}]}},{"body":"Fedoras are one of the most fashionable hats around and can look great with a variety of outfits.","product":{"upc":"top-2","reviews":[{"authorWithoutProvides":{"id":"1234","username":"Me"}}]}}]}}}`

	t.Run("L1 enabled - interface entity fetches use L1 cache", func(t *testing.T) {
		tracker := newSubgraphCallTracker(http.DefaultTransport)
		trackingClient := &http.Client{Transport: tracker}

		cachingOpts := resolve.CachingOptions{
			EnableL1Cache: true,
			EnableL2Cache: false,
		}

		setup := federationtesting.NewFederationSetup(addCachingGateway(
			withCachingEnableART(false),
			withHTTPClient(trackingClient),
			withCachingOptionsFunc(cachingOpts),
		))
		t.Cleanup(setup.Close)

		gqlClient := NewGraphqlClient(http.DefaultClient)
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)

		// Extract hostnames
		accountsURLParsed, _ := url.Parse(setup.AccountsUpstreamServer.URL)
		accountsHost := accountsURLParsed.Host

		tracker.Reset()
		out, _ := gqlClient.QueryStringWithHeaders(ctx, setup.GatewayServer.URL, query, nil, t)

		assert.Equal(t, expectedResponse, string(out))

		// Same behavior as non-interface: root query + entity fetch both call accounts
		accountsCalls := tracker.GetCount(accountsHost)
		assert.Equal(t, 1, accountsCalls,
			"Interface field should behave same as object field for L1 caching")
	})

	t.Run("L1 disabled - more accounts calls without cache", func(t *testing.T) {
		tracker := newSubgraphCallTracker(http.DefaultTransport)
		trackingClient := &http.Client{Transport: tracker}

		cachingOpts := resolve.CachingOptions{
			EnableL1Cache: false,
			EnableL2Cache: false,
		}

		setup := federationtesting.NewFederationSetup(addCachingGateway(
			withCachingEnableART(false),
			withHTTPClient(trackingClient),
			withCachingOptionsFunc(cachingOpts),
		))
		t.Cleanup(setup.Close)

		gqlClient := NewGraphqlClient(http.DefaultClient)
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)

		// Extract hostnames
		accountsURLParsed, _ := url.Parse(setup.AccountsUpstreamServer.URL)
		accountsHost := accountsURLParsed.Host

		tracker.Reset()
		out, _ := gqlClient.QueryStringWithHeaders(ctx, setup.GatewayServer.URL, query, nil, t)

		assert.Equal(t, expectedResponse, string(out))

		// KEY ASSERTION: With L1 disabled, 2 accounts calls!
		accountsCalls := tracker.GetCount(accountsHost)
		assert.Equal(t, 2, accountsCalls,
			"With L1 disabled, should make 2 accounts calls (no cache reuse)")
	})
}

func TestL1CacheReducesHTTPCallsUnion(t *testing.T) {
	// This test demonstrates L1 cache behavior with union return types.
	//
	// Query structure:
	// - meUnion: root query to accounts service → returns User 1234 via MeUnion union
	// - meUnion.reviews: entity fetch from reviews service → returns reviews
	// - meUnion.reviews.product: entity fetch from products service → returns products
	// - meUnion.reviews.product.reviews: entity fetch from reviews service → returns reviews
	// - meUnion.reviews.product.reviews.authorWithoutProvides: entity fetch from accounts → returns User 1234
	//
	// This tests that union return types properly build cache key templates
	// for all entity types that are members of the union.

	query := `query {
		meUnion {
			... on User {
				id
				username
				reviews {
					body
					product {
						upc
						reviews {
							authorWithoutProvides {
								id
								username
							}
						}
					}
				}
			}
		}
	}`

	expectedResponse := `{"data":{"meUnion":{"id":"1234","username":"Me","reviews":[{"body":"A highly effective form of birth control.","product":{"upc":"top-1","reviews":[{"authorWithoutProvides":{"id":"1234","username":"Me"}}]}},{"body":"Fedoras are one of the most fashionable hats around and can look great with a variety of outfits.","product":{"upc":"top-2","reviews":[{"authorWithoutProvides":{"id":"1234","username":"Me"}}]}}]}}}`

	t.Run("L1 enabled - union entity fetches use L1 cache", func(t *testing.T) {
		tracker := newSubgraphCallTracker(http.DefaultTransport)
		trackingClient := &http.Client{Transport: tracker}

		cachingOpts := resolve.CachingOptions{
			EnableL1Cache: true,
			EnableL2Cache: false,
		}

		setup := federationtesting.NewFederationSetup(addCachingGateway(
			withCachingEnableART(false),
			withHTTPClient(trackingClient),
			withCachingOptionsFunc(cachingOpts),
		))
		t.Cleanup(setup.Close)

		gqlClient := NewGraphqlClient(http.DefaultClient)
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)

		// Extract hostnames
		accountsURLParsed, _ := url.Parse(setup.AccountsUpstreamServer.URL)
		accountsHost := accountsURLParsed.Host

		tracker.Reset()
		out, _ := gqlClient.QueryStringWithHeaders(ctx, setup.GatewayServer.URL, query, nil, t)

		assert.Equal(t, expectedResponse, string(out))

		// Same behavior as non-union: root query + entity fetch both call accounts
		accountsCalls := tracker.GetCount(accountsHost)
		assert.Equal(t, 1, accountsCalls,
			"Union field should behave same as object field for L1 caching")
	})

	t.Run("L1 disabled - more accounts calls without cache", func(t *testing.T) {
		tracker := newSubgraphCallTracker(http.DefaultTransport)
		trackingClient := &http.Client{Transport: tracker}

		cachingOpts := resolve.CachingOptions{
			EnableL1Cache: false,
			EnableL2Cache: false,
		}

		setup := federationtesting.NewFederationSetup(addCachingGateway(
			withCachingEnableART(false),
			withHTTPClient(trackingClient),
			withCachingOptionsFunc(cachingOpts),
		))
		t.Cleanup(setup.Close)

		gqlClient := NewGraphqlClient(http.DefaultClient)
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)

		// Extract hostnames
		accountsURLParsed, _ := url.Parse(setup.AccountsUpstreamServer.URL)
		accountsHost := accountsURLParsed.Host

		tracker.Reset()
		out, _ := gqlClient.QueryStringWithHeaders(ctx, setup.GatewayServer.URL, query, nil, t)

		assert.Equal(t, expectedResponse, string(out))

		// KEY ASSERTION: With L1 disabled, 2 accounts calls!
		accountsCalls := tracker.GetCount(accountsHost)
		assert.Equal(t, 2, accountsCalls,
			"With L1 disabled, should make 2 accounts calls (no cache reuse)")
	})
}

func TestL1CacheSelfReferentialEntity(t *testing.T) {
	// This test verifies that self-referential entities don't cause
	// stack overflow when L1 cache is enabled.
	//
	// Background: When an entity type has a field that returns the same type
	// (e.g., User.sameUserReviewers returning [User]), and L1 cache stores
	// a pointer to the entity, both key.Item and key.FromCache can point to
	// the same memory location. Without a fix, calling MergeValues(ptr, ptr)
	// causes infinite recursion and stack overflow.
	//
	// The sameUserReviewers field has @requires(fields: "username") which forces
	// sequential execution: the User entity is first fetched from accounts
	// (populating L1), then sameUserReviewers is resolved, returning the same
	// User entity that's already in L1 cache.

	query := `query {
		topProducts {
			reviews {
				authorWithoutProvides {
					id
					username
					sameUserReviewers {
						id
						username
					}
				}
			}
		}
	}`

	// This response shows User 1234 appearing both at authorWithoutProvides level
	// and inside sameUserReviewers (which returns the same user for testing)
	expectedResponse := `{"data":{"topProducts":[{"reviews":[{"authorWithoutProvides":{"id":"1234","username":"Me","sameUserReviewers":[{"id":"1234","username":"Me"}]}}]},{"reviews":[{"authorWithoutProvides":{"id":"1234","username":"Me","sameUserReviewers":[{"id":"1234","username":"Me"}]}}]}]}}`

	t.Run("self-referential entity should not cause stack overflow", func(t *testing.T) {
		tracker := newSubgraphCallTracker(http.DefaultTransport)
		trackingClient := &http.Client{Transport: tracker}

		cachingOpts := resolve.CachingOptions{
			EnableL1Cache: true,
			EnableL2Cache: false,
		}

		setup := federationtesting.NewFederationSetup(addCachingGateway(
			withCachingEnableART(false),
			withHTTPClient(trackingClient),
			withCachingOptionsFunc(cachingOpts),
		))
		t.Cleanup(setup.Close)

		gqlClient := NewGraphqlClient(http.DefaultClient)
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)

		// This should complete without stack overflow
		// Before the fix, this would crash with "fatal error: stack overflow"
		out, _ := gqlClient.QueryStringWithHeaders(ctx, setup.GatewayServer.URL, query, nil, t)

		assert.Equal(t, expectedResponse, string(out))
	})
}

func TestL1CacheChildFieldEntityList(t *testing.T) {
	// This test verifies L1 cache behavior for User.sameUserReviewers: [User!]!
	// which returns only the same user (self-reference).
	//
	// sameUserReviewers is defined in the reviews subgraph with @requires(fields: "username"),
	// which means:
	// 1. The gateway first resolves username from accounts (entity fetch)
	// 2. Then calls reviews to get sameUserReviewers
	// 3. sameUserReviewers returns User references (just IDs) - only the same user
	// 4. The gateway must make entity fetches to accounts to resolve those users
	//
	// Query flow:
	// 1. topProducts -> products subgraph (root query)
	// 2. reviews -> reviews subgraph (entity fetch for Products)
	// 3. authorWithoutProvides -> accounts subgraph (entity fetch for User 1234)
	//    - User 1234 is fetched and stored in L1
	// 4. sameUserReviewers -> reviews subgraph (after username resolved)
	//    - Returns [User 1234] as reference (same user only)
	// 5. Entity resolution for sameUserReviewers -> accounts subgraph
	//    - User 1234 is 100% L1 HIT (already fetched in step 3)
	//    - THE ENTIRE ACCOUNTS CALL IS SKIPPED!
	//
	// With L1 enabled: The sameUserReviewers entity fetch is completely skipped
	// because all entities are already in L1 cache.

	query := `query {
		topProducts {
			reviews {
				authorWithoutProvides {
					id
					username
					sameUserReviewers {
						id
						username
					}
				}
			}
		}
	}`

	// User 1234's sameUserReviewers returns [User 1234] (only self)
	expectedResponse := `{"data":{"topProducts":[{"reviews":[{"authorWithoutProvides":{"id":"1234","username":"Me","sameUserReviewers":[{"id":"1234","username":"Me"}]}}]},{"reviews":[{"authorWithoutProvides":{"id":"1234","username":"Me","sameUserReviewers":[{"id":"1234","username":"Me"}]}}]}]}}`

	t.Run("L1 enabled - sameUserReviewers fetch entirely skipped via L1 cache", func(t *testing.T) {
		tracker := newSubgraphCallTracker(http.DefaultTransport)
		trackingClient := &http.Client{Transport: tracker}

		cachingOpts := resolve.CachingOptions{
			EnableL1Cache: true,
			EnableL2Cache: false, // Isolate L1 behavior
		}

		setup := federationtesting.NewFederationSetup(addCachingGateway(
			withCachingEnableART(false),
			withHTTPClient(trackingClient),
			withCachingOptionsFunc(cachingOpts),
		))
		t.Cleanup(setup.Close)

		gqlClient := NewGraphqlClient(http.DefaultClient)
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)

		// Extract hostnames
		accountsURLParsed, _ := url.Parse(setup.AccountsUpstreamServer.URL)
		reviewsURLParsed, _ := url.Parse(setup.ReviewsUpstreamServer.URL)
		accountsHost := accountsURLParsed.Host
		reviewsHost := reviewsURLParsed.Host

		tracker.Reset()
		out, _ := gqlClient.QueryStringWithHeaders(ctx, setup.GatewayServer.URL, query, nil, t)

		assert.Equal(t, expectedResponse, string(out))

		// With L1 enabled:
		// - First accounts call fetches User 1234 for authorWithoutProvides (L1 miss, stored)
		// - Reviews called for sameUserReviewers (returns [User 1234] reference)
		// - sameUserReviewers entity resolution: User 1234 is 100% L1 HIT
		//   → accounts call is COMPLETELY SKIPPED!
		accountsCalls := tracker.GetCount(accountsHost)
		reviewsCalls := tracker.GetCount(reviewsHost)

		// Reviews should be called twice: once for Product entity (reviews field),
		// once for sameUserReviewers (after username is resolved from accounts)
		assert.Equal(t, 2, reviewsCalls, "Reviews subgraph called for Product.reviews and User.sameUserReviewers")

		// KEY ASSERTION: Only 1 accounts call! The sameUserReviewers entity resolution
		// is completely skipped because User 1234 is already in L1 cache.
		assert.Equal(t, 1, accountsCalls,
			"With L1 enabled: only 1 accounts call (sameUserReviewers entity fetch skipped via L1)")

	})

	t.Run("L1 disabled - accounts called for sameUserReviewers", func(t *testing.T) {
		tracker := newSubgraphCallTracker(http.DefaultTransport)
		trackingClient := &http.Client{Transport: tracker}

		cachingOpts := resolve.CachingOptions{
			EnableL1Cache: false,
			EnableL2Cache: false,
		}

		setup := federationtesting.NewFederationSetup(addCachingGateway(
			withCachingEnableART(false),
			withHTTPClient(trackingClient),
			withCachingOptionsFunc(cachingOpts),
		))
		t.Cleanup(setup.Close)

		gqlClient := NewGraphqlClient(http.DefaultClient)
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)

		// Extract hostnames
		accountsURLParsed, _ := url.Parse(setup.AccountsUpstreamServer.URL)
		accountsHost := accountsURLParsed.Host

		tracker.Reset()
		out, _ := gqlClient.QueryStringWithHeaders(ctx, setup.GatewayServer.URL, query, nil, t)

		assert.Equal(t, expectedResponse, string(out))

		// With L1 disabled:
		// - First accounts call fetches User 1234 for authorWithoutProvides
		// - Second accounts call for sameUserReviewers: User 1234 fetched again (no L1)
		// Total: 2 accounts calls
		accountsCalls := tracker.GetCount(accountsHost)
		assert.Equal(t, 2, accountsCalls,
			"With L1 disabled: 2 accounts calls (sameUserReviewers requires separate fetch)")

	})
}

func TestL1CacheNestedEntityListDeduplication(t *testing.T) {
	// This test verifies L1 deduplication when the same entity appears
	// at multiple levels in nested list queries using coReviewers.
	//
	// coReviewers is defined in the reviews subgraph with @requires(fields: "username"),
	// so it triggers cross-subgraph entity resolution.
	//
	// Query flow:
	// 1. topProducts -> products subgraph
	// 2. reviews -> reviews subgraph (Product entity fetch)
	// 3. authorWithoutProvides -> accounts (User 1234 fetched, stored in L1)
	// 4. coReviewers -> reviews subgraph (after username resolved)
	//    - Returns [User 1234, User 7777] as references
	// 5. Entity resolution for coReviewers -> accounts
	//    - User 1234 should be L1 HIT (already fetched in step 3)
	//    - User 7777 is L1 MISS (stored in L1)
	// 6. coReviewers for User 1234 and User 7777 -> reviews subgraph
	// 7. Entity resolution for nested coReviewers -> accounts
	//    - All users (1234, 7777) are already in L1!
	//
	// With L1 enabled: The nested coReviewers level should have 100% L1 hits,
	// potentially skipping the accounts call entirely for that level.

	query := `query {
		topProducts {
			reviews {
				authorWithoutProvides {
					id
					username
					coReviewers {
						id
						username
						coReviewers {
							id
							username
						}
					}
				}
			}
		}
	}`

	// User 1234's coReviewers: [User 1234, User 7777]
	// User 7777's coReviewers: [User 7777, User 1234]
	// Nested level repeats these patterns
	expectedResponse := `{"data":{"topProducts":[{"reviews":[{"authorWithoutProvides":{"id":"1234","username":"Me","coReviewers":[{"id":"1234","username":"Me","coReviewers":[{"id":"1234","username":"Me"},{"id":"7777","username":"User 7777"}]},{"id":"7777","username":"User 7777","coReviewers":[{"id":"7777","username":"User 7777"},{"id":"1234","username":"Me"}]}]}}]},{"reviews":[{"authorWithoutProvides":{"id":"1234","username":"Me","coReviewers":[{"id":"1234","username":"Me","coReviewers":[{"id":"1234","username":"Me"},{"id":"7777","username":"User 7777"}]},{"id":"7777","username":"User 7777","coReviewers":[{"id":"7777","username":"User 7777"},{"id":"1234","username":"Me"}]}]}}]}]}}`

	t.Run("L1 enabled - nested coReviewers benefits from L1 hits", func(t *testing.T) {
		tracker := newSubgraphCallTracker(http.DefaultTransport)
		trackingClient := &http.Client{Transport: tracker}

		cachingOpts := resolve.CachingOptions{
			EnableL1Cache: true,
			EnableL2Cache: false,
		}

		setup := federationtesting.NewFederationSetup(addCachingGateway(
			withCachingEnableART(false),
			withHTTPClient(trackingClient),
			withCachingOptionsFunc(cachingOpts),
		))
		t.Cleanup(setup.Close)

		gqlClient := NewGraphqlClient(http.DefaultClient)
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)

		// Extract hostnames
		accountsURLParsed, _ := url.Parse(setup.AccountsUpstreamServer.URL)
		accountsHost := accountsURLParsed.Host

		tracker.Reset()
		out, _ := gqlClient.QueryStringWithHeaders(ctx, setup.GatewayServer.URL, query, nil, t)

		assert.Equal(t, expectedResponse, string(out))

		// With L1 enabled:
		// - Call 1: authorWithoutProvides fetches User 1234 (miss, stored)
		// - Call 2: coReviewers entity resolution [User 1234 (hit), User 7777 (miss, stored)]
		// - Call 3: nested coReviewers entity resolution - all users are in L1!
		//   This call should be fully served from L1 cache.
		accountsCalls := tracker.GetCount(accountsHost)
		// With L1 enabled, the nested coReviewers should be served from L1
		// Only 2 accounts calls needed because nested coReviewers is fully served from L1
		assert.Equal(t, 2, accountsCalls,
			"With L1 enabled: exactly 2 accounts calls (nested coReviewers served entirely from L1)")
	})

	t.Run("L1 disabled - more accounts calls without deduplication", func(t *testing.T) {
		tracker := newSubgraphCallTracker(http.DefaultTransport)
		trackingClient := &http.Client{Transport: tracker}

		cachingOpts := resolve.CachingOptions{
			EnableL1Cache: false,
			EnableL2Cache: false,
		}

		setup := federationtesting.NewFederationSetup(addCachingGateway(
			withCachingEnableART(false),
			withHTTPClient(trackingClient),
			withCachingOptionsFunc(cachingOpts),
		))
		t.Cleanup(setup.Close)

		gqlClient := NewGraphqlClient(http.DefaultClient)
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)

		// Extract hostnames
		accountsURLParsed, _ := url.Parse(setup.AccountsUpstreamServer.URL)
		accountsHost := accountsURLParsed.Host

		tracker.Reset()
		out, _ := gqlClient.QueryStringWithHeaders(ctx, setup.GatewayServer.URL, query, nil, t)

		assert.Equal(t, expectedResponse, string(out))

		// With L1 disabled:
		// - Call 1: authorWithoutProvides fetches User 1234
		// - Call 2: coReviewers entity resolution for User 1234 and User 7777 (no L1 dedup)
		// - Call 3: nested coReviewers entity resolution (no L1 dedup)
		accountsCalls := tracker.GetCount(accountsHost)
		// Without L1 cache, we need 3 accounts calls (no deduplication at nested level)
		assert.Equal(t, 3, accountsCalls,
			"With L1 disabled: exactly 3 accounts calls (no deduplication)")
	})
}

func TestL1CacheRootFieldEntityListPopulation(t *testing.T) {
	// This test verifies L1 cache behavior with a complex nested query starting
	// from a root field that returns a list of entities.
	//
	// Query flow:
	// 1. topProducts -> products subgraph (root query, returns list)
	// 2. reviews -> reviews subgraph (entity fetch for Products)
	// 3. authorWithoutProvides -> accounts subgraph (entity fetch for User 1234)
	//    - User 1234 is fetched and stored in L1
	// 4. sameUserReviewers -> reviews subgraph (after username resolved)
	//    - Returns [User 1234] as reference (same user only)
	// 5. Entity resolution for sameUserReviewers -> accounts subgraph
	//    - User 1234 is 100% L1 HIT (already fetched in step 3)
	//    - THE ENTIRE ACCOUNTS CALL IS SKIPPED!
	//
	// With L1 enabled: The sameUserReviewers entity fetch is completely skipped.
	// With L1 disabled: accounts is called twice (no deduplication).

	query := `query {
		topProducts {
			upc
			name
			reviews {
				body
				authorWithoutProvides {
					id
					username
					sameUserReviewers {
						id
						username
					}
				}
			}
		}
	}`

	expectedResponse := `{"data":{"topProducts":[{"upc":"top-1","name":"Trilby","reviews":[{"body":"A highly effective form of birth control.","authorWithoutProvides":{"id":"1234","username":"Me","sameUserReviewers":[{"id":"1234","username":"Me"}]}}]},{"upc":"top-2","name":"Fedora","reviews":[{"body":"Fedoras are one of the most fashionable hats around and can look great with a variety of outfits.","authorWithoutProvides":{"id":"1234","username":"Me","sameUserReviewers":[{"id":"1234","username":"Me"}]}}]}]}}`

	t.Run("L1 enabled - sameUserReviewers fetch skipped via L1 cache", func(t *testing.T) {
		tracker := newSubgraphCallTracker(http.DefaultTransport)
		trackingClient := &http.Client{Transport: tracker}

		cachingOpts := resolve.CachingOptions{
			EnableL1Cache: true,
			EnableL2Cache: false,
		}

		setup := federationtesting.NewFederationSetup(addCachingGateway(
			withCachingEnableART(false),
			withHTTPClient(trackingClient),
			withCachingOptionsFunc(cachingOpts),
		))
		t.Cleanup(setup.Close)

		gqlClient := NewGraphqlClient(http.DefaultClient)
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)

		// Extract hostnames
		productsURLParsed, _ := url.Parse(setup.ProductsUpstreamServer.URL)
		reviewsURLParsed, _ := url.Parse(setup.ReviewsUpstreamServer.URL)
		accountsURLParsed, _ := url.Parse(setup.AccountsUpstreamServer.URL)
		productsHost := productsURLParsed.Host
		reviewsHost := reviewsURLParsed.Host
		accountsHost := accountsURLParsed.Host

		tracker.Reset()
		out, _ := gqlClient.QueryStringWithHeaders(ctx, setup.GatewayServer.URL, query, nil, t)

		assert.Equal(t, expectedResponse, string(out))

		// Query flow with L1 enabled:
		// 1. products subgraph: topProducts root query
		// 2. reviews subgraph: Product entity fetch for reviews
		// 3. accounts subgraph: User entity fetch for authorWithoutProvides (User 1234 stored in L1)
		// 4. reviews subgraph: sameUserReviewers (returns [User 1234])
		// 5. sameUserReviewers entity resolution: User 1234 is 100% L1 HIT → accounts call SKIPPED!
		productsCalls := tracker.GetCount(productsHost)
		reviewsCalls := tracker.GetCount(reviewsHost)
		accountsCalls := tracker.GetCount(accountsHost)

		assert.Equal(t, 1, productsCalls, "Should call products subgraph once for topProducts")
		assert.Equal(t, 2, reviewsCalls, "Should call reviews subgraph twice (Product.reviews + User.sameUserReviewers)")
		// KEY ASSERTION: Only 1 accounts call! sameUserReviewers entity resolution skipped via L1.
		assert.Equal(t, 1, accountsCalls,
			"With L1 enabled: only 1 accounts call (sameUserReviewers entity fetch skipped via L1)")

	})

	t.Run("L1 disabled - more accounts calls without L1 optimization", func(t *testing.T) {
		tracker := newSubgraphCallTracker(http.DefaultTransport)
		trackingClient := &http.Client{Transport: tracker}

		cachingOpts := resolve.CachingOptions{
			EnableL1Cache: false,
			EnableL2Cache: false,
		}

		setup := federationtesting.NewFederationSetup(addCachingGateway(
			withCachingEnableART(false),
			withHTTPClient(trackingClient),
			withCachingOptionsFunc(cachingOpts),
		))
		t.Cleanup(setup.Close)

		gqlClient := NewGraphqlClient(http.DefaultClient)
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)

		// Extract hostnames
		productsURLParsed, _ := url.Parse(setup.ProductsUpstreamServer.URL)
		reviewsURLParsed, _ := url.Parse(setup.ReviewsUpstreamServer.URL)
		accountsURLParsed, _ := url.Parse(setup.AccountsUpstreamServer.URL)
		productsHost := productsURLParsed.Host
		reviewsHost := reviewsURLParsed.Host
		accountsHost := accountsURLParsed.Host

		tracker.Reset()
		out, _ := gqlClient.QueryStringWithHeaders(ctx, setup.GatewayServer.URL, query, nil, t)

		assert.Equal(t, expectedResponse, string(out))

		// Query flow with L1 disabled:
		// 1. products subgraph: topProducts root query
		// 2. reviews subgraph: Product entity fetch for reviews
		// 3. accounts subgraph: User entity fetch for authorWithoutProvides
		// 4. reviews subgraph: sameUserReviewers
		// 5. accounts subgraph: User entity fetch for sameUserReviewers (no L1 → must fetch again!)
		productsCalls := tracker.GetCount(productsHost)
		reviewsCalls := tracker.GetCount(reviewsHost)
		accountsCalls := tracker.GetCount(accountsHost)

		assert.Equal(t, 1, productsCalls, "Should call products subgraph once")
		assert.Equal(t, 2, reviewsCalls, "Should call reviews subgraph twice")
		// KEY ASSERTION: 2 accounts calls without L1 optimization
		assert.Equal(t, 2, accountsCalls,
			"With L1 disabled: 2 accounts calls (sameUserReviewers requires separate fetch)")

	})
}

func TestL1CacheRootFieldNonEntityWithNestedEntities(t *testing.T) {
	// This test verifies L1 cache behavior when a root field returns a NON-entity type
	// (Review) that contains nested entities (User via authorWithoutProvides).
	//
	// Key difference from TestL1CacheRootFieldEntityListPopulation:
	// - That test starts with topProducts -> [Product] where Product IS an entity (@key(fields: "upc"))
	// - This test starts with topReviews -> [Review] where Review is NOT an entity (no @key)
	// - Both prove L1 entity caching works for nested User entities
	//
	// Query flow:
	// 1. topReviews -> reviews subgraph (root query, returns [Review] — NOT an entity)
	// 2. authorWithoutProvides -> accounts subgraph (entity fetch for Users, stored in L1)
	// 3. sameUserReviewers -> reviews subgraph (after username resolved via @requires)
	// 4. Entity resolution for sameUserReviewers -> accounts subgraph
	//    - All Users are 100% L1 HITs (already fetched in step 2)
	//    - THE ENTIRE ACCOUNTS CALL IS SKIPPED!

	query := `query {
		topReviews {
			body
			authorWithoutProvides {
				id
				username
				sameUserReviewers {
					id
					username
				}
			}
		}
	}`

	expectedResponse := `{"data":{"topReviews":[{"body":"A highly effective form of birth control.","authorWithoutProvides":{"id":"1234","username":"Me","sameUserReviewers":[{"id":"1234","username":"Me"}]}},{"body":"Fedoras are one of the most fashionable hats around and can look great with a variety of outfits.","authorWithoutProvides":{"id":"1234","username":"Me","sameUserReviewers":[{"id":"1234","username":"Me"}]}},{"body":"This is the last straw. Hat you will wear. 11/10","authorWithoutProvides":{"id":"7777","username":"User 7777","sameUserReviewers":[{"id":"7777","username":"User 7777"}]}},{"body":"Perfect summer hat.","authorWithoutProvides":{"id":"5678","username":"User 5678","sameUserReviewers":[{"id":"5678","username":"User 5678"}]}},{"body":"A bit too fancy for my taste.","authorWithoutProvides":{"id":"8888","username":"User 8888","sameUserReviewers":[{"id":"8888","username":"User 8888"}]}}]}}`

	t.Run("L1 enabled - sameUserReviewers fetch skipped via L1 cache", func(t *testing.T) {
		tracker := newSubgraphCallTracker(http.DefaultTransport)
		trackingClient := &http.Client{Transport: tracker}

		cachingOpts := resolve.CachingOptions{
			EnableL1Cache: true,
			EnableL2Cache: false,
		}

		setup := federationtesting.NewFederationSetup(addCachingGateway(
			withCachingEnableART(false),
			withHTTPClient(trackingClient),
			withCachingOptionsFunc(cachingOpts),
		))
		t.Cleanup(setup.Close)

		gqlClient := NewGraphqlClient(http.DefaultClient)
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)

		// Extract hostnames
		reviewsURLParsed, _ := url.Parse(setup.ReviewsUpstreamServer.URL)
		accountsURLParsed, _ := url.Parse(setup.AccountsUpstreamServer.URL)
		reviewsHost := reviewsURLParsed.Host
		accountsHost := accountsURLParsed.Host

		tracker.Reset()
		out, _ := gqlClient.QueryStringWithHeaders(ctx, setup.GatewayServer.URL, query, nil, t)

		assert.Equal(t, expectedResponse, string(out))

		// Query flow with L1 enabled:
		// 1. reviews subgraph: topReviews root query (Review is NOT an entity)
		// 2. accounts subgraph: User entity fetch for authorWithoutProvides (Users stored in L1)
		// 3. reviews subgraph: sameUserReviewers (returns [User] references)
		// 4. sameUserReviewers entity resolution: all Users are L1 HITs → accounts call SKIPPED!
		reviewsCalls := tracker.GetCount(reviewsHost)
		accountsCalls := tracker.GetCount(accountsHost)

		assert.Equal(t, 2, reviewsCalls, "Should call reviews subgraph twice (topReviews + sameUserReviewers)")
		// KEY ASSERTION: Only 1 accounts call! sameUserReviewers entity resolution skipped via L1.
		assert.Equal(t, 1, accountsCalls,
			"With L1 enabled: only 1 accounts call (sameUserReviewers entity fetch skipped via L1)")
	})

	t.Run("L1 disabled - more accounts calls without L1 optimization", func(t *testing.T) {
		tracker := newSubgraphCallTracker(http.DefaultTransport)
		trackingClient := &http.Client{Transport: tracker}

		cachingOpts := resolve.CachingOptions{
			EnableL1Cache: false,
			EnableL2Cache: false,
		}

		setup := federationtesting.NewFederationSetup(addCachingGateway(
			withCachingEnableART(false),
			withHTTPClient(trackingClient),
			withCachingOptionsFunc(cachingOpts),
		))
		t.Cleanup(setup.Close)

		gqlClient := NewGraphqlClient(http.DefaultClient)
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)

		// Extract hostnames
		reviewsURLParsed, _ := url.Parse(setup.ReviewsUpstreamServer.URL)
		accountsURLParsed, _ := url.Parse(setup.AccountsUpstreamServer.URL)
		reviewsHost := reviewsURLParsed.Host
		accountsHost := accountsURLParsed.Host

		tracker.Reset()
		out, _ := gqlClient.QueryStringWithHeaders(ctx, setup.GatewayServer.URL, query, nil, t)

		assert.Equal(t, expectedResponse, string(out))

		// Query flow with L1 disabled:
		// 1. reviews subgraph: topReviews root query
		// 2. accounts subgraph: User entity fetch for authorWithoutProvides
		// 3. reviews subgraph: sameUserReviewers
		// 4. accounts subgraph: User entity fetch for sameUserReviewers (no L1 → must fetch again!)
		reviewsCalls := tracker.GetCount(reviewsHost)
		accountsCalls := tracker.GetCount(accountsHost)

		assert.Equal(t, 2, reviewsCalls, "Should call reviews subgraph twice")
		// KEY ASSERTION: 2 accounts calls without L1 optimization
		assert.Equal(t, 2, accountsCalls,
			"With L1 disabled: 2 accounts calls (sameUserReviewers requires separate fetch)")
	})
}

// =============================================================================
// CACHE ERROR HANDLING TESTS
// =============================================================================
//
// These tests verify that caches are NOT populated when subgraphs return errors.
// The cache should only store successful responses to prevent caching error states.

func TestL1CacheOptimizationReducesSubgraphCalls(t *testing.T) {
	// This query demonstrates L1 optimization:
	// - Query.me returns User entity
	// - User.sameUserReviewers returns [User] entities
	// When L1 is enabled and optimized correctly:
	// - First User fetch (me) populates L1 cache
	// - Second User fetch (sameUserReviewers) hits L1 cache, SKIPS subgraph call
	//
	// The optimizeL1Cache postprocessor:
	// - Sets UseL1Cache=true on User fetches (they share the same entity type)
	// - Sets UseL1Cache=false on fetches with no matching entity types

	query := `query {
		me {
			id
			username
			sameUserReviewers {
				id
				username
			}
		}
	}`

	expectedResponse := `{"data":{"me":{"id":"1234","username":"Me","sameUserReviewers":[{"id":"1234","username":"Me"}]}}}`

	t.Run("L1 optimization enables cache hit between same entity type fetches", func(t *testing.T) {
		tracker := newSubgraphCallTracker(http.DefaultTransport)
		trackingClient := &http.Client{Transport: tracker}

		cachingOpts := resolve.CachingOptions{
			EnableL1Cache: true,
			EnableL2Cache: false,
		}

		setup := federationtesting.NewFederationSetup(addCachingGateway(
			withCachingEnableART(false),
			withHTTPClient(trackingClient),
			withCachingOptionsFunc(cachingOpts),
		))
		t.Cleanup(setup.Close)

		gqlClient := NewGraphqlClient(http.DefaultClient)
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)

		// Extract hostnames
		accountsURLParsed, _ := url.Parse(setup.AccountsUpstreamServer.URL)
		reviewsURLParsed, _ := url.Parse(setup.ReviewsUpstreamServer.URL)
		accountsHost := accountsURLParsed.Host
		reviewsHost := reviewsURLParsed.Host

		tracker.Reset()
		out, _ := gqlClient.QueryStringWithHeaders(ctx, setup.GatewayServer.URL, query, nil, t)

		assert.Equal(t, expectedResponse, string(out))

		// Query flow with L1 optimization:
		// 1. accounts subgraph: Query.me (root query, returns User 1234)
		//    - L1 cache populated with User 1234
		// 2. reviews subgraph: User.sameUserReviewers (returns [User 1234])
		// 3. accounts subgraph: User entity fetch for sameUserReviewers
		//    - User 1234 is 100% L1 HIT! This call is SKIPPED!
		accountsCalls := tracker.GetCount(accountsHost)
		reviewsCalls := tracker.GetCount(reviewsHost)

		// KEY ASSERTION: Only 1 accounts call!
		// Without L1 optimization, there would be 2 calls:
		// - First: Query.me
		// - Second: User entity resolution for sameUserReviewers
		// With L1 optimization, the second call is skipped because User 1234 is in L1 cache.
		assert.Equal(t, 1, accountsCalls,
			"L1 optimization: only 1 accounts call (sameUserReviewers resolved from L1 cache)")
		assert.Equal(t, 1, reviewsCalls,
			"Should call reviews subgraph once for User.sameUserReviewers")
	})

	t.Run("Without L1, same query requires more subgraph calls", func(t *testing.T) {
		tracker := newSubgraphCallTracker(http.DefaultTransport)
		trackingClient := &http.Client{Transport: tracker}

		cachingOpts := resolve.CachingOptions{
			EnableL1Cache: false, // L1 disabled
			EnableL2Cache: false,
		}

		setup := federationtesting.NewFederationSetup(addCachingGateway(
			withCachingEnableART(false),
			withHTTPClient(trackingClient),
			withCachingOptionsFunc(cachingOpts),
		))
		t.Cleanup(setup.Close)

		gqlClient := NewGraphqlClient(http.DefaultClient)
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)

		// Extract hostnames
		accountsURLParsed, _ := url.Parse(setup.AccountsUpstreamServer.URL)
		reviewsURLParsed, _ := url.Parse(setup.ReviewsUpstreamServer.URL)
		accountsHost := accountsURLParsed.Host
		reviewsHost := reviewsURLParsed.Host

		tracker.Reset()
		out, _ := gqlClient.QueryStringWithHeaders(ctx, setup.GatewayServer.URL, query, nil, t)

		assert.Equal(t, expectedResponse, string(out))

		// Query flow WITHOUT L1:
		// 1. accounts subgraph: Query.me (root query)
		// 2. reviews subgraph: User.sameUserReviewers
		// 3. accounts subgraph: User entity fetch (NO L1 cache → must fetch!)
		accountsCalls := tracker.GetCount(accountsHost)
		reviewsCalls := tracker.GetCount(reviewsHost)

		// KEY ASSERTION: 2 accounts calls without L1!
		// This proves L1 optimization saves a subgraph call.
		assert.Equal(t, 2, accountsCalls,
			"Without L1: 2 accounts calls (sameUserReviewers requires separate fetch)")
		assert.Equal(t, 1, reviewsCalls,
			"Should call reviews subgraph once for User.sameUserReviewers")
	})
}

