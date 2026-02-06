package resolve

import (
	"github.com/wundergraph/astjson"
	"github.com/wundergraph/go-arena"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/internal/unsafebytes"
)

type CacheKeyTemplate interface {
	// RenderCacheKeys returns multiple cache keys (one per root field or entity)
	// Generates keys for all items at once
	RenderCacheKeys(a arena.Arena, ctx *Context, items []*astjson.Value, prefix string) ([]*CacheKey, error)
}

type CacheKey struct {
	Item      *astjson.Value
	FromCache *astjson.Value
	Keys      []string
}

type RootQueryCacheKeyTemplate struct {
	RootFields []QueryField
}

type QueryField struct {
	Coordinate GraphCoordinate
	Args       []FieldArgument
}

type FieldArgument struct {
	Name     string
	Variable Variable
}

// RenderCacheKeys returns multiple cache keys, one per item
// Each cache key contains one or more KeyEntry objects (one per root field)
func (r *RootQueryCacheKeyTemplate) RenderCacheKeys(a arena.Arena, ctx *Context, items []*astjson.Value, prefix string) ([]*CacheKey, error) {
	if len(r.RootFields) == 0 {
		return nil, nil
	}
	// Estimate capacity: one CacheKey per item
	cacheKeys := arena.AllocateSlice[*CacheKey](a, 0, len(items))
	jsonBytes := arena.AllocateSlice[byte](a, 0, 64)

	for _, item := range items {
		// Create KeyEntry for each root field
		keyEntries := arena.AllocateSlice[string](a, 0, len(r.RootFields))
		for _, field := range r.RootFields {
			var key string
			key, jsonBytes = r.renderField(a, ctx, item, jsonBytes, field)
			if prefix != "" {
				l := len(prefix) + 1 + len(key)
				tmp := arena.AllocateSlice[byte](a, 0, l)
				tmp = arena.SliceAppend(a, tmp, unsafebytes.StringToBytes(prefix)...)
				tmp = arena.SliceAppend(a, tmp, []byte(`:`)...)
				tmp = arena.SliceAppend(a, tmp, unsafebytes.StringToBytes(key)...)
				key = unsafebytes.BytesToString(tmp)
			}
			keyEntries = arena.SliceAppend(a, keyEntries, key)
		}
		cacheKeys = arena.SliceAppend(a, cacheKeys, &CacheKey{
			Item: item,
			Keys: keyEntries,
		})
	}
	return cacheKeys, nil
}

// renderField renders a single field cache key as JSON
func (r *RootQueryCacheKeyTemplate) renderField(a arena.Arena, ctx *Context, item *astjson.Value, jsonBytes []byte, field QueryField) (string, []byte) {
	// Build JSON object starting with __typename
	keyObj := astjson.ObjectValue(a)
	typeName := field.Coordinate.TypeName
	keyObj.Set(a, "__typename", astjson.StringValue(a, typeName))
	keyObj.Set(a, "field", astjson.StringValue(a, field.Coordinate.FieldName))

	// Build args object if there are any arguments
	if len(field.Args) > 0 {
		argsObj := astjson.ObjectValue(a)
		for _, arg := range field.Args {
			var argValue *astjson.Value
			segment := arg.Variable.TemplateSegment()
			if segment.Renderer != nil {
				switch segment.VariableKind {
				case ContextVariableKind:
					// Extract value from context variables
					variableSourcePath := segment.VariableSourcePath
					if len(variableSourcePath) == 1 && ctx.RemapVariables != nil {
						if nameToUse, hasMapping := ctx.RemapVariables[variableSourcePath[0]]; hasMapping && nameToUse != variableSourcePath[0] {
							variableSourcePath = []string{nameToUse}
						}
					}
					argValue = ctx.Variables.Get(variableSourcePath...)
					if argValue == nil {
						argValue = astjson.NullValue
					}
				case ObjectVariableKind:
					// Use data parameter for object variables
					if item != nil {
						value := item.Get(segment.VariableSourcePath...)
						if value == nil || value.Type() == astjson.TypeNull {
							argValue = astjson.NullValue
						} else {
							// Values are already JSON-compatible astjson.Value
							argValue = value
						}
					} else {
						argValue = astjson.NullValue
					}
				default:
					// For other variable kinds, use data parameter
					if item != nil {
						argValue = item
					} else {
						argValue = astjson.NullValue
					}
				}
			} else {
				argValue = astjson.NullValue
			}
			argsObj.Set(a, arg.Name, argValue)
		}
		keyObj.Set(a, "args", argsObj)
	}

	// Marshal to JSON and write to output
	jsonBytes = keyObj.MarshalTo(jsonBytes[:0])
	slice := arena.AllocateSlice[byte](a, len(jsonBytes), len(jsonBytes))
	copy(slice, jsonBytes)
	return unsafebytes.BytesToString(slice), jsonBytes
}

type EntityQueryCacheKeyTemplate struct {
	// Keys contains only @key fields (without @requires fields).
	// Used for both L1 and L2 cache keys to ensure stable entity identity.
	Keys *ResolvableObjectVariable
}

