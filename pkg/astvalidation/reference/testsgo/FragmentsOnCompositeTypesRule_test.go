package testsgo

import (
	"testing"
)

func TestFragmentsOnCompositeTypesRule(t *testing.T) {

	expectErrors := func(queryStr string) ResultCompare {
		return ExpectValidationErrors("FragmentsOnCompositeTypesRule", queryStr)
	}

	expectValid := func(queryStr string) {
		expectErrors(queryStr)(t, []Err{})
	}

	t.Run("Validate: Fragments on composite types", func(t *testing.T) {
		t.Run("object is valid fragment type", func(t *testing.T) {
			expectValid(`
      fragment validFragment on Dog {
        barks
      }
    `)
		})

		t.Run("interface is valid fragment type", func(t *testing.T) {
			expectValid(`
      fragment validFragment on Pet {
        name
      }
    `)
		})

		t.Run("object is valid inline fragment type", func(t *testing.T) {
			expectValid(`
      fragment validFragment on Pet {
        ... on Dog {
          barks
        }
      }
    `)
		})

		t.Run("interface is valid inline fragment type", func(t *testing.T) {
			expectValid(`
      fragment validFragment on Mammal {
        ... on Canine {
          name
        }
      }
    `)
		})

		t.Run("inline fragment without type is valid", func(t *testing.T) {
			expectValid(`
      fragment validFragment on Pet {
        ... {
          name
        }
      }
    `)
		})

		t.Run("union is valid fragment type", func(t *testing.T) {
			expectValid(`
      fragment validFragment on CatOrDog {
        __typename
      }
    `)
		})

		t.Run("scalar is invalid fragment type", func(t *testing.T) {
			expectErrors(`
      fragment scalarFragment on Boolean {
        bad
      }
    `)(t, []Err{
				{
					message:   `Fragment "scalarFragment" cannot condition on non composite type "Boolean".`,
					locations: []Loc{{line: 2, column: 34}},
				},
			})
		})

		t.Run("enum is invalid fragment type", func(t *testing.T) {
			expectErrors(`
      fragment scalarFragment on FurColor {
        bad
      }
    `)(t, []Err{
				{
					message:   `Fragment "scalarFragment" cannot condition on non composite type "FurColor".`,
					locations: []Loc{{line: 2, column: 34}},
				},
			})
		})

		t.Run("input object is invalid fragment type", func(t *testing.T) {
			expectErrors(`
      fragment inputFragment on ComplexInput {
        stringField
      }
    `)(t, []Err{
				{
					message:   `Fragment "inputFragment" cannot condition on non composite type "ComplexInput".`,
					locations: []Loc{{line: 2, column: 33}},
				},
			})
		})

		t.Run("scalar is invalid inline fragment type", func(t *testing.T) {
			expectErrors(`
      fragment invalidFragment on Pet {
        ... on String {
          barks
        }
      }
    `)(t, []Err{
				{
					message:   `Fragment cannot condition on non composite type "String".`,
					locations: []Loc{{line: 3, column: 16}},
				},
			})
		})
	})

}
