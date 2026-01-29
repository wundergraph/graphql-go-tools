package postprocess

import (
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan"
)

type deferProcessor struct {
	disable bool
}

func (d *deferProcessor) Process(deferPlan *plan.DeferResponsePlan) {

}