package testsgo

import (
	"testing"
)

func TestKnownTypeNamesRule(t *testing.T) {
	ExpectErrors := func(t *testing.T, queryStr string) ResultCompare {
		return ExpectValidationErrors(t, KnownTypeNamesOperationRule, queryStr)
	}

	ExpectErrorsWithSchema := func(t *testing.T, schema string, queryStr string) ResultCompare {
		return ExpectValidationErrorsWithSchema(t, schema, KnownTypeNamesOperationRule, queryStr)
	}

	ExpectValid := func(t *testing.T, queryStr string) {
		ExpectErrors(t, queryStr)([]Err{})
	}

	ExpectSDLErrors := func(t *testing.T, sdlStr string, schemas ...string) ResultCompare {
		schema := ""
		if len(schemas) > 0 {
			schema = schemas[0]
		}
		return ExpectSDLValidationErrors(t, schema, KnownTypeNamesRule, sdlStr)
	}

	ExpectValidSDL := func(t *testing.T, sdlStr string, schemas ...string) {
		ExpectSDLErrors(t, sdlStr, schemas...)([]Err{})
	}

	t.Run("Validate: Known type names", func(t *testing.T) {
		t.Run("known type names are valid", func(t *testing.T) {
			ExpectValid(t, `
      query Foo(
        $var: String
        $required: [Int!]!
        # $introspectionType: __EnumValue - disabled cause it is not valid input type
      ) {
        human(id: 4) {
          pets { ... on Pet { name }, ... { name } }
        }
      }
    `)
		})

		t.Run("unknown type names are invalid", func(t *testing.T) {
			ExpectErrors(t, `
      query Foo($var: JumbledUpLetters) {
        human(id: 4) {
          name
          pets { ... on Badger { name } }
        }
      }
      fragment PetFields on Peat {
        name
      }
    `)([]Err{
				{
					message:   `Unknown type "JumbledUpLetters".`,
					locations: []Loc{{line: 2, column: 23}},
				},
				{
					message:   `Unknown type "Badger".`,
					locations: []Loc{{line: 5, column: 25}},
				},
				{
					message:   `Unknown type "Peat".`,
					locations: []Loc{{line: 8, column: 29}},
				},
			})
		})

		t.Run("unknown type names are invalid", func(t *testing.T) {
			t.Skip("1. Suggestions is not supported. 2. Fragment cycle is reported incorrectly for PetFields fragment")

			ExpectErrors(t, `
      query Foo($var: JumbledUpLetters) {
        human(id: 4) {
          name
          pets { ... on Badger { name } ...PetFields }
        }
      }
      fragment PetFields on Peat {
        name
      }
    `)([]Err{
				{
					message:   `Unknown type "JumbledUpLetters".`,
					locations: []Loc{{line: 2, column: 23}},
				},
				{
					message:   `Unknown type "Badger".`,
					locations: []Loc{{line: 5, column: 25}},
				},
				{
					message:   `Unknown type "Peat". Did you mean "Pet" or "Cat"?`,
					locations: []Loc{{line: 8, column: 29}},
				},
			})
		})

		t.Run("references to standard scalars that are missing in schema", func(t *testing.T) {
			t.Skip("think fix or remove - currently harness helpers are always merging base schema with schema")

			schema := BuildSchema("type Query { foo: String }")
			query := `
      query ($id: ID, $float: Float, $int: Int) {
        __typename
      }
    `
			ExpectErrorsWithSchema(t, schema, query)([]Err{
				{
					message:   `Unknown type "ID".`,
					locations: []Loc{{line: 2, column: 19}},
				},
				{
					message:   `Unknown type "Float".`,
					locations: []Loc{{line: 2, column: 31}},
				},
				{
					message:   `Unknown type "Int".`,
					locations: []Loc{{line: 2, column: 44}},
				},
			})
		})

		t.Run("within SDL", func(t *testing.T) {
			t.Skip()

			t.Run("use standard types", func(t *testing.T) {
				ExpectValidSDL(t, `
        type Query {
          string: String
          int: Int
          float: Float
          boolean: Boolean
          id: ID
          introspectionType: __EnumValue
        }
      `)
			})

			t.Run("reference types defined inside the same document", func(t *testing.T) {
				ExpectValidSDL(t, `
        union SomeUnion = SomeObject | AnotherObject

        type SomeObject implements SomeInterface {
          someScalar(arg: SomeInputObject): SomeScalar
        }

        type AnotherObject {
          foo(arg: SomeInputObject): String
        }

        type SomeInterface {
          someScalar(arg: SomeInputObject): SomeScalar
        }

        input SomeInputObject {
          someScalar: SomeScalar
        }

        scalar SomeScalar

        type RootQuery {
          someInterface: SomeInterface
          someUnion: SomeUnion
          someScalar: SomeScalar
          someObject: SomeObject
        }

        schema {
          query: RootQuery
        }
      `)
			})

			t.Run("unknown type references", func(t *testing.T) {
				ExpectSDLErrors(t, `
        type A
        type B

        type SomeObject implements C {
          e(d: D): E
        }

        union SomeUnion = F | G

        interface SomeInterface {
          i(h: H): I
        }

        input SomeInput {
          j: J
        }

        directive @SomeDirective(k: K) on QUERY

        schema {
          query: L
          mutation: M
          subscription: N
        }
      `)([]Err{
					{
						message:   `Unknown type "C". Did you mean "A" or "B"?`,
						locations: []Loc{{line: 5, column: 36}},
					},
					{
						message:   `Unknown type "D". Did you mean "A", "B", or "ID"?`,
						locations: []Loc{{line: 6, column: 16}},
					},
					{
						message:   `Unknown type "E". Did you mean "A" or "B"?`,
						locations: []Loc{{line: 6, column: 20}},
					},
					{
						message:   `Unknown type "F". Did you mean "A" or "B"?`,
						locations: []Loc{{line: 9, column: 27}},
					},
					{
						message:   `Unknown type "G". Did you mean "A" or "B"?`,
						locations: []Loc{{line: 9, column: 31}},
					},
					{
						message:   `Unknown type "H". Did you mean "A" or "B"?`,
						locations: []Loc{{line: 12, column: 16}},
					},
					{
						message:   `Unknown type "I". Did you mean "A", "B", or "ID"?`,
						locations: []Loc{{line: 12, column: 20}},
					},
					{
						message:   `Unknown type "J". Did you mean "A" or "B"?`,
						locations: []Loc{{line: 16, column: 14}},
					},
					{
						message:   `Unknown type "K". Did you mean "A" or "B"?`,
						locations: []Loc{{line: 19, column: 37}},
					},
					{
						message:   `Unknown type "L". Did you mean "A" or "B"?`,
						locations: []Loc{{line: 22, column: 18}},
					},
					{
						message:   `Unknown type "M". Did you mean "A" or "B"?`,
						locations: []Loc{{line: 23, column: 21}},
					},
					{
						message:   `Unknown type "N". Did you mean "A" or "B"?`,
						locations: []Loc{{line: 24, column: 25}},
					},
				})
			})

			t.Run("does not consider non-type definitions", func(t *testing.T) {
				ExpectSDLErrors(t, `
        query Foo { __typename }
        fragment Foo on Query { __typename }
        directive @Foo on QUERY

        type Query {
          foo: Foo
        }
      `)([]Err{
					{
						message:   `Unknown type "Foo".`,
						locations: []Loc{{line: 7, column: 16}},
					},
				})
			})

			t.Run("reference standard types inside extension document", func(t *testing.T) {
				schema := BuildSchema("type Foo")
				sdl := `
        type SomeType {
          string: String
          int: Int
          float: Float
          boolean: Boolean
          id: ID
          introspectionType: __EnumValue
        }
      `

				ExpectValidSDL(t, sdl, schema)
			})

			t.Run("reference types inside extension document", func(t *testing.T) {
				schema := BuildSchema("type Foo")
				sdl := `
        type QueryRoot {
          foo: Foo
          bar: Bar
        }

        scalar Bar

        schema {
          query: QueryRoot
        }
      `

				ExpectValidSDL(t, sdl, schema)
			})

			t.Run("unknown type references inside extension document", func(t *testing.T) {
				schema := BuildSchema("type A")
				sdl := `
        type B

        type SomeObject implements C {
          e(d: D): E
        }

        union SomeUnion = F | G

        interface SomeInterface {
          i(h: H): I
        }

        input SomeInput {
          j: J
        }

        directive @SomeDirective(k: K) on QUERY

        schema {
          query: L
          mutation: M
          subscription: N
        }
      `

				ExpectSDLErrors(t, sdl, schema)([]Err{
					{
						message:   `Unknown type "C". Did you mean "A" or "B"?`,
						locations: []Loc{{line: 4, column: 36}},
					},
					{
						message:   `Unknown type "D". Did you mean "A", "B", or "ID"?`,
						locations: []Loc{{line: 5, column: 16}},
					},
					{
						message:   `Unknown type "E". Did you mean "A" or "B"?`,
						locations: []Loc{{line: 5, column: 20}},
					},
					{
						message:   `Unknown type "F". Did you mean "A" or "B"?`,
						locations: []Loc{{line: 8, column: 27}},
					},
					{
						message:   `Unknown type "G". Did you mean "A" or "B"?`,
						locations: []Loc{{line: 8, column: 31}},
					},
					{
						message:   `Unknown type "H". Did you mean "A" or "B"?`,
						locations: []Loc{{line: 11, column: 16}},
					},
					{
						message:   `Unknown type "I". Did you mean "A", "B", or "ID"?`,
						locations: []Loc{{line: 11, column: 20}},
					},
					{
						message:   `Unknown type "J". Did you mean "A" or "B"?`,
						locations: []Loc{{line: 15, column: 14}},
					},
					{
						message:   `Unknown type "K". Did you mean "A" or "B"?`,
						locations: []Loc{{line: 18, column: 37}},
					},
					{
						message:   `Unknown type "L". Did you mean "A" or "B"?`,
						locations: []Loc{{line: 21, column: 18}},
					},
					{
						message:   `Unknown type "M". Did you mean "A" or "B"?`,
						locations: []Loc{{line: 22, column: 21}},
					},
					{
						message:   `Unknown type "N". Did you mean "A" or "B"?`,
						locations: []Loc{{line: 23, column: 25}},
					},
				})
			})
		})
	})

}
