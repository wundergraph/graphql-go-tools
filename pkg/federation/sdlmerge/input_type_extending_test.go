package sdlmerge

import "testing"

func TestExtendInputObjectType(t *testing.T) {
	t.Run("extend simple input type by field", func(t *testing.T) {
		run(t, newExtendInputObjectTypeDefinition(), `
			input Mammal {
				name: String
			}
			extend input Mammal {
				furType: String
			}
		`, `
			input Mammal {
				name: String
				furType: String
			}
			extend input Mammal {
				furType: String
			}
		`)
	})

	t.Run("extend simple input type by directive", func(t *testing.T) {
		run(t, newExtendInputObjectTypeDefinition(), `
			input Mammal {
				name: String
			}
			extend input Mammal @deprecated(reason: "some reason")
		`, `
			input Mammal @deprecated(reason: "some reason") {
				name: String
			}
			extend input Mammal @deprecated(reason: "some reason")
		`)
	})

	t.Run("extend input type by complex extends", func(t *testing.T) {
		run(t, newExtendInputObjectTypeDefinition(), `
			input Mammal {
				name: String
			}
			extend input Mammal @deprecated(reason: "some reason") @skip(if: false) {
				furType: String
				age: Int
			}
		`, `
			input Mammal @deprecated(reason: "some reason") @skip(if: false) {
				name: String
				furType: String
				age: Int
			}
			extend input Mammal @deprecated(reason: "some reason") @skip(if: false) {
				furType: String
				age: Int
			}
		`)
	})

	t.Run("Extending an input that is a shared type returns an error", func(t *testing.T) {
		runAndExpectError(t, newExtendInputObjectTypeDefinition(), `
			input Mammal {
				name: String
			}
			input Mammal {
				name: String
			}
			extend input Mammal @deprecated(reason: "some reason") @skip(if: false) {
				furType: String
				age: Int
			}
		`, sharedTypeExtensionErrorMessage("Mammal"))
	})

	t.Run("Unresolved input extension orphan returns an error", func(t *testing.T) {
		runAndExpectError(t, newExtendInputObjectTypeDefinition(), `
			extend input Badges {
				name: String!
			}
		`, unresolvedExtensionOrphansErrorMessage("Badges"))
	})
}
