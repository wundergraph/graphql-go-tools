package astnormalization

import "testing"

func TestDeferPopulateParentIds(t *testing.T) {
	t.Run("no deferred fields - no change", func(t *testing.T) {
		run(t, deferPopulateParentIds, testDefinition, `
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
			}`, withIndent())
	})

	t.Run("single top-level defer - no parent to assign", func(t *testing.T) {
		run(t, deferPopulateParentIds, testDefinition, `
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
			}`, withIndent())
	})

	t.Run("genuinely nested defers - inner gets parentDeferId from outer", func(t *testing.T) {
		run(t, deferPopulateParentIds, testDefinition, `
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
			}`, withIndent())
	})

	t.Run("parallel defers merged into one tree - sibling gets parentDeferId from winning group", func(t *testing.T) {
		run(t, deferPopulateParentIds, testDefinition, `
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
			}`, withIndent())
	})

	t.Run("parallel defers at depth - nested sibling gets correct parentDeferId", func(t *testing.T) {
		run(t, deferPopulateParentIds, testDefinition, `
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
			}`, withIndent())
	})

	t.Run("stale parentDeferId discarded into non-deferred path - parent removed", func(t *testing.T) {
		// parentDeferId 1 references a defer that was merged into a non-deferred
		// path during field merging, so id 1 no longer exists. With no enclosing
		// deferred ancestor the parent must be dropped.
		run(t, deferPopulateParentIds, testDefinition, `
			query dog {
				dog {
					extra {
						string @__defer_internal(id: 2, parentDeferId: 1)
					}
				}
			}`,
			`
			query dog {
				dog {
					extra {
						string @__defer_internal(id: 2)
					}
				}
			}`, withIndent())
	})

	t.Run("stale parentDeferId merged into enclosing defer - parent repaired", func(t *testing.T) {
		// parentDeferId 2 references a defer that was merged into the enclosing
		// defer (id 1) during field merging, so id 2 no longer exists. The parent
		// must be repaired to the nearest enclosing deferred ancestor.
		run(t, deferPopulateParentIds, testDefinition, `
			query dog {
				dog @__defer_internal(id: 1) {
					extra @__defer_internal(id: 1) {
						string @__defer_internal(id: 3, parentDeferId: 2)
					}
				}
			}`,
			`
			query dog {
				dog @__defer_internal(id: 1) {
					extra @__defer_internal(id: 1) {
						string @__defer_internal(id: 3, parentDeferId: 1)
					}
				}
			}`, withIndent())
	})

	t.Run("valid sibling parentDeferId still present - preserved", func(t *testing.T) {
		// parentDeferId 1 still references a live defer (on the name field), so it
		// must be kept even though the parent is a sibling, not an ancestor.
		run(t, deferPopulateParentIds, testDefinition, `
			query dog {
				dog {
					name @__defer_internal(id: 1)
					nickname @__defer_internal(id: 2, parentDeferId: 1)
				}
			}`,
			`
			query dog {
				dog {
					name @__defer_internal(id: 1)
					nickname @__defer_internal(id: 2, parentDeferId: 1)
				}
			}`, withIndent())
	})
}
