package astnormalization

import (
	"testing"
)

func TestDeferEnsureTypename(t *testing.T) {
	t.Run("mixed deferred and non-deferred fields - no placeholder needed", func(t *testing.T) {
		run(t, deferEnsureTypename, testDefinition, `
				{
					user {
						id
						name @__defer_internal(id: "1")
					}
				}`, `
				{
					user {
						id
						name @__defer_internal(id: "1")
					}
				}`)
	})

	t.Run("all fields deferred, parent not deferred - plain placeholder added", func(t *testing.T) {
		run(t, deferEnsureTypename, testDefinition, `
				{
					user {
						name @__defer_internal(id: "1")
						age @__defer_internal(id: "1")
					}
				}`, `
				{
					user {
						name @__defer_internal(id: "1")
						age @__defer_internal(id: "1")
						___typename: __typename
					}
				}`)
	})

	t.Run("all fields deferred with different ids, parent not deferred - plain placeholder added", func(t *testing.T) {
		run(t, deferEnsureTypename, testDefinition, `
				{
					user {
						name @__defer_internal(id: "1")
						age @__defer_internal(id: "2")
					}
				}`, `
				{
					user {
						name @__defer_internal(id: "1")
						age @__defer_internal(id: "2")
						___typename: __typename
					}
				}`)
	})

	t.Run("all fields deferred, parent deferred with same id - intersection, no placeholder", func(t *testing.T) {
		run(t, deferEnsureTypename, testDefinition, `
				{
					user @__defer_internal(id: "1") {
						name @__defer_internal(id: "1")
						age @__defer_internal(id: "2")
					}
				}`, `
				{
					user @__defer_internal(id: "1") {
						name @__defer_internal(id: "1")
						age @__defer_internal(id: "2")
					}
				}`)
	})

	t.Run("all fields deferred, parent deferred with different id - no intersection, placeholder with parent id added", func(t *testing.T) {
		run(t, deferEnsureTypename, testDefinition, `
				{
					user @__defer_internal(id: "1") {
						name @__defer_internal(id: "2")
						age @__defer_internal(id: "3")
					}
				}`, `
				{
					user @__defer_internal(id: "1") {
						name @__defer_internal(id: "2")
						age @__defer_internal(id: "3")
						___typename: __typename @__defer_internal(id: "1")
					}
				}`)
	})

}