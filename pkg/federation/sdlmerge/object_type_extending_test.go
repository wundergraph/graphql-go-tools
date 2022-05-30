package sdlmerge

import (
	"testing"
)

func TestExtendObjectType(t *testing.T) {
	t.Run("extend object type by field", func(t *testing.T) {
		run(t, newExtendObjectTypeDefinition(N), `
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
		run(t, newExtendObjectTypeDefinition(N), `
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
		run(t, newExtendObjectTypeDefinition(N), `
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
		run(t, newExtendObjectTypeDefinition(N), `
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
		run(t, newExtendObjectTypeDefinition(N), `
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
		runAndExpectError(t, newExtendObjectTypeDefinition(N), `
			type Cat {
				name: String
			}

			type Cat {
				name: String
			}

			extend type Cat @deprecated(reason: "not as cool as dogs") @skip(if: false) {
				age: Int
				breed: String
			}
		`, sharedTypeExtensionErrorMessage("Cat"))
	})

	t.Run("Unresolved object extension orphan returns an error", func(t *testing.T) {
		runAndExpectError(t, newExtendObjectTypeDefinition(N), `
			extend type Cat {
				name: String!
			}
		`, unresolvedExtensionOrphansErrorMessage("Cat"))
	})

	t.Run("Entity is extended successfully", func(t *testing.T) {
		run(t, newExtendObjectTypeDefinition(&normalizer{nil, map[string]map[string]bool{"Cat": {"name": true}}}), `
			 type Cat @key(fields: "name") {
				name: String!
			}
			
			extend type Cat @key(fields: "name") {
				name: String! @external
				age: Int!
			}
		`, `
			 type Cat @key(fields: "name") @key(fields: "name") {

				name: String!
				name: String! @external
				age: Int!
			}
			
			extend type Cat @key(fields: "name") {
				name: String! @external
				age: Int!
			}
		`)
	})

	t.Run("Primary key field reference without an external directive returns an error", func(t *testing.T) {
		runAndExpectError(t, newExtendObjectTypeDefinition(&normalizer{nil, map[string]map[string]bool{"Cat": {"name": true}}}), `
			 type Cat @key(fields: "name") {
				name: String!
			}

			extend type Cat @key(fields: "name") {
				name: String!
				age: Int!
			}
		`, externalDirectiveErrorMessage("Cat"))
	})

	t.Run("Multiple arguments in a key directive returns an error", func(t *testing.T) {
		runAndExpectError(t, newExtendObjectTypeDefinition(&normalizer{nil, map[string]map[string]bool{"Cat": {"name": true}}}), `
			 type Cat @key(fields: "name") {
				name: String!
			}
			
			extend type Cat @key(fields: "name" arg: "name") {
				name: String! @external
				age: Int!
			}
		`, incorrectArgumentErrorMessage("Cat"))
	})

	t.Run("Incorrect argument in a key directive returns an error", func(t *testing.T) {
		runAndExpectError(t, newExtendObjectTypeDefinition(&normalizer{nil, map[string]map[string]bool{"Cat": {"name": true}}}), `
			 type Cat @key(fields: "name") {
				name: String!
			}
			
			extend type Cat @key(feline: "name") {
				name: String! @external
				age: Int!
			}
		`, incorrectArgumentErrorMessage("Cat"))
	})

	t.Run("Empty primary key in key directive returns an error", func(t *testing.T) {
		runAndExpectError(t, newExtendObjectTypeDefinition(&normalizer{nil, map[string]map[string]bool{"Cat": {"name": true}}}), `
			 type Cat @key(fields: "name") {
				name: String!
			}
			
			extend type Cat @key(fields: "") {
				name: String! @external
				age: Int!
			}
		`, emptyPrimaryKeyErrorMessage("Cat"))
	})

	t.Run("Unresolved primary key in key directive returns an error", func(t *testing.T) {
		runAndExpectError(t, newExtendObjectTypeDefinition(&normalizer{nil, map[string]map[string]bool{"Cat": {"name": true}}}), `
			 type Cat @key(fields: "name") {
				name: String!
			}
			
			extend type Cat @key(fields: "coat") {
				name: String! @external
				age: Int!
			}
		`, unresolvedPrimaryKeyErrorMessage("coat", "Cat"))
	})

	t.Run("No key directive when extending an entity returns an error", func(t *testing.T) {
		runAndExpectError(t, newExtendObjectTypeDefinition(&normalizer{nil, map[string]map[string]bool{"Cat": {"name": true}}}), `
			 type Cat @key(fields: "name") {
				name: String!
			}
			
			extend type Cat {
				name: String! @external
				age: Int!
			}
		`, noKeyDirectiveErrorMessage("Cat"))
	})

	t.Run("Extending multiple entities returns an error", func(t *testing.T) {
		runAndExpectError(t, newExtendObjectTypeDefinition(&normalizer{nil, map[string]map[string]bool{"Cat": {"name": true}}}), `
			 type Cat @key(fields: "name") {
				name: String!
			}

			 type Cat @key(fields: "name") {
				name: String!
			}
			
			extend type Cat @key(fields: "name") {
				name: String! @external
				age: Int!
			}
		`, DuplicateEntityErrorMessage("Cat"))
	})
}
