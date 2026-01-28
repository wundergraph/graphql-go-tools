package plan

import (
	"strings"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astvisitor"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/operationreport"
)

type providesInput struct {
	providesFieldSet, operation, definition *ast.Document
	report                                  *operationreport.Report
	operationSelectionSet                   int
	parentPath                              string
	dataSource                              DataSource
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

func providesSuggestions(input *providesInput) []*NodeSuggestion {
	walker := astvisitor.NewWalkerWithID(48, "ProvidesVisitor")

	visitor := &providesVisitor{
		walker: &walker,
		input:  input,
	}
	walker.RegisterEnterDocumentVisitor(visitor)
	walker.RegisterEnterFragmentDefinitionVisitor(visitor)
	walker.RegisterEnterFieldVisitor(visitor)

	walker.Walk(input.providesFieldSet, input.definition, input.report)

	return visitor.suggestions
}

type providesVisitor struct {
	walker *astvisitor.Walker
	input  *providesInput

	suggestions []*NodeSuggestion
	pathPrefix  string
}

func (v *providesVisitor) EnterFragmentDefinition(ref int) {
	v.pathPrefix = v.input.providesFieldSet.FragmentDefinitionTypeNameString(ref)
}

func (v *providesVisitor) EnterDocument(_, _ *ast.Document) {

}

func (v *providesVisitor) EnterField(ref int) {
	fieldName := v.input.providesFieldSet.FieldNameUnsafeString(ref)
	typeName := v.walker.EnclosingTypeDefinition.NameString(v.input.definition)

	currentPathWithoutFragments := v.walker.Path.WithoutInlineFragmentNames().DotDelimitedString(true)
	parentPath := v.input.parentPath + strings.TrimPrefix(currentPathWithoutFragments, v.pathPrefix)
	currentPath := parentPath + "." + fieldName

	suggestion := &NodeSuggestion{
		FieldRef:       ast.InvalidRef,
		TypeName:       typeName,
		FieldName:      fieldName,
		DataSourceHash: v.input.dataSource.Hash(),
		DataSourceID:   v.input.dataSource.Id(),
		DataSourceName: v.input.dataSource.Name(),
		Path:           currentPath,
		ParentPath:     parentPath,
	}

	v.suggestions = append(v.suggestions, suggestion)
}

func (v *providesVisitor) LeaveField(ref int) {

}
