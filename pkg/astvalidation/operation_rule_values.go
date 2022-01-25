package astvalidation

import (
	"bytes"

	"github.com/wundergraph/graphql-go-tools/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/pkg/astimport"
	"github.com/wundergraph/graphql-go-tools/pkg/astvisitor"
	"github.com/wundergraph/graphql-go-tools/pkg/operationreport"
)

// Values validates if values are used properly
func Values() Rule {
	return func(walker *astvisitor.Walker) {
		visitor := valuesVisitor{
			Walker: walker,
		}
		walker.RegisterEnterDocumentVisitor(&visitor)
		walker.RegisterEnterArgumentVisitor(&visitor)
	}
}

type valuesVisitor struct {
	*astvisitor.Walker
	operation, definition *ast.Document
	importer              astimport.Importer
}

func (v *valuesVisitor) EnterDocument(operation, definition *ast.Document) {
	v.operation = operation
	v.definition = definition
}

func (v *valuesVisitor) EnterArgument(ref int) {

	definition, exists := v.ArgumentInputValueDefinition(ref)

	if !exists {
		return
	}

	value := v.operation.ArgumentValue(ref)
	if value.Kind == ast.ValueKindVariable {
		variableName := v.operation.VariableValueNameBytes(value.Ref)
		variableDefinition, exists := v.operation.VariableDefinitionByNameAndOperation(v.Ancestors[0].Ref, variableName)
		if !exists {
			operationName := v.operation.NodeNameBytes(v.Ancestors[0])
			v.StopWithExternalErr(operationreport.ErrVariableNotDefinedOnOperation(variableName, operationName))
			return
		}
		if !v.operation.VariableDefinitions[variableDefinition].DefaultValue.IsDefined {
			return // variable has no default value, deep type check not required
		}
		value = v.operation.VariableDefinitions[variableDefinition].DefaultValue.Value
	}

	v.valueSatisfiesInputValueDefinitionType(value, v.definition.InputValueDefinitions[definition].Type)
}

func (v *valuesVisitor) valueSatisfiesInputValueDefinitionType(value ast.Value, definitionTypeRef int) {
	switch v.definition.Types[definitionTypeRef].TypeKind {
	case ast.TypeKindNonNull:
		v.valuesSatisfiesNonNullType(value, definitionTypeRef)
	case ast.TypeKindNamed:
		v.valuesSatisfiesNamedType(value, definitionTypeRef)
	case ast.TypeKindList:
		v.valueSatisfiesListType(value, definitionTypeRef, v.definition.Types[definitionTypeRef].OfType)
	default:
		v.handleTypeError(value, definitionTypeRef)
	}
}

func (v *valuesVisitor) valuesSatisfiesNonNullType(value ast.Value, definitionTypeRef int) {
	switch value.Kind {
	case ast.ValueKindNull:
		v.handleTypeError(value, definitionTypeRef)
		return
	case ast.ValueKindVariable:
		variableTypeRef, _, ok := v.operationVariableType(value.Ref)
		if !ok {
			v.handleTypeError(value, definitionTypeRef)
			return
		}
		importedDefinitionType := v.importer.ImportType(definitionTypeRef, v.definition, v.operation)
		if !v.operation.TypesAreEqualDeep(importedDefinitionType, variableTypeRef) {
			v.handleTypeError(value, definitionTypeRef)
			return
		}
	}
	v.valueSatisfiesInputValueDefinitionType(value, v.definition.Types[definitionTypeRef].OfType)
}

func (v *valuesVisitor) valuesSatisfiesNamedType(value ast.Value, definitionTypeRef int) {
	if value.Kind == ast.ValueKindNull {
		// null always satisfies not required type
		return
	}

	typeName := v.definition.ResolveTypeNameBytes(definitionTypeRef)
	node, exists := v.definition.Index.FirstNodeByNameBytes(typeName)
	if !exists {
		v.handleTypeError(value, definitionTypeRef)
		return
	}
	if !v.valueSatisfiesTypeDefinitionNode(value, node) {
		v.handleTypeError(value, definitionTypeRef)
	}
}

