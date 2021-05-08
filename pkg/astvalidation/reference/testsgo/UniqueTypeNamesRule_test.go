package testsgo

import (
	"testing"

	"github.com/jensneuse/graphql-go-tools/pkg/astvalidation/reference/helpers"
)

func TestUniqueTypeNamesRule(t *testing.T) {

	expectSDLErrors := func(sdlStr string, sch ...string) helpers.ResultCompare {
		schema := ""
		if len(sch) > 0 {
			schema = sch[0]
		}
		return helpers.ExpectSDLValidationErrors(schema, "UniqueTypeNamesRule", sdlStr)
	}

	expectValidSDL := func(sdlStr string, schema ...string) {
		expectSDLErrors(sdlStr, schema...)(`[]`)
	}

	t.Run("Validate: Unique type names", func(t *testing.T) {
		t.Run("no types", func(t *testing.T) {
			expectValidSDL(`
      directive @test on SCHEMA
    `)
		})

		t.Run("one type", func(t *testing.T) {
			expectValidSDL(`
      type Foo
    `)
		})

		t.Run("many types", func(t *testing.T) {
			expectValidSDL(`
      type Foo
      type Bar
      type Baz
    `)
		})

		t.Run("type and non-type definitions named the same", func(t *testing.T) {
			expectValidSDL(`
      query Foo { __typename }
      fragment Foo on Query { __typename }
      directive @Foo on SCHEMA

      type Foo
    `)
		})

		t.Run("types named the same", func(t *testing.T) {
			expectSDLErrors(`
      type Foo

      scalar Foo
      type Foo
      interface Foo
      union Foo
      enum Foo
      input Foo
    `)(`[
      {
        message: 'There can be only one type named "Foo".',
        locations: [
          { line: 2, column: 12 },
          { line: 4, column: 14 },
        ],
      },
      {
        message: 'There can be only one type named "Foo".',
        locations: [
          { line: 2, column: 12 },
          { line: 5, column: 12 },
        ],
      },
      {
        message: 'There can be only one type named "Foo".',
        locations: [
          { line: 2, column: 12 },
          { line: 6, column: 17 },
        ],
      },
      {
        message: 'There can be only one type named "Foo".',
        locations: [
          { line: 2, column: 12 },
          { line: 7, column: 13 },
        ],
      },
      {
        message: 'There can be only one type named "Foo".',
        locations: [
          { line: 2, column: 12 },
          { line: 8, column: 12 },
        ],
      },
      {
        message: 'There can be only one type named "Foo".',
        locations: [
          { line: 2, column: 12 },
          { line: 9, column: 13 },
        ],
      },
]`)
		})

		t.Run("adding new type to existing schema", func(t *testing.T) {
			schema := helpers.BuildSchema("type Foo")

			expectValidSDL("type Bar", schema)
		})

		t.Run("adding new type to existing schema with same-named directive", func(t *testing.T) {
			schema := helpers.BuildSchema("directive @Foo on SCHEMA")

			expectValidSDL("type Foo", schema)
		})

		t.Run("adding conflicting types to existing schema", func(t *testing.T) {
			schema := helpers.BuildSchema("type Foo")
			sdl := `
      scalar Foo
      type Foo
      interface Foo
      union Foo
      enum Foo
      input Foo
    `

			expectSDLErrors(sdl, schema)(`[
      {
        message:
          'Type "Foo" already exists in the schema. It cannot also be defined in this type definition.',
        locations: [{ line: 2, column: 14 }],
      },
      {
        message:
          'Type "Foo" already exists in the schema. It cannot also be defined in this type definition.',
        locations: [{ line: 3, column: 12 }],
      },
      {
        message:
          'Type "Foo" already exists in the schema. It cannot also be defined in this type definition.',
        locations: [{ line: 4, column: 17 }],
      },
      {
        message:
          'Type "Foo" already exists in the schema. It cannot also be defined in this type definition.',
        locations: [{ line: 5, column: 13 }],
      },
      {
        message:
          'Type "Foo" already exists in the schema. It cannot also be defined in this type definition.',
        locations: [{ line: 6, column: 12 }],
      },
      {
        message:
          'Type "Foo" already exists in the schema. It cannot also be defined in this type definition.',
        locations: [{ line: 7, column: 13 }],
      },
]`)
		})
	})

}
