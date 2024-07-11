package astvalidation

import (
	"bytes"

	"github.com/wundergraph/graphql-go-tools/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/pkg/astimport"
	"github.com/wundergraph/graphql-go-tools/pkg/astvisitor"
	"github.com/wundergraph/graphql-go-tools/pkg/lexer/literal"
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
		walker.RegisterEnterVariableDefinitionVisitor(&visitor)
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

func (v *valuesVisitor) EnterVariableDefinition(ref int) {
	if !v.operation.VariableDefinitionHasDefaultValue(ref) {
		return // variable has no default value, deep type check not required
	}

	v.valueSatisfiesOperationType(v.operation.VariableDefinitions[ref].DefaultValue.Value, v.operation.VariableDefinitions[ref].Type)
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
			operationName := v.operation.OperationDefinitionNameBytes(v.Ancestors[0].Ref)
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

func (v *valuesVisitor) valueSatisfiesOperationType(value ast.Value, operationTypeRef int) bool {
	switch v.operation.Types[operationTypeRef].TypeKind {
	case ast.TypeKindNonNull:
		return v.valuesSatisfiesOperationNonNullType(value, operationTypeRef)
	case ast.TypeKindNamed:
		return v.valuesSatisfiesOperationNamedType(value, operationTypeRef)
	case ast.TypeKindList:
		return v.valueSatisfiesOperationListType(value, operationTypeRef, v.operation.Types[operationTypeRef].OfType)
	default:
		v.handleOperationTypeError(value, operationTypeRef)
		return false
	}
}

func (v *valuesVisitor) valuesSatisfiesOperationNonNullType(value ast.Value, operationTypeRef int) bool {
	if value.Kind == ast.ValueKindNull {
		v.handleOperationUnexpectedNullError(value, operationTypeRef)
		return false
	}
	return v.valueSatisfiesOperationType(value, v.operation.Types[operationTypeRef].OfType)
}

func (v *valuesVisitor) valuesSatisfiesOperationNamedType(value ast.Value, operationTypeRef int) bool {
	if value.Kind == ast.ValueKindNull {
		// null always satisfies not required type
		return true
	}

	typeName := v.operation.ResolveTypeNameBytes(operationTypeRef)
	node, exists := v.definition.Index.FirstNodeByNameBytes(typeName)
	if !exists {
		v.handleOperationTypeError(value, operationTypeRef)
		return false
	}

	definitionTypeRef := ast.InvalidRef

	for ref := 0; ref < len(v.definition.Types); ref++ {
		if v.definition.Types[ref].TypeKind != ast.TypeKindNamed {
			continue
		}

		if bytes.Equal(v.definition.TypeNameBytes(ref), typeName) {
			definitionTypeRef = ref
			break
		}
	}

	if definitionTypeRef == ast.InvalidRef {
		// should not happen, as in case we have not found named type node we will report it earlier
		return false
	}

	return v.valueSatisfiesTypeDefinitionNode(value, definitionTypeRef, node)
}

func (v *valuesVisitor) valueSatisfiesOperationListType(value ast.Value, operationTypeRef int, listItemType int) bool {
	if value.Kind == ast.ValueKindNull {
		return true
	}

	if value.Kind != ast.ValueKindList {
		return v.valueSatisfiesOperationType(value, listItemType)
	}

	if v.operation.Types[listItemType].TypeKind == ast.TypeKindNonNull {
		if len(v.operation.ListValues[value.Ref].Refs) == 0 {
			v.handleOperationTypeError(value, operationTypeRef)
			return false
		}
		listItemType = v.operation.Types[listItemType].OfType
	}

	valid := true

	for _, i := range v.operation.ListValues[value.Ref].Refs {
		listValue := v.operation.Value(i)
		if !v.valueSatisfiesOperationType(listValue, listItemType) {
			valid = false
		}
	}

	return valid
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
		variableDefinitionRef, variableTypeRef, _, ok := v.operationVariableType(value.Ref)
		if !ok {
			v.handleTypeError(value, definitionTypeRef)
			return false
		}

		if v.operation.VariableDefinitionHasDefaultValue(variableDefinitionRef) {
			return v.valueSatisfiesInputValueDefinitionType(v.operation.VariableDefinitions[variableDefinitionRef].DefaultValue.Value, definitionTypeRef)
		}

		importedDefinitionType := v.importer.ImportType(definitionTypeRef, v.definition, v.operation)
		if !v.operation.TypesAreEqualDeep(importedDefinitionType, variableTypeRef) {
			v.handleVariableHasIncompatibleTypeError(value, definitionTypeRef)
			return false
		}
		return true
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
		variableDefinitionRef, actualType, _, ok := v.operationVariableType(value.Ref)
		if !ok {
			v.handleTypeError(value, definitionTypeRef)
			return false
		}

		if v.operation.VariableDefinitionHasDefaultValue(variableDefinitionRef) {
			return v.valueSatisfiesInputValueDefinitionType(v.operation.VariableDefinitions[variableDefinitionRef].DefaultValue.Value, definitionTypeRef)
		}

		expectedType := v.importer.ImportType(listItemType, v.definition, v.operation)
		if v.operation.Types[actualType].TypeKind == ast.TypeKindNonNull {
			actualType = v.operation.Types[actualType].OfType
		}
		if v.operation.Types[actualType].TypeKind == ast.TypeKindList {
			actualType = v.operation.Types[actualType].OfType
		}
		if !v.operation.TypesAreEqualDeep(expectedType, actualType) {
			v.handleVariableHasIncompatibleTypeError(value, definitionTypeRef)
			return false
		}
		return true
	}

	if value.Kind == ast.ValueKindNull {
		return true
	}

	if value.Kind != ast.ValueKindList {
		return v.valueSatisfiesInputValueDefinitionType(value, listItemType)
	}

	if v.definition.Types[listItemType].TypeKind == ast.TypeKindNonNull {
		if len(v.operation.ListValues[value.Ref].Refs) == 0 {
			// [] empty list is a valid input for [item!] lists
			return true
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
		return v.valueSatisfiesScalar(value, definitionTypeRef, node.Ref)
	case ast.NodeKindInputObjectTypeDefinition:
		return v.valueSatisfiesInputObjectTypeDefinition(value, definitionTypeRef, node.Ref)
	}
	return false
}

func (v *valuesVisitor) valueSatisfiesEnum(value ast.Value, definitionTypeRef int, node ast.Node) bool {
	if value.Kind == ast.ValueKindVariable {
		expectedTypeName := node.NameBytes(v.definition)
		return v.variableValueHasMatchingTypeName(value, definitionTypeRef, expectedTypeName)
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

func (v *valuesVisitor) valueSatisfiesScalar(value ast.Value, definitionTypeRef int, scalar int) bool {
	scalarName := v.definition.ScalarTypeDefinitionNameBytes(scalar)

	if value.Kind == ast.ValueKindVariable {
		return v.variableValueHasMatchingTypeName(value, definitionTypeRef, scalarName)
	}

	switch {
	case bytes.Equal(scalarName, literal.ID):
		return v.valueSatisfiesScalarID(value, definitionTypeRef)
	case bytes.Equal(scalarName, literal.BOOLEAN):
		return v.valueSatisfiesScalarBoolean(value, definitionTypeRef)
	case bytes.Equal(scalarName, literal.INT):
		return v.valueSatisfiesScalarInt(value, definitionTypeRef)
	case bytes.Equal(scalarName, literal.FLOAT):
		return v.valueSatisfiesScalarFloat(value, definitionTypeRef)
	case bytes.Equal(scalarName, literal.STRING):
		return v.valueSatisfiesScalarString(value, definitionTypeRef, true)
	default:
		return v.valueSatisfiesScalarString(value, definitionTypeRef, false)
	}
}

func (v *valuesVisitor) valueSatisfiesScalarID(value ast.Value, definitionTypeRef int) bool {
	if value.Kind == ast.ValueKindString || value.Kind == ast.ValueKindInteger {
		return true
	}

	printedValue, printedType, ok := v.printValueAndUnderlyingType(value, definitionTypeRef)
	if !ok {
		return false
	}

	v.Report.AddExternalError(operationreport.ErrValueDoesntSatisfyID(printedValue, printedType, value.Position))

	return false
}

func (v *valuesVisitor) valueSatisfiesScalarBoolean(value ast.Value, definitionTypeRef int) bool {
	if value.Kind == ast.ValueKindBoolean {
		return true
	}

	printedValue, printedType, ok := v.printValueAndUnderlyingType(value, definitionTypeRef)
	if !ok {
		return false
	}

	v.Report.AddExternalError(operationreport.ErrValueDoesntSatisfyBoolean(printedValue, printedType, value.Position))

	return false
}

func (v *valuesVisitor) valueSatisfiesScalarInt(value ast.Value, definitionTypeRef int) bool {
	var isValidInt32 bool
	isInt := value.Kind == ast.ValueKindInteger

	if isInt {
		isValidInt32 = v.operation.IntValueValidInt32(value.Ref)
	}

	if isInt && isValidInt32 {
		return true
	}

	printedValue, printedType, ok := v.printValueAndUnderlyingType(value, definitionTypeRef)
	if !ok {
		return false
	}

	if !isInt {
		v.Report.AddExternalError(operationreport.ErrValueDoesntSatisfyInt(printedValue, printedType, value.Position))
		return false
	}

	v.Report.AddExternalError(operationreport.ErrBigIntValueDoesntSatisfyInt(printedValue, printedType, value.Position))
	return false
}

func (v *valuesVisitor) valueSatisfiesScalarFloat(value ast.Value, definitionTypeRef int) bool {
	if value.Kind == ast.ValueKindFloat || value.Kind == ast.ValueKindInteger {
		return true
	}

	printedValue, printedType, ok := v.printValueAndUnderlyingType(value, definitionTypeRef)
	if !ok {
		return false
	}

	v.Report.AddExternalError(operationreport.ErrValueDoesntSatisfyFloat(printedValue, printedType, value.Position))

	return false
}

func (v *valuesVisitor) valueSatisfiesScalarString(value ast.Value, definitionTypeRef int, builtInStringScalar bool) bool {
	if value.Kind == ast.ValueKindString {
		return true
	}

	printedValue, printedType, ok := v.printValueAndUnderlyingType(value, definitionTypeRef)
	if !ok {
		return false
	}

	if builtInStringScalar {
		v.Report.AddExternalError(operationreport.ErrValueDoesntSatisfyString(printedValue, printedType, value.Position))
	} else {
		v.Report.AddExternalError(operationreport.ErrValueDoesntSatisfyType(printedValue, printedType, value.Position))
	}

	return false
}

func (v *valuesVisitor) valueSatisfiesInputObjectTypeDefinition(value ast.Value, definitionTypeRef int, inputObjectTypeDefinition int) bool {
	if value.Kind == ast.ValueKindVariable {
		expectedTypeName := v.definition.InputObjectTypeDefinitionNameBytes(inputObjectTypeDefinition)
		return v.variableValueHasMatchingTypeName(value, definitionTypeRef, expectedTypeName)
	}

	if value.Kind != ast.ValueKindObject {
		v.handleNotObjectTypeError(value, definitionTypeRef)
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
			objectFieldName := v.operation.ObjectFieldNameBytes(i)
			def := v.definition.Input.ByteSlice(v.definition.InputObjectTypeDefinitions[inputObjectTypeDefinition].Name)

			v.Report.AddExternalError(operationreport.ErrUnknownFieldOfInputObject(objectFieldName, def, v.operation.ObjectField(i).Position))
			valid = false
		}
	}

	if !valid {
		return false
	}

	if v.objectValueHasDuplicateFields(value.Ref) {
		return false
	}

	return true
}

func (v *valuesVisitor) objectValueHasDuplicateFields(objectValue int) bool {
	hasDuplicates := false

	reportedFieldRefs := make(map[int]struct{})
	for i, j := range v.operation.ObjectValues[objectValue].Refs {
		for k, l := range v.operation.ObjectValues[objectValue].Refs {
			if i == k || i > k {
				continue
			}

			if _, ok := reportedFieldRefs[l]; ok {
				continue
			}

			fieldName := v.operation.ObjectFieldNameBytes(j)
			otherFieldName := v.operation.ObjectFieldNameBytes(l)

			if bytes.Equal(fieldName, otherFieldName) {
				v.Report.AddExternalError(operationreport.ErrDuplicatedFieldInputObject(
					fieldName,
					v.operation.ObjectField(j).Position,
					v.operation.ObjectField(l).Position))
				hasDuplicates = true
				reportedFieldRefs[l] = struct{}{}
			}
		}
	}

	return hasDuplicates
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

func (v *valuesVisitor) variableValueHasMatchingTypeName(value ast.Value, definitionTypeRef int, expectedTypeName []byte) bool {
	variableDefinitionRef, _, actualTypeName, ok := v.operationVariableType(value.Ref)
	if !ok {
		v.handleVariableHasIncompatibleTypeError(value, definitionTypeRef)
		return false
	}

	if v.operation.VariableDefinitionHasDefaultValue(variableDefinitionRef) {
		return v.valueSatisfiesInputValueDefinitionType(v.operation.VariableDefinitions[variableDefinitionRef].DefaultValue.Value, definitionTypeRef)
	}

	if !bytes.Equal(actualTypeName, expectedTypeName) {
		v.handleVariableHasIncompatibleTypeError(value, definitionTypeRef)
		return false
	}

	return true
}

func (v *valuesVisitor) handleTypeError(value ast.Value, definitionTypeRef int) {
	printedValue, printedType, ok := v.printValueAndUnderlyingType(value, definitionTypeRef)
	if !ok {
		return
	}

	v.Report.AddExternalError(operationreport.ErrValueDoesntSatisfyType(printedValue, printedType, value.Position))
}

func (v *valuesVisitor) handleNotObjectTypeError(value ast.Value, definitionTypeRef int) {
	printedValue, printedType, ok := v.printValueAndUnderlyingType(value, definitionTypeRef)
	if !ok {
		return
	}

	v.Report.AddExternalError(operationreport.ErrValueIsNotAnInputObjectType(printedValue, printedType, value.Position))
}

func (v *valuesVisitor) handleUnexpectedNullError(value ast.Value, definitionTypeRef int) {
	printedType, err := v.definition.PrintTypeBytes(definitionTypeRef, nil)
	if v.HandleInternalErr(err) {
		return
	}

	v.Report.AddExternalError(operationreport.ErrNullValueDoesntSatisfyInputValueDefinition(printedType, value.Position))
}

func (v *valuesVisitor) handleUnexpectedEnumValueError(value ast.Value, definitionTypeRef int) {
	printedValue, printedType, ok := v.printValueAndUnderlyingType(value, definitionTypeRef)
	if !ok {
		return
	}

	v.Report.AddExternalError(operationreport.ErrValueDoesntSatisfyEnum(printedValue, printedType, value.Position))
}

func (v *valuesVisitor) handleNotExistingEnumValueError(value ast.Value, definitionTypeRef int) {
	printedValue, printedType, ok := v.printValueAndUnderlyingType(value, definitionTypeRef)
	if !ok {
		return
	}

	v.Report.AddExternalError(operationreport.ErrValueDoesntExistsInEnum(printedValue, printedType, value.Position))
}

func (v *valuesVisitor) handleVariableHasIncompatibleTypeError(value ast.Value, definitionTypeRef int) {
	printedValue, ok := v.printOperationValue(value)
	if !ok {
		return
	}

	expectedTypeName, err := v.definition.PrintTypeBytes(definitionTypeRef, nil)
	if v.HandleInternalErr(err) {
		return
	}

	variableDefinitionRef, _, actualTypeName, ok := v.operationVariableType(value.Ref)
	if !ok {
		return
	}

	v.Report.AddExternalError(operationreport.ErrVariableTypeDoesntSatisfyInputValueDefinition(
		printedValue,
		actualTypeName,
		expectedTypeName,
		value.Position,
		v.operation.VariableDefinitions[variableDefinitionRef].VariableValue.Position,
	))
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

func (v *valuesVisitor) handleOperationTypeError(value ast.Value, operationTypeRef int) {
	printedValue, printedType, ok := v.printOperationValueAndUnderlyingType(value, operationTypeRef)
	if !ok {
		return
	}

	v.Report.AddExternalError(operationreport.ErrValueDoesntSatisfyType(printedValue, printedType, value.Position))
}

func (v *valuesVisitor) handleOperationUnexpectedNullError(value ast.Value, operationTypeRef int) {
	printedType, err := v.operation.PrintTypeBytes(operationTypeRef, nil)
	if v.HandleInternalErr(err) {
		return
	}

	v.Report.AddExternalError(operationreport.ErrNullValueDoesntSatisfyInputValueDefinition(printedType, value.Position))
}

func (v *valuesVisitor) printValueAndUnderlyingType(value ast.Value, definitionTypeRef int) (printedValue, printedType []byte, ok bool) {
	var err error

	printedValue, ok = v.printOperationValue(value)
	if !ok {
		return nil, nil, false
	}

	underlyingType := v.definition.ResolveUnderlyingType(definitionTypeRef)
	printedType, err = v.definition.PrintTypeBytes(underlyingType, nil)
	if v.HandleInternalErr(err) {
		return nil, nil, false
	}

	return printedValue, printedType, true
}

func (v *valuesVisitor) printOperationValueAndUnderlyingType(value ast.Value, operationTypeRef int) (printedValue, printedType []byte, ok bool) {
	printedValue, ok = v.printOperationValue(value)
	if !ok {
		return nil, nil, false
	}

	printedType, ok = v.printUnderlyingOperationType(operationTypeRef)
	if !ok {
		return nil, nil, false
	}

	return printedValue, printedType, true
}

func (v *valuesVisitor) printUnderlyingOperationType(operationTypeRef int) (printedType []byte, ok bool) {
	var err error

	underlyingType := v.operation.ResolveUnderlyingType(operationTypeRef)
	printedType, err = v.operation.PrintTypeBytes(underlyingType, nil)
	if v.HandleInternalErr(err) {
		return nil, false
	}

	return printedType, true
}

func (v *valuesVisitor) printOperationValue(value ast.Value) (printedValue []byte, ok bool) {
	var err error
	printedValue, err = v.operation.PrintValueBytes(value, nil)
	if v.HandleInternalErr(err) {
		return nil, false
	}

	return printedValue, true
}

func (v *valuesVisitor) operationVariableDefinition(variableValueRef int) (ref int, exists bool) {
	variableName := v.operation.VariableValueNameBytes(variableValueRef)

	if v.Ancestors[0].Kind == ast.NodeKindOperationDefinition {
		return v.operation.VariableDefinitionByNameAndOperation(v.Ancestors[0].Ref, variableName)
	}

	for opDefRef := 0; opDefRef < len(v.operation.OperationDefinitions); opDefRef++ {
		ref, exists = v.operation.VariableDefinitionByNameAndOperation(opDefRef, variableName)
		if exists {
			return
		}
	}

	return ast.InvalidRef, false
}

func (v *valuesVisitor) operationVariableType(variableValueRef int) (variableDefinitionRef int, variableTypeRef int, typeName ast.ByteSlice, ok bool) {
	variableDefRef, exists := v.operationVariableDefinition(variableValueRef)
	if !exists {
		return ast.InvalidRef, ast.InvalidRef, nil, false
	}

	variableTypeRef = v.operation.VariableDefinitions[variableDefRef].Type
	typeName = v.operation.ResolveTypeNameBytes(variableTypeRef)

	return variableDefRef, variableTypeRef, typeName, true
}
