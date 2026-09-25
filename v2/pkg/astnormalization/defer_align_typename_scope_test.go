package astnormalization

import "testing"

func TestDeferAlignTypenameScope(t *testing.T) {
	t.Run("deferred __typename on non-deferred object - defer stripped", func(t *testing.T) {
		run(t, deferAlignTypenameScope, testDefinition, `
			query dog {
				dog {
					__typename @__defer_internal(id: 1)
					name @__defer_internal(id: 1)
				}
			}`,
			`
			query dog {
				dog {
					__typename
					name @__defer_internal(id: 1)
				}
			}`, withIndent())
	})

	t.Run("deferred __typename on object in same defer - unchanged", func(t *testing.T) {
		run(t, deferAlignTypenameScope, testDefinition, `
			query dog {
				dog @__defer_internal(id: 1) {
					__typename @__defer_internal(id: 1)
					name @__defer_internal(id: 1)
				}
			}`,
			`
			query dog {
				dog @__defer_internal(id: 1) {
					__typename @__defer_internal(id: 1)
					name @__defer_internal(id: 1)
				}
			}`, withIndent())
	})

	t.Run("__typename deferred deeper than its object - realigned to object scope", func(t *testing.T) {
		run(t, deferAlignTypenameScope, testDefinition, `
			query dog {
				dog @__defer_internal(id: 1) {
					extra @__defer_internal(id: 1) {
						__typename @__defer_internal(id: 2, parentDeferId: 1)
						string @__defer_internal(id: 2, parentDeferId: 1)
					}
				}
			}`,
			`
			query dog {
				dog @__defer_internal(id: 1) {
					extra @__defer_internal(id: 1) {
						__typename @__defer_internal(id: 1)
						string @__defer_internal(id: 2, parentDeferId: 1)
					}
				}
			}`, withIndent())
	})

	t.Run("root-level deferred __typename - defer stripped", func(t *testing.T) {
		run(t, deferAlignTypenameScope, testDefinition, `
			query dog {
				__typename @__defer_internal(id: 1)
				dog {
					name
				}
			}`,
			`
			query dog {
				__typename
				dog {
					name
				}
			}`, withIndent())
	})

	t.Run("no deferred fields - no change", func(t *testing.T) {
		run(t, deferAlignTypenameScope, testDefinition, `
			query dog {
				dog {
					__typename
					name
				}
			}`,
			`
			query dog {
				dog {
					__typename
					name
				}
			}`, withIndent())
	})
}
