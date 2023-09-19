package testsgo

import (
	"testing"
)

func TestSingleFieldSubscriptionsRule(t *testing.T) {
	t.Skip()

	ExpectErrors := func(t *testing.T, queryStr string) ResultCompare {
		return ExpectValidationErrors(t, SingleFieldSubscriptionsRule, queryStr)
	}

	ExpectValid := func(t *testing.T, queryStr string) {
		ExpectErrors(t, queryStr)([]Err{})
	}

	t.Run("Validate: Subscriptions with single field", func(t *testing.T) {
		t.Run("valid subscription", func(t *testing.T) {
			ExpectValid(t, `
      subscription ImportantEmails {
        importantEmails
      }
    `)
		})

		t.Run("fails with more than one root field", func(t *testing.T) {
			ExpectErrors(t, `
      subscription ImportantEmails {
        importantEmails
        notImportantEmails
      }
    `)([]Err{
				{
					message:   `Subscription "ImportantEmails" must select only one top level field.`,
					locations: []Loc{{line: 4, column: 9}},
				},
			})
		})

		t.Run("fails with more than one root field including introspection", func(t *testing.T) {
			ExpectErrors(t, `
      subscription ImportantEmails {
        importantEmails
        __typename
      }
    `)([]Err{
				{
					message:   `Subscription "ImportantEmails" must select only one top level field.`,
					locations: []Loc{{line: 4, column: 9}},
				},
			})
		})

		t.Run("fails with many more than one root field", func(t *testing.T) {
			ExpectErrors(t, `
      subscription ImportantEmails {
        importantEmails
        notImportantEmails
        spamEmails
      }
    `)([]Err{
				{
					message: `Subscription "ImportantEmails" must select only one top level field.`,
					locations: []Loc{
						{line: 4, column: 9},
						{line: 5, column: 9},
					},
				},
			})
		})

		t.Run("fails with more than one root field in anonymous subscriptions", func(t *testing.T) {
			ExpectErrors(t, `
      subscription {
        importantEmails
        notImportantEmails
      }
    `)([]Err{
				{
					message:   "Anonymous Subscription must select only one top level field.",
					locations: []Loc{{line: 4, column: 9}},
				},
			})
		})
	})

}
