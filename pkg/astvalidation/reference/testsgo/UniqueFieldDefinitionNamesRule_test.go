package testsgo

import (
	"testing"

	"github.com/jensneuse/graphql-go-tools/pkg/astvalidation/reference/helpers"
)

func TestUniqueFieldDefinitionNamesRule(t *testing.T) {

	expectSDLErrors := func(sdlStr string, sch ...string) helpers.ResultCompare {
		schema := ""
		if len(sch) > 0 {
			schema = sch[0]
		}
		return helpers.ExpectSDLValidationErrors(
			schema,
			"UniqueFieldDefinitionNamesRule",
			sdlStr,
		)
	}

	expectValidSDL := func(sdlStr string, schema ...string) {
		expectSDLErrors(sdlStr, schema...)(`[]`)
	}

	t.Run("Validate: Unique field definition names", func(t *testing.T) {
		t.Run("no fields", func(t *testing.T) {
			expectValidSDL(`
      type SomeObject
      interface SomeInterface
      input SomeInputObject
    `)
		})

		t.Run("one field", func(t *testing.T) {
			expectValidSDL(`
      type SomeObject {
        foo: String
      }

      interface SomeInterface {
        foo: String
      }

      input SomeInputObject {
        foo: String
      }
    `)
		})

		t.Run("multiple fields", func(t *testing.T) {
			expectValidSDL(`
      type SomeObject {
        foo: String
        bar: String
      }

      interface SomeInterface {
        foo: String
        bar: String
      }

      input SomeInputObject {
        foo: String
        bar: String
      }
    `)
		})

		t.Run("duplicate fields inside the same type definition", func(t *testing.T) {
			expectSDLErrors(`
      type SomeObject {
        foo: String
        bar: String
        foo: String
      }

      interface SomeInterface {
        foo: String
        bar: String
        foo: String
      }

      input SomeInputObject {
        foo: String
        bar: String
        foo: String
      }
    `)(`[
      {
        message: 'Field "SomeObject.foo" can only be defined once.',
        locations: [
          { line: 3, column: 9 },
          { line: 5, column: 9 },
        ],
      },
      {
        message: 'Field "SomeInterface.foo" can only be defined once.',
        locations: [
          { line: 9, column: 9 },
          { line: 11, column: 9 },
        ],
      },
      {
        message: 'Field "SomeInputObject.foo" can only be defined once.',
        locations: [
          { line: 15, column: 9 },
          { line: 17, column: 9 },
        ],
      },
]`)
		})

		t.Run("extend type with new field", func(t *testing.T) {
			expectValidSDL(`
      type SomeObject {
        foo: String
      }
      extend type SomeObject {
        bar: String
      }
      extend type SomeObject {
        baz: String
      }

      interface SomeInterface {
        foo: String
      }
      extend interface SomeInterface {
        bar: String
      }
      extend interface SomeInterface {
        baz: String
      }

      input SomeInputObject {
        foo: String
      }
      extend input SomeInputObject {
        bar: String
      }
      extend input SomeInputObject {
        baz: String
      }
    `)
		})

		t.Run("extend type with duplicate field", func(t *testing.T) {
			expectSDLErrors(`
      extend type SomeObject {
        foo: String
      }
      type SomeObject {
        foo: String
      }

      extend interface SomeInterface {
        foo: String
      }
      interface SomeInterface {
        foo: String
      }

      extend input SomeInputObject {
        foo: String
      }
      input SomeInputObject {
        foo: String
      }
    `)(`[
      {
        message: 'Field "SomeObject.foo" can only be defined once.',
        locations: [
          { line: 3, column: 9 },
          { line: 6, column: 9 },
        ],
      },
      {
        message: 'Field "SomeInterface.foo" can only be defined once.',
        locations: [
          { line: 10, column: 9 },
          { line: 13, column: 9 },
        ],
      },
      {
        message: 'Field "SomeInputObject.foo" can only be defined once.',
        locations: [
          { line: 17, column: 9 },
          { line: 20, column: 9 },
        ],
      },
]`)
		})

		t.Run("duplicate field inside extension", func(t *testing.T) {
			expectSDLErrors(`
      type SomeObject
      extend type SomeObject {
        foo: String
        bar: String
        foo: String
      }

      interface SomeInterface
      extend interface SomeInterface {
        foo: String
        bar: String
        foo: String
      }

      input SomeInputObject
      extend input SomeInputObject {
        foo: String
        bar: String
        foo: String
      }
    `)(`[
      {
        message: 'Field "SomeObject.foo" can only be defined once.',
        locations: [
          { line: 4, column: 9 },
          { line: 6, column: 9 },
        ],
      },
      {
        message: 'Field "SomeInterface.foo" can only be defined once.',
        locations: [
          { line: 11, column: 9 },
          { line: 13, column: 9 },
        ],
      },
      {
        message: 'Field "SomeInputObject.foo" can only be defined once.',
        locations: [
          { line: 18, column: 9 },
          { line: 20, column: 9 },
        ],
      },
]`)
		})

		t.Run("duplicate field inside different extensions", func(t *testing.T) {
			expectSDLErrors(`
      type SomeObject
      extend type SomeObject {
        foo: String
      }
      extend type SomeObject {
        foo: String
      }

      interface SomeInterface
      extend interface SomeInterface {
        foo: String
      }
      extend interface SomeInterface {
        foo: String
      }

      input SomeInputObject
      extend input SomeInputObject {
        foo: String
      }
      extend input SomeInputObject {
        foo: String
      }
    `)(`[
      {
        message: 'Field "SomeObject.foo" can only be defined once.',
        locations: [
          { line: 4, column: 9 },
          { line: 7, column: 9 },
        ],
      },
      {
        message: 'Field "SomeInterface.foo" can only be defined once.',
        locations: [
          { line: 12, column: 9 },
          { line: 15, column: 9 },
        ],
      },
      {
        message: 'Field "SomeInputObject.foo" can only be defined once.',
        locations: [
          { line: 20, column: 9 },
          { line: 23, column: 9 },
        ],
      },
]`)
		})

		t.Run("adding new field to the type inside existing schema", func(t *testing.T) {
			schema := helpers.BuildSchema(`
      type SomeObject
      interface SomeInterface
      input SomeInputObject
    `)
			sdl := `
      extend type SomeObject {
        foo: String
      }

      extend interface SomeInterface {
        foo: String
      }

      extend input SomeInputObject {
        foo: String
      }
    `

			expectValidSDL(sdl, schema)
		})

		t.Run("adding conflicting fields to existing schema twice", func(t *testing.T) {
			schema := helpers.BuildSchema(`
      type SomeObject {
        foo: String
      }

      interface SomeInterface {
        foo: String
      }

      input SomeInputObject {
        foo: String
      }
    `)
			sdl := `
      extend type SomeObject {
        foo: String
      }
      extend interface SomeInterface {
        foo: String
      }
      extend input SomeInputObject {
        foo: String
      }

      extend type SomeObject {
        foo: String
      }
      extend interface SomeInterface {
        foo: String
      }
      extend input SomeInputObject {
        foo: String
      }
    `

			expectSDLErrors(sdl, schema)(`[
      {
        message:
          'Field "SomeObject.foo" already exists in the schema. It cannot also be defined in this type extension.',
        locations: [{ line: 3, column: 9 }],
      },
      {
        message:
          'Field "SomeInterface.foo" already exists in the schema. It cannot also be defined in this type extension.',
        locations: [{ line: 6, column: 9 }],
      },
      {
        message:
          'Field "SomeInputObject.foo" already exists in the schema. It cannot also be defined in this type extension.',
        locations: [{ line: 9, column: 9 }],
      },
      {
        message:
          'Field "SomeObject.foo" already exists in the schema. It cannot also be defined in this type extension.',
        locations: [{ line: 13, column: 9 }],
      },
      {
        message:
          'Field "SomeInterface.foo" already exists in the schema. It cannot also be defined in this type extension.',
        locations: [{ line: 16, column: 9 }],
      },
      {
        message:
          'Field "SomeInputObject.foo" already exists in the schema. It cannot also be defined in this type extension.',
        locations: [{ line: 19, column: 9 }],
      },
]`)
		})

		t.Run("adding fields to existing schema twice", func(t *testing.T) {
			schema := helpers.BuildSchema(`
      type SomeObject
      interface SomeInterface
      input SomeInputObject
    `)
			sdl := `
      extend type SomeObject {
        foo: String
      }
      extend type SomeObject {
        foo: String
      }

      extend interface SomeInterface {
        foo: String
      }
      extend interface SomeInterface {
        foo: String
      }

      extend input SomeInputObject {
        foo: String
      }
      extend input SomeInputObject {
        foo: String
      }
    `

			expectSDLErrors(sdl, schema)(`[
      {
        message: 'Field "SomeObject.foo" can only be defined once.',
        locations: [
          { line: 3, column: 9 },
          { line: 6, column: 9 },
        ],
      },
      {
        message: 'Field "SomeInterface.foo" can only be defined once.',
        locations: [
          { line: 10, column: 9 },
          { line: 13, column: 9 },
        ],
      },
      {
        message: 'Field "SomeInputObject.foo" can only be defined once.',
        locations: [
          { line: 17, column: 9 },
          { line: 20, column: 9 },
        ],
      },
]`)
		})
	})

}
