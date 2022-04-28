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
			newRemoveDuplicateFieldedValueTypesVisitor(),
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

type FieldedValueType interface {
	FieldRefs() []int
	FieldSet() map[string]string
	fieldName(ref int) string
	fieldTypeName(ref int) string
}

type NonInputFieldedValueType struct {
	document  *ast.Document
	fieldRefs []int
	fieldSet  map[string]string
}

func createFieldSet(f FieldedValueType) map[string]string {
	fieldSet := make(map[string]string)
	for _, fieldRef := range f.FieldRefs() {
		fieldSet[f.fieldName(fieldRef)] = f.fieldTypeName(fieldRef)
	}
	return fieldSet
}

func AreFieldsIdentical(f FieldedValueType, fieldRefsToCompare []int) bool {
	if len(f.FieldRefs()) != len(fieldRefsToCompare) {
		return false
	}
	for _, fieldRef := range fieldRefsToCompare {
		actualFieldName := f.fieldName(fieldRef)
		expectedTypeName, exists := f.FieldSet()[actualFieldName]
		if !exists {
			return false
		}
		actualTypeName := f.fieldTypeName(fieldRef)
		if expectedTypeName != actualTypeName {
			return false
		}
	}
	return true
}

func NewNonInputFieldedValueType(document *ast.Document, fieldRefs []int) NonInputFieldedValueType {
	i := NonInputFieldedValueType{
		document,
		fieldRefs,
		nil,
	}
	i.fieldSet = createFieldSet(i)
	return i
}

func (n NonInputFieldedValueType) FieldRefs() []int {
	return n.fieldRefs
}

func (n NonInputFieldedValueType) FieldSet() map[string]string {
	return n.fieldSet
}

func (n NonInputFieldedValueType) fieldName(ref int) string {
	return n.document.FieldDefinitionNameString(ref)
}

func (n NonInputFieldedValueType) fieldTypeName(ref int) string {
	document := n.document
	typeRef := document.FieldDefinitions[ref].Type
	return document.TypeNameString(typeRef)
}

type InputFieldedValueType struct {
	document  *ast.Document
	fieldRefs []int
	fieldSet  map[string]string
}

func NewInputFieldedValueType(document *ast.Document, fieldRefs []int) InputFieldedValueType {
	i := InputFieldedValueType{
		document,
		fieldRefs,
		nil,
	}
	i.fieldSet = createFieldSet(i)
	return i
}

func (i InputFieldedValueType) FieldRefs() []int {
	return i.fieldRefs
}

func (i InputFieldedValueType) FieldSet() map[string]string {
	return i.fieldSet
}

func (i InputFieldedValueType) fieldName(ref int) string {
	return i.document.InputValueDefinitionNameString(ref)
}

func (i InputFieldedValueType) fieldTypeName(ref int) string {
	document := i.document
	typeRef := document.InputValueDefinitions[ref].Type
	return document.TypeNameString(typeRef)
}

type FieldlessValueType interface {
	AreValuesIdentical(valueRefsToCompare []int) bool
	valueRefs() []int
	valueName(ref int) string
}

func createValueSet(f FieldlessValueType) map[string]bool {
	valueSet := make(map[string]bool)
	for _, valueRef := range f.valueRefs() {
		valueSet[f.valueName(valueRef)] = true
	}
	return valueSet
}

type EnumValueType struct {
	*ast.EnumTypeDefinition
	document *ast.Document
	valueSet map[string]bool
}

func NewEnumValueType(document *ast.Document, ref int) EnumValueType {
	e := EnumValueType{
		&document.EnumTypeDefinitions[ref],
		document,
		nil,
	}
	e.valueSet = createValueSet(e)
	return e
}

func (e EnumValueType) AreValuesIdentical(valueRefsToCompare []int) bool {
	if len(e.valueRefs()) != len(valueRefsToCompare) {
		return false
	}
	for _, valueRefToCompare := range valueRefsToCompare {
		name := e.valueName(valueRefToCompare)
		if !e.valueSet[name] {
			return false
		}
	}
	return true
}

func (e EnumValueType) valueRefs() []int {
	return e.EnumValuesDefinition.Refs
}

func (e EnumValueType) valueName(ref int) string {
	return e.document.EnumValueDefinitionNameString(ref)
}

type UnionValueType struct {
	*ast.UnionTypeDefinition
	document *ast.Document
	valueSet map[string]bool
}

func NewUnionValueType(document *ast.Document, ref int) UnionValueType {
	u := UnionValueType{
		&document.UnionTypeDefinitions[ref],
		document,
		nil,
	}
	u.valueSet = createValueSet(u)
	return u
}

func (u UnionValueType) AreValuesIdentical(valueRefsToCompare []int) bool {
	if len(u.valueRefs()) != len(valueRefsToCompare) {
		return false
	}
	for _, refToCompare := range valueRefsToCompare {
		name := u.valueName(refToCompare)
		if !u.valueSet[name] {
			return false
		}
	}
	return true
}

func (u UnionValueType) valueRefs() []int {
	return u.UnionMemberTypes.Refs
}

func (u UnionValueType) valueName(ref int) string {
	return u.document.TypeNameString(ref)
}

type ScalarValueType struct {
}

func (_ ScalarValueType) AreValuesIdentical(_ []int) bool {
	return true
}

func (_ ScalarValueType) valueRefs() []int {
	return nil
}

func (_ ScalarValueType) valueName(_ int) string {
	return ""
}
