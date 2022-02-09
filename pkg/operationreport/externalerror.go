package operationreport

import (
	"bytes"
	"fmt"

	"github.com/wundergraph/graphql-go-tools/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/pkg/graphqlerrors"
	"github.com/wundergraph/graphql-go-tools/pkg/lexer/literal"
	"github.com/wundergraph/graphql-go-tools/pkg/lexer/position"
)

const (
	NotCompatibleTypeErrMsg                 = "%s cannot represent value: %s"
	NotStringErrMsg                         = "%s cannot represent a non string value: %s"
	NotIntegerErrMsg                        = "%s cannot represent non-integer value: %s"
	BigIntegerErrMsg                        = "%s cannot represent non 32-bit signed integer value: %s"
	NotFloatErrMsg                          = "%s cannot represent non numeric value: %s"
	NotBooleanErrMsg                        = "%s cannot represent a non boolean value: %s"
	NotIDErrMsg                             = "%s cannot represent a non-string and non-integer value: %s"
	NotEnumErrMsg                           = `Enum "%s" cannot represent non-enum value: %s.`
	NotAnEnumMemberErrMsg                   = `Value "%s" does not exist in "%s" enum.`
	NullValueErrMsg                         = `Expected value of type "%s", found null.`
	UnknownArgumentOnDirectiveErrMsg        = `Unknown argument "%s" on directive "@%s".`
	UnknownArgumentOnFieldErrMsg            = `Unknown argument "%s" on field "%s.%s".`
	VariableIsNotInputTypeErrMsg            = `Variable "$%s" cannot be non-input type "%s".`
	MissingRequiredFieldOfInputObjectErrMsg = `Field "%s.%s" of required type "%s" was not provided.`
	UnknownFieldOfInputObjectErrMsg         = `Field "%s" is not defined by type "%s".`
)

type ExternalError struct {
	Message   string                   `json:"message"`
	Path      ast.Path                 `json:"path"`
	Locations []graphqlerrors.Location `json:"locations"`
}

func LocationsFromPosition(position position.Position) []graphqlerrors.Location {
	return []graphqlerrors.Location{
		{
			Line:   position.LineStart,
			Column: position.CharStart,
		},
	}
}

func ErrDocumentDoesntContainExecutableOperation() (err ExternalError) {
	err.Message = "document doesn't contain any executable operation"
	return
}

func ErrFieldUndefinedOnType(fieldName, typeName ast.ByteSlice) (err ExternalError) {
	err.Message = fmt.Sprintf("field: %s not defined on type: %s", fieldName, typeName)
	return err
}

func ErrFieldNameMustBeUniqueOnType(fieldName, typeName ast.ByteSlice) (err ExternalError) {
	err.Message = fmt.Sprintf("field '%s.%s' can only be defined once", typeName, fieldName)
	return err
}

func ErrTypeUndefined(typeName ast.ByteSlice) (err ExternalError) {
	err.Message = fmt.Sprintf("type not defined: %s", typeName)
	return err
}

func ErrScalarTypeUndefined(scalarName ast.ByteSlice) (err ExternalError) {
	err.Message = fmt.Sprintf("scalar not defined: %s", scalarName)
	return err
}

func ErrInterfaceTypeUndefined(interfaceName ast.ByteSlice) (err ExternalError) {
	err.Message = fmt.Sprintf("interface type not defined: %s", interfaceName)
	return err
}

func ErrUnionTypeUndefined(unionName ast.ByteSlice) (err ExternalError) {
	err.Message = fmt.Sprintf("union type not defined: %s", unionName)
	return err
}

func ErrEnumTypeUndefined(enumName ast.ByteSlice) (err ExternalError) {
	err.Message = fmt.Sprintf("enum type not defined: %s", enumName)
	return err
}

func ErrInputObjectTypeUndefined(inputObjectName ast.ByteSlice) (err ExternalError) {
	err.Message = fmt.Sprintf("input object type not defined: %s", inputObjectName)
	return err
}

func ErrTypeNameMustBeUnique(typeName ast.ByteSlice) (err ExternalError) {
	err.Message = fmt.Sprintf("there can be only one type named '%s'", typeName)
	return err
}

