package resolve

import (
	"fmt"
	"time"
)

// FetchCacheConfig is the self-contained, federation-free per-fetch cache
// config the caching planner passes set onto SingleFetch / EntityFetch /
// BatchEntityFetch. The loader carries it forward-only (it never reads a field
// beyond the L1/L2 gate, it hands it to the controller); the cache package
// interprets it. A nil *FetchCacheConfig means caching is disabled for this
// fetch — the gate that makes a not-run planner pass behave as a NO-OP. It
// imports NO federation types: KeySpec references the fetch's own
// representation node, frozen at plan time by the cache key builder.
type FetchCacheConfig struct {
	L1 bool // participate in the request-lifetime shared L1 cache
	L2 bool // participate in the cross-request L2 cache

	// SubgraphName is the fetch's subgraph — the visible cache key prefix, so
	// every subgraph owns its own keyspace.
	SubgraphName string
	// TTL is the STATIC tier of the TTL ladder; a subgraph response's
	// Cache-Control max-age overrides it per fetch result.
	TTL time.Duration
	// MaxTTL clamps whichever tier wins, static or header-driven; 0 = no clamp.
	MaxTTL           time.Duration
	NegativeCacheTTL time.Duration // > 0 enables negative caching
	// IncludeSubgraphHeaders keys the fetch's entries by the forwarded subgraph
	// header hash. It has one role per scope: on a PUBLIC fetch the hash leads
	// the visible L2 key prefix (vary-by-headers, no privacy); on a private one
	// it is the fallback source of the requester identity the key's partition
	// segment derives from — so a private fetch is partitioned exactly once,
	// never by prefix AND partition both.
	IncludeSubgraphHeaders bool
	// Private marks a STATICALLY private fetch: its entries belong to one
	// requester, so their L2 keys carry a partition segment and are not stored
	// at all when the request carries no identity. The request-lifetime layer is
	// unaffected — one request is one requester.
	Private                bool
	EnablePartialCacheLoad bool // partial cache load (serve covered items, fetch the rest)
	PartialBatchLoad       bool // partial batch realign
	ShadowMode             bool // L2 read-but-never-serve probe
	HashAnalyticsKeys      bool

	// KeySpec is the frozen key derivation, data only.
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
// ProvidesData is compared by response SHAPE (Object.Equals); the cache alias
// metadata on it (OriginalName / CacheArgs / HasAliases) is not compared.
// That is sound because fetch dedup additionally requires byte-identical
// rendered Input (FetchConfiguration.Equals), which pins the alias/argument
// structure — and the production dedup pass runs before configs are attached
// (postprocess: dedupe precedes ConfigureCaching).
func (c *FetchCacheConfig) Equals(other *FetchCacheConfig) bool {
	if c == nil || other == nil {
		return c == other
	}
	if c.L1 != other.L1 ||
		c.L2 != other.L2 ||
		c.SubgraphName != other.SubgraphName ||
		c.TTL != other.TTL ||
		c.MaxTTL != other.MaxTTL ||
		c.NegativeCacheTTL != other.NegativeCacheTTL ||
		c.IncludeSubgraphHeaders != other.IncludeSubgraphHeaders ||
		c.Private != other.Private ||
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
		"{l1:%t l2:%t subgraph:%s ttl:%s maxTTL:%s negativeTTL:%s includeHeaders:%t private:%t partial:%t partialBatch:%t shadow:%t hashAnalytics:%t scope:%s type:%s field:%s representation:%t providesData:%t populateL2OnMutation:%t mutationTTL:%s}",
		c.L1,
		c.L2,
		c.SubgraphName,
		c.TTL,
		c.MaxTTL,
		c.NegativeCacheTTL,
		c.IncludeSubgraphHeaders,
		c.Private,
		c.EnablePartialCacheLoad,
		c.PartialBatchLoad,
		c.ShadowMode,
		c.HashAnalyticsKeys,
		c.KeySpec.Scope,
		c.KeySpec.TypeName,
		c.KeySpec.FieldName,
		c.KeySpec.Representation != nil,
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

// CacheKeySpec is DATA ONLY (no renderer, no federation types). It identifies
// the ONE cache entry a fetch reads and writes per item. In entity scope the
// identity is the fetch's own merged representation node — the very object the
// fetch sends as its `representations` element — so read key, write key and
// fetch identity are byte-identical by construction. In root-field scope the
// identity is TypeName/FieldName plus the request variables instead.
type CacheKeySpec struct {
	Scope     CacheScope
	TypeName  string
	FieldName string

	// Representation is the fetch's merged representation node (entity scope
	// only; nil means the fetch is not cached). It is the plan-owned node the
	// fetch's input template renders, with the interfaceObject /
	// entityInterface __typename remap already baked in, and it carries every
	// field the fetch sends — the chosen @key set plus any @requires sets.
	Representation *Object
}

// Equals structurally compares two specs; the representation is compared by
// response SHAPE and is nil-safe.
func (c CacheKeySpec) Equals(other CacheKeySpec) bool {
	if c.Scope != other.Scope || c.TypeName != other.TypeName || c.FieldName != other.FieldName {
		return false
	}
	if (c.Representation == nil) != (other.Representation == nil) {
		return false
	}
	return c.Representation == nil || c.Representation.Equals(other.Representation)
}
