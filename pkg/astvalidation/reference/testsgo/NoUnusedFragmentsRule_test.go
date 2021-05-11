package testsgo

import (
	"testing"
)

func TestNoUnusedFragmentsRule(t *testing.T) {

	expectErrors := func(queryStr string) ResultCompare {
		return ExpectValidationErrors("NoUnusedFragmentsRule", queryStr)
	}

	expectValid := func(queryStr string) {
		expectErrors(queryStr)(t, []Err{})
	}

	t.Run("Validate: No unused fragments", func(t *testing.T) {
		t.Run("all fragment names are used", func(t *testing.T) {
			expectValid(`
      {
        human(id: 4) {
          ...HumanFields1
          ... on Human {
            ...HumanFields2
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

		t.Run("all fragment names are used by multiple operations", func(t *testing.T) {
			expectValid(`
      query Foo {
        human(id: 4) {
          ...HumanFields1
        }
      }
      query Bar {
        human(id: 4) {
          ...HumanFields2
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

		t.Run("contains unknown fragments", func(t *testing.T) {
			expectErrors(`
      query Foo {
        human(id: 4) {
          ...HumanFields1
        }
      }
      query Bar {
        human(id: 4) {
          ...HumanFields2
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
      fragment Unused1 on Human {
        name
      }
      fragment Unused2 on Human {
        name
      }
    `)(t, []Err{
				{
					message:   `Fragment "Unused1" is never used.`,
					locations: []Loc{{line: 22, column: 7}},
				},
				{
					message:   `Fragment "Unused2" is never used.`,
					locations: []Loc{{line: 25, column: 7}},
				},
			})
		})

		t.Run("contains unknown fragments with ref cycle", func(t *testing.T) {
			expectErrors(`
      query Foo {
        human(id: 4) {
          ...HumanFields1
        }
      }
      query Bar {
        human(id: 4) {
          ...HumanFields2
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
      fragment Unused1 on Human {
        name
        ...Unused2
      }
      fragment Unused2 on Human {
        name
        ...Unused1
      }
    `)(t, []Err{
				{
					message:   `Fragment "Unused1" is never used.`,
					locations: []Loc{{line: 22, column: 7}},
				},
				{
					message:   `Fragment "Unused2" is never used.`,
					locations: []Loc{{line: 26, column: 7}},
				},
			})
		})

		t.Run("contains unknown and undef fragments", func(t *testing.T) {
			expectErrors(`
      query Foo {
        human(id: 4) {
          ...bar
        }
      }
      fragment foo on Human {
        name
      }
    `)(t, []Err{
				{
					message:   `Fragment "foo" is never used.`,
					locations: []Loc{{line: 7, column: 7}},
				},
			})
		})
	})

}
