package testsgo

import (
	"testing"

	"github.com/jensneuse/graphql-go-tools/pkg/astvalidation/reference/helpers"
)

func TestLoneAnonymousOperationRule(t *testing.T) {

	expectErrors := func(queryStr string) helpers.ResultCompare {
		return helpers.ExpectValidationErrors("LoneAnonymousOperationRule", queryStr)
	}

	expectValid := func(queryStr string) {
		expectErrors(queryStr)(`[]`)
	}

	t.Run("Validate: Anonymous operation must be alone", func(t *testing.T) {
		t.Run("no operations", func(t *testing.T) {
			expectValid(`
      fragment fragA on Type {
        field
      }
    `)
		})

		t.Run("one anon operation", func(t *testing.T) {
			expectValid(`
      {
        field
      }
    `)
		})

		t.Run("multiple named operations", func(t *testing.T) {
			expectValid(`
      query Foo {
        field
      }

      query Bar {
        field
      }
    `)
		})

		t.Run("anon operation with fragment", func(t *testing.T) {
			expectValid(`
      {
        ...Foo
      }
      fragment Foo on Type {
        field
      }
    `)
		})

		t.Run("multiple anon operations", func(t *testing.T) {
			expectErrors(`
      {
        fieldA
      }
      {
        fieldB
      }
    `)(`[
      {
        message: "This anonymous operation must be the only defined operation.",
        locations: [{ line: 2, column: 7 }],
      },
      {
        message: "This anonymous operation must be the only defined operation.",
        locations: [{ line: 5, column: 7 }],
      },
]`)
		})

		t.Run("anon operation with a mutation", func(t *testing.T) {
			expectErrors(`
      {
        fieldA
      }
      mutation Foo {
        fieldB
      }
    `)(`[
      {
        message: "This anonymous operation must be the only defined operation.",
        locations: [{ line: 2, column: 7 }],
      },
]`)
		})

		t.Run("anon operation with a subscription", func(t *testing.T) {
			expectErrors(`
      {
        fieldA
      }
      subscription Foo {
        fieldB
      }
    `)(`[
      {
        message: "This anonymous operation must be the only defined operation.",
        locations: [{ line: 2, column: 7 }],
      },
]`)
		})
	})

}
