package postprocess

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan/cacheconfig"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

func TestCacheConfigStamperStampsEntityFetches(t *testing.T) {
	definition := parseFreezerDefinition(t, `
		scalar String

		type User {
			id: String!
		}
	`)
	federation := initFreezerFederation(t, []plan.FederationFieldConfiguration{
		{TypeName: "User", SelectionSet: "id"},
	})
	freezer := &cacheKeySpecFreezer{
		federation: map[string]plan.FederationMetaData{"ds": federation},
		definition: definition,
	}
	provider := testCacheConfigProvider{
		entityPolicies: map[string]cacheconfig.EntityCachePolicy{
			"User": {
				TypeName:                    "User",
				CacheName:                   "users",
				TTL:                         time.Minute,
				NegativeCacheTTL:            5 * time.Second,
				IncludeSubgraphHeaderPrefix: true,
				EnablePartialCacheLoad:      true,
				HashAnalyticsKeys:           true,
				ShadowMode:                  true,
			},
		},
	}
	stamper := &cacheConfigStamper{
		providers: map[string]cacheconfig.CacheConfigProvider{"ds": provider},
		freezer:   freezer,
	}
	info := testEntityFetchInfo("ds", "User")
	providesData := &resolve.Object{
		Fields: []*resolve.Field{
			{
				Name:         []byte("displayName"),
				OriginalName: []byte("name"),
				Value: &resolve.String{
					Path: []string{"displayName"},
				},
			},
		},
	}
	expected := &resolve.FetchCacheConfig{
		L1:                          true,
		L2:                          true,
		CacheName:                   "users",
		TTL:                         time.Minute,
		NegativeCacheTTL:            5 * time.Second,
		IncludeSubgraphHeaderPrefix: true,
		EnablePartialCacheLoad:      true,
		ShadowMode:                  true,
		HashAnalyticsKeys:           true,
		KeySpec: resolve.CacheKeySpec{
			Scope:     resolve.CacheScopeEntity,
			TypeName:  "User",
			FieldName: "_entities",
			Candidates: []resolve.CacheKeyCandidate{
				{Representation: entityKeyObject("User", scalarKeyField("id"))},
			},
		},
		ProvidesData: providesData,
	}

	entityFetch := &resolve.EntityFetch{Info: info}
	batchFetch := &resolve.BatchEntityFetch{Info: info}
	singleEntityFetch := &resolve.SingleFetch{
		FetchConfiguration: resolve.FetchConfiguration{RequiresEntityFetch: true},
		Info:               info,
	}
	tree := resolve.Sequence(
		resolve.Single(entityFetch),
		resolve.Single(batchFetch),
		resolve.Single(singleEntityFetch),
	)

	stamper.process(tree, map[*resolve.FetchInfo]*resolve.Object{info: providesData})

	assert.Equal(t, expected, entityFetch.Cache)
	assert.Equal(t, expected, batchFetch.Cache)
	assert.Equal(t, expected, singleEntityFetch.Cache)
	assert.Equal(t, true, providesData.HasAliases)
}

func TestCacheConfigStamperLeavesCacheNilWhenEntityPolicyIsMissing(t *testing.T) {
	definition := parseFreezerDefinition(t, `
		scalar String

		type User {
			id: String!
		}
	`)
	federation := initFreezerFederation(t, []plan.FederationFieldConfiguration{
		{TypeName: "User", SelectionSet: "id"},
	})
	stamper := &cacheConfigStamper{
		providers: map[string]cacheconfig.CacheConfigProvider{"ds": testCacheConfigProvider{}},
		freezer: &cacheKeySpecFreezer{
			federation: map[string]plan.FederationMetaData{"ds": federation},
			definition: definition,
		},
	}
	fetch := &resolve.EntityFetch{Info: testEntityFetchInfo("ds", "User")}

	stamper.process(resolve.Single(fetch), nil)

	assert.Equal(t, (*resolve.FetchCacheConfig)(nil), fetch.Cache)
}

func TestCacheConfigStamperLeavesRootFieldFetchNil(t *testing.T) {
	stamper := &cacheConfigStamper{
		providers: map[string]cacheconfig.CacheConfigProvider{"ds": testCacheConfigProvider{
			entityPolicies: map[string]cacheconfig.EntityCachePolicy{
				"User": {TypeName: "User", CacheName: "users", TTL: time.Minute},
			},
		}},
		freezer: &cacheKeySpecFreezer{},
	}
	fetch := &resolve.SingleFetch{
		Info: &resolve.FetchInfo{
			DataSourceID: "ds",
			RootFields: []resolve.GraphCoordinate{
				{TypeName: "Query", FieldName: "viewer"},
			},
		},
	}

	stamper.process(resolve.Single(fetch), nil)

	assert.Equal(t, (*resolve.FetchCacheConfig)(nil), fetch.Cache)
}

func testEntityFetchInfo(dataSourceID, typeName string) *resolve.FetchInfo {
	return &resolve.FetchInfo{
		DataSourceID: dataSourceID,
		RootFields: []resolve.GraphCoordinate{
			{TypeName: typeName, FieldName: "_entities"},
		},
	}
}

type testCacheConfigProvider struct {
	entityPolicies map[string]cacheconfig.EntityCachePolicy
}

func (p testCacheConfigProvider) EntityPolicy(typeName string) (cacheconfig.EntityCachePolicy, bool) {
	pol, ok := p.entityPolicies[typeName]
	return pol, ok
}

func (p testCacheConfigProvider) RootFieldPolicy(typeName, fieldName string) (cacheconfig.RootFieldCachePolicy, bool) {
	return cacheconfig.RootFieldCachePolicy{}, false
}

func (p testCacheConfigProvider) MutationPolicy(fieldName string) (cacheconfig.MutationCachePolicy, bool) {
	return cacheconfig.MutationCachePolicy{}, false
}

func (p testCacheConfigProvider) SubscriptionPolicy(typeName, fieldName string) (cacheconfig.SubscriptionCachePolicy, bool) {
	return cacheconfig.SubscriptionCachePolicy{}, false
}

func (p testCacheConfigProvider) KeySpec(scope resolve.CacheScope, typeName, fieldName string) (resolve.CacheKeySpec, bool) {
	return resolve.CacheKeySpec{}, false
}

func TestCacheConfigStamperProviderInterface(t *testing.T) {
	require.Implements(t, (*cacheconfig.CacheConfigProvider)(nil), testCacheConfigProvider{})
}
