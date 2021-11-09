package astnormalization

import "testing"

func TestExtendInterfaceType(t *testing.T) {
	t.Run("extend simple interface type by field", func(t *testing.T) {
		run(extendInterfaceTypeDefinition, testDefinition, `
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
	t.Run("extend interface by implements interface", func(t *testing.T) {
		run(extendInterfaceTypeDefinition, testDefinition, `
					interface A {
						name: String
					}
					extend interface A implements B {
						age: Int
					}
					interface B {
						age: Int
					}
					 `, `
					interface A implements B {
						name: String
						age: Int
					}
					extend interface A implements B {
						age: Int
					}
					interface B {
						age: Int
					}
					`)
	})
	t.Run("extend interface by implements interface and field", func(t *testing.T) {
		run(extendInterfaceTypeDefinition, testDefinition, `
					interface A {
						name: String
					}
					extend interface A implements B {
						field: String
						age: Int
					}
					interface B {
						age: Int
					}
					 `, `
					interface A implements B {
						name: String
						field: String
						age: Int
					}
					extend interface A implements B {
						field: String
						age: Int
					}
					interface B {
						age: Int
					}
					`)
	})
	t.Run("extend simple interface type by directive", func(t *testing.T) {
		run(extendInterfaceTypeDefinition, testDefinition, `
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
		run(extendInterfaceTypeDefinition, testDefinition, `
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

	t.Run("extend non existent interface", func(t *testing.T) {
		run(extendInterfaceTypeDefinition, "", `
					extend interface Entity { id: ID }
					extend interface Mammal @deprecated(reason: "some reason") @skip(if: false) { name: String }
					 `, `
					extend interface Entity { id: ID }
					extend interface Mammal @deprecated(reason: "some reason") @skip(if: false) { name: String }
					interface Entity { id: ID }
					interface Mammal @deprecated(reason: "some reason") @skip(if: false) { name: String }
					`)
	})
}
