package astnormalization

import "testing"

func TestDeDuplicateFields(t *testing.T) {
	t.Run("simple", func(t *testing.T) {
		run(t, deduplicateFields, testDefinition, `
					query conflictingBecauseAlias {
						dog {
							extra { 
								string
								string: noString
								string
								string: noString
							}
						}
					}`, `
					query conflictingBecauseAlias {
						dog {
							extra { 
								string
								string: noString
							}
						}
					}`)
	})
	t.Run("with different args", func(t *testing.T) {
		run(t, deduplicateFields, testDefinition, `
					fragment mergeIdenticalFieldsWithIdenticalValues on Dog {
						doesKnowCommand(dogCommand: 1)
						doesKnowCommand(dogCommand: 0)
					}`, `
					fragment mergeIdenticalFieldsWithIdenticalValues on Dog {
						doesKnowCommand(dogCommand: 1)
						doesKnowCommand(dogCommand: 0)
					}`)
	})

	t.Run("with internal defer", func(t *testing.T) {
		run(t, deduplicateFields, testDefinition, `
					query pet {
						pet {
							... on Dog {
								name @__defer_internal(id: "1")
								nickname @__defer_internal(id: "2", parentDeferId: "1")
								nickname @__defer_internal(id: "1")
								barkVolume @__defer_internal(id: "2", parentDeferId: "1")
							}
							... on Cat {
								name @__defer_internal(id: "4")
								name @__defer_internal(id: "3")
								name
								extra {
									bool
									bool @__defer_internal(id: "3")
								}
								meowVolume @__defer_internal(id: "4")
								meowVolume @__defer_internal(id: "3")
								nickname @__defer_internal(id: "4")
							}
						}
					}`, `
					query pet {
						pet {
							... on Dog {
								name @__defer_internal(id: "1")
								nickname @__defer_internal(id: "1")
								barkVolume @__defer_internal(id: "2", parentDeferId: "1")
							}
							... on Cat {
								name
								extra {
									bool
								}
								meowVolume @__defer_internal(id: "3")
								nickname @__defer_internal(id: "4")
							}
						}
					}`, runOptions{indent: true})
	})
}
