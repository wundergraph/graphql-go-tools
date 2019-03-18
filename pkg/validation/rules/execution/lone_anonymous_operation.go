package execution

import (
	"github.com/jensneuse/graphql-go-tools/pkg/lookup"
	"github.com/jensneuse/graphql-go-tools/pkg/validation"
	"github.com/jensneuse/graphql-go-tools/pkg/validation/rules"
)

// LoneAnonymousOperation
// https://facebook.github.io/graphql/draft/#sec-Lone-Anonymous-Operation
func LoneAnonymousOperation() rules.Rule {
	return func(l *lookup.Lookup, w *lookup.Walker) validation.Result {

		definitions := l.OperationDefinitions()
		if len(definitions) <= 1 {
			return validation.Valid()
		}

		for _, definition := range definitions {
			if definition.Name == -1 {
				return validation.Invalid(validation.LoneAnonymousOperation, validation.AnonymousOperationMustBeLonePerDocument, definition.Position, definition.Name)
			}
		}

		return validation.Valid()
	}
}
