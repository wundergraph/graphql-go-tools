package testsgo

import (
	"testing"
)

func TestPossibleTypeExtensionsRule(t *testing.T) {
	t.Skip()

	ExpectSDLErrors := func(t *testing.T, sdlStr string, schemas ...string) ResultCompare {
		schema := ""
		if len(schemas) > 0 {
			schema = schemas[0]
		}
		return ExpectSDLValidationErrors(t, schema, PossibleTypeExtensionsRule, sdlStr)
	}

	ExpectValidSDL := func(t *testing.T, sdlStr string, schemas ...string) {
		ExpectSDLErrors(t, sdlStr, schemas...)([]Err{})
	}

	t.Run("Validate: Possible type extensions", func(t *testing.T) {
		t.Run("no extensions", func(t *testing.T) {
			ExpectValidSDL(t, `
      scalar FooScalar
      type FooObject
      interface FooInterface
      union FooUnion
      enum FooEnum
      input FooInputObject
    `)
		})

		t.Run("one extension per type", func(t *testing.T) {
			ExpectValidSDL(t, `
      scalar FooScalar
      type FooObject
      interface FooInterface
      union FooUnion
      enum FooEnum
      input FooInputObject

      extend scalar FooScalar @dummy
      extend type FooObject @dummy
      extend interface FooInterface @dummy
      extend union FooUnion @dummy
      extend enum FooEnum @dummy
      extend input FooInputObject @dummy
    `)
		})

		t.Run("many extensions per type", func(t *testing.T) {
			ExpectValidSDL(t, `
      scalar FooScalar
      type FooObject
      interface FooInterface
      union FooUnion
      enum FooEnum
      input FooInputObject

      extend scalar FooScalar @dummy
      extend type FooObject @dummy
      extend interface FooInterface @dummy
      extend union FooUnion @dummy
      extend enum FooEnum @dummy
      extend input FooInputObject @dummy

      extend scalar FooScalar @dummy
      extend type FooObject @dummy
      extend interface FooInterface @dummy
      extend union FooUnion @dummy
      extend enum FooEnum @dummy
      extend input FooInputObject @dummy
    `)
		})

		t.Run("extending unknown type", func(t *testing.T) {
			message :=
				`Cannot extend type "Unknown" because it is not defined. Did you mean "Known"?`

			ExpectSDLErrors(t, `
      type Known

      extend scalar Unknown @dummy
      extend type Unknown @dummy
      extend interface Unknown @dummy
      extend union Unknown @dummy
      extend enum Unknown @dummy
      extend input Unknown @dummy
    `)([]Err{
				{message: message, locations: []Loc{{line: 4, column: 21}}},
				{message: message, locations: []Loc{{line: 5, column: 19}}},
				{message: message, locations: []Loc{{line: 6, column: 24}}},
				{message: message, locations: []Loc{{line: 7, column: 20}}},
				{message: message, locations: []Loc{{line: 8, column: 19}}},
				{message: message, locations: []Loc{{line: 9, column: 20}}},
			})
		})

		t.Run("does not consider non-type definitions", func(t *testing.T) {
			message := `Cannot extend type "Foo" because it is not defined.`

			ExpectSDLErrors(t, `
      query Foo { __typename }
      fragment Foo on Query { __typename }
      directive @Foo on SCHEMA

      extend scalar Foo @dummy
      extend type Foo @dummy
      extend interface Foo @dummy
      extend union Foo @dummy
      extend enum Foo @dummy
      extend input Foo @dummy
    `)([]Err{
				{message: message, locations: []Loc{{line: 6, column: 21}}},
				{message: message, locations: []Loc{{line: 7, column: 19}}},
				{message: message, locations: []Loc{{line: 8, column: 24}}},
				{message: message, locations: []Loc{{line: 9, column: 20}}},
				{message: message, locations: []Loc{{line: 10, column: 19}}},
				{message: message, locations: []Loc{{line: 11, column: 20}}},
			})
		})

		t.Run("extending with different kinds", func(t *testing.T) {
			ExpectSDLErrors(t, `
      scalar FooScalar
      type FooObject
      interface FooInterface
      union FooUnion
      enum FooEnum
      input FooInputObject

      extend type FooScalar @dummy
      extend interface FooObject @dummy
      extend union FooInterface @dummy
      extend enum FooUnion @dummy
      extend input FooEnum @dummy
      extend scalar FooInputObject @dummy
    `)([]Err{
				{
					message: `Cannot extend non-object type "FooScalar".`,
					locations: []Loc{
						{line: 2, column: 7},
						{line: 9, column: 7},
					},
				},
				{
					message: `Cannot extend non-interface type "FooObject".`,
					locations: []Loc{
						{line: 3, column: 7},
						{line: 10, column: 7},
					},
				},
				{
					message: `Cannot extend non-union type "FooInterface".`,
					locations: []Loc{
						{line: 4, column: 7},
						{line: 11, column: 7},
					},
				},
				{
					message: `Cannot extend non-enum type "FooUnion".`,
					locations: []Loc{
						{line: 5, column: 7},
						{line: 12, column: 7},
					},
				},
				{
					message: `Cannot extend non-input object type "FooEnum".`,
					locations: []Loc{
						{line: 6, column: 7},
						{line: 13, column: 7},
					},
				},
				{
					message: `Cannot extend non-scalar type "FooInputObject".`,
					locations: []Loc{
						{line: 7, column: 7},
						{line: 14, column: 7},
					},
				},
			})
		})

		t.Run("extending types within existing schema", func(t *testing.T) {
			schema := BuildSchema(`
      scalar FooScalar
      type FooObject
      interface FooInterface
      union FooUnion
      enum FooEnum
      input FooInputObject
    `)
			sdl := `
      extend scalar FooScalar @dummy
      extend type FooObject @dummy
      extend interface FooInterface @dummy
      extend union FooUnion @dummy
      extend enum FooEnum @dummy
      extend input FooInputObject @dummy
    `

			ExpectValidSDL(t, sdl, schema)
		})

		t.Run("extending unknown types within existing schema", func(t *testing.T) {
			schema := BuildSchema("type Known")
			sdl := `
      extend scalar Unknown @dummy
      extend type Unknown @dummy
      extend interface Unknown @dummy
      extend union Unknown @dummy
      extend enum Unknown @dummy
      extend input Unknown @dummy
    `

			message :=
				`Cannot extend type "Unknown" because it is not defined. Did you mean "Known"?`
			ExpectSDLErrors(t, sdl, schema)([]Err{
				{message: message, locations: []Loc{{line: 2, column: 21}}},
				{message: message, locations: []Loc{{line: 3, column: 19}}},
				{message: message, locations: []Loc{{line: 4, column: 24}}},
				{message: message, locations: []Loc{{line: 5, column: 20}}},
				{message: message, locations: []Loc{{line: 6, column: 19}}},
				{message: message, locations: []Loc{{line: 7, column: 20}}},
			})
		})

		t.Run("extending types with different kinds within existing schema", func(t *testing.T) {
			schema := BuildSchema(`
      scalar FooScalar
      type FooObject
      interface FooInterface
      union FooUnion
      enum FooEnum
      input FooInputObject
    `)
			sdl := `
      extend type FooScalar @dummy
      extend interface FooObject @dummy
      extend union FooInterface @dummy
      extend enum FooUnion @dummy
      extend input FooEnum @dummy
      extend scalar FooInputObject @dummy
    `

			ExpectSDLErrors(t, sdl, schema)([]Err{
				{
					message:   `Cannot extend non-object type "FooScalar".`,
					locations: []Loc{{line: 2, column: 7}},
				},
				{
					message:   `Cannot extend non-interface type "FooObject".`,
					locations: []Loc{{line: 3, column: 7}},
				},
				{
					message:   `Cannot extend non-union type "FooInterface".`,
					locations: []Loc{{line: 4, column: 7}},
				},
				{
					message:   `Cannot extend non-enum type "FooUnion".`,
					locations: []Loc{{line: 5, column: 7}},
				},
				{
					message:   `Cannot extend non-input object type "FooEnum".`,
					locations: []Loc{{line: 6, column: 7}},
				},
				{
					message:   `Cannot extend non-scalar type "FooInputObject".`,
					locations: []Loc{{line: 7, column: 7}},
				},
			})
		})
	})

}