// RenderCacheKeys implements CacheKeyTemplate interface.
// Uses Keys template (only @key fields) for stable entity identity.
// Prefix is used for L2 cache isolation (typically subgraph header hash).
func (e *EntityQueryCacheKeyTemplate) RenderCacheKeys(a arena.Arena, ctx *Context, items []*astjson.Value, prefix string) ([]*CacheKey, error) {
	return e.renderCacheKeys(a, ctx, items, e.Keys, prefix)
}

// renderCacheKeys is the internal implementation for RenderCacheKeys.
// Returns one cache key per item for entity queries with keys nested under "key".
func (e *EntityQueryCacheKeyTemplate) renderCacheKeys(a arena.Arena, ctx *Context, items []*astjson.Value, keysTemplate *ResolvableObjectVariable, prefix string) ([]*CacheKey, error) {
	jsonBytes := arena.AllocateSlice[byte](a, 0, 64)
	cacheKeys := arena.AllocateSlice[*CacheKey](a, 0, len(items))

	for _, item := range items {
		if item == nil {
			continue
		}

		// Build JSON object starting with __typename
		keyObj := astjson.ObjectValue(a)

		// Extract __typename from the data
		typename := item.Get("__typename")
		if typename == nil {
			// Fallback if no __typename in data
			keyObj.Set(a, "__typename", astjson.StringValue(a, "Entity"))
		} else {
			keyObj.Set(a, "__typename", typename)
		}

		// Put entity keys under "key" nested object
		keysObj := astjson.ObjectValue(a)

		// Extract only the fields defined in the template (not all fields from data)
		if keysTemplate != nil && keysTemplate.Renderer != nil {
			if obj, ok := keysTemplate.Renderer.Node.(*Object); ok {
				for _, field := range obj.Fields {
					fieldName := unsafebytes.BytesToString(field.Name)
					// Skip __typename as it's already handled separately
					if fieldName == "__typename" {
						continue
					}
					// Resolve field value based on its template definition
					fieldValue := e.resolveFieldValue(a, field.Value, item)
					if fieldValue != nil && fieldValue.Type() != astjson.TypeNull {
						keysObj.Set(a, fieldName, fieldValue)
					}
				}
			}
		}

		keyObj.Set(a, "key", keysObj)

		// Marshal to JSON and write to buffer
		jsonBytes = keyObj.MarshalTo(jsonBytes[:0])
		l := len(jsonBytes)
		if prefix != "" {
			l += 1 + len(prefix)
		}
		slice := arena.AllocateSlice[byte](a, 0, l)
		if prefix != "" {
			slice = arena.SliceAppend(a, slice, unsafebytes.StringToBytes(prefix)...)
			slice = arena.SliceAppend(a, slice, []byte(`:`)...)
		}
		slice = arena.SliceAppend(a, slice, jsonBytes...)

		// Create KeyEntry with empty path for entity queries
		keyEntries := arena.AllocateSlice[string](a, 0, 1)
		keyEntries = arena.SliceAppend(a, keyEntries, unsafebytes.BytesToString(slice))

		cacheKeys = arena.SliceAppend(a, cacheKeys, &CacheKey{
			Item: item,
			Keys: keyEntries,
		})
	}

	return cacheKeys, nil
}

// resolveFieldValue resolves a field value from data based on its template definition
func (e *EntityQueryCacheKeyTemplate) resolveFieldValue(a arena.Arena, valueNode Node, data *astjson.Value) *astjson.Value {
	switch node := valueNode.(type) {
	case *String:
		// Extract string value from data using the path
		return data.Get(node.Path...)
	case *Scalar:
		// Handle scalar types (like ID) - extract value from data using the path
		return data.Get(node.Path...)
	case *Integer:
		// Handle integer type
		return data.Get(node.Path...)
	case *Float:
		// Handle float type
		return data.Get(node.Path...)
	case *Boolean:
		// Handle boolean type
		return data.Get(node.Path...)
	case *CustomNode:
		return data.Get(node.Path...)
	case *Object:
		// For nested objects, recursively build the object using only template-defined fields
		nestedObj := astjson.ObjectValue(a)
		// Get the base object from data using the object's path
		baseData := data.Get(node.Path...)
		if baseData == nil || baseData.Type() == astjson.TypeNull {
			return nil
		}
		// Recursively resolve each field in the nested object template
		for _, field := range node.Fields {
			fieldName := unsafebytes.BytesToString(field.Name)
			// Skip __typename in nested objects
			if fieldName == "__typename" {
				continue
			}
			fieldValue := e.resolveFieldValue(a, field.Value, baseData)
			if fieldValue != nil && fieldValue.Type() != astjson.TypeNull {
				nestedObj.Set(a, fieldName, fieldValue)
			}
		}
		return nestedObj
	case *Array:
		// Handle arrays by resolving each item based on the Item template
		arrayValue := data.Get(node.Path...)
		if arrayValue == nil || arrayValue.Type() != astjson.TypeArray {
			return nil
		}
		items := arrayValue.GetArray()
		resultArray := astjson.ArrayValue(a)
		resultIndex := 0
		for _, itemData := range items {
			if itemData == nil {
				continue
			}
			resolvedItem := e.resolveFieldValue(a, node.Item, itemData)
			if resolvedItem != nil {
				resultArray.SetArrayItem(a, resultIndex, resolvedItem)
				resultIndex++
			}
		}
		return resultArray
	default:
		// For other types not handled above, return nil
		return nil
	}
}
