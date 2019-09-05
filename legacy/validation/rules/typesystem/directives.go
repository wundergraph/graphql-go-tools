package typesystem

import (
	"github.com/jensneuse/graphql-go-tools/legacy/document"
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

func DirectivesHaveRequiredArguments() rules.Rule {
	return func(l *lookup.Lookup, w *lookup.Walker) validation.Result {

		sets := w.DirectiveSetIterable()
		for sets.Next() {
			set, _ := sets.Value()
			directives := l.DirectiveIterable(set)
			for directives.Next() {
				directive, _ := directives.Value()
				definition, exists := l.DirectiveDefinitionByName(directive.Name)
				if !exists {
					return validation.Invalid(validation.DirectivesHaveRequiredArguments, validation.DirectiveNotDefined, directive.Position, directive.Name)
				}
				argumentsDefinition := l.ArgumentsDefinition(definition.ArgumentsDefinition)
				inputValueDefinitions := argumentsDefinition.InputValueDefinitions

				argumentSet := l.ArgumentSet(directive.ArgumentSet)

			WithNextInputValueDefinition:
				for inputValueDefinitions.Next(l) {
					inputValueDefinition, _ := inputValueDefinitions.Value()
					hasDefaultValue := inputValueDefinition.DefaultValue != -1
					wantType := l.Type(inputValueDefinition.Type)
					if wantType.Kind != document.TypeKindNON_NULL {
						continue
					}
					args := l.ArgumentsIterable(argumentSet)
					for args.Next() {
						argument, _ := args.Value()
						if l.ByteSliceReferenceContentsEquals(argument.Name, inputValueDefinition.Name) && l.ValueIsValid(l.Value(argument.Value), wantType, nil, hasDefaultValue) {
							continue WithNextInputValueDefinition
						}
					}

					if !hasDefaultValue {
						return validation.Invalid(validation.DirectivesHaveRequiredArguments, validation.ArgumentRequired, directive.Position, inputValueDefinition.Name)
					}
				}
			}
		}

		return validation.Valid()
	}
}

func DirectiveArgumentsAreDefined() rules.Rule {
	return func(l *lookup.Lookup, w *lookup.Walker) validation.Result {

		sets := w.DirectiveSetIterable()
		for sets.Next() {
			set, _ := sets.Value()
			directives := l.DirectiveIterable(set)
			for directives.Next() {
				directive, _ := directives.Value()
				definition, exists := l.DirectiveDefinitionByName(directive.Name)
				if !exists {
					return validation.Invalid(validation.DirectivesArgumentsAreDefined, validation.DirectiveNotDefined, directive.Position, directive.Name)
				}
				argumentsDefinition := l.ArgumentsDefinition(definition.ArgumentsDefinition)

				argumentSet := l.ArgumentSet(directive.ArgumentSet)

				args := l.ArgumentsIterable(argumentSet)
				for args.Next() {
					argument, _ := args.Value()

					inputValueDefinition, ok := l.InputValueDefinitionByNameFromDefinitions(argument.Name, argumentsDefinition.InputValueDefinitions)
					if !ok {
						return validation.Invalid(validation.DirectivesArgumentsAreDefined, validation.InputValueNotDefined, argument.Position, argument.Name)
					}

					wantType := l.Type(inputValueDefinition.Type)

					if !l.ValueIsValid(l.Value(argument.Value), wantType, nil, false) {
						return validation.Invalid(validation.DirectivesArgumentsAreDefined, validation.ArgumentValueTypeMismatch, argument.Position, argument.Name)
					}
				}
			}
		}

		return validation.Valid()
	}
}

func DirectiveArgumentsAreConstants() rules.Rule {
	return func(l *lookup.Lookup, w *lookup.Walker) validation.Result {

		sets := w.DirectiveSetIterable()
		for sets.Next() {
			set, _ := sets.Value()
			directives := l.DirectiveIterable(set)
			for directives.Next() {
				directive, _ := directives.Value()
				argumentSet := l.ArgumentSet(directive.ArgumentSet)
				args := l.ArgumentsIterable(argumentSet)
				for args.Next() {
					argument, _ := args.Value()
					value := l.Value(argument.Value)
					if value.ValueType == document.ValueTypeVariable {
						return validation.Invalid(validation.DirectiveArgumentsAreConstants, validation.ValueInvalid, value.Position, argument.Name)
					}
				}
			}
		}

		return validation.Valid()
	}
}

func DirectiveDefinitionDefaultValuesAreOfCorrectType() rules.Rule {
	return func(l *lookup.Lookup, w *lookup.Walker) validation.Result {
		return validation.Valid()
	}
}
