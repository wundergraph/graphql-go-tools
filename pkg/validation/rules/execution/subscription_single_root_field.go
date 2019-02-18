package execution

import (
	"github.com/jensneuse/graphql-go-tools/pkg/document"
	"github.com/jensneuse/graphql-go-tools/pkg/lookup"
	"github.com/jensneuse/graphql-go-tools/pkg/validation"
	"github.com/jensneuse/graphql-go-tools/pkg/validation/rules"
)

// SubscriptionSingleRootField
// https://facebook.github.io/graphql/draft/#sec-Single-root-field
func SubscriptionSingleRootField() rules.ExecutionRule {
	return func(l *lookup.Lookup, w *lookup.Walker) validation.Result {

		for _, operation := range l.OperationDefinitions() {

			if operation.OperationType != document.OperationTypeSubscription {
				continue
			}

			rootFields := l.SelectionSetNumRootFields(l.SelectionSet(operation.SelectionSet))

			if rootFields > 1 {
				return validation.Invalid(validation.SubscriptionSingleRootField, validation.SubscriptionsMustHaveMaxOneRootField, operation.Position, operation.Name)
			}

		}

		return validation.Valid()
	}
}
