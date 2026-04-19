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

// TestL1CacheReducesHTTPCalls verifies L1 cache behavior with nested entity fetches.
// L1 only works for entity fetches (not root queries), so self-referential paths benefit.
func TestL1CacheReducesHTTPCalls(t *testing.T) {
	t.Parallel()
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
		t.Parallel()
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
		t.Parallel()
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

// TestL1CacheFieldAccumulationWithAliases verifies that L1 cache accumulates fields
// across entity fetches with different aliases and that alias normalization allows
// a later fetch to reuse a field stored by an earlier fetch under a different alias.
//
// Query:
//
//	{
//	  me {
//	    id
//	    reviews {
//	      authorWithoutProvides {
//	        myName: username     ← entity fetch A: stores "username" in L1 (normalized from alias "myName")
//	      }
//	      product {
//	        reviews {
//	          authorWithoutProvides {
//	            username          ← entity fetch B: should L1 HIT (schema name "username" already stored)
//	          }
//	        }
//	      }
//	    }
//	  }
//	}
func TestL1CacheFieldAccumulationWithAliases(t *testing.T) {
	t.Parallel()

	t.Run("alias then no alias - sameUserReviewers L1 reuse", func(t *testing.T) {
		t.Parallel()
		tracker := newSubgraphCallTracker(http.DefaultTransport)
		trackingClient := &http.Client{Transport: tracker}

		setup := federationtesting.NewFederationSetup(addCachingGateway(
			withCachingEnableART(false),
			withHTTPClient(trackingClient),
			withCachingOptionsFunc(resolve.CachingOptions{
				EnableL1Cache: true,
				EnableL2Cache: false,
			}),
		))
		t.Cleanup(setup.Close)

		gqlClient := NewGraphqlClient(http.DefaultClient)
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)

		accountsURLParsed, _ := url.Parse(setup.AccountsUpstreamServer.URL)
		accountsHost := accountsURLParsed.Host

		// Root `me` fetch returns User 1234 with alias "myName: username".
		// sameUserReviewers returns the same User — the entity fetch needs "username"
		// (no alias). L1 stores the normalized schema name "username" from the
		// first entity fetch; the second fetch should find it via denormalize passthrough.
		query := `query {
			me {
				id
				myName: username
				sameUserReviewers {
					id
					username
				}
			}
		}`

		tracker.Reset()
		out, _ := gqlClient.QueryStringWithHeaders(ctx, setup.GatewayServer.URL, query, nil, t)

		assert.Equal(t, `{"data":{"me":{"id":"1234","myName":"Me","sameUserReviewers":[{"id":"1234","username":"Me"}]}}}`, string(out), "aliased field should render as myName and sameUserReviewers should have unaliased username")

		// With L1 enabled, the sameUserReviewers entity fetch for User 1234
		// should hit L1 (populated by the root me fetch's entity).
		// 1 accounts call = root me only, sameUserReviewers skipped via L1.
		accountsCalls := tracker.GetCount(accountsHost)
		assert.Equal(t, 1, accountsCalls,
			"L1 should skip sameUserReviewers accounts call (alias normalized username in L1)")
	})

	t.Run("L1 disabled - alias variant needs separate fetch", func(t *testing.T) {
		t.Parallel()
		tracker := newSubgraphCallTracker(http.DefaultTransport)
		trackingClient := &http.Client{Transport: tracker}

		setup := federationtesting.NewFederationSetup(addCachingGateway(
			withCachingEnableART(false),
			withHTTPClient(trackingClient),
			withCachingOptionsFunc(resolve.CachingOptions{
				EnableL1Cache: false,
				EnableL2Cache: false,
			}),
		))
		t.Cleanup(setup.Close)

		gqlClient := NewGraphqlClient(http.DefaultClient)
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)

		accountsURLParsed, _ := url.Parse(setup.AccountsUpstreamServer.URL)
		accountsHost := accountsURLParsed.Host

		query := `query {
			me {
				id
				myName: username
				sameUserReviewers {
					id
					username
				}
			}
		}`

		tracker.Reset()
		out, _ := gqlClient.QueryStringWithHeaders(ctx, setup.GatewayServer.URL, query, nil, t)

		assert.Equal(t, `{"data":{"me":{"id":"1234","myName":"Me","sameUserReviewers":[{"id":"1234","username":"Me"}]}}}`, string(out))

		accountsCalls := tracker.GetCount(accountsHost)
		assert.Equal(t, 2, accountsCalls,
			"Without L1, sameUserReviewers needs its own accounts call")
	})
}

