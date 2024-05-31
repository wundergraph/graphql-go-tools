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
	dataSourceID                            string
	dataSourceHash                          DSHash
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
	walker := astvisitor.NewWalker(32)
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
	walker := astvisitor.NewWalker(48)

	visitor := &providesVisitor{
		walker: &walker,
		input:  input,
	}
	walker.RegisterEnterDocumentVisitor(visitor)
	walker.RegisterEnterFragmentDefinitionVisitor(visitor)
	walker.RegisterFieldVisitor(visitor)
	walker.RegisterSelectionSetVisitor(visitor)

	walker.Walk(input.providesFieldSet, input.definition, input.report)

	return visitor.suggestions
}

type currentFiedInfo struct {
	fieldRef                           int
	fieldName                          string
	isSelected                         bool
	hasSelections                      bool
	hasSelectedNestedFieldsInOperation bool
	suggestion                         *NodeSuggestion
}

type providesVisitor struct {
	walker         *astvisitor.Walker
	input          *providesInput
	OperationNodes []ast.Node

	suggestions   []*NodeSuggestion
	pathPrefix    string
	currentFields []*currentFiedInfo
}

func (v *providesVisitor) EnterFragmentDefinition(ref int) {
	v.pathPrefix = v.input.providesFieldSet.FragmentDefinitionTypeNameString(ref)
}

func (v *providesVisitor) EnterDocument(_, _ *ast.Document) {
	v.OperationNodes = make([]ast.Node, 0, 3)
	v.OperationNodes = append(v.OperationNodes,
		ast.Node{Kind: ast.NodeKindSelectionSet, Ref: v.input.operationSelectionSet})
}

func (v *providesVisitor) EnterSelectionSet(_ int) {
	if v.walker.Depth == 2 {
		return
	}
	fieldNode := v.OperationNodes[len(v.OperationNodes)-1]

	if fieldSelectionSetRef, ok := v.input.operation.FieldSelectionSet(fieldNode.Ref); ok {
		selectionSetNode := ast.Node{Kind: ast.NodeKindSelectionSet, Ref: fieldSelectionSetRef}
		v.OperationNodes = append(v.OperationNodes, selectionSetNode)
	}
}

func (v *providesVisitor) LeaveSelectionSet(ref int) {
	if v.walker.Depth == 0 {
		return
	}

	v.OperationNodes = v.OperationNodes[:len(v.OperationNodes)-1]
}

func (v *providesVisitor) EnterField(ref int) {
	fieldNameBytes := v.input.providesFieldSet.FieldNameBytes(ref)
	fieldName := v.input.providesFieldSet.FieldNameUnsafeString(ref)

	selectionSetRef := v.OperationNodes[len(v.OperationNodes)-1].Ref
	operationHasField, operationFieldRef := v.input.operation.SelectionSetHasFieldSelectionWithExactName(selectionSetRef, fieldNameBytes)
	if !operationHasField {
		// we haven't selected this field in the operation,
		// so we don't have to add node suggestions for it
		// and we don't want to check nested fields for this field if any present in the fieldset
		v.walker.SkipNode()
		return
	}
	if v.input.providesFieldSet.FieldHasSelections(ref) {
		v.OperationNodes = append(v.OperationNodes, ast.Node{Kind: ast.NodeKindField, Ref: operationFieldRef})
	}

	typeName := v.walker.EnclosingTypeDefinition.NameString(v.input.definition)
	parentPath := v.input.parentPath + strings.TrimPrefix(v.walker.Path.DotDelimitedString(), v.pathPrefix)
	currentPath := parentPath + "." + fieldName

	if len(v.currentFields) > 0 {
		v.currentFields[len(v.currentFields)-1].hasSelectedNestedFieldsInOperation = true
	}

	suggestion := &NodeSuggestion{
		fieldRef:       operationFieldRef,
		TypeName:       typeName,
		FieldName:      fieldName,
		DataSourceHash: v.input.dataSourceHash,
		DataSourceID:   v.input.dataSourceID,
		Path:           currentPath,
		ParentPath:     parentPath,
		Selected:       false,
		IsProvided:     true,
	}

	v.currentFields = append(v.currentFields, &currentFiedInfo{
		fieldRef:      ref,
		fieldName:     fieldName,
		isSelected:    true,
		hasSelections: v.input.providesFieldSet.FieldHasSelections(ref),
		suggestion:    suggestion,
	})
}

func (v *providesVisitor) LeaveField(ref int) {
	if v.input.providesFieldSet.FieldHasSelections(ref) {
		v.OperationNodes = v.OperationNodes[:len(v.OperationNodes)-1]
	}

	currentField := v.currentFields[len(v.currentFields)-1]
	v.currentFields = v.currentFields[:len(v.currentFields)-1]

	if !currentField.isSelected {
		return
	}

	if currentField.hasSelections && !currentField.hasSelectedNestedFieldsInOperation {
		// we don't have to add node suggestions for this field
		// if it has selections in the fieldset but no nested fields selected in the operation
		return
	}

	v.suggestions = append(v.suggestions, currentField.suggestion)
}
