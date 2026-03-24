package astvalidation

import (
	"bytes"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/operationreport"
)

func (v *valuesVisitor) validateIfValueSatisfiesInputFieldDefinition(value ast.Value, inputValueDefinitionRef int) {
	var (
		satisfied             bool
		operationTypeRef      int
		variableDefinitionRef int
	)

	satisfied, operationTypeRef, variableDefinitionRef = v.variableValueSatisfiesInputValueDefinition(value.Ref, inputValueDefinitionRef)

	if satisfied {
		return
	}

	if operationTypeRef == ast.InvalidRef {
		// variable is not defined
		return
	}

	printedValue, err := v.operation.PrintValueBytes(value, nil)
	if v.HandleInternalErr(err) {
		return
	}

	typeRef := v.definition.InputValueDefinitionType(inputValueDefinitionRef)
	expectedTypeName, err := v.definition.PrintTypeBytes(typeRef, nil)
	if v.HandleInternalErr(err) {
		return
	}

	actualTypeName, err := v.operation.PrintTypeBytes(operationTypeRef, nil)
	if v.HandleInternalErr(err) {
		return
	}

	v.StopWithExternalErr(operationreport.ErrVariableTypeDoesntSatisfyInputValueDefinition(printedValue, actualTypeName, expectedTypeName, value.Position, v.operation.VariableDefinitions[variableDefinitionRef].VariableValue.Position))
}

func (v *valuesVisitor) variableValueSatisfiesInputValueDefinition(variableValue, inputValueDefinition int) (satisfies bool, operationTypeRef int, variableDefRef int) {
	variableDefinitionRef, exists := v.variableDefinition(variableValue)
	if !exists {
		return false, ast.InvalidRef, variableDefinitionRef
	}

	operationTypeRef = v.operation.VariableDefinitions[variableDefinitionRef].Type
	definitionTypeRef := v.definition.InputValueDefinitions[inputValueDefinition].Type

	hasDefaultValue := v.validDefaultValue(v.operation.VariableDefinitions[variableDefinitionRef].DefaultValue) ||
		v.validDefaultValue(v.definition.InputValueDefinitions[inputValueDefinition].DefaultValue)

	return v.operationTypeSatisfiesDefinitionType(operationTypeRef, definitionTypeRef, hasDefaultValue), operationTypeRef, variableDefinitionRef
}

func (v *valuesVisitor) variableDefinition(variableValueRef int) (ref int, exists bool) {
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

func (v *valuesVisitor) validDefaultValue(value ast.DefaultValue) bool {
	return value.IsDefined && value.Value.Kind != ast.ValueKindNull
}

func (v *valuesVisitor) operationTypeSatisfiesDefinitionType(operationTypeRef int, definitionTypeRef int, hasDefaultValue bool) bool {
	opKind := v.operation.Types[operationTypeRef].TypeKind
	defKind := v.definition.Types[definitionTypeRef].TypeKind

	// A nullable op type is compatible with a non-null def type if the def has
	// a default value. Strip the def non-null and continue comparing. This
	// logic is only valid before any unnesting of types occurs, which is why
	// it's outside the for loop below.
	//
	// Example:
	// Op:  someField(arg: Boolean): String
	// Def: someField(arg: Boolean! = false): String  #  Boolean! -> Boolean
	if opKind != ast.TypeKindNonNull && defKind == ast.TypeKindNonNull && hasDefaultValue {
		definitionTypeRef = v.definition.Types[definitionTypeRef].OfType
	}

	// Unnest the op and def arg types until a named type is reached,
	// then compare.
	for {
		if operationTypeRef == -1 || definitionTypeRef == -1 {
			return false
		}
		opKind = v.operation.Types[operationTypeRef].TypeKind
		defKind = v.definition.Types[definitionTypeRef].TypeKind

		// If the op arg type is stricter than the def arg type, that's okay.
		// Strip the op non-null and continue comparing.
		//
		// Example:
		// Op:  someField(arg: Boolean!): String  # Boolean! -> Boolean
		// Def: someField(arg: Boolean): String
		if opKind == ast.TypeKindNonNull && defKind != ast.TypeKindNonNull {
			operationTypeRef = v.operation.Types[operationTypeRef].OfType
			continue
		}

		if opKind != defKind {
			return false
		}
		if opKind == ast.TypeKindNamed {
			// defKind is also a named type because at this point both kinds
			// are the same! Compare the names.

			return bytes.Equal(v.operation.Input.ByteSlice(v.operation.Types[operationTypeRef].Name),
				v.definition.Input.ByteSlice(v.definition.Types[definitionTypeRef].Name))
		}
		// Both types are non-null or list. Unnest and continue comparing.
		operationTypeRef = v.operation.Types[operationTypeRef].OfType
		definitionTypeRef = v.definition.Types[definitionTypeRef].OfType
	}
}
