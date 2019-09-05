package execution

import (
	"github.com/jensneuse/graphql-go-tools/pkg/lookup"
	"github.com/jensneuse/graphql-go-tools/pkg/validation"
	"github.com/jensneuse/graphql-go-tools/pkg/validation/rules"
)

func DirectivesAreDefined() rules.Rule {
	return func(l *lookup.Lookup, w *lookup.Walker) validation.Result {

		sets := w.DirectiveSetIterable()
		for sets.Next() {
			set, _ := sets.Value()
			directives := l.DirectiveIterable(set)
			for directives.Next() {
				directive, _ := directives.Value()

				_, ok := l.DirectiveDefinitionByName(directive.Name)
				if !ok {
					return validation.Invalid(validation.DirectivesAreDefined, validation.DirectiveNotDefined, directive.Position, directive.Name)
				}
			}
		}

		return validation.Valid()
	}
}

func DirectivesAreInValidLocations() rules.Rule {

	locationIsValid := func(validLocations []int, actual int) bool {
		for _, expected := range validLocations {
			if expected == actual {
				return true
			}
		}
		return false
	}

	return func(l *lookup.Lookup, w *lookup.Walker) validation.Result {

		sets := w.DirectiveSetIterable()
		for sets.Next() {
			set, parent := sets.Value()
			directives := l.DirectiveIterable(set)
			for directives.Next() {
				directive, _ := directives.Value()

				definition, ok := l.DirectiveDefinitionByName(directive.Name)
				if !ok {
					return validation.Invalid(validation.DirectivesAreInValidLocations, validation.DirectiveNotDefined, directive.Position, directive.Name)
				}

				node, _ := w.Parent(parent)
				// In the current implementation directives cannot be parsed on their own,
				// they're always attached to a parent node and therefore this case can currently not occur

				directiveLocation := l.DirectiveLocationFromNode(node)
				if !locationIsValid(definition.DirectiveLocations, int(directiveLocation)) {
					return validation.Invalid(validation.DirectivesAreInValidLocations, validation.DirectiveLocationInvalid, directive.Position, directive.Name)
				}
			}
		}

		return validation.Valid()
	}
}

func DirectivesAreUniquePerLocation() rules.Rule {

	return func(l *lookup.Lookup, w *lookup.Walker) validation.Result {

		sets := w.DirectiveSetIterable()
		for sets.Next() {
			set, _ := sets.Value()
			leftDirectives := l.DirectiveIterable(set)
			for leftDirectives.Next() {
				left, i := leftDirectives.Value()
				rightDirectives := l.DirectiveIterable(set)
				for rightDirectives.Next() {
					right, j := rightDirectives.Value()
					if i == j {
						continue
					}
					if l.ByteSliceReferenceContentsEquals(left.Name, right.Name) {
						return validation.Invalid(validation.DirectivesAreUniquePerLocation, validation.DirectiveMustBeUniquePerLocation, left.Position, left.Name)
					}
				}
			}
		}

		return validation.Valid()
	}
}
