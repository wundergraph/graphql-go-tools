package resolve

import (
	"fmt"
	"slices"
	"time"
)

// FetchCacheConfig is the self-contained, federation-free per-fetch cache config
// stamped by RFC-2 onto SingleFetch / EntityFetch / BatchEntityFetch.
type FetchCacheConfig struct {
	L1 bool
	L2 bool

	CacheName                   string
	TTL                         time.Duration
	NegativeCacheTTL            time.Duration
	IncludeSubgraphHeaderPrefix bool
	EnablePartialCacheLoad      bool
	PartialBatchLoad            bool
	ShadowMode                  bool
	HashAnalyticsKeys           bool

	KeySpec CacheKeySpec

	// ProvidesData is the field tree the fetch returns.
	ProvidesData *Object

	PopulateL2OnMutation bool
	MutationTTLOverride  time.Duration
}

// Equals lets FetchConfiguration.Equals deep-compare cache config.
func (c *FetchCacheConfig) Equals(other *FetchCacheConfig) bool {
	if c.L1 != other.L1 ||
		c.L2 != other.L2 ||
		c.CacheName != other.CacheName ||
		c.TTL != other.TTL ||
		c.NegativeCacheTTL != other.NegativeCacheTTL ||
		c.IncludeSubgraphHeaderPrefix != other.IncludeSubgraphHeaderPrefix ||
		c.EnablePartialCacheLoad != other.EnablePartialCacheLoad ||
		c.PartialBatchLoad != other.PartialBatchLoad ||
		c.ShadowMode != other.ShadowMode ||
		c.HashAnalyticsKeys != other.HashAnalyticsKeys ||
		c.PopulateL2OnMutation != other.PopulateL2OnMutation ||
		c.MutationTTLOverride != other.MutationTTLOverride {
		return false
	}
	if !c.KeySpec.Equals(other.KeySpec) {
		return false
	}
	if (c.ProvidesData == nil) != (other.ProvidesData == nil) {
		return false
	}
	if c.ProvidesData != nil && !c.ProvidesData.Equals(other.ProvidesData) {
		return false
	}
	return true
}

// String renders a compact, nil-safe summary for logs and the plan pretty-printer.
func (c *FetchCacheConfig) String() string {
	if c == nil {
		return "<nil>"
	}
	return fmt.Sprintf(
		"{l1:%t l2:%t cacheName:%s ttl:%s negativeTTL:%s includeHeaders:%t partial:%t partialBatch:%t shadow:%t hashAnalytics:%t scope:%s type:%s field:%s candidates:%d entityKeyMappings:%d providesData:%t populateL2OnMutation:%t mutationTTL:%s}",
		c.L1,
		c.L2,
		c.CacheName,
		c.TTL,
		c.NegativeCacheTTL,
		c.IncludeSubgraphHeaderPrefix,
		c.EnablePartialCacheLoad,
		c.PartialBatchLoad,
		c.ShadowMode,
		c.HashAnalyticsKeys,
		c.KeySpec.Scope,
		c.KeySpec.TypeName,
		c.KeySpec.FieldName,
		len(c.KeySpec.Candidates),
		len(c.KeySpec.EntityKeyMappings),
		c.ProvidesData != nil,
		c.PopulateL2OnMutation,
		c.MutationTTLOverride,
	)
}

type CacheScope uint8

const (
	CacheScopeRootField CacheScope = iota
	CacheScopeEntity
)

func (s CacheScope) String() string {
	switch s {
	case CacheScopeRootField:
		return "RootField"
	case CacheScopeEntity:
		return "Entity"
	default:
		return fmt.Sprintf("CacheScope(%d)", s)
	}
}

// CacheKeySpec is DATA ONLY. It models the MULTI-KEY identity of an entity / root field.
type CacheKeySpec struct {
	Scope     CacheScope
	TypeName  string
	FieldName string

	Candidates        []CacheKeyCandidate
	EntityKeyMappings []EntityKeyMapping
}

func (c CacheKeySpec) Equals(other CacheKeySpec) bool {
	if c.Scope != other.Scope || c.TypeName != other.TypeName || c.FieldName != other.FieldName {
		return false
	}
	if !slices.EqualFunc(c.Candidates, other.Candidates, func(a, b CacheKeyCandidate) bool {
		if (a.Representation == nil) != (b.Representation == nil) {
			return false
		}
		if a.Representation == nil {
			return true
		}
		return a.Representation.Equals(b.Representation)
	}) {
		return false
	}
	return slices.EqualFunc(c.EntityKeyMappings, other.EntityKeyMappings, func(a, b EntityKeyMapping) bool {
		return a.EntityTypeName == b.EntityTypeName && slices.EqualFunc(a.FieldMappings, b.FieldMappings, func(left, right EntityFieldMapping) bool {
			return left.EntityKeyField == right.EntityKeyField &&
				slices.Equal(left.ArgumentPath, right.ArgumentPath) &&
				left.ArgumentIsEntityKey == right.ArgumentIsEntityKey
		})
	})
}

// CacheKeyCandidate is ONE candidate @key template, frozen from a single @key set at plan time.
type CacheKeyCandidate struct {
	Representation *Object
}

// CacheWriteReason is metadata only; it does NOT gate writes.
type CacheWriteReason string

const (
	CacheWriteReasonRefresh  CacheWriteReason = "refresh"
	CacheWriteReasonBackfill CacheWriteReason = "backfill"
)

// EntityKeyMapping maps root-field arguments to entity @key fields.
type EntityKeyMapping struct {
	EntityTypeName string
	FieldMappings  []EntityFieldMapping
}

// EntityFieldMapping maps one root argument path to one entity @key field.
type EntityFieldMapping struct {
	EntityKeyField      string
	ArgumentPath        []string
	ArgumentIsEntityKey bool
}
