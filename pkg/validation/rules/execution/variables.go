package execution

import (
	"github.com/jensneuse/graphql-go-tools/pkg/document"
	"github.com/jensneuse/graphql-go-tools/pkg/lookup"
	"github.com/jensneuse/graphql-go-tools/pkg/validation"
	"github.com/jensneuse/graphql-go-tools/pkg/validation/rules"
)

func VariableUniqueness() rules.ExecutionRule {
	return func(l *lookup.Lookup, w *lookup.Walker) validation.Result {

		iter := w.OperationDefinitionIterable()
		for iter.Next() {

			definition := iter.Value()

			if len(definition.VariableDefinitions) <= 1 {
				continue
			}

			leftVariables := l.VariableDefinitionIterator(definition.VariableDefinitions)
			for leftVariables.Next() {
				left, i := leftVariables.Value()
				rightVariables := l.VariableDefinitionIterator(definition.VariableDefinitions)
				for rightVariables.Next() {
					right, j := rightVariables.Value()
					if i == j {
						continue
					}
					if left.Variable == right.Variable {
						return validation.Invalid(validation.VariableUniqueness, validation.VariableMustBeUniquePerOperation, left.Position, left.Variable)
					}
				}
			}
		}

		return validation.Valid()
	}
}

func VariablesAreInputTypes() rules.ExecutionRule {
	return func(l *lookup.Lookup, w *lookup.Walker) validation.Result {
		iter := w.OperationDefinitionIterable()
		for iter.Next() {
			definition := iter.Value()

			variables := l.VariableDefinitionIterator(definition.VariableDefinitions)
			for variables.Next() {
				variable, _ := variables.Value()
				variableType := l.Type(variable.Type)
				unwrappedType := l.UnwrappedNamedType(variableType)
				_, isScalar := l.ScalarTypeDefinitionByName(unwrappedType.Name)
				if isScalar {
					continue
				}
				_, isEnum := l.EnumTypeDefinitionByName(unwrappedType.Name)
				if isEnum {
					continue
				}
				_, isInputObjectType := l.InputObjectTypeDefinitionByName(unwrappedType.Name)
				if isInputObjectType {
					continue
				}

				return validation.Invalid(validation.VariablesAreInputTypes, validation.VariableMustBeValidInputType, variable.Position, variable.Variable)
			}
		}

		return validation.Valid()
	}
}

func AllVariableUsesDefined() rules.ExecutionRule {
	return func(l *lookup.Lookup, w *lookup.Walker) validation.Result {

		isVariable := func(value document.Value) bool {
			return value.ValueType == document.ValueTypeVariable
		}

		iter := w.ArgumentSetIterable()
		for iter.Next() {
			set, _ := iter.Value()
			arguments := l.ArgumentsIterable(set)

			for arguments.Next() {
				argument, ref := arguments.Value()
				value := l.Value(argument.Value)
				if isVariable(value) {

					operationDefinitions := w.NodeUsageInOperationsIterator(ref)
					for operationDefinitions.Next() {
						operationDefinition := l.OperationDefinition(operationDefinitions.Value())
						_, isDefined := l.VariableDefinition(value.Reference, operationDefinition.VariableDefinitions)
						if !isDefined {
							return validation.Invalid(validation.AllVariableUsesDefined, validation.VariableNotDefined, value.Position, value.Reference)
						}
					}
				}
			}
		}

		return validation.Valid()
	}
}

func AllVariablesUsed() rules.ExecutionRule {
	return func(l *lookup.Lookup, w *lookup.Walker) validation.Result {

		isVariable := func(value document.Value) bool {
			return value.ValueType == document.ValueTypeVariable
		}

		iter := w.OperationDefinitionIterable()
		for iter.Next() {
			definition := iter.Value()

			variables := l.VariableDefinitionIterator(definition.VariableDefinitions)

		withNextVariable:
			for variables.Next() {
				variable, _ := variables.Value()
				argumentSetIter := w.ArgumentSetIterable()
				for argumentSetIter.Next() {
					set, _ := argumentSetIter.Value()
					arguments := l.ArgumentsIterable(set)
					for arguments.Next() {
						argument, _ := arguments.Value()
						value := l.Value(argument.Value)
						if isVariable(value) && value.Reference == variable.Variable {
							continue withNextVariable
						}
					}
				}

				return validation.Invalid(validation.AllVariablesUsed, validation.VariableDefinedButNotUsed, variable.Position, variable.Variable)
			}
		}

		return validation.Valid()
	}
}
