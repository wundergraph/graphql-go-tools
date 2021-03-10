package astnormalization

import (
	"bytes"

	"github.com/buger/jsonparser"

	"github.com/jensneuse/graphql-go-tools/pkg/ast"
	"github.com/jensneuse/graphql-go-tools/pkg/astvisitor"
)

func deleteUnusedVariables(walker *astvisitor.Walker) *deleteUnusedVariablesVisitor {
	visitor := &deleteUnusedVariablesVisitor{
		Walker: walker,
	}
	visitor.Walker.RegisterEnterDocumentVisitor(visitor)
	visitor.Walker.RegisterOperationDefinitionVisitor(visitor)
	visitor.Walker.RegisterEnterVariableDefinitionVisitor(visitor)
	visitor.Walker.RegisterEnterArgumentVisitor(visitor)
	return visitor
}

type deleteUnusedVariablesVisitor struct {
	*astvisitor.Walker
	operation, definition *ast.Document
	definedVariables      []int
	operationName         []byte
}

func (d *deleteUnusedVariablesVisitor) LeaveOperationDefinition(ref int) {
	for _, variable := range d.definedVariables {
		variableName := d.operation.VariableDefinitionNameString(variable)
		for i, variableDefinitionRef := range d.operation.OperationDefinitions[ref].VariableDefinitions.Refs {
			if variable == variableDefinitionRef {
				d.operation.OperationDefinitions[ref].VariableDefinitions.Refs = append(d.operation.OperationDefinitions[ref].VariableDefinitions.Refs[:i],d.operation.OperationDefinitions[ref].VariableDefinitions.Refs[i+1:]...)
				d.operation.Input.Variables = jsonparser.Delete(d.operation.Input.Variables,variableName)
				d.operation.OperationDefinitions[ref].HasVariableDefinitions = len(d.operation.OperationDefinitions[ref].VariableDefinitions.Refs) != 0
			}
		}

	}
}

func (d *deleteUnusedVariablesVisitor) EnterArgument(ref int) {
	if d.operation.Arguments[ref].Value.Kind != ast.ValueKindVariable {
		return
	}
	usedVariableNameBytes := d.operation.VariableValueNameBytes(d.operation.Arguments[ref].Value.Ref)
	for i, variable := range d.definedVariables {
		definedVariableNameBytes := d.operation.VariableDefinitionNameBytes(variable)
		if bytes.Equal(usedVariableNameBytes, definedVariableNameBytes) {
			d.definedVariables = append(d.definedVariables[:i], d.definedVariables[i+1:]...)
			return
		}
	}
}

func (d *deleteUnusedVariablesVisitor) EnterVariableDefinition(ref int) {
	d.definedVariables = append(d.definedVariables, ref)
}

func (d *deleteUnusedVariablesVisitor) EnterOperationDefinition(ref int) {
	d.definedVariables = d.definedVariables[:0]
}

func (d *deleteUnusedVariablesVisitor) EnterDocument(operation, definition *ast.Document) {
	d.operation, d.definition = operation, definition
}
