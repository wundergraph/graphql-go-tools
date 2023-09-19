package testsgo

import (
	"testing"
)

func TestNoUnusedVariablesRule(t *testing.T) {
	t.Skip()

	ExpectErrors := func(t *testing.T, queryStr string) ResultCompare {
		return ExpectValidationErrors(t, NoUnusedVariablesRule, queryStr)
	}

	ExpectValid := func(t *testing.T, queryStr string) {
		ExpectErrors(t, queryStr)([]Err{})
	}

	t.Run("Validate: No unused variables", func(t *testing.T) {
		t.Run("uses all variables", func(t *testing.T) {
			ExpectValid(t, `
      query ($a: String, $b: String, $c: String) {
        field(a: $a, b: $b, c: $c)
      }
    `)
		})

		t.Run("uses all variables deeply", func(t *testing.T) {
			ExpectValid(t, `
      query Foo($a: String, $b: String, $c: String) {
        field(a: $a) {
          field(b: $b) {
            field(c: $c)
          }
        }
      }
    `)
		})

		t.Run("uses all variables deeply in inline fragments", func(t *testing.T) {
			ExpectValid(t, `
      query Foo($a: String, $b: String, $c: String) {
        ... on Type {
          field(a: $a) {
            field(b: $b) {
              ... on Type {
                field(c: $c)
              }
            }
          }
        }
      }
    `)
		})

		t.Run("uses all variables in fragments", func(t *testing.T) {
			ExpectValid(t, `
      query Foo($a: String, $b: String, $c: String) {
        ...FragA
      }
      fragment FragA on Type {
        field(a: $a) {
          ...FragB
        }
      }
      fragment FragB on Type {
        field(b: $b) {
          ...FragC
        }
      }
      fragment FragC on Type {
        field(c: $c)
      }
    `)
		})

		t.Run("variable used by fragment in multiple operations", func(t *testing.T) {
			ExpectValid(t, `
      query Foo($a: String) {
        ...FragA
      }
      query Bar($b: String) {
        ...FragB
      }
      fragment FragA on Type {
        field(a: $a)
      }
      fragment FragB on Type {
        field(b: $b)
      }
    `)
		})

		t.Run("variable used by recursive fragment", func(t *testing.T) {
			ExpectValid(t, `
      query Foo($a: String) {
        ...FragA
      }
      fragment FragA on Type {
        field(a: $a) {
          ...FragA
        }
      }
    `)
		})

		t.Run("variable not used", func(t *testing.T) {
			ExpectErrors(t, `
      query ($a: String, $b: String, $c: String) {
        field(a: $a, b: $b)
      }
    `)([]Err{
				{
					message:   `Variable "$c" is never used.`,
					locations: []Loc{{line: 2, column: 38}},
				},
			})
		})

		t.Run("multiple variables not used", func(t *testing.T) {
			ExpectErrors(t, `
      query Foo($a: String, $b: String, $c: String) {
        field(b: $b)
      }
    `)([]Err{
				{
					message:   `Variable "$a" is never used in operation "Foo".`,
					locations: []Loc{{line: 2, column: 17}},
				},
				{
					message:   `Variable "$c" is never used in operation "Foo".`,
					locations: []Loc{{line: 2, column: 41}},
				},
			})
		})

		t.Run("variable not used in fragments", func(t *testing.T) {
			ExpectErrors(t, `
      query Foo($a: String, $b: String, $c: String) {
        ...FragA
      }
      fragment FragA on Type {
        field(a: $a) {
          ...FragB
        }
      }
      fragment FragB on Type {
        field(b: $b) {
          ...FragC
        }
      }
      fragment FragC on Type {
        field
      }
    `)([]Err{
				{
					message:   `Variable "$c" is never used in operation "Foo".`,
					locations: []Loc{{line: 2, column: 41}},
				},
			})
		})

		t.Run("multiple variables not used in fragments", func(t *testing.T) {
			ExpectErrors(t, `
      query Foo($a: String, $b: String, $c: String) {
        ...FragA
      }
      fragment FragA on Type {
        field {
          ...FragB
        }
      }
      fragment FragB on Type {
        field(b: $b) {
          ...FragC
        }
      }
      fragment FragC on Type {
        field
      }
    `)([]Err{
				{
					message:   `Variable "$a" is never used in operation "Foo".`,
					locations: []Loc{{line: 2, column: 17}},
				},
				{
					message:   `Variable "$c" is never used in operation "Foo".`,
					locations: []Loc{{line: 2, column: 41}},
				},
			})
		})

		t.Run("variable not used by unreferenced fragment", func(t *testing.T) {
			ExpectErrors(t, `
      query Foo($b: String) {
        ...FragA
      }
      fragment FragA on Type {
        field(a: $a)
      }
      fragment FragB on Type {
        field(b: $b)
      }
    `)([]Err{
				{
					message:   `Variable "$b" is never used in operation "Foo".`,
					locations: []Loc{{line: 2, column: 17}},
				},
			})
		})

		t.Run("variable not used by fragment used by other operation", func(t *testing.T) {
			ExpectErrors(t, `
      query Foo($b: String) {
        ...FragA
      }
      query Bar($a: String) {
        ...FragB
      }
      fragment FragA on Type {
        field(a: $a)
      }
      fragment FragB on Type {
        field(b: $b)
      }
    `)([]Err{
				{
					message:   `Variable "$b" is never used in operation "Foo".`,
					locations: []Loc{{line: 2, column: 17}},
				},
				{
					message:   `Variable "$a" is never used in operation "Bar".`,
					locations: []Loc{{line: 5, column: 17}},
				},
			})
		})
	})

}
