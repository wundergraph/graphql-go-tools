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

func (v *valuesVisitor) valueSatisfiesInputValueDefinitionType(value ast.Value, definitionTypeRef int) bool {
	switch v.definition.Types[definitionTypeRef].TypeKind {
	case ast.TypeKindNonNull:
		return v.valuesSatisfiesNonNullType(value, definitionTypeRef)
	case ast.TypeKindNamed:
		return v.valuesSatisfiesNamedType(value, definitionTypeRef)
	case ast.TypeKindList:
		return v.valueSatisfiesListType(value, definitionTypeRef, v.definition.Types[definitionTypeRef].OfType)
	default:
		v.handleTypeError(value, definitionTypeRef)
		return false
	}
}

func (v *valuesVisitor) valuesSatisfiesNonNullType(value ast.Value, definitionTypeRef int) bool {
	switch value.Kind {
	case ast.ValueKindNull:
		v.handleUnexpectedNullError(value, definitionTypeRef)
		return false
	case ast.ValueKindVariable:
		variableTypeRef, _, ok := v.operationVariableType(value.Ref)
		if !ok {
			v.handleTypeError(value, definitionTypeRef)
			return false
		}
		importedDefinitionType := v.importer.ImportType(definitionTypeRef, v.definition, v.operation)
		if !v.operation.TypesAreEqualDeep(importedDefinitionType, variableTypeRef) {
			v.handleTypeError(value, definitionTypeRef)
			return false
		}
	}
	return v.valueSatisfiesInputValueDefinitionType(value, v.definition.Types[definitionTypeRef].OfType)
}

func (v *valuesVisitor) valuesSatisfiesNamedType(value ast.Value, definitionTypeRef int) bool {
	if value.Kind == ast.ValueKindNull {
		// null always satisfies not required type
		return true
	}

	typeName := v.definition.ResolveTypeNameBytes(definitionTypeRef)
	node, exists := v.definition.Index.FirstNodeByNameBytes(typeName)
	if !exists {
		v.handleTypeError(value, definitionTypeRef)
		return false
	}

	return v.valueSatisfiesTypeDefinitionNode(value, definitionTypeRef, node)
}

func (v *valuesVisitor) valueSatisfiesListType(value ast.Value, definitionTypeRef int, listItemType int) bool {

	if value.Kind == ast.ValueKindVariable {
		actualType, _, ok := v.operationVariableType(value.Ref)
		if !ok {
			v.handleTypeError(value, definitionTypeRef)
			return false
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
			return false
		}
	}

	if value.Kind == ast.ValueKindNull {
		return true
	}

	if value.Kind != ast.ValueKindList {
		return v.valueSatisfiesInputValueDefinitionType(value, listItemType)
	}

	if v.definition.Types[listItemType].TypeKind == ast.TypeKindNonNull {
		if len(v.operation.ListValues[value.Ref].Refs) == 0 {
			v.handleTypeError(value, definitionTypeRef)
			return false
		}
		listItemType = v.definition.Types[listItemType].OfType
	}

	valid := true

	for _, i := range v.operation.ListValues[value.Ref].Refs {
		listValue := v.operation.Value(i)
		if !v.valueSatisfiesInputValueDefinitionType(listValue, listItemType) {
			valid = false
		}
	}

	return valid
}

func (v *valuesVisitor) valueSatisfiesTypeDefinitionNode(value ast.Value, definitionTypeRef int, node ast.Node) bool {
	switch node.Kind {
	case ast.NodeKindEnumTypeDefinition:
		return v.valueSatisfiesEnum(value, definitionTypeRef, node)
	case ast.NodeKindScalarTypeDefinition:
		if !v.valueSatisfiesScalar(value, node.Ref) {
			v.handleTypeError(value, definitionTypeRef)
			return false
		}
	case ast.NodeKindInputObjectTypeDefinition:
		return v.valueSatisfiesInputObjectTypeDefinition(value, definitionTypeRef, node.Ref)
	}
	return false
}

