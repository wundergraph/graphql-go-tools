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
}
