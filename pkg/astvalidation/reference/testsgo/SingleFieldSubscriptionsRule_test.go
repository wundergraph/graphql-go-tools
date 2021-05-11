package testsgo

import (
	"testing"
)

func TestSingleFieldSubscriptionsRule(t *testing.T) {

	expectErrors := func(queryStr string) ResultCompare {
		return ExpectValidationErrors("SingleFieldSubscriptionsRule", queryStr)
	}

	expectValid := func(queryStr string) {
		expectErrors(queryStr)(t, []Err{})
	}

	t.Run("Validate: Subscriptions with single field", func(t *testing.T) {
		t.Run("valid subscription", func(t *testing.T) {
			expectValid(`
      subscription ImportantEmails {
        importantEmails
      }
    `)
		})

		t.Run("fails with more than one root field", func(t *testing.T) {
			expectErrors(`
      subscription ImportantEmails {
        importantEmails
        notImportantEmails
      }
    `)(t, []Err{
				{
					message:   `Subscription "ImportantEmails" must select only one top level field.`,
					locations: []Loc{{line: 4, column: 9}},
				},
			})
		})

		t.Run("fails with more than one root field including introspection", func(t *testing.T) {
			expectErrors(`
      subscription ImportantEmails {
        importantEmails
        __typename
      }
    `)(t, []Err{
				{
					message:   `Subscription "ImportantEmails" must select only one top level field.`,
					locations: []Loc{{line: 4, column: 9}},
				},
			})
		})

		t.Run("fails with many more than one root field", func(t *testing.T) {
			expectErrors(`
      subscription ImportantEmails {
        importantEmails
        notImportantEmails
        spamEmails
      }
    `)(t, []Err{
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
			expectErrors(`
      subscription {
        importantEmails
        notImportantEmails
      }
    `)(t, []Err{
				{
					message:   "Anonymous Subscription must select only one top level field.",
					locations: []Loc{{line: 4, column: 9}},
				},
			})
		})
	})

}