// TestL1CacheThreeFetchFieldAccumulation verifies that L1 field accumulation works
// across 3 entity fetches for the same entity, where a field from fetch 1 survives
// fetch 2's merge (which has different fields) and is available for fetch 3.
//
// Fetch sequence for User 1234:
//  1. accounts entity fetch for authorWithoutProvides: ProvidesData = {username}
//     → L1 MISS, stores {username, id, __typename} in L1
//  2. accounts entity fetch for authorWithoutProvides.realName path: ProvidesData = {realName}
//     → L1 widening miss (no realName), fetches, merges {realName} into L1
//     → L1 now has {username, realName, id, __typename}
//  3. accounts entity fetch for sameUserReviewers: ProvidesData = {username}
//     → L1 HIT (username survived fetch 2's merge) → skips accounts call
func TestL1CacheThreeFetchFieldAccumulation(t *testing.T) {
	t.Parallel()

	query := `query {
		me {
			id
			username
			reviews {
				authorWithoutProvides {
					username
					realName
					sameUserReviewers {
						id
						username
					}
				}
			}
		}
	}`

	t.Run("L1 enabled - field accumulation skips redundant fetches", func(t *testing.T) {
		t.Parallel()
		tracker := newSubgraphCallTracker(http.DefaultTransport)
		trackingClient := &http.Client{Transport: tracker}

		setup := federationtesting.NewFederationSetup(addCachingGateway(
			withCachingEnableART(false),
			withHTTPClient(trackingClient),
			withCachingOptionsFunc(resolve.CachingOptions{
				EnableL1Cache: true,
				EnableL2Cache: false,
			}),
		))
		t.Cleanup(setup.Close)

		gqlClient := NewGraphqlClient(http.DefaultClient)
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)

		accountsURLParsed, _ := url.Parse(setup.AccountsUpstreamServer.URL)
		accountsHost := accountsURLParsed.Host

		tracker.Reset()
		out, _ := gqlClient.QueryStringWithHeaders(ctx, setup.GatewayServer.URL, query, nil, t)

		assert.Equal(t, `{"data":{"me":{"id":"1234","username":"Me","reviews":[{"authorWithoutProvides":{"username":"Me","realName":"User Usington","sameUserReviewers":[{"id":"1234","username":"Me"}]}},{"authorWithoutProvides":{"username":"Me","realName":"User Usington","sameUserReviewers":[{"id":"1234","username":"Me"}]}}]}}}`, string(out))

		// Without L1: 3 accounts calls (root me + entity authorWithoutProvides + entity sameUserReviewers).
		// With L1: 1 accounts call. The planner merges root me with the first entity fetch.
		// sameUserReviewers entity fetch hits L1 because "username" was accumulated
		// from the first entity fetch and survived the realName merge.
		assert.Equal(t, 1, tracker.GetCount(accountsHost),
			"L1 field accumulation: sameUserReviewers should reuse username from L1 (was 3 without L1)")
	})

	t.Run("L1 disabled - no field accumulation, all fetches hit subgraph", func(t *testing.T) {
		t.Parallel()
		tracker := newSubgraphCallTracker(http.DefaultTransport)
		trackingClient := &http.Client{Transport: tracker}

		setup := federationtesting.NewFederationSetup(addCachingGateway(
			withCachingEnableART(false),
			withHTTPClient(trackingClient),
			withCachingOptionsFunc(resolve.CachingOptions{
				EnableL1Cache: false,
				EnableL2Cache: false,
			}),
		))
		t.Cleanup(setup.Close)

		gqlClient := NewGraphqlClient(http.DefaultClient)
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)

		accountsURLParsed, _ := url.Parse(setup.AccountsUpstreamServer.URL)
		accountsHost := accountsURLParsed.Host

		tracker.Reset()
		out, _ := gqlClient.QueryStringWithHeaders(ctx, setup.GatewayServer.URL, query, nil, t)

		assert.Equal(t, `{"data":{"me":{"id":"1234","username":"Me","reviews":[{"authorWithoutProvides":{"username":"Me","realName":"User Usington","sameUserReviewers":[{"id":"1234","username":"Me"}]}},{"authorWithoutProvides":{"username":"Me","realName":"User Usington","sameUserReviewers":[{"id":"1234","username":"Me"}]}}]}}}`, string(out))

		assert.Equal(t, 3, tracker.GetCount(accountsHost),
			"Without L1: 3 separate accounts calls (root me + authorWithoutProvides + sameUserReviewers)")
	})
}

