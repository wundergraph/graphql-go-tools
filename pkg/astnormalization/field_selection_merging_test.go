package astnormalization

import "testing"

func TestMergeFieldSelections(t *testing.T) {
	t.Run("depth 1", func(t *testing.T) {
		run(t, mergeFieldSelections, testDefinition, `
					query conflictingBecauseAlias {
						dog {
							extra { string }
							extra { noString: string }
						}
					}`, `
					query conflictingBecauseAlias {
						dog {
							extra { 
								string
								noString: string
							}
						}
					}`)
	})
	t.Run("depth 2", func(t *testing.T) {
		run(t, mergeFieldSelections, testDefinition, `
					query conflictingBecauseAlias {
						dog {
							extra { string }
							extra { string: noString }
						}
						dog {
							extra { string }
							extra { string: noString }
						}
					}`, `
					query conflictingBecauseAlias {
						dog {
							extra { 
								string
								string: noString
								string
								string: noString
							}
						}
					}`)
	})
	t.Run("aliased", func(t *testing.T) {
		t.Run("aliased", func(t *testing.T) {
			run(t, mergeFieldSelections, testDefinition, `
					query conflictingBecauseAlias {
						dog {
							x: extras { string }
							x: mustExtras { string }
						}
					}`, `
					query conflictingBecauseAlias {
						dog {
							x: extras { string }
							x: mustExtras { string }
						}
					}`)
		})
	})
	t.Run("fields with directives", func(t *testing.T) {
		run(t, mergeFieldSelections, testDefinition, `
					{
						field @skip(if: $foo) {
							subfieldA
						}
						field @skip(if: $bar) {
							subfieldB
						}
					}`, `
					{
						field @skip(if: $foo) {
							subfieldA
						}
						field @skip(if: $bar) {
							subfieldB
						}
					}`)
	})
}
