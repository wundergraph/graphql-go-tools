package introspection

import (
	"github.com/wundergraph/astjson"
)

// BuildJSON pre-computes self-contained JSON for each type and the full schema.
//
// The introspection data model has two Go types: FullType (rich, with fields,
// description, interfaces, etc.) and TypeRef (sparse, only kind/name/ofType).
// Field.Type and TypeRef.OfType are TypeRef, so a query like
// `ofType { fields { name } }` wouldn't find the expected data.
//
// This method walks the Go structs and builds JSON directly with astjson,
// resolving each TypeRef to its full type data on the fly. The result is
// self-contained JSON where every type reference carries the full type info.
//
// Self-referencing types (e.g. User with field friends: [User!]!) are handled
// by a visited set per DFS path — if a type is already being serialized in an
// ancestor, we emit only kind/name/ofType/__typename to break the cycle.
//
// Computed once at startup; Source.Load returns the pre-built bytes.
func (s *Schema) BuildJSON() error {
	s.enrichedTypeJSON = make(map[string][]byte, len(s.Types))
	for _, ft := range s.Types {
		visited := make(map[string]struct{})
		v := marshalFullType(ft, s.fullTypeMap, visited)
		s.enrichedTypeJSON[ft.Name] = v.MarshalTo(nil)
	}

	visited := make(map[string]struct{})
	v := marshalSchema(s, s.fullTypeMap, visited)
	s.enrichedSchemaJSON = v.MarshalTo(nil)

	return nil
}

func marshalSchema(s *Schema, typeMap map[string]*FullType, visited map[string]struct{}) *astjson.Value {
	obj := astjson.ObjectValue(nil)

	obj.Set(nil, "queryType", marshalFullType(&s.QueryType, typeMap, visited))

	if s.MutationType != nil {
		obj.Set(nil, "mutationType", marshalFullType(s.MutationType, typeMap, visited))
	} else {
		obj.Set(nil, "mutationType", astjson.NullValue)
	}

	if s.SubscriptionType != nil {
		obj.Set(nil, "subscriptionType", marshalFullType(s.SubscriptionType, typeMap, visited))
	} else {
		obj.Set(nil, "subscriptionType", astjson.NullValue)
	}

	typesArr := astjson.ArrayValue(nil)
	for i, ft := range s.Types {
		typesArr.SetArrayItem(nil, i, marshalFullType(ft, typeMap, visited))
	}
	obj.Set(nil, "types", typesArr)

	directivesArr := astjson.ArrayValue(nil)
	for i, d := range s.Directives {
		directivesArr.SetArrayItem(nil, i, marshalDirective(d, typeMap, visited))
	}
	obj.Set(nil, "directives", directivesArr)

	obj.Set(nil, "__typename", astjson.StringValue(nil, s.TypeName))

	if s.Description != nil {
		obj.Set(nil, "description", astjson.StringValue(nil, *s.Description))
	}

	return obj
}

// marshalFullType serializes a FullType as a top-level JSON object.
// Key order matches json.Marshal of the FullType struct.
func marshalFullType(ft *FullType, typeMap map[string]*FullType, visited map[string]struct{}) *astjson.Value {
	visited[ft.Name] = struct{}{}

	obj := astjson.ObjectValue(nil)
	obj.Set(nil, "kind", kindValue(ft.Kind))
	obj.Set(nil, "name", astjson.StringValue(nil, ft.Name))
	appendFullTypeBody(obj, ft, typeMap, visited)
	obj.Set(nil, "__typename", astjson.StringValue(nil, ft.TypeName))
	if ft.SpecifiedByURL != nil {
		obj.Set(nil, "specifiedByURL", astjson.StringValue(nil, *ft.SpecifiedByURL))
	}

	delete(visited, ft.Name)
	return obj
}

// marshalTypeRef serializes a TypeRef. For named types (OBJECT, SCALAR, etc.),
// it resolves the reference to the full type data from typeMap. For wrapper
// types (NON_NULL, LIST), it recurses into OfType. Cycle detection prevents
// infinite expansion of self-referencing types.
func marshalTypeRef(tr TypeRef, typeMap map[string]*FullType, visited map[string]struct{}) *astjson.Value {
	obj := astjson.ObjectValue(nil)
	obj.Set(nil, "kind", kindValue(tr.Kind))
	obj.Set(nil, "name", ptrStringValue(tr.Name))

	if tr.OfType != nil {
		obj.Set(nil, "ofType", marshalTypeRef(*tr.OfType, typeMap, visited))
	} else {
		obj.Set(nil, "ofType", astjson.NullValue)
	}

	obj.Set(nil, "__typename", astjson.StringValue(nil, tr.TypeName))

	// Wrapper types (NON_NULL, LIST) have nil Name — nothing to resolve.
	if tr.Name == nil {
		return obj
	}

	name := *tr.Name

	// Cycle: this type is already being serialized in an ancestor.
	if _, ok := visited[name]; ok {
		return obj
	}

	ft, ok := typeMap[name]
	if !ok {
		return obj
	}

	// Resolve: append full type data after the TypeRef keys.
	visited[name] = struct{}{}
	appendFullTypeBody(obj, ft, typeMap, visited)
	if ft.SpecifiedByURL != nil {
		obj.Set(nil, "specifiedByURL", astjson.StringValue(nil, *ft.SpecifiedByURL))
	}
	delete(visited, name)

	return obj
}

