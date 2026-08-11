// Package cacheconfig is the LEAF caching configuration package on the plan
// side: the declarative cascade an integrator supplies and its resolution into
// the effective per-subgraph configuration. It carries no cache logic and
// imports no engine packages, so plan and engine/cache can both depend on it
// without a cycle. The cascade is global defaults → per-subgraph overrides →
// per-TYPE declarations (plus per-root-field entries), most specific wins;
// enablement is DERIVED from the resolved values — a positive lifetime,
// NegativeCacheTTL, or shadow mode — with no explicit L1/L2 switches.
package cacheconfig

import (
	"strconv"
	"time"
)

// CacheScope is the privacy scope of the entries a subgraph — or one of its
// root-field coordinates — writes. PUBLIC is the absence of privacy and needs
// no declaration, which is why the cascade has no global scope knob: privacy is
// always narrower than "everything".
type CacheScope uint8

const (
	// CacheScopePublic entries are shared by every requester.
	CacheScopePublic CacheScope = iota
	// CacheScopePrivate entries belong to ONE requester: their keys carry a
	// partition segment derived from the request's identity, and without an
	// identity they are not stored at all.
	CacheScopePrivate
)

// String renders the CacheScope for logs and test assertions.
func (s CacheScope) String() string {
	switch s {
	case CacheScopePublic:
		return "PUBLIC"
	case CacheScopePrivate:
		return "PRIVATE"
	default:
		return "CacheScope(" + strconv.Itoa(int(s)) + ")"
	}
}

// CachingConfiguration is the whole declarative caching configuration: global
// defaults plus per-subgraph overrides keyed by DATASOURCE ID. Consumers read
// it through Resolve, never by walking the levels themselves.
type CachingConfiguration struct {
	Global    GlobalCacheConfig
	Subgraphs map[string]SubgraphCacheConfig
}

// GlobalCacheConfig holds the defaults every subgraph inherits unless it
// overrides them.
type GlobalCacheConfig struct {
	// DefaultTTL is the lifetime of a cached entry; > 0 enables entity caching.
	DefaultTTL time.Duration
	// MaxTTL clamps any resolved TTL; 0 means no clamp.
	MaxTTL time.Duration
	// NegativeCacheTTL is the lifetime of the empty-entity sentinel; > 0
	// enables entity caching on its own.
	NegativeCacheTTL time.Duration
	// EnablePartialCacheLoad serves the covered items of a batch from the cache
	// and fetches only the rest.
	EnablePartialCacheLoad bool
	// HashAnalyticsKeys hashes cache keys before they reach observers.
	HashAnalyticsKeys bool
	// ShadowMode reads the cache and compares, but never serves cached data.
	ShadowMode bool
	// EmitClientCacheControl emits an aggregated Cache-Control response header;
	// nil means enabled. Like EmitCdnTags it describes ONE response rather than
	// one subgraph, so it has no per-subgraph level and the runtime controller
	// reads it off this level directly instead of through Resolve.
	EmitClientCacheControl *bool
	// EmitCdnTags accumulates the invalidation tags of the entries a response
	// was built from, for a CDN that purges by them. Off by default: without
	// such a CDN in front, the union is pure work.
	EmitCdnTags bool
}

// EmitsClientCacheControl reports whether the aggregated client cache answer is
// computed. It is the one knob that defaults to ON, so an unset value enables
// it.
func (g GlobalCacheConfig) EmitsClientCacheControl() bool {
	return g.EmitClientCacheControl == nil || *g.EmitClientCacheControl
}

