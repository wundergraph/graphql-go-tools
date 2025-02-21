package postprocess

import (
	"slices"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

type extractDeferredFields struct{}

func (e *extractDeferredFields) Process(resp *resolve.GraphQLResponse) {
	baseResponse := &resolve.GraphQLResponse{}
	visitor := &deferredFieldsVisitor{
		currentResponse: baseResponse,
		responseStack:   []*resolve.GraphQLResponse{baseResponse},
	}
	walker := &PlanWalker{
		objectVisitor: visitor,
		fieldVisitor:  visitor,
		arrayVisitor:  visitor,
	}
	walker.Walk(resp.Data, resp.Info)

	for len(visitor.responseStack) > 1 {
		visitor.leaveDefer()
	}

	resp.Data.Fields = visitor.rootObject.Fields
	resp.DeferredResponses = baseResponse.DeferredResponses
}

type deferredFieldsVisitor struct {
	currentObject *resolve.Object
	objectStack   []*resolve.Object
	rootObject    *resolve.Object

	currentResponse *resolve.GraphQLResponse // Just for convenience.
	responseStack   []*resolve.GraphQLResponse

	currentDeferredPath []string
}

func (v *deferredFieldsVisitor) EnterObject(obj *resolve.Object) {
	newObj := &resolve.Object{
		Nullable: obj.Nullable,
		Path:     obj.Path,
		Fetches:  obj.Fetches,
		// Leave Fields empty for now. They will be filled in by the field visitor.

		PossibleTypes: obj.PossibleTypes,
		TypeName:      obj.TypeName,
	}

	// This is likely the object for the last field of the current object.
	if v.currentObject != nil && len(v.currentObject.Fields) > 0 {
		v.currentObject.Fields[len(v.currentObject.Fields)-1].Value = newObj
	}
	v.currentObject = newObj
	v.objectStack = append(v.objectStack, v.currentObject)

	if v.rootObject == nil {
		v.rootObject = v.currentObject
	}

	if v.currentResponse.Data == nil {
		v.currentResponse.Data = v.currentObject
	}
}

func (v *deferredFieldsVisitor) LeaveObject(*resolve.Object) {
	if depth := len(v.objectStack); depth > 1 {
		v.currentObject = v.objectStack[depth-1]
		v.objectStack = v.objectStack[:depth-1]
	} else {
		v.currentObject = nil
		v.objectStack = nil
	}
}

func (v *deferredFieldsVisitor) EnterArray(obj *resolve.Array) {
	// TODO
}

func (v *deferredFieldsVisitor) LeaveArray(*resolve.Array) {
	// TODO
}

func (v *deferredFieldsVisitor) EnterField(field *resolve.Field) {
	if field.Defer == nil {
		dup := copyFieldWithoutObjectFields(field)
		v.currentObject.Fields = append(v.currentObject.Fields, dup)
		return
	}
	// A deferred field.
	switch {
	case v.currentDeferredPath == nil:
		v.enterDefer(field)
	case len(v.currentDeferredPath) == len(field.Defer.Path):
		if slices.Equal(v.currentDeferredPath, field.Defer.Path) {
			// Same defer, just add the field to it.
			v.currentObject.Fields = append(v.currentObject.Fields, copyFieldWithoutObjectFields(field))
		} else {
			// Different defer, start a new one.
			v.leaveDefer()
			v.enterDefer(field)
		}
	case len(v.currentDeferredPath) > len(field.Defer.Path):
		v.enterDefer(field)
	case len(v.currentDeferredPath) < len(field.Defer.Path):
		v.leaveDefer()
	}
}

func (v *deferredFieldsVisitor) LeaveField(field *resolve.Field) {
	// Nothing to do here.
}

func (v *deferredFieldsVisitor) enterDefer(field *resolve.Field) {
	// Start a new subdefer.
	parentObject := v.currentObject
	v.currentObject = &resolve.Object{
		Nullable: parentObject.Nullable,
		Fields:   []*resolve.Field{copyFieldWithoutObjectFields(field)},
		Path:     parentObject.Path,
		Fetches:  parentObject.Fetches,

		PossibleTypes: parentObject.PossibleTypes,
		TypeName:      parentObject.TypeName,
	}
	v.objectStack = append(v.objectStack, v.currentObject)

	parentResponse := v.currentResponse
	v.currentResponse = &resolve.GraphQLResponse{
		Data: v.currentObject,
	}
	parentResponse.DeferredResponses = append(parentResponse.DeferredResponses, v.currentResponse)

	v.responseStack = append(v.responseStack, v.currentResponse)

	v.currentDeferredPath = field.Defer.Path
}

func (v *deferredFieldsVisitor) leaveDefer() {
	// Add the response to the parent deferred responses.
	v.currentResponse = v.responseStack[len(v.responseStack)-2]
	v.responseStack = v.responseStack[:len(v.responseStack)-1]

	v.currentDeferredPath = nil
}

func copyFieldWithoutObjectFields(f *resolve.Field) *resolve.Field {
	switch fv := f.Value.(type) {
	case *resolve.Object:
		ret := &resolve.Field{
			Name:        f.Name,
			Position:    f.Position,
			Defer:       f.Defer,
			Stream:      f.Stream,
			OnTypeNames: f.OnTypeNames,
			Info:        f.Info,
		}
		if rv, ok := f.Value.(*resolve.Object); ok {
			newValue := rv.Copy().(*resolve.Object)
			newValue.Fields = nil

			possibleTypes := make(map[string]struct{}, len(fv.PossibleTypes))
			for k, v := range fv.PossibleTypes {
				possibleTypes[k] = v
			}
			newValue.PossibleTypes = possibleTypes
			newValue.SourceName = fv.SourceName
			newValue.TypeName = fv.TypeName
		}
		return ret
	default:
		return f.Copy()

	}
}
