package postprocess

import (
	"slices"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

type extractDeferredFields struct{}

func (e *extractDeferredFields) Process(resp *resolve.GraphQLResponse) {
	baseResponse := &resolve.GraphQLResponse{
		Info: resp.Info,
	}
	visitor := &deferredFieldsVisitor{
		responseStack: []*responseItem{
			{
				response: baseResponse,
			},
		},
	}
	visitor.walker = &PlanWalker{
		objectVisitor: visitor,
		fieldVisitor:  visitor,
	}
	visitor.walker.Walk(resp.Data, resp.Info)

	for len(visitor.responseStack) > 1 {
		visitor.leaveDefer()
	}

	resp.Data.Fields = visitor.rootObject.Fields
	resp.DeferredResponses = baseResponse.DeferredResponses
}

type deferredFieldsVisitor struct {
	rootObject    *resolve.Object
	responseStack []*responseItem
	lastArray     *resolve.Array

	walker *PlanWalker
}

type responseItem struct {
	response *resolve.GraphQLResponse

	objectStack []*resolve.Object

	deferPath []string
}

func (v *deferredFieldsVisitor) EnterObject(obj *resolve.Object) {
	newObject := &resolve.Object{
		Nullable: obj.Nullable,
		Path:     obj.Path,

		// Leave Fields empty for now. They will be filled in by the field visitor.
		PossibleTypes: obj.PossibleTypes,
		TypeName:      obj.TypeName,

		Fetches: obj.Fetches,
	}

	if len(v.walker.CurrentFields) > 0 {
		switch v.walker.CurrentFields[len(v.walker.CurrentFields)-1].Value.(type) {
		case *resolve.Array:
			// This object is an item in an array.
			if v.lastArray != nil {
				if _, ok := v.lastArray.Item.(*resolve.Object); ok {
					v.lastArray.Item = newObject
				}
			}
		case *resolve.Object:
			// This object is a field in another object.
			if v.currentObject() != nil && len(v.currentObject().Fields) > 0 {
				v.currentObject().Fields[len(v.currentObject().Fields)-1].Value = newObject
			}
		}
	}

	resp := v.currentResponseItem()
	resp.objectStack = append(resp.objectStack, newObject)

	if v.rootObject == nil {
		v.rootObject = v.currentObject()
	}

	if v.currentResponseItem().response.Data == nil {
		v.currentResponseItem().response.Data = v.currentObject()
	}
}

func (v *deferredFieldsVisitor) LeaveObject(*resolve.Object) {
	resp := v.currentResponseItem()
	if depth := len(resp.objectStack); depth > 1 {
		resp.objectStack = resp.objectStack[:depth-1]
	}
}

func (v *deferredFieldsVisitor) EnterField(field *resolve.Field) {
	if field.Defer == nil {
		v.appendField(field)
		return
	}
	// A deferred field.

	var currentDeferPath []string
	if v.currentResponseItem() != nil {
		currentDeferPath = v.currentResponseItem().deferPath
	}

	if len(field.Defer.Path) > len(currentDeferPath) {
		v.enterDefer(field)
		return
	}

	for len(field.Defer.Path) < len(currentDeferPath) {
		v.leaveDefer()
		currentDeferPath = v.currentResponseItem().deferPath
	}
	if slices.Equal(currentDeferPath, field.Defer.Path) {
		// Same defer, just add the field to it.
		v.appendField(field)
	} else {
		// Different defer, start a new one.
		v.leaveDefer()
		v.enterDefer(field)
	}
}

func (v *deferredFieldsVisitor) LeaveField(field *resolve.Field) {
	// Nothing to do here.
}

func (v *deferredFieldsVisitor) currentObject() *resolve.Object {
	resp := v.currentResponseItem()
	if resp == nil {
		return nil
	}
	if len(resp.objectStack) == 0 {
		return nil
	}
	return resp.objectStack[len(resp.objectStack)-1]
}

func (v *deferredFieldsVisitor) currentResponseItem() *responseItem {
	if len(v.responseStack) == 0 {
		return nil // panic?
	}
	return v.responseStack[len(v.responseStack)-1]
}

func (v *deferredFieldsVisitor) appendField(field *resolve.Field) {
	newField := copyFieldWithoutObjectFields(field)
	v.currentObject().Fields = append(v.currentObject().Fields, newField)

	if _, ok := field.Value.(*resolve.Array); ok {
		v.lastArray = newField.Value.(*resolve.Array)
	} else {
		v.lastArray = nil
	}
}

func (v *deferredFieldsVisitor) enterDefer(field *resolve.Field) {
	// Start a new subdefer.
	preDeferObject := v.currentObject()
	newObj := &resolve.Object{
		Nullable: preDeferObject.Nullable,
		// Field will be appended below.
		Path:    preDeferObject.Path,
		Fetches: v.fetchesFromPath(),

		PossibleTypes: preDeferObject.PossibleTypes,
		TypeName:      preDeferObject.TypeName,
	}

	parentResponse := v.currentResponseItem()
	newResponse := &resolve.GraphQLResponse{
		Data: newObj,
		Info: parentResponse.response.Info,
	}
	parentResponse.response.DeferredResponses = append(parentResponse.response.DeferredResponses, newResponse)

	v.responseStack = append(v.responseStack, &responseItem{
		response:    newResponse,
		deferPath:   field.Defer.Path,
		objectStack: []*resolve.Object{newObj},
	})
	v.appendField(field)
}

func (v *deferredFieldsVisitor) leaveDefer() {
	// Don't let the response stack drain.
	if len(v.responseStack) <= 1 {
		return
	}
	v.responseStack = v.responseStack[:len(v.responseStack)-1]
}

func (v *deferredFieldsVisitor) fetchesFromPath() []resolve.Fetch {
	for ri := len(v.responseStack) - 1; ri >= 0; ri-- {
		resp := v.responseStack[ri]

		for oi := len(resp.objectStack) - 1; oi >= 0; oi-- {
			if len(resp.objectStack[oi].Fetches) > 0 {
				return resp.objectStack[oi].Fetches
			}
		}
	}
	return nil
}

func copyFieldWithoutObjectFields(f *resolve.Field) *resolve.Field {
	switch fv := f.Value.(type) {
	case *resolve.Object:
		ret := &resolve.Field{
			Name:              f.Name,
			Position:          f.Position,
			Defer:             f.Defer,
			Stream:            f.Stream,
			OnTypeNames:       f.OnTypeNames,
			ParentOnTypeNames: f.ParentOnTypeNames,
			Info:              f.Info,
		}
		newValue := fv.Copy().(*resolve.Object)
		newValue.Fields = nil

		if len(fv.PossibleTypes) > 0 {
			possibleTypes := make(map[string]struct{}, len(fv.PossibleTypes))
			for k, v := range fv.PossibleTypes {
				possibleTypes[k] = v
			}
			newValue.PossibleTypes = possibleTypes
		}
		newValue.SourceName = fv.SourceName
		newValue.TypeName = fv.TypeName
		newValue.Fetches = fv.Fetches

		ret.Value = newValue
		return ret
	case *resolve.Array:
		arrObj, ok := fv.Item.(*resolve.Object)
		if !ok {
			return f.Copy()
		}
		ret := &resolve.Field{
			Name:              f.Name,
			Position:          f.Position,
			Defer:             f.Defer,
			Stream:            f.Stream,
			OnTypeNames:       f.OnTypeNames,
			ParentOnTypeNames: f.ParentOnTypeNames,
			Info:              f.Info,
		}
		newItem := arrObj.Copy().(*resolve.Object)
		newItem.Fields = nil

		ret.Value = &resolve.Array{
			Path:     fv.Path,
			Nullable: fv.Nullable,
			Item:     newItem,
		}
		return ret
	default:
		return f.Copy()
	}
}
