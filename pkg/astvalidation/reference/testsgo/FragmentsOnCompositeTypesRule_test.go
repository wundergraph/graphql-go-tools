package testsgo

import (
	"testing"
)

func TestFragmentsOnCompositeTypesRule(t *testing.T) {
	t.Skip()

	ExpectErrors := func(t *testing.T, queryStr string) ResultCompare {
		return ExpectValidationErrors(t, FragmentsOnCompositeTypesRule, queryStr)
	}

	ExpectValid := func(t *testing.T, queryStr string) {
		ExpectErrors(t, queryStr)([]Err{})
	}

	t.Run("Validate: Fragments on composite types", func(t *testing.T) {
		t.Run("object is valid fragment type", func(t *testing.T) {
			ExpectValid(t, `
      fragment validFragment on Dog {
        barks
      }
    `)
		})

		t.Run("interface is valid fragment type", func(t *testing.T) {
			ExpectValid(t, `
      fragment validFragment on Pet {
        name
      }
    `)
		})

		t.Run("object is valid inline fragment type", func(t *testing.T) {
			ExpectValid(t, `
      fragment validFragment on Pet {
        ... on Dog {
          barks
        }
      }
    `)
		})

		t.Run("interface is valid inline fragment type", func(t *testing.T) {
			ExpectValid(t, `
      fragment validFragment on Mammal {
        ... on Canine {
          name
        }
      }
    `)
		})

		t.Run("inline fragment without type is valid", func(t *testing.T) {
			ExpectValid(t, `
      fragment validFragment on Pet {
        ... {
          name
        }
      }
    `)
		})

		t.Run("union is valid fragment type", func(t *testing.T) {
			ExpectValid(t, `
      fragment validFragment on CatOrDog {
        __typename
      }
    `)
		})

		t.Run("scalar is invalid fragment type", func(t *testing.T) {
			ExpectErrors(t, `
      fragment scalarFragment on Boolean {
        bad
      }
    `)([]Err{
				{
					message:   `Fragment "scalarFragment" cannot condition on non composite type "Boolean".`,
					locations: []Loc{{line: 2, column: 34}},
				},
			})
		})

		t.Run("enum is invalid fragment type", func(t *testing.T) {
			ExpectErrors(t, `
      fragment scalarFragment on FurColor {
        bad
      }
    `)([]Err{
				{
					message:   `Fragment "scalarFragment" cannot condition on non composite type "FurColor".`,
					locations: []Loc{{line: 2, column: 34}},
				},
			})
		})

		t.Run("input object is invalid fragment type", func(t *testing.T) {
			ExpectErrors(t, `
      fragment inputFragment on ComplexInput {
        stringField
      }
    `)([]Err{
				{
					message:   `Fragment "inputFragment" cannot condition on non composite type "ComplexInput".`,
					locations: []Loc{{line: 2, column: 33}},
				},
			})
		})

		t.Run("scalar is invalid inline fragment type", func(t *testing.T) {
			ExpectErrors(t, `
      fragment invalidFragment on Pet {
        ... on String {
          barks
        }
      }
    `)([]Err{
				{
					message:   `Fragment cannot condition on non composite type "String".`,
					locations: []Loc{{line: 3, column: 16}},
				},
			})
		})
	})

}
