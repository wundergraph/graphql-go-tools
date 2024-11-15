package astnormalization

import (
	"bytes"

	"github.com/buger/jsonparser"
	"github.com/tidwall/sjson"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astimport"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astvisitor"
)

func extractVariablesDefaultValue(walker *astvisitor.Walker) *variablesDefaultValueExtractionVisitor {
	visitor := &variablesDefaultValueExtractionVisitor{
		Walker: walker,
	}
	walker.RegisterEnterDocumentVisitor(visitor)
	walker.RegisterOperationDefinitionVisitor(visitor)
	walker.RegisterEnterVariableDefinitionVisitor(visitor)
	walker.RegisterEnterFieldVisitor(visitor)
	walker.RegisterEnterArgumentVisitor(visitor)

	return visitor
}

type variablesDefaultValueExtractionVisitor struct {
	*astvisitor.Walker
	operation, definition                             *ast.Document
	importer                                          astimport.Importer
	operationRef                                      int
	variablesNamesUsedInPositionsExpectingNonNullType [][]byte
	variableRefsWithDefaultValuesDefined              []int
}

func (v *variablesDefaultValueExtractionVisitor) EnterArgument(ref int) {
	// variable could be used in directive argument which requires non-null type
	// in case such variable has default value and not of non-null type, we need to make it non-null

	if len(v.Ancestors) == 0 || v.Ancestors[0].Kind != ast.NodeKindOperationDefinition {
		return
	}
	if v.Ancestors[len(v.Ancestors)-1].Kind != ast.NodeKindDirective {
		return // skip non directives
	}

	containsVariable := v.operation.ValueContainsVariable(v.operation.Arguments[ref].Value)
	if !containsVariable {
		return
	}

	inputValueDefinition, ok := v.Walker.ArgumentInputValueDefinition(ref)
	if !ok {
		return
	}
	v.traverseValue(v.operation.Arguments[ref].Value, v.definition.InputValueDefinitions[inputValueDefinition].Type)
}

func (v *variablesDefaultValueExtractionVisitor) EnterField(ref int) {
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
		}
	}
}

func (v *variablesDefaultValueExtractionVisitor) EnterVariableDefinition(ref int) {
	// skip when we have no default value for variable
	if !v.operation.VariableDefinitionHasDefaultValue(ref) {
		return
	}

	variableName := v.operation.VariableDefinitionNameString(ref)

	// remove variable DefaultValue from operation
	v.operation.VariableDefinitions[ref].DefaultValue.IsDefined = false

	// store variable ref
	v.variableRefsWithDefaultValuesDefined = append(v.variableRefsWithDefaultValuesDefined, ref)

	// skip extracting variable default value when variable value is provided
	_, _, _, err := jsonparser.Get(v.operation.Input.Variables, variableName)
	if err == nil {
		return
	}

	valueBytes, err := v.operation.ValueToJSON(v.operation.VariableDefinitionDefaultValue(ref))
	if err != nil {
		return
	}

	isListVariable := v.operation.TypeIsList(v.operation.VariableDefinitions[ref].Type)
	if isListVariable && len(valueBytes) > 0 && valueBytes[0] != '[' {
		listWraps := v.operation.TypeNumberOfListWraps(v.operation.VariableDefinitions[ref].Type)
		for i := 0; i < listWraps; i++ {
			valueBytes = append([]byte{'['}, append(valueBytes, ']')...)
		}
	}

	v.operation.Input.Variables, err = sjson.SetRawBytes(v.operation.Input.Variables, variableName, valueBytes)
	if err != nil {
		v.StopWithInternalErr(err)
		return
	}
}

func (v *variablesDefaultValueExtractionVisitor) EnterOperationDefinition(ref int) {
	v.operationRef = ref
}

func (v *variablesDefaultValueExtractionVisitor) LeaveOperationDefinition(ref int) {
	// find and make variable not null
	for j := 0; j < len(v.variableRefsWithDefaultValuesDefined); j++ {
		variableDefRef := v.variableRefsWithDefaultValuesDefined[j]

		if v.operation.TypeIsNonNull(v.operation.VariableDefinitions[variableDefRef].Type) {
			// when variable is already not null, skip
			continue
		}

		for i := 0; i < len(v.variablesNamesUsedInPositionsExpectingNonNullType); i++ {
			if !bytes.Equal(v.operation.VariableDefinitionNameBytes(variableDefRef), v.variablesNamesUsedInPositionsExpectingNonNullType[i]) {
				continue
			}

			if v.operation.TypeIsNonNull(v.operation.VariableDefinitions[variableDefRef].Type) {
				// when variable is already not null, skip
				// second check is required because we could use variable in a few different places
				// so on next places we should not do anything if variable is already not null
				continue
			}

			// if variable is nullable, make it not null as it satisfies both not null and nullable types
			// it is required to keep operation valid after variable extraction
			v.operation.VariableDefinitions[variableDefRef].Type = v.operation.AddNonNullType(v.operation.VariableDefinitions[variableDefRef].Type)
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
			// omit not null to get to list itself
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

	v.variablesNamesUsedInPositionsExpectingNonNullType = append(v.variablesNamesUsedInPositionsExpectingNonNullType, v.operation.VariableValueNameBytes(operationVariableValueRef))
}

func (v *variablesDefaultValueExtractionVisitor) EnterDocument(operation, definition *ast.Document) {
	v.operation, v.definition = operation, definition
	v.variablesNamesUsedInPositionsExpectingNonNullType = make([][]byte, 0, len(v.operation.VariableDefinitions))
	v.variableRefsWithDefaultValuesDefined = make([]int, 0, len(v.operation.VariableDefinitions))
}
