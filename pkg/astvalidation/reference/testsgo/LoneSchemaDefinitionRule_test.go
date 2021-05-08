package testsgo

import (
	"testing"

	"github.com/jensneuse/graphql-go-tools/pkg/astvalidation/reference/helpers"
)

func TestLoneSchemaDefinitionRule(t *testing.T) {

	expectSDLErrors := func(sdlStr string, sch ...string) helpers.ResultCompare {
		schema := ""
		if len(sch) > 0 {
			schema = sch[0]
		}
		return helpers.ExpectSDLValidationErrors(schema, "LoneSchemaDefinitionRule", sdlStr)
	}

	expectValidSDL := func(sdlStr string, schema ...string) {
		expectSDLErrors(sdlStr, schema...)(`[]`)
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
    `)(`[
      {
        message: "Must provide only one schema definition.",
        locations: [{ line: 10, column: 7 }],
      },
      {
        message: "Must provide only one schema definition.",
        locations: [{ line: 14, column: 7 }],
      },
]`)
		})

		t.Run("define schema in schema extension", func(t *testing.T) {
			schema := helpers.BuildSchema(`
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
			)(`[]`)
		})

		t.Run("redefine schema in schema extension", func(t *testing.T) {
			schema := helpers.BuildSchema(`
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
			)(`[
      {
        message: "Cannot define a new schema within a schema extension.",
        locations: [{ line: 2, column: 9 }],
      },
]`)
		})

		t.Run("redefine implicit schema in schema extension", func(t *testing.T) {
			schema := helpers.BuildSchema(`
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
			)(`[
      {
        message: "Cannot define a new schema within a schema extension.",
        locations: [{ line: 2, column: 9 }],
      },
]`)
		})

		t.Run("extend schema in schema extension", func(t *testing.T) {
			schema := helpers.BuildSchema(`
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
