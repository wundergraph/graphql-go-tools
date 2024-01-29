package astnormalization

import (
	"bytes"

	"github.com/buger/jsonparser"
	"github.com/tidwall/sjson"

	"github.com/wundergraph/graphql-go-tools/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/pkg/astimport"
	"github.com/wundergraph/graphql-go-tools/pkg/astvisitor"
	"github.com/wundergraph/graphql-go-tools/pkg/internal/unsafebytes"
)

func extractVariablesDefaultValue(walker *astvisitor.Walker) *variablesDefaultValueExtractionVisitor {
	visitor := &variablesDefaultValueExtractionVisitor{
		Walker: walker,
	}
	walker.RegisterEnterDocumentVisitor(visitor)
	walker.RegisterOperationDefinitionVisitor(visitor)
	walker.RegisterEnterVariableDefinitionVisitor(visitor)
	walker.RegisterEnterFieldVisitor(visitor)
	return visitor
}

type variablesDefaultValueExtractionVisitor struct {
	*astvisitor.Walker
	operation, definition     *ast.Document
	importer                  astimport.Importer
	operationName             []byte
	operationRef              int
	skip                      bool
	nonNullableVariablesNames [][]byte
	extractedVariablesRefs    []int
}

func (v *variablesDefaultValueExtractionVisitor) EnterField(ref int) {
	if v.skip {
		return
	}

	// find field definition from document
	fieldName := v.operation.FieldNameBytes(ref)
	fieldDefRef, ok := v.definition.NodeFieldDefinitionByName(v.EnclosingTypeDefinition, fieldName)
	if !ok {
		return
	}

	// skip when field has no args in the document
	if !v.definition.FieldDefinitions[fieldDefRef].HasArgumentsDefinitions {
		return
	}

	for _, definitionInputValueDefRef := range v.definition.FieldDefinitions[fieldDefRef].ArgumentsDefinition.Refs {
		operationArgRef, exists := v.operation.FieldArgument(ref, v.definition.InputValueDefinitionNameBytes(definitionInputValueDefRef))

		if exists {
			operationArgValue := v.operation.ArgumentValue(operationArgRef)
			if v.operation.ValueContainsVariable(operationArgValue) {
				defTypeRef := v.definition.InputValueDefinitions[definitionInputValueDefRef].Type
				v.traverseValue(operationArgValue, defTypeRef)
			}
		} else {
			v.processDefaultFieldArguments(ref, definitionInputValueDefRef)
		}
	}
}

func (v *variablesDefaultValueExtractionVisitor) EnterVariableDefinition(ref int) {
	if v.skip {
		return
	}

	// skip when we have no default value for variable
	if !v.operation.VariableDefinitionHasDefaultValue(ref) {
		return
	}

	variableName := v.operation.VariableDefinitionNameString(ref)

	// remove variable DefaultValue from operation
	v.operation.VariableDefinitions[ref].DefaultValue.IsDefined = false

	// skip when variable was provided
	_, _, _, err := jsonparser.Get(v.operation.Input.Variables, variableName)
	if err == nil {
		return
	}

	// store extracted variable ref
	v.extractedVariablesRefs = append(v.extractedVariablesRefs, ref)

	valueBytes, err := v.operation.ValueToJSON(v.operation.VariableDefinitionDefaultValue(ref))
	if err != nil {
		return
	}

	v.operation.Input.Variables, err = sjson.SetRawBytes(v.operation.Input.Variables, variableName, valueBytes)
	if err != nil {
		v.StopWithInternalErr(err)
		return
	}
}

func (v *variablesDefaultValueExtractionVisitor) EnterOperationDefinition(ref int) {
	if len(v.operationName) == 0 {
		v.skip = false
		return
	}
	operationName := v.operation.OperationDefinitionNameBytes(ref)
	v.operationRef = ref
	v.skip = !bytes.Equal(operationName, v.operationName)

	v.nonNullableVariablesNames = make([][]byte, 0, len(v.operation.VariableDefinitions))
	v.extractedVariablesRefs = make([]int, 0, len(v.operation.VariableDefinitions))
}

