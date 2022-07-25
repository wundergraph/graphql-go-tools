package sdlmerge

import "testing"

func TestExtendInterfaceType(t *testing.T) {
	t.Run("extend simple interface type by field", func(t *testing.T) {
		run(t, newExtendInterfaceTypeDefinition(newTestNormalizer(false)), `
			interface Mammal {
				name: String
			}

			extend interface Mammal {
				furType: String
			}
		`, `
			interface Mammal {
				name: String
				furType: String
			}

			extend interface Mammal {
				furType: String
			}
		`)
	})

	t.Run("extend simple interface type by directive", func(t *testing.T) {
		run(t, newExtendInterfaceTypeDefinition(newTestNormalizer(false)), `
			interface Mammal {
				name: String
			}

			extend interface Mammal @deprecated(reason: "some reason")
		`, `
			interface Mammal @deprecated(reason: "some reason") {
				name: String
			}

			extend interface Mammal @deprecated(reason: "some reason")
		`)
	})

	t.Run("extend interface type by complex extends", func(t *testing.T) {
		run(t, newExtendInterfaceTypeDefinition(newTestNormalizer(false)), `
			interface Mammal {
				name: String
			}

			extend interface Mammal @deprecated(reason: "some reason") @skip(if: false) {
				furType: String
				age: Int
			}
		`, `
			interface Mammal @deprecated(reason: "some reason") @skip(if: false) {
				name: String
				furType: String
				age: Int
			}

			extend interface Mammal @deprecated(reason: "some reason") @skip(if: false) {
				furType: String
				age: Int
			}
		`)
	})

	t.Run("Extending an interface that is a shared type returns an error", func(t *testing.T) {
		runAndExpectError(t, newExtendInterfaceTypeDefinition(newTestNormalizer(false)), `
			interface Mammal {
				name: String
			}

			interface Mammal {
				name: String
			}

			extend interface Mammal @deprecated(reason: "some reason") @skip(if: false) {
				furType: String
				age: Int
			}
		`, sharedTypeExtensionErrorMessage("Mammal"))
	})

	t.Run("Unresolved interface extension orphan returns an error", func(t *testing.T) {
		runAndExpectError(t, newExtendInterfaceTypeDefinition(newTestNormalizer(false)), `
			extend interface Mammal {
				name: String!
			}
		`, unresolvedExtensionOrphansErrorMessage("Mammal"))
	})

	t.Run("Entity is extended successfully", func(t *testing.T) {
		run(t, newExtendInterfaceTypeDefinition(newTestNormalizer(true)), `
			 interface Mammal @key(fields: "name") {
				name: String!
			}
			
			extend interface Mammal @key(fields: "name") {
				name: String! @external
				age: Int!
			}
		`, `
			 interface Mammal @key(fields: "name") @key(fields: "name") {

				name: String!
				name: String! @external
				age: Int!
			}
			
			extend interface Mammal @key(fields: "name") {
				name: String! @external
				age: Int!
			}
		`)
	})

	t.Run("No key directive on entity extension returns an error", func(t *testing.T) {
		runAndExpectError(t, newExtendInterfaceTypeDefinition(newTestNormalizer(true)), `
			 interface Mammal @key(fields: "name") {
				name: String!
			}
			
			extend interface Mammal {
				name: String! @external
				age: Int!
			}
		`, noKeyDirectiveErrorMessage("Mammal"))
	})

	t.Run("Non-key directive when extending an entity returns an error", func(t *testing.T) {
		runAndExpectError(t, newExtendInterfaceTypeDefinition(newTestNormalizer(true)), `
			 interface Mammal @key(fields: "name") {
				name: String!
			}
			
			extend interface Mammal @deprecated {
				name: String! @external
				age: Int!
			}
		`, noKeyDirectiveErrorMessage("Mammal"))
	})

	t.Run("Extending multiple entities returns an error", func(t *testing.T) {
		runAndExpectError(t, newExtendInterfaceTypeDefinition(newTestNormalizer(true)), `
			 interface Mammal @key(fields: "name") {
				name: String!
			}

			 interface Mammal @key(fields: "name") {
				name: String!
			}
			
			extend interface Mammal @key(fields: "name") {
				name: String! @external
				age: Int!
			}
		`, duplicateEntityErrorMessage("Mammal"))
	})

	t.Run("A non-entity that is extended by an extension with a key directive returns an error", func(t *testing.T) {
		runAndExpectError(t, newExtendInterfaceTypeDefinition(newTestNormalizer(false)), `
			 interface Mammal {
				name: String!
			}
			
			extend interface Mammal @key(fields: "name") {
				name: String! @external
				age: Int!
			}
		`, nonEntityExtensionErrorMessage("Mammal"))
	})
}
