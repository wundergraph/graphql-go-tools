package cache

import (
	"slices"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan/cacheconfig"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

// fetchCacheConfigurator assembles the self-contained *resolve.FetchCacheConfig
// for each cache-eligible fetch (resolved configuration + built key spec +
// ProvidesData) and sets it on the concrete fetch via Fetch.SetCacheConfig —
// after createConcreteSingleFetchTypes, so the config lands on the final fetch
// types.
type fetchCacheConfigurator struct {
	caching *cacheconfig.CachingConfiguration
}

// configureTree walks one flat fetch tree and sets per-fetch cache config; a
// fetch keeps a nil Cache when its subgraph resolves to no caching, its
// coordinate has no entry, no usable key exists, or the assembled config
// enables nothing.
func (c *fetchCacheConfigurator) configureTree(response *resolve.GraphQLResponse, tree *resolve.FetchTreeNode) {
	if tree == nil {
		return
	}
	if tree.Item != nil && tree.Item.Fetch != nil {
		if cfg := c.buildConfig(tree.Item.Fetch, response.CacheProvidesData()); cfg != nil {
			tree.Item.Fetch.SetCacheConfig(cfg)
		}
	}
	for _, child := range tree.ChildNodes {
		c.configureTree(response, child)
	}
}

// buildConfig assembles the config for one fetch, or nil when the fetch is not
// cacheable. Info may be nil despite the engine forcing FetchInfo on — e.g.
// integrators driving postprocess directly — and then the fetch is simply not
// cached.
func (c *fetchCacheConfigurator) buildConfig(fetch resolve.Fetch, pd map[*resolve.FetchInfo]*resolve.Object) *resolve.FetchCacheConfig {
	info := fetch.FetchInfo()
	if info == nil || len(info.RootFields) == 0 {
		return nil
	}
	subgraph := c.caching.Resolve(info.DataSourceID)
	var cfg resolve.FetchCacheConfig
	if fetch.IsEntityFetch() || fetch.IsBatchEntityFetch() {
		// The key spec derives from RootFields[0].TypeName, so a fetch resolving
		// entities of MIXED types (an abstract-path entity fetch collects one
		// root coordinate per enclosing concrete type) has no single key space —
		// decline caching entirely, the conservative mirror of the root-field
		// all-or-nothing rule below.
		for _, rootField := range info.RootFields[1:] {
			if rootField.TypeName != info.RootFields[0].TypeName {
				return nil
			}
		}
		spec, ok := buildEntitySpec(fetch, info)
		if !ok {
			return nil
		}
		entity, ok := subgraph.Entities(entityTypeNames(info, spec.Representation))
		if !ok {
			return nil
		}
		cfg = resolve.FetchCacheConfig{
			// L1 marks ELIGIBILITY here; optimizeL1Cache narrows it to the
			// fetches whose values are actually reusable within the request.
			L1:                     true,
			L2:                     true,
			SubgraphName:           info.DataSourceName,
			TTL:                    entity.TTL,
			MaxTTL:                 subgraph.MaxTTL,
			NegativeCacheTTL:       subgraph.NegativeCacheTTL,
			IncludeSubgraphHeaders: subgraph.IncludeSubgraphHeaders,
			Private:                entity.Scope == cacheconfig.CacheScopePrivate,
			EnablePartialCacheLoad: subgraph.EnablePartialCacheLoad,
			ShadowMode:             subgraph.ShadowMode,
			HashAnalyticsKeys:      subgraph.HashAnalyticsKeys,
			KeySpec:                spec,
		}
	} else {
		entry, ok := rootFieldConfigForAllRootFields(subgraph, info)
		if !ok {
			return nil
		}
		spec, ok := buildRootFieldSpec(info)
		if !ok {
			return nil
		}
		cfg = resolve.FetchCacheConfig{
			// Root fields act only as L2 providers, never L1.
			L1:                     false,
			L2:                     entry.TTL > 0,
			SubgraphName:           info.DataSourceName,
			TTL:                    entry.TTL,
			MaxTTL:                 subgraph.MaxTTL,
			IncludeSubgraphHeaders: entry.IncludeSubgraphHeaders,
			Private:                entry.Scope == cacheconfig.CacheScopePrivate,
			ShadowMode:             entry.ShadowMode,
			PartialBatchLoad:       entry.PartialBatchLoad,
			HashAnalyticsKeys:      subgraph.HashAnalyticsKeys,
			KeySpec:                spec,
		}
	}
	cfg.ProvidesData = pd[info]
	if cfg.ProvidesData != nil {
		resolve.ComputeHasAliases(cfg.ProvidesData)
	}
	if !cfg.L1 && !cfg.L2 && !cfg.ShadowMode {
		// All-flags-false safety net: a config that enables nothing must not
		// reach the loader (the per-fetch gate is cfg == nil).
		return nil
	}
	return &cfg
}

// entityTypeNames lists the entity types ONE entity fetch can resolve: its
// root-field type plus every type its merged representation conditions fields
// on. A batch fetch over an abstract type sends representations of several
// concrete types under one fetch, and each of them may carry its own static
// declaration, so all of them contribute to the fetch's resolved static tier.
func entityTypeNames(info *resolve.FetchInfo, representation *resolve.Object) []string {
	names := []string{info.RootFields[0].TypeName}
	if representation == nil {
		return names
	}
	for _, field := range representation.Fields {
		for _, onTypeName := range field.OnTypeNames {
			if name := string(onTypeName); !slices.Contains(names, name) {
				names = append(names, name)
			}
		}
	}
	return names
}

// rootFieldConfigForAllRootFields returns an entry only when EVERY root field
// of the fetch resolves to the SAME cache settings. A merged fetch mixing
// settings (or mixing cached and uncached fields) declines caching entirely —
// the conservative safety net that the per-root-field planner isolation keeps
// rare.
func rootFieldConfigForAllRootFields(subgraph cacheconfig.EffectiveSubgraphConfig, info *resolve.FetchInfo) (cacheconfig.RootFieldCacheConfig, bool) {
	first, ok := subgraph.RootField(info.RootFields[0].TypeName, info.RootFields[0].FieldName)
	if !ok {
		return cacheconfig.RootFieldCacheConfig{}, false
	}
	for _, rootField := range info.RootFields[1:] {
		entry, ok := subgraph.RootField(rootField.TypeName, rootField.FieldName)
		if !ok || !sameRootFieldCacheConfig(first, entry) {
			return cacheconfig.RootFieldCacheConfig{}, false
		}
	}
	return first, true
}

// sameRootFieldCacheConfig compares the cache settings, excluding the
// coordinate: a merged fetch whose root fields all carry equal settings is
// cacheable as one unit (the value covers all its fields; coverage guards
// servability).
func sameRootFieldCacheConfig(a, b cacheconfig.RootFieldCacheConfig) bool {
	return a.TTL == b.TTL &&
		a.IncludeSubgraphHeaders == b.IncludeSubgraphHeaders &&
		a.Scope == b.Scope &&
		a.ShadowMode == b.ShadowMode &&
		a.PartialBatchLoad == b.PartialBatchLoad
}