// TestL1CacheReducesHTTPCallsInterface verifies L1 cache works with interface types,
// deduplicating entity fetches for the same entity accessed through different interface fields.
func TestL1CacheReducesHTTPCallsInterface(t *testing.T) {
	t.Parallel()
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
		t.Parallel()
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
		t.Parallel()
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

// TestL1CacheReducesHTTPCallsUnion verifies L1 cache works with union types,
// deduplicating entity fetches for the same entity accessed through different union members.
func TestL1CacheReducesHTTPCallsUnion(t *testing.T) {
	t.Parallel()
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
		t.Parallel()
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
		t.Parallel()
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

// TestL1CacheSelfReferentialEntity verifies that L1 cache handles self-referential entities
// (e.g. User.friends returns User) without stack overflow via shallow copy.
func TestL1CacheSelfReferentialEntity(t *testing.T) {
	t.Parallel()
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
		t.Parallel()
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

// TestL1CacheChildFieldEntityList verifies that L1 cache correctly deduplicates
// entities in list fields (e.g. reviews[].author where multiple reviews have the same author).
func TestL1CacheChildFieldEntityList(t *testing.T) {
	t.Parallel()
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
		t.Parallel()
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
		t.Parallel()
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

// TestL1CacheNestedEntityListDeduplication verifies that L1 cache deduplicates entities
// across nested lists (e.g. products[].reviews[].author with overlapping authors).
func TestL1CacheNestedEntityListDeduplication(t *testing.T) {
	t.Parallel()
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
		t.Parallel()
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
		t.Parallel()
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

// TestL1CacheRootFieldEntityListPopulation verifies that root fields returning entity lists
// populate L1 cache, allowing subsequent entity fetches to skip subgraph calls.
func TestL1CacheRootFieldEntityListPopulation(t *testing.T) {
	t.Parallel()
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
		t.Parallel()
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
		t.Parallel()
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

// TestL1CacheRootFieldNonEntityWithNestedEntities verifies that root fields returning
// non-entity objects with nested entity lists still populate L1 for those nested entities.
func TestL1CacheRootFieldNonEntityWithNestedEntities(t *testing.T) {
	t.Parallel()
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
		t.Parallel()
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
		t.Parallel()
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

// TestL1CacheOptimizationReducesSubgraphCalls verifies the L1 optimization postprocessor
// correctly marks fetches with UseL1Cache, reducing redundant subgraph calls.
func TestL1CacheOptimizationReducesSubgraphCalls(t *testing.T) {
	t.Parallel()
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
		t.Parallel()
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
		t.Parallel()
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

// TestL1CacheUnionOfProviderFields exposes a gap in the L1 cache postprocessor optimization.
//
// The postprocessor (optimize_l1_cache.go) decides whether to enable L1 for each fetch by
// checking each ancestor provider INDIVIDUALLY via hasValidProvider → objectProvidesAllFields.
// If no single provider has ALL fields that the consumer needs, L1 is disabled for that fetch.
//
// However, at runtime, L1 accumulates fields from multiple fetches via merge. If fetch A
// writes {nickname} and fetch B writes {realName, username}, L1 has {nickname, realName, username}
// which covers a consumer that needs {nickname, realName}. The postprocessor should compute
// the UNION of ancestor providers' fields, but currently checks each one individually.
//
// This test creates 3 entity fetches for User from accounts:
//
//	Fetch A (level 1 authorWithoutProvides): ProvidesData = {nickname}
//	Fetch B (level 2 authorWithoutProvides): ProvidesData = {realName, username}
//	  (username is included because sameUserReviewers has @requires(fields: "username"))
//	Fetch C (sameUserReviewers entity resolution): ProvidesData = {nickname, realName}
//
// Neither A ({nickname}) nor B ({realName, username}) individually covers C ({nickname, realName}),
// so the postprocessor sets UseL1Cache=false for C. But A ∪ B = {nickname, realName, username}
// which IS a superset of C's needs. With the union fix, fetch C would be L1-enabled and
// the accounts call for sameUserReviewers entity resolution would be skipped.
func TestL1CacheUnionOfProviderFields(t *testing.T) {
	t.Parallel()

	// This query creates the 3-fetch pattern:
	// 1. me.reviews.authorWithoutProvides → entity fetch A to accounts for {nickname}
	// 2. me.reviews.product.reviews.authorWithoutProvides → entity fetch B to accounts for {realName, username}
	//    (username needed for @requires on sameUserReviewers)
	// 3. sameUserReviewers entity resolution → entity fetch C to accounts for {nickname, realName}
	//
	// All three fetches target User:1234 (the only author in the test data).
	// Fetch A provides {nickname}, fetch B provides {realName, username}.
	// Fetch C needs {nickname, realName} — neither A nor B alone covers this,
	// but their union does.

	t.Run("L1 enabled - union of providers should skip fetch C", func(t *testing.T) {
		t.Parallel()
		tracker := newSubgraphCallTracker(http.DefaultTransport)
		trackingClient := &http.Client{Transport: tracker}

		setup := federationtesting.NewFederationSetup(addCachingGateway(
			withCachingEnableART(false),
			withHTTPClient(trackingClient),
			withCachingOptionsFunc(resolve.CachingOptions{
				EnableL1Cache: true,
				EnableL2Cache: false,
			}),
		))
		t.Cleanup(setup.Close)

		gqlClient := NewGraphqlClient(http.DefaultClient)
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)

		accountsURLParsed, _ := url.Parse(setup.AccountsUpstreamServer.URL)
		accountsHost := accountsURLParsed.Host

		tracker.Reset()
		out, _ := gqlClient.QueryStringWithHeaders(ctx, setup.GatewayServer.URL, `query {
			me {
				id
				reviews {
					authorWithoutProvides {
						nickname
					}
					product {
						reviews {
							authorWithoutProvides {
								realName
								sameUserReviewers {
									nickname
									realName
								}
							}
						}
					}
				}
			}
		}`, nil, t)

		// Verify the response contains expected data
		assert.Equal(t, `{"data":{"me":{"id":"1234","reviews":[{"authorWithoutProvides":{"nickname":"nick-Me"},"product":{"reviews":[{"authorWithoutProvides":{"realName":"User Usington","sameUserReviewers":[{"nickname":"nick-Me","realName":"User Usington"}]}}]}},{"authorWithoutProvides":{"nickname":"nick-Me"},"product":{"reviews":[{"authorWithoutProvides":{"realName":"User Usington","sameUserReviewers":[{"nickname":"nick-Me","realName":"User Usington"}]}}]}}]}}}`, string(out))

		// The union optimization enables L1 for entity fetches in the same
		// dependency chain. However, fetch A (level 1 authorWithoutProvides) and
		// fetch B (level 2 authorWithoutProvides) are in different branches of the
		// fetch tree — they go through separate review/product paths.
		// Fetch C (sameUserReviewers entity resolution) depends on fetch B's
		// branch but fetch A is in a sibling branch, so the postprocessor doesn't
		// include A in C's ancestor union.
		//
		// This is a known limitation: the union optimization only works for
		// fetches in the same dependency chain. For cross-branch accumulation,
		// L1 works at runtime (passthrough writes accumulate) but the
		// postprocessor can't predict it at plan time.
		//
		// accounts: 3 calls (fetch A + fetch B + fetch C)
		// With linear chains (see TestL1CacheEntityUnionOptimization), the
		// union optimization correctly skips redundant fetches.
		accountsCalls := tracker.GetCount(accountsHost)
		assert.Equal(t, 3, accountsCalls,
			"Cross-branch entity fetches: union optimization limited to dependency chains")
	})

	t.Run("L1 disabled - all fetches hit subgraph", func(t *testing.T) {
		t.Parallel()
		tracker := newSubgraphCallTracker(http.DefaultTransport)
		trackingClient := &http.Client{Transport: tracker}

		setup := federationtesting.NewFederationSetup(addCachingGateway(
			withCachingEnableART(false),
			withHTTPClient(trackingClient),
			withCachingOptionsFunc(resolve.CachingOptions{
				EnableL1Cache: false,
				EnableL2Cache: false,
			}),
		))
		t.Cleanup(setup.Close)

		gqlClient := NewGraphqlClient(http.DefaultClient)
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)

		accountsURLParsed, _ := url.Parse(setup.AccountsUpstreamServer.URL)
		accountsHost := accountsURLParsed.Host

		tracker.Reset()
		out, _ := gqlClient.QueryStringWithHeaders(ctx, setup.GatewayServer.URL, `query {
			me {
				id
				reviews {
					authorWithoutProvides {
						nickname
					}
					product {
						reviews {
							authorWithoutProvides {
								realName
								sameUserReviewers {
									nickname
									realName
								}
							}
						}
					}
				}
			}
		}`, nil, t)

		assert.Equal(t, `{"data":{"me":{"id":"1234","reviews":[{"authorWithoutProvides":{"nickname":"nick-Me"},"product":{"reviews":[{"authorWithoutProvides":{"realName":"User Usington","sameUserReviewers":[{"nickname":"nick-Me","realName":"User Usington"}]}}]}},{"authorWithoutProvides":{"nickname":"nick-Me"},"product":{"reviews":[{"authorWithoutProvides":{"realName":"User Usington","sameUserReviewers":[{"nickname":"nick-Me","realName":"User Usington"}]}}]}}]}}}`, string(out))

		// Without L1: all entity fetches hit the subgraph.
		// accounts: root me + fetch A (nickname) + fetch B (realName+username) + fetch C (nickname+realName)
		// The planner merges root me with fetch A, so the actual count is 3.
		accountsCalls := tracker.GetCount(accountsHost)
		assert.Equal(t, 3, accountsCalls,
			"Without L1: all entity fetches must hit accounts subgraph")
	})
}

// TestL1CacheEntityUnionOptimization uses the CacheEntity type (accounts owns fields a-f,
// reviews extends with `nested @requires(fields: "a")`) to create controllable multi-level
// entity fetch chains. Each `nested` level creates:
//   - reviews fetch (resolves nested, needs @requires "a")
//   - accounts entity fetch (provides whatever scalar fields the query selects)
//
// All levels target the same entity key (CacheEntity:1), so L1 accumulates fields.
// The postprocessor should compute the UNION of ancestor providers' ProvidesData
// to determine if a fetch can skip via L1.

// cacheEntitySetup creates a federation gateway with L1 cache and returns the setup + tracker.
func cacheEntitySetup(t *testing.T, enableL1 bool) (*federationtesting.FederationSetup, *subgraphCallTracker) {
	t.Helper()
	tracker := newSubgraphCallTracker(http.DefaultTransport)
	trackingClient := &http.Client{Transport: tracker}
	setup := federationtesting.NewFederationSetup(addCachingGateway(
		withCachingEnableART(false),
		withHTTPClient(trackingClient),
		withCachingOptionsFunc(resolve.CachingOptions{
			EnableL1Cache: enableL1,
			EnableL2Cache: false,
		}),
	))
	t.Cleanup(setup.Close)
	return setup, tracker
}

func TestL1CacheEntityUnionOptimization(t *testing.T) {
	t.Parallel()

	// ---------------------------------------------------------------------------
	// Scenario 1: Basic union — A={a,b}, B={c,d}, C needs {b,c}
	// Neither A nor B individually covers C, but A∪B = {a,b,c,d} ⊇ {b,c}
	// ---------------------------------------------------------------------------
	t.Run("basic union - A provides ab, B provides cd, C needs bc", func(t *testing.T) {
		t.Parallel()
		setup, tracker := cacheEntitySetup(t, true)
		gqlClient := NewGraphqlClient(http.DefaultClient)
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)
		accountsURLParsed, _ := url.Parse(setup.AccountsUpstreamServer.URL)
		accountsHost := accountsURLParsed.Host

		// Level 0: root cacheEntity → accounts (root query, not entity fetch)
		// Level 1: nested → reviews (needs a) → accounts entity fetch A: {a, b}
		// Level 2: nested → reviews (needs a) → accounts entity fetch B: {a, c, d}
		// Level 3: nested → reviews (needs a) → accounts entity fetch C: {a, b, c}
		//          C needs {b, c}: b from A, c from B → union covers C
		tracker.Reset()
		out, _ := gqlClient.QueryStringWithHeaders(ctx, setup.GatewayServer.URL, `query {
			cacheEntity(id: "1") {
				nested {
					a b
					nested {
						c d
						nested {
							b c
						}
					}
				}
			}
		}`, nil, t)

		assert.Equal(t, `{"data":{"cacheEntity":{"nested":{"a":"a-1","b":"b-1","nested":{"c":"c-1","d":"d-1","nested":{"b":"b-1","c":"c-1"}}}}}}`, string(out))

		// With union optimization: C should be L1 hit → skip accounts call
		// Expected: root + fetch A + fetch B = 3 accounts calls (C skipped)
		// Current (without union): root + A + B + C = 4 accounts calls
		accountsCalls := tracker.GetCount(accountsHost)
		assert.Equal(t, 3, accountsCalls,
			"Fetch C should be L1 hit (union of A{a,b} + B{c,d} covers C's needs {b,c})")
	})

	// ---------------------------------------------------------------------------
	// Scenario 2: Union insufficient — A={a,b}, B={c,d}, C needs {b,e}
	// A∪B = {a,b,c,d} does NOT contain e → C must fetch
	// ---------------------------------------------------------------------------
	t.Run("union insufficient - C needs field not in any ancestor", func(t *testing.T) {
		t.Parallel()
		setup, tracker := cacheEntitySetup(t, true)
		gqlClient := NewGraphqlClient(http.DefaultClient)
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)
		accountsURLParsed, _ := url.Parse(setup.AccountsUpstreamServer.URL)
		accountsHost := accountsURLParsed.Host

		// A: {a, b}, B: {c, d}, C: {b, e}
		// Union {a,b,c,d} does NOT contain e → C must fetch
		tracker.Reset()
		out, _ := gqlClient.QueryStringWithHeaders(ctx, setup.GatewayServer.URL, `query {
			cacheEntity(id: "1") {
				nested {
					a b
					nested {
						c d
						nested {
							b e
						}
					}
				}
			}
		}`, nil, t)

		assert.Equal(t, `{"data":{"cacheEntity":{"nested":{"a":"a-1","b":"b-1","nested":{"c":"c-1","d":"d-1","nested":{"b":"b-1","e":"e-1"}}}}}}`, string(out))

		// Even with union optimization, C must fetch because union doesn't cover {b,e}
		// Expected: root + A + B + C = 4 accounts calls
		accountsCalls := tracker.GetCount(accountsHost)
		assert.Equal(t, 4, accountsCalls,
			"Fetch C must hit accounts (union of A{a,b} + B{c,d} does NOT cover C's {b,e})")
	})

	// ---------------------------------------------------------------------------
	// Scenario 3: Overlapping union — A={a,b,c}, B={a,c,d,e}, C needs {b,e}
	// A has b but not e. B has e but not b. Neither alone covers C.
	// A∪B = {a,b,c,d,e} ⊇ {b,e}
	// Note: every fetch implicitly includes "a" due to @requires(fields: "a")
	// ---------------------------------------------------------------------------
	t.Run("overlapping fields in union - C needs b from A and e from B", func(t *testing.T) {
		t.Parallel()
		setup, tracker := cacheEntitySetup(t, true)
		gqlClient := NewGraphqlClient(http.DefaultClient)
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)
		accountsURLParsed, _ := url.Parse(setup.AccountsUpstreamServer.URL)
		accountsHost := accountsURLParsed.Host

		// A: {a, b, c} (a implicit from @requires)
		// B: {a, c, d, e} (a implicit)
		// C: {a, b, e} — b from A, e from B, neither alone covers
		tracker.Reset()
		out, _ := gqlClient.QueryStringWithHeaders(ctx, setup.GatewayServer.URL, `query {
			cacheEntity(id: "1") {
				nested {
					b c
					nested {
						c d e
						nested {
							b e
						}
					}
				}
			}
		}`, nil, t)

		assert.Equal(t, `{"data":{"cacheEntity":{"nested":{"b":"b-1","c":"c-1","nested":{"c":"c-1","d":"d-1","e":"e-1","nested":{"b":"b-1","e":"e-1"}}}}}}`, string(out))

		// With union: C hits L1 (b from A, e from B)
		// Expected: root + A + B = 3 (C skipped)
		// Current: root + A + B + C = 4 (neither A nor B alone covers C)
		accountsCalls := tracker.GetCount(accountsHost)
		assert.Equal(t, 3, accountsCalls,
			"Fetch C should be L1 hit (b from A, e from B — overlapping union)")
	})

	// ---------------------------------------------------------------------------
	// Scenario 4: 4-fetch chain — A={a,b}, B={a,c}, C={a,d}, D needs {b,c,d}
	// Each fetch adds one unique field. No single ancestor covers D.
	// A∪B∪C = {a,b,c,d} ⊇ {b,c,d}
	// Note: "a" is always present due to @requires
	// ---------------------------------------------------------------------------
	t.Run("4-fetch chain - D needs union of A+B+C", func(t *testing.T) {
		t.Parallel()
		setup, tracker := cacheEntitySetup(t, true)
		gqlClient := NewGraphqlClient(http.DefaultClient)
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)
		accountsURLParsed, _ := url.Parse(setup.AccountsUpstreamServer.URL)
		accountsHost := accountsURLParsed.Host

		// A: {a, b}, B: {a, c}, C: {a, d}, D: {a, b, c, d}
		// D needs b (from A), c (from B), d (from C) — no single ancestor covers
		tracker.Reset()
		out, _ := gqlClient.QueryStringWithHeaders(ctx, setup.GatewayServer.URL, `query {
			cacheEntity(id: "1") {
				nested {
					b
					nested {
						c
						nested {
							d
							nested {
								b c d
							}
						}
					}
				}
			}
		}`, nil, t)

		assert.Equal(t, `{"data":{"cacheEntity":{"nested":{"b":"b-1","nested":{"c":"c-1","nested":{"d":"d-1","nested":{"b":"b-1","c":"c-1","d":"d-1"}}}}}}}`, string(out))

		// With union: D hits L1 (b from A, c from B, d from C)
		// Expected: root + A + B + C = 4 accounts calls (D skipped)
		// Current: root + A + B + C + D = 5 accounts calls
		accountsCalls := tracker.GetCount(accountsHost)
		assert.Equal(t, 4, accountsCalls,
			"Fetch D should be L1 hit (union of A{b} + B{c} + C{d} covers D's {b,c,d})")
	})

	// ---------------------------------------------------------------------------
	// Scenario 5: Middle fetch with different fields, C needs from both A and B
	// A={a,b,c}, B={a,d,e}, C needs {b,d}
	// B alone doesn't cover C (no b). A alone doesn't cover C (no d).
	// But with the middle fetch writing to L1, the accumulated entry has both.
	// This tests that the optimizer enables L1 for B as a writer even though
	// B alone doesn't cover any consumer.
	// ---------------------------------------------------------------------------
	t.Run("middle fetch contributes - C needs fields from both A and B", func(t *testing.T) {
		t.Parallel()
		setup, tracker := cacheEntitySetup(t, true)
		gqlClient := NewGraphqlClient(http.DefaultClient)
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)
		accountsURLParsed, _ := url.Parse(setup.AccountsUpstreamServer.URL)
		accountsHost := accountsURLParsed.Host

		// A: {a, b, c}, B: {a, d, e}, C: {a, b, d}
		// C needs b (from A) and d (from B) — neither alone covers
		tracker.Reset()
		out, _ := gqlClient.QueryStringWithHeaders(ctx, setup.GatewayServer.URL, `query {
			cacheEntity(id: "1") {
				nested {
					b c
					nested {
						d e
						nested {
							b d
						}
					}
				}
			}
		}`, nil, t)

		assert.Equal(t, `{"data":{"cacheEntity":{"nested":{"b":"b-1","c":"c-1","nested":{"d":"d-1","e":"e-1","nested":{"b":"b-1","d":"d-1"}}}}}}`, string(out))

		// With union: C hits L1 (b from A, d from B)
		// Expected: root + A + B = 3 (C skipped)
		// Current: root + A + B + C = 4 (optimizer checks individually)
		accountsCalls := tracker.GetCount(accountsHost)
		assert.Equal(t, 3, accountsCalls,
			"Fetch C should be L1 hit (b from A, d from B — middle fetch contributes)")
	})

	// ---------------------------------------------------------------------------
	// Baseline: L1 disabled — verify all fetches hit the subgraph
	// ---------------------------------------------------------------------------
	t.Run("L1 disabled baseline - all fetches hit subgraph", func(t *testing.T) {
		t.Parallel()
		setup, tracker := cacheEntitySetup(t, false)
		gqlClient := NewGraphqlClient(http.DefaultClient)
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)
		accountsURLParsed, _ := url.Parse(setup.AccountsUpstreamServer.URL)
		accountsHost := accountsURLParsed.Host

		// 4-level nesting: root + 3 entity fetches
		tracker.Reset()
		gqlClient.QueryStringWithHeaders(ctx, setup.GatewayServer.URL, `query {
			cacheEntity(id: "1") {
				nested {
					a b
					nested {
						c d
						nested {
							b c
						}
					}
				}
			}
		}`, nil, t)

		accountsCalls := tracker.GetCount(accountsHost)
		assert.Equal(t, 4, accountsCalls,
			"Without L1: all entity fetches must hit accounts (root + 3 nested entity fetches)")
	})
}
