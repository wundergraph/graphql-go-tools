package resolve

import (
	"github.com/wundergraph/astjson"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/internal/unsafebytes"
)

// shallowCopyProvidedFields creates a shallow copy of the cached entity
// containing only the fields specified in providesData.
// This prevents pointer aliasing when the same entity is used as both
// source and target in merge operations (self-referential entities).
// "Shallow" means we only copy the fields required by the fetch, not a deep copy.
func (l *Loader) shallowCopyProvidedFields(cached *astjson.Value, providesData *Object) *astjson.Value {
	if cached == nil || providesData == nil {
		return cached
	}
	return l.shallowCopyObject(cached, providesData)
}

// shallowCopyObject recursively copies only the fields specified in the Object schema.
func (l *Loader) shallowCopyObject(cached *astjson.Value, obj *Object) *astjson.Value {
	if cached == nil || obj == nil {
		return cached
	}
	if cached.Type() != astjson.TypeObject {
		return cached
	}

	result := astjson.ObjectValue(l.jsonArena)
	for _, field := range obj.Fields {
		fieldName := unsafebytes.BytesToString(field.Name)
		fieldValue := cached.Get(fieldName)
		if fieldValue == nil {
			continue
		}

		// Recursively copy based on the field's value type in the schema
		copiedValue := l.shallowCopyNode(fieldValue, field.Value)
		if copiedValue != nil {
			result.Set(l.jsonArena, fieldName, copiedValue)
		}
	}
	return result
}

// shallowCopyNode copies a value according to the schema node type.
func (l *Loader) shallowCopyNode(cached *astjson.Value, node Node) *astjson.Value {
	if cached == nil || node == nil {
		return cached
	}

	switch n := node.(type) {
	case *Object:
		return l.shallowCopyObject(cached, n)
	case *Array:
		return l.shallowCopyArray(cached, n)
	default:
		// For scalars, copy the value to break pointer aliasing
		return l.shallowCopyScalar(cached)
	}
}

// shallowCopyArray copies array elements according to the item schema.
func (l *Loader) shallowCopyArray(cached *astjson.Value, arr *Array) *astjson.Value {
	if cached == nil || arr == nil {
		return cached
	}
	if cached.Type() != astjson.TypeArray {
		return cached
	}

	items := cached.GetArray()
	result := astjson.ArrayValue(l.jsonArena)
	for i, item := range items {
		copiedItem := l.shallowCopyNode(item, arr.Item)
		if copiedItem != nil {
			result.SetArrayItem(l.jsonArena, i, copiedItem)
		}
	}
	return result
}

// shallowCopyScalar creates a copy of a scalar value to break pointer aliasing.
func (l *Loader) shallowCopyScalar(cached *astjson.Value) *astjson.Value {
	if cached == nil {
		return nil
	}

	switch cached.Type() {
	case astjson.TypeNull:
		return astjson.NullValue
	case astjson.TypeTrue:
		return astjson.TrueValue(l.jsonArena)
	case astjson.TypeFalse:
		return astjson.FalseValue(l.jsonArena)
	case astjson.TypeNumber:
		// Marshal to get the raw number string, then create new number value
		raw := cached.MarshalTo(nil)
		return astjson.NumberValue(l.jsonArena, string(raw))
	case astjson.TypeString:
		// Copy the string bytes
		str := cached.GetStringBytes()
		return astjson.StringValueBytes(l.jsonArena, str)
	case astjson.TypeObject:
		// For objects without schema info, copy all fields
		return l.shallowCopyObjectAllFields(cached)
	case astjson.TypeArray:
		// For arrays without schema info, copy all elements
		return l.shallowCopyArrayAllItems(cached)
	default:
		return cached
	}
}

// shallowCopyObjectAllFields copies all fields of an object (used when no schema info available).
func (l *Loader) shallowCopyObjectAllFields(cached *astjson.Value) *astjson.Value {
	if cached == nil || cached.Type() != astjson.TypeObject {
		return cached
	}

	result := astjson.ObjectValue(l.jsonArena)
	obj, _ := cached.Object()
	obj.Visit(func(key []byte, v *astjson.Value) {
		result.Set(l.jsonArena, string(key), l.shallowCopyScalar(v))
	})
	return result
}

// shallowCopyArrayAllItems copies all items of an array (used when no schema info available).
func (l *Loader) shallowCopyArrayAllItems(cached *astjson.Value) *astjson.Value {
	if cached == nil || cached.Type() != astjson.TypeArray {
		return cached
	}

	items := cached.GetArray()
	result := astjson.ArrayValue(l.jsonArena)
	for i, item := range items {
		result.SetArrayItem(l.jsonArena, i, l.shallowCopyScalar(item))
	}
	return result
}
