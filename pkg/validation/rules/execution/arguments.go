package execution

import (
	"github.com/jensneuse/graphql-go-tools/pkg/document"
	"github.com/jensneuse/graphql-go-tools/pkg/lookup"
	"github.com/jensneuse/graphql-go-tools/pkg/validation"
	"github.com/jensneuse/graphql-go-tools/pkg/validation/rules"
)

// ValidArguments checks if arguments present fit the input value definition
func ValidArguments() rules.Rule {
	return func(l *lookup.Lookup, w *lookup.Walker) validation.Result {

		argumentSets := w.ArgumentSetIterable()
		for argumentSets.Next() {
			set, parent := argumentSets.Value()
			arguments := l.ArgumentsIterable(set)

			operationDefinitions := w.NodeUsageInOperationsIterator(parent)
			for operationDefinitions.Next() {
				operationDefinition := l.OperationDefinition(operationDefinitions.Value())

				argumentsDefinition := w.ArgumentsDefinition(parent)

				for arguments.Next() {
					argument, _ := arguments.Value()
					inputValueDefinition, ok := l.InputValueDefinitionByNameFromDefinitions(argument.Name, argumentsDefinition.InputValueDefinitions)
					if !ok {
						return validation.Invalid(validation.ValidArguments, validation.InputValueNotDefined, argument.Position, argument.Name)
					}
					value := l.Value(argument.Value)
					inputType := l.Type(inputValueDefinition.Type)

					if !l.ValueIsValid(value, inputType, operationDefinition.VariableDefinitions, l.InputValueDefinitionHasDefaultValue(inputValueDefinition)) {
						return validation.Invalid(validation.ValidArguments, validation.ValueInvalid, value.Position, argument.Name)
					}
				}
			}
		}

		return validation.Valid()
	}
}

// ArgumentUniqueness checks if arguments are unique per argument set
func ArgumentUniqueness() rules.Rule {
	return func(l *lookup.Lookup, w *lookup.Walker) validation.Result {

		iter := w.ArgumentSetIterable()
		for iter.Next() {
			set, _ := iter.Value()
			leftArguments := l.ArgumentsIterable(set)
			for leftArguments.Next() {
				left, i := leftArguments.Value()
				rightArguments := l.ArgumentsIterable(set)
				for rightArguments.Next() {
					right, j := rightArguments.Value()
					if i == j {
						continue
					}
					if l.ByteSliceReferenceContentsEquals(left.Name, right.Name) {
						return validation.Invalid(validation.ArgumentUniqueness, validation.ArgumentMustBeUnique, left.Position, left.Name)
					}
				}
			}
		}

		return validation.Valid()
	}
}

// RequiredArguments checks if required arguments are defined
func RequiredArguments() rules.Rule {
	return func(l *lookup.Lookup, w *lookup.Walker) validation.Result {

		hasNamedArgument := func(argumentSet int, name document.ByteSliceReference) bool {
			args := l.ArgumentsIterable(l.ArgumentSet(argumentSet))
			for args.Next() {
				arg, _ := args.Value()
				if l.ByteSliceReferenceContentsEquals(arg.Name, name) {
					return true
				}
			}
			return false
		}

		fields := w.FieldsIterable()
		for fields.Next() {

			field, _, parent := fields.Value()
			typeName := w.SelectionSetTypeName(l.SelectionSet(field.SelectionSet), parent)

			fieldsDefinition := l.FieldsDefinitionFromNamedType(typeName)
			definition, ok := l.FieldDefinitionByNameFromIndex(fieldsDefinition, field.Name)
			if !ok {
				return validation.Invalid(validation.RequiredArguments, validation.TypeNotDefined, field.Position, field.Name)
			}

			argumentsDefinition := l.ArgumentsDefinition(definition.ArgumentsDefinition)
			if !argumentsDefinition.InputValueDefinitions.HasNext() {
				continue
			}

			inputValueDefinitions := argumentsDefinition.InputValueDefinitions
			for inputValueDefinitions.Next(l) {
				inputValueDefinition, _ := inputValueDefinitions.Value()
				if !l.InputValueDefinitionIsRequired(inputValueDefinition) {
					continue
				}
				if !hasNamedArgument(field.ArgumentSet, inputValueDefinition.Name) {
					return validation.Invalid(validation.RequiredArguments, validation.ArgumentRequired, definition.Position, definition.Name)
				}
			}
		}

		return validation.Valid()
	}
}