func ErrOperationNameMustBeUnique(operationName ast.ByteSlice) (err ExternalError) {
	err.Message = fmt.Sprintf("operation name must be unique: %s", operationName)
	return err
}

func ErrAnonymousOperationMustBeTheOnlyOperationInDocument() (err ExternalError) {
	err.Message = "anonymous operation name the only operation in a graphql document"
	return err
}

func ErrRequiredOperationNameIsMissing() (err ExternalError) {
	err.Message = "operation name is required when providing multiple operations"
	return err
}

func ErrOperationWithProvidedOperationNameNotFound(operationName string) (err ExternalError) {
	err.Message = fmt.Sprintf("cannot find an operation with name: %s", operationName)
	return err
}

func ErrSubscriptionMustOnlyHaveOneRootSelection(subscriptionName ast.ByteSlice) (err ExternalError) {
	err.Message = fmt.Sprintf("subscription: %s must only have one root selection", subscriptionName)
	return err
}

func ErrFieldSelectionOnUnion(fieldName, unionName ast.ByteSlice) (err ExternalError) {

	err.Message = fmt.Sprintf("cannot select field: %s on union: %s", fieldName, unionName)
	return err
}

func ErrFieldsConflict(objectName, leftType, rightType ast.ByteSlice) (err ExternalError) {
	err.Message = fmt.Sprintf("fields '%s' conflict because they return conflicting types '%s' and '%s'", objectName, leftType, rightType)
	return err
}

func ErrTypesForFieldMismatch(objectName, leftType, rightType ast.ByteSlice) (err ExternalError) {
	err.Message = fmt.Sprintf("differing types '%s' and '%s' for objectName '%s'", leftType, rightType, objectName)
	return err
}

func ErrResponseOfDifferingTypesMustBeOfSameShape(leftObjectName, rightObjectName ast.ByteSlice) (err ExternalError) {
	err.Message = fmt.Sprintf("objects '%s' and '%s' on differing response types must be of same response shape", leftObjectName, rightObjectName)
	return err
}

func ErrDifferingFieldsOnPotentiallySameType(objectName ast.ByteSlice) (err ExternalError) {
	err.Message = fmt.Sprintf("differing fields for objectName '%s' on (potentially) same type", objectName)
	return err
}

func ErrFieldSelectionOnScalar(fieldName, scalarTypeName ast.ByteSlice) (err ExternalError) {
	err.Message = fmt.Sprintf("cannot select field: %s on scalar %s", fieldName, scalarTypeName)
	return err
}

func ErrMissingFieldSelectionOnNonScalar(fieldName, enclosingTypeName ast.ByteSlice) (err ExternalError) {
	err.Message = fmt.Sprintf("non scalar field: %s on type: %s must have selections", fieldName, enclosingTypeName)
	return err
}

func ErrArgumentNotDefinedOnDirective(argName, directiveName ast.ByteSlice, position position.Position) (err ExternalError) {
	err.Message = fmt.Sprintf(UnknownArgumentOnDirectiveErrMsg, argName, directiveName)
	err.Locations = LocationsFromPosition(position)

	return err
}

func ErrMissingRequiredFieldOfInputObject(objName, fieldName, typeName ast.ByteSlice, position position.Position) (err ExternalError) {
	err.Message = fmt.Sprintf(MissingRequiredFieldOfInputObjectErrMsg, objName, fieldName, typeName)
	err.Locations = LocationsFromPosition(position)

	return err
}

func ErrUnknownFieldOfInputObject(objName, fieldName ast.ByteSlice, position position.Position) (err ExternalError) {
	err.Message = fmt.Sprintf(UnknownFieldOfInputObjectErrMsg, objName, fieldName)
	err.Locations = LocationsFromPosition(position)

	return err
}

func ErrArgumentNotDefinedOnField(argName, typeName, fieldName ast.ByteSlice, position position.Position) (err ExternalError) {
	err.Message = fmt.Sprintf(UnknownArgumentOnFieldErrMsg, argName, typeName, fieldName)
	err.Locations = LocationsFromPosition(position)

	return err
}

