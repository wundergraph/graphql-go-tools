package testsgo

import (
	"testing"
)

func TestLoneSchemaDefinitionRule(t *testing.T) {
	t.Skip()

	ExpectSDLErrors := func(t *testing.T, sdlStr string, schemas ...string) ResultCompare {
		schema := ""
		if len(schemas) > 0 {
			schema = schemas[0]
		}
		return ExpectSDLValidationErrors(t, schema, LoneSchemaDefinitionRule, sdlStr)
	}

	ExpectValidSDL := func(t *testing.T, sdlStr string, schemas ...string) {
		ExpectSDLErrors(t, sdlStr, schemas...)([]Err{})
	}

	t.Run("Validate: Schema definition should be alone", func(t *testing.T) {
		t.Run("no schema", func(t *testing.T) {
			ExpectValidSDL(t, `
      type Query {
        foo: String
      }
    `)
		})

		t.Run("one schema definition", func(t *testing.T) {
			ExpectValidSDL(t, `
      schema {
        query: Foo
      }

      type Foo {
        foo: String
      }
    `)
		})

		t.Run("multiple schema definitions", func(t *testing.T) {
			ExpectSDLErrors(t, `
      schema {
        query: Foo
      }

      type Foo {
        foo: String
      }

      schema {
        mutation: Foo
      }

      schema {
        subscription: Foo
      }
    `)([]Err{
				{
					message:   "Must provide only one schema definition.",
					locations: []Loc{{line: 10, column: 7}},
				},
				{
					message:   "Must provide only one schema definition.",
					locations: []Loc{{line: 14, column: 7}},
				},
			})
		})

		t.Run("define schema in schema extension", func(t *testing.T) {
			schema := BuildSchema(`
      type Foo {
        foo: String
      }
    `)

			ExpectSDLErrors(t,
				`
        schema {
          query: Foo
        }
      `,
				schema,
			)([]Err{})
		})

		t.Run("redefine schema in schema extension", func(t *testing.T) {
			schema := BuildSchema(`
      schema {
        query: Foo
      }

      type Foo {
        foo: String
      }
    `)

			ExpectSDLErrors(t,
				`
        schema {
          mutation: Foo
        }
      `,
				schema,
			)([]Err{
				{
					message:   "Cannot define a new schema within a schema extension.",
					locations: []Loc{{line: 2, column: 9}},
				},
			})
		})

		t.Run("redefine implicit schema in schema extension", func(t *testing.T) {
			schema := BuildSchema(`
      type Query {
        fooField: Foo
      }

      type Foo {
        foo: String
      }
    `)

			ExpectSDLErrors(t,
				`
        schema {
          mutation: Foo
        }
      `,
				schema,
			)([]Err{
				{
					message:   "Cannot define a new schema within a schema extension.",
					locations: []Loc{{line: 2, column: 9}},
				},
			})
		})

		t.Run("extend schema in schema extension", func(t *testing.T) {
			schema := BuildSchema(`
      type Query {
        fooField: Foo
      }

      type Foo {
        foo: String
      }
    `)

			ExpectValidSDL(t,
				`
        extend schema {
          mutation: Foo
        }
      `,
				schema,
			)
		})
	})

}
