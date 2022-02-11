package astvalidation

import (
	"github.com/wundergraph/graphql-go-tools/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/pkg/astvisitor"
	"github.com/wundergraph/graphql-go-tools/pkg/operationreport"
)

// VariablesAreInputTypes validates if variables are correct input types
func VariablesAreInputTypes() Rule {
	return func(walker *astvisitor.Walker) {
		visitor := variablesAreInputTypesVisitor{
			Walker: walker,
		}
		walker.RegisterEnterDocumentVisitor(&visitor)
		walker.RegisterEnterVariableDefinitionVisitor(&visitor)
	}
}

type variablesAreInputTypesVisitor struct {
	*astvisitor.Walker
	operation, definition *ast.Document
}

func (v *variablesAreInputTypesVisitor) EnterDocument(operation, definition *ast.Document) {
	v.operation = operation
	v.definition = definition
}

func (v *variablesAreInputTypesVisitor) EnterVariableDefinition(ref int) {

	typeName := v.operation.ResolveTypeNameBytes(v.operation.VariableDefinitions[ref].Type)
	typeDefinitionNode, ok := v.definition.Index.FirstNodeByNameBytes(typeName)
	if !ok {
		v.Report.AddExternalError(operationreport.ErrUnknownType(typeName, v.operation.Types[v.operation.VariableDefinitions[ref].Type].Position))
		return
	}

	switch typeDefinitionNode.Kind {
	case ast.NodeKindInputObjectTypeDefinition, ast.NodeKindScalarTypeDefinition, ast.NodeKindEnumTypeDefinition:
		return
	default:
		variableName := v.operation.VariableDefinitionNameBytes(ref)
		variableTypePos := v.operation.Types[v.operation.VariableDefinitions[ref].Type].Position

		printedType, err := v.operation.PrintTypeBytes(v.operation.VariableDefinitions[ref].Type, nil)
		if v.HandleInternalErr(err) {
			return
		}

		v.Report.AddExternalError(operationreport.ErrVariableOfTypeIsNoValidInputValue(variableName, printedType, variableTypePos))
		return
	}
}
