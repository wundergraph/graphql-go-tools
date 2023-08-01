package testsgo

import (
	"testing"
)

func TestScalarLeafsRule(t *testing.T) {
	t.Skip()

	ExpectErrors := func(t *testing.T, queryStr string) ResultCompare {
		return ExpectValidationErrors(t, ScalarLeafsRule, queryStr)
	}

	ExpectValid := func(t *testing.T, queryStr string) {
		ExpectErrors(t, queryStr)([]Err{})
	}

	t.Run("Validate: Scalar leafs", func(t *testing.T) {
		t.Run("valid scalar selection", func(t *testing.T) {
			ExpectValid(t, `
      fragment scalarSelection on Dog {
        barks
      }
    `)
		})

		t.Run("object type missing selection", func(t *testing.T) {
			ExpectErrors(t, `
      query directQueryOnObjectWithoutSubFields {
        human
      }
    `)([]Err{
				{
					message:   `Field "human" of type "Human" must have a selection of subfields. Did you mean "human { ... }"?`,
					locations: []Loc{{line: 3, column: 9}},
				},
			})
		})

		t.Run("interface type missing selection", func(t *testing.T) {
			ExpectErrors(t, `
      {
        human { pets }
      }
    `)([]Err{
				{
					message:   `Field "pets" of type "[Pet]" must have a selection of subfields. Did you mean "pets { ... }"?`,
					locations: []Loc{{line: 3, column: 17}},
				},
			})
		})

		t.Run("valid scalar selection with args", func(t *testing.T) {
			ExpectValid(t, `
      fragment scalarSelectionWithArgs on Dog {
        doesKnowCommand(dogCommand: SIT)
      }
    `)
		})

		t.Run("scalar selection not allowed on Boolean", func(t *testing.T) {
			ExpectErrors(t, `
      fragment scalarSelectionsNotAllowedOnBoolean on Dog {
        barks { sinceWhen }
      }
    `)([]Err{
				{
					message:   `Field "barks" must not have a selection since type "Boolean" has no subfields.`,
					locations: []Loc{{line: 3, column: 15}},
				},
			})
		})

		t.Run("scalar selection not allowed on Enum", func(t *testing.T) {
			ExpectErrors(t, `
      fragment scalarSelectionsNotAllowedOnEnum on Cat {
        furColor { inHexDec }
      }
    `)([]Err{
				{
					message:   `Field "furColor" must not have a selection since type "FurColor" has no subfields.`,
					locations: []Loc{{line: 3, column: 18}},
				},
			})
		})

		t.Run("scalar selection not allowed with args", func(t *testing.T) {
			ExpectErrors(t, `
      fragment scalarSelectionsNotAllowedWithArgs on Dog {
        doesKnowCommand(dogCommand: SIT) { sinceWhen }
      }
    `)([]Err{
				{
					message:   `Field "doesKnowCommand" must not have a selection since type "Boolean" has no subfields.`,
					locations: []Loc{{line: 3, column: 42}},
				},
			})
		})

		t.Run("Scalar selection not allowed with directives", func(t *testing.T) {
			ExpectErrors(t, `
      fragment scalarSelectionsNotAllowedWithDirectives on Dog {
        name @include(if: true) { isAlsoHumanName }
      }
    `)([]Err{
				{
					message:   `Field "name" must not have a selection since type "String" has no subfields.`,
					locations: []Loc{{line: 3, column: 33}},
				},
			})
		})

		t.Run("Scalar selection not allowed with directives and args", func(t *testing.T) {
			ExpectErrors(t, `
      fragment scalarSelectionsNotAllowedWithDirectivesAndArgs on Dog {
        doesKnowCommand(dogCommand: SIT) @include(if: true) { sinceWhen }
      }
    `)([]Err{
				{
					message:   `Field "doesKnowCommand" must not have a selection since type "Boolean" has no subfields.`,
					locations: []Loc{{line: 3, column: 61}},
				},
			})
		})
	})

}
