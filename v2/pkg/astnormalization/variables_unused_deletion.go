package astnormalization

import (
	"slices"

	"github.com/buger/jsonparser"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astvisitor"
)

func deleteUnusedVariables(walker *astvisitor.Walker) *deleteUnusedVariablesVisitor {
	visitor := &deleteUnusedVariablesVisitor{
		Walker: walker,
	}
	visitor.Walker.RegisterEnterDocumentVisitor(visitor)
	visitor.Walker.RegisterEnterArgumentVisitor(visitor)
	visitor.Walker.RegisterLeaveOperationVisitor(visitor)
	return visitor
}

type deleteUnusedVariablesVisitor struct {
	*astvisitor.Walker
	operation, definition *ast.Document
	usedVariableNames     []string
}

func (d *deleteUnusedVariablesVisitor) LeaveOperationDefinition(ref int) {
	filterOutIndices := make([]int, 0, len(d.operation.OperationDefinitions[ref].VariableDefinitions.Refs))
	for i := range d.operation.OperationDefinitions[ref].VariableDefinitions.Refs {
		name := d.operation.VariableDefinitionNameString(d.operation.OperationDefinitions[ref].VariableDefinitions.Refs[i])
		if slices.Contains(d.usedVariableNames, name) {
			continue
		}
		filterOutIndices = append(filterOutIndices, i)
		d.operation.Input.Variables = jsonparser.Delete(d.operation.Input.Variables, name)
	}
	if len(filterOutIndices) == 0 {
		return
	}
	cop := make([]int, 0, len(d.operation.OperationDefinitions[ref].VariableDefinitions.Refs)-len(filterOutIndices))
	for i := range d.operation.OperationDefinitions[ref].VariableDefinitions.Refs {
		if slices.Contains(filterOutIndices, i) {
			continue
		}
		cop = append(cop, d.operation.OperationDefinitions[ref].VariableDefinitions.Refs[i])
	}
	d.operation.OperationDefinitions[ref].VariableDefinitions.Refs = cop
	if len(d.operation.OperationDefinitions[ref].VariableDefinitions.Refs) == 0 {
		d.operation.OperationDefinitions[ref].HasVariableDefinitions = false
	}
}

func (d *deleteUnusedVariablesVisitor) EnterArgument(ref int) {
	d.traverseValue(d.operation.Arguments[ref].Value)
}

func (d *deleteUnusedVariablesVisitor) traverseValue(value ast.Value) {
	switch value.Kind {
	case ast.ValueKindVariable:
		name := d.operation.VariableValueNameString(value.Ref)
		d.usedVariableNames = append(d.usedVariableNames, name)
	case ast.ValueKindList:
		for _, ref := range d.operation.ListValues[value.Ref].Refs {
			d.traverseValue(d.operation.Value(ref))
		}
	case ast.ValueKindObject:
		for _, ref := range d.operation.ObjectValues[value.Ref].Refs {
			d.traverseValue(d.operation.ObjectField(ref).Value)
		}
	}
}

func (d *deleteUnusedVariablesVisitor) EnterDocument(operation, definition *ast.Document) {
	d.operation, d.definition = operation, definition
	d.usedVariableNames = d.usedVariableNames[:0]
}
