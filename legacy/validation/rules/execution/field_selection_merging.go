package execution

import (
	"github.com/jensneuse/graphql-go-tools/legacy/document"
	"github.com/jensneuse/graphql-go-tools/pkg/lookup"
	"github.com/jensneuse/graphql-go-tools/pkg/validation"
	"github.com/jensneuse/graphql-go-tools/pkg/validation/rules"
)

// FieldSelectionMerging
// https://facebook.github.io/graphql/draft/#sec-Field-Selection-Merging
func FieldSelectionMerging() rules.Rule {

	return func(l *lookup.Lookup, w *lookup.Walker) validation.Result {

		validateSet := func(set document.SelectionSet, typeName document.ByteSliceReference) validation.Result {
			lefts := l.SelectionSetCollectedFields(set, typeName)
			for lefts.Next() {
				_, left := lefts.Value()
				rights := l.SelectionSetCollectedFields(set, typeName)
				for rights.Next() {
					_, right := rights.Value()
					if !l.FieldResponseNamesAreEqual(left, right) {
						continue
					}

					if !l.FieldsDeepEqual(left, right) {
						return validation.Invalid(validation.FieldSelectionMerging, validation.FieldNameOrAliasMismatch, right.Position, right.Name)
					}
				}
			}
			return validation.Valid()
		}

		sets := w.SelectionSetIterable()
		for sets.Next() {

			set, nodeRef, _, _ := sets.Value()
			typeName := w.SelectionSetTypeName(set, nodeRef)

			if result := validateSet(set, typeName); !result.Valid {
				return result
			}

			leftSets := l.SelectionSetDifferingSelectionSetIterator(set, typeName)
			for leftSets.Next() {
				left := leftSets.Value()
				rightSets := l.SelectionSetDifferingSelectionSetIterator(set, typeName)
				for rightSets.Next() {
					right := rightSets.Value()
					if left.SetRef == right.SetRef {
						continue
					}
					if !l.SelectionSetsAreOfSameResponseShape(left, right) {
						return validation.Invalid(validation.FieldSelectionMerging, validation.SelectionSetResponseShapesCannotMerge, set.Position, typeName)
					}
				}
			}
		}

		return validation.Valid()
	}
}
