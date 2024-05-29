package resolve

import (
	"fmt"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"strconv"
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

	return h.Renderer.Node.Equals(anotherVariable.Renderer.Node)
}

func (v *Variables) AddContextVariableByArgumentRef(
	operation *ast.Document,
	definition *ast.Document,
	operationDefinitionRef int,
	argumentRef int,
	fullArgumentPath []string,
	finalInputValueTypeRef int,
) (string, error) {
	argumentValue := operation.ArgumentValue(argumentRef)
	if argumentValue.Kind != ast.ValueKindVariable {
		return "", fmt.Errorf(`expected argument to be kind "ValueKindVariable" but received "%s"`, argumentValue.Kind)
	}
	variableNameBytes := operation.VariableValueNameBytes(argumentValue.Ref)
	if _, ok := operation.VariableDefinitionByNameAndOperation(operationDefinitionRef, variableNameBytes); !ok {
		return "", fmt.Errorf(`expected definition for variable "%s" to exist`, variableNameBytes)
	}
	// The variable path should be the variable name, e.g., "a", and then the 2nd element from the path onwards
	variablePath := append([]string{string(variableNameBytes)}, fullArgumentPath[1:]...)
	/* The definition is passed as both definition and operation below because getJSONRootType resolves the type
	 * from the first argument, but finalInputValueTypeRef comes from the definition
	 */
	renderer, err := NewPlainVariableRendererWithValidationFromTypeRef(definition, definition, finalInputValueTypeRef, variablePath...)
	if err != nil {
		return "", err
	}
	contextVariable := &ContextVariable{
		Path:     variablePath,
		Renderer: renderer,
	}
	variablePlaceHolder, _ := v.AddVariable(contextVariable)
	return variablePlaceHolder, nil
}
