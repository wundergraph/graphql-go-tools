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

	t.Run("Identical same name inputs are merged into a single input", func(t *testing.T) {
		run(t, newRemoveDuplicateFieldedValueTypesVisitor(), `
			input Pokemon {
				type: Type!
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
				type: Type!
				isEvolved: Boolean!
			}
			`, `
			input Pokemon {
				type: Type!
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

	t.Run("Identical same name objects are merged into a single object", func(t *testing.T) {
		run(t, newRemoveDuplicateFieldedValueTypesVisitor(), `
			type Pokemon {
				type: Type!
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
				type: Type!
				isEvolved: Boolean!
			}
			`, `
			type Pokemon {
				type: Type!
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

	t.Run("Identical same name objects are merged into a single object", func(t *testing.T) {
		run(t, newRemoveDuplicateFieldedValueTypesVisitor(), `
			type Pokemon {
				type: Type!
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
				type: Type!
				isEvolved: Boolean!
			}
			`, `
			type Pokemon {
				type: Type!
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
}
