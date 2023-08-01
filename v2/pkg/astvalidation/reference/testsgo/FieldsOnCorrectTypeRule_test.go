package testsgo

import (
	"testing"
)

func TestFieldsOnCorrectTypeRule(t *testing.T) {
	t.Skip()

	ExpectErrors := func(t *testing.T, queryStr string) ResultCompare {
		return ExpectValidationErrors(t, FieldsOnCorrectTypeRule, queryStr)
	}

	ExpectValid := func(t *testing.T, queryStr string) {
		ExpectErrors(t, queryStr)([]Err{})
	}

	t.Run("Validate: Fields on correct type", func(t *testing.T) {
		t.Run("Object field selection", func(t *testing.T) {
			ExpectValid(t, `
      fragment objectFieldSelection on Dog {
        __typename
        name
      }
    `)
		})

		t.Run("Aliased object field selection", func(t *testing.T) {
			ExpectValid(t, `
      fragment aliasedObjectFieldSelection on Dog {
        tn : __typename
        otherName : name
      }
    `)
		})

		t.Run("Interface field selection", func(t *testing.T) {
			ExpectValid(t, `
      fragment interfaceFieldSelection on Pet {
        __typename
        name
      }
    `)
		})

		t.Run("Aliased interface field selection", func(t *testing.T) {
			ExpectValid(t, `
      fragment interfaceFieldSelection on Pet {
        otherName : name
      }
    `)
		})

		t.Run("Lying alias selection", func(t *testing.T) {
			ExpectValid(t, `
      fragment lyingAliasSelection on Dog {
        name : nickname
      }
    `)
		})

		t.Run("Ignores fields on unknown type", func(t *testing.T) {
			ExpectValid(t, `
      fragment unknownSelection on UnknownType {
        unknownField
      }
    `)
		})

		t.Run("reports errors when type is known again", func(t *testing.T) {
			ExpectErrors(t, `
      fragment typeKnownAgain on Pet {
        unknown_pet_field {
          ... on Cat {
            unknown_cat_field
          }
        }
      }
    `)([]Err{
				{
					message:   `Cannot query field "unknown_pet_field" on type "Pet".`,
					locations: []Loc{{line: 3, column: 9}},
				},
				{
					message:   `Cannot query field "unknown_cat_field" on type "Cat".`,
					locations: []Loc{{line: 5, column: 13}},
				},
			})
		})

		t.Run("Field not defined on fragment", func(t *testing.T) {
			ExpectErrors(t, `
      fragment fieldNotDefined on Dog {
        meowVolume
      }
    `)([]Err{
				{
					message:   `Cannot query field "meowVolume" on type "Dog". Did you mean "barkVolume"?`,
					locations: []Loc{{line: 3, column: 9}},
				},
			})
		})

		t.Run("Ignores deeply unknown field", func(t *testing.T) {
			ExpectErrors(t, `
      fragment deepFieldNotDefined on Dog {
        unknown_field {
          deeper_unknown_field
        }
      }
    `)([]Err{
				{
					message:   `Cannot query field "unknown_field" on type "Dog".`,
					locations: []Loc{{line: 3, column: 9}},
				},
			})
		})

		t.Run("Sub-field not defined", func(t *testing.T) {
			ExpectErrors(t, `
      fragment subFieldNotDefined on Human {
        pets {
          unknown_field
        }
      }
    `)([]Err{
				{
					message:   `Cannot query field "unknown_field" on type "Pet".`,
					locations: []Loc{{line: 4, column: 11}},
				},
			})
		})

		t.Run("Field not defined on inline fragment", func(t *testing.T) {
			ExpectErrors(t, `
      fragment fieldNotDefined on Pet {
        ... on Dog {
          meowVolume
        }
      }
    `)([]Err{
				{
					message:   `Cannot query field "meowVolume" on type "Dog". Did you mean "barkVolume"?`,
					locations: []Loc{{line: 4, column: 11}},
				},
			})
		})

		t.Run("Aliased field target not defined", func(t *testing.T) {
			ExpectErrors(t, `
      fragment aliasedFieldTargetNotDefined on Dog {
        volume : mooVolume
      }
    `)([]Err{
				{
					message:   `Cannot query field "mooVolume" on type "Dog". Did you mean "barkVolume"?`,
					locations: []Loc{{line: 3, column: 9}},
				},
			})
		})

		t.Run("Aliased lying field target not defined", func(t *testing.T) {
			ExpectErrors(t, `
      fragment aliasedLyingFieldTargetNotDefined on Dog {
        barkVolume : kawVolume
      }
    `)([]Err{
				{
					message:   `Cannot query field "kawVolume" on type "Dog". Did you mean "barkVolume"?`,
					locations: []Loc{{line: 3, column: 9}},
				},
			})
		})

		t.Run("Not defined on interface", func(t *testing.T) {
			ExpectErrors(t, `
      fragment notDefinedOnInterface on Pet {
        tailLength
      }
    `)([]Err{
				{
					message:   `Cannot query field "tailLength" on type "Pet".`,
					locations: []Loc{{line: 3, column: 9}},
				},
			})
		})

		t.Run("Defined on implementors but not on interface", func(t *testing.T) {
			ExpectErrors(t, `
      fragment definedOnImplementorsButNotInterface on Pet {
        nickname
      }
    `)([]Err{
				{
					message:   `Cannot query field "nickname" on type "Pet". Did you mean to use an inline fragment on "Cat" or "Dog"?`,
					locations: []Loc{{line: 3, column: 9}},
				},
			})
		})

		t.Run("Meta field selection on union", func(t *testing.T) {
			ExpectValid(t, `
      fragment directFieldSelectionOnUnion on CatOrDog {
        __typename
      }
    `)
		})

		t.Run("Direct field selection on union", func(t *testing.T) {
			ExpectErrors(t, `
      fragment directFieldSelectionOnUnion on CatOrDog {
        directField
      }
    `)([]Err{
				{
					message:   `Cannot query field "directField" on type "CatOrDog".`,
					locations: []Loc{{line: 3, column: 9}},
				},
			})
		})

		t.Run("Defined on implementors queried on union", func(t *testing.T) {
			ExpectErrors(t, `
      fragment definedOnImplementorsQueriedOnUnion on CatOrDog {
        name
      }
    `)([]Err{
				{
					message:   `Cannot query field "name" on type "CatOrDog". Did you mean to use an inline fragment on "Being", "Pet", "Canine", "Cat", or "Dog"?`,
					locations: []Loc{{line: 3, column: 9}},
				},
			})
		})

		t.Run("valid field in inline fragment", func(t *testing.T) {
			ExpectValid(t, `
      fragment objectFieldSelection on Pet {
        ... on Dog {
          name
        }
        ... {
          name
        }
      }
    `)
		})

		t.Run("Fields on correct type error message", func(t *testing.T) {
			ExpectErrorMessage := func(t *testing.T, schema string, queryStr string) MessageCompare {
				return ExpectValidationErrorMessage(t, schema, queryStr)
			}
			t.Run("Works with no suggestions", func(t *testing.T) {
				schema := BuildSchema(`
        type T {
          fieldWithVeryLongNameThatWillNeverBeSuggested: String
        }
        type Query { t: T }
      `)

				ExpectErrorMessage(t, schema, "{ t { f } }")(
					`Cannot query field "f" on type "T".`,
				)
			})

			t.Run("Works with no small numbers of type suggestions", func(t *testing.T) {
				schema := BuildSchema(`
        union T = A | B
        type Query { t: T }

        type A { f: String }
        type B { f: String }
      `)

				ExpectErrorMessage(t, schema, "{ t { f } }")(
					`Cannot query field "f" on type "T". Did you mean to use an inline fragment on "A" or "B"?`,
				)
			})

			t.Run("Works with no small numbers of field suggestions", func(t *testing.T) {
				schema := BuildSchema(`
        type T {
          y: String
          z: String
        }
        type Query { t: T }
      `)

				ExpectErrorMessage(t, schema, "{ t { f } }")(
					`Cannot query field "f" on type "T". Did you mean "y" or "z"?`,
				)
			})

			t.Run("Only shows one set of suggestions at a time, preferring types", func(t *testing.T) {
				schema := BuildSchema(`
        interface T {
          y: String
          z: String
        }
        type Query { t: T }

        type A implements T {
          f: String
          y: String
          z: String
        }
        type B implements T {
          f: String
          y: String
          z: String
        }
      `)

				ExpectErrorMessage(t, schema, "{ t { f } }")(
					`Cannot query field "f" on type "T". Did you mean to use an inline fragment on "A" or "B"?`,
				)
			})

			t.Run("Sort type suggestions based on inheritance order", func(t *testing.T) {
				schema := BuildSchema(`
        interface T { bar: String }
        type Query { t: T }

        interface Z implements T {
          foo: String
          bar: String
        }

        interface Y implements Z & T {
          foo: String
          bar: String
        }

        type X implements Y & Z & T {
          foo: String
          bar: String
        }
      `)

				ExpectErrorMessage(t, schema, "{ t { foo } }")(
					`Cannot query field "foo" on type "T". Did you mean to use an inline fragment on "Z", "Y", or "X"?`,
				)
			})

			t.Run("Limits lots of type suggestions", func(t *testing.T) {
				schema := BuildSchema(`
        union T = A | B | C | D | E | F
        type Query { t: T }

        type A { f: String }
        type B { f: String }
        type C { f: String }
        type D { f: String }
        type E { f: String }
        type F { f: String }
      `)

				ExpectErrorMessage(t, schema, "{ t { f } }")(
					`Cannot query field "f" on type "T". Did you mean to use an inline fragment on "A", "B", "C", "D", or "E"?`,
				)
			})

			t.Run("Limits lots of field suggestions", func(t *testing.T) {
				schema := BuildSchema(`
        type T {
          u: String
          v: String
          w: String
          x: String
          y: String
          z: String
        }
        type Query { t: T }
      `)

				ExpectErrorMessage(t, schema, "{ t { f } }")(
					`Cannot query field "f" on type "T". Did you mean "u", "v", "w", "x", or "y"?`,
				)
			})
		})
	})

}
