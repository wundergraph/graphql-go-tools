package astnormalization

import "testing"

func TestInlineFragmentAddOnType(t *testing.T) {
	// NOTE: we are internally adding on type condition name for the planner needs
	// but printed query remains the same

	t.Run("simple", func(t *testing.T) {
		run(t, inlineFragmentAddOnType, testDefinition, `
					query dog {
						dog {
							... {
								name
							}
						}
					}`,
			`
					query dog {
						dog {
							... {
								name
							}
						}
					}`)
	})
	t.Run("with interface type", func(t *testing.T) {
		run(t, inlineFragmentAddOnType, testDefinition, `
					query pet {
						pet {
							... {
								name
							}
						}
					}`,
			`
					query pet {
						pet {
							... {
								name
							}
						}
					}`)
	})
}
