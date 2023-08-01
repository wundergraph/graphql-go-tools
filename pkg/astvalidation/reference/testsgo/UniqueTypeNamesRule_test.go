package testsgo

import (
	"testing"
)

func TestUniqueTypeNamesRule(t *testing.T) {
	t.Skip()

	ExpectSDLErrors := func(t *testing.T, sdlStr string, schemas ...string) ResultCompare {
		schema := ""
		if len(schemas) > 0 {
			schema = schemas[0]
		}
		return ExpectSDLValidationErrors(t, schema, UniqueTypeNamesRule, sdlStr)
	}

	ExpectValidSDL := func(t *testing.T, sdlStr string, schemas ...string) {
		ExpectSDLErrors(t, sdlStr, schemas...)([]Err{})
	}

	t.Run("Validate: Unique type names", func(t *testing.T) {
		t.Run("no types", func(t *testing.T) {
			ExpectValidSDL(t, `
      directive @test on SCHEMA
    `)
		})

		t.Run("one type", func(t *testing.T) {
			ExpectValidSDL(t, `
      type Foo
    `)
		})

		t.Run("many types", func(t *testing.T) {
			ExpectValidSDL(t, `
      type Foo
      type Bar
      type Baz
    `)
		})

		t.Run("type and non-type definitions named the same", func(t *testing.T) {
			ExpectValidSDL(t, `
      query Foo { __typename }
      fragment Foo on Query { __typename }
      directive @Foo on SCHEMA

      type Foo
    `)
		})

		t.Run("types named the same", func(t *testing.T) {
			ExpectSDLErrors(t, `
      type Foo

      scalar Foo
      type Foo
      interface Foo
      union Foo
      enum Foo
      input Foo
    `)([]Err{
				{
					message: `There can be only one type named "Foo".`,
					locations: []Loc{
						{line: 2, column: 12},
						{line: 4, column: 14},
					},
				},
				{
					message: `There can be only one type named "Foo".`,
					locations: []Loc{
						{line: 2, column: 12},
						{line: 5, column: 12},
					},
				},
				{
					message: `There can be only one type named "Foo".`,
					locations: []Loc{
						{line: 2, column: 12},
						{line: 6, column: 17},
					},
				},
				{
					message: `There can be only one type named "Foo".`,
					locations: []Loc{
						{line: 2, column: 12},
						{line: 7, column: 13},
					},
				},
				{
					message: `There can be only one type named "Foo".`,
					locations: []Loc{
						{line: 2, column: 12},
						{line: 8, column: 12},
					},
				},
				{
					message: `There can be only one type named "Foo".`,
					locations: []Loc{
						{line: 2, column: 12},
						{line: 9, column: 13},
					},
				},
			})
		})

		t.Run("adding new type to existing schema", func(t *testing.T) {
			schema := BuildSchema("type Foo")

			ExpectValidSDL(t, "type Bar", schema)
		})

		t.Run("adding new type to existing schema with same-named directive", func(t *testing.T) {
			schema := BuildSchema("directive @Foo on SCHEMA")

			ExpectValidSDL(t, "type Foo", schema)
		})

		t.Run("adding conflicting types to existing schema", func(t *testing.T) {
			schema := BuildSchema("type Foo")
			sdl := `
      scalar Foo
      type Foo
      interface Foo
      union Foo
      enum Foo
      input Foo
    `

			ExpectSDLErrors(t, sdl, schema)([]Err{
				{
					message:   `Type "Foo" already exists in the schema. It cannot also be defined in this type definition.`,
					locations: []Loc{{line: 2, column: 14}},
				},
				{
					message:   `Type "Foo" already exists in the schema. It cannot also be defined in this type definition.`,
					locations: []Loc{{line: 3, column: 12}},
				},
				{
					message:   `Type "Foo" already exists in the schema. It cannot also be defined in this type definition.`,
					locations: []Loc{{line: 4, column: 17}},
				},
				{
					message:   `Type "Foo" already exists in the schema. It cannot also be defined in this type definition.`,
					locations: []Loc{{line: 5, column: 13}},
				},
				{
					message:   `Type "Foo" already exists in the schema. It cannot also be defined in this type definition.`,
					locations: []Loc{{line: 6, column: 12}},
				},
				{
					message:   `Type "Foo" already exists in the schema. It cannot also be defined in this type definition.`,
					locations: []Loc{{line: 7, column: 13}},
				},
			})
		})
	})

}
