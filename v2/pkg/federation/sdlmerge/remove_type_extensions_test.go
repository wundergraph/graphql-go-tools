package sdlmerge

import "testing"

func TestRemoveTypeExtensions(t *testing.T) {
	t.Run("remove single type extension of fieldDefinition", func(t *testing.T) {
		runMany(t, `
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
			newExtendObjectTypeDefinition(newTestNormalizer(false)),
			newRemoveMergedTypeExtensions())
	})
	t.Run("remove single type extension of directive", func(t *testing.T) {
		runMany(t, `
					type Cat {
						name: String
					}
					extend type Cat @deprecated(reason: "not as cool as dogs")
					 `, `
					type Cat @deprecated(reason: "not as cool as dogs") {
						name: String
					}
					`,
			newExtendObjectTypeDefinition(newTestNormalizer(false)),
			newRemoveMergedTypeExtensions())
	})
	t.Run("remove multiple type extensions at once", func(t *testing.T) {
		runMany(t, `
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
			newExtendObjectTypeDefinition(newTestNormalizer(false)),
			newRemoveMergedTypeExtensions())
	})
	t.Run("remove interface type extensions", func(t *testing.T) {
		runMany(t, `
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
					`,
			newExtendInterfaceTypeDefinition(newTestNormalizer(false)),
			newRemoveMergedTypeExtensions())
	})
	t.Run("keep not merged type extension", func(t *testing.T) {
		runMany(t, `
				extend type User { 
					field: String! 
				}
		`, `
				extend type User { 
					field: String! 
				}
		`,
			newExtendInterfaceTypeDefinition(newTestNormalizer(false)),
			newRemoveMergedTypeExtensions(),
		)
	})
}
