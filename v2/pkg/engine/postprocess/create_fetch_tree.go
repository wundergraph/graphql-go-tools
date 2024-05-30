package postprocess

import (
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

type CreateFetchTree struct {
	*PlanWalker

	data *resolve.Object

	currentObjects []*resolve.Object
	currentFields  []*resolve.Field

	fieldForPath  map[string]*resolve.Field
	objectForPath map[string]*resolve.Object
	fieldHasFetch map[*resolve.Field]struct{}
}

func NewFetchTreeCreator(fieldHasFetch map[*resolve.Field]struct{}) *CreateFetchTree {
	e := &CreateFetchTree{
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

func (e *CreateFetchTree) ExtractFetchTree(res *resolve.GraphQLResponse) *resolve.Object {
	e.Walk(res.Data, res.Info)

	return e.data
}

func (e *CreateFetchTree) EnterArray(array *resolve.Array) {
	currentField := e.currentFields[len(e.currentFields)-1]

	switch currentField.Value.(type) {
	case *resolve.Array:
		// nothing to do
	case nil:
		cloned := e.CloneArray(array)
		currentField.Value = cloned
	default:
		panic("should not happen")
	}
}

func (e *CreateFetchTree) CloneArray(a *resolve.Array) *resolve.Array {
	return &resolve.Array{
		Nullable: true, // fetches do not care about nullability
		Path:     a.Path,
	}
}

func (e *CreateFetchTree) LeaveArray(_ *resolve.Array) {

}

func (e *CreateFetchTree) EnterObject(object *resolve.Object) {
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

func (e *CreateFetchTree) setFieldObject(o *resolve.Object) {
	currentField := e.currentFields[len(e.currentFields)-1]

	switch t := currentField.Value.(type) {
	case nil:
		currentField.Value = o
	case *resolve.Object:
		// nothing to do
	case *resolve.Array:
		t.Item = o
	default:
		panic("should not happen")
	}
}

func (e *CreateFetchTree) appendFetch(existing resolve.Fetch, additional resolve.Fetch) resolve.Fetch {
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
	case nil:
		return additional
	default:
		panic("there should be no other fetch types")
	}

	return existing
}

func (e *CreateFetchTree) CloneObject(o *resolve.Object) *resolve.Object {
	return &resolve.Object{
		Nullable: true, // fetches do not care about nullability
		Path:     o.Path,
		Fetch:    o.Fetch,
	}
}

func (e *CreateFetchTree) LeaveObject(_ *resolve.Object) {
	e.currentObjects = e.currentObjects[:len(e.currentObjects)-1]
}

func (e *CreateFetchTree) EnterField(field *resolve.Field) {
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

func (e *CreateFetchTree) CloneField(f *resolve.Field) *resolve.Field {
	return &resolve.Field{
		Name: f.Name,
	}
}

func (e *CreateFetchTree) LeaveField(field *resolve.Field) {
	if _, ok := e.fieldHasFetch[field]; !ok {
		e.SetSkip(false)
		return
	}

	e.currentFields = e.currentFields[:len(e.currentFields)-1]
}
