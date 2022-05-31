package sdlmerge

import "testing"

func TestExtendObjectType(t *testing.T) {
	t.Run("extend object type by field", func(t *testing.T) {
		run(t, newExtendObjectTypeDefinition(newTestNormalizer(false)), `
			type Dog {
				name: String
			}

			extend type Dog {
				favoriteToy: String
			}
		`, `
			type Dog {
				name: String
				favoriteToy: String
			}

			extend type Dog {
				favoriteToy: String
			}
		`)
	})

	t.Run("extend object type by directive", func(t *testing.T) {
		run(t, newExtendObjectTypeDefinition(newTestNormalizer(false)), `
			type Cat {
				name: String
			}

			extend type Cat @deprecated(reason: "not as cool as dogs")
		`, `
			type Cat @deprecated(reason: "not as cool as dogs") {
				name: String
			}

			extend type Cat @deprecated(reason: "not as cool as dogs")
		`)
	})

	t.Run("extend object type by multiple field", func(t *testing.T) {
		run(t, newExtendObjectTypeDefinition(newTestNormalizer(false)), `
			type Dog {
				name: String
			}

			extend type Dog {
				favoriteToy: String
				breed: String
			}
		`, `
			type Dog {
				name: String
				favoriteToy: String
				breed: String
			}

			extend type Dog {
				favoriteToy: String
				breed: String
			}
		`)
	})

	t.Run("extend object type by multiple directives", func(t *testing.T) {
		run(t, newExtendObjectTypeDefinition(newTestNormalizer(false)), `
			type Cat {
				name: String
			}

			extend type Cat @deprecated(reason: "not as cool as dogs") @skip(if: false)
		`, `
			type Cat @deprecated(reason: "not as cool as dogs") @skip(if: false) {
				name: String
			}

			extend type Cat @deprecated(reason: "not as cool as dogs") @skip(if: false)
		`)
	})

	t.Run("extend object type by complex extends", func(t *testing.T) {
		run(t, newExtendObjectTypeDefinition(newTestNormalizer(false)), `
			type Cat {
				name: String
			}

			extend type Cat @deprecated(reason: "not as cool as dogs") @skip(if: false) {
				age: Int
				breed: String
			}
		`, `
			type Cat @deprecated(reason: "not as cool as dogs") @skip(if: false) {
				name: String
				age: Int
				breed: String
			}

			extend type Cat @deprecated(reason: "not as cool as dogs") @skip(if: false) {
				age: Int
				breed: String
			}
		`)
	})

	t.Run("Extending an object that is a shared type returns an error", func(t *testing.T) {
		runAndExpectError(t, newExtendObjectTypeDefinition(newTestNormalizer(false)), `
			type Mammal {
				name: String
			}

			type Mammal {
				name: String
			}

			extend type Mammal @deprecated(reason: "not as cool as dogs") @skip(if: false) {
				age: Int
				breed: String
			}
		`, sharedTypeExtensionErrorMessage("Mammal"))
	})

	t.Run("Unresolved object extension orphan returns an error", func(t *testing.T) {
		runAndExpectError(t, newExtendObjectTypeDefinition(newTestNormalizer(false)), `
			extend type Mammal {
				name: String!
			}
		`, unresolvedExtensionOrphansErrorMessage("Mammal"))
	})

	t.Run("Entity is extended successfully", func(t *testing.T) {
		run(t, newExtendObjectTypeDefinition(newTestNormalizer(true)), `
			 type Mammal @key(fields: "name") {
				name: String!
			}
			
			extend type Mammal @key(fields: "name") {
				name: String! @external
				age: Int!
			}
		`, `
			 type Mammal @key(fields: "name") @key(fields: "name") {

				name: String!
				name: String! @external
				age: Int!
			}
			
			extend type Mammal @key(fields: "name") {
				name: String! @external
				age: Int!
			}
		`)
	})

	t.Run("Primary key field reference without an external directive returns an error", func(t *testing.T) {
		runAndExpectError(t, newExtendObjectTypeDefinition(newTestNormalizer(true)), `
			 type Mammal @key(fields: "name") {
				name: String!
			}

			extend type Mammal @key(fields: "name") {
				name: String!
				age: Int!
			}
		`, externalDirectiveErrorMessage("Mammal"))
	})

	t.Run("Multiple arguments in a key directive returns an error", func(t *testing.T) {
		runAndExpectError(t, newExtendObjectTypeDefinition(newTestNormalizer(true)), `
			 type Mammal @key(fields: "name") {
				name: String!
			}
			
			extend type Mammal @key(fields: "name" arg: "name") {
				name: String! @external
				age: Int!
			}
		`, incorrectArgumentErrorMessage("Mammal"))
	})

	t.Run("Incorrect argument in a key directive returns an error", func(t *testing.T) {
		runAndExpectError(t, newExtendObjectTypeDefinition(newTestNormalizer(true)), `
			 type Mammal @key(fields: "name") {
				name: String!
			}
			
			extend type Mammal @key(feline: "name") {
				name: String! @external
				age: Int!
			}
		`, incorrectArgumentErrorMessage("Mammal"))
	})

	t.Run("Empty primary key in key directive returns an error", func(t *testing.T) {
		runAndExpectError(t, newExtendObjectTypeDefinition(newTestNormalizer(true)), `
			 type Mammal @key(fields: "name") {
				name: String!
			}
			
			extend type Mammal @key(fields: "") {
				name: String! @external
				age: Int!
			}
		`, emptyPrimaryKeyErrorMessage("Mammal"))
	})

	t.Run("Unresolved primary key in key directive returns an error", func(t *testing.T) {
		runAndExpectError(t, newExtendObjectTypeDefinition(newTestNormalizer(true)), `
			 type Mammal @key(fields: "name") {
				name: String!
			}
			
			extend type Mammal @key(fields: "coat") {
				name: String! @external
				age: Int!
			}
		`, unresolvedPrimaryKeyErrorMessage("coat", "Mammal"))
	})

	t.Run("No key directive on entity extension returns an error", func(t *testing.T) {
		runAndExpectError(t, newExtendObjectTypeDefinition(newTestNormalizer(true)), `
			 type Mammal @key(fields: "name") {
				name: String!
			}
			
			extend type Mammal {
				name: String! @external
				age: Int!
			}
		`, noKeyDirectiveErrorMessage("Mammal"))
	})

	t.Run("Non-key directive when extending an entity returns an error", func(t *testing.T) {
		runAndExpectError(t, newExtendObjectTypeDefinition(newTestNormalizer(true)), `
			 type Mammal @key(fields: "name") {
				name: String!
			}
			
			extend type Mammal @deprecated {
				name: String! @external
				age: Int!
			}
		`, noKeyDirectiveErrorMessage("Mammal"))
	})

	t.Run("Extending multiple entities returns an error", func(t *testing.T) {
		runAndExpectError(t, newExtendObjectTypeDefinition(newTestNormalizer(true)), `
			 type Mammal @key(fields: "name") {
				name: String!
			}

			 type Mammal @key(fields: "name") {
				name: String!
			}
			
			extend type Mammal @key(fields: "name") {
				name: String! @external
				age: Int!
			}
		`, duplicateEntityErrorMessage("Mammal"))
	})

	t.Run("A valid primary key whose reference does not exist as an external field returns an error", func(t *testing.T) {
		runAndExpectError(t, newExtendObjectTypeDefinition(newTestNormalizer(true)), `
			 type Mammal @key(fields: "name") {
				name: String!
			}
			
			extend type Mammal @key(fields: "name") {
				age: Int!
			}
		`, validPrimaryKeyMustBeFieldErrorMessage("Mammal"))
	})

	t.Run("A non-entity that is extended by an extension with a key directive returns an error", func(t *testing.T) {
		runAndExpectError(t, newExtendObjectTypeDefinition(newTestNormalizer(false)), `
			 type Mammal {
				name: String!
			}
			
			extend type Mammal @key(fields: "name") {
				name: String! @external
				age: Int!
			}
		`, nonEntityExtensionErrorMessage("Mammal"))
	})
}
