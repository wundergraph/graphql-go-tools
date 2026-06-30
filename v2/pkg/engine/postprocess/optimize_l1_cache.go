package postprocess

import "github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"

type optimizeL1Cache struct {
	disable bool
}

func (o *optimizeL1Cache) processTrees(roots ...*resolve.FetchTreeNode) {
	if o.disable {
		return
	}
}
