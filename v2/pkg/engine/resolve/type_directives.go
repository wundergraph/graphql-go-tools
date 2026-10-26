package resolve

import (
	"strconv"

	"github.com/wundergraph/astjson"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/fastjsonext"
)

// TypeDirective is a directive usage that applies to every object of a concrete type
// in the response, wherever the object appears (root field, nested field, list item,
// entity fetch, interface or union field).
type TypeDirective struct {
	// Name is the directive name without "@".
	Name string
	// TypeName is the concrete object type the directive applies to.
	TypeName string
	// Handle is opaque data owned by the component that configured the directive,
	// e.g. the router's module chain and precomputed state.
	Handle any
}

// ObjectDirectiveError is an error added to the response at the path of an object.
type ObjectDirectiveError struct {
	Message string
	// Code is rendered as extensions.code when not empty.
	Code string
}

// ObjectDirectiveResult is the decision of an ObjectDirectiveHandler for one object.
type ObjectDirectiveResult struct {
	// Null sets the object to null. Null bubbling applies for non-null positions.
	Null bool
	// Error is added to the response at the path of the object when not nil.
	Error *ObjectDirectiveError
}

// ObjectDirectiveHandler is called in the pre-render walk for every object whose
// concrete type has TypeDirectives.
type ObjectDirectiveHandler interface {
	OnObject(ctx *Context, directive *TypeDirective, path []string, object *astjson.Value) (ObjectDirectiveResult, error)
}

// SetObjectDirectiveHandler sets the handler called for objects with type directives.
func (c *Context) SetObjectDirectiveHandler(handler ObjectDirectiveHandler) {
	c.objectDirectiveHandler = handler
}

// applyTypeDirectives calls the object directive handler for every type directive of the
// object's concrete type. It returns true when the object must be set to null.
// It must be called with the object path already pushed onto r.path.
func (r *Resolvable) applyTypeDirectives(obj *Object, typeName []byte, value *astjson.Value) (nullObject bool) {
	concreteTypeName := obj.TypeName
	if typeName != nil {
		concreteTypeName = string(typeName)
	}
	directives := obj.TypeDirectives[concreteTypeName]
	if len(directives) == 0 {
		return false
	}
	path := r.typeDirectivePath()
	for _, directive := range directives {
		result, err := r.ctx.objectDirectiveHandler.OnObject(r.ctx, directive, path, value)
		if err != nil {
			// fail closed: an object that could not be checked is never returned
			r.addObjectDirectiveError(&ObjectDirectiveError{
				Message: "Failed to evaluate directive @" + directive.Name + ".",
				Code:    "INTERNAL_SERVER_ERROR",
			})
			return true
		}
		if result.Error != nil {
			r.addObjectDirectiveError(result.Error)
		}
		if result.Null {
			return true
		}
	}
	return false
}

func (r *Resolvable) addObjectDirectiveError(err *ObjectDirectiveError) {
	r.ensureErrorsInitialized()
	if err.Code == "" {
		fastjsonext.AppendErrorToArray(r.astjsonArena, r.errors, err.Message, r.path)
		return
	}
	fastjsonext.AppendErrorWithExtensionsCodeToArray(r.astjsonArena, r.errors, err.Message, err.Code, r.path)
}

func (r *Resolvable) typeDirectivePath() []string {
	path := make([]string, 0, len(r.path))
	for _, element := range r.path {
		if element.Name != "" {
			path = append(path, element.Name)
			continue
		}
		path = append(path, strconv.Itoa(element.Idx))
	}
	return path
}
