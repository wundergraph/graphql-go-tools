package postprocess

import (
	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

type cacheKeySpecFreezer struct {
	federation map[string]plan.FederationMetaData
	definition *ast.Document
}

func (f *cacheKeySpecFreezer) freeze(scope resolve.CacheScope, info *resolve.FetchInfo) (resolve.CacheKeySpec, bool) {
	return resolve.CacheKeySpec{}, false
}
