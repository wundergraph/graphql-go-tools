package astnormalization

import "testing"

func TestExtendInterfaceType(t *testing.T) {
	t.Run("extend simple interface type by field", func(t *testing.T) {
		run(t, extendInterfaceTypeDefinition, testDefinition, `
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
		run(t, extendInterfaceTypeDefinition, testDefinition, `
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
		run(t, extendInterfaceTypeDefinition, testDefinition, `
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
		run(t, extendInterfaceTypeDefinition, testDefinition, `
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
		run(t, extendInterfaceTypeDefinition, testDefinition, `
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
		run(t, extendInterfaceTypeDefinition, "", `
					extend interface Entity { id: ID }
					extend interface Mammal @deprecated(reason: "some reason") @skip(if: false) { name: String }
					 `, `
					extend interface Entity { id: ID }
					extend interface Mammal @deprecated(reason: "some reason") @skip(if: false) { name: String }
					interface Entity { id: ID }
					interface Mammal @deprecated(reason: "some reason") @skip(if: false) { name: String }
					`)
	})

	t.Run("interface extensions implementing other interface implemented by object type", func(t *testing.T) {
		run(t, extendInterfaceTypeDefinition, "", `
			interface Entity {
			  name: String
			}
			
			interface Tall{  
			  height: String
			}
			
			extend interface Tall implements Entity{
			  name: String
			}
			
			type People implements Entity & Tall{
			  name: String
			  height: String
			  mass: String  
			  birth_year: String
			  gender: String
			  homeworld: String
			  homeplanet: Planet
			  url: String
			  skin_color: String
			  hair_color: String
			  eye_color: String
			}`, `
			interface Entity {
			  name: String
			}
			
			interface Tall implements Entity{  
			  height: String
			  name: String
			}
			
			extend interface Tall implements Entity{
			  name: String
			}
			
			type People implements Entity & Tall{
			  name: String
			  height: String
			  mass: String  
			  birth_year: String
			  gender: String
			  homeworld: String
			  homeplanet: Planet
			  url: String
			  skin_color: String
			  hair_color: String
			  eye_color: String
			}`)
	})
}
