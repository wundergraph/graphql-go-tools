package execution

import (
	"github.com/jensneuse/graphql-go-tools/legacy/document"
	"github.com/jensneuse/graphql-go-tools/pkg/lookup"
	"github.com/jensneuse/graphql-go-tools/pkg/validation"
	"github.com/jensneuse/graphql-go-tools/pkg/validation/rules"
)

// FieldSelections
func FieldSelections() rules.Rule {
	return func(l *lookup.Lookup, w *lookup.Walker) validation.Result {

		for _, operation := range l.OperationDefinitions() {

			var rootType document.ObjectTypeDefinition
			var exists bool

			switch operation.OperationType {
			case document.OperationTypeQuery:
				rootType, exists = l.QueryObjectTypeDefinition()
			case document.OperationTypeMutation:
				rootType, exists = l.MutationObjectTypeDefinition()
			case document.OperationTypeSubscription:
				rootType, exists = l.SubscriptionObjectTypeDefinition()
			}

			if !exists {
				return validation.Invalid(validation.FieldSelections, validation.RootTypeNotDefined, operation.Position, operation.Name)
			}
			if !l.FieldSelectionsArePossible(rootType.Name, l.SelectionSet(operation.SelectionSet)) {
				return validation.Invalid(validation.FieldSelections, validation.FieldSelectionsInvalid, rootType.Position, rootType.Name)
			}
		}

		for _, fragmentDefinition := range l.FragmentDefinitions() {
			typeCondition := l.Type(fragmentDefinition.TypeCondition)
			if !l.FieldSelectionsArePossible(typeCondition.Name, l.SelectionSet(fragmentDefinition.SelectionSet)) {
				return validation.Invalid(validation.FieldSelections, validation.FieldSelectionsInvalid, fragmentDefinition.Position, fragmentDefinition.FragmentName)
			}
		}

		return validation.Valid()
	}
}
