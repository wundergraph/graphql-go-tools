package resolve

import (
	"github.com/wundergraph/astjson"
	"github.com/wundergraph/go-arena"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/internal/unsafebytes"
)

type CacheKeyTemplate interface {
	// RenderCacheKeys returns multiple cache keys (one per root field or entity)
	// Generates keys for all items at once
	RenderCacheKeys(a arena.Arena, ctx *Context, items []*astjson.Value) ([]*CacheKey, error)
}

type CacheKey struct {
	Item      *astjson.Value
	FromCache *astjson.Value
	Keys      []KeyEntry
}

type KeyEntry struct {
	Name string
	Path string
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
func (r *RootQueryCacheKeyTemplate) RenderCacheKeys(a arena.Arena, ctx *Context, items []*astjson.Value) ([]*CacheKey, error) {
	if len(r.RootFields) == 0 {
		return nil, nil
	}
	// Estimate capacity: one CacheKey per item
	cacheKeys := arena.AllocateSlice[*CacheKey](a, 0, len(items))
	jsonBytes := arena.AllocateSlice[byte](a, 0, 64)

	for _, item := range items {
		// Create KeyEntry for each root field
		keyEntries := arena.AllocateSlice[KeyEntry](a, 0, len(r.RootFields))
		for _, field := range r.RootFields {
			var key string
			key, jsonBytes = r.renderField(a, ctx, item, jsonBytes, field)
			keyEntries = arena.SliceAppend(a, keyEntries, KeyEntry{
				Name: key,
				Path: field.Coordinate.FieldName,
			})
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
	Keys *ResolvableObjectVariable
}

// RenderCacheKeys returns one cache key per item for entity queries with keys nested under "keys"
func (e *EntityQueryCacheKeyTemplate) RenderCacheKeys(a arena.Arena, ctx *Context, items []*astjson.Value) ([]*CacheKey, error) {
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

		// Put entity keys under "keys" nested object
		keysObj := astjson.ObjectValue(a)

		// Extract only the fields defined in the Keys template (not all fields from data)
		if e.Keys != nil && e.Keys.Renderer != nil {
			if obj, ok := e.Keys.Renderer.Node.(*Object); ok {
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

		keyObj.Set(a, "keys", keysObj)

		// Marshal to JSON and write to buffer
		jsonBytes = keyObj.MarshalTo(jsonBytes[:0])
		slice := arena.AllocateSlice[byte](a, len(jsonBytes), len(jsonBytes))
		copy(slice, jsonBytes)

		// Create KeyEntry with empty path for entity queries
		keyEntries := arena.AllocateSlice[KeyEntry](a, 0, 1)
		keyEntries = arena.SliceAppend(a, keyEntries, KeyEntry{
			Name: unsafebytes.BytesToString(slice),
			Path: "",
		})

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
	default:
		// For other types not handled above, return nil
		return nil
	}
}
