package postprocess

import (
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

type CreateFetchesCopy struct {
	*PlanWalker

	data *resolve.Object

	currentObjects []*resolve.Object
	currentFields  []*resolve.Field

	fieldForPath  map[string]*resolve.Field
	objectForPath map[string]*resolve.Object
	fieldHasFetch map[*resolve.Field]struct{}
}

func NewFetchesCopier(fieldHasFetch map[*resolve.Field]struct{}) *CreateFetchesCopy {
	e := &CreateFetchesCopy{
		PlanWalker:    &PlanWalker{},
		objectForPath: make(map[string]*resolve.Object),
		fieldForPath:  make(map[string]*resolve.Field),
		fieldHasFetch: fieldHasFetch,
	}

	e.registerObjectVisitor(e)
	e.registerArrayVisitor(e)
	e.registerFieldVisitor(e)

	return e
}

func (e *CreateFetchesCopy) Process(pre plan.Plan) plan.Plan {
	switch t := pre.(type) {
	case *plan.SynchronousResponsePlan:
		e.Walk(t.Response.Data, t.Response.Info)

		t.Response.FetchData = e.data

	case *plan.SubscriptionResponsePlan:
		e.Walk(t.Response.Response.Data, t.Response.Response.Info)

		t.Response.Response.FetchData = e.data
	}
	return pre
}

func (e *CreateFetchesCopy) EnterArray(array *resolve.Array) {
	currentField := e.currentFields[len(e.currentFields)-1]

	switch currentField.Value.(type) {
	case *resolve.Object:
		panic("not implemented")
	case *resolve.Array:
		// nothing to do
	case nil:
		cloned := e.CloneArray(array)
		currentField.Value = cloned
	}

}

func (e *CreateFetchesCopy) CloneArray(a *resolve.Array) *resolve.Array {
	return &resolve.Array{
		Nullable: a.Nullable,
		Path:     a.Path,
	}
}

func (e *CreateFetchesCopy) LeaveArray(array *resolve.Array) {

}

func (e *CreateFetchesCopy) EnterObject(object *resolve.Object) {
	if e.data == nil {
		e.data = e.CloneObject(object)
		e.currentObjects = append(e.currentObjects, e.data)
		object.Fetch = nil
		return
	}

	currentPath := e.renderPath()

	objectForPath, ok := e.objectForPath[currentPath]
	if !ok {
		cloned := e.CloneObject(object)
		e.objectForPath[currentPath] = cloned
		e.currentObjects = append(e.currentObjects, cloned)
		e.setFieldObject(cloned)
		object.Fetch = nil
		return
	}

	if object.Fetch != nil {
		objectForPath.Fetch = e.appendFetch(objectForPath.Fetch, object.Fetch)
		object.Fetch = nil
	}

	e.currentObjects = append(e.currentObjects, objectForPath)
}

func (e *CreateFetchesCopy) setFieldObject(o *resolve.Object) {
	currentField := e.currentFields[len(e.currentFields)-1]

	if currentField.Value == nil {
		currentField.Value = o
		return
	}

	switch t := currentField.Value.(type) {
	case *resolve.Object:
		panic("not implemented")
	case *resolve.Array:
		t.Item = o
	default:
		panic("not implemented")
	}
}

func (e *CreateFetchesCopy) appendFetch(existing resolve.Fetch, additional resolve.Fetch) resolve.Fetch {
	switch t := existing.(type) {
	case *resolve.SingleFetch:
		switch at := additional.(type) {
		case *resolve.SingleFetch:
			return &resolve.MultiFetch{
				Fetches: []*resolve.SingleFetch{t, at},
			}
		case *resolve.MultiFetch:
			return &resolve.MultiFetch{
				Fetches: append([]*resolve.SingleFetch{t}, at.Fetches...),
			}
		}
	case *resolve.MultiFetch:
		switch at := additional.(type) {
		case *resolve.SingleFetch:
			t.Fetches = append(t.Fetches, at)
		case *resolve.MultiFetch:
			t.Fetches = append(t.Fetches, at.Fetches...)
		}
		return t
	}

	return existing
}

func (e *CreateFetchesCopy) CloneObject(o *resolve.Object) *resolve.Object {
	return &resolve.Object{
		Nullable:             o.Nullable,
		Path:                 o.Path,
		Fetch:                o.Fetch,
		UnescapeResponseJson: o.UnescapeResponseJson,
	}
}

func (e *CreateFetchesCopy) LeaveObject(object *resolve.Object) {
	e.currentObjects = e.currentObjects[:len(e.currentObjects)-1]
}

func (e *CreateFetchesCopy) EnterField(field *resolve.Field) {
	if _, ok := e.fieldHasFetch[field]; !ok {
		e.SetSkip(true)
		return
	}

	currentPath := e.renderPath()

	existingField, ok := e.fieldForPath[currentPath]
	if !ok {
		clonedField := e.CloneField(field)
		e.currentFields = append(e.currentFields, clonedField)
		e.currentObjects[len(e.currentObjects)-1].Fields = append(e.currentObjects[len(e.currentObjects)-1].Fields, clonedField)
		e.fieldForPath[currentPath] = clonedField
		return
	}

	e.currentFields = append(e.currentFields, existingField)
}

func (e *CreateFetchesCopy) CloneField(f *resolve.Field) *resolve.Field {
	return &resolve.Field{
		Name: f.Name,
	}
}

func (e *CreateFetchesCopy) LeaveField(field *resolve.Field) {
	if _, ok := e.fieldHasFetch[field]; !ok {
		e.SetSkip(false)
		return
	}

	e.currentFields = e.currentFields[:len(e.currentFields)-1]
}
