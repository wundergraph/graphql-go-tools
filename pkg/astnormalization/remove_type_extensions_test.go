package astnormalization

import "testing"

func TestRemoveTypeExtensions(t *testing.T) {
	t.Run("remove single type extension of fieldDefinition", func(t *testing.T) {
		runMany(testDefinition, `
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
					`,
			extendObjectTypeDefinition,
			removeMergedTypeExtensions)
	})
	t.Run("remove single type extension of directive", func(t *testing.T) {
		runMany(testDefinition, `
					type Cat {
						name: String
					}
					extend type Cat @deprecated(reason: "not as cool as dogs")
					 `, `
					type Cat @deprecated(reason: "not as cool as dogs") {
						name: String
					}
					`,
			extendObjectTypeDefinition,
			removeMergedTypeExtensions)
	})
	t.Run("remove multiple type extensions at once", func(t *testing.T) {
		runMany(testDefinition, `
					type Cat {
						name: String
					}
					extend type Cat @deprecated(reason: "not as cool as dogs")
					extend type Cat {
						age: Int
					}
					 `, `
					type Cat @deprecated(reason: "not as cool as dogs") {
						name: String
						age: Int
					}
					`,
			extendObjectTypeDefinition,
			removeMergedTypeExtensions)
	})
	t.Run("remove multiple scalar type extensions", func(t *testing.T) {
		runMany(testDefinition, `
					scalar Coordinates
					extend scalar Coordinates @deprecated(reason: "some reason") @skip(if: false)
					 `, `
					scalar Coordinates @deprecated(reason: "some reason") @skip(if: false)
					`,
			extendScalarTypeDefinition,
			removeMergedTypeExtensions)
	})
	t.Run("remove multiple scalar type extensions", func(t *testing.T) {
		runMany(testDefinition, `
					enum Countries {DE ES NL}
					extend enum Countries @deprecated(reason: "some reason") @skip(if: false) {EN IT}
					 `, `
					enum Countries @deprecated(reason: "some reason") @skip(if: false) {DE ES NL EN IT}
					`,
			extendEnumTypeDefinition,
			removeMergedTypeExtensions)
	})
	t.Run("remove multiple scalar type extensions", func(t *testing.T) {
		runMany(testDefinition, `
					input DogSize {width: Float height: Float}
					extend input DogSize @deprecated(reason: "some reason") @skip(if: false) {breadth: Float weight: Float}
					 `, `
					input DogSize @deprecated(reason: "some reason") @skip(if: false) {width: Float height: Float breadth: Float weight: Float}
					`,
			extendInputObjectTypeDefinition,
			removeMergedTypeExtensions)
	})
}
