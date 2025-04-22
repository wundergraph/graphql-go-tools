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
	if len(v.walker.CurrentFields) == 0 {
		// Set up root objects.
		for _, resp := range v.responseItems {
			newObject := resp.copyObjectWithoutFields(obj)
			resp.objectStack = append(resp.objectStack, newObject)

			if resp.response.Data == nil {
				resp.response.Data = newObject
			}
		}
		return
	}
}

func (v *deferredFieldsVisitor) LeaveObject(*resolve.Object) {}

func (v *deferredFieldsVisitor) EnterField(field *resolve.Field) {
	// Reasons to append a field:
	// 1. It's above a defer fragment. matchingResponseItems does this.
	// 2. It's marked as deferred for some responses, send it there. The field has this information.
	// 3. It's not marked as deferred, so sent to the immediate response.
	// 4. It's above a non-deferred field? TODO(cd): post-cleanup.
	var deferred bool
	for resp := range v.matchingResponseItems() {
		if resp.deferInfo != nil && slices.ContainsFunc(field.DeferPaths, func(el ast.Path) bool {
			return el.Equals(resp.deferInfo.Path)
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

func (v *deferredFieldsVisitor) LeaveField(field *resolve.Field) {
	popObjectStack := func(resp *responseItem) {
		if depth := len(resp.objectStack); depth > 1 {
			switch fv := field.Value.(type) {
			case *resolve.Object:
				resp.objectStack = resp.objectStack[:depth-1]
			case *resolve.Array:
				if _, ok := fv.Item.(*resolve.Object); ok {
					resp.objectStack = resp.objectStack[:depth-1]
				}
			}
		}
	}
	for resp := range v.matchingResponseItems() {
		popObjectStack(resp)
	}
	popObjectStack(v.responseItems[""])
}

// matchingResponseItems returns a sequence of response items that match the current path.
// It specifically excludes the immediate response.
func (v *deferredFieldsVisitor) matchingResponseItems() iter.Seq[*responseItem] {
	return func(yield func(*responseItem) bool) {
		for path, resp := range v.responseItems {
			if path != "" && resp.deferInfo.HasPrefix(v.walker.path) {
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

	switch fv := newField.Value.(type) {
	case *resolve.Object:
		r.objectStack = append(r.objectStack, fv)
	case *resolve.Array:
		if item, ok := fv.Item.(*resolve.Object); ok {
			r.objectStack = append(r.objectStack, item)
		}
	}
}

func (r *responseItem) copyFieldWithoutObjectFields(f *resolve.Field) *resolve.Field {
	ret := &resolve.Field{
		Name:              f.Name,
		Value:             f.Value.Copy(),
		Position:          f.Position,
		DeferPaths:        f.DeferPaths,
		Stream:            f.Stream,
		OnTypeNames:       f.OnTypeNames,
		ParentOnTypeNames: f.ParentOnTypeNames,
		Info:              f.Info,
	}
	switch fv := f.Value.(type) {
	case *resolve.Object:
		ret.Value = r.copyObjectWithoutFields(fv)
	case *resolve.Array:
		if arrObj, ok := fv.Item.(*resolve.Object); ok {
			ret.Value.(*resolve.Array).Item = r.copyObjectWithoutFields(arrObj)
		}
	}
	return ret
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
