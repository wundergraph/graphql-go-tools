package cacheconfig_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan/cacheconfig"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

type fakeProvider struct {
	entity       cacheconfig.EntityCachePolicy
	rootField    cacheconfig.RootFieldCachePolicy
	mutation     cacheconfig.MutationCachePolicy
	subscription cacheconfig.SubscriptionCachePolicy
	keySpec      resolve.CacheKeySpec
}

func (f fakeProvider) EntityPolicy(typeName string) (cacheconfig.EntityCachePolicy, bool) {
	return f.entity, typeName == f.entity.TypeName
}

func (f fakeProvider) RootFieldPolicy(typeName, fieldName string) (cacheconfig.RootFieldCachePolicy, bool) {
	return f.rootField, typeName == f.rootField.TypeName && fieldName == f.rootField.FieldName
}

func (f fakeProvider) MutationPolicy(fieldName string) (cacheconfig.MutationCachePolicy, bool) {
	return f.mutation, fieldName == f.mutation.FieldName
}

func (f fakeProvider) SubscriptionPolicy(typeName, fieldName string) (cacheconfig.SubscriptionCachePolicy, bool) {
	return f.subscription, typeName == f.subscription.TypeName && fieldName == f.subscription.FieldName
}

func (f fakeProvider) KeySpec(scope resolve.CacheScope, typeName, fieldName string) (resolve.CacheKeySpec, bool) {
	return f.keySpec, scope == f.keySpec.Scope && typeName == f.keySpec.TypeName && fieldName == f.keySpec.FieldName
}

func TestPoliciesAndProviderExposeExactValues(t *testing.T) {
	entity := cacheconfig.EntityCachePolicy{
		TypeName:                    "Product",
		CacheName:                   "entities",
		TTL:                         5 * time.Minute,
		NegativeCacheTTL:            time.Minute,
		IncludeSubgraphHeaderPrefix: true,
		EnablePartialCacheLoad:      true,
		HashAnalyticsKeys:           true,
		ShadowMode:                  true,
	}
	rootField := cacheconfig.RootFieldCachePolicy{
		TypeName:                    "Query",
		FieldName:                   "product",
		CacheName:                   "roots",
		TTL:                         3 * time.Minute,
		IncludeSubgraphHeaderPrefix: true,
		ShadowMode:                  true,
		PartialBatchLoad:            true,
	}
	mutation := cacheconfig.MutationCachePolicy{
		FieldName:   "updateProduct",
		Invalidate:  true,
		PopulateL2:  true,
		TTLOverride: 30 * time.Second,
	}
	subscription := cacheconfig.SubscriptionCachePolicy{
		TypeName:                    "Subscription",
		FieldName:                   "productUpdated",
		CacheName:                   "subscriptions",
		TTL:                         10 * time.Second,
		IncludeSubgraphHeaderPrefix: true,
		EnableInvalidationOnKeyOnly: true,
	}
	keySpec := resolve.CacheKeySpec{
		Scope:     resolve.CacheScopeRootField,
		TypeName:  "Query",
		FieldName: "product",
	}
	cfg := cacheconfig.CachingConfiguration{
		Entities:      []cacheconfig.EntityCachePolicy{entity},
		RootFields:    []cacheconfig.RootFieldCachePolicy{rootField},
		Mutations:     []cacheconfig.MutationCachePolicy{mutation},
		Subscriptions: []cacheconfig.SubscriptionCachePolicy{subscription},
		KeySpecs:      []resolve.CacheKeySpec{keySpec},
	}
	provider := fakeProvider{
		entity:       entity,
		rootField:    rootField,
		mutation:     mutation,
		subscription: subscription,
		keySpec:      keySpec,
	}

	assert.Equal(t, cacheconfig.CachingConfiguration{
		Entities:      []cacheconfig.EntityCachePolicy{entity},
		RootFields:    []cacheconfig.RootFieldCachePolicy{rootField},
		Mutations:     []cacheconfig.MutationCachePolicy{mutation},
		Subscriptions: []cacheconfig.SubscriptionCachePolicy{subscription},
		KeySpecs:      []resolve.CacheKeySpec{keySpec},
	}, cfg)

	entityPolicy, entityOK := provider.EntityPolicy("Product")
	assert.Equal(t, entity, entityPolicy)
	assert.Equal(t, true, entityOK)

	rootFieldPolicy, rootFieldOK := provider.RootFieldPolicy("Query", "product")
	assert.Equal(t, rootField, rootFieldPolicy)
	assert.Equal(t, true, rootFieldOK)

	mutationPolicy, mutationOK := provider.MutationPolicy("updateProduct")
	assert.Equal(t, mutation, mutationPolicy)
	assert.Equal(t, true, mutationOK)

	subscriptionPolicy, subscriptionOK := provider.SubscriptionPolicy("Subscription", "productUpdated")
	assert.Equal(t, subscription, subscriptionPolicy)
	assert.Equal(t, true, subscriptionOK)

	actualKeySpec, keySpecOK := provider.KeySpec(resolve.CacheScopeRootField, "Query", "product")
	assert.Equal(t, keySpec, actualKeySpec)
	assert.Equal(t, true, keySpecOK)
}
