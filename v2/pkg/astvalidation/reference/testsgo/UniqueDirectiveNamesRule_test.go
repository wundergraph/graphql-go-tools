package testsgo

import (
	"testing"
)

func TestUniqueDirectiveNamesRule(t *testing.T) {
	t.Skip()

	ExpectSDLErrors := func(t *testing.T, sdlStr string, schemas ...string) ResultCompare {
		schema := ""
		if len(schemas) > 0 {
			schema = schemas[0]
		}
		return ExpectSDLValidationErrors(t, schema, UniqueDirectiveNamesRule, sdlStr)
	}

	ExpectValidSDL := func(t *testing.T, sdlStr string, schemas ...string) {
		ExpectSDLErrors(t, sdlStr, schemas...)([]Err{})
	}

	t.Run("Validate: Unique directive names", func(t *testing.T) {
		t.Run("no directive", func(t *testing.T) {
			ExpectValidSDL(t, `
      type Foo
    `)
		})

		t.Run("one directive", func(t *testing.T) {
			ExpectValidSDL(t, `
      directive @foo on SCHEMA
    `)
		})

		t.Run("many directives", func(t *testing.T) {
			ExpectValidSDL(t, `
      directive @foo on SCHEMA
      directive @bar on SCHEMA
      directive @baz on SCHEMA
    `)
		})

		t.Run("directive and non-directive definitions named the same", func(t *testing.T) {
			ExpectValidSDL(t, `
      query foo { __typename }
      fragment foo on foo { __typename }
      type foo

      directive @foo on SCHEMA
    `)
		})

		t.Run("directives named the same", func(t *testing.T) {
			ExpectSDLErrors(t, `
      directive @foo on SCHEMA

      directive @foo on SCHEMA
    `)([]Err{
				{
					message: `There can be only one directive named "@foo".`,
					locations: []Loc{
						{line: 2, column: 18},
						{line: 4, column: 18},
					},
				},
			})
		})

		t.Run("adding new directive to existing schema", func(t *testing.T) {
			schema := BuildSchema("directive @foo on SCHEMA")

			ExpectValidSDL(t, "directive @bar on SCHEMA", schema)
		})

		t.Run("adding new directive with standard name to existing schema", func(t *testing.T) {
			schema := BuildSchema("type foo")

			ExpectSDLErrors(t, "directive @skip on SCHEMA", schema)([]Err{
				{
					message:   `Directive "@skip" already exists in the schema. It cannot be redefined.`,
					locations: []Loc{{line: 1, column: 12}},
				},
			})
		})

		t.Run("adding new directive to existing schema with same-named type", func(t *testing.T) {
			schema := BuildSchema("type foo")

			ExpectValidSDL(t, "directive @foo on SCHEMA", schema)
		})

		t.Run("adding conflicting directives to existing schema", func(t *testing.T) {
			schema := BuildSchema("directive @foo on SCHEMA")

			ExpectSDLErrors(t, "directive @foo on SCHEMA", schema)([]Err{
				{
					message:   `Directive "@foo" already exists in the schema. It cannot be redefined.`,
					locations: []Loc{{line: 1, column: 12}},
				},
			})
		})
	})

}
