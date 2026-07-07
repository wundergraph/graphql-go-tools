// Package cacheconfig is the LEAF caching policy package on the plan side: the
// declarative per-datasource cache policies an integrator supplies, and the
// CacheConfigProvider port the caching planner passes consume. It carries no
// cache logic and imports no engine packages, so plan and engine/cache can both
// depend on it without a cycle. L2 enablement is DERIVED from the TTLs
// (TTL > 0 || NegativeCacheTTL > 0 for entities, TTL > 0 for root fields);
// the policy structs deliberately carry no explicit L1/L2 switches.
package cacheconfig

import (
	"time"
)

// CachingConfiguration is the declarative caching policy set for ONE
// datasource. It implements CacheConfigProvider by lookup, so it can be handed
// to the engine directly.
type CachingConfiguration struct {
	Entities      []EntityCachePolicy
	RootFields    []RootFieldCachePolicy
	Mutations     []MutationCachePolicy
	Subscriptions []SubscriptionCachePolicy
}

// EntityCachePolicy caches an entity type resolved via _entities fetches.
type EntityCachePolicy struct {
	TypeName, CacheName         string
	TTL, NegativeCacheTTL       time.Duration
	IncludeSubgraphHeaderPrefix bool
	EnablePartialCacheLoad      bool
	HashAnalyticsKeys           bool
	ShadowMode                  bool
}

// RootFieldCachePolicy caches one query root field.
type RootFieldCachePolicy struct {
	TypeName, FieldName, CacheName string
	TTL                            time.Duration
	IncludeSubgraphHeaderPrefix    bool
	ShadowMode, PartialBatchLoad   bool
}

// EnablesCaching reports whether the policy enables ANY cache behavior for its
// root field: L2 is derived from the TTL (see the package doc) and root fields
// never carry L1, so only a positive TTL or shadow mode makes the policy
// effective. An ineffective policy yields no FetchCacheConfig (the
// configurator's all-flags-false safety net), so consumers that change plans
// for cached fields — the per-root-field isolation gate — must treat it as
// "not cached".
func (p RootFieldCachePolicy) EnablesCaching() bool {
	return p.TTL > 0 || p.ShadowMode
}

// MutationCachePolicy declares how a mutation root field interacts with the
// cache (invalidation / L2 population inheritance).
type MutationCachePolicy struct {
	FieldName              string
	Invalidate, PopulateL2 bool
	TTLOverride            time.Duration
}

// SubscriptionCachePolicy declares caching for a subscription root field.
// Subscription caching itself is an out-of-core follow-up; the policy shape is
// part of the provider contract.
type SubscriptionCachePolicy struct {
	TypeName, FieldName, CacheName string
	TTL                            time.Duration
	IncludeSubgraphHeaderPrefix    bool
	EnableInvalidationOnKeyOnly    bool
}

// CacheConfigProvider is the lookup port the caching planner passes use: given
// a coordinate, return the policy and whether one is configured. A nil
// provider means no caching for that datasource; a (zero, false) return means
// no caching for that coordinate.
type CacheConfigProvider interface {
	EntityPolicy(typeName string) (EntityCachePolicy, bool)
	RootFieldPolicy(typeName, fieldName string) (RootFieldCachePolicy, bool)
	MutationPolicy(fieldName string) (MutationCachePolicy, bool)
	SubscriptionPolicy(typeName, fieldName string) (SubscriptionCachePolicy, bool)
}

// EntityPolicy returns the policy for an entity type name.
func (c *CachingConfiguration) EntityPolicy(typeName string) (EntityCachePolicy, bool) {
	for _, p := range c.Entities {
		if p.TypeName == typeName {
			return p, true
		}
	}
	return EntityCachePolicy{}, false
}

// RootFieldPolicy returns the policy for a root-field coordinate.
func (c *CachingConfiguration) RootFieldPolicy(typeName, fieldName string) (RootFieldCachePolicy, bool) {
	for _, p := range c.RootFields {
		if p.TypeName == typeName && p.FieldName == fieldName {
			return p, true
		}
	}
	return RootFieldCachePolicy{}, false
}

// MutationPolicy returns the policy for a mutation root field.
func (c *CachingConfiguration) MutationPolicy(fieldName string) (MutationCachePolicy, bool) {
	for _, p := range c.Mutations {
		if p.FieldName == fieldName {
			return p, true
		}
	}
	return MutationCachePolicy{}, false
}

// SubscriptionPolicy returns the policy for a subscription root field.
func (c *CachingConfiguration) SubscriptionPolicy(typeName, fieldName string) (SubscriptionCachePolicy, bool) {
	for _, p := range c.Subscriptions {
		if p.TypeName == typeName && p.FieldName == fieldName {
			return p, true
		}
	}
	return SubscriptionCachePolicy{}, false
}
