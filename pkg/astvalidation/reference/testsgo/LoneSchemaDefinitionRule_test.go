package testsgo

import (
	"testing"
)

func TestLoneSchemaDefinitionRule(t *testing.T) {

	expectSDLErrors := func(sdlStr string, sch ...string) ResultCompare {
		schema := ""
		if len(sch) > 0 {
			schema = sch[0]
		}
		return ExpectSDLValidationErrors(schema, "LoneSchemaDefinitionRule", sdlStr)
	}

	expectValidSDL := func(sdlStr string, schema ...string) {
		expectSDLErrors(sdlStr, schema...)(t, []Err{})
	}

	t.Run("Validate: Schema definition should be alone", func(t *testing.T) {
		t.Run("no schema", func(t *testing.T) {
			expectValidSDL(`
      type Query {
        foo: String
      }
    `)
		})

		t.Run("one schema definition", func(t *testing.T) {
			expectValidSDL(`
      schema {
        query: Foo
      }

      type Foo {
        foo: String
      }
    `)
		})

		t.Run("multiple schema definitions", func(t *testing.T) {
			expectSDLErrors(`
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
    `)(t, []Err{
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

			expectSDLErrors(
				`
        schema {
          query: Foo
        }
      `,
				schema,
			)(t, []Err{})
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

			expectSDLErrors(
				`
        schema {
          mutation: Foo
        }
      `,
				schema,
			)(t, []Err{
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

			expectSDLErrors(
				`
        schema {
          mutation: Foo
        }
      `,
				schema,
			)(t, []Err{
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

			expectValidSDL(
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
