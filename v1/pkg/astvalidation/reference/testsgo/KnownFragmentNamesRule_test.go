package testsgo

import (
	"testing"
)

func TestKnownFragmentNamesRule(t *testing.T) {
	t.Skip()

	ExpectErrors := func(t *testing.T, queryStr string) ResultCompare {
		return ExpectValidationErrors(t, KnownFragmentNamesRule, queryStr)
	}

	ExpectValid := func(t *testing.T, queryStr string) {
		ExpectErrors(t, queryStr)([]Err{})
	}

	t.Run("Validate: Known fragment names", func(t *testing.T) {
		t.Run("known fragment names are valid", func(t *testing.T) {
			ExpectValid(t, `
      {
        human(id: 4) {
          ...HumanFields1
          ... on Human {
            ...HumanFields2
          }
          ... {
            name
          }
        }
      }
      fragment HumanFields1 on Human {
        name
        ...HumanFields3
      }
      fragment HumanFields2 on Human {
        name
      }
      fragment HumanFields3 on Human {
        name
      }
    `)
		})

		t.Run("unknown fragment names are invalid", func(t *testing.T) {
			ExpectErrors(t, `
      {
        human(id: 4) {
          ...UnknownFragment1
          ... on Human {
            ...UnknownFragment2
          }
        }
      }
      fragment HumanFields on Human {
        name
        ...UnknownFragment3
      }
    `)([]Err{
				{
					message:   `Unknown fragment "UnknownFragment1".`,
					locations: []Loc{{line: 4, column: 14}},
				},
				{
					message:   `Unknown fragment "UnknownFragment2".`,
					locations: []Loc{{line: 6, column: 16}},
				},
				{
					message:   `Unknown fragment "UnknownFragment3".`,
					locations: []Loc{{line: 12, column: 12}},
				},
			})
		})
	})

}
