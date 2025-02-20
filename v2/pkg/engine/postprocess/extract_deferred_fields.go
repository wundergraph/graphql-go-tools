package postprocess

import "github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"

type extractDeferredFields struct{}

func (e *extractDeferredFields) processDeferred(resp *resolve.GraphQLIncrementalResponse) {
	resp.DeferredResponses = &resolve.GraphQLResponse{
		Data: &resolve.Object{},
	}
	resp.DeferredResponses.Data.Fetches = append(resp.DeferredResponses.Data.Fetches, resp.ImmediateResponse.Data.Fetches...)

	visitor := &deferredFieldsVisitor{
		destination: resp.DeferredResponses.Data,
	}
	walker := &PlanWalker{
		objectVisitor: visitor,
		fieldVisitor:  visitor,
	}
	walker.Walk(resp.ImmediateResponse.Data, resp.ImmediateResponse.Info)
}

type deferredFieldsVisitor struct {
	destination    *resolve.Object
	retainedFields []*resolve.Field
	currentObject  *resolve.Object
}

func (v *deferredFieldsVisitor) EnterObject(obj *resolve.Object) {
	v.currentObject = obj
	v.retainedFields = nil
}

func (v *deferredFieldsVisitor) LeaveObject(*resolve.Object) {
	v.currentObject.Fields = v.retainedFields
}

func (v *deferredFieldsVisitor) EnterField(field *resolve.Field) {
	if field.Defer == nil {
		v.retainedFields = append(v.retainedFields, field)
		return
	}
	v.destination.Fields = append(v.destination.Fields, field)

	if len(v.destination.Path) == 0 {
		v.destination.Path = v.currentObject.Path
	}
}

func (v *deferredFieldsVisitor) LeaveField(*resolve.Field) {
}
