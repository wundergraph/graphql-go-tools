package testsgo

import (
	"testing"
)

func TestUniqueFragmentNamesRule(t *testing.T) {
	t.Skip()

	ExpectErrors := func(t *testing.T, queryStr string) ResultCompare {
		return ExpectValidationErrors(t, UniqueFragmentNamesRule, queryStr)
	}

	ExpectValid := func(t *testing.T, queryStr string) {
		ExpectErrors(t, queryStr)([]Err{})
	}

	t.Run("Validate: Unique fragment names", func(t *testing.T) {
		t.Run("no fragments", func(t *testing.T) {
			ExpectValid(t, `
      {
        field
      }
    `)
		})

		t.Run("one fragment", func(t *testing.T) {
			ExpectValid(t, `
      {
        ...fragA
      }

      fragment fragA on Type {
        field
      }
    `)
		})

		t.Run("many fragments", func(t *testing.T) {
			ExpectValid(t, `
      {
        ...fragA
        ...fragB
        ...fragC
      }
      fragment fragA on Type {
        fieldA
      }
      fragment fragB on Type {
        fieldB
      }
      fragment fragC on Type {
        fieldC
      }
    `)
		})

		t.Run("inline fragments are always unique", func(t *testing.T) {
			ExpectValid(t, `
      {
        ...on Type {
          fieldA
        }
        ...on Type {
          fieldB
        }
      }
    `)
		})

		t.Run("fragment and operation named the same", func(t *testing.T) {
			ExpectValid(t, `
      query Foo {
        ...Foo
      }
      fragment Foo on Type {
        field
      }
    `)
		})

		t.Run("fragments named the same", func(t *testing.T) {
			ExpectErrors(t, `
      {
        ...fragA
      }
      fragment fragA on Type {
        fieldA
      }
      fragment fragA on Type {
        fieldB
      }
    `)([]Err{
				{
					message: `There can be only one fragment named "fragA".`,
					locations: []Loc{
						{line: 5, column: 16},
						{line: 8, column: 16},
					},
				},
			})
		})

		t.Run("fragments named the same without being referenced", func(t *testing.T) {
			ExpectErrors(t, `
      fragment fragA on Type {
        fieldA
      }
      fragment fragA on Type {
        fieldB
      }
    `)([]Err{
				{
					message: `There can be only one fragment named "fragA".`,
					locations: []Loc{
						{line: 2, column: 16},
						{line: 5, column: 16},
					},
				},
			})
		})
	})

}
