package engine_test

import (
	"context"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/execution/engine"
	"github.com/wundergraph/graphql-go-tools/execution/federationtesting"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

// TestRequestScopedFieldDeduplication verifies that @requestScoped fields are
// exported from the first fetch (root or entity) into the per-request
// requestScopedL1 cache and injected into subsequent entity fetches, skipping
// the subgraph call entirely.
//
// Scenario:
//   - accounts subgraph: root field `me` returns User entity
//   - reviews subgraph: extends User with entity fields (reviews, coReviewers)
//   - The `username` field on User is declared @requestScoped on the reviews
//     subgraph, meaning its value is the same for all User instances in a request.
//
// Expected flow:
//  1. Root query `me` resolves User from accounts, exports `username` to requestScopedL1.
//  2. Entity resolution for coReviewers (also User) finds `username` in requestScopedL1
//     and injects it, skipping the accounts subgraph call for that batch.
//
// NOTE: This test requires the planner to generate RequestScopedFields on the
// accounts datasource and reviews entity fetch.
// Until that planner work is complete, the test is skipped.
func TestRequestScopedFieldDeduplication(t *testing.T) {
	t.Skip("waiting for planner implementation: SubgraphCachingConfig does not yet include RequestScopedFields, and the planner does not yet generate RequestScopedFields on fetch configurations")

	t.Parallel()

	defaultCache := NewFakeLoaderCache()
	caches := map[string]resolve.LoaderCache{
		"default": defaultCache,
	}

	tracker := newSubgraphCallTracker(http.DefaultTransport)
	trackingClient := &http.Client{Transport: tracker}

	// Configure the accounts subgraph with @requestScoped fields.
	// The planner should read RequestScopedFields from FederationMetaData and
	// generate RequestScopedFields on both the root fetch and the entity fetch
	// for the reviews subgraph.
	subgraphCachingConfigs := engine.SubgraphCachingConfigs{
		{
			SubgraphName: "accounts",
			EntityCaching: plan.EntityCacheConfigurations{
				{TypeName: "User", CacheName: "default", TTL: 30 * time.Second},
			},
		},
		{
			SubgraphName: "reviews",
			EntityCaching: plan.EntityCacheConfigurations{
				{TypeName: "User", CacheName: "default", TTL: 30 * time.Second},
			},
		},
	}

	setup := federationtesting.NewFederationSetup(addCachingGateway(
		withCachingEnableART(false),
		withCachingLoaderCache(caches),
		withHTTPClient(trackingClient),
		withCachingOptionsFunc(resolve.CachingOptions{
			EnableL1Cache: true,
			EnableL2Cache: true,
		}),
		withSubgraphEntityCachingConfigs(subgraphCachingConfigs),
	))
	t.Cleanup(setup.Close)

	gqlClient := NewGraphqlClient(http.DefaultClient)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	accountsURLParsed, _ := url.Parse(setup.AccountsUpstreamServer.URL)
	accountsHost := accountsURLParsed.Host
	reviewsURLParsed, _ := url.Parse(setup.ReviewsUpstreamServer.URL)
	reviewsHost := reviewsURLParsed.Host

	// Query: me { id username reviews { body authorWithoutProvides { id username } } }
	//
	// This triggers:
	// 1. Root fetch to accounts for `me` -> returns User{id, username}
	//    -> requestScopedL1 exports username
	// 2. Entity fetch to reviews for User.reviews
	// 3. Entity fetch to accounts for authorWithoutProvides (User entity)
	//    -> requestScopedL1 should inject username, skipping the fetch
	query := `query {
		me {
			id
			username
			reviews {
				body
				authorWithoutProvides {
					id
					username
				}
			}
		}
	}`

	tracker.Reset()
	defaultCache.ClearLog()

	resp := gqlClient.QueryString(ctx, setup.GatewayServer.URL, query, nil, t)

	// Verify response is correct
	assert.Equal(t, `{"data":{"me":{"id":"1234","username":"Me","reviews":[{"body":"A highly effective form of birth control.","authorWithoutProvides":{"id":"1234","username":"Me"}},{"body":"Fedoras are one of the most fashionable hats around and can look great with a variety of outfits.","authorWithoutProvides":{"id":"1234","username":"Me"}}]}}}`, string(resp))

	// With @requestScoped deduplication:
	// - accounts should be called once for the root `me` query
	// - The second accounts call (for authorWithoutProvides entity resolution)
	//   should be skipped because `username` was injected from requestScopedL1
	accountsCalls := tracker.GetCount(accountsHost)
	assert.Equal(t, 1, accountsCalls,
		"accounts subgraph should be called only once; the entity fetch for "+
			"authorWithoutProvides should be skipped via requestScoped injection")

	// reviews subgraph should still be called for User.reviews
	reviewsCalls := tracker.GetCount(reviewsHost)
	// Fuzzy: kept as a smoke-check while this test is under t.Skip pending planner
	// implementation. The exact call count is planner-dependent and will be locked
	// down when the test is re-enabled.
	if reviewsCalls == 0 {
		t.Fatalf("reviews subgraph should be called at least once for User.reviews")
	}
}

// TestRequestScopedFieldFallbackWithoutProvider verifies that when the root
// field that provides a @requestScoped value is NOT in the query, the first
// entity batch fetch populates the requestScopedL1 cache, and the second
// entity batch fetch skips the subgraph call by reading from requestScopedL1.
//
// Scenario:
//   - No root field provides the @requestScoped value (no export source).
//   - First entity batch fetch resolves the field normally and exports to requestScopedL1.
//   - Second entity batch fetch finds the value in requestScopedL1 and skips.
//
// NOTE: This test requires the planner to generate RequestScopedFields on the
// first entity fetch when no root field is available.
func TestRequestScopedFieldFallbackWithoutProvider(t *testing.T) {
	t.Skip("waiting for planner implementation: SubgraphCachingConfig does not yet include RequestScopedFields, and the planner does not yet generate RequestScopedFields on fetch configurations")

	t.Parallel()

	defaultCache := NewFakeLoaderCache()
	caches := map[string]resolve.LoaderCache{
		"default": defaultCache,
	}

	tracker := newSubgraphCallTracker(http.DefaultTransport)
	trackingClient := &http.Client{Transport: tracker}

	subgraphCachingConfigs := engine.SubgraphCachingConfigs{
		{
			SubgraphName: "accounts",
			EntityCaching: plan.EntityCacheConfigurations{
				{TypeName: "User", CacheName: "default", TTL: 30 * time.Second},
			},
		},
		{
			SubgraphName: "reviews",
			EntityCaching: plan.EntityCacheConfigurations{
				{TypeName: "User", CacheName: "default", TTL: 30 * time.Second},
			},
		},
	}

	setup := federationtesting.NewFederationSetup(addCachingGateway(
		withCachingEnableART(false),
		withCachingLoaderCache(caches),
		withHTTPClient(trackingClient),
		withCachingOptionsFunc(resolve.CachingOptions{
			EnableL1Cache: true,
			EnableL2Cache: true,
		}),
		withSubgraphEntityCachingConfigs(subgraphCachingConfigs),
	))
	t.Cleanup(setup.Close)

	gqlClient := NewGraphqlClient(http.DefaultClient)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	accountsURLParsed, _ := url.Parse(setup.AccountsUpstreamServer.URL)
	accountsHost := accountsURLParsed.Host

	// Query topReviews without querying `me` first.
	// This means there is no root field to export @requestScoped values.
	//
	// Expected flow:
	// 1. Root fetch to reviews for topReviews -> returns Review list
	// 2. First entity batch to accounts for authorWithoutProvides (User entities)
	//    -> fetches normally + exports username to requestScopedL1
	// 3. If there are additional entity batches for other User fields,
	//    they should find username in requestScopedL1 and skip the fetch.
	//
	// For the sameUserReviewers path:
	// - reviews.authorWithoutProvides resolves User{id:1234}
	// - reviews.sameUserReviewers @requires(fields: "username") triggers:
	//   a) Entity fetch to accounts for username (first batch -> fetches + exports)
	//   b) Entity fetch to accounts for sameUserReviewers' User entities
	//      -> should find username in requestScopedL1 and skip
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

	tracker.Reset()
	defaultCache.ClearLog()

	resp := gqlClient.QueryString(ctx, setup.GatewayServer.URL, query, nil, t)
	require.NotEmpty(t, resp)

	// Without @requestScoped: accounts would be called for:
	//   1. authorWithoutProvides entity fetch (username for all review authors)
	//   2. sameUserReviewers @requires entity fetch (username needed first)
	//   3. sameUserReviewers result entity fetch
	//
	// With @requestScoped: after the first entity batch populates requestScopedL1,
	// subsequent batches for the same @requestScoped field should skip.
	// The exact reduction depends on how many entity batches the planner creates.
	accountsCalls := tracker.GetCount(accountsHost)

	// We expect at least 1 call (the initial entity fetch) but fewer than
	// the non-optimized case. The exact count depends on planner output.
	if accountsCalls == 0 {
		t.Fatalf("accounts should be called at least once for the initial entity fetch")
	}

	// Log the actual call count for debugging during development.
	t.Logf("accounts subgraph calls: %d (expected fewer with @requestScoped optimization)", accountsCalls)
	t.Logf("all subgraph calls: %v", tracker.GetCounts())
}
