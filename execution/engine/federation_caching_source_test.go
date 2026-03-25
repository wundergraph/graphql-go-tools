package engine_test

import (
	"context"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/execution/engine"
	"github.com/wundergraph/graphql-go-tools/execution/federationtesting"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

func TestCacheWriteEventSource_MutationL2Write(t *testing.T) {
	// Verify that L2 writes triggered by a mutation have Source=CacheSourceMutation in the analytics snapshot.
	defaultCache := NewFakeLoaderCache()

	setup := federationtesting.NewFederationSetup(addCachingGateway(
		withCachingEnableART(false),
		withCachingLoaderCache(map[string]resolve.LoaderCache{"default": defaultCache}),
		withCachingOptionsFunc(resolve.CachingOptions{EnableL2Cache: true, EnableCacheAnalytics: true}),
		withSubgraphEntityCachingConfigs(engine.SubgraphCachingConfigs{
			{
				SubgraphName: "accounts",
				EntityCaching: plan.EntityCacheConfigurations{
					{TypeName: "User", CacheName: "default", TTL: 30 * time.Second},
				},
			},
			{
				SubgraphName: "reviews",
				MutationFieldCaching: plan.MutationFieldCacheConfigurations{
					{FieldName: "addReview", EnableEntityL2CachePopulation: true},
				},
			},
		}),
	))
	t.Cleanup(setup.Close)

	gqlClient := NewGraphqlClient(http.DefaultClient)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	// Execute mutation that triggers User entity resolution → L2 write
	resp, headers := gqlClient.QueryStringWithHeaders(ctx, setup.GatewayServer.URL,
		`mutation AddReview($authorID: String!, $upc: String!, $review: String!) {
			addReview(authorID: $authorID, upc: $upc, review: $review) {
				body
				authorWithoutProvides {
					username
				}
			}
		}`,
		queryVariables{"authorID": "1234", "upc": "top-1", "review": "Great!"}, t)
	assert.Equal(t, `{"data":{"addReview":{"body":"Great!","authorWithoutProvides":{"username":"Me"}}}}`, string(resp))

	// Assert entire snapshot — L2 write must have Source=CacheSourceMutation
	assert.Equal(t, normalizeSnapshot(resolve.CacheAnalyticsSnapshot{
		L2Writes: []resolve.CacheWriteEvent{
			{
				CacheKey:   `{"__typename":"User","key":{"id":"1234"}}`,
				EntityType: "User",
				ByteSize:   49,
				DataSource: "accounts",
				CacheLevel: resolve.CacheLevelL2,
				TTL:        30 * time.Second,
				Source:     resolve.CacheSourceMutation, // Mutation-triggered L2 write carries Source=mutation
			},
		},
		FieldHashes: []resolve.EntityFieldHash{
			{EntityType: "User", FieldName: "username", FieldHash: 4957449860898447395, KeyRaw: `{"id":"1234"}`}, // xxhash("Me")
		},
		EntityTypes: []resolve.EntityTypeInfo{
			{TypeName: "User", Count: 1, UniqueKeys: 1}, // Mutation triggered resolution of 1 User entity
		},
	}), normalizeSnapshot(parseCacheAnalytics(t, headers)))
}

func TestMutationCacheTTLOverride_E2E(t *testing.T) {
	// Verify that MutationFieldCacheConfiguration.TTL overrides the entity's default TTL.
	defaultCache := NewFakeLoaderCache()

	setup := federationtesting.NewFederationSetup(addCachingGateway(
		withCachingEnableART(false),
		withCachingLoaderCache(map[string]resolve.LoaderCache{"default": defaultCache}),
		withCachingOptionsFunc(resolve.CachingOptions{EnableL2Cache: true}),
		withSubgraphEntityCachingConfigs(engine.SubgraphCachingConfigs{
			{
				SubgraphName: "accounts",
				EntityCaching: plan.EntityCacheConfigurations{
					{TypeName: "User", CacheName: "default", TTL: 300 * time.Second},
				},
			},
			{
				SubgraphName: "reviews",
				MutationFieldCaching: plan.MutationFieldCacheConfigurations{
					{FieldName: "addReview", EnableEntityL2CachePopulation: true, TTL: 60 * time.Second},
				},
			},
		}),
	))
	t.Cleanup(setup.Close)

	gqlClient := NewGraphqlClient(http.DefaultClient)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	defaultCache.ClearLog()

	// Execute mutation — TTL should be 60s (mutation override), not 300s (entity default)
	resp := gqlClient.QueryString(ctx, setup.GatewayServer.URL,
		`mutation AddReview($authorID: String!, $upc: String!, $review: String!) {
			addReview(authorID: $authorID, upc: $upc, review: $review) {
				body
				authorWithoutProvides {
					username
				}
			}
		}`,
		queryVariables{"authorID": "1234", "upc": "top-1", "review": "Great!"}, t)
	assert.Equal(t, `{"data":{"addReview":{"body":"Great!","authorWithoutProvides":{"username":"Me"}}}}`, string(resp))

	// Assert entire cache log — single Set with mutation TTL override (60s), no Get (mutations skip L2 reads)
	assert.Equal(t, []CacheLogEntry{
		{Operation: "set", Keys: []string{`{"__typename":"User","key":{"id":"1234"}}`}, TTL: 60 * time.Second}, // L2 write uses mutation TTL override (60s), not entity default (300s)
	}, defaultCache.GetLog())
}

func TestOnSubscriptionCacheCallbacks(t *testing.T) {
	t.Run("OnSubscriptionCacheWrite fires on subscription entity population", func(t *testing.T) {
		defaultCache := NewFakeLoaderCache()

		var mu sync.Mutex
		var writeEvents []resolve.CacheWriteEvent

		setup := federationtesting.NewFederationSetup(addCachingGateway(
			withCachingEnableART(false),
			withCachingLoaderCache(map[string]resolve.LoaderCache{"default": defaultCache}),
			withCachingOptionsFunc(resolve.CachingOptions{EnableL2Cache: true}),
			withSubgraphEntityCachingConfigs(engine.SubgraphCachingConfigs{
				{
					SubgraphName: "products",
					SubscriptionEntityPopulation: plan.SubscriptionEntityPopulationConfigurations{
						{TypeName: "Product", CacheName: "default", TTL: 30 * time.Second},
					},
				},
			}),
			withResolverOptions(func(opts *resolve.ResolverOptions) {
				opts.OnSubscriptionCacheWrite = func(event resolve.CacheWriteEvent) {
					mu.Lock()
					writeEvents = append(writeEvents, event)
					mu.Unlock()
				}
			}),
		))
		t.Cleanup(setup.Close)

		gqlClient := NewGraphqlClient(http.DefaultClient)
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)

		wsAddr := strings.ReplaceAll(setup.GatewayServer.URL, "http://", "ws://")

		// Subscribe to product updates — subscription entity population writes Product to L2
		messages := collectSubscriptionMessages(ctx, gqlClient, wsAddr,
			cachingTestQueryPath("subscriptions/subscription_product_only.query"),
			queryVariables{"upc": "top-4"}, 1, t)
		assert.Equal(t, `{"id":"1","type":"data","payload":{"data":{"updateProductPrice":{"upc":"top-4","name":"Bowler","price":1}}}}`, messages[0])

		// Assert entire callback events slice — exactly 1 event with all fields matching
		mu.Lock()
		defer mu.Unlock()
		require.Equal(t, 1, len(writeEvents), "OnSubscriptionCacheWrite should be called exactly once for 1 subscription event")
		// ByteSize depends on serialized entity; use the actual value from the event
		assert.Equal(t, resolve.CacheWriteEvent{
			CacheKey:   `{"__typename":"Product","key":{"upc":"top-4"}}`,
			EntityType: "Product",
			ByteSize:   writeEvents[0].ByteSize, // Varies with serialization; verified non-zero below
			DataSource: "products",
			CacheLevel: resolve.CacheLevelL2,
			TTL:        30 * time.Second,
			Source:     resolve.CacheSourceSubscription, // Subscription cache write carries Source=subscription
		}, writeEvents[0])
		assert.Greater(t, writeEvents[0].ByteSize, 0, "subscription cache write should have non-zero byte size")
	})

	t.Run("OnSubscriptionCacheInvalidate fires on invalidation-only subscription", func(t *testing.T) {
		defaultCache := NewFakeLoaderCache()

		var mu sync.Mutex
		var invalidateCalls []struct {
			entityType string
			keys       []string
		}

		setup := federationtesting.NewFederationSetup(addCachingGateway(
			withCachingEnableART(false),
			withCachingLoaderCache(map[string]resolve.LoaderCache{"default": defaultCache}),
			withCachingOptionsFunc(resolve.CachingOptions{EnableL2Cache: true}),
			withSubgraphEntityCachingConfigs(engine.SubgraphCachingConfigs{
				{
					SubgraphName: "products",
					SubscriptionEntityPopulation: plan.SubscriptionEntityPopulationConfigurations{
						{TypeName: "Product", CacheName: "default", TTL: 30 * time.Second, EnableInvalidationOnKeyOnly: true},
					},
				},
			}),
			withResolverOptions(func(opts *resolve.ResolverOptions) {
				opts.OnSubscriptionCacheInvalidate = func(entityType string, keys []string) {
					mu.Lock()
					invalidateCalls = append(invalidateCalls, struct {
						entityType string
						keys       []string
					}{entityType, keys})
					mu.Unlock()
				}
			}),
		))
		t.Cleanup(setup.Close)

		gqlClient := NewGraphqlClient(http.DefaultClient)
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)

		// Pre-populate L2 so there's something to invalidate
		err := defaultCache.Set(ctx, []*resolve.CacheEntry{
			{Key: `{"__typename":"Product","key":{"upc":"top-4"}}`, Value: []byte(`{"upc":"top-4","name":"Bowler","price":100,"__typename":"Product"}`)},
		}, 30*time.Second)
		require.NoError(t, err)

		wsAddr := strings.ReplaceAll(setup.GatewayServer.URL, "http://", "ws://")

		// Subscribe using key-only query — selects only @key field (upc), so invalidation mode triggers
		defaultCache.ClearLog()
		messages := collectSubscriptionMessages(ctx, gqlClient, wsAddr,
			cachingTestQueryPath("subscriptions/subscription_product_key_only.query"),
			queryVariables{"upc": "top-4"}, 1, t)
		require.Equal(t, 1, len(messages))

		// Assert entire cache log — should contain a delete for the Product entity key
		cacheLog := defaultCache.GetLog()
		assert.Equal(t, []CacheLogEntry{
			{Operation: "delete", Keys: []string{`{"__typename":"Product","key":{"upc":"top-4"}}`}}, // Subscription key-only event triggers L2 delete
		}, cacheLog)

		// Assert entire callback data — exactly 1 invalidation call
		mu.Lock()
		defer mu.Unlock()
		require.Equal(t, 1, len(invalidateCalls), "OnSubscriptionCacheInvalidate should be called exactly once")
		assert.Equal(t, "Product", invalidateCalls[0].entityType)
		assert.Equal(t, []string{`{"__typename":"Product","key":{"upc":"top-4"}}`}, invalidateCalls[0].keys)
	})
}
