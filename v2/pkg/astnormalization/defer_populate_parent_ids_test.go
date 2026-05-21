package astnormalization

import "testing"

func TestDeferPopulateParentIds(t *testing.T) {
	t.Run("no deferred fields - no change", func(t *testing.T) {
		runWithOptions(t, deferPopulateParentIds, testDefinition, `
			query dog {
				dog {
					name
				}
			}`,
			`
			query dog {
				dog {
					name
				}
			}`, runOptions{indent: true})
	})

	t.Run("single top-level defer - no parent to assign", func(t *testing.T) {
		runWithOptions(t, deferPopulateParentIds, testDefinition, `
			query dog {
				dog @__defer_internal(id: 1) {
					name @__defer_internal(id: 1)
				}
			}`,
			`
			query dog {
				dog @__defer_internal(id: 1) {
					name @__defer_internal(id: 1)
				}
			}`, runOptions{indent: true})
	})

	t.Run("genuinely nested defers - inner gets parentDeferId from outer", func(t *testing.T) {
		runWithOptions(t, deferPopulateParentIds, testDefinition, `
			query dog {
				dog @__defer_internal(id: 1) {
					name @__defer_internal(id: 2)
				}
			}`,
			`
			query dog {
				dog @__defer_internal(id: 1) {
					name @__defer_internal(id: 2, parentDeferId: 1)
				}
			}`, runOptions{indent: true})
	})

	t.Run("parallel defers merged into one tree - sibling gets parentDeferId from winning group", func(t *testing.T) {
		runWithOptions(t, deferPopulateParentIds, testDefinition, `
			query dog {
				dog @__defer_internal(id: 1) {
					name @__defer_internal(id: 1)
					nickname @__defer_internal(id: 2)
				}
			}`,
			`
			query dog {
				dog @__defer_internal(id: 1) {
					name @__defer_internal(id: 1)
					nickname @__defer_internal(id: 2, parentDeferId: 1)
				}
			}`, runOptions{indent: true})
	})

	t.Run("parallel defers at depth - nested sibling gets correct parentDeferId", func(t *testing.T) {
		runWithOptions(t, deferPopulateParentIds, testDefinition, `
			query dog {
				dog @__defer_internal(id: 1) {
					extra @__defer_internal(id: 1) {
						noString @__defer_internal(id: 1)
						string @__defer_internal(id: 2)
					}
				}
			}`,
			`
			query dog {
				dog @__defer_internal(id: 1) {
					extra @__defer_internal(id: 1) {
						noString @__defer_internal(id: 1)
						string @__defer_internal(id: 2, parentDeferId: 1)
					}
				}
			}`, runOptions{indent: true})
	})
}
