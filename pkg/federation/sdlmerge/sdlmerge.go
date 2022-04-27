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
	ValueRefs() []int
	AreValuesIdentical(r *removeDuplicateFieldlessValueTypesVisitor, valueRefsToCompare []int) bool
	ValueName(r *removeDuplicateFieldlessValueTypesVisitor, ref int) string
}

type EnumValueType struct {
	*ast.EnumTypeDefinition
	name     string
	valueSet map[string]bool
}

func createValueSet(r *removeDuplicateFieldlessValueTypesVisitor, f FieldlessValueType) map[string]bool {
	valueSet := make(map[string]bool)
	for _, valueRef := range f.ValueRefs() {
		valueSet[f.ValueName(r, valueRef)] = true
	}
	return valueSet
}

func NewEnumValueType(r *removeDuplicateFieldlessValueTypesVisitor, ref int) EnumValueType {
	document := r.document
	e := EnumValueType{
		&document.EnumTypeDefinitions[ref],
		document.EnumTypeDefinitionNameString(ref),
		nil,
	}
	e.valueSet = createValueSet(r, e)
	return e
}

func (e EnumValueType) ValueRefs() []int {
	return e.EnumValuesDefinition.Refs
}

func (e EnumValueType) AreValuesIdentical(r *removeDuplicateFieldlessValueTypesVisitor, valueRefsToCompare []int) bool {
	if len(e.ValueRefs()) != len(valueRefsToCompare) {
		return false
	}
	for _, refToCompare := range valueRefsToCompare {
		name := e.ValueName(r, refToCompare)
		if !e.valueSet[name] {
			return false
		}
	}
	return true
}

func (_ EnumValueType) ValueName(r *removeDuplicateFieldlessValueTypesVisitor, ref int) string {
	return r.document.EnumValueDefinitionNameString(ref)
}

type UnionValueType struct {
	*ast.UnionTypeDefinition
	name     string
	valueSet map[string]bool
}

func NewUnionValueType(r *removeDuplicateFieldlessValueTypesVisitor, ref int) UnionValueType {
	document := r.document
	u := UnionValueType{
		&document.UnionTypeDefinitions[ref],
		document.UnionTypeDefinitionNameString(ref),
		nil,
	}
	u.valueSet = createValueSet(r, u)
	return u
}

func (u UnionValueType) AreValuesIdentical(r *removeDuplicateFieldlessValueTypesVisitor, valueRefsToCompare []int) bool {
	if len(u.ValueRefs()) != len(valueRefsToCompare) {
		return false
	}
	for _, refToCompare := range valueRefsToCompare {
		name := u.ValueName(r, refToCompare)
		if !u.valueSet[name] {
			return false
		}
	}
	return true
}

func (u UnionValueType) ValueRefs() []int {
	return u.UnionMemberTypes.Refs
}

func (_ UnionValueType) ValueName(r *removeDuplicateFieldlessValueTypesVisitor, ref int) string {
	return r.document.TypeNameString(ref)
}

type ScalarValueType struct {
	name string
}

func (_ ScalarValueType) ValueRefs() []int {
	return nil
}

func (_ ScalarValueType) AreValuesIdentical(_ *removeDuplicateFieldlessValueTypesVisitor, _ []int) bool {
	return true
}

func (_ ScalarValueType) ValueName(_ *removeDuplicateFieldlessValueTypesVisitor, _ int) string {
	return ""
}
