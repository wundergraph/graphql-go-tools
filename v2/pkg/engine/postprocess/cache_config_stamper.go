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
		s.processFetch(node.Item.Fetch, pd)
	case resolve.FetchTreeNodeKindParallel, resolve.FetchTreeNodeKindSequence:
		for _, child := range node.ChildNodes {
			s.process(child, pd)
		}
	}
}

func (s *cacheConfigStamper) processFetch(fetch resolve.Fetch, pd map[*resolve.FetchInfo]*resolve.Object) {
	info := fetch.FetchInfo()
	if s.buildConfig(info, pd[info]) == nil {
		return
	}
}

func (s *cacheConfigStamper) buildConfig(info *resolve.FetchInfo, providesData *resolve.Object) *resolve.FetchCacheConfig {
	return nil
}
