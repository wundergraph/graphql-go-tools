package testsgo

import (
	"testing"
)

func TestKnownTypeNamesRule(t *testing.T) {

	expectErrors := func(queryStr string) ResultCompare {
		return ExpectValidationErrors("KnownTypeNamesRule", queryStr)
	}

	expectErrorsWithSchema := func(schema string, queryStr string) ResultCompare {
		return ExpectValidationErrorsWithSchema(schema, "KnownTypeNamesRule", queryStr)
	}

	expectValid := func(queryStr string) {
		expectErrors(queryStr)(t, []Err{})
	}

	expectSDLErrors := func(sdlStr string, sch ...string) ResultCompare {
		schema := ""
		if len(sch) > 0 {
			schema = sch[0]
		}
		return ExpectSDLValidationErrors(schema, "KnownTypeNamesRule", sdlStr)
	}

	expectValidSDL := func(sdlStr string, schema ...string) {
		expectSDLErrors(sdlStr, schema...)(t, []Err{})
	}

	t.Run("Validate: Known type names", func(t *testing.T) {
		t.Run("known type names are valid", func(t *testing.T) {
			expectValid(`
      query Foo(
        $var: String
        $required: [Int!]!
        $introspectionType: __EnumValue
      ) {
        user(id: 4) {
          pets { ... on Pet { name }, ...PetFields, ... { name } }
        }
      }

      fragment PetFields on Pet {
        name
      }
    `)
		})

		t.Run("unknown type names are invalid", func(t *testing.T) {
			expectErrors(`
      query Foo($var: JumbledUpLetters) {
        user(id: 4) {
          name
          pets { ... on Badger { name }, ...PetFields }
        }
      }
      fragment PetFields on Peat {
        name
      }
    `)(t, []Err{
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
			schema := BuildSchema("type Query { foo: String }")
			query := `
      query ($id: ID, $float: Float, $int: Int) {
        __typename
      }
    `
			expectErrorsWithSchema(schema, query)(t, []Err{
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
			t.Run("use standard types", func(t *testing.T) {
				expectValidSDL(`
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
				expectValidSDL(`
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
				expectSDLErrors(`
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
      `)(t, []Err{
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
				expectSDLErrors(`
        query Foo { __typename }
        fragment Foo on Query { __typename }
        directive @Foo on QUERY

        type Query {
          foo: Foo
        }
      `)(t, []Err{
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

				expectValidSDL(sdl, schema)
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

				expectValidSDL(sdl, schema)
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

				expectSDLErrors(sdl, schema)(t, []Err{
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
