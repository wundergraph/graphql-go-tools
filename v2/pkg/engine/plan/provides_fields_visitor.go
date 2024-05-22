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

	parentPath                        string
	DSHash                            DSHash
	suggestionSelectionReasonsEnabled bool
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

type providesVisitor struct {
	walker         *astvisitor.Walker
	input          *providesInput
	OperationNodes []ast.Node

	suggestions []*NodeSuggestion
	pathPrefix  string
}

func (v *providesVisitor) EnterFragmentDefinition(ref int) {
	v.pathPrefix = v.input.providesFieldSet.FragmentDefinitionTypeNameString(ref)
}

func (v *providesVisitor) EnterDocument(_, _ *ast.Document) {
	v.suggestions = make([]*NodeSuggestion, 0, 8)
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

	suggestion := &NodeSuggestion{
		fieldRef:       operationFieldRef,
		TypeName:       typeName,
		FieldName:      fieldName,
		DataSourceHash: v.input.DSHash,
		Path:           currentPath,
		ParentPath:     parentPath,
		Selected:       true,
		IsProvided:     true,
	}

	if v.input.suggestionSelectionReasonsEnabled {
		suggestion.SelectionReasons = append(suggestion.SelectionReasons, ReasonProvidesProvidedByPlanner)
	}

	v.suggestions = append(v.suggestions, suggestion)
}

func (v *providesVisitor) LeaveField(ref int) {
	if v.input.providesFieldSet.FieldHasSelections(ref) {
		v.OperationNodes = v.OperationNodes[:len(v.OperationNodes)-1]
	}
}
