package testsgo

import (
	"testing"
)

func TestUniqueEnumValueNamesRule(t *testing.T) {

	expectSDLErrors := func(sdlStr string, sch ...string) ResultCompare {
		schema := ""
		if len(sch) > 0 {
			schema = sch[0]
		}
		return ExpectSDLValidationErrors(schema, "UniqueEnumValueNamesRule", sdlStr)
	}

	expectValidSDL := func(sdlStr string, schema ...string) {
		expectSDLErrors(sdlStr, schema...)([]Err{})
	}

	t.Run("Validate: Unique enum value names", func(t *testing.T) {
		t.Run("no values", func(t *testing.T) {
			expectValidSDL(`
      enum SomeEnum
    `)
		})

		t.Run("one value", func(t *testing.T) {
			expectValidSDL(`
      enum SomeEnum {
        FOO
      }
    `)
		})

		t.Run("multiple values", func(t *testing.T) {
			expectValidSDL(`
      enum SomeEnum {
        FOO
        BAR
      }
    `)
		})

		t.Run("duplicate values inside the same enum definition", func(t *testing.T) {
			expectSDLErrors(`
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
			expectValidSDL(`
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
			expectSDLErrors(`
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
			expectSDLErrors(`
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
			expectSDLErrors(`
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

			expectValidSDL(sdl, schema)
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

			expectSDLErrors(sdl, schema)([]Err{
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

			expectSDLErrors(sdl, schema)([]Err{
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