func (v *valuesVisitor) valueSatisfiesListType(value ast.Value, definitionTypeRef int, listItemType int) {

	if value.Kind == ast.ValueKindVariable {
		actualType, _, ok := v.operationVariableType(value.Ref)
		if !ok {
			v.handleTypeError(value, definitionTypeRef)
			return
		}
		expectedType := v.importer.ImportType(listItemType, v.definition, v.operation)
		if v.operation.Types[actualType].TypeKind == ast.TypeKindNonNull {
			actualType = v.operation.Types[actualType].OfType
		}
		if v.operation.Types[actualType].TypeKind == ast.TypeKindList {
			actualType = v.operation.Types[actualType].OfType
		}
		if !v.operation.TypesAreEqualDeep(expectedType, actualType) {
			v.handleTypeError(value, definitionTypeRef)
		}
	}

	if value.Kind != ast.ValueKindList {
		v.handleTypeError(value, definitionTypeRef)
		return
	}

	if v.definition.Types[listItemType].TypeKind == ast.TypeKindNonNull {
		if len(v.operation.ListValues[value.Ref].Refs) == 0 {
			v.handleTypeError(value, definitionTypeRef)
			return
		}
		listItemType = v.definition.Types[listItemType].OfType
	}

	for _, i := range v.operation.ListValues[value.Ref].Refs {
		listValue := v.operation.Value(i)
		v.valueSatisfiesInputValueDefinitionType(listValue, listItemType)
	}
}

func (v *valuesVisitor) valueSatisfiesTypeDefinitionNode(value ast.Value, node ast.Node) bool {
	switch node.Kind {
	case ast.NodeKindEnumTypeDefinition:
		return v.valueSatisfiesEnum(value, node)
	case ast.NodeKindScalarTypeDefinition:
		return v.valueSatisfiesScalar(value, node.Ref)
	case ast.NodeKindInputObjectTypeDefinition:
		return v.valueSatisfiesInputObjectTypeDefinition(value, node.Ref)
	default:
		return false
	}
}

func (v *valuesVisitor) valueSatisfiesEnum(value ast.Value, node ast.Node) bool {
	if value.Kind == ast.ValueKindVariable {
		_, actualTypeName, ok := v.operationVariableType(value.Ref)
		if !ok {
			return false
		}
		expectedTypeName := node.NameBytes(v.definition)
		return bytes.Equal(actualTypeName, expectedTypeName)
	}
	if value.Kind != ast.ValueKindEnum {
		return false
	}
	enumValue := v.operation.EnumValueNameBytes(value.Ref)
	return v.definition.EnumTypeDefinitionContainsEnumValue(node.Ref, enumValue)
}

func (v *valuesVisitor) valueSatisfiesScalar(value ast.Value, scalar int) bool {
	scalarName := v.definition.ScalarTypeDefinitionNameBytes(scalar)
	if value.Kind == ast.ValueKindVariable {
		_, typeName, ok := v.operationVariableType(value.Ref)
		if !ok {
			return false
		}
		return bytes.Equal(scalarName, typeName)
	}
	switch string(scalarName) {
	case "ID":
		return value.Kind == ast.ValueKindString || value.Kind == ast.ValueKindInteger
	case "Boolean":
		return value.Kind == ast.ValueKindBoolean
	case "Int":
		return value.Kind == ast.ValueKindInteger
	case "Float":
		return value.Kind == ast.ValueKindFloat || value.Kind == ast.ValueKindInteger
	default:
		return value.Kind == ast.ValueKindString
	}
}

