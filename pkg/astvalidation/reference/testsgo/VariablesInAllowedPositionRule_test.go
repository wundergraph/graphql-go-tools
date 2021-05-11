package testsgo

import (
	"testing"
)

func TestVariablesInAllowedPositionRule(t *testing.T) {

	expectErrors := func(queryStr string) ResultCompare {
		return ExpectValidationErrors("VariablesInAllowedPositionRule", queryStr)
	}

	expectValid := func(queryStr string) {
		expectErrors(queryStr)(t, []Err{})
	}

	t.Run("Validate: Variables are in allowed positions", func(t *testing.T) {
		t.Run("Boolean => Boolean", func(t *testing.T) {
			expectValid(`
      query Query($booleanArg: Boolean)
      {
        complicatedArgs {
          booleanArgField(booleanArg: $booleanArg)
        }
      }
    `)
		})

		t.Run("Boolean => Boolean within fragment", func(t *testing.T) {
			expectValid(`
      fragment booleanArgFrag on ComplicatedArgs {
        booleanArgField(booleanArg: $booleanArg)
      }
      query Query($booleanArg: Boolean)
      {
        complicatedArgs {
          ...booleanArgFrag
        }
      }
    `)

			expectValid(`
      query Query($booleanArg: Boolean)
      {
        complicatedArgs {
          ...booleanArgFrag
        }
      }
      fragment booleanArgFrag on ComplicatedArgs {
        booleanArgField(booleanArg: $booleanArg)
      }
    `)
		})

		t.Run("Boolean! => Boolean", func(t *testing.T) {
			expectValid(`
      query Query($nonNullBooleanArg: Boolean!)
      {
        complicatedArgs {
          booleanArgField(booleanArg: $nonNullBooleanArg)
        }
      }
    `)
		})

		t.Run("Boolean! => Boolean within fragment", func(t *testing.T) {
			expectValid(`
      fragment booleanArgFrag on ComplicatedArgs {
        booleanArgField(booleanArg: $nonNullBooleanArg)
      }

      query Query($nonNullBooleanArg: Boolean!)
      {
        complicatedArgs {
          ...booleanArgFrag
        }
      }
    `)
		})

		t.Run("[String] => [String]", func(t *testing.T) {
			expectValid(`
      query Query($stringListVar: [String])
      {
        complicatedArgs {
          stringListArgField(stringListArg: $stringListVar)
        }
      }
    `)
		})

		t.Run("[String!] => [String]", func(t *testing.T) {
			expectValid(`
      query Query($stringListVar: [String!])
      {
        complicatedArgs {
          stringListArgField(stringListArg: $stringListVar)
        }
      }
    `)
		})

		t.Run("String => [String] in item position", func(t *testing.T) {
			expectValid(`
      query Query($stringVar: String)
      {
        complicatedArgs {
          stringListArgField(stringListArg: [$stringVar])
        }
      }
    `)
		})

		t.Run("String! => [String] in item position", func(t *testing.T) {
			expectValid(`
      query Query($stringVar: String!)
      {
        complicatedArgs {
          stringListArgField(stringListArg: [$stringVar])
        }
      }
    `)
		})

		t.Run("ComplexInput => ComplexInput", func(t *testing.T) {
			expectValid(`
      query Query($complexVar: ComplexInput)
      {
        complicatedArgs {
          complexArgField(complexArg: $complexVar)
        }
      }
    `)
		})

		t.Run("ComplexInput => ComplexInput in field position", func(t *testing.T) {
			expectValid(`
      query Query($boolVar: Boolean = false)
      {
        complicatedArgs {
          complexArgField(complexArg: {requiredArg: $boolVar})
        }
      }
    `)
		})

		t.Run("Boolean! => Boolean! in directive", func(t *testing.T) {
			expectValid(`
      query Query($boolVar: Boolean!)
      {
        dog @include(if: $boolVar)
      }
    `)
		})

		t.Run("Int => Int!", func(t *testing.T) {
			expectErrors(`
      query Query($intArg: Int) {
        complicatedArgs {
          nonNullIntArgField(nonNullIntArg: $intArg)
        }
      }
    `)(t, []Err{
				{
					message: `Variable "$intArg" of type "Int" used in position expecting type "Int!".`,
					locations: []Loc{
						{line: 2, column: 19},
						{line: 4, column: 45},
					},
				},
			})
		})

		t.Run("Int => Int! within fragment", func(t *testing.T) {
			expectErrors(`
      fragment nonNullIntArgFieldFrag on ComplicatedArgs {
        nonNullIntArgField(nonNullIntArg: $intArg)
      }

      query Query($intArg: Int) {
        complicatedArgs {
          ...nonNullIntArgFieldFrag
        }
      }
    `)(t, []Err{
				{
					message: `Variable "$intArg" of type "Int" used in position expecting type "Int!".`,
					locations: []Loc{
						{line: 6, column: 19},
						{line: 3, column: 43},
					},
				},
			})
		})

		t.Run("Int => Int! within nested fragment", func(t *testing.T) {
			expectErrors(`
      fragment outerFrag on ComplicatedArgs {
        ...nonNullIntArgFieldFrag
      }

      fragment nonNullIntArgFieldFrag on ComplicatedArgs {
        nonNullIntArgField(nonNullIntArg: $intArg)
      }

      query Query($intArg: Int) {
        complicatedArgs {
          ...outerFrag
        }
      }
    `)(t, []Err{
				{
					message: `Variable "$intArg" of type "Int" used in position expecting type "Int!".`,
					locations: []Loc{
						{line: 10, column: 19},
						{line: 7, column: 43},
					},
				},
			})
		})

		t.Run("String over Boolean", func(t *testing.T) {
			expectErrors(`
      query Query($stringVar: String) {
        complicatedArgs {
          booleanArgField(booleanArg: $stringVar)
        }
      }
    `)(t, []Err{
				{
					message: `Variable "$stringVar" of type "String" used in position expecting type "Boolean".`,
					locations: []Loc{
						{line: 2, column: 19},
						{line: 4, column: 39},
					},
				},
			})
		})

		t.Run("String => [String]", func(t *testing.T) {
			expectErrors(`
      query Query($stringVar: String) {
        complicatedArgs {
          stringListArgField(stringListArg: $stringVar)
        }
      }
    `)(t, []Err{
				{
					message: `Variable "$stringVar" of type "String" used in position expecting type "[String]".`,
					locations: []Loc{
						{line: 2, column: 19},
						{line: 4, column: 45},
					},
				},
			})
		})

		t.Run("Boolean => Boolean! in directive", func(t *testing.T) {
			expectErrors(`
      query Query($boolVar: Boolean) {
        dog @include(if: $boolVar)
      }
    `)(t, []Err{
				{
					message: `Variable "$boolVar" of type "Boolean" used in position expecting type "Boolean!".`,
					locations: []Loc{
						{line: 2, column: 19},
						{line: 3, column: 26},
					},
				},
			})
		})

		t.Run("String => Boolean! in directive", func(t *testing.T) {
			expectErrors(`
      query Query($stringVar: String) {
        dog @include(if: $stringVar)
      }
    `)(t, []Err{
				{
					message: `Variable "$stringVar" of type "String" used in position expecting type "Boolean!".`,
					locations: []Loc{
						{line: 2, column: 19},
						{line: 3, column: 26},
					},
				},
			})
		})

		t.Run("[String] => [String!]", func(t *testing.T) {
			expectErrors(`
      query Query($stringListVar: [String])
      {
        complicatedArgs {
          stringListNonNullArgField(stringListNonNullArg: $stringListVar)
        }
      }
    `)(t, []Err{
				{
					message: `Variable "$stringListVar" of type "[String]" used in position expecting type "[String!]".`,
					locations: []Loc{
						{line: 2, column: 19},
						{line: 5, column: 59},
					},
				},
			})
		})

		t.Run("Allows optional (nullable) variables with default values", func(t *testing.T) {
			t.Run("Int => Int! fails when variable provides null default value", func(t *testing.T) {
				expectErrors(`
        query Query($intVar: Int = null) {
          complicatedArgs {
            nonNullIntArgField(nonNullIntArg: $intVar)
          }
        }
      `)(t, []Err{
					{
						message: `Variable "$intVar" of type "Int" used in position expecting type "Int!".`,
						locations: []Loc{
							{line: 2, column: 21},
							{line: 4, column: 47},
						},
					},
				})
			})

			t.Run("Int => Int! when variable provides non-null default value", func(t *testing.T) {
				expectValid(`
        query Query($intVar: Int = 1) {
          complicatedArgs {
            nonNullIntArgField(nonNullIntArg: $intVar)
          }
        }`)
			})

			t.Run("Int => Int! when optional argument provides default value", func(t *testing.T) {
				expectValid(`
        query Query($intVar: Int) {
          complicatedArgs {
            nonNullFieldWithDefault(nonNullIntArg: $intVar)
          }
        }`)
			})

			t.Run("Boolean => Boolean! in directive with default value with option", func(t *testing.T) {
				expectValid(`
        query Query($boolVar: Boolean = false) {
          dog @include(if: $boolVar)
        }`)
			})
		})
	})

}
