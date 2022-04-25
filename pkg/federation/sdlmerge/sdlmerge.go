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
			newRemoveDuplicateFieldlessValueTypesVisitor(),
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

type FieldlessValueType interface {
	Name() string
	AppendValueRefs(refs []int)
	ValueRefs() []int
	SetValueRefs(refs []int)
	ValueName(r *removeDuplicateFieldlessValueTypesVisitor, ref int) string
}

type EnumValueType struct {
	*ast.EnumTypeDefinition
	name string
}

func (e EnumValueType) Name() string {
	return e.name
}

func (e EnumValueType) AppendValueRefs(refs []int) {
	e.EnumValuesDefinition.Refs = append(e.EnumValuesDefinition.Refs, refs...)
}

func (e EnumValueType) ValueRefs() []int {
	return e.EnumValuesDefinition.Refs
}

func (e EnumValueType) SetValueRefs(refs []int) {
	e.HasEnumValuesDefinition = true
	e.EnumValuesDefinition.Refs = refs
}

func (_ EnumValueType) ValueName(r *removeDuplicateFieldlessValueTypesVisitor, ref int) string {
	return r.document.EnumValueDefinitionNameString(ref)
}

type UnionValueType struct {
	*ast.UnionTypeDefinition
	name string
}

func (u UnionValueType) Name() string {
	return u.name
}

func (u UnionValueType) AppendValueRefs(refs []int) {
	u.UnionMemberTypes.Refs = append(u.UnionMemberTypes.Refs, refs...)
}

func (u UnionValueType) ValueRefs() []int {
	return u.UnionMemberTypes.Refs
}

func (u UnionValueType) SetValueRefs(refs []int) {
	u.UnionMemberTypes.Refs = refs
}

func (_ UnionValueType) ValueName(r *removeDuplicateFieldlessValueTypesVisitor, ref int) string {
	return r.document.TypeNameString(ref)
}

type ScalarValueType struct {
	name string
}

func (s ScalarValueType) Name() string {
	return s.name
}

func (_ ScalarValueType) AppendValueRefs(_ []int) {
	return
}

func (_ ScalarValueType) ValueRefs() []int {
	return nil
}

func (_ ScalarValueType) SetValueRefs(_ []int) {
	return
}

func (_ ScalarValueType) ValueName(_ *removeDuplicateFieldlessValueTypesVisitor, _ int) string {
	return ""
}
