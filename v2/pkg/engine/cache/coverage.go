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
// Cached values are stored NORMALIZED (task 09), so the walk reads fields by
// their normalized name — schema name plus argument suffix — which is exactly
// what makes an argument-mismatched cached field a MISS instead of a hit.
func covers(ctx *resolve.Context, value *astjson.Value, obj *resolve.Object) bool {
	if value == nil || obj == nil {
		return false
	}
	typeName := valueTypeName(value)
	for _, field := range obj.Fields {
		if skipFieldForTypeName(field, typeName) {
			continue
		}
		fieldValue := value.Get(normalizedFieldName(ctx, field))
		if fieldValue == nil {
			return false
		}
		if !coversNode(ctx, fieldValue, field.Value) {
			return false
		}
	}
	return true
}

// valueTypeName reads the cached value's __typename ("" when absent).
func valueTypeName(value *astjson.Value) string {
	typeName := value.Get("__typename")
	if typeName == nil {
		return ""
	}
	return string(typeName.GetStringBytes())
}

// skipFieldForTypeName reports whether a type-conditioned field does not apply
// to the cached value's concrete type: requiring it would turn every valid
// polymorphic response into a coverage miss. An absent __typename keeps the
// field REQUIRED (conservative: no evidence the condition is satisfied means
// no serve).
func skipFieldForTypeName(field *resolve.Field, typeName string) bool {
	if len(field.OnTypeNames) == 0 || typeName == "" {
		return false
	}
	for _, onTypeName := range field.OnTypeNames {
		if string(onTypeName) == typeName {
			return false
		}
	}
	return true
}

// coversNode applies the coverage rules per node shape: scalars accept null
// only when nullable; objects recurse (null OK when nullable); arrays require
// every element to cover the item shape.
func coversNode(ctx *resolve.Context, value *astjson.Value, node resolve.Node) bool {
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
		return covers(ctx, value, typed)
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
			if !coversNode(ctx, item, typed.Item) {
				return false
			}
		}
		return true
	default:
		return false
	}
}
