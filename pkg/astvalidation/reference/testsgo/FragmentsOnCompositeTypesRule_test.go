package testsgo

import (
	"testing"

	"github.com/jensneuse/graphql-go-tools/pkg/astvalidation/reference/helpers"
)

func TestFragmentsOnCompositeTypesRule(t *testing.T) {

	expectErrors := func(queryStr string) helpers.ResultCompare {
		return helpers.ExpectValidationErrors("FragmentsOnCompositeTypesRule", queryStr)
	}

	expectValid := func(queryStr string) {
		expectErrors(queryStr)(`[]`)
	}

	t.Run("Validate: Fragments on composite types", func(t *testing.T) {
		t.Run("object is valid fragment type", func(t *testing.T) {
			expectValid(`
      fragment validFragment on Dog {
        barks
      }
    `)
		})

		t.Run("interface is valid fragment type", func(t *testing.T) {
			expectValid(`
      fragment validFragment on Pet {
        name
      }
    `)
		})

		t.Run("object is valid inline fragment type", func(t *testing.T) {
			expectValid(`
      fragment validFragment on Pet {
        ... on Dog {
          barks
        }
      }
    `)
		})

		t.Run("interface is valid inline fragment type", func(t *testing.T) {
			expectValid(`
      fragment validFragment on Mammal {
        ... on Canine {
          name
        }
      }
    `)
		})

		t.Run("inline fragment without type is valid", func(t *testing.T) {
			expectValid(`
      fragment validFragment on Pet {
        ... {
          name
        }
      }
    `)
		})

		t.Run("union is valid fragment type", func(t *testing.T) {
			expectValid(`
      fragment validFragment on CatOrDog {
        __typename
      }
    `)
		})

		t.Run("scalar is invalid fragment type", func(t *testing.T) {
			expectErrors(`
      fragment scalarFragment on Boolean {
        bad
      }
    `)(`[
      {
        message:
          'Fragment "scalarFragment" cannot condition on non composite type "Boolean".',
        locations: [{ line: 2, column: 34 }],
      },
]`)
		})

		t.Run("enum is invalid fragment type", func(t *testing.T) {
			expectErrors(`
      fragment scalarFragment on FurColor {
        bad
      }
    `)(`[
      {
        message:
          'Fragment "scalarFragment" cannot condition on non composite type "FurColor".',
        locations: [{ line: 2, column: 34 }],
      },
]`)
		})

		t.Run("input object is invalid fragment type", func(t *testing.T) {
			expectErrors(`
      fragment inputFragment on ComplexInput {
        stringField
      }
    `)(`[
      {
        message:
          'Fragment "inputFragment" cannot condition on non composite type "ComplexInput".',
        locations: [{ line: 2, column: 33 }],
      },
]`)
		})

		t.Run("scalar is invalid inline fragment type", func(t *testing.T) {
			expectErrors(`
      fragment invalidFragment on Pet {
        ... on String {
          barks
        }
      }
    `)(`[
      {
        message: 'Fragment cannot condition on non composite type "String".',
        locations: [{ line: 3, column: 16 }],
      },
]`)
		})
	})

}