func (v *valuesVisitor) valueSatisfiesInputObjectTypeDefinition(value ast.Value, inputObjectTypeDefinition int) bool {

	if value.Kind == ast.ValueKindVariable {
		_, actualTypeName, ok := v.operationVariableType(value.Ref)
		if !ok {
			return false
		}

		expectedTypeName := v.definition.InputObjectTypeDefinitionNameBytes(inputObjectTypeDefinition)
		return bytes.Equal(actualTypeName, expectedTypeName)
	}

	if value.Kind != ast.ValueKindObject {
		return false
	}

	for _, i := range v.definition.InputObjectTypeDefinitions[inputObjectTypeDefinition].InputFieldsDefinition.Refs {
		if !v.objectValueSatisfiesInputValueDefinition(value.Ref, i) {
			return false
		}
	}

	for _, i := range v.operation.ObjectValues[value.Ref].Refs {
		if !v.objectFieldDefined(i, inputObjectTypeDefinition) {
			objectFieldName := string(v.operation.ObjectFieldNameBytes(i))
			def := string(v.definition.Input.ByteSlice(v.definition.InputObjectTypeDefinitions[inputObjectTypeDefinition].Name))
			_, _ = objectFieldName, def
			return false
		}
	}

	return !v.objectValueHasDuplicateFields(value.Ref)
}

func (v *valuesVisitor) objectValueHasDuplicateFields(objectValue int) bool {
	for i, j := range v.operation.ObjectValues[objectValue].Refs {
		for k, l := range v.operation.ObjectValues[objectValue].Refs {
			if i == k || i > k {
				continue
			}
			if bytes.Equal(v.operation.ObjectFieldNameBytes(j), v.operation.ObjectFieldNameBytes(l)) {
				return true
			}
		}
	}
	return false
}

func (v *valuesVisitor) objectFieldDefined(objectField, inputObjectTypeDefinition int) bool {
	name := v.operation.ObjectFieldNameBytes(objectField)
	for _, i := range v.definition.InputObjectTypeDefinitions[inputObjectTypeDefinition].InputFieldsDefinition.Refs {
		if bytes.Equal(name, v.definition.InputValueDefinitionNameBytes(i)) {
			return true
		}
	}
	return false
}

func (v *valuesVisitor) objectValueSatisfiesInputValueDefinition(objectValue, inputValueDefinition int) bool {

	name := v.definition.InputValueDefinitionNameBytes(inputValueDefinition)
	definitionType := v.definition.InputValueDefinitionType(inputValueDefinition)

	for _, i := range v.operation.ObjectValues[objectValue].Refs {
		if bytes.Equal(name, v.operation.ObjectFieldNameBytes(i)) {
			value := v.operation.ObjectFieldValue(i)
			v.valueSatisfiesInputValueDefinitionType(value, definitionType)
		}
	}

	// argument is not present on object value, if arg is optional it's still ok, otherwise not satisfied
	return v.definition.InputValueDefinitionArgumentIsOptional(inputValueDefinition)
}

func (v *valuesVisitor) operationVariableType(valueRef int) (variableTypeRef int, typeName ast.ByteSlice, ok bool) {
	variableName := v.operation.VariableValueNameBytes(valueRef)
	variableDefinition, exists := v.operation.VariableDefinitionByNameAndOperation(v.Ancestors[0].Ref, variableName)
	if !exists {
		return ast.InvalidRef, nil, false
	}
	variableTypeRef = v.operation.VariableDefinitions[variableDefinition].Type
	typeName = v.operation.ResolveTypeNameBytes(variableTypeRef)

	return variableTypeRef, typeName, true
}

func (v *valuesVisitor) handleTypeError(value ast.Value, definitionTypeRef int) {
	printedValue, err := v.operation.PrintValueBytes(value, nil)
	if v.HandleInternalErr(err) {
		return
	}

	underlyingType := v.definition.ResolveUnderlyingType(definitionTypeRef)
	printedType, err := v.definition.PrintTypeBytes(underlyingType, nil)
	if v.HandleInternalErr(err) {
		return
	}

	v.Report.AddExternalError(operationreport.ErrValueDoesntSatisfyInputValueDefinition(printedValue, printedType, value.Position))
}
