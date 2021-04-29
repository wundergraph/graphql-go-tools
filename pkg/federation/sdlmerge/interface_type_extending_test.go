package sdlmerge

import "testing"

func TestExtendInterfaceType(t *testing.T) {
	t.Run("extend simple interface type by field", func(t *testing.T) {
		run(t, newExtendInterfaceTypeDefinition(), `
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
		run(t, newExtendInterfaceTypeDefinition(), `
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
		run(t, newExtendInterfaceTypeDefinition(), `
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
}
