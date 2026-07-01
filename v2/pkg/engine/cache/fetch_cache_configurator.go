package cache

import (
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan/cacheconfig"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

// fetchCacheConfigurator assembles the self-contained *resolve.FetchCacheConfig
// for each cache-eligible fetch (policy + built key spec + ProvidesData) and
// sets it on the concrete fetch via Fetch.SetCacheConfig — after
// createConcreteSingleFetchTypes, so the config lands on the final fetch types.
// Task 06 covers the entity arm; the root-field arm lands with task 13.
type fetchCacheConfigurator struct {
	providers  map[string]cacheconfig.CacheConfigProvider
	keyBuilder *cacheKeyBuilder
}

// configureTree walks one flat fetch tree and sets per-fetch cache config; a
// fetch keeps a nil Cache when its datasource has no provider, its coordinate
// has no policy, no usable key exists, or the assembled config enables nothing.
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
	provider := c.providers[info.DataSourceID]
	if provider == nil {
		return nil
	}
	var cfg resolve.FetchCacheConfig
	if fetch.IsEntityFetch() || fetch.IsBatchEntityFetch() {
		policy, ok := provider.EntityPolicy(info.RootFields[0].TypeName)
		if !ok {
			return nil
		}
		spec, ok := c.keyBuilder.buildEntitySpec(info)
		if !ok {
			return nil
		}
		cfg = resolve.FetchCacheConfig{
			// L1 marks ELIGIBILITY here; optimizeL1Cache (task 16) narrows it
			// to the fetches whose values are actually reusable within the
			// request.
			L1:                          true,
			L2:                          policy.TTL > 0 || policy.NegativeCacheTTL > 0,
			CacheName:                   policy.CacheName,
			TTL:                         policy.TTL,
			NegativeCacheTTL:            policy.NegativeCacheTTL,
			IncludeSubgraphHeaderPrefix: policy.IncludeSubgraphHeaderPrefix,
			EnablePartialCacheLoad:      policy.EnablePartialCacheLoad,
			ShadowMode:                  policy.ShadowMode,
			HashAnalyticsKeys:           policy.HashAnalyticsKeys,
			KeySpec:                     spec,
		}
	} else {
		policy, ok := rootFieldPolicyForAllRootFields(provider, info)
		if !ok {
			return nil
		}
		spec, ok := c.keyBuilder.buildRootFieldSpec(info)
		if !ok {
			return nil
		}
		cfg = resolve.FetchCacheConfig{
			// Root fields act only as L2 providers; root->entity L1 promotion
			// is an out-of-core follow-up.
			L1:                          false,
			L2:                          policy.TTL > 0,
			CacheName:                   policy.CacheName,
			TTL:                         policy.TTL,
			IncludeSubgraphHeaderPrefix: policy.IncludeSubgraphHeaderPrefix,
			ShadowMode:                  policy.ShadowMode,
			PartialBatchLoad:            policy.PartialBatchLoad,
			KeySpec:                     spec,
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

// rootFieldPolicyForAllRootFields returns a policy only when EVERY root field
// of the fetch resolves to the SAME policy. A merged fetch mixing policies (or
// mixing cached and uncached fields) declines caching entirely — the
// conservative safety net; task 14 splits such fetches so the decline stays a
// rare residual.
func rootFieldPolicyForAllRootFields(provider cacheconfig.CacheConfigProvider, info *resolve.FetchInfo) (cacheconfig.RootFieldCachePolicy, bool) {
	first, ok := provider.RootFieldPolicy(info.RootFields[0].TypeName, info.RootFields[0].FieldName)
	if !ok {
		return cacheconfig.RootFieldCachePolicy{}, false
	}
	for _, rootField := range info.RootFields[1:] {
		policy, ok := provider.RootFieldPolicy(rootField.TypeName, rootField.FieldName)
		if !ok || !sameRootFieldCachePolicy(first, policy) {
			return cacheconfig.RootFieldCachePolicy{}, false
		}
	}
	return first, true
}

// sameRootFieldCachePolicy compares the policy VALUES, excluding the
// coordinate: a merged fetch whose root fields all carry equal cache settings
// is cacheable as one unit (the value covers all its fields; coverage guards
// servability).
func sameRootFieldCachePolicy(a, b cacheconfig.RootFieldCachePolicy) bool {
	return a.CacheName == b.CacheName &&
		a.TTL == b.TTL &&
		a.IncludeSubgraphHeaderPrefix == b.IncludeSubgraphHeaderPrefix &&
		a.ShadowMode == b.ShadowMode &&
		a.PartialBatchLoad == b.PartialBatchLoad
}