// SubgraphCacheConfig overrides the global defaults for one subgraph. A nil
// pointer field means "not set at this level" and inherits the global value.
type SubgraphCacheConfig struct {
	// Enabled set to false vetoes caching for the whole subgraph, whatever the
	// resolved TTLs and whatever the root-field entries say.
	Enabled                *bool
	DefaultTTL             *time.Duration
	MaxTTL                 *time.Duration
	NegativeCacheTTL       *time.Duration
	EnablePartialCacheLoad *bool
	ShadowMode             *bool
	// IncludeSubgraphHeaders keys entries by the forwarded subgraph header hash,
	// so they never cross header variants. It has TWO roles, one per scope: on a
	// PUBLIC subgraph it is a plain vary-by-headers knob (the hash leads the
	// visible key prefix); on a PRIVATE one it is the fallback source of the
	// requester identity the partition segment derives from.
	IncludeSubgraphHeaders *bool
	// Scope declares the subgraph's entries private, so their keys are
	// partitioned per requester; nil (and PUBLIC) means shared entries.
	Scope *CacheScope
	// RootFields carries the per-coordinate entries; a root field is cached
	// only when its exact coordinate has an entry here.
	RootFields []RootFieldCacheConfig
	// Types carries the per-type declarations keyed by ENTITY TYPE NAME — the
	// narrowest tier of the cascade, composed from the subgraph's @cache
	// directives (see ExtractTypeCacheConfigs). A type without an entry falls
	// back to the subgraph level.
	Types map[string]TypeCacheConfig
}

// TypeCacheConfig is what ONE type declares about its own caching. Its PRESENCE
// in SubgraphCacheConfig.Types is the declaration itself, so no separate marker
// field is needed and a MaxAge of 0 carries meaning: the type is declared
// UNCACHEABLE and its fetches get no cache configuration at all — the negative
// sentinel included, whatever NegativeCacheTTL the subgraph resolves. Negative
// caching stays a subgraph-level knob in this tier; a type declares only its
// own lifetime and privacy.
type TypeCacheConfig struct {
	// MaxAge is the type's static lifetime, replacing the subgraph DefaultTTL
	// for its entities; 0 declares the type uncacheable.
	MaxAge time.Duration
	// Scope declares the type's entries private, so their keys carry a partition
	// segment. It only ever ADDS privacy: a PUBLIC type inside a private
	// subgraph stays private.
	Scope CacheScope
}

// RootFieldCacheConfig caches one query root-field coordinate of a subgraph.
type RootFieldCacheConfig struct {
	TypeName, FieldName string
	// TTL is the coordinate's own lifetime; 0 falls back to the subgraph's (and
	// then the global) DefaultTTL.
	TTL time.Duration
	// ShadowMode, IncludeSubgraphHeaders and Scope add to the subgraph-level
	// values; they cannot switch an enabled subgraph-level setting back off, so
	// a coordinate can declare itself private inside a public subgraph but never
	// the reverse.
	ShadowMode             bool
	PartialBatchLoad       bool
	IncludeSubgraphHeaders bool
	Scope                  CacheScope
}

// EffectiveSubgraphConfig is the cascade resolved for ONE subgraph: concrete
// values only, no "unset" state left for consumers to interpret.
type EffectiveSubgraphConfig struct {
	// Enabled is false when the subgraph vetoed caching; nothing of it is then
	// cached, entities and root fields alike.
	Enabled                bool
	DefaultTTL             time.Duration
	MaxTTL                 time.Duration
	NegativeCacheTTL       time.Duration
	EnablePartialCacheLoad bool
	HashAnalyticsKeys      bool
	ShadowMode             bool
	IncludeSubgraphHeaders bool
	Scope                  CacheScope
	RootFields             []RootFieldCacheConfig
	Types                  map[string]TypeCacheConfig
}

// EntityCacheConfig is the static tier resolved for ONE entity fetch: the
// lifetime its entries take when the subgraph response carries no max-age, and
// the scope their keys derive under.
type EntityCacheConfig struct {
	TTL   time.Duration
	Scope CacheScope
}

// Entities resolves the static tier for an entity fetch over typeNames — the
// entity types ONE fetch can resolve, which is more than one when it batches
// representations of several concrete types under an abstract type. Each type
// contributes its own declaration where it has one and the subgraph DefaultTTL
// where it has none; the fetch takes the MINIMUM contributed lifetime and turns
// private as soon as one contribution is private, so privacy widens downwards
// and never up.
//
// ok=false when the subgraph vetoed caching, when a contributing type is
// declared uncacheable (MaxAge 0, a veto covering the negative sentinel too),
// or when nothing is left to enable — neither a positive resolved TTL nor the
// subgraph's NegativeCacheTTL.
func (c EffectiveSubgraphConfig) Entities(typeNames []string) (EntityCacheConfig, bool) {
	if !c.Enabled || len(typeNames) == 0 {
		return EntityCacheConfig{}, false
	}
	entity := EntityCacheConfig{Scope: c.Scope}
	for i, typeName := range typeNames {
		declaration, declared := c.Types[typeName]
		if declared && declaration.MaxAge <= 0 {
			return EntityCacheConfig{}, false
		}
		ttl := c.DefaultTTL
		if declared {
			ttl = declaration.MaxAge
			if declaration.Scope == CacheScopePrivate {
				entity.Scope = CacheScopePrivate
			}
		}
		if i == 0 || ttl < entity.TTL {
			entity.TTL = ttl
		}
	}
	if entity.TTL <= 0 && c.NegativeCacheTTL <= 0 {
		return EntityCacheConfig{}, false
	}
	return entity, true
}

