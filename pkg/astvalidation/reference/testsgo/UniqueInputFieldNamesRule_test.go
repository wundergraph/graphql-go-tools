package testsgo

import (
	"testing"
)

func TestUniqueInputFieldNamesRule(t *testing.T) {
	ExpectErrors := func(t *testing.T, schema, queryStr string) ResultCompare {
		return ExpectValidationErrorsWithSchema(t, schema, UniqueInputFieldNamesRule, queryStr)
	}

	ExpectValid := func(t *testing.T, schemaStr, queryStr string) {
		ExpectErrors(t, schemaStr, queryStr)([]Err{})
	}

	t.Run("Validate: Unique input field names", func(t *testing.T) {
		t.Run("input object with fields", func(t *testing.T) {
			ExpectValid(t, `
		input Input {
			f: Boolean
		}

		type Query {
			field(arg: Input): String
        }`, `
      {
        field(arg: { f: true })
      }
    `)
		})

		t.Run("same input object within two args", func(t *testing.T) {
			ExpectValid(t, `
		input Input {
			f: Boolean
		}

		type Query {
			field(arg1: Input, arg2: Input): String
        }`, `
      {
        field(arg1: { f: true }, arg2: { f: true })
      }
    `)
		})

		t.Run("multiple input object fields", func(t *testing.T) {
			ExpectValid(t, `
		input Input {
			f1: String
			f2: String
			f3: String
		}

		type Query {
			field(arg: Input): String
        }`, `
      {
        field(arg: { f1: "value", f2: "value", f3: "value" })
      }
    `)
		})

		t.Run("allows for nested input objects with similar fields", func(t *testing.T) {
			ExpectValid(t, `
		input Nested1 {
			id: ID
			deep: Nested2
		}

		input Nested2 {
			id: ID
		}

		input Input {
			id: ID
			deep: Nested1
		}

		type Query {
			field(arg: Input): String
        }`, `
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
			ExpectErrors(t, `
		input Input {
			f1: String
		}

		type Query {
			field(arg: Input): String
        }`, `
      {
        field(arg: { f1: "value", f1: "value" })
      }
    `)([]Err{
				{
					message: `There can be only one input field named "f1".`,
					locations: []Loc{
						{line: 3, column: 22},
						{line: 3, column: 35},
					},
				},
			})
		})

		t.Run("many duplicate input object fields", func(t *testing.T) {
			ExpectErrors(t, `
		input Input {
			f1: String
		}

		type Query {
			field(arg: Input): String
        }`, `
      {
        field(arg: { f1: "value", f1: "value", f1: "value" })
      }
    `)([]Err{
				{
					message: `There can be only one input field named "f1".`,
					locations: []Loc{
						{line: 3, column: 22},
						{line: 3, column: 35},
					},
				},
				{
					message: `There can be only one input field named "f1".`,
					locations: []Loc{
						{line: 3, column: 22},
						{line: 3, column: 48},
					},
				},
			})
		})

		t.Run("nested duplicate input object fields", func(t *testing.T) {
			ExpectErrors(t, `
		input Nested {
			f2: String
		}

		input Input {
			f1: Nested
		}

		type Query {
			field(arg: Input): String
        }`, `
      {
        field(arg: { f1: {f2: "value", f2: "value" }})
      }
    `)([]Err{
				{
					message: `There can be only one input field named "f2".`,
					locations: []Loc{
						{line: 3, column: 27},
						{line: 3, column: 40},
					},
				},
			})
		})
	})

}
