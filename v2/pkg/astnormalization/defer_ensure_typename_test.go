package astnormalization

import (
	"testing"
)

func TestDeferEnsureTypename(t *testing.T) {
	t.Run("mixed fields and deferred fragments", func(t *testing.T) {
		run(t, deferEnsureTypename, testDefinition, `
				{
					user {
						id
						... @defer {
							name
						}
					}
				}`, `
				{
					user {
						id
						... @defer {
							name
						}
					}
				}`)
	})

	t.Run("only deferred fragments", func(t *testing.T) {
		run(t, deferEnsureTypename, testDefinition, `
				{
					user {
						... @defer {
							name
						}
						... @defer {
							age
						}
					}
				}`, `
				{
					user {
						... @defer {
							name
						}
						... @defer {
							age
						}
						__internal__typename_placeholder: __typename
					}
				}`)
	})

	t.Run("mixed deferred and non-deferred fragments", func(t *testing.T) {
		run(t, deferEnsureTypename, testDefinition, `
				{
					user {
						... @defer {
							name
						}
						... {
							age
						}
					}
				}`, `
				{
					user {
						... @defer {
							name
						}
						... {
							age
						}
					}
				}`)
	})

	t.Run("deferred fragment with other directives", func(t *testing.T) {
		run(t, deferEnsureTypename, testDefinition, `
				{
					user {
						... @defer @skip(if: false) {
							name
						}
					}
				}`, `
				{
					user {
						... @defer @skip(if: false) {
							name
						}
						__internal__typename_placeholder: __typename
					}
				}`)
	})
}
