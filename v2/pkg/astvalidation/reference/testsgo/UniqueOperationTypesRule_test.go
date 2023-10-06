package testsgo

import (
	"testing"
)

func TestUniqueOperationTypesRule(t *testing.T) {
	t.Skip()

	ExpectSDLErrors := func(t *testing.T, sdlStr string, schemas ...string) ResultCompare {
		schema := ""
		if len(schemas) > 0 {
			schema = schemas[0]
		}
		return ExpectSDLValidationErrors(t, schema, UniqueOperationTypesRule, sdlStr)
	}

	ExpectValidSDL := func(t *testing.T, sdlStr string, schemas ...string) {
		ExpectSDLErrors(t, sdlStr, schemas...)([]Err{})
	}

	t.Run("Validate: Unique operation types", func(t *testing.T) {
		t.Run("no schema definition", func(t *testing.T) {
			ExpectValidSDL(t, `
      type Foo
    `)
		})

		t.Run("schema definition with all types", func(t *testing.T) {
			ExpectValidSDL(t, `
      type Foo

      schema {
        query: Foo
        mutation: Foo
        subscription: Foo
      }
    `)
		})

		t.Run("schema definition with single extension", func(t *testing.T) {
			ExpectValidSDL(t, `
      type Foo

      schema { query: Foo }

      extend schema {
        mutation: Foo
        subscription: Foo
      }
    `)
		})

		t.Run("schema definition with separate extensions", func(t *testing.T) {
			ExpectValidSDL(t, `
      type Foo

      schema { query: Foo }
      extend schema { mutation: Foo }
      extend schema { subscription: Foo }
    `)
		})

		t.Run("extend schema before definition", func(t *testing.T) {
			ExpectValidSDL(t, `
      type Foo

      extend schema { mutation: Foo }
      extend schema { subscription: Foo }

      schema { query: Foo }
    `)
		})

		t.Run("duplicate operation types inside single schema definition", func(t *testing.T) {
			ExpectSDLErrors(t, `
      type Foo

      schema {
        query: Foo
        mutation: Foo
        subscription: Foo

        query: Foo
        mutation: Foo
        subscription: Foo
      }
    `)([]Err{
				{
					message: "There can be only one query type in schema.",
					locations: []Loc{
						{line: 5, column: 9},
						{line: 9, column: 9},
					},
				},
				{
					message: "There can be only one mutation type in schema.",
					locations: []Loc{
						{line: 6, column: 9},
						{line: 10, column: 9},
					},
				},
				{
					message: "There can be only one subscription type in schema.",
					locations: []Loc{
						{line: 7, column: 9},
						{line: 11, column: 9},
					},
				},
			})
		})

		t.Run("duplicate operation types inside schema extension", func(t *testing.T) {
			ExpectSDLErrors(t, `
      type Foo

      schema {
        query: Foo
        mutation: Foo
        subscription: Foo
      }

      extend schema {
        query: Foo
        mutation: Foo
        subscription: Foo
      }
    `)([]Err{
				{
					message: "There can be only one query type in schema.",
					locations: []Loc{
						{line: 5, column: 9},
						{line: 11, column: 9},
					},
				},
				{
					message: "There can be only one mutation type in schema.",
					locations: []Loc{
						{line: 6, column: 9},
						{line: 12, column: 9},
					},
				},
				{
					message: "There can be only one subscription type in schema.",
					locations: []Loc{
						{line: 7, column: 9},
						{line: 13, column: 9},
					},
				},
			})
		})

		t.Run("duplicate operation types inside schema extension twice", func(t *testing.T) {
			ExpectSDLErrors(t, `
      type Foo

      schema {
        query: Foo
        mutation: Foo
        subscription: Foo
      }

      extend schema {
        query: Foo
        mutation: Foo
        subscription: Foo
      }

      extend schema {
        query: Foo
        mutation: Foo
        subscription: Foo
      }
    `)([]Err{
				{
					message: "There can be only one query type in schema.",
					locations: []Loc{
						{line: 5, column: 9},
						{line: 11, column: 9},
					},
				},
				{
					message: "There can be only one mutation type in schema.",
					locations: []Loc{
						{line: 6, column: 9},
						{line: 12, column: 9},
					},
				},
				{
					message: "There can be only one subscription type in schema.",
					locations: []Loc{
						{line: 7, column: 9},
						{line: 13, column: 9},
					},
				},
				{
					message: "There can be only one query type in schema.",
					locations: []Loc{
						{line: 5, column: 9},
						{line: 17, column: 9},
					},
				},
				{
					message: "There can be only one mutation type in schema.",
					locations: []Loc{
						{line: 6, column: 9},
						{line: 18, column: 9},
					},
				},
				{
					message: "There can be only one subscription type in schema.",
					locations: []Loc{
						{line: 7, column: 9},
						{line: 19, column: 9},
					},
				},
			})
		})

		t.Run("duplicate operation types inside second schema extension", func(t *testing.T) {
			ExpectSDLErrors(t, `
      type Foo

      schema {
        query: Foo
      }

      extend schema {
        mutation: Foo
        subscription: Foo
      }

      extend schema {
        query: Foo
        mutation: Foo
        subscription: Foo
      }
    `)([]Err{
				{
					message: "There can be only one query type in schema.",
					locations: []Loc{
						{line: 5, column: 9},
						{line: 14, column: 9},
					},
				},
				{
					message: "There can be only one mutation type in schema.",
					locations: []Loc{
						{line: 9, column: 9},
						{line: 15, column: 9},
					},
				},
				{
					message: "There can be only one subscription type in schema.",
					locations: []Loc{
						{line: 10, column: 9},
						{line: 16, column: 9},
					},
				},
			})
		})

		t.Run("define schema inside extension SDL", func(t *testing.T) {
			schema := BuildSchema("type Foo")
			sdl := `
      schema {
        query: Foo
        mutation: Foo
        subscription: Foo
      }
    `

			ExpectValidSDL(t, sdl, schema)
		})

		t.Run("define and extend schema inside extension SDL", func(t *testing.T) {
			schema := BuildSchema("type Foo")
			sdl := `
      schema { query: Foo }
      extend schema { mutation: Foo }
      extend schema { subscription: Foo }
    `

			ExpectValidSDL(t, sdl, schema)
		})

		t.Run("adding new operation types to existing schema", func(t *testing.T) {
			schema := BuildSchema("type Query")
			sdl := `
      extend schema { mutation: Foo }
      extend schema { subscription: Foo }
    `

			ExpectValidSDL(t, sdl, schema)
		})

		t.Run("adding conflicting operation types to existing schema", func(t *testing.T) {
			schema := BuildSchema(`
      type Query
      type Mutation
      type Subscription

      type Foo
    `)

			sdl := `
      extend schema {
        query: Foo
        mutation: Foo
        subscription: Foo
      }
    `

			ExpectSDLErrors(t, sdl, schema)([]Err{
				{
					message:   "Type for query already defined in the schema. It cannot be redefined.",
					locations: []Loc{{line: 3, column: 9}},
				},
				{
					message:   "Type for mutation already defined in the schema. It cannot be redefined.",
					locations: []Loc{{line: 4, column: 9}},
				},
				{
					message:   "Type for subscription already defined in the schema. It cannot be redefined.",
					locations: []Loc{{line: 5, column: 9}},
				},
			})
		})

		t.Run("adding conflicting operation types to existing schema twice", func(t *testing.T) {
			schema := BuildSchema(`
      type Query
      type Mutation
      type Subscription
    `)

			sdl := `
      extend schema {
        query: Foo
        mutation: Foo
        subscription: Foo
      }

      extend schema {
        query: Foo
        mutation: Foo
        subscription: Foo
      }
    `

			ExpectSDLErrors(t, sdl, schema)([]Err{
				{
					message:   "Type for query already defined in the schema. It cannot be redefined.",
					locations: []Loc{{line: 3, column: 9}},
				},
				{
					message:   "Type for mutation already defined in the schema. It cannot be redefined.",
					locations: []Loc{{line: 4, column: 9}},
				},
				{
					message:   "Type for subscription already defined in the schema. It cannot be redefined.",
					locations: []Loc{{line: 5, column: 9}},
				},
				{
					message:   "Type for query already defined in the schema. It cannot be redefined.",
					locations: []Loc{{line: 9, column: 9}},
				},
				{
					message:   "Type for mutation already defined in the schema. It cannot be redefined.",
					locations: []Loc{{line: 10, column: 9}},
				},
				{
					message:   "Type for subscription already defined in the schema. It cannot be redefined.",
					locations: []Loc{{line: 11, column: 9}},
				},
			})
		})
	})

}
