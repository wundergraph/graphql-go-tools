package resolve

import (
	"context"
	"time"

	"github.com/wundergraph/astjson"
	"github.com/wundergraph/go-arena"
)

// CacheWriteReason describes why the engine is writing an entry to the cache.
type CacheWriteReason string

const (
	// CacheWriteReasonDefault leaves the write reason unspecified.
	CacheWriteReasonDefault CacheWriteReason = ""
	// CacheWriteReasonRefresh records a write of freshly fetched data.
	CacheWriteReasonRefresh CacheWriteReason = "refresh"
	// CacheWriteReasonBackfill records a write that fills another cache tier from known data.
	CacheWriteReasonBackfill CacheWriteReason = "backfill"
	// CacheWriteReasonDerived records a write derived from another cached or fetched value.
	CacheWriteReasonDerived CacheWriteReason = "derived"
)

// CacheEntry is the opaque JSON payload exchanged with a router-provided L2 cache backend.
type CacheEntry struct {
	// Key is the fully rendered cache key for this entry.
	Key string
	// Value is the opaque JSON payload stored by the backend.
	Value []byte
	// TTL is the per-entry write expiration: 0 uses the backend default, negative means indefinite.
	TTL time.Duration
	// RemainingTTL is set by the backend on read; 0 means unknown.
	RemainingTTL time.Duration
	// WriteReason is set by the engine to classify the cache write.
	WriteReason CacheWriteReason
}

// LoaderCache is the router-facing L2 backend contract for named cache instances.
//
// Get must return a slice with the same length as keys, using nil entries for misses.
// Set receives per-entry TTLs on CacheEntry.TTL rather than a call-level TTL.
// Backends should be concurrency-safe.
type LoaderCache interface {
	// Get returns cache entries aligned 1:1 with keys; nil entries represent cache misses.
	Get(ctx context.Context, keys []string) ([]*CacheEntry, error)
	// Set stores cache entries using each entry's TTL.
	Set(ctx context.Context, entries []*CacheEntry) error
	// Delete removes the provided keys from the backend.
	Delete(ctx context.Context, keys []string) error
}

// EntityCacheInvalidationConfig configures the resolve-side cache target for entity invalidation.
type EntityCacheInvalidationConfig struct {
	// CacheName selects the named L2 cache backend.
	CacheName string
	// IncludeSubgraphHeaderPrefix includes the subgraph header-hash prefix in invalidation keys.
	IncludeSubgraphHeaderPrefix bool
}

// MutationEntityImpactConfig describes entity-cache writes and invalidations caused by a mutation field.
type MutationEntityImpactConfig struct {
	// EntityTypeName is the entity typename whose key is impacted by the mutation.
	EntityTypeName string
	// KeyFields is the entity @key field tree used to render the impacted entity key.
	KeyFields []KeyField
	// CacheName selects the named L2 cache backend.
	CacheName string
	// IncludeSubgraphHeaderPrefix includes the subgraph header hash in impacted L2 cache keys.
	IncludeSubgraphHeaderPrefix bool
	// InvalidateCache deletes the impacted L2 key after a successful mutation.
	InvalidateCache bool
	// PopulateCache writes the mutation payload directly to L2 after a successful mutation.
	PopulateCache bool
	// PopulateTTL is the TTL for direct mutation population writes.
	PopulateTTL time.Duration
}

// CacheKey is the rendered key data for one input item.
type CacheKey struct {
	// Item is the input value this key was rendered from.
	Item *astjson.Value
	// BatchIndex records the source list position for batch entity-key mappings.
	BatchIndex int
	// Keys contains the rendered cache key strings for Item.
	Keys []string
	// FromCache is populated with the cached value on a cache hit; nil otherwise.
	FromCache *astjson.Value
	// NegativeCacheHit reports that FromCache is a known-absent entity sentinel.
	NegativeCacheHit bool
}

// CacheKeyTemplate renders cache keys for a fetch inside the resolve engine.
//
// The template is engine-internal because it depends on arena and astjson values.
// The router configures cache keys declaratively and does not implement this interface.
type CacheKeyTemplate interface {
	// RenderCacheKeys renders cache keys for the provided items and key prefix.
	RenderCacheKeys(a arena.Arena, ctx *Context, items []*astjson.Value, prefix string) ([]*CacheKey, error)
	// IsEntityFetch reports whether the template renders entity-fetch keys.
	IsEntityFetch() bool
	// BatchEntityKeyArgumentPath returns the batch argument path, or nil when batch support is not needed.
	BatchEntityKeyArgumentPath() []string
	// EntityMergePath returns the merge path for entity payloads, or nil when full payloads are cached.
	EntityMergePath(pp PostProcessingConfiguration) []string
}

type RequestScopedField struct {
	FieldName    string
	FieldPath    []string
	L1Key        string
	ProvidesData *Object
}

// FetchCacheConfiguration describes the per-fetch cache settings attached by planning.
//
// Future cache layers extend this shape with analytics and argument metadata.
type FetchCacheConfiguration struct {
	// CacheName selects the named L2 cache backend for this fetch.
	CacheName string
	// EnableL2Cache enables reads and writes against the selected L2 cache backend.
	EnableL2Cache bool
	// IncludeSubgraphHeaderPrefix includes the subgraph header hash in L2 cache keys.
	IncludeSubgraphHeaderPrefix bool
	// TTL is the per-entry L2 write expiration for this fetch.
	TTL time.Duration
	// NegativeCacheTTL is the per-entry L2 write expiration for known-absent entity sentinels.
	NegativeCacheTTL time.Duration
	// KeyTemplate renders L1 and L2 keys for this fetch.
	KeyTemplate CacheKeyTemplate
	// ProvidesData describes the per-fetch field shape used for cache payloads.
	ProvidesData *Object
	// RequestScopedFields describes request-scoped coordinate L1 fields selected by this fetch.
	RequestScopedFields []RequestScopedField
	// UseL1Cache enables per-request L1 cache reads and writes for this fetch.
	UseL1Cache bool
	// ShadowMode reads and writes L2 but always serves fresh subgraph data.
	ShadowMode bool
	// EnablePartialCacheLoad allows batch entity fetches to refetch only cache-missed entities.
	EnablePartialCacheLoad bool
	// EnableMutationL2CachePopulation allows mutation-triggered entity fetches to write to L2.
	EnableMutationL2CachePopulation bool
	// MutationCacheTTLOverride overrides entity L2 write TTLs for mutation-triggered population.
	MutationCacheTTLOverride time.Duration
	// MutationEntityImpactConfig describes direct mutation population and invalidation.
	MutationEntityImpactConfig *MutationEntityImpactConfig
}
