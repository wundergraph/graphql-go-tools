package testsgo

import (
	"testing"

	"github.com/jensneuse/graphql-go-tools/pkg/astvalidation/reference/helpers"
)

func TestVariablesAreInputTypesRule(t *testing.T) {

	expectErrors := func(queryStr string) helpers.ResultCompare {
		return helpers.ExpectValidationErrors("VariablesAreInputTypesRule", queryStr)
	}

	expectValid := func(queryStr string) {
		expectErrors(queryStr)(`[]`)
	}

	t.Run("Validate: Variables are input types", func(t *testing.T) {
		t.Run("input types are valid", func(t *testing.T) {
			expectValid(`
      query Foo($a: String, $b: [Boolean!]!, $c: ComplexInput) {
        field(a: $a, b: $b, c: $c)
      }
    `)
		})

		t.Run("output types are invalid", func(t *testing.T) {
			expectErrors(`
      query Foo($a: Dog, $b: [[CatOrDog!]]!, $c: Pet) {
        field(a: $a, b: $b, c: $c)
      }
    `)(`[
      {
        locations: [{ line: 2, column: 21 }],
        message: 'Variable "$a" cannot be non-input type "Dog".',
      },
      {
        locations: [{ line: 2, column: 30 }],
        message: 'Variable "$b" cannot be non-input type "[[CatOrDog!]]!".',
      },
      {
        locations: [{ line: 2, column: 50 }],
        message: 'Variable "$c" cannot be non-input type "Pet".',
      },
]`)
		})
	})

}
