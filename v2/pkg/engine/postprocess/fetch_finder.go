package postprocess

import (
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

type FetchFinder struct {
	*PlanWalker

	fieldHasFetches map[*resolve.Field]struct{}
}

func NewFetchFinder() *FetchFinder {
	e := &FetchFinder{
		fieldHasFetches: make(map[*resolve.Field]struct{}),
		PlanWalker:      &PlanWalker{},
	}

	e.registerObjectVisitor(e)

	return e
}

func (e *FetchFinder) Find(pre plan.Plan) map[*resolve.Field]struct{} {
	switch t := pre.(type) {
	case *plan.SynchronousResponsePlan:
		e.Walk(t.Response.Data, t.Response.Info)
	case *plan.SubscriptionResponsePlan:
		e.Walk(t.Response.Response.Data, t.Response.Response.Info)
	}
	return e.fieldHasFetches
}

func (e *FetchFinder) markCurrentFieldsHasFetch() {
	for i := range e.CurrentFields {
		e.fieldHasFetches[e.CurrentFields[i]] = struct{}{}
	}
}

func (e *FetchFinder) EnterObject(object *resolve.Object) {
	if object.Fetch != nil {
		e.markCurrentFieldsHasFetch()
	}
}

func (e *FetchFinder) LeaveObject(object *resolve.Object) {
}
