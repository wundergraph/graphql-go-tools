package resolve

import (
	"strconv"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/lexer/literal"
)

type VariableKind int

const (
	ContextVariableKind VariableKind = iota + 1
	ObjectVariableKind
	HeaderVariableKind
	ResolvableObjectVariableKind
	ListVariableKind
)

const (
	variablePrefixSuffix = "$$"
)

type Variable interface {
	GetVariableKind() VariableKind
	Equals(another Variable) bool
	TemplateSegment() TemplateSegment
}

type Variables []Variable

func NewVariables(variables ...Variable) Variables {
	return variables
}

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

type ResolvableObjectVariable struct {
	Renderer *GraphQLVariableResolveRenderer
}

func NewResolvableObjectVariable(node *Object) *ResolvableObjectVariable {
	return &ResolvableObjectVariable{
		Renderer: NewGraphQLVariableResolveRenderer(node),
	}
}

func (h *ResolvableObjectVariable) TemplateSegment() TemplateSegment {
	return TemplateSegment{
		SegmentType:  VariableSegmentType,
		VariableKind: ResolvableObjectVariableKind,
		Renderer:     h.Renderer,
	}
}

func (h *ResolvableObjectVariable) GetVariableKind() VariableKind {
	return ResolvableObjectVariableKind
}

func (h *ResolvableObjectVariable) Equals(another Variable) bool {
	if another == nil {
		return false
	}
	if another.GetVariableKind() != h.GetVariableKind() {
		return false
	}
	anotherVariable := another.(*ResolvableObjectVariable)

	return h.Renderer.Node == anotherVariable.Renderer.Node
}

type ListVariable struct {
	Variables
}

func NewListVariable(variables Variables) *ListVariable {
	return &ListVariable{
		Variables: variables,
	}
}

func (h *ListVariable) TemplateSegment() TemplateSegment {
	// len: lb + rb + (variables + commas -1 )
	segments := make([]TemplateSegment, 0, (len(h.Variables)*2-1)+2)

	segments = append(segments, TemplateSegment{
		SegmentType: StaticSegmentType,
		Data:        literal.LBRACK,
	})

	for i := range h.Variables {
		segments = append(segments, h.Variables[i].TemplateSegment())
		if i < len(h.Variables)-1 {
			segments = append(segments, TemplateSegment{
				SegmentType: StaticSegmentType,
				Data:        literal.COMMA,
			})
		}
	}

	segments = append(segments, TemplateSegment{
		SegmentType: StaticSegmentType,
		Data:        literal.RBRACK,
	})

	return TemplateSegment{
		SegmentType: ListSegmentType,
		Segments:    segments,
	}
}

func (h *ListVariable) GetVariableKind() VariableKind {
	return ListVariableKind
}

func (h *ListVariable) Equals(another Variable) bool {
	if another == nil {
		return false
	}
	if another.GetVariableKind() != h.GetVariableKind() {
		return false
	}
	anotherVariable := another.(*ListVariable)

	for i, variable := range h.Variables {
		if !variable.Equals(anotherVariable.Variables[i]) {
			return false
		}
	}
	return true
}
