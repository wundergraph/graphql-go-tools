package execution

import (
	"github.com/jensneuse/graphql-go-tools/pkg/lookup"
	"github.com/jensneuse/graphql-go-tools/pkg/validation"
	"github.com/jensneuse/graphql-go-tools/pkg/validation/rules"
)

// Values
func Values() rules.Rule {
	return func(l *lookup.Lookup, w *lookup.Walker) validation.Result {

		iter := w.ArgumentSetIterable()
		for iter.Next() {
			set, parent := iter.Value()
			arguments := l.ArgumentsIterable(set)

			operationDefinitions := w.NodeUsageInOperationsIterator(parent)
			for operationDefinitions.Next() {
				operationDefinition := l.OperationDefinition(operationDefinitions.Value())

				argumentsDefinition := w.ArgumentsDefinition(parent)

				for arguments.Next() {
					argument, _ := arguments.Value()
					inputValueDefinitions := argumentsDefinition.InputValueDefinitions
					inputValueDefinition, ok := l.InputValueDefinitionByNameFromDefinitions(argument.Name, inputValueDefinitions)
					if !ok {
						return validation.Invalid(validation.Values, validation.InputValueNotDefined, argument.Position, argument.Name)
					}

					if !l.ValueIsValid(l.Value(argument.Value), l.Type(inputValueDefinition.Type), operationDefinition.VariableDefinitions, l.InputValueDefinitionHasDefaultValue(inputValueDefinition)) {
						return validation.Invalid(validation.Values, validation.ValueInvalid, argument.Position, argument.Name)
					}
				}
			}
		}

		return validation.Valid()
	}
}
