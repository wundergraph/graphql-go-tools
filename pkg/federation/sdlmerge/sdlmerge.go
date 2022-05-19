package sdlmerge

import (
	"fmt"
	"github.com/jensneuse/graphql-go-tools/pkg/astvalidation"
	"strings"

	"github.com/jensneuse/graphql-go-tools/pkg/ast"
	"github.com/jensneuse/graphql-go-tools/pkg/astnormalization"
	"github.com/jensneuse/graphql-go-tools/pkg/astparser"
	"github.com/jensneuse/graphql-go-tools/pkg/astprinter"
	"github.com/jensneuse/graphql-go-tools/pkg/astvisitor"
	"github.com/jensneuse/graphql-go-tools/pkg/operationreport"
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

type ParsedSubgraph struct {
	document *ast.Document
	report   *operationreport.Report
}

func MergeSDLs(SDLs ...string) (string, error) {
	rawDocs := make([]string, 0, len(SDLs)+1)
	rawDocs = append(rawDocs, rootOperationTypeDefinitions)
	rawDocs = append(rawDocs, SDLs...)
	parsedSubgraphs, validationError := validateSubgraphs(rawDocs)
	if validationError != nil {
		return "", validationError
	}
	if normalizationError := normalizeSubgraphs(rawDocs, parsedSubgraphs); normalizationError != nil {
		return "", normalizationError
	}

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

func validateSubgraphs(subgraphs []string) ([]ParsedSubgraph, error) {
	validator := astvalidation.NewDefinitionValidator(astvalidation.PopulatedTypeBodies())
	parsedSubgraphs := make([]ParsedSubgraph, 0, len(subgraphs))
	for _, subgraph := range subgraphs {
		doc, report := astparser.ParseGraphqlDocumentString(subgraph)
		parsedSubgraph := ParsedSubgraph{&doc, &report}
		if report.HasErrors() {
			return parsedSubgraphs, fmt.Errorf("parse graphql document string: %s", report.Error())
		}
		validator.Validate(&doc, &report)
		if report.HasErrors() {
			return parsedSubgraphs, fmt.Errorf("validate subgraph: %s", report.Error())
		}
		parsedSubgraphs = append(parsedSubgraphs, parsedSubgraph)
	}
	return parsedSubgraphs, nil
}

func normalizeSubgraphs(subgraphs []string, parsedSubgraph []ParsedSubgraph) error {
	subgraphNormalizer := astnormalization.NewSubgraphDefinitionNormalizer()
	for i := range subgraphs {
		doc := parsedSubgraph[i].document
		report := parsedSubgraph[i].report
		subgraphNormalizer.NormalizeDefinition(doc, report)
		if report.HasErrors() {
			return fmt.Errorf("normalize subgraph: %s", report.Error())
		}
		out, err := astprinter.PrintString(doc, nil)
		if err != nil {
			return fmt.Errorf("stringify schema: %s", err.Error())
		}
		subgraphs[i] = out
	}
	return nil
}

type normalizer struct {
	walkers []*astvisitor.Walker
}

func (m *normalizer) setupWalkers() {
	visitorGroups := [][]Visitor{
		// visitors for extending objects and interfaces
		{
			newExtendEnumTypeDefinition(),
			newExtendInputObjectTypeDefinition(),
			newExtendInterfaceTypeDefinition(),
			newExtendScalarTypeDefinition(),
			newExtendUnionTypeDefinition(),
			newExtendObjectTypeDefinition(),
			newRemoveEmptyObjectTypeDefinition(),
			newRemoveMergedTypeExtensions(),
		},
		// visitors for cleaning up federated duplicated fields and directives
		{
			newRemoveFieldDefinitions("external"),
			newRemoveDuplicateFieldedSharedTypesVisitor(),
			newRemoveDuplicateFieldlessSharedTypesVisitor(),
			newRemoveInterfaceDefinitionDirective("key"),
			newRemoveObjectTypeDefinitionDirective("key"),
			newRemoveFieldDefinitionDirective("provides", "requires"),
			newCheckUnresolvedExtensionOrphansVisitor(),
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

type FieldedSharedType struct {
	document  *ast.Document
	fieldKind ast.NodeKind
	fieldRefs []int
	fieldSet  map[string]int
}

func NewFieldedSharedType(document *ast.Document, fieldKind ast.NodeKind, fieldRefs []int) FieldedSharedType {
	f := FieldedSharedType{
		document,
		fieldKind,
		fieldRefs,
		nil,
	}
	f.createFieldSet()
	return f
}

func (f FieldedSharedType) AreFieldsIdentical(fieldRefsToCompare []int) bool {
	if len(f.fieldRefs) != len(fieldRefsToCompare) {
		return false
	}
	for _, fieldRef := range fieldRefsToCompare {
		actualFieldName := f.fieldName(fieldRef)
		expectedTypeRef, exists := f.fieldSet[actualFieldName]
		if !exists {
			return false
		}
		actualTypeRef := f.fieldTypeRef(fieldRef)
		if !f.document.TypesAreCompatibleDeep(expectedTypeRef, actualTypeRef) {
			return false
		}
	}
	return true
}

func (f *FieldedSharedType) createFieldSet() {
	fieldSet := make(map[string]int)
	for _, fieldRef := range f.fieldRefs {
		fieldSet[f.fieldName(fieldRef)] = f.fieldTypeRef(fieldRef)
	}
	f.fieldSet = fieldSet
}

func (f FieldedSharedType) fieldName(ref int) string {
	switch f.fieldKind {
	case ast.NodeKindInputValueDefinition:
		return f.document.InputValueDefinitionNameString(ref)
	default:
		return f.document.FieldDefinitionNameString(ref)
	}
}

func (f FieldedSharedType) fieldTypeRef(ref int) int {
	document := f.document
	switch f.fieldKind {
	case ast.NodeKindInputValueDefinition:
		return document.InputValueDefinitions[ref].Type
	default:
		return document.FieldDefinitions[ref].Type
	}
}

type FieldlessSharedType interface {
	AreValuesIdentical(valueRefsToCompare []int) bool
	valueRefs() []int
	valueName(ref int) string
}

func createValueSet(f FieldlessSharedType) map[string]bool {
	valueSet := make(map[string]bool)
	for _, valueRef := range f.valueRefs() {
		valueSet[f.valueName(valueRef)] = true
	}
	return valueSet
}

type EnumSharedType struct {
	*ast.EnumTypeDefinition
	document *ast.Document
	valueSet map[string]bool
}

func NewEnumSharedType(document *ast.Document, ref int) EnumSharedType {
	e := EnumSharedType{
		&document.EnumTypeDefinitions[ref],
		document,
		nil,
	}
	e.valueSet = createValueSet(e)
	return e
}

func (e EnumSharedType) AreValuesIdentical(valueRefsToCompare []int) bool {
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

func (e EnumSharedType) valueRefs() []int {
	return e.EnumValuesDefinition.Refs
}

func (e EnumSharedType) valueName(ref int) string {
	return e.document.EnumValueDefinitionNameString(ref)
}

type UnionSharedType struct {
	*ast.UnionTypeDefinition
	document *ast.Document
	valueSet map[string]bool
}

func NewUnionSharedType(document *ast.Document, ref int) UnionSharedType {
	u := UnionSharedType{
		&document.UnionTypeDefinitions[ref],
		document,
		nil,
	}
	u.valueSet = createValueSet(u)
	return u
}

func (u UnionSharedType) AreValuesIdentical(valueRefsToCompare []int) bool {
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

func (u UnionSharedType) valueRefs() []int {
	return u.UnionMemberTypes.Refs
}

func (u UnionSharedType) valueName(ref int) string {
	return u.document.TypeNameString(ref)
}

type ScalarSharedType struct {
}

func (_ ScalarSharedType) AreValuesIdentical(_ []int) bool {
	return true
}

func (_ ScalarSharedType) valueRefs() []int {
	return nil
}

func (_ ScalarSharedType) valueName(_ int) string {
	return ""
}
