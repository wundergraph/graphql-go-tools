package resolve

import (
	"context"
	"errors"
	"io"
	"strconv"

	"github.com/buger/jsonparser"
	"github.com/jensneuse/graphql-go-tools/pkg/ast"
	"github.com/jensneuse/graphql-go-tools/pkg/graphqljsonschema"
	"github.com/jensneuse/graphql-go-tools/pkg/lexer/literal"
)

var (
	ErrInvalidJsonSchema = errors.New("json schema validation failed on Variable Renderer")
)

type VariableKind int

const (
	ContextVariableKind VariableKind = iota + 1
	ObjectVariableKind
	HeaderVariableKind
)

type VariableRenderer interface {
	RenderVariable(data []byte, out io.Writer) error
}

func NewPlainVariableRenderer() *PlainVariableRenderer {
	return &PlainVariableRenderer{}
}

func NewPlainVariableRendererWithValidation(validator *graphqljsonschema.Validator) *PlainVariableRenderer {
	return &PlainVariableRenderer{
		validator: validator,
	}
}

type PlainVariableRenderer struct {
	validator *graphqljsonschema.Validator
}

func (p *PlainVariableRenderer) RenderVariable(data []byte, out io.Writer) error {
	if p.validator != nil {
		valid := p.validator.Validate(context.Background(), data)
		if !valid {
			return ErrInvalidJsonSchema
		}
	}
	_, err := out.Write(data)
	return err
}

func NewGraphQLVariableRendererFromTypeRef(definition *ast.Document, typeRef int) (*GraphQLVariableRenderer, error) {
	jsonSchema := graphqljsonschema.FromTypeRef(definition, typeRef)
	validator, err := graphqljsonschema.NewValidatorFromSchema(jsonSchema)
	if err != nil {
		return nil, err
	}
	return &GraphQLVariableRenderer{
		validator:     validator,
		rootValueType: getJSONRootType(definition, typeRef),
	}, nil
}

func NewGraphQLVariableRenderer(validator *graphqljsonschema.Validator, rootValueType jsonparser.ValueType) *GraphQLVariableRenderer {
	return &GraphQLVariableRenderer{
		validator:     validator,
		rootValueType: rootValueType,
	}
}

func getJSONRootType(definition *ast.Document, typeRef int) jsonparser.ValueType {
	typeRef = definition.ResolveListOrNameType(typeRef)
	if definition.TypeIsList(typeRef) {
		return jsonparser.Array
	}
	if definition.TypeIsEnum(typeRef, definition) {
		return jsonparser.String
	}
	if definition.TypeIsScalar(typeRef, definition) {
		typeName := definition.TypeNameString(typeRef)
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
			return jsonparser.String
		}
	}
	return jsonparser.Object
}

type GraphQLVariableRenderer struct {
	validator     *graphqljsonschema.Validator
	rootValueType jsonparser.ValueType
}

func (g *GraphQLVariableRenderer) RenderVariable(data []byte, out io.Writer) error {
	valid := g.validator.Validate(context.Background(), data)
	if !valid {
		return ErrInvalidJsonSchema
	}
	return g.renderGraphQLValue(data, g.rootValueType, out)
}

func (g *GraphQLVariableRenderer) renderGraphQLValue(data []byte, valueType jsonparser.ValueType, out io.Writer) (err error) {
	switch valueType {
	case jsonparser.String:
		_, _ = out.Write(literal.BACKSLASH)
		_, _ = out.Write(literal.QUOTE)
		_, _ = out.Write(data)
		_, _ = out.Write(literal.BACKSLASH)
		_, _ = out.Write(literal.QUOTE)
	case jsonparser.Object:
		_, _ = out.Write(literal.LBRACE)
		first := true
		err = jsonparser.ObjectEach(data, func(key []byte, value []byte, objectFieldValueType jsonparser.ValueType, offset int) error {
			if !first {
				_, _ = out.Write(literal.COMMA)
			} else {
				first = false
			}
			_, _ = out.Write(key)
			_, _ = out.Write(literal.COLON)
			return g.renderGraphQLValue(value, objectFieldValueType, out)
		})
		if err != nil {
			return err
		}
		_, _ = out.Write(literal.RBRACE)
	case jsonparser.Null:
		_, _ = out.Write(literal.NULL)
	case jsonparser.Boolean:
		_, _ = out.Write(data)
	case jsonparser.Array:
		_, _ = out.Write(literal.LBRACK)
		first := true
		var arrayErr error
		_, err = jsonparser.ArrayEach(data, func(value []byte, arrayItemValueType jsonparser.ValueType, offset int, err error) {
			if !first {
				_, _ = out.Write(literal.COMMA)
			} else {
				first = false
			}
			arrayErr = g.renderGraphQLValue(value, arrayItemValueType, out)
		})
		if arrayErr != nil {
			return arrayErr
		}
		if err != nil {
			return err
		}
		_, _ = out.Write(literal.RBRACK)
	case jsonparser.Number:
		_, _ = out.Write(data)
	}
	return
}

func NewCSVVariableRenderer(arrayValueType jsonparser.ValueType) *CSVVariableRenderer {
	return &CSVVariableRenderer{
		arrayValueType: arrayValueType,
	}
}

func NewCSVVariableRendererFromTypeRef(definition *ast.Document, typeRef int) *CSVVariableRenderer {
	return &CSVVariableRenderer{
		arrayValueType: getJSONRootType(definition, typeRef),
	}
}

type CSVVariableRenderer struct {
	arrayValueType jsonparser.ValueType
}

func (c *CSVVariableRenderer) RenderVariable(data []byte, out io.Writer) error {
	isFirst := true
	_, err := jsonparser.ArrayEach(data, func(value []byte, dataType jsonparser.ValueType, offset int, err error) {
		if dataType != c.arrayValueType {
			return
		}
		if isFirst {
			isFirst = false
		} else {
			_, _ = out.Write(literal.COMMA)
		}
		_, _ = out.Write(value)
	})
	return err
}

type ContextVariable struct {
	Path     []string
	Renderer VariableRenderer
}

func (c *ContextVariable) TemplateSegment() TemplateSegment {
	return TemplateSegment{
		SegmentType:        VariableSegmentType,
		VariableKind:       ContextVariableKind,
		VariableSourcePath: c.Path,
		Renderer:           c.Renderer,
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
	Path     []string
	Renderer VariableRenderer
}

func (o *ObjectVariable) TemplateSegment() TemplateSegment {
	return TemplateSegment{
		SegmentType:        VariableSegmentType,
		VariableKind:       ObjectVariableKind,
		VariableSourcePath: o.Path,
		Renderer:           o.Renderer,
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

type VariableSchema struct {
}
