package testsgo

import (
	"testing"
)

func TestVariablesAreInputTypesRule(t *testing.T) {

	ExpectErrors := func(t *testing.T, queryStr string) ResultCompare {
		return ExpectValidationErrors(t, VariablesAreInputTypesRule, queryStr)
	}

	ExpectValid := func(t *testing.T, queryStr string) {
		ExpectErrors(t, queryStr)([]Err{})
	}

	t.Run("Validate: Variables are input types", func(t *testing.T) {
		t.Run("input types are valid", func(t *testing.T) {
			ExpectValid(t, `
      query Foo($a: String, $b: [Boolean!]!, $c: ComplexInput) {
        field(a: $a, b: $b, c: $c)
      }
    `)
		})

		t.Run("output types are invalid", func(t *testing.T) {
			ExpectErrors(t, `
      query Foo($a: Dog, $b: [[CatOrDog!]]!, $c: Pet) {
        field(a: $a, b: $b, c: $c)
      }
    `)([]Err{
				{
					locations: []Loc{{line: 2, column: 21}},
					message:   `Variable "$a" cannot be non-input type "Dog".`,
				},
				{
					locations: []Loc{{line: 2, column: 30}},
					message:   `Variable "$b" cannot be non-input type "[[CatOrDog!]]!".`,
				},
				{
					locations: []Loc{{line: 2, column: 50}},
					message:   `Variable "$c" cannot be non-input type "Pet".`,
				},
			})
		})
	})

}
