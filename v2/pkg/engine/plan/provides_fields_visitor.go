package plan

import (
	"strings"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astvisitor"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/operationreport"
)

type providesInput struct {
	parentTypeName       string
	providesSelectionSet string
	definition           *ast.Document
	parentPath           string
}

type addTypenamesVisitor struct {
	walker                       *astvisitor.Walker
	providesFieldSet, definition *ast.Document
}

func (a *addTypenamesVisitor) EnterSelectionSet(ref int) {
	exist, _ := a.providesFieldSet.SelectionSetHasFieldSelectionWithExactName(ref, typeNameFieldBytes)
	if exist {
		return
	}

	// Add __typename to the selection set
	fieldNode := a.providesFieldSet.AddField(ast.Field{
		Name: a.providesFieldSet.Input.AppendInputBytes(typeNameFieldBytes),
	})
	selectionRef := a.providesFieldSet.AddSelectionToDocument(ast.Selection{
		Ref:  fieldNode.Ref,
		Kind: ast.SelectionKindField,
	})
	a.providesFieldSet.AddSelectionRefToSelectionSet(ref, selectionRef)
}

func addTypenames(operation, definition *ast.Document, report *operationreport.Report) {
	walker := astvisitor.NewWalkerWithID(32, "AddTypenamesVisitor")
	visitor := &addTypenamesVisitor{
		walker:           &walker,
		providesFieldSet: operation,
		definition:       definition,
	}
	walker.RegisterEnterSelectionSetVisitor(visitor)
	walker.Walk(operation, definition, report)
}

func providesFragment(fieldTypeName string, providesSelectionSet string, definition *ast.Document) (*ast.Document, *operationreport.Report) {
	providesFieldSet, report := RequiredFieldsFragment(fieldTypeName, providesSelectionSet, false)
	if report.HasErrors() {
		return nil, report
	}

	// prewalk provides selection set and add a typename to each selection set
	// as when we could select a field we could select __typename as well
	addTypenames(providesFieldSet, definition, report)
	if report.HasErrors() {
		return nil, report
	}

	return providesFieldSet, report
}

func providesSuggestions(input *providesInput) (map[string]struct{}, *operationreport.Report) {
	providesFieldSet, report := providesFragment(input.parentTypeName, input.providesSelectionSet, input.definition)
	if report.HasErrors() {
		return nil, report
	}

	walker := astvisitor.NewWalkerWithID(48, "ProvidesVisitor")

	visitor := &providesVisitor{
		walker:           &walker,
		suggestions:      make(map[string]struct{}),
		providesFieldSet: providesFieldSet,
		definition:       input.definition,
		parentTypeName:   input.parentTypeName,
		parentPath:       input.parentPath,
	}
	walker.RegisterEnterFieldVisitor(visitor)

	walker.Walk(providesFieldSet, input.definition, report)

	return visitor.suggestions, report
}

type providesVisitor struct {
	walker      *astvisitor.Walker
	suggestions map[string]struct{}

	providesFieldSet *ast.Document
	definition       *ast.Document
	parentTypeName   string
	parentPath       string
}

func (v *providesVisitor) EnterField(ref int) {
	fieldName := v.providesFieldSet.FieldNameUnsafeString(ref)
	typeName := v.walker.EnclosingTypeDefinition.NameString(v.definition)

	currentPathWithoutFragments := v.walker.Path.WithoutInlineFragmentNames().DotDelimitedString(true)
	// remove the parent type name from the path because we are walking a fragment with the provided fields
	parentPath := v.parentPath + strings.TrimPrefix(currentPathWithoutFragments, v.parentTypeName)
	currentPath := parentPath + "." + fieldName

	v.suggestions[providedFieldKey(typeName, fieldName, currentPath)] = struct{}{}
}

// providedFieldKey returns a unique key for a provided field
// it consists of the type name, field name and dot delimited path from a query
func providedFieldKey(typeName, fieldName, path string) string {
	return typeName + "|" + fieldName + "|" + path
}
