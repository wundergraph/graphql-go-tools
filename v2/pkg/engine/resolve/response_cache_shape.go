package resolve

import (
	"bytes"
	"slices"

	"github.com/wundergraph/astjson"
)

// planObjectsAt is every object the plan merges into at path. Arrays are
// stepped through, so "@" elements are skipped.
func planObjectsAt(node Node, path []string) []*Object {
	if arr, ok := node.(*Array); ok {
		return planObjectsAt(arr.Item, path)
	}
	obj, ok := node.(*Object)
	if !ok {
		return nil
	}
	for len(path) > 0 && path[0] == "@" {
		path = path[1:]
	}
	if len(path) == 0 {
		return []*Object{obj}
	}
	var out []*Object
	for _, field := range obj.Fields {
		if string(field.Name) == path[0] {
			out = append(out, planObjectsAt(field.Value, path[1:])...)
		}
	}
	return out
}

// shapeObjectValid checks the fields of obj that dataSourceID provides.
func shapeObjectValid(obj *Object, value *astjson.Value, dataSourceID string) bool {
	if value.Type() != astjson.TypeObject {
		return false
	}
	typeName := value.GetStringBytes("__typename")
	for _, field := range obj.Fields {
		if field.Info == nil || !slices.Contains(field.Info.Source.IDs, dataSourceID) {
			continue
		}
		if len(field.OnTypeNames) > 0 && !slices.ContainsFunc(field.OnTypeNames, func(name []byte) bool {
			return bytes.Equal(name, typeName)
		}) {
			continue
		}
		fieldValue := value.Get(field.Value.NodePath()...)
		if fieldValue == nil {
			continue
		}
		if !shapeNodeValid(field.Value, fieldValue, dataSourceID) {
			return false
		}
	}
	return true
}

func shapeNodeValid(node Node, value *astjson.Value, dataSourceID string) bool {
	if value.Type() == astjson.TypeNull {
		return node.NodeNullable()
	}
	switch n := node.(type) {
	case *Object:
		return shapeObjectValid(n, value, dataSourceID)
	case *Array:
		if value.Type() != astjson.TypeArray {
			return false
		}
		for _, item := range value.GetArray() {
			itemValue := item.Get(n.Item.NodePath()...)
			if itemValue == nil || !shapeNodeValid(n.Item, itemValue, dataSourceID) {
				return false
			}
		}
		return true
	case *String:
		return value.Type() == astjson.TypeString
	case *Enum:
		return value.Type() == astjson.TypeString && n.isValidValue(string(value.GetStringBytes()))
	case *Boolean:
		return value.Type() == astjson.TypeTrue || value.Type() == astjson.TypeFalse
	case *Integer, *Float:
		return value.Type() == astjson.TypeNumber
	default:
		// Scalar, BigInt and custom nodes take any non-null value.
		return true
	}
}
