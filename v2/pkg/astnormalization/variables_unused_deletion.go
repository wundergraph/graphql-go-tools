package astnormalization

import (
	"fmt"
	"slices"

	"github.com/tidwall/sjson"

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

	operation, definition        *ast.Document
	variableNamesUsed            []string
	variableNamesSafeForDeletion []string
}

func (d *deleteUnusedVariablesVisitor) LeaveOperationDefinition(ref int) {
	var (
		err error
	)
	filterOutIndices := make([]int, 0, len(d.operation.OperationDefinitions[ref].VariableDefinitions.Refs))
	for i := range d.operation.OperationDefinitions[ref].VariableDefinitions.Refs {
		name := d.operation.VariableDefinitionNameString(d.operation.OperationDefinitions[ref].VariableDefinitions.Refs[i])
		if slices.Contains(d.variableNamesUsed, name) {
			continue
		}
		if !slices.Contains(d.variableNamesSafeForDeletion, name) {
			continue
		}
		filterOutIndices = append(filterOutIndices, i)
		d.operation.Input.Variables, err = sjson.DeleteBytes(d.operation.Input.Variables, name)
		if err != nil {
			d.Walker.StopWithInternalErr(fmt.Errorf("deleteUnusedVariablesVisitor.LeaveOperationDefinition: unable to delete variable %s: %w", name, err))
			return
		}
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
		d.variableNamesUsed = append(d.variableNamesUsed, name)
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
	d.variableNamesUsed = d.variableNamesUsed[:0]
}

func detectVariableUsage(walker *astvisitor.Walker, deletion *deleteUnusedVariablesVisitor) *variableUsageDetector {
	visitor := &variableUsageDetector{
		Walker:   walker,
		deletion: deletion,
	}
	visitor.Walker.RegisterEnterDocumentVisitor(visitor)
	visitor.Walker.RegisterEnterArgumentVisitor(visitor)
	return visitor
}

type variableUsageDetector struct {
	*astvisitor.Walker

	operation, definition *ast.Document
	deletion              *deleteUnusedVariablesVisitor
}

func (v *variableUsageDetector) EnterDocument(operation, definition *ast.Document) {
	v.operation, v.definition = operation, definition
	v.deletion.variableNamesSafeForDeletion = v.deletion.variableNamesSafeForDeletion[:0]
}

func (v *variableUsageDetector) EnterArgument(ref int) {
	v.traverseValue(v.operation.Arguments[ref].Value)
}

func (v *variableUsageDetector) traverseValue(value ast.Value) {
	switch value.Kind {
	case ast.ValueKindVariable:
		name := v.operation.VariableValueNameString(value.Ref)
		if !slices.Contains(v.deletion.variableNamesSafeForDeletion, name) {
			v.deletion.variableNamesSafeForDeletion = append(v.deletion.variableNamesSafeForDeletion, name)
		}
	case ast.ValueKindList:
		for _, ref := range v.operation.ListValues[value.Ref].Refs {
			v.traverseValue(v.operation.Value(ref))
		}
	case ast.ValueKindObject:
		for _, ref := range v.operation.ObjectValues[value.Ref].Refs {
			v.traverseValue(v.operation.ObjectField(ref).Value)
		}
	}
}
