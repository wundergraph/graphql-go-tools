package testsgo

import (
	"testing"
)

func TestUniqueEnumValueNamesRule(t *testing.T) {
	t.Skip()

	ExpectSDLErrors := func(t *testing.T, sdlStr string, schemas ...string) ResultCompare {
		schema := ""
		if len(schemas) > 0 {
			schema = schemas[0]
		}
		return ExpectSDLValidationErrors(t, schema, UniqueEnumValueNamesRule, sdlStr)
	}

	ExpectValidSDL := func(t *testing.T, sdlStr string, schemas ...string) {
		ExpectSDLErrors(t, sdlStr, schemas...)([]Err{})
	}

	t.Run("Validate: Unique enum value names", func(t *testing.T) {
		t.Run("no values", func(t *testing.T) {
			ExpectValidSDL(t, `
      enum SomeEnum
    `)
		})

		t.Run("one value", func(t *testing.T) {
			ExpectValidSDL(t, `
      enum SomeEnum {
        FOO
      }
    `)
		})

		t.Run("multiple values", func(t *testing.T) {
			ExpectValidSDL(t, `
      enum SomeEnum {
        FOO
        BAR
      }
    `)
		})

		t.Run("duplicate values inside the same enum definition", func(t *testing.T) {
			ExpectSDLErrors(t, `
      enum SomeEnum {
        FOO
        BAR
        FOO
      }
    `)([]Err{
				{
					message: `Enum value "SomeEnum.FOO" can only be defined once.`,
					locations: []Loc{
						{line: 3, column: 9},
						{line: 5, column: 9},
					},
				},
			})
		})

		t.Run("extend enum with new value", func(t *testing.T) {
			ExpectValidSDL(t, `
      enum SomeEnum {
        FOO
      }
      extend enum SomeEnum {
        BAR
      }
      extend enum SomeEnum {
        BAZ
      }
    `)
		})

		t.Run("extend enum with duplicate value", func(t *testing.T) {
			ExpectSDLErrors(t, `
      extend enum SomeEnum {
        FOO
      }
      enum SomeEnum {
        FOO
      }
    `)([]Err{
				{
					message: `Enum value "SomeEnum.FOO" can only be defined once.`,
					locations: []Loc{
						{line: 3, column: 9},
						{line: 6, column: 9},
					},
				},
			})
		})

		t.Run("duplicate value inside extension", func(t *testing.T) {
			ExpectSDLErrors(t, `
      enum SomeEnum
      extend enum SomeEnum {
        FOO
        BAR
        FOO
      }
    `)([]Err{
				{
					message: `Enum value "SomeEnum.FOO" can only be defined once.`,
					locations: []Loc{
						{line: 4, column: 9},
						{line: 6, column: 9},
					},
				},
			})
		})

		t.Run("duplicate value inside different extensions", func(t *testing.T) {
			ExpectSDLErrors(t, `
      enum SomeEnum
      extend enum SomeEnum {
        FOO
      }
      extend enum SomeEnum {
        FOO
      }
    `)([]Err{
				{
					message: `Enum value "SomeEnum.FOO" can only be defined once.`,
					locations: []Loc{
						{line: 4, column: 9},
						{line: 7, column: 9},
					},
				},
			})
		})

		t.Run("adding new value to the type inside existing schema", func(t *testing.T) {
			schema := BuildSchema("enum SomeEnum")
			sdl := `
      extend enum SomeEnum {
        FOO
      }
    `

			ExpectValidSDL(t, sdl, schema)
		})

		t.Run("adding conflicting value to existing schema twice", func(t *testing.T) {
			schema := BuildSchema(`
      enum SomeEnum {
        FOO
      }
    `)
			sdl := `
      extend enum SomeEnum {
        FOO
      }
      extend enum SomeEnum {
        FOO
      }
    `

			ExpectSDLErrors(t, sdl, schema)([]Err{
				{
					message:   `Enum value "SomeEnum.FOO" already exists in the schema. It cannot also be defined in this type extension.`,
					locations: []Loc{{line: 3, column: 9}},
				},
				{
					message:   `Enum value "SomeEnum.FOO" already exists in the schema. It cannot also be defined in this type extension.`,
					locations: []Loc{{line: 6, column: 9}},
				},
			})
		})

		t.Run("adding enum values to existing schema twice", func(t *testing.T) {
			schema := BuildSchema("enum SomeEnum")
			sdl := `
      extend enum SomeEnum {
        FOO
      }
      extend enum SomeEnum {
        FOO
      }
    `

			ExpectSDLErrors(t, sdl, schema)([]Err{
				{
					message: `Enum value "SomeEnum.FOO" can only be defined once.`,
					locations: []Loc{
						{line: 3, column: 9},
						{line: 6, column: 9},
					},
				},
			})
		})
	})

}
