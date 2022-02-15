package testsgo

import (
	"testing"
)

func TestUniqueVariableNamesRule(t *testing.T) {
	t.Skip()

	ExpectErrors := func(t *testing.T, queryStr string) ResultCompare {
		return ExpectValidationErrors(t, UniqueVariableNamesRule, queryStr)
	}

	ExpectValid := func(t *testing.T, queryStr string) {
		ExpectErrors(t, queryStr)([]Err{})
	}

	t.Run("Validate: Unique variable names", func(t *testing.T) {
		t.Run("unique variable names", func(t *testing.T) {
			ExpectValid(t, `
      query A($x: Int, $y: String) { __typename }
      query B($x: String, $y: Int) { __typename }
    `)
		})

		t.Run("duplicate variable names", func(t *testing.T) {
			ExpectErrors(t, `
      query A($x: Int, $x: Int, $x: String) { __typename }
      query B($x: String, $x: Int) { __typename }
      query C($x: Int, $x: Int) { __typename }
    `)([]Err{
				{
					message: `There can be only one variable named "$x".`,
					locations: []Loc{
						{line: 2, column: 16},
						{line: 2, column: 25},
					},
				},
				{
					message: `There can be only one variable named "$x".`,
					locations: []Loc{
						{line: 2, column: 16},
						{line: 2, column: 34},
					},
				},
				{
					message: `There can be only one variable named "$x".`,
					locations: []Loc{
						{line: 3, column: 16},
						{line: 3, column: 28},
					},
				},
				{
					message: `There can be only one variable named "$x".`,
					locations: []Loc{
						{line: 4, column: 16},
						{line: 4, column: 25},
					},
				},
			})
		})
	})

}
