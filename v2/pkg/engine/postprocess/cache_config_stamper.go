package postprocess

import (
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan/cacheconfig"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

type cacheConfigStamper struct {
	providers map[string]cacheconfig.CacheConfigProvider
	freezer   *cacheKeySpecFreezer
}

func (s *cacheConfigStamper) process(node *resolve.FetchTreeNode, pd map[*resolve.FetchInfo]*resolve.Object) {
	if node == nil {
		return
	}
	switch node.Kind {
	case resolve.FetchTreeNodeKindSingle:
		if node.Item == nil || node.Item.Fetch == nil {
			return
		}
		s.stamp(node.Item.Fetch, pd)
	case resolve.FetchTreeNodeKindParallel, resolve.FetchTreeNodeKindSequence:
		for _, child := range node.ChildNodes {
			s.process(child, pd)
		}
	}
}

func (s *cacheConfigStamper) stamp(fetch resolve.Fetch, pd map[*resolve.FetchInfo]*resolve.Object) {
	cfg := s.buildConfig(fetch, pd)
	if cfg == nil {
		return
	}
	switch f := fetch.(type) {
	case *resolve.SingleFetch:
		f.Cache = cfg
	case *resolve.EntityFetch:
		f.Cache = cfg
	case *resolve.BatchEntityFetch:
		f.Cache = cfg
	}
}

func (s *cacheConfigStamper) buildConfig(fetch resolve.Fetch, pd map[*resolve.FetchInfo]*resolve.Object) *resolve.FetchCacheConfig {
	info := fetch.FetchInfo()
	if info == nil || len(info.RootFields) == 0 {
		return nil
	}
	provider := s.providers[info.DataSourceID]
	if provider == nil {
		return nil
	}

	var cfg resolve.FetchCacheConfig
	switch {
	case fetchIsEntity(fetch):
		pol, ok := provider.EntityPolicy(info.RootFields[0].TypeName)
		if !ok {
			return nil
		}
		spec, ok := s.freezer.freeze(resolve.CacheScopeEntity, info)
		if !ok {
			return nil
		}
		cfg = resolve.FetchCacheConfig{
			L1:                          true,
			L2:                          pol.TTL > 0 || pol.NegativeCacheTTL > 0,
			CacheName:                   pol.CacheName,
			TTL:                         pol.TTL,
			NegativeCacheTTL:            pol.NegativeCacheTTL,
			IncludeSubgraphHeaderPrefix: pol.IncludeSubgraphHeaderPrefix,
			EnablePartialCacheLoad:      pol.EnablePartialCacheLoad,
			ShadowMode:                  pol.ShadowMode,
			HashAnalyticsKeys:           pol.HashAnalyticsKeys,
			KeySpec:                     spec,
		}
	default:
		pol, ok := rootFieldPolicyForAllRootFields(provider, info)
		if !ok {
			return nil
		}
		spec, _ := s.freezer.freeze(resolve.CacheScopeRootField, info)
		cfg = resolve.FetchCacheConfig{
			L1:                          false,
			L2:                          pol.TTL > 0,
			CacheName:                   pol.CacheName,
			TTL:                         pol.TTL,
			IncludeSubgraphHeaderPrefix: pol.IncludeSubgraphHeaderPrefix,
			ShadowMode:                  pol.ShadowMode,
			PartialBatchLoad:            pol.PartialBatchLoad,
			KeySpec:                     spec,
		}
	}

	cfg.ProvidesData = pd[info]
	if cfg.ProvidesData != nil {
		resolve.ComputeHasAliases(cfg.ProvidesData)
	}
	if !cfg.L1 && !cfg.L2 && !cfg.ShadowMode {
		return nil
	}
	return &cfg
}

func fetchIsEntity(fetch resolve.Fetch) bool {
	switch f := fetch.(type) {
	case *resolve.EntityFetch, *resolve.BatchEntityFetch:
		return true
	case *resolve.SingleFetch:
		return f.RequiresEntityFetch || f.RequiresEntityBatchFetch
	default:
		return false
	}
}

func rootFieldPolicyForAllRootFields(provider cacheconfig.CacheConfigProvider, info *resolve.FetchInfo) (cacheconfig.RootFieldCachePolicy, bool) {
	if info == nil || len(info.RootFields) == 0 {
		return cacheconfig.RootFieldCachePolicy{}, false
	}
	first, ok := provider.RootFieldPolicy(info.RootFields[0].TypeName, info.RootFields[0].FieldName)
	if !ok {
		return cacheconfig.RootFieldCachePolicy{}, false
	}
	for _, rootField := range info.RootFields[1:] {
		pol, ok := provider.RootFieldPolicy(rootField.TypeName, rootField.FieldName)
		if !ok || !sameRootFieldCachePolicy(first, pol) {
			return cacheconfig.RootFieldCachePolicy{}, false
		}
	}
	return first, true
}

func sameRootFieldCachePolicy(a, b cacheconfig.RootFieldCachePolicy) bool {
	return a.CacheName == b.CacheName &&
		a.TTL == b.TTL &&
		a.IncludeSubgraphHeaderPrefix == b.IncludeSubgraphHeaderPrefix &&
		a.ShadowMode == b.ShadowMode &&
		a.PartialBatchLoad == b.PartialBatchLoad
}
