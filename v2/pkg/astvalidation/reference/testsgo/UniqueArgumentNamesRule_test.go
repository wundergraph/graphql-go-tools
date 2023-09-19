package testsgo

import (
	"testing"
)

func TestUniqueArgumentNamesRule(t *testing.T) {
	t.Skip()

	ExpectErrors := func(t *testing.T, queryStr string) ResultCompare {
		return ExpectValidationErrors(t, UniqueArgumentNamesRule, queryStr)
	}

	ExpectValid := func(t *testing.T, queryStr string) {
		ExpectErrors(t, queryStr)([]Err{})
	}

	t.Run("Validate: Unique argument names", func(t *testing.T) {
		t.Run("no arguments on field", func(t *testing.T) {
			ExpectValid(t, `
      {
        field
      }
    `)
		})

		t.Run("no arguments on directive", func(t *testing.T) {
			ExpectValid(t, `
      {
        field @directive
      }
    `)
		})

		t.Run("argument on field", func(t *testing.T) {
			ExpectValid(t, `
      {
        field(arg: "value")
      }
    `)
		})

		t.Run("argument on directive", func(t *testing.T) {
			ExpectValid(t, `
      {
        field @directive(arg: "value")
      }
    `)
		})

		t.Run("same argument on two fields", func(t *testing.T) {
			ExpectValid(t, `
      {
        one: field(arg: "value")
        two: field(arg: "value")
      }
    `)
		})

		t.Run("same argument on field and directive", func(t *testing.T) {
			ExpectValid(t, `
      {
        field(arg: "value") @directive(arg: "value")
      }
    `)
		})

		t.Run("same argument on two directives", func(t *testing.T) {
			ExpectValid(t, `
      {
        field @directive1(arg: "value") @directive2(arg: "value")
      }
    `)
		})

		t.Run("multiple field arguments", func(t *testing.T) {
			ExpectValid(t, `
      {
        field(arg1: "value", arg2: "value", arg3: "value")
      }
    `)
		})

		t.Run("multiple directive arguments", func(t *testing.T) {
			ExpectValid(t, `
      {
        field @directive(arg1: "value", arg2: "value", arg3: "value")
      }
    `)
		})

		t.Run("duplicate field arguments", func(t *testing.T) {
			ExpectErrors(t, `
      {
        field(arg1: "value", arg1: "value")
      }
    `)([]Err{
				{
					message: `There can be only one argument named "arg1".`,
					locations: []Loc{
						{line: 3, column: 15},
						{line: 3, column: 30},
					},
				},
			})
		})

		t.Run("many duplicate field arguments", func(t *testing.T) {
			ExpectErrors(t, `
      {
        field(arg1: "value", arg1: "value", arg1: "value")
      }
    `)([]Err{
				{
					message: `There can be only one argument named "arg1".`,
					locations: []Loc{
						{line: 3, column: 15},
						{line: 3, column: 30},
					},
				},
				{
					message: `There can be only one argument named "arg1".`,
					locations: []Loc{
						{line: 3, column: 15},
						{line: 3, column: 45},
					},
				},
			})
		})

		t.Run("duplicate directive arguments", func(t *testing.T) {
			ExpectErrors(t, `
      {
        field @directive(arg1: "value", arg1: "value")
      }
    `)([]Err{
				{
					message: `There can be only one argument named "arg1".`,
					locations: []Loc{
						{line: 3, column: 26},
						{line: 3, column: 41},
					},
				},
			})
		})

		t.Run("many duplicate directive arguments", func(t *testing.T) {
			ExpectErrors(t, `
      {
        field @directive(arg1: "value", arg1: "value", arg1: "value")
      }
    `)([]Err{
				{
					message: `There can be only one argument named "arg1".`,
					locations: []Loc{
						{line: 3, column: 26},
						{line: 3, column: 41},
					},
				},
				{
					message: `There can be only one argument named "arg1".`,
					locations: []Loc{
						{line: 3, column: 26},
						{line: 3, column: 56},
					},
				},
			})
		})
	})

}
