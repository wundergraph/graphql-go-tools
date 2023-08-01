package testsgo

import (
	"testing"
)

func TestLoneAnonymousOperationRule(t *testing.T) {
	t.Skip()

	ExpectErrors := func(t *testing.T, queryStr string) ResultCompare {
		return ExpectValidationErrors(t, LoneAnonymousOperationRule, queryStr)
	}

	ExpectValid := func(t *testing.T, queryStr string) {
		ExpectErrors(t, queryStr)([]Err{})
	}

	t.Run("Validate: Anonymous operation must be alone", func(t *testing.T) {
		t.Run("no operations", func(t *testing.T) {
			ExpectValid(t, `
      fragment fragA on Type {
        field
      }
    `)
		})

		t.Run("one anon operation", func(t *testing.T) {
			ExpectValid(t, `
      {
        field
      }
    `)
		})

		t.Run("multiple named operations", func(t *testing.T) {
			ExpectValid(t, `
      query Foo {
        field
      }

      query Bar {
        field
      }
    `)
		})

		t.Run("anon operation with fragment", func(t *testing.T) {
			ExpectValid(t, `
      {
        ...Foo
      }
      fragment Foo on Type {
        field
      }
    `)
		})

		t.Run("multiple anon operations", func(t *testing.T) {
			ExpectErrors(t, `
      {
        fieldA
      }
      {
        fieldB
      }
    `)([]Err{
				{
					message:   "This anonymous operation must be the only defined operation.",
					locations: []Loc{{line: 2, column: 7}},
				},
				{
					message:   "This anonymous operation must be the only defined operation.",
					locations: []Loc{{line: 5, column: 7}},
				},
			})
		})

		t.Run("anon operation with a mutation", func(t *testing.T) {
			ExpectErrors(t, `
      {
        fieldA
      }
      mutation Foo {
        fieldB
      }
    `)([]Err{
				{
					message:   "This anonymous operation must be the only defined operation.",
					locations: []Loc{{line: 2, column: 7}},
				},
			})
		})

		t.Run("anon operation with a subscription", func(t *testing.T) {
			ExpectErrors(t, `
      {
        fieldA
      }
      subscription Foo {
        fieldB
      }
    `)([]Err{
				{
					message:   "This anonymous operation must be the only defined operation.",
					locations: []Loc{{line: 2, column: 7}},
				},
			})
		})
	})

}
