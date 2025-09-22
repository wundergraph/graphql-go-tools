package resolve

import (
	"strconv"

	"github.com/wundergraph/astjson"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/internal/unsafebytes"
)

// getTaintedIndices identifies indices of malformed entities based on error paths
// in the response. It uses errors to find entities that have null value for nullable fields that
// are required for other fetches.
//
// The high-level flow of how it is used:
//
// 1. Subgraph returns errors for specific entities in the "_entities" array;
// 2. getTaintedIndices examines error paths like ["_entities", 1, "requiredField"];
// 3. It validates that the failed field was actually requested for @requires;
// 4. Marks object at index 1 as "tainted";
// 5. Later fetches will ignore this tainted object.
func getTaintedIndices(fetch Fetch, data *astjson.Value, errors *astjson.Value) (indices []int) {
	info := fetch.FetchInfo()
	if info == nil {
		return nil
	}
	// build a map to search with
	requestedForRequires := map[GraphCoordinate]struct{}{}
	for _, fr := range info.FetchReasons {
		if fr.IsRequires && fr.Nullable {
			coord := GraphCoordinate{TypeName: fr.TypeName, FieldName: fr.FieldName}
			requestedForRequires[coord] = struct{}{}
		}
	}
	if len(requestedForRequires) == 0 {
		return
	}

	errorsArray := errors.GetArray()
	for _, candidate := range errorsArray {
		errorPath := candidate.Get("path")
		if astjson.ValueIsNull(errorPath) || errorPath.Type() != astjson.TypeArray {
			continue
		}
		pathItems := errorPath.GetArray()
		if len(pathItems) == 0 {
			continue
		}
		for i, item := range pathItems {
			if unsafebytes.BytesToString(item.GetStringBytes()) != "_entities" {
				continue
			}

			lastIndex := len(pathItems) - 1
			// The remaining pathItems should have at least 2 items:
			if lastIndex-i <= 1 {
				break
			}
			// We have the full path to the failed item.
			//
			// For example, if pathItems == ["_entities",0,"nested","a"],
			// then fieldName would be "a" and [0,"nested","a"] used for selection.
			fieldName := unsafebytes.BytesToString(pathItems[lastIndex].GetStringBytes())
			obj, index := selectObjectAndIndex(data, pathItems[i+1:lastIndex])
			if index == -1 || astjson.ValueIsNull(obj) || obj.Type() != astjson.TypeObject {
				break
			}

			// Verify that the value selected by the path is null and extract the enclosing typename.
			possibleNull := obj.Get(fieldName)
			if possibleNull == nil || possibleNull.Type() != astjson.TypeNull {
				break
			}
			typeName := unsafebytes.BytesToString(obj.GetStringBytes("__typename"))
			if typeName == "" {
				break
			}

			coord := GraphCoordinate{TypeName: typeName, FieldName: fieldName}
			if _, ok := requestedForRequires[coord]; !ok {
				break
			}
			indices = append(indices, index)
			break
		}
	}
	return
}

// selectObjectAndIndex returns an object and its index using the path as selectors on the response.
// Path should contain at least the index as the first element. Other elements would lead
// to deeply nested objects.
func selectObjectAndIndex(response *astjson.Value, path []*astjson.Value) (*astjson.Value, int) {
	index := -1
	if len(path) == 0 {
		return nil, index
	}
	for _, el := range path {
		var key string
		switch el.Type() {
		case astjson.TypeNumber:
			parsed := el.GetInt()
			if parsed < 0 {
				return nil, index
			}
			if index == -1 {
				// index is assigned only once
				index = parsed
			}
			key = strconv.Itoa(parsed)
		case astjson.TypeString:
			key = unsafebytes.BytesToString(el.GetStringBytes())
		default:
			return nil, -1
		}
		response = response.Get(key)
		if response == nil {
			return nil, -1
		}
	}
	return response, index
}

// taintedObjects tracks objects fetched with errors.
// Later fetches should ignore such objects.
type taintedObjects map[*astjson.Value]struct{}

// filterOutTainted removes tainted objects from the given items list.
func (t taintedObjects) filterOutTainted(items []*astjson.Value) []*astjson.Value {
	if len(items) == 0 || len(t) == 0 {
		return items
	}
	filtered := make([]*astjson.Value, 0, len(items))
	for _, item := range items {
		if t.isTainted(item, 0) {
			continue
		}
		filtered = append(filtered, item)
	}
	return filtered
}

const maximumDepthOfTaintedTraversal = 100

// isTainted checks if the given `item` is considered isTainted in the Loader context.
// Not only the item is being considered, but also its elements if the item is an array,
// or its values if the item is an object.
func (t taintedObjects) isTainted(item *astjson.Value, depth int) bool {
	_, ok := t[item]
	if ok {
		return true
	}
	if depth > maximumDepthOfTaintedTraversal {
		// hard limit to prevent stack overflow
		return false
	}
	switch item.Type() {
	case astjson.TypeArray:
		for _, elem := range item.GetArray() {
			if t.isTainted(elem, depth+1) {
				return true
			}
		}
	case astjson.TypeObject:
		obj := item.GetObject()
		found := false
		obj.Visit(func(key []byte, value *astjson.Value) {
			if !found && t.isTainted(value, depth+1) {
				found = true
			}
		})
		return found
	}
	return false
}

func (t taintedObjects) add(item *astjson.Value) {
	t[item] = struct{}{}
}