// RootField returns the effective entry for a root-field coordinate, with the
// TTL ladder applied (coordinate entry → subgraph DefaultTTL → global
// DefaultTTL) and the subgraph-level ShadowMode, IncludeSubgraphHeaders and
// Scope folded in. An entry for the exact coordinate must EXIST for the ladder
// to run — a root field is never cached by a bare DefaultTTL. ok=false when
// the subgraph is vetoed, the coordinate has no entry, or the entry enables
// nothing (root fields never carry L1, so that means no positive TTL and no
// shadow mode) — consumers must treat that as "not cached".
func (c EffectiveSubgraphConfig) RootField(typeName, fieldName string) (RootFieldCacheConfig, bool) {
	if !c.Enabled {
		return RootFieldCacheConfig{}, false
	}
	for _, entry := range c.RootFields {
		if entry.TypeName != typeName || entry.FieldName != fieldName {
			continue
		}
		if entry.TTL <= 0 {
			// The coordinate entry declares PARTICIPATION; its TTL is optional and
			// falls back to the subgraph's DefaultTTL, which Resolve has already
			// filled from the global default when the subgraph set none.
			entry.TTL = c.DefaultTTL
		}
		entry.ShadowMode = entry.ShadowMode || c.ShadowMode
		entry.IncludeSubgraphHeaders = entry.IncludeSubgraphHeaders || c.IncludeSubgraphHeaders
		if c.Scope == CacheScopePrivate {
			entry.Scope = CacheScopePrivate
		}
		if entry.TTL <= 0 && !entry.ShadowMode {
			return RootFieldCacheConfig{}, false
		}
		return entry, true
	}
	return RootFieldCacheConfig{}, false
}

// Resolve walks the cascade for one datasource ID: global values, then the
// subgraph's overrides where it set any. A nil configuration and a vetoed
// subgraph both resolve to a disabled configuration.
func (c *CachingConfiguration) Resolve(subgraphID string) EffectiveSubgraphConfig {
	if c == nil {
		return EffectiveSubgraphConfig{}
	}
	effective := EffectiveSubgraphConfig{
		Enabled:                true,
		DefaultTTL:             c.Global.DefaultTTL,
		MaxTTL:                 c.Global.MaxTTL,
		NegativeCacheTTL:       c.Global.NegativeCacheTTL,
		EnablePartialCacheLoad: c.Global.EnablePartialCacheLoad,
		HashAnalyticsKeys:      c.Global.HashAnalyticsKeys,
		ShadowMode:             c.Global.ShadowMode,
	}
	subgraph, ok := c.Subgraphs[subgraphID]
	if !ok {
		return effective
	}
	applyOverride(&effective.Enabled, subgraph.Enabled)
	applyOverride(&effective.DefaultTTL, subgraph.DefaultTTL)
	applyOverride(&effective.MaxTTL, subgraph.MaxTTL)
	applyOverride(&effective.NegativeCacheTTL, subgraph.NegativeCacheTTL)
	applyOverride(&effective.EnablePartialCacheLoad, subgraph.EnablePartialCacheLoad)
	applyOverride(&effective.ShadowMode, subgraph.ShadowMode)
	applyOverride(&effective.IncludeSubgraphHeaders, subgraph.IncludeSubgraphHeaders)
	applyOverride(&effective.Scope, subgraph.Scope)
	effective.RootFields = subgraph.RootFields
	effective.Types = subgraph.Types
	return effective
}

// applyOverride overwrites the effective value when the narrower level set one.
func applyOverride[T any](effective *T, override *T) {
	if override != nil {
		*effective = *override
	}
}