func (v *variablesDefaultValueExtractionVisitor) LeaveOperationDefinition(_ int) {
	if v.skip {
		return
	}

	// find and make variable not null
	for j := 0; j < len(v.extractedVariablesRefs); j++ {
		variableDefRef := v.extractedVariablesRefs[j]

		if v.operation.Types[v.operation.VariableDefinitions[variableDefRef].Type].TypeKind == ast.TypeKindNonNull {
			// when variable is already not null, skip
			continue
		}

		for i := 0; i < len(v.nonNullableVariablesNames); i++ {
			if bytes.Equal(v.operation.VariableDefinitionNameBytes(variableDefRef), v.nonNullableVariablesNames[i]) {
				// if variable is nullable, make it not null as it satisfies both not null and nullable types
				// it is required to keep operation valid after variable extraction
				v.operation.VariableDefinitions[variableDefRef].Type = v.operation.AddNonNullType(v.operation.VariableDefinitions[variableDefRef].Type)
			}
		}
	}
}

func (v *variablesDefaultValueExtractionVisitor) traverseValue(value ast.Value, defTypeRef int) {
	switch value.Kind {
	case ast.ValueKindVariable:
		v.saveArgumentsWithTypeNotNull(value.Ref, defTypeRef)
	case ast.ValueKindList:
		for _, ref := range v.operation.ListValues[value.Ref].Refs {
			listValue := v.operation.Value(ref)
			if !v.operation.ValueContainsVariable(listValue) {
				continue
			}

			listTypeRef := defTypeRef
			// ommit not null to get to list itself
			if v.definition.Types[listTypeRef].TypeKind == ast.TypeKindNonNull {
				listTypeRef = v.definition.Types[listTypeRef].OfType
			}

			listItemType := v.definition.Types[listTypeRef].OfType
			v.traverseValue(listValue, listItemType)
		}
	case ast.ValueKindObject:
		for _, ref := range v.operation.ObjectValues[value.Ref].Refs {
			fieldName := v.operation.Input.ByteSlice(v.operation.ObjectFields[ref].Name)
			fieldValue := v.operation.ObjectFields[ref].Value

			typeName := v.definition.ResolveTypeNameString(defTypeRef)
			typeDefinitionNode, ok := v.definition.Index.FirstNodeByNameStr(typeName)
			if !ok {
				continue
			}
			objectFieldDefinitionRef, ok := v.definition.NodeInputFieldDefinitionByName(typeDefinitionNode, fieldName)
			if !ok {
				continue
			}

			if v.operation.ValueContainsVariable(fieldValue) {
				v.traverseValue(fieldValue, v.definition.InputValueDefinitions[objectFieldDefinitionRef].Type)
			}
		}
	}
}

func (v *variablesDefaultValueExtractionVisitor) saveArgumentsWithTypeNotNull(operationVariableValueRef, defTypeRef int) {
	if v.definition.Types[defTypeRef].TypeKind != ast.TypeKindNonNull {
		return
	}

	v.nonNullableVariablesNames = append(v.nonNullableVariablesNames, v.operation.VariableValueNameBytes(operationVariableValueRef))
}

func (v *variablesDefaultValueExtractionVisitor) processDefaultFieldArguments(operationFieldRef, definitionInputValueDefRef int) {
	if !v.definition.InputValueDefinitionHasDefaultValue(definitionInputValueDefRef) {
		return
	}

	variableNameBytes := v.operation.GenerateUnusedVariableDefinitionName(v.Ancestors[0].Ref)
	valueBytes, err := v.definition.ValueToJSON(v.definition.InputValueDefinitionDefaultValue(definitionInputValueDefRef))
	if err != nil {
		return
	}
	v.operation.Input.Variables, err = sjson.SetRawBytes(v.operation.Input.Variables, unsafebytes.BytesToString(variableNameBytes), valueBytes)
	if err != nil {
		v.StopWithInternalErr(err)
		return
	}

	variableValueRef, argRef := v.operation.ImportVariableValueArgument(v.definition.InputValueDefinitionNameBytes(definitionInputValueDefRef), variableNameBytes)
	defType := v.definition.InputValueDefinitions[definitionInputValueDefRef].Type
	importedDefType := v.importer.ImportType(defType, v.definition, v.operation)

	v.operation.AddArgumentToField(operationFieldRef, argRef)
	v.operation.AddVariableDefinitionToOperationDefinition(v.operationRef, variableValueRef, importedDefType)
}

func (v *variablesDefaultValueExtractionVisitor) EnterDocument(operation, definition *ast.Document) {
	v.operation, v.definition = operation, definition
}
