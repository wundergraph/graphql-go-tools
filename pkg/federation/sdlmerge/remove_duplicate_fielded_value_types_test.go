package sdlmerge

import "testing"

func TestRemoveDuplicateFieldedValueTypes(t *testing.T) {
	t.Run("Same name empty inputs are merged into a single input", func(t *testing.T) {
		run(t, newRemoveDuplicateFieldedValueTypesVisitor(), `
			input Trainer {
			}
	
			input Trainer {
			}
			`, `
			input Trainer {
			}
			`,
		)
	})

	t.Run("Identical same name inputs are merged into a single input", func(t *testing.T) {
		run(t, newRemoveDuplicateFieldedValueTypesVisitor(), `
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
			`,
		)
	})

	t.Run("Identical same name inputs are merged into a single input regardless of field order", func(t *testing.T) {
		run(t, newRemoveDuplicateFieldedValueTypesVisitor(), `
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
			`,
		)
	})

	t.Run("Groups of identical same name inputs are respectively merged into single inputs", func(t *testing.T) {
		run(t, newRemoveDuplicateFieldedValueTypesVisitor(), `
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
			`,
		)
	})

	t.Run("Same name inputs with different nullability of fields return an error", func(t *testing.T) {
		runAndExpectError(t, newRemoveDuplicateFieldedValueTypesVisitor(), `
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
			`, FederatingValueTypeErrorMessage("Trainer"))
	})

	t.Run("Same name inputs with different fields return an error", func(t *testing.T) {
		runAndExpectError(t, newRemoveDuplicateFieldedValueTypesVisitor(), `
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
			`, FederatingValueTypeErrorMessage("Trainer"))
	})

	t.Run("Same name inputs with a slight difference in nested field values return an error", func(t *testing.T) {
		runAndExpectError(t, newRemoveDuplicateFieldedValueTypesVisitor(), `
			input Pokemon {
				type: [[[[Type!]]!]!]!
			}
	
			input Pokemon {
				type: [[[[Type!]]]!]!
			}
	
			input Pokemon {
				type: [[[[Type!]]!]!]!
			}
			`, FederatingValueTypeErrorMessage("Pokemon"))
	})

	t.Run("Same name inputs with different non-nullable field values return an error", func(t *testing.T) {
		runAndExpectError(t, newRemoveDuplicateFieldedValueTypesVisitor(), `
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
			`, FederatingValueTypeErrorMessage("Trainer"))
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
			`,
		)
	})

	t.Run("Identical same name interfaces are merged into a single input regardless of field order", func(t *testing.T) {
		run(t, newRemoveDuplicateFieldedValueTypesVisitor(), `
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
			`,
		)
	})

	t.Run("Groups of identical same name interfaces are respectively merged into single interfaces", func(t *testing.T) {
		run(t, newRemoveDuplicateFieldedValueTypesVisitor(), `
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
			`,
		)
	})

	t.Run("Same name interfaces with different nullability of fields return an error", func(t *testing.T) {
		runAndExpectError(t, newRemoveDuplicateFieldedValueTypesVisitor(), `
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
			`, FederatingValueTypeErrorMessage("Trainer"))
	})

	t.Run("Same name interfaces with different fields return an error", func(t *testing.T) {
		runAndExpectError(t, newRemoveDuplicateFieldedValueTypesVisitor(), `
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
			`, FederatingValueTypeErrorMessage("Trainer"))
	})

	t.Run("Same name interfaces with a slight difference in nested field values return an error", func(t *testing.T) {
		runAndExpectError(t, newRemoveDuplicateFieldedValueTypesVisitor(), `
			interface Pokemon {
				type: [[[[Type!]]!]!]!
			}
	
			interface Pokemon {
				type: [[[[Type!]]]!]!
			}
	
			interface Pokemon {
				type: [[[[Type!]]!]!]!
			}
			`, FederatingValueTypeErrorMessage("Pokemon"))
	})

	t.Run("Same name interfaces with different non-nullable field values return an error", func(t *testing.T) {
		runAndExpectError(t, newRemoveDuplicateFieldedValueTypesVisitor(), `
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
			`, FederatingValueTypeErrorMessage("Trainer"))
	})

	t.Run("Same name empty objects are merged into a single object", func(t *testing.T) {
		run(t, newRemoveDuplicateFieldedValueTypesVisitor(), `
			type Trainer {
			}

			type Trainer {
			}
			`, `
			type Trainer {
			}
			`,
		)
	})

	t.Run("Identical same name objects are merged into a single object", func(t *testing.T) {
		run(t, newRemoveDuplicateFieldedValueTypesVisitor(), `
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
			`,
		)
	})

	t.Run("Identical same name objects are merged into a single input regardless of field order", func(t *testing.T) {
		run(t, newRemoveDuplicateFieldedValueTypesVisitor(), `
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
			`,
		)
	})

	t.Run("Groups of identical same name objects are respectively merged into single objects", func(t *testing.T) {
		run(t, newRemoveDuplicateFieldedValueTypesVisitor(), `
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
			`,
		)
	})

	t.Run("Same name objects with different nullability of fields return an error", func(t *testing.T) {
		runAndExpectError(t, newRemoveDuplicateFieldedValueTypesVisitor(), `
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
			`, FederatingValueTypeErrorMessage("Trainer"))
	})

	t.Run("Same name objects with different fields return an error", func(t *testing.T) {
		runAndExpectError(t, newRemoveDuplicateFieldedValueTypesVisitor(), `
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
			`, FederatingValueTypeErrorMessage("Trainer"))
	})

	t.Run("Same name objects with a slight difference in nested field values return an error", func(t *testing.T) {
		runAndExpectError(t, newRemoveDuplicateFieldedValueTypesVisitor(), `
			type Pokemon {
				type: [[[[Type!]]!]!]!
			}
	
			type Pokemon {
				type: [[[[Type!]]]!]!
			}
	
			type Pokemon {
				type: [[[[Type!]]!]!]!
			}
			`, FederatingValueTypeErrorMessage("Pokemon"))
	})

	t.Run("Same name objects with different non-nullable field values return an error", func(t *testing.T) {
		runAndExpectError(t, newRemoveDuplicateFieldedValueTypesVisitor(), `
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
			`, FederatingValueTypeErrorMessage("Trainer"))
	})
}