func ErrNullValueDoesntSatisfyInputValueDefinition(inputType ast.ByteSlice, position position.Position) (err ExternalError) {
	err.Message = fmt.Sprintf(NullValueErrMsg, inputType)
	err.Locations = LocationsFromPosition(position)

	return err
}

func ErrValueDoesntSatisfyEnum(value, inputType ast.ByteSlice, position position.Position) (err ExternalError) {
	err.Message = fmt.Sprintf(NotEnumErrMsg, inputType, value)
	err.Locations = LocationsFromPosition(position)

	return err
}

func ErrValueDoesntExistsInEnum(value, inputType ast.ByteSlice, position position.Position) (err ExternalError) {
	err.Message = fmt.Sprintf(NotAnEnumMemberErrMsg, value, inputType)
	err.Locations = LocationsFromPosition(position)

	return err
}

func ErrValueDoesntSatisfyType(value, inputType ast.ByteSlice, position position.Position) (err ExternalError) {
	err.Message = fmt.Sprintf(NotCompatibleTypeErrMsg, inputType, value)
	err.Locations = LocationsFromPosition(position)

	return err
}

func ErrValueDoesntSatisfyString(value, inputType ast.ByteSlice, position position.Position) (err ExternalError) {
	err.Message = fmt.Sprintf(NotStringErrMsg, inputType, value)
	err.Locations = LocationsFromPosition(position)

	return err
}

func ErrValueDoesntSatisfyInt(value, inputType ast.ByteSlice, position position.Position) (err ExternalError) {
	err.Message = fmt.Sprintf(NotIntegerErrMsg, inputType, value)
	err.Locations = LocationsFromPosition(position)

	return err
}

func ErrBigIntValueDoesntSatisfyInt(value, inputType ast.ByteSlice, position position.Position) (err ExternalError) {
	err.Message = fmt.Sprintf(BigIntegerErrMsg, inputType, value)
	err.Locations = LocationsFromPosition(position)

	return err
}

func ErrValueDoesntSatisfyFloat(value, inputType ast.ByteSlice, position position.Position) (err ExternalError) {
	err.Message = fmt.Sprintf(NotFloatErrMsg, inputType, value)
	err.Locations = LocationsFromPosition(position)

	return err
}

func ErrValueDoesntSatisfyBoolean(value, inputType ast.ByteSlice, position position.Position) (err ExternalError) {
	err.Message = fmt.Sprintf(NotBooleanErrMsg, inputType, value)
	err.Locations = LocationsFromPosition(position)

	return err
}

func ErrValueDoesntSatisfyID(value, inputType ast.ByteSlice, position position.Position) (err ExternalError) {
	err.Message = fmt.Sprintf(NotIDErrMsg, inputType, value)
	err.Locations = LocationsFromPosition(position)

	return err
}

func ErrValueDoesntSatisfyInputValueDefinition(value, inputType ast.ByteSlice, position position.Position) (err ExternalError) {
	switch {
	case bytes.Equal(literal.STRING, inputType):
		return ErrValueDoesntSatisfyString(value, inputType, position)
	case bytes.Equal(literal.INT, inputType):
		return ErrValueDoesntSatisfyInt(value, inputType, position)
	case bytes.Equal(literal.FLOAT, inputType):
		return ErrValueDoesntSatisfyFloat(value, inputType, position)
	case bytes.Equal(literal.BOOLEAN, inputType):
		return ErrValueDoesntSatisfyBoolean(value, inputType, position)
	case bytes.Equal(literal.ID, inputType):
		return ErrValueDoesntSatisfyID(value, inputType, position)
	default:
		return ErrValueDoesntSatisfyType(value, inputType, position)
	}
}

