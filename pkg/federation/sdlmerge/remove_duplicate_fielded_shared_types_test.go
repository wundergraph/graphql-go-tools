package sdlmerge

import (
	"testing"
)

func TestRemoveDuplicateFieldedValueTypes(t *testing.T) {
	t.Run("Same name empty inputs are merged into a single input", func(t *testing.T) {
		run(t, newRemoveDuplicateFieldedSharedTypesVisitor(), `
			input Trainer {
			}
	
			input Trainer {
			}
		`, `
			input Trainer {
			}
		`)
	})

	t.Run("Identical same name inputs are merged into a single input", func(t *testing.T) {
		run(t, newRemoveDuplicateFieldedSharedTypesVisitor(), `
			input Trainer {
				name: String!
				age: Int!
			}
	
			input Trainer {
				name: String!
				age: Int!
			}
	
			input Trainer {
				name: String!
				age: Int!
			}
		`, `
			input Trainer {
				name: String!
				age: Int!
			}
		`)
	})

	t.Run("Identical same name inputs are merged into a single input regardless of field order", func(t *testing.T) {
		run(t, newRemoveDuplicateFieldedSharedTypesVisitor(), `
			input Trainer {
				age: Int!
				name: String!
			}
	
			input Trainer {
				name: String!
				age: Int!
			}
	
			input Trainer {
				name: String!
				age: Int!
			}
		`, `
			input Trainer {
				age: Int!
				name: String!
			}
		`)
	})

	t.Run("Groups of identical same name inputs are respectively merged into single inputs", func(t *testing.T) {
		run(t, newRemoveDuplicateFieldedSharedTypesVisitor(), `
			input Pokemon {
				type: [Type!]!
				isEvolved: Boolean!
			}
	
			input Trainer {
				name: String!
				age: Int!
			}
	
			input Trainer {
				name: String!
				age: Int!
			}
	
			input Trainer {
				name: String!
				age: Int!
			}
	
			input Pokemon {
				type: [Type!]!
				isEvolved: Boolean!
			}
		`, `
			input Pokemon {
				type: [Type!]!
				isEvolved: Boolean!
			}
	
			input Trainer {
				name: String!
				age: Int!
			}
		`)
	})

	t.Run("Same name inputs with different nullability of fields return an error", func(t *testing.T) {
		runAndExpectError(t, newRemoveDuplicateFieldedSharedTypesVisitor(), `
			input Trainer {
				name: String!
				age: Int!
			}
	
			input Trainer {
				name: String
				age: Int!
			}
	
			input Trainer {
				name: String!
				age: Int
			}
		`, NonIdenticalSharedTypeErrorMessage("Trainer"))
	})

	t.Run("Same name inputs with different fields return an error", func(t *testing.T) {
		runAndExpectError(t, newRemoveDuplicateFieldedSharedTypesVisitor(), `
			input Trainer {
				name: String
				age: Int
			}
	
			input Trainer {
				name: String
				age: Int
			}
	
			input Trainer {
				name: String
				age: Int
				badges: Int
			}
		`, NonIdenticalSharedTypeErrorMessage("Trainer"))
	})

	t.Run("Same name inputs with a slight difference in nested field values return an error", func(t *testing.T) {
		runAndExpectError(t, newRemoveDuplicateFieldedSharedTypesVisitor(), `
			input Pokemon {
				type: [[[[Type!]]!]!]!
			}
	
			input Pokemon {
				type: [[[[Type!]]]!]!
			}
	
			input Pokemon {
				type: [[[[Type!]]!]!]!
			}
		`, NonIdenticalSharedTypeErrorMessage("Pokemon"))
	})

	t.Run("Same name inputs with different non-nullable field values return an error", func(t *testing.T) {
		runAndExpectError(t, newRemoveDuplicateFieldedSharedTypesVisitor(), `
			input Trainer {
				name: String!
				age: Int!
			}
	
			input Trainer {
				name: String!
				age: Int!
			}
	
			input Trainer {
				name: String!
				age: String!
			}
		`, NonIdenticalSharedTypeErrorMessage("Trainer"))
	})

	t.Run("Same name empty interfaces are merged into a single interface", func(t *testing.T) {
		run(t, newRemoveDuplicateFieldedSharedTypesVisitor(), `
			interface Trainer {
			}

			interface Trainer {
			}
		`, `
			interface Trainer {
			}
		`)
	})

	t.Run("Identical same name interfaces are merged into a single interface", func(t *testing.T) {
		run(t, newRemoveDuplicateFieldedSharedTypesVisitor(), `
			interface Trainer {
				name: String!
				age: Int!
			}

			interface Trainer {
				name: String!
				age: Int!
			}

			interface Trainer {
				name: String!
				age: Int!
			}
		`, `
			interface Trainer {
				name: String!
				age: Int!
			}
		`)
	})

	t.Run("Identical same name interfaces are merged into a single input regardless of field order", func(t *testing.T) {
		run(t, newRemoveDuplicateFieldedSharedTypesVisitor(), `
			interface Trainer {
				age: Int!
				name: String!
			}
	
			interface Trainer {
				name: String!
				age: Int!
			}
	
			interface Trainer {
				name: String!
				age: Int!
			}
		`, `
			interface Trainer {
				age: Int!
				name: String!
			}
		`)
	})

	t.Run("Groups of identical same name interfaces are respectively merged into single interfaces", func(t *testing.T) {
		run(t, newRemoveDuplicateFieldedSharedTypesVisitor(), `
			interface Pokemon {
				type: [Type!]!
				isEvolved: Boolean!
			}

			interface Trainer {
				name: String!
				age: Int!
			}

			interface Trainer {
				name: String!
				age: Int!
			}

			interface Trainer {
				name: String!
				age: Int!
			}

			interface Pokemon {
				type: [Type!]!
				isEvolved: Boolean!
			}
		`, `
			interface Pokemon {
				type: [Type!]!
				isEvolved: Boolean!
			}

			interface Trainer {
				name: String!
				age: Int!
			}
		`)
	})

	t.Run("Same name interfaces with different nullability of fields return an error", func(t *testing.T) {
		runAndExpectError(t, newRemoveDuplicateFieldedSharedTypesVisitor(), `
			interface Trainer {
				name: String!
				age: Int!
			}

			interface Trainer {
				name: String
				age: Int!
			}

			interface Trainer {
				name: String!
				age: Int
			}
		`, NonIdenticalSharedTypeErrorMessage("Trainer"))
	})

	t.Run("Same name interfaces with different fields return an error", func(t *testing.T) {
		runAndExpectError(t, newRemoveDuplicateFieldedSharedTypesVisitor(), `
			interface Trainer {
				name: String
				age: Int
			}

			interface Trainer {
				name: String
				age: Int
			}

			interface Trainer {
				name: String
				age: Int
				badges: Int
			}
		`, NonIdenticalSharedTypeErrorMessage("Trainer"))
	})

	t.Run("Same name interfaces with a slight difference in nested field values return an error", func(t *testing.T) {
		runAndExpectError(t, newRemoveDuplicateFieldedSharedTypesVisitor(), `
			interface Pokemon {
				type: [[[[Type!]]!]!]!
			}
	
			interface Pokemon {
				type: [[[[Type!]]]!]!
			}
	
			interface Pokemon {
				type: [[[[Type!]]!]!]!
			}
		`, NonIdenticalSharedTypeErrorMessage("Pokemon"))
	})

	t.Run("Same name interfaces with different non-nullable field values return an error", func(t *testing.T) {
		runAndExpectError(t, newRemoveDuplicateFieldedSharedTypesVisitor(), `
			interface Trainer {
				name: String!
				age: Int!
			}
	
			interface Trainer {
				name: String!
				age: Int!
			}
	
			interface Trainer {
				name: String!
				age: String!
			}
		`, NonIdenticalSharedTypeErrorMessage("Trainer"))
	})

	t.Run("Same name empty objects are merged into a single object", func(t *testing.T) {
		run(t, newRemoveDuplicateFieldedSharedTypesVisitor(), `
			type Trainer {
			}

			type Trainer {
			}
		`, `
			type Trainer {
			}
		`)
	})

	t.Run("Identical same name objects are merged into a single object", func(t *testing.T) {
		run(t, newRemoveDuplicateFieldedSharedTypesVisitor(), `
			type Trainer {
				name: String!
				age: Int!
			}

			type Trainer {
				name: String!
				age: Int!
			}

			type Trainer {
				name: String!
				age: Int!
			}
		`, `
			type Trainer {
				name: String!
				age: Int!
			}
		`)
	})

	t.Run("Identical same name objects are merged into a single input regardless of field order", func(t *testing.T) {
		run(t, newRemoveDuplicateFieldedSharedTypesVisitor(), `
			type Trainer {
				age: Int!
				name: String!
			}
	
			type Trainer {
				name: String!
				age: Int!
			}
	
			type Trainer {
				name: String!
				age: Int!
			}
		`, `
			type Trainer {
				age: Int!
				name: String!
			}
		`)
	})

	t.Run("Groups of identical same name objects are respectively merged into single objects", func(t *testing.T) {
		run(t, newRemoveDuplicateFieldedSharedTypesVisitor(), `
			type Pokemon {
				type: [Type!]!
				isEvolved: Boolean!
			}

			type Trainer {
				name: String!
				age: Int!
			}

			type Trainer {
				name: String!
				age: Int!
			}

			type Trainer {
				name: String!
				age: Int!
			}

			type Pokemon {
				type: [Type!]!
				isEvolved: Boolean!
			}
		`, `
			type Pokemon {
				type: [Type!]!
				isEvolved: Boolean!
			}

			type Trainer {
				name: String!
				age: Int!
			}
		`)
	})

	t.Run("Same name objects with different nullability of fields return an error", func(t *testing.T) {
		runAndExpectError(t, newRemoveDuplicateFieldedSharedTypesVisitor(), `
			type Trainer {
				name: String!
				age: Int!
			}

			type Trainer {
				name: String
				age: Int!
			}

			type Trainer {
				name: String!
				age: Int
			}
		`, NonIdenticalSharedTypeErrorMessage("Trainer"))
	})

	t.Run("Same name objects with different fields return an error", func(t *testing.T) {
		runAndExpectError(t, newRemoveDuplicateFieldedSharedTypesVisitor(), `
			type Trainer {
				name: String
				age: Int
			}

			type Trainer {
				name: String
				age: Int
			}

			type Trainer {
				name: String
				age: Int
				badges: Int
			}
		`, NonIdenticalSharedTypeErrorMessage("Trainer"))
	})

	t.Run("Same name objects with a slight difference in nested field values return an error", func(t *testing.T) {
		runAndExpectError(t, newRemoveDuplicateFieldedSharedTypesVisitor(), `
			type Pokemon {
				type: [[[[Type!]]!]!]!
			}
	
			type Pokemon {
				type: [[[[Type!]]]!]!
			}
	
			type Pokemon {
				type: [[[[Type!]]!]!]!
			}
		`, NonIdenticalSharedTypeErrorMessage("Pokemon"))
	})

	t.Run("Same name objects with different non-nullable field values return an error", func(t *testing.T) {
		runAndExpectError(t, newRemoveDuplicateFieldedSharedTypesVisitor(), `
			type Trainer {
				name: String!
				age: Int!
			}
	
			type Trainer {
				name: String!
				age: Int!
			}
	
			type Trainer {
				name: String!
				age: String!
			}
		`, NonIdenticalSharedTypeErrorMessage("Trainer"))
	})
}
