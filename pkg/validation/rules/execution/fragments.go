package execution

import (
	"github.com/jensneuse/graphql-go-tools/pkg/lookup"
	"github.com/jensneuse/graphql-go-tools/pkg/validation"
	"github.com/jensneuse/graphql-go-tools/pkg/validation/rules"
)

// Fragments
// https://facebook.github.io/graphql/draft/#sec-Fragment-Name-Uniqueness
func Fragments() rules.ExecutionRule {
	return func(l *lookup.Lookup, w *lookup.Walker) validation.Result {

		definitions := l.FragmentDefinitions()
		for i, definition := range definitions {

			typeCondition := l.Type(definition.TypeCondition)
			if l.TypeIsScalarOrEnum(typeCondition.Name) {
				return validation.Invalid(validation.Fragments, validation.FragmentDefinitionOnLeafNode, definition.Position, definition.FragmentName)
			}
			if !l.TypeIsValidFragmentTypeCondition(typeCondition.Name) {
				return validation.Invalid(validation.Fragments, validation.TypeNotDefined, typeCondition.Position, typeCondition.Name)
			}

			if !l.IsUniqueFragmentName(i, definition.FragmentName) {
				return validation.Invalid(validation.Fragments, validation.FragmentRedeclared, definition.Position, definition.FragmentName)
			}
			if !l.IsFragmentDefinitionUsedInOperation(definition.FragmentName) {
				return validation.Invalid(validation.Fragments, validation.FragmentDeclaredButNeverUsed, definition.Position, definition.FragmentName)
			}
			if !l.FragmentSelectionsArePossible(typeCondition.Name, l.SelectionSet(definition.SelectionSet)) {
				return validation.Invalid(validation.Fragments, validation.SelectionSetInvalid, l.SelectionSet(definition.SelectionSet).Position, definition.FragmentName)
			}
		}

		fragmentSpreads := l.FragmentSpreads()
		for i, fragment := range fragmentSpreads {
			definition, _, ok := l.FragmentDefinitionByName(fragment.FragmentName)
			if !ok {
				return validation.Invalid(validation.Fragments, validation.FragmentNotDefined, fragment.Position, fragment.FragmentName)
			}

			if l.SelectionSetContainsFragmentSpread(l.SelectionSet(definition.SelectionSet), i) {
				return validation.Invalid(validation.Fragments, validation.FragmentSpreadCyclicReference, definition.Position, definition.FragmentName)
			}
		}

		return validation.Valid()
	}
}
