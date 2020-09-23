package postprocess

import (
	"github.com/jensneuse/graphql-go-tools/pkg/engine/plan"
)

type PostProcessor interface {
	Process(pre plan.Plan) plan.Plan
}
