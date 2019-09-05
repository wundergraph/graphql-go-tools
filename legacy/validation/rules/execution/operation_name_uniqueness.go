package execution

import (
	"github.com/jensneuse/graphql-go-tools/pkg/lookup"
	"github.com/jensneuse/graphql-go-tools/pkg/validation"
	"github.com/jensneuse/graphql-go-tools/pkg/validation/rules"
)

// OperationNameUniqueness
// https://facebook.github.io/graphql/draft/#sec-Operation-Name-Uniqueness
func OperationNameUniqueness() rules.Rule {
	return func(l *lookup.Lookup, w *lookup.Walker) validation.Result {

		definitions := l.OperationDefinitions()

		for i, first := range definitions {
			for k, second := range definitions {
				if i != k && l.ByteSliceReferenceContentsEquals(first.Name, second.Name) {
					return validation.Invalid(validation.OperationNameUniqueness, validation.OperationNameMustBeUnique, first.Position, first.Name)
				}
			}
		}

		return validation.Valid()
	}
}