func ErrVariableTypeDoesntSatisfyInputValueDefinition(value, inputType, expectedType ast.ByteSlice, valuePos, variableDefinitionPos position.Position) (err ExternalError) {
	err.Message = fmt.Sprintf(`Variable "%v" of type "%v" used in position expecting type "%v".`, value, inputType, expectedType)
	err.Locations = []graphqlerrors.Location{
		{
			Line:   variableDefinitionPos.LineStart,
			Column: variableDefinitionPos.CharStart,
		},
		{
			Line:   valuePos.LineStart,
			Column: valuePos.CharStart,
		},
	}
	return err
}

func ErrVariableNotDefinedOnOperation(variableName, operationName ast.ByteSlice) (err ExternalError) {
	err.Message = fmt.Sprintf("variable: %s not defined on operation: %s", variableName, operationName)
	return err
}

func ErrVariableDefinedButNeverUsed(variableName, operationName ast.ByteSlice) (err ExternalError) {
	err.Message = fmt.Sprintf("variable: %s defined on operation: %s but never used", variableName, operationName)
	return err
}

func ErrVariableMustBeUnique(variableName, operationName ast.ByteSlice) (err ExternalError) {
	err.Message = fmt.Sprintf("variable: %s must be unique per operation: %s", variableName, operationName)
	return err
}

func ErrVariableNotDefinedOnArgument(variableName, argumentName ast.ByteSlice) (err ExternalError) {
	err.Message = fmt.Sprintf("variable: %s not defined on argument: %s", variableName, argumentName)
	return err
}

func ErrVariableOfTypeIsNoValidInputValue(variableName, ofTypeName ast.ByteSlice, position position.Position) (err ExternalError) {
	err.Message = fmt.Sprintf(VariableIsNotInputTypeErrMsg, variableName, ofTypeName)
	err.Locations = LocationsFromPosition(position)

	return err
}

func ErrArgumentMustBeUnique(argName ast.ByteSlice) (err ExternalError) {
	err.Message = fmt.Sprintf("argument: %s must be unique", argName)
	return err
}

func ErrArgumentRequiredOnField(argName, fieldName ast.ByteSlice) (err ExternalError) {
	err.Message = fmt.Sprintf("argument: %s is required on field: %s but missing", argName, fieldName)
	return err
}

func ErrArgumentOnFieldMustNotBeNull(argName, fieldName ast.ByteSlice) (err ExternalError) {
	err.Message = fmt.Sprintf("argument: %s on field: %s must not be null", argName, fieldName)
	return err
}

func ErrFragmentSpreadFormsCycle(spreadName ast.ByteSlice) (err ExternalError) {
	err.Message = fmt.Sprintf("fragment spread: %s forms fragment cycle", spreadName)
	return err
}

func ErrFragmentDefinedButNotUsed(fragmentName ast.ByteSlice) (err ExternalError) {
	err.Message = fmt.Sprintf("fragment: %s defined but not used", fragmentName)
	return err
}

func ErrFragmentUndefined(fragmentName ast.ByteSlice) (err ExternalError) {
	err.Message = fmt.Sprintf("fragment: %s undefined", fragmentName)
	return err
}

func ErrInlineFragmentOnTypeDisallowed(onTypeName ast.ByteSlice) (err ExternalError) {
	err.Message = fmt.Sprintf("inline fragment on type: %s disallowed", onTypeName)
	return err
}

func ErrInlineFragmentOnTypeMismatchEnclosingType(fragmentTypeName, enclosingTypeName ast.ByteSlice) (err ExternalError) {
	err.Message = fmt.Sprintf("inline fragment on type: %s mismatches enclosing type: %s", fragmentTypeName, enclosingTypeName)
	return err
}

func ErrFragmentDefinitionOnTypeDisallowed(fragmentName, onTypeName ast.ByteSlice) (err ExternalError) {
	err.Message = fmt.Sprintf("fragment: %s on type: %s disallowed", fragmentName, onTypeName)
	return err
}

func ErrFragmentDefinitionMustBeUnique(fragmentName ast.ByteSlice) (err ExternalError) {
	err.Message = fmt.Sprintf("fragment: %s must be unique per document", fragmentName)
	return err
}

func ErrDirectiveUndefined(directiveName ast.ByteSlice) (err ExternalError) {
	err.Message = fmt.Sprintf("directive: %s undefined", directiveName)
	return err
}

