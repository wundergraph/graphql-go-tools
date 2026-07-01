package cache

import (
	"github.com/wundergraph/astjson"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

// covers is the per-item coverage walk: it reports whether the cached value
// contains EVERY field of the fetch's ProvidesData tree, with null accepted
// only where the schema allows it. A partially-covering (stale-shape) value
// must never be served — the walk is the always-on guard between "some cached
// bytes exist" and "this fetch can be skipped".
//
// Task 07 reads fields by their response name; store-time normalization
// (schema names + argument-suffix keys, task 09) extends the naming.
func covers(value *astjson.Value, obj *resolve.Object) bool {
	if value == nil || obj == nil {
		return false
	}
	for _, field := range obj.Fields {
		fieldValue := value.Get(string(field.Name))
		if fieldValue == nil {
			return false
		}
		if !coversNode(fieldValue, field.Value) {
			return false
		}
	}
	return true
}

// coversNode applies the coverage rules per node shape: scalars accept null
// only when nullable; objects recurse (null OK when nullable); arrays require
// every element to cover the item shape.
func coversNode(value *astjson.Value, node resolve.Node) bool {
	switch typed := node.(type) {
	case *resolve.Scalar:
		return value.Type() != astjson.TypeNull || typed.Nullable
	case *resolve.Object:
		if value.Type() == astjson.TypeNull {
			return typed.Nullable
		}
		if value.Type() != astjson.TypeObject {
			return false
		}
		return covers(value, typed)
	case *resolve.Array:
		if value.Type() == astjson.TypeNull {
			return typed.Nullable
		}
		if value.Type() != astjson.TypeArray {
			return false
		}
		if typed.Item == nil {
			return true
		}
		items, err := value.Array()
		if err != nil {
			return false
		}
		for _, item := range items {
			if !coversNode(item, typed.Item) {
				return false
			}
		}
		return true
	default:
		return false
	}
}
