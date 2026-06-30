package cacheconfig

import (
	"time"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

type CachingConfiguration struct {
	Entities      []EntityCachePolicy
	RootFields    []RootFieldCachePolicy
	Mutations     []MutationCachePolicy
	Subscriptions []SubscriptionCachePolicy
	KeySpecs      []resolve.CacheKeySpec
}

type EntityCachePolicy struct {
	TypeName, CacheName         string
	TTL, NegativeCacheTTL       time.Duration
	IncludeSubgraphHeaderPrefix bool
	EnablePartialCacheLoad      bool
	HashAnalyticsKeys           bool
	ShadowMode                  bool
}

type RootFieldCachePolicy struct {
	TypeName, FieldName, CacheName string
	TTL                            time.Duration
	IncludeSubgraphHeaderPrefix    bool
	ShadowMode, PartialBatchLoad   bool
}

type MutationCachePolicy struct {
	FieldName              string
	Invalidate, PopulateL2 bool
	TTLOverride            time.Duration
}

type SubscriptionCachePolicy struct {
	TypeName, FieldName, CacheName string
	TTL                            time.Duration
	IncludeSubgraphHeaderPrefix    bool
	EnableInvalidationOnKeyOnly    bool
}

type CacheConfigProvider interface {
	EntityPolicy(typeName string) (EntityCachePolicy, bool)
	RootFieldPolicy(typeName, fieldName string) (RootFieldCachePolicy, bool)
	MutationPolicy(fieldName string) (MutationCachePolicy, bool)
	SubscriptionPolicy(typeName, fieldName string) (SubscriptionCachePolicy, bool)
	KeySpec(scope resolve.CacheScope, typeName, fieldName string) (resolve.CacheKeySpec, bool)
}