func ErrDirectiveNotAllowedOnNode(directiveName, nodeKindName ast.ByteSlice) (err ExternalError) {
	err.Message = fmt.Sprintf("directive: %s not allowed on node of kind: %s", directiveName, nodeKindName)
	return err
}

func ErrDirectiveMustBeUniquePerLocation(directiveName ast.ByteSlice) (err ExternalError) {
	err.Message = fmt.Sprintf("directive: %s must be unique per location", directiveName)
	return err
}

func ErrOnlyOneQueryTypeAllowed() (err ExternalError) {
	err.Message = "there can be only one query type in schema"
	return err
}

func ErrOnlyOneMutationTypeAllowed() (err ExternalError) {
	err.Message = "there can be only one mutation type in schema"
	return err
}

func ErrOnlyOneSubscriptionTypeAllowed() (err ExternalError) {
	err.Message = "there can be only one subscription type in schema"
	return err
}

func ErrEnumValueNameMustBeUnique(enumName, enumValueName ast.ByteSlice) (err ExternalError) {
	err.Message = fmt.Sprintf("enum value '%s.%s' can only be defined once", enumName, enumValueName)
	return err
}

func ErrUnionMembersMustBeUnique(unionName, memberName ast.ByteSlice) (err ExternalError) {
	err.Message = fmt.Sprintf("union member '%s.%s' can only be defined once", unionName, memberName)
	return err
}

func ErrTransitiveInterfaceNotImplemented(typeName, transitiveInterfaceName ast.ByteSlice) (err ExternalError) {
	err.Message = fmt.Sprintf("type %s does not implement transitive interface %s", typeName, transitiveInterfaceName)
	return err
}

func ErrTransitiveInterfaceExtensionImplementingWithoutBody(interfaceExtensionName ast.ByteSlice) (err ExternalError) {
	err.Message = fmt.Sprintf("interface extension %s implementing interface without body", interfaceExtensionName)
	return err
}

func ErrTypeDoesNotImplementFieldFromInterface(typeName, interfaceName, fieldName ast.ByteSlice) (err ExternalError) {
	err.Message = fmt.Sprintf("type '%s' does not implement field '%s' from interface '%s'", typeName, fieldName, interfaceName)
	return err
}

func ErrImplementingTypeDoesNotHaveFields(typeName ast.ByteSlice) (err ExternalError) {
	err.Message = fmt.Sprintf("type '%s' implements an interface but does not have any fields defined", typeName)
	return err
}

func ErrSharedTypesMustBeIdenticalToFederate(typeName string) (err ExternalError) {
	err.Message = fmt.Sprintf("the shared type named '%s' must be identical in any subgraphs to federate", typeName)
	return err
}

func ErrEntitiesMustNotBeDuplicated(typeName string) (err ExternalError) {
	err.Message = fmt.Sprintf("the entity named '%s' is defined in the subgraph(s) more than once", typeName)
	return err
}

func ErrSharedTypesMustNotBeExtended(typeName string) (err ExternalError) {
	err.Message = fmt.Sprintf("the type named '%s' cannot be extended because it is a shared type", typeName)
	return err
}

func ErrExtensionOrphansMustResolveInSupergraph(extensionNameBytes []byte) (err ExternalError) {
	err.Message = fmt.Sprintf("the extension orphan named '%s' was never resolved in the supergraph", extensionNameBytes)
	return err
}

func ErrTypeBodyMustNotBeEmpty(definitionType, typeName string) (err ExternalError) {
	err.Message = fmt.Sprintf("the %s named '%s' is invalid due to an empty body", definitionType, typeName)
	return err
}

func ErrEntityExtensionMustHaveKeyDirective(typeName string) (err ExternalError) {
	err.Message = fmt.Sprintf("an extension of the entity named '%s' does not have a key directive", typeName)
	return err
}

func ErrExtensionWithKeyDirectiveMustExtendEntity(typeName string) (err ExternalError) {
	err.Message = fmt.Sprintf("the extension named '%s' has a key directive but there is no entity of the same name", typeName)
	return err
}
