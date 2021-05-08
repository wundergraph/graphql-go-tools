package testsgo

import (
	"testing"

	"github.com/jensneuse/graphql-go-tools/pkg/astvalidation/reference/helpers"
)

func TestUniqueDirectiveNamesRule(t *testing.T) {

	expectSDLErrors := func(sdlStr string, sch ...string) helpers.ResultCompare {
		schema := ""
		if len(sch) > 0 {
			schema = sch[0]
		}
		return helpers.ExpectSDLValidationErrors(schema, "UniqueDirectiveNamesRule", sdlStr)
	}

	expectValidSDL := func(sdlStr string, schema ...string) {
		expectSDLErrors(sdlStr, schema...)(`[]`)
	}

	t.Run("Validate: Unique directive names", func(t *testing.T) {
		t.Run("no directive", func(t *testing.T) {
			expectValidSDL(`
      type Foo
    `)
		})

		t.Run("one directive", func(t *testing.T) {
			expectValidSDL(`
      directive @foo on SCHEMA
    `)
		})

		t.Run("many directives", func(t *testing.T) {
			expectValidSDL(`
      directive @foo on SCHEMA
      directive @bar on SCHEMA
      directive @baz on SCHEMA
    `)
		})

		t.Run("directive and non-directive definitions named the same", func(t *testing.T) {
			expectValidSDL(`
      query foo { __typename }
      fragment foo on foo { __typename }
      type foo

      directive @foo on SCHEMA
    `)
		})

		t.Run("directives named the same", func(t *testing.T) {
			expectSDLErrors(`
      directive @foo on SCHEMA

      directive @foo on SCHEMA
    `)(`[
      {
        message: 'There can be only one directive named "@foo".',
        locations: [
          { line: 2, column: 18 },
          { line: 4, column: 18 },
        ],
      },
]`)
		})

		t.Run("adding new directive to existing schema", func(t *testing.T) {
			schema := helpers.BuildSchema("directive @foo on SCHEMA")

			expectValidSDL("directive @bar on SCHEMA", schema)
		})

		t.Run("adding new directive with standard name to existing schema", func(t *testing.T) {
			schema := helpers.BuildSchema("type foo")

			expectSDLErrors("directive @skip on SCHEMA", schema)(`[
      {
        message:
          'Directive "@skip" already exists in the schema. It cannot be redefined.',
        locations: [{ line: 1, column: 12 }],
      },
]`)
		})

		t.Run("adding new directive to existing schema with same-named type", func(t *testing.T) {
			schema := helpers.BuildSchema("type foo")

			expectValidSDL("directive @foo on SCHEMA", schema)
		})

		t.Run("adding conflicting directives to existing schema", func(t *testing.T) {
			schema := helpers.BuildSchema("directive @foo on SCHEMA")

			expectSDLErrors("directive @foo on SCHEMA", schema)(`[
      {
        message:
          'Directive "@foo" already exists in the schema. It cannot be redefined.',
        locations: [{ line: 1, column: 12 }],
      },
]`)
		})
	})

}
