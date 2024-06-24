package plan

import (
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astminify"
)

type SubgraphRequestMinifier interface {
	EnableSubgraphRequestMinifier(options astminify.MinifyOptions)
}
