package testsgo

import (
	"testing"

	"github.com/jensneuse/graphql-go-tools/pkg/astvalidation/reference/helpers"
)

func TestUniqueInputFieldNamesRule(t *testing.T) {

	expectErrors := func(queryStr string) helpers.ResultCompare {
		return helpers.ExpectValidationErrors("UniqueInputFieldNamesRule", queryStr)
	}

	expectValid := func(queryStr string) {
		expectErrors(queryStr)(`[]`)
	}

	t.Run("Validate: Unique input field names", func(t *testing.T) {
		t.Run("input object with fields", func(t *testing.T) {
			expectValid(`
      {
        field(arg: { f: true })
      }
    `)
		})

		t.Run("same input object within two args", func(t *testing.T) {
			expectValid(`
      {
        field(arg1: { f: true }, arg2: { f: true })
      }
    `)
		})

		t.Run("multiple input object fields", func(t *testing.T) {
			expectValid(`
      {
        field(arg: { f1: "value", f2: "value", f3: "value" })
      }
    `)
		})

		t.Run("allows for nested input objects with similar fields", func(t *testing.T) {
			expectValid(`
      {
        field(arg: {
          deep: {
            deep: {
              id: 1
            }
            id: 1
          }
          id: 1
        })
      }
    `)
		})

		t.Run("duplicate input object fields", func(t *testing.T) {
			expectErrors(`
      {
        field(arg: { f1: "value", f1: "value" })
      }
    `)(`[
      {
        message: 'There can be only one input field named "f1".',
        locations: [
          { line: 3, column: 22 },
          { line: 3, column: 35 },
        ],
      },
]`)
		})

		t.Run("many duplicate input object fields", func(t *testing.T) {
			expectErrors(`
      {
        field(arg: { f1: "value", f1: "value", f1: "value" })
      }
    `)(`[
      {
        message: 'There can be only one input field named "f1".',
        locations: [
          { line: 3, column: 22 },
          { line: 3, column: 35 },
        ],
      },
      {
        message: 'There can be only one input field named "f1".',
        locations: [
          { line: 3, column: 22 },
          { line: 3, column: 48 },
        ],
      },
]`)
		})

		t.Run("nested duplicate input object fields", func(t *testing.T) {
			expectErrors(`
      {
        field(arg: { f1: {f2: "value", f2: "value" }})
      }
    `)(`[
      {
        message: 'There can be only one input field named "f2".',
        locations: [
          { line: 3, column: 27 },
          { line: 3, column: 40 },
        ],
      },
]`)
		})
	})

}
