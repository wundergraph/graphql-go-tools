package cache

import (
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

// optimizeL1Cache narrows cfg.L1 CROSS-TREE after all fetch configs are set:
// L1 stays enabled only where a value written by one fetch can actually be
// reused by another within the same request.
//
// Skeleton only — the narrowing pass lands with task 16.
type optimizeL1Cache struct{}

// optimize runs the cross-tree narrowing over all fetch trees of one response.
// Inert until task 16.
func (o *optimizeL1Cache) optimize(trees []*resolve.FetchTreeNode) {
}
