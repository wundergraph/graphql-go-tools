package sdlmerge

import "testing"

func TestRemoveDuplicateFieldedValueTypes(t *testing.T) {
	t.Run("Same name empty interfaces are merged into a single interface", func(t *testing.T) {
		run(t, newRemoveDuplicateFieldedValueTypesVisitor(), `
			interface Trainer {
			}

			interface Trainer {
			}
			`, `
			interface Trainer {
			}
			`,
		)
	})

	t.Run("Same name empty interfaces are merged into a single interface", func(t *testing.T) {
		run(t, newRemoveDuplicateFieldedValueTypesVisitor(), `
			interface Trainer {
			}

			interface Trainer {
			}
			`, `
			interface Trainer {
			}
			`,
		)
	})

	t.Run("Identical same name interfaces are merged into a single interface", func(t *testing.T) {
		run(t, newRemoveDuplicateFieldedValueTypesVisitor(), `
			interface Trainer {
				name: String
				age: int
			}

			interface Trainer {
				name: String
				age: int
			}

			interface Trainer {
				name: String
				age: int
			}
			`, `
			interface Trainer {
				name: String
				age: int
			}
			`,
		)
	})
}
