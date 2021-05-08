package testsgo

import (
	"testing"

	"github.com/jensneuse/graphql-go-tools/pkg/astvalidation/reference/helpers"
)

func TestScalarLeafsRule(t *testing.T) {

	expectErrors := func(queryStr string) helpers.ResultCompare {
		return helpers.ExpectValidationErrors("ScalarLeafsRule", queryStr)
	}

	expectValid := func(queryStr string) {
		expectErrors(queryStr)(`[]`)
	}

	t.Run("Validate: Scalar leafs", func(t *testing.T) {
		t.Run("valid scalar selection", func(t *testing.T) {
			expectValid(`
      fragment scalarSelection on Dog {
        barks
      }
    `)
		})

		t.Run("object type missing selection", func(t *testing.T) {
			expectErrors(`
      query directQueryOnObjectWithoutSubFields {
        human
      }
    `)(`[
      {
        message:
          'Field "human" of type "Human" must have a selection of subfields. Did you mean "human { ... }"?',
        locations: [{ line: 3, column: 9 }],
      },
]`)
		})

		t.Run("interface type missing selection", func(t *testing.T) {
			expectErrors(`
      {
        human { pets }
      }
    `)(`[
      {
        message:
          'Field "pets" of type "[Pet]" must have a selection of subfields. Did you mean "pets { ... }"?',
        locations: [{ line: 3, column: 17 }],
      },
]`)
		})

		t.Run("valid scalar selection with args", func(t *testing.T) {
			expectValid(`
      fragment scalarSelectionWithArgs on Dog {
        doesKnowCommand(dogCommand: SIT)
      }
    `)
		})

		t.Run("scalar selection not allowed on Boolean", func(t *testing.T) {
			expectErrors(`
      fragment scalarSelectionsNotAllowedOnBoolean on Dog {
        barks { sinceWhen }
      }
    `)(`[
      {
        message:
          'Field "barks" must not have a selection since type "Boolean" has no subfields.',
        locations: [{ line: 3, column: 15 }],
      },
]`)
		})

		t.Run("scalar selection not allowed on Enum", func(t *testing.T) {
			expectErrors(`
      fragment scalarSelectionsNotAllowedOnEnum on Cat {
        furColor { inHexDec }
      }
    `)(`[
      {
        message:
          'Field "furColor" must not have a selection since type "FurColor" has no subfields.',
        locations: [{ line: 3, column: 18 }],
      },
]`)
		})

		t.Run("scalar selection not allowed with args", func(t *testing.T) {
			expectErrors(`
      fragment scalarSelectionsNotAllowedWithArgs on Dog {
        doesKnowCommand(dogCommand: SIT) { sinceWhen }
      }
    `)(`[
      {
        message:
          'Field "doesKnowCommand" must not have a selection since type "Boolean" has no subfields.',
        locations: [{ line: 3, column: 42 }],
      },
]`)
		})

		t.Run("Scalar selection not allowed with directives", func(t *testing.T) {
			expectErrors(`
      fragment scalarSelectionsNotAllowedWithDirectives on Dog {
        name @include(if: true) { isAlsoHumanName }
      }
    `)(`[
      {
        message:
          'Field "name" must not have a selection since type "String" has no subfields.',
        locations: [{ line: 3, column: 33 }],
      },
]`)
		})

		t.Run("Scalar selection not allowed with directives and args", func(t *testing.T) {
			expectErrors(`
      fragment scalarSelectionsNotAllowedWithDirectivesAndArgs on Dog {
        doesKnowCommand(dogCommand: SIT) @include(if: true) { sinceWhen }
      }
    `)(`[
      {
        message:
          'Field "doesKnowCommand" must not have a selection since type "Boolean" has no subfields.',
        locations: [{ line: 3, column: 61 }],
      },
]`)
		})
	})

}