// appendFullTypeBody appends the FullType data keys (description through
// possibleTypes) to an object. Shared between marshalFullType and the
// resolve path in marshalTypeRef.
func appendFullTypeBody(obj *astjson.Value, ft *FullType, typeMap map[string]*FullType, visited map[string]struct{}) {
	obj.Set(nil, "description", astjson.StringValue(nil, ft.Description))

	if len(ft.Fields) > 0 {
		arr := astjson.ArrayValue(nil)
		for i, f := range ft.Fields {
			arr.SetArrayItem(nil, i, marshalField(f, typeMap, visited))
		}
		obj.Set(nil, "fields", arr)
	}

	obj.Set(nil, "inputFields", marshalInputValueArray(ft.InputFields, typeMap, visited))
	obj.Set(nil, "interfaces", marshalTypeRefArray(ft.Interfaces, typeMap, visited))

	if len(ft.EnumValues) > 0 {
		arr := astjson.ArrayValue(nil)
		for i, ev := range ft.EnumValues {
			arr.SetArrayItem(nil, i, marshalEnumValue(ev))
		}
		obj.Set(nil, "enumValues", arr)
	}

	obj.Set(nil, "possibleTypes", marshalTypeRefArray(ft.PossibleTypes, typeMap, visited))
}

func marshalField(f Field, typeMap map[string]*FullType, visited map[string]struct{}) *astjson.Value {
	obj := astjson.ObjectValue(nil)
	obj.Set(nil, "name", astjson.StringValue(nil, f.Name))
	obj.Set(nil, "description", astjson.StringValue(nil, f.Description))
	obj.Set(nil, "args", marshalInputValueArray(f.Args, typeMap, visited))
	obj.Set(nil, "type", marshalTypeRef(f.Type, typeMap, visited))
	obj.Set(nil, "isDeprecated", boolValue(f.IsDeprecated))
	obj.Set(nil, "deprecationReason", ptrStringValue(f.DeprecationReason))
	obj.Set(nil, "__typename", astjson.StringValue(nil, f.TypeName))
	return obj
}

func marshalInputValue(iv InputValue, typeMap map[string]*FullType, visited map[string]struct{}) *astjson.Value {
	obj := astjson.ObjectValue(nil)
	obj.Set(nil, "name", astjson.StringValue(nil, iv.Name))
	obj.Set(nil, "description", astjson.StringValue(nil, iv.Description))
	obj.Set(nil, "type", marshalTypeRef(iv.Type, typeMap, visited))
	obj.Set(nil, "defaultValue", ptrStringValue(iv.DefaultValue))
	obj.Set(nil, "isDeprecated", boolValue(iv.IsDeprecated))
	obj.Set(nil, "deprecationReason", ptrStringValue(iv.DeprecationReason))
	obj.Set(nil, "__typename", astjson.StringValue(nil, iv.TypeName))
	return obj
}

func marshalEnumValue(ev EnumValue) *astjson.Value {
	obj := astjson.ObjectValue(nil)
	obj.Set(nil, "name", astjson.StringValue(nil, ev.Name))
	obj.Set(nil, "description", astjson.StringValue(nil, ev.Description))
	obj.Set(nil, "isDeprecated", boolValue(ev.IsDeprecated))
	obj.Set(nil, "deprecationReason", ptrStringValue(ev.DeprecationReason))
	obj.Set(nil, "__typename", astjson.StringValue(nil, ev.TypeName))
	return obj
}

func marshalDirective(d Directive, typeMap map[string]*FullType, visited map[string]struct{}) *astjson.Value {
	obj := astjson.ObjectValue(nil)
	obj.Set(nil, "name", astjson.StringValue(nil, d.Name))
	obj.Set(nil, "description", astjson.StringValue(nil, d.Description))

	locArr := astjson.ArrayValue(nil)
	for i, loc := range d.Locations {
		locArr.SetArrayItem(nil, i, astjson.StringValue(nil, loc))
	}
	obj.Set(nil, "locations", locArr)

	obj.Set(nil, "args", marshalInputValueArray(d.Args, typeMap, visited))
	obj.Set(nil, "isRepeatable", boolValue(d.IsRepeatable))
	obj.Set(nil, "__typename", astjson.StringValue(nil, d.TypeName))
	return obj
}

func marshalTypeRefArray(refs []TypeRef, typeMap map[string]*FullType, visited map[string]struct{}) *astjson.Value {
	arr := astjson.ArrayValue(nil)
	for i, tr := range refs {
		arr.SetArrayItem(nil, i, marshalTypeRef(tr, typeMap, visited))
	}
	return arr
}

func marshalInputValueArray(ivs []InputValue, typeMap map[string]*FullType, visited map[string]struct{}) *astjson.Value {
	arr := astjson.ArrayValue(nil)
	for i, iv := range ivs {
		arr.SetArrayItem(nil, i, marshalInputValue(iv, typeMap, visited))
	}
	return arr
}

func kindValue(k __TypeKind) *astjson.Value {
	text, _ := k.MarshalText()
	return astjson.StringValue(nil, string(text))
}

func boolValue(b bool) *astjson.Value {
	if b {
		return astjson.TrueValue(nil)
	}
	return astjson.FalseValue(nil)
}

func ptrStringValue(s *string) *astjson.Value {
	if s == nil {
		return astjson.NullValue
	}
	return astjson.StringValue(nil, *s)
}
