package cache

import (
	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan"
)

// cacheKeyBuilder freezes every resolvable @key set of a cached type into
// multi-key resolve.CacheKeySpec candidates BY VALUE. It is the SOLE
// federation reader on the plan side: no federation type or pointer ever
// reaches the runtime config it produces.
//
// Skeleton only — the freeze logic lands with task 06.
type cacheKeyBuilder struct {
	// federation is the per-datasource federation metadata, keyed by
	// datasource ID; read-only input to the freeze.
	federation map[string]plan.FederationMetaData
	// definition is the composed schema the @key selection sets resolve against.
	definition *ast.Document
}
