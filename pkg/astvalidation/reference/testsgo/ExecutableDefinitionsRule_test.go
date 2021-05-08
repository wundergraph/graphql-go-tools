package testsgo

import (
	"testing"

	"github.com/jensneuse/graphql-go-tools/pkg/astvalidation/reference/helpers"
)

func TestExecutableDefinitionsRule(t *testing.T) {

	expectErrors := func(queryStr string) helpers.ResultCompare {
		return helpers.ExpectValidationErrors("ExecutableDefinitionsRule", queryStr)
	}

	expectValid := func(queryStr string) {
		expectErrors(queryStr)(`[]`)
	}

	t.Run("Validate: Executable definitions", func(t *testing.T) {
		t.Run("with only operation", func(t *testing.T) {
			expectValid(`
      query Foo {
        dog {
          name
        }
      }
    `)
		})

		t.Run("with operation and fragment", func(t *testing.T) {
			expectValid(`
      query Foo {
        dog {
          name
          ...Frag
        }
      }

      fragment Frag on Dog {
        name
      }
    `)
		})

		t.Run("with type definition", func(t *testing.T) {
			expectErrors(`
      query Foo {
        dog {
          name
        }
      }

      type Cow {
        name: String
      }

      extend type Dog {
        color: String
      }
    `)(`[
      {
        message: 'The "Cow" definition is not executable.',
        locations: [{ line: 8, column: 7 }],
      },
      {
        message: 'The "Dog" definition is not executable.',
        locations: [{ line: 12, column: 7 }],
      },
]`)
		})

		t.Run("with schema definition", func(t *testing.T) {
			expectErrors(`
      schema {
        query: Query
      }

      type Query {
        test: String
      }

      extend schema @directive
    `)(`[
      {
        message: "The schema definition is not executable.",
        locations: [{ line: 2, column: 7 }],
      },
      {
        message: 'The "Query" definition is not executable.',
        locations: [{ line: 6, column: 7 }],
      },
      {
        message: "The schema definition is not executable.",
        locations: [{ line: 10, column: 7 }],
      },
]`)
		})
	})

}
