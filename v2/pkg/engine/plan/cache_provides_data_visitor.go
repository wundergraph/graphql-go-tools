package plan

import (
	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astvisitor"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

type cacheProvidesDataVisitor struct {
	*astvisitor.Walker
	operation, definition *ast.Document
	planners              []PlannerConfiguration
	fieldPlanners         map[int][]int
	objects               map[int]*resolve.Object
}

func (v *cacheProvidesDataVisitor) EnterField(ref int) {
}

func (v *cacheProvidesDataVisitor) LeaveField(ref int) {
}

func (v *cacheProvidesDataVisitor) attachTo(p Plan) {
	resp := responseOf(p)
	if resp == nil {
		return
	}
	resp.SetCacheProvidesData(map[*resolve.FetchInfo]*resolve.Object{})
}

func responseOf(p Plan) *resolve.GraphQLResponse {
	switch t := p.(type) {
	case *SynchronousResponsePlan:
		return t.Response
	case *DeferResponsePlan:
		if t.Response == nil {
			return nil
		}
		return t.Response.Response
	case *SubscriptionResponsePlan:
		if t.Response == nil {
			return nil
		}
		return t.Response.Response
	default:
		return nil
	}
}
