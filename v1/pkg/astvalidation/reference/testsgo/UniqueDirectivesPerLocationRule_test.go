package testsgo

import (
	"testing"
)

func TestUniqueDirectivesPerLocationRule(t *testing.T) {

	extensionSDL := `
  directive @directive on FIELD | FRAGMENT_DEFINITION
  directive @directiveA on FIELD | FRAGMENT_DEFINITION
  directive @directiveB on FIELD | FRAGMENT_DEFINITION
  directive @repeatable repeatable on FIELD | FRAGMENT_DEFINITION

  # adding type here to make test queries valid
  type Type {
    field: String!
  }
`
	schemaWithDirectives := ExtendSchema(testSchema, extensionSDL)

	ExpectErrors := func(t *testing.T, queryStr string) ResultCompare {
		return ExpectValidationErrorsWithSchema(t,
			schemaWithDirectives,
			UniqueDirectivesPerLocationRule,
			queryStr,
		)
	}

	ExpectValid := func(t *testing.T, queryStr string) {
		ExpectErrors(t, queryStr)([]Err{})
	}

	ExpectSDLErrors := func(t *testing.T, sdlStr string, schemas ...string) ResultCompare {
		schema := ""
		if len(schemas) > 0 {
			schema = schemas[0]
		}
		return ExpectSDLValidationErrors(t,
			schema,
			UniqueDirectivesPerLocationRule,
			sdlStr,
		)
	}

	t.Run("Validate: Directives Are Unique Per Location", func(t *testing.T) {
		t.Run("no directives", func(t *testing.T) {
			ExpectValid(t, `
      fragment Test on Type {
        field
      }
    `)
		})

		t.Run("unique directives in different locations", func(t *testing.T) {
			ExpectValid(t, `
      fragment Test on Type @directiveA {
        field @directiveB
      }
    `)
		})

		t.Run("unique directives in same locations", func(t *testing.T) {
			ExpectValid(t, `
      fragment Test on Type @directiveA @directiveB {
        field @directiveA @directiveB
      }
    `)
		})

		t.Run("same directives in different locations", func(t *testing.T) {
			ExpectValid(t, `
      fragment Test on Type @directiveA {
        field @directiveA
      }
    `)
		})

		t.Run("same directives in similar locations", func(t *testing.T) {
			ExpectValid(t, `
      fragment Test on Type {
        field @directive
        field @directive
      }
    `)
		})

		t.Run("repeatable directives in same location", func(t *testing.T) {
			ExpectValid(t, `
      fragment Test on Type @repeatable @repeatable {
        field @repeatable @repeatable
      }
    `)
		})

		t.Run("unknown directives must be ignored", func(t *testing.T) {
			ExpectValid(t, `
      type Test @unknown @unknown {
        field: String! @unknown @unknown
      }

      extend type Test @unknown {
        anotherField: String!
      }
    `)
		})

		t.Run("duplicate directives in one location", func(t *testing.T) {
			ExpectErrors(t, `
      fragment Test on Type {
        field @directive @directive
      }
    `)([]Err{
				{
					message: `The directive "@directive" can only be used once at this location.`,
					locations: []Loc{
						{line: 3, column: 15},
						{line: 3, column: 26},
					},
				},
			})
		})

		t.Run("many duplicate directives in one location", func(t *testing.T) {
			ExpectErrors(t, `
      fragment Test on Type {
        field @directive @directive @directive
      }
    `)([]Err{
				{
					message: `The directive "@directive" can only be used once at this location.`,
					locations: []Loc{
						{line: 3, column: 15},
						{line: 3, column: 26},
					},
				},
				{
					message: `The directive "@directive" can only be used once at this location.`,
					locations: []Loc{
						{line: 3, column: 15},
						{line: 3, column: 37},
					},
				},
			})
		})

		t.Run("different duplicate directives in one location", func(t *testing.T) {
			ExpectErrors(t, `
      fragment Test on Type {
        field @directiveA @directiveB @directiveA @directiveB
      }
    `)([]Err{
				{
					message: `The directive "@directiveA" can only be used once at this location.`,
					locations: []Loc{
						{line: 3, column: 15},
						{line: 3, column: 39},
					},
				},
				{
					message: `The directive "@directiveB" can only be used once at this location.`,
					locations: []Loc{
						{line: 3, column: 27},
						{line: 3, column: 51},
					},
				},
			})
		})

		t.Run("duplicate directives in many locations", func(t *testing.T) {
			ExpectErrors(t, `
      fragment Test on Type @directive @directive {
        field @directive @directive
      }
    `)([]Err{
				{
					message: `The directive "@directive" can only be used once at this location.`,
					locations: []Loc{
						{line: 2, column: 29},
						{line: 2, column: 40},
					},
				},
				{
					message: `The directive "@directive" can only be used once at this location.`,
					locations: []Loc{
						{line: 3, column: 15},
						{line: 3, column: 26},
					},
				},
			})
		})

		t.Run("duplicate directives on SDL definitions", func(t *testing.T) {
			ExpectSDLErrors(t, `
      directive @nonRepeatable on
        SCHEMA | SCALAR | OBJECT | INTERFACE | UNION | INPUT_OBJECT

      schema @nonRepeatable @nonRepeatable { query: Dummy }

      scalar TestScalar @nonRepeatable @nonRepeatable
      type TestObject @nonRepeatable @nonRepeatable
      interface TestInterface @nonRepeatable @nonRepeatable
      union TestUnion @nonRepeatable @nonRepeatable
      input TestInput @nonRepeatable @nonRepeatable
    `)([]Err{
				{
					message: `The directive "@nonRepeatable" can only be used once at this location.`,
					locations: []Loc{
						{line: 5, column: 14},
						{line: 5, column: 29},
					},
				},
				{
					message: `The directive "@nonRepeatable" can only be used once at this location.`,
					locations: []Loc{
						{line: 7, column: 25},
						{line: 7, column: 40},
					},
				},
				{
					message: `The directive "@nonRepeatable" can only be used once at this location.`,
					locations: []Loc{
						{line: 8, column: 23},
						{line: 8, column: 38},
					},
				},
				{
					message: `The directive "@nonRepeatable" can only be used once at this location.`,
					locations: []Loc{
						{line: 9, column: 31},
						{line: 9, column: 46},
					},
				},
				{
					message: `The directive "@nonRepeatable" can only be used once at this location.`,
					locations: []Loc{
						{line: 10, column: 23},
						{line: 10, column: 38},
					},
				},
				{
					message: `The directive "@nonRepeatable" can only be used once at this location.`,
					locations: []Loc{
						{line: 11, column: 23},
						{line: 11, column: 38},
					},
				},
			})
		})

		t.Run("duplicate directives on SDL extensions", func(t *testing.T) {
			t.Skip("Parser do not support directives on extensions")

			ExpectSDLErrors(t, `
      directive @nonRepeatable on
        SCHEMA | SCALAR | OBJECT | INTERFACE | UNION | INPUT_OBJECT

      extend schema @nonRepeatable @nonRepeatable

      extend scalar TestScalar @nonRepeatable @nonRepeatable
      extend type TestObject @nonRepeatable @nonRepeatable
      extend interface TestInterface @nonRepeatable @nonRepeatable
      extend union TestUnion @nonRepeatable @nonRepeatable
      extend input TestInput @nonRepeatable @nonRepeatable
    `)([]Err{
				{
					message: `The directive "@nonRepeatable" can only be used once at this location.`,
					locations: []Loc{
						{line: 5, column: 21},
						{line: 5, column: 36},
					},
				},
				{
					message: `The directive "@nonRepeatable" can only be used once at this location.`,
					locations: []Loc{
						{line: 7, column: 32},
						{line: 7, column: 47},
					},
				},
				{
					message: `The directive "@nonRepeatable" can only be used once at this location.`,
					locations: []Loc{
						{line: 8, column: 30},
						{line: 8, column: 45},
					},
				},
				{
					message: `The directive "@nonRepeatable" can only be used once at this location.`,
					locations: []Loc{
						{line: 9, column: 38},
						{line: 9, column: 53},
					},
				},
				{
					message: `The directive "@nonRepeatable" can only be used once at this location.`,
					locations: []Loc{
						{line: 10, column: 30},
						{line: 10, column: 45},
					},
				},
				{
					message: `The directive "@nonRepeatable" can only be used once at this location.`,
					locations: []Loc{
						{line: 11, column: 30},
						{line: 11, column: 45},
					},
				},
			})
		})

		t.Run("duplicate directives between SDL definitions and extensions", func(t *testing.T) {
			t.Skip("Parser do not support directives on extensions")

			ExpectSDLErrors(t, `
      directive @nonRepeatable on SCHEMA

      schema @nonRepeatable { query: Dummy }
      extend schema @nonRepeatable
    `)([]Err{
				{
					message: `The directive "@nonRepeatable" can only be used once at this location.`,
					locations: []Loc{
						{line: 4, column: 14},
						{line: 5, column: 21},
					},
				},
			})

			ExpectSDLErrors(t, `
      directive @nonRepeatable on SCALAR

      scalar TestScalar @nonRepeatable
      extend scalar TestScalar @nonRepeatable
      scalar TestScalar @nonRepeatable
    `)([]Err{
				{
					message: `The directive "@nonRepeatable" can only be used once at this location.`,
					locations: []Loc{
						{line: 4, column: 25},
						{line: 5, column: 32},
					},
				},
				{
					message: `The directive "@nonRepeatable" can only be used once at this location.`,
					locations: []Loc{
						{line: 4, column: 25},
						{line: 6, column: 25},
					},
				},
			})

			ExpectSDLErrors(t, `
      directive @nonRepeatable on OBJECT

      extend type TestObject @nonRepeatable
      type TestObject @nonRepeatable
      extend type TestObject @nonRepeatable
    `)([]Err{
				{
					message: `The directive "@nonRepeatable" can only be used once at this location.`,
					locations: []Loc{
						{line: 4, column: 30},
						{line: 5, column: 23},
					},
				},
				{
					message: `The directive "@nonRepeatable" can only be used once at this location.`,
					locations: []Loc{
						{line: 4, column: 30},
						{line: 6, column: 30},
					},
				},
			})
		})
	})

}