func (v *valuesVisitor) valueSatisfiesEnum(value ast.Value, definitionTypeRef int, node ast.Node) bool {
	if value.Kind == ast.ValueKindVariable {
		_, actualTypeName, ok := v.operationVariableType(value.Ref)
		if !ok {
			v.handleUnexpectedEnumValueError(value, definitionTypeRef)
			return false
		}
		expectedTypeName := node.NameBytes(v.definition)

		if !bytes.Equal(actualTypeName, expectedTypeName) {
			v.handleUnexpectedEnumValueError(value, definitionTypeRef)
			return false
		}
	}
	if value.Kind != ast.ValueKindEnum {
		v.handleUnexpectedEnumValueError(value, definitionTypeRef)
		return false
	}
	enumValue := v.operation.EnumValueNameBytes(value.Ref)

	if !v.definition.EnumTypeDefinitionContainsEnumValue(node.Ref, enumValue) {
		v.handleNotExistingEnumValueError(value, definitionTypeRef)
		return false
	}

	return true
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

func (v *valuesVisitor) valueSatisfiesInputObjectTypeDefinition(value ast.Value, definitionTypeRef int, inputObjectTypeDefinition int) bool {

	if value.Kind == ast.ValueKindVariable {
		_, actualTypeName, ok := v.operationVariableType(value.Ref)
		if !ok {
			v.handleTypeError(value, definitionTypeRef)
			return false
		}

		expectedTypeName := v.definition.InputObjectTypeDefinitionNameBytes(inputObjectTypeDefinition)
		if !bytes.Equal(actualTypeName, expectedTypeName) {
			v.handleTypeError(value, definitionTypeRef)
			return false
		}
	}

	if value.Kind != ast.ValueKindObject {
		v.handleTypeError(value, definitionTypeRef)
		return false
	}

	valid := true

	for _, i := range v.definition.InputObjectTypeDefinitions[inputObjectTypeDefinition].InputFieldsDefinition.Refs {
		if !v.objectValueSatisfiesInputValueDefinition(value, inputObjectTypeDefinition, i) {
			valid = false
		}
	}

	if !valid {
		return false
	}

	for _, i := range v.operation.ObjectValues[value.Ref].Refs {
		if !v.objectFieldDefined(i, inputObjectTypeDefinition) {
			objectFieldName := string(v.operation.ObjectFieldNameBytes(i))
			def := string(v.definition.Input.ByteSlice(v.definition.InputObjectTypeDefinitions[inputObjectTypeDefinition].Name))
			_, _ = objectFieldName, def
			v.handleTypeError(value, definitionTypeRef)
			valid = false
		}
	}

	if !valid {
		return false
	}

	if v.objectValueHasDuplicateFields(value.Ref) {
		v.handleTypeError(value, definitionTypeRef)
		return false
	}

	return true
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

func (v *valuesVisitor) objectValueSatisfiesInputValueDefinition(objectValue ast.Value, inputObjectDefinition, inputValueDefinition int) bool {

	name := v.definition.InputValueDefinitionNameBytes(inputValueDefinition)
	definitionTypeRef := v.definition.InputValueDefinitionType(inputValueDefinition)

	for _, i := range v.operation.ObjectValues[objectValue.Ref].Refs {
		if bytes.Equal(name, v.operation.ObjectFieldNameBytes(i)) {
			value := v.operation.ObjectFieldValue(i)
			return v.valueSatisfiesInputValueDefinitionType(value, definitionTypeRef)
		}
	}

	// argument is not present on object value, if arg is optional it's still ok, otherwise not satisfied
	if !v.definition.InputValueDefinitionArgumentIsOptional(inputValueDefinition) {
		v.handleMissingRequiredFieldOfInputObjectError(objectValue, name, inputObjectDefinition, inputValueDefinition)
		return false
	}

	return true
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

func (v *valuesVisitor) handleUnexpectedNullError(value ast.Value, definitionTypeRef int) {
	printedType, err := v.definition.PrintTypeBytes(definitionTypeRef, nil)
	if v.HandleInternalErr(err) {
		return
	}

	v.Report.AddExternalError(operationreport.ErrNullValueDoesntSatisfyInputValueDefinition(printedType, value.Position))
}

func (v *valuesVisitor) handleUnexpectedEnumValueError(value ast.Value, definitionTypeRef int) {
	printedValue, err := v.operation.PrintValueBytes(value, nil)
	if v.HandleInternalErr(err) {
		return
	}

	underlyingType := v.definition.ResolveUnderlyingType(definitionTypeRef)
	printedType, err := v.definition.PrintTypeBytes(underlyingType, nil)
	if v.HandleInternalErr(err) {
		return
	}

	v.Report.AddExternalError(operationreport.ErrValueDoesntSatisfyEnum(printedValue, printedType, value.Position))
}

func (v *valuesVisitor) handleNotExistingEnumValueError(value ast.Value, definitionTypeRef int) {
	printedValue, err := v.operation.PrintValueBytes(value, nil)
	if v.HandleInternalErr(err) {
		return
	}

	underlyingType := v.definition.ResolveUnderlyingType(definitionTypeRef)
	printedType, err := v.definition.PrintTypeBytes(underlyingType, nil)
	if v.HandleInternalErr(err) {
		return
	}

	v.Report.AddExternalError(operationreport.ErrValueDoesntExistsInEnum(printedValue, printedType, value.Position))
}

func (v *valuesVisitor) handleMissingRequiredFieldOfInputObjectError(value ast.Value, fieldName ast.ByteSlice, inputObjectDefinition, inputValueDefinition int) {
	printedType, err := v.definition.PrintTypeBytes(v.definition.InputValueDefinitions[inputValueDefinition].Type, nil)
	if v.HandleInternalErr(err) {
		return
	}

	v.Report.AddExternalError(operationreport.ErrMissingRequiredFieldOfInputObject(
		v.definition.InputObjectTypeDefinitionNameBytes(inputObjectDefinition),
		fieldName,
		printedType,
		value.Position,
	))
}
