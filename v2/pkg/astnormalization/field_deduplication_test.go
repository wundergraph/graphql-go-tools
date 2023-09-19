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
}
