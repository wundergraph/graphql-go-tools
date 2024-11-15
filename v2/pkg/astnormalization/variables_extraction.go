package astnormalization

import (
	"bytes"

	"github.com/buger/jsonparser"
	"github.com/tidwall/sjson"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astimport"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astvisitor"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/internal/unsafebytes"
)

func extractVariables(walker *astvisitor.Walker) *variablesExtractionVisitor {
	visitor := &variablesExtractionVisitor{
		Walker: walker,
	}
	walker.RegisterEnterDocumentVisitor(visitor)
	walker.RegisterEnterArgumentVisitor(visitor)
	return visitor
}

type variablesExtractionVisitor struct {
	*astvisitor.Walker
	operation, definition     *ast.Document
	importer                  astimport.Importer
	skip                      bool
	extractedVariables        [][]byte
	extractedVariableTypeRefs []int
}

func (v *variablesExtractionVisitor) EnterArgument(ref int) {
	if v.operation.Arguments[ref].Value.Kind == ast.ValueKindVariable {
		return
	}
	if len(v.Ancestors) == 0 || v.Ancestors[0].Kind != ast.NodeKindOperationDefinition {
		return
	}

	for i := range v.Ancestors {
		if v.Ancestors[i].Kind == ast.NodeKindDirective {
			return // skip all directives in any case
		}
	}

	inputValueDefinition, ok := v.Walker.ArgumentInputValueDefinition(ref)
	if !ok {
		return
	}
	valueBytes, err := v.operation.ValueToJSON(v.operation.Arguments[ref].Value)
	if err != nil {
		v.StopWithInternalErr(err)
		return
	}
	if exists, name, _ := v.variableExists(valueBytes, inputValueDefinition); exists {
		variable := ast.VariableValue{
			Name: v.operation.Input.AppendInputBytes(name),
		}
		value := v.operation.AddVariableValue(variable)
		v.operation.Arguments[ref].Value.Kind = ast.ValueKindVariable
		v.operation.Arguments[ref].Value.Ref = value
		return
	}
	variableNameBytes := v.operation.GenerateUnusedVariableDefinitionName(v.Ancestors[0].Ref)
	v.operation.Input.Variables, err = sjson.SetRawBytes(v.operation.Input.Variables, unsafebytes.BytesToString(variableNameBytes), valueBytes)
	if err != nil {
		v.StopWithInternalErr(err)
		return
	}

	v.extractedVariables = append(v.extractedVariables, variableNameBytes)
	v.extractedVariableTypeRefs = append(v.extractedVariableTypeRefs, v.definition.InputValueDefinitions[inputValueDefinition].Type)

	variable := ast.VariableValue{
		Name: v.operation.Input.AppendInputBytes(variableNameBytes),
	}

	v.operation.VariableValues = append(v.operation.VariableValues, variable)

	varRef := len(v.operation.VariableValues) - 1

	v.operation.Arguments[ref].Value.Ref = varRef
	v.operation.Arguments[ref].Value.Kind = ast.ValueKindVariable

	defRef, ok := v.ArgumentInputValueDefinition(ref)
	if !ok {
		return
	}

	defType := v.definition.InputValueDefinitions[defRef].Type

	importedDefType := v.importer.ImportType(defType, v.definition, v.operation)

	v.operation.VariableDefinitions = append(v.operation.VariableDefinitions, ast.VariableDefinition{
		VariableValue: ast.Value{
			Kind: ast.ValueKindVariable,
			Ref:  varRef,
		},
		Type: importedDefType,
	})

	newVariableRef := len(v.operation.VariableDefinitions) - 1

	v.operation.OperationDefinitions[v.Ancestors[0].Ref].VariableDefinitions.Refs =
		append(v.operation.OperationDefinitions[v.Ancestors[0].Ref].VariableDefinitions.Refs, newVariableRef)
	v.operation.OperationDefinitions[v.Ancestors[0].Ref].HasVariableDefinitions = true
}

func (v *variablesExtractionVisitor) EnterDocument(operation, definition *ast.Document) {
	v.operation, v.definition = operation, definition
	v.extractedVariables = v.extractedVariables[:0]
	v.extractedVariableTypeRefs = v.extractedVariableTypeRefs[:0]
}

func (v *variablesExtractionVisitor) variableExists(variableValue []byte, inputValueDefinition int) (exists bool, name []byte, definition int) {
	_ = jsonparser.ObjectEach(v.operation.Input.Variables, func(key []byte, value []byte, dataType jsonparser.ValueType, offset int) error {
		if !v.extractedVariablesContainsKey(key, inputValueDefinition) {
			// skip variables that were not extracted but user defined
			return nil
		}
		if dataType == jsonparser.String {
			value = v.operation.Input.Variables[offset-len(value)-2 : offset]
		}
		if bytes.Equal(value, variableValue) {
			exists = true
			name = key
		}
		return nil
	})
	if exists {
		definition, exists = v.operation.VariableDefinitionByNameAndOperation(v.Ancestors[0].Ref, name)
	}
	return
}

func (v *variablesExtractionVisitor) extractedVariablesContainsKey(key []byte, inputValueDefinition int) bool {
	typeRef := v.definition.InputValueDefinitions[inputValueDefinition].Type
	for i := range v.extractedVariables {
		if bytes.Equal(v.extractedVariables[i], key) && v.definition.TypesAreEqualDeep(typeRef, v.extractedVariableTypeRefs[i]) {
			return true
		}
	}
	return false
}
