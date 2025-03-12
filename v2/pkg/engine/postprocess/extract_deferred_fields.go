package postprocess

import (
	"iter"
	"slices"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

type extractDeferredFields struct{}

func (e *extractDeferredFields) Process(resp *resolve.GraphQLResponse, defers []resolve.DeferInfo) {
	visitor := newDeferredFieldsVisitor(resp, defers)
	visitor.walker = &PlanWalker{
		objectVisitor: visitor,
		fieldVisitor:  visitor,
	}
	visitor.walker.Walk(resp.Data, resp.Info)

	for key, di := range visitor.responseItems {
		if key == "" {
			resp.Data.Fields = di.response.Data.Fields
			resp.Data.Fetches = di.response.Data.Fetches
			continue
		}
		resp.DeferredResponses = append(resp.DeferredResponses, di.response)
	}
}

func newDeferredFieldsVisitor(resp *resolve.GraphQLResponse, defers []resolve.DeferInfo) *deferredFieldsVisitor {
	ret := &deferredFieldsVisitor{
		deferredFragments: defers,
		responseItems:     make(map[string]*responseItem, len(defers)),
	}

	for _, di := range defers {
		ret.responseItems[di.Path.DotDelimitedString()] = &responseItem{
			deferInfo: &di,
			response: &resolve.GraphQLResponse{
				Info: resp.Info,
			},
		}
	}

	// The immediate response.
	ret.responseItems[""] = &responseItem{
		response: &resolve.GraphQLResponse{
			Info: resp.Info,
		},
	}
	return ret
}

type deferredFieldsVisitor struct {
	deferredFragments []resolve.DeferInfo
	responseItems     map[string]*responseItem

	walker *PlanWalker
}

func (v *deferredFieldsVisitor) EnterObject(obj *resolve.Object) {
	var (
		currentField *resolve.Field
	)

	if len(v.walker.CurrentFields) > 0 {
		currentField = v.walker.CurrentFields[len(v.walker.CurrentFields)-1]
	}

	for resp := range v.matchingResponseItems() {
		newObject := resp.copyObjectWithoutFields(obj)

		if currentField != nil {
			if resp.deferInfo == nil || slices.ContainsFunc(currentField.DeferPaths, func(el ast.Path) bool {
				return resp.deferInfo != nil && el.Equals(resp.deferInfo.Path)
			}) {
				resp.updateCurrentFieldObject(currentField, newObject)
			}
		}
		resp.objectStack = append(resp.objectStack, newObject)

		if resp.response.Data == nil {
			resp.response.Data = newObject
		}
	}
}

func (v *deferredFieldsVisitor) LeaveObject(*resolve.Object) {
	for resp := range v.matchingResponseItems() {
		if depth := len(resp.objectStack); depth > 1 {
			resp.objectStack = resp.objectStack[:depth-1]
		}
	}
}

func (v *deferredFieldsVisitor) EnterField(field *resolve.Field) {
	// Reasons to append a field:
	// 1. It's above a defer fragment. matchingResponseItems does this.
	// 2. It's marked as deferred for some responses, send it there. The field has this information.
	// 3. It's not marked as deferred, so sent to the immediate response.
	// 4. It's above a non-deferred field? TODO(cd): post-cleanup.
	var deferred bool
	for resp := range v.matchingResponseItems() {
		if slices.ContainsFunc(field.DeferPaths, func(el ast.Path) bool {
			return resp.deferInfo != nil && el.Equals(resp.deferInfo.Path)
		}) {
			resp.appendField(field)
			deferred = true
		}
	}
	resp := v.responseItems[""]

	switch field.Value.(type) {
	case *resolve.Object, *resolve.Array:
		resp.appendField(field)
	default:
		if !deferred {
			resp.appendField(field)
		}
	}
}

func (v *deferredFieldsVisitor) LeaveField(field *resolve.Field) {}

func (v *deferredFieldsVisitor) matchingResponseItems() iter.Seq[*responseItem] {
	return func(yield func(*responseItem) bool) {
		for path, resp := range v.responseItems {
			if path == "" || resp.deferInfo.HasPrefix(v.walker.path) {
				if !yield(resp) {
					return
				}
			}
		}
	}
}

type responseItem struct {
	deferInfo   *resolve.DeferInfo
	response    *resolve.GraphQLResponse
	objectStack []*resolve.Object
	lastArray   *resolve.Array
}

func (r *responseItem) currentObject() *resolve.Object {
	if len(r.objectStack) == 0 {
		return nil
	}
	return r.objectStack[len(r.objectStack)-1]
}

func (r *responseItem) appendField(field *resolve.Field) {
	newField := r.copyFieldWithoutObjectFields(field)
	r.currentObject().Fields = append(r.currentObject().Fields, newField)

	if _, ok := field.Value.(*resolve.Array); ok {
		r.lastArray = newField.Value.(*resolve.Array)
	} else {
		r.lastArray = nil
	}
}

func (r *responseItem) copyFieldWithoutObjectFields(f *resolve.Field) *resolve.Field {
	switch fv := f.Value.(type) {
	case *resolve.Object:
		ret := &resolve.Field{
			Name:              f.Name,
			Position:          f.Position,
			DeferPaths:        f.DeferPaths,
			Stream:            f.Stream,
			OnTypeNames:       f.OnTypeNames,
			ParentOnTypeNames: f.ParentOnTypeNames,
			Info:              f.Info,
		}
		ret.Value = r.copyObjectWithoutFields(fv)
		return ret
	case *resolve.Array:
		arrObj, ok := fv.Item.(*resolve.Object)
		if !ok {
			return f.Copy()
		}
		ret := &resolve.Field{
			Name:              f.Name,
			Position:          f.Position,
			DeferPaths:        f.DeferPaths,
			Stream:            f.Stream,
			OnTypeNames:       f.OnTypeNames,
			ParentOnTypeNames: f.ParentOnTypeNames,
			Info:              f.Info,
		}
		newItem := r.copyObjectWithoutFields(arrObj)

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

func (r *responseItem) copyObjectWithoutFields(fv *resolve.Object) *resolve.Object {
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
	newValue.Fetches = r.fetchesForDeferFrom(fv.Fetches)

	return newValue
}

func (r *responseItem) updateCurrentFieldObject(field *resolve.Field, obj *resolve.Object) {
	switch field.Value.(type) {
	case *resolve.Array:
		// This object is an item in an array.
		if r.lastArray != nil {
			if _, ok := r.lastArray.Item.(*resolve.Object); ok {
				r.lastArray.Item = obj
			}
		}
	case *resolve.Object:
		// This object is a field in another object.
		if r.currentObject() != nil && len(r.currentObject().Fields) > 0 {
			r.currentObject().Fields[len(r.currentObject().Fields)-1].Value = obj
		}
	}
}

func (r *responseItem) fetchesForDeferFrom(fetches []resolve.Fetch) []resolve.Fetch {
	var ret []resolve.Fetch
	for _, fetch := range fetches {
		if single, ok := fetch.(*resolve.SingleFetch); ok && single != nil {
			if !single.DeferInfo.Equals(r.deferInfo) {
				continue
			}
		}
		ret = append(ret, fetch)
	}
	return ret
}
