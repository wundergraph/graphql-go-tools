package ast

import (
	"bytes"
	"errors"

	"github.com/buger/jsonparser"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/internal/unsafebytes"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/lexer/literal"
)

// IfArgumentCoercion is the result of the coercion of an "if: Boolean! = true" directive argument.
type IfArgumentCoercion uint8

const (
	IfArgumentTrue IfArgumentCoercion = iota
	IfArgumentFalse
	// IfArgumentNull is a nullable variable with a coerced null value.
	// No validator reports it, so the caller must report it.
	IfArgumentNull
	// IfArgumentInvalid is a value that a validator reports.
	// Operation validation or variables validation reports it, when the directive stays in the operation.
	IfArgumentInvalid
)

// CoerceIfArgument coerces the "if: Boolean! = true" argument of the directive.
// It follows CoerceVariableValues and then CoerceArgumentValues of the GraphQL specification.
// It reads the variable definitions of the operation definition operationDefinitionRef and the raw request variables in Input.Variables.
func (d *Document) CoerceIfArgument(directiveRef, operationDefinitionRef int) IfArgumentCoercion {
	value, ok := d.DirectiveArgumentValueByName(directiveRef, literal.IF)
	if !ok {
		return IfArgumentTrue
	}
	switch value.Kind {
	case ValueKindBoolean:
		return ifArgumentCoercion(bool(d.BooleanValue(value.Ref)))
	case ValueKindVariable:
		return d.coerceIfVariable(value, operationDefinitionRef)
	default:
		return IfArgumentInvalid
	}
}

// coerceIfVariable does not report a missing or null value of a non-null variable.
// Variables validation reports it, when the directive stays in the operation.
// An absent nullable variable without a default takes the argument default true.
func (d *Document) coerceIfVariable(value Value, operationDefinitionRef int) IfArgumentCoercion {
	name := d.VariableValueNameBytes(value.Ref)
	variableDefinitionRef, ok := d.VariableDefinitionByNameAndOperation(operationDefinitionRef, name)
	if !ok {
		return IfArgumentInvalid
	}
	typeRef := d.VariableDefinitions[variableDefinitionRef].Type
	if d.TypeIsList(typeRef) || !bytes.Equal(d.ResolveTypeNameBytes(typeRef), literal.BOOLEAN) {
		return IfArgumentInvalid
	}
	nonNullVariable := d.TypeIsNonNull(typeRef)

	isNull := false
	variableValue, dataType, _, err := jsonparser.Get(d.Input.Variables, unsafebytes.BytesToString(name))
	switch {
	case errors.Is(err, jsonparser.KeyPathNotFoundError):
		if !d.VariableDefinitionHasDefaultValue(variableDefinitionRef) {
			if nonNullVariable {
				return IfArgumentInvalid
			}
			return IfArgumentTrue
		}
		defaultValue := d.VariableDefinitionDefaultValue(variableDefinitionRef)
		if defaultValue.Kind == ValueKindBoolean {
			return ifArgumentCoercion(bool(d.BooleanValue(defaultValue.Ref)))
		}
		isNull = defaultValue.Kind == ValueKindNull
	case err == nil && dataType == jsonparser.Boolean:
		return ifArgumentCoercion(bytes.Equal(variableValue, literal.TRUE))
	case err == nil && dataType == jsonparser.Null:
		isNull = true
	}

	if isNull && !nonNullVariable {
		return IfArgumentNull
	}
	return IfArgumentInvalid
}

func ifArgumentCoercion(enabled bool) IfArgumentCoercion {
	if enabled {
		return IfArgumentTrue
	}
	return IfArgumentFalse
}
