package testsgo

import (
	"testing"
)

func TestUniqueOperationNamesRule(t *testing.T) {

	expectErrors := func(queryStr string) ResultCompare {
		return ExpectValidationErrors("UniqueOperationNamesRule", queryStr)
	}

	expectValid := func(queryStr string) {
		expectErrors(queryStr)([]Err{})
	}

	t.Run("Validate: Unique operation names", func(t *testing.T) {
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

		t.Run("one named operation", func(t *testing.T) {
			expectValid(`
      query Foo {
        field
      }
    `)
		})

		t.Run("multiple operations", func(t *testing.T) {
			expectValid(`
      query Foo {
        field
      }

      query Bar {
        field
      }
    `)
		})

		t.Run("multiple operations of different types", func(t *testing.T) {
			expectValid(`
      query Foo {
        field
      }

      mutation Bar {
        field
      }

      subscription Baz {
        field
      }
    `)
		})

		t.Run("fragment and operation named the same", func(t *testing.T) {
			expectValid(`
      query Foo {
        ...Foo
      }
      fragment Foo on Type {
        field
      }
    `)
		})

		t.Run("multiple operations of same name", func(t *testing.T) {
			expectErrors(`
      query Foo {
        fieldA
      }
      query Foo {
        fieldB
      }
    `)([]Err{
				{
					message: `There can be only one operation named "Foo".`,
					locations: []Loc{
						{line: 2, column: 13},
						{line: 5, column: 13},
					},
				},
			})
		})

		t.Run("multiple ops of same name of different types (mutation)", func(t *testing.T) {
			expectErrors(`
      query Foo {
        fieldA
      }
      mutation Foo {
        fieldB
      }
    `)([]Err{
				{
					message: `There can be only one operation named "Foo".`,
					locations: []Loc{
						{line: 2, column: 13},
						{line: 5, column: 16},
					},
				},
			})
		})

		t.Run("multiple ops of same name of different types (subscription)", func(t *testing.T) {
			expectErrors(`
      query Foo {
        fieldA
      }
      subscription Foo {
        fieldB
      }
    `)([]Err{
				{
					message: `There can be only one operation named "Foo".`,
					locations: []Loc{
						{line: 2, column: 13},
						{line: 5, column: 20},
					},
				},
			})
		})
	})

}
