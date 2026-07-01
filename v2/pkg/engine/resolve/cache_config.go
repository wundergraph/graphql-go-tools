package resolve

import (
	"fmt"
	"slices"
	"time"
)

// FetchCacheConfig is the self-contained, federation-free per-fetch cache
// config the caching planner passes set onto SingleFetch / EntityFetch /
// BatchEntityFetch. The loader carries it forward-only (it never reads a field
// beyond the L1/L2 gate, it hands it to the controller); the cache package
// interprets it. A nil *FetchCacheConfig means caching is disabled for this
// fetch — the gate that makes a not-run planner pass behave as a NO-OP. It
// imports NO federation types: federation @key selection sets are a plan-time
// input only, frozen into KeySpec by the cache key builder.
type FetchCacheConfig struct {
	L1 bool // participate in the request-lifetime shared L1 cache
	L2 bool // participate in the cross-request L2 cache

	CacheName                   string
	TTL                         time.Duration
	NegativeCacheTTL            time.Duration // > 0 enables negative caching
	IncludeSubgraphHeaderPrefix bool          // fold the subgraph header hash into the L2 key
	EnablePartialCacheLoad      bool          // partial cache load (serve covered items, fetch the rest)
	PartialBatchLoad            bool          // partial batch realign
	ShadowMode                  bool          // L2 read-but-never-serve probe
	HashAnalyticsKeys           bool

	// KeySpec is the frozen multi-key derivation, data only.
	KeySpec CacheKeySpec

	// ProvidesData is the field tree the fetch returns. It is request-independent
	// plan data, but it is consumed at RUNTIME by the coverage walk in
	// PrepareFetch. Folding it here keeps the loader from referencing *Object for
	// caching.
	ProvidesData *Object

	// Mutation populate inheritance (request-lifetime carry).
	PopulateL2OnMutation bool
	MutationTTLOverride  time.Duration
}

// Equals deep-compares two configs so plan dedup can never lose or conflate
// cache policy. It is nil-safe: two nils are equal, nil never equals non-nil.
func (c *FetchCacheConfig) Equals(other *FetchCacheConfig) bool {
	if c == nil || other == nil {
		return c == other
	}
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

// String renders a compact, nil-safe summary for the plan pretty-printer and
// for logs.
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

// CacheScope says whether a config caches a root field or an entity.
type CacheScope uint8

const (
	CacheScopeRootField CacheScope = iota
	CacheScopeEntity
)

// String renders the CacheScope for logs and test assertions.
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

// CacheKeySpec is DATA ONLY (no renderer, no federation types). It models the
// MULTI-KEY identity of an entity / root field: an entity type may declare
// more than one @key set, so Candidates is a list. None is required — each
// candidate is independently and best-effort renderable from whatever data is
// available at the moment it is rendered: render what you can at lookup,
// backfill the rest at write. Frozen once from @key at plan time.
type CacheKeySpec struct {
	Scope     CacheScope
	TypeName  string
	FieldName string

	// Candidates is the list of candidate key templates, one per @key set.
	Candidates []CacheKeyCandidate

	// EntityKeyMappings (root-arg <-> @key) contribute additional candidates
	// rather than a separate key space: a key derivable from root-field args is
	// renderable at lookup, one derivable only from returned entity data becomes
	// renderable at write.
	EntityKeyMappings []EntityKeyMapping
}

// Equals structurally compares two specs, walking Candidates and
// EntityKeyMappings by value.
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

// CacheKeyCandidate is ONE candidate @key template, frozen from a single @key
// set at plan time. Representation is the federation-pointer-free key template
// node tree (built via representationvariable.BuildRepresentationVariableNode)
// with the interfaceObject / entityInterface __typename remap already baked in.
// A candidate renders only when every field it references is present in the
// data at hand; an unrenderable candidate is skipped at lookup and retried at
// write, never an error.
type CacheKeyCandidate struct {
	Representation *Object
}

// CacheWriteReason is metadata only; it does NOT gate writes.
type CacheWriteReason string

const (
	CacheWriteReasonRefresh  CacheWriteReason = "refresh"  // a key already populated, re-written with fresh data
	CacheWriteReasonBackfill CacheWriteReason = "backfill" // a candidate key unrenderable/absent at lookup, populated now
)

// EntityKeyMapping maps a by-key root field's arguments onto an entity's @key
// fields, so the root field can render an entity cache candidate.
type EntityKeyMapping struct {
	EntityTypeName string
	FieldMappings  []EntityFieldMapping
}

// EntityFieldMapping maps one root-field argument path to one entity @key field.
type EntityFieldMapping struct {
	EntityKeyField      string
	ArgumentPath        []string
	ArgumentIsEntityKey bool
}
