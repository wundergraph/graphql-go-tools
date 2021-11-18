package resolve

import (
	"strconv"

	"github.com/buger/jsonparser"
	"github.com/jensneuse/graphql-go-tools/pkg/ast"
)

type VariableKind int

const (
	ContextVariableKind VariableKind = iota + 1
	ObjectVariableKind
	HeaderVariableKind
)

type ContextVariable struct {
	Path                 []string
	JsonValueType        jsonparser.ValueType
	ArrayJsonValueType   jsonparser.ValueType
	RenderAsArrayCSV     bool
	RenderAsPlainValue   bool
	RenderAsGraphQLValue bool
	OmitObjectKeyQuotes  bool
	EscapeQuotes         bool
}

func (c *ContextVariable) SetJsonValueType(operation, definition *ast.Document, typeRef int) {
	// TODO: check is it reachable
	if operation.TypeIsList(typeRef) {
		c.JsonValueType = jsonparser.Array
		c.ArrayJsonValueType = getJsonValueTypeType(operation, definition, operation.ResolveUnderlyingType(typeRef))
		return
	}

	c.JsonValueType = getJsonValueTypeType(operation, definition, typeRef)
}

func getJsonValueTypeType(operation, definition *ast.Document, typeRef int) jsonparser.ValueType {
	if operation.TypeIsList(typeRef) {
		return jsonparser.Array
	}

	if operation.TypeIsEnum(typeRef, definition) {
		return jsonparser.String
	}

	if operation.TypeIsScalar(typeRef, definition) {
		return getScalarJsonValueTypeType(typeRef, operation)
	}

	// TODO: this is not checking nested objects, consider using JSON Schema instead
	return jsonparser.Object
}

func getScalarJsonValueTypeType(typeRef int, document *ast.Document) jsonparser.ValueType {
	typeName := document.ResolveTypeNameString(typeRef)
	switch typeName {
	case "Boolean":
		return jsonparser.Boolean
	case "Int", "Float":
		return jsonparser.Number
	case "String", "Date", "ID":
		return jsonparser.String
	case "_Any":
		return jsonparser.Object
	default:
		// TODO: this could be wrong in case of custom scalars
		return jsonparser.String
	}
}

func (c *ContextVariable) TemplateSegment() TemplateSegment {
	return TemplateSegment{
		SegmentType:                  VariableSegmentType,
		VariableKind:                 ContextVariableKind,
		VariableSourcePath:           c.Path,
		VariableValueType:            c.JsonValueType,
		VariableValueArrayValueType:  c.ArrayJsonValueType,
		RenderVariableAsArrayCSV:     c.RenderAsArrayCSV,
		RenderVariableAsPlainValue:   c.RenderAsPlainValue,
		RenderVariableAsGraphQLValue: c.RenderAsGraphQLValue,
		OmitObjectKeyQuotes:          c.OmitObjectKeyQuotes,
		EscapeQuotes:                 c.EscapeQuotes,
	}
}

func (c *ContextVariable) Equals(another Variable) bool {
	if another == nil {
		return false
	}
	if another.GetVariableKind() != c.GetVariableKind() {
		return false
	}
	anotherContextVariable := another.(*ContextVariable)
	if len(c.Path) != len(anotherContextVariable.Path) {
		return false
	}
	for i := range c.Path {
		if c.Path[i] != anotherContextVariable.Path[i] {
			return false
		}
	}
	return true
}

func (_ *ContextVariable) GetVariableKind() VariableKind {
	return ContextVariableKind
}

type ObjectVariable struct {
	Path                 []string
	JsonValueType        jsonparser.ValueType
	ArrayJsonValueType   jsonparser.ValueType
	RenderAsGraphQLValue bool
	RenderAsPlainValue   bool
	RenderAsArrayCSV     bool
	OmitObjectKeyQuotes  bool
	EscapeQuotes         bool
}

func (o *ObjectVariable) SetJsonValueType(definition *ast.Document, typeRef int) {
	// TODO: check is it reachable
	if definition.TypeIsList(typeRef) {
		o.JsonValueType = jsonparser.Array
		o.ArrayJsonValueType = getJsonValueTypeType(definition, definition, definition.ResolveUnderlyingType(typeRef))
		return
	}

	o.JsonValueType = getJsonValueTypeType(definition, definition, typeRef)
}

func (o *ObjectVariable) TemplateSegment() TemplateSegment {
	return TemplateSegment{
		SegmentType:                  VariableSegmentType,
		VariableKind:                 ObjectVariableKind,
		VariableSourcePath:           o.Path,
		VariableValueType:            o.JsonValueType,
		VariableValueArrayValueType:  o.ArrayJsonValueType,
		RenderVariableAsArrayCSV:     o.RenderAsArrayCSV,
		RenderVariableAsPlainValue:   o.RenderAsPlainValue,
		RenderVariableAsGraphQLValue: o.RenderAsGraphQLValue,
		OmitObjectKeyQuotes:          o.OmitObjectKeyQuotes,
		EscapeQuotes:                 o.EscapeQuotes,
	}
}

func (o *ObjectVariable) Equals(another Variable) bool {
	if another == nil {
		return false
	}
	if another.GetVariableKind() != o.GetVariableKind() {
		return false
	}
	anotherObjectVariable := another.(*ObjectVariable)
	if len(o.Path) != len(anotherObjectVariable.Path) {
		return false
	}
	for i := range o.Path {
		if o.Path[i] != anotherObjectVariable.Path[i] {
			return false
		}
	}
	return true
}

func (o *ObjectVariable) GetVariableKind() VariableKind {
	return ObjectVariableKind
}

type HeaderVariable struct {
	Path []string
}

func (h *HeaderVariable) TemplateSegment() TemplateSegment {
	return TemplateSegment{
		SegmentType:        VariableSegmentType,
		VariableKind:       HeaderVariableKind,
		VariableSourcePath: h.Path,
	}
}

func (h *HeaderVariable) GetVariableKind() VariableKind {
	return HeaderVariableKind
}

func (h *HeaderVariable) Equals(another Variable) bool {
	if another == nil {
		return false
	}
	if another.GetVariableKind() != h.GetVariableKind() {
		return false
	}
	anotherHeaderVariable := another.(*HeaderVariable)
	if len(h.Path) != len(anotherHeaderVariable.Path) {
		return false
	}
	for i := range h.Path {
		if h.Path[i] != anotherHeaderVariable.Path[i] {
			return false
		}
	}
	return true
}

type Variable interface {
	GetVariableKind() VariableKind
	Equals(another Variable) bool
	TemplateSegment() TemplateSegment
}

type Variables []Variable

func NewVariables(variables ...Variable) Variables {
	return variables
}

const (
	variablePrefixSuffix = "$$"
)

func (v *Variables) AddVariable(variable Variable) (name string, exists bool) {
	index := -1
	for i := range *v {
		if (*v)[i].Equals(variable) {
			index = i
			exists = true
			break
		}
	}
	if index == -1 {
		*v = append(*v, variable)
		index = len(*v) - 1
	}
	i := strconv.Itoa(index)
	name = variablePrefixSuffix + i + variablePrefixSuffix
	return
}
