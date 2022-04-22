package sdlmerge

import (
	"fmt"
	"strings"

	"github.com/wundergraph/graphql-go-tools/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/pkg/astnormalization"
	"github.com/wundergraph/graphql-go-tools/pkg/astparser"
	"github.com/wundergraph/graphql-go-tools/pkg/astprinter"
	"github.com/wundergraph/graphql-go-tools/pkg/astvisitor"
	"github.com/wundergraph/graphql-go-tools/pkg/operationreport"
)

const rootOperationTypeDefinitions = `
	type Query {}
	type Mutation {}
	type Subscription {}
`

type Visitor interface {
	Register(walker *astvisitor.Walker)
}

func MergeAST(ast *ast.Document) error {
	normalizer := normalizer{}
	normalizer.setupWalkers()

	return normalizer.normalize(ast)
}

func MergeSDLs(SDLs ...string) (string, error) {
	rawDocs := make([]string, 0, len(SDLs)+1)
	rawDocs = append(rawDocs, rootOperationTypeDefinitions)
	rawDocs = append(rawDocs, SDLs...)

	doc, report := astparser.ParseGraphqlDocumentString(strings.Join(rawDocs, "\n"))
	if report.HasErrors() {
		return "", fmt.Errorf("parse graphql document string: %s", report.Error())
	}

	astnormalization.NormalizeSubgraphSDL(&doc, &report)
	if report.HasErrors() {
		return "", fmt.Errorf("merge ast: %s", report.Error())
	}

	if err := MergeAST(&doc); err != nil {
		return "", fmt.Errorf("merge ast: %s", err.Error())
	}

	out, err := astprinter.PrintString(&doc, nil)
	if err != nil {
		return "", fmt.Errorf("stringify schema: %s", err.Error())
	}

	return out, nil
}

type normalizer struct {
	walkers []*astvisitor.Walker
}

func (m *normalizer) setupWalkers() {
	visitorGroups := [][]Visitor{
		// visitors for extending objects and interfaces
		{
			newExtendInterfaceTypeDefinition(),
			newExtendUnionTypeDefinition(),
			newExtendObjectTypeDefinition(),
			newRemoveEmptyObjectTypeDefinition(),
			newRemoveMergedTypeExtensions(),
		},
		// visitors for cleaning up federated duplicated fields and directives
		{
			newRemoveFieldDefinitions("external"),
			newRemoveInterfaceDefinitionDirective("key"),
			newRemoveObjectTypeDefinitionDirective("key"),
			newRemoveFieldDefinitionDirective("provides", "requires"),
			newRemoveDuplicateScalarTypeDefinitionVisitor(),
			newRemoveDuplicateEnumTypeDefinitionVisitor(),
			newRemoveDuplicateUnionTypeDefinitionVisitor(),
		},
	}

	for _, visitorGroup := range visitorGroups {
		walker := astvisitor.NewWalker(48)
		for _, visitor := range visitorGroup {
			visitor.Register(&walker)
			m.walkers = append(m.walkers, &walker)
		}
	}
}

func (m *normalizer) normalize(operation *ast.Document) error {
	report := operationreport.Report{}

	for _, walker := range m.walkers {
		walker.Walk(operation, nil, &report)
		if report.HasErrors() {
			return fmt.Errorf("walk: %s", report.Error())
		}
	}

	return nil
}

type FieldlessParentType interface {
	Name() string
	AppendValueRefs(refs []int)
	ValueRefs() []int
	SetValueRefs(refs []int)
}

type EnumParentType struct {
	*ast.EnumTypeDefinition
	name string
}

func (e EnumParentType) Name() string {
	return e.name
}

func (e EnumParentType) AppendValueRefs(refs []int) {
	e.EnumValuesDefinition.Refs = append(e.EnumValuesDefinition.Refs, refs...)
}

func (e EnumParentType) ValueRefs() []int {
	return e.EnumValuesDefinition.Refs
}

func (e EnumParentType) SetValueRefs(refs []int) {
	e.HasEnumValuesDefinition = true
	e.EnumValuesDefinition.Refs = refs
}

type UnionParentType struct {
	*ast.UnionTypeDefinition
	name string
}

func (u UnionParentType) Name() string {
	return u.name
}

func (u UnionParentType) AppendValueRefs(refs []int) {
	u.UnionMemberTypes.Refs = append(u.UnionMemberTypes.Refs, refs...)
}

func (u UnionParentType) ValueRefs() []int {
	return u.UnionMemberTypes.Refs
}

func (u UnionParentType) SetValueRefs(refs []int) {
	u.UnionMemberTypes.Refs = refs
}
