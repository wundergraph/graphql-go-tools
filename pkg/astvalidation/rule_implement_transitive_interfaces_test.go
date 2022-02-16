package astvalidation

import (
	"testing"
)

func TestImplementTransitiveInterfaces(t *testing.T) {
	t.Run("Definition", func(t *testing.T) {
		t.Run("interface and type implementing the same interface without transition", func(t *testing.T) {
			runDefinitionValidation(t, `
					interface IDType {
					  id: ID!
					}
					
					interface SoftDelete implements IDType {
					  id: ID!
					  deleted: Boolean!
					}
					
					type Record implements IDType {
					  id: ID!
					  data: String!
					}
				`, Valid, ImplementTransitiveInterfaces(),
			)
		})

		t.Run("interface implementing interface and type implementing transitive interface", func(t *testing.T) {
			runDefinitionValidation(t, `
					interface IDType {
					  id: ID!
					}
					
					interface SoftDelete implements IDType {
					  id: ID!
					  deleted: Boolean!
					}
					
					type Record implements SoftDelete & IDType {
					  id: ID!
					  deleted: Boolean!
					  data: String!
					}
				`, Valid, ImplementTransitiveInterfaces(),
			)
		})

		t.Run("interface extension implements interface and type implementing transitive interface", func(t *testing.T) {
			runDefinitionValidation(t, `
					interface IDType {
					  id: ID!
					}
					
					interface SoftDelete {
					  deleted: Boolean!
					}
					
					extend interface SoftDelete implements IDType {
					  id: ID!
					}
					
					type Record implements SoftDelete & IDType {
					  id: ID!
					  deleted: Boolean!
					  data: String!
					}
				`, Valid, ImplementTransitiveInterfaces(),
			)
		})

		t.Run("interface extension implements interface and type extension implementing transitive interface", func(t *testing.T) {
			runDefinitionValidation(t, `
					interface IDType {
					  id: ID!
					}
					
					interface SoftDelete {
					  deleted: Boolean!
					}
					
					extend interface SoftDelete implements IDType {
					  id: ID!
					}
					
					type Record {
					  data: String!
					}
					
					extend type Record implements SoftDelete & IDType {
					  id: ID!
					  deleted: Boolean!
					}
				`, Valid, ImplementTransitiveInterfaces(),
			)
		})

		t.Run("interface extension implements interface and type extension implementing transitive interface as more complex example", func(t *testing.T) {
			runDefinitionValidation(t, `
					interface BaseInterface {
						fieldOne: Bool!
					}

					interface SecondInterface implements BaseInterface {
						fieldOne: Bool!
						fieldTwo: Bool!
					}

					interface ThirdInterface {
						fieldThree: Bool!
					}

					extend interface ThirdInterface implements SecondInterface & BaseInterface {
						fieldOne: Bool!
						fieldTwo: Bool!
					}

					interface FourthInterface implements ThirdInterface & SecondInterface & BaseInterface {
						fieldOne: Bool!
						fieldTwo: Bool!
						fieldThree: Bool!
						fieldFour: Bool!
					}

					type ImplementingType {
						fieldObjectType: Bool!
					}

					extend type ImplementingType implements FourthInterface & ThirdInterface & SecondInterface & BaseInterface {
						fieldOne: Bool!
						fieldTwo: Bool!
						fieldThree: Bool!
						fieldFour: Bool!
					}
				`, Valid, ImplementTransitiveInterfaces(),
			)
		})

		t.Run("interface implementing interface and type not implementing transitive interface", func(t *testing.T) {
			runDefinitionValidation(t, `
					interface IDType {
					  id: ID!
					}
					
					interface SoftDelete implements IDType {
					  id: ID!
					  deleted: Boolean!
					}
					
					type Record implements SoftDelete {
					  id: ID!
					  deleted: Boolean!
					  data: String!
					}
				`, Invalid, ImplementTransitiveInterfaces(),
			)
		})

		t.Run("interface implementing interface and type extension not implementing transitive interface", func(t *testing.T) {
			runDefinitionValidation(t, `
					interface IDType {
					  id: ID!
					}
					
					interface SoftDelete implements IDType {
					  id: ID!
					  deleted: Boolean!
					}
					
					type Record {
					  data: String!
					}
					
					extend type Record implements SoftDelete {
					  id: ID!
					  deleted: Boolean!
					}
				`, Invalid, ImplementTransitiveInterfaces(),
			)
		})

		t.Run("interface implementing interface and type extension not implementing transitive interface as more complex example", func(t *testing.T) {
			runDefinitionValidation(t, `
					interface BaseInterface {
						fieldOne: Bool!
					}

					interface SecondInterface implements BaseInterface {
						fieldOne: Bool!
						fieldTwo: Bool!
					}

					interface ThirdInterface {
						fieldThree: Bool!
					}

					extend interface ThirdInterface implements SecondInterface {
						fieldOne: Bool!
						fieldTwo: Bool!
					}

					interface FourthInterface implements ThirdInterface & SecondInterface & BaseInterface {
						fieldOne: Bool!
						fieldTwo: Bool!
						fieldThree: Bool!
						fieldFour: Bool!
					}

					type ImplementingType {
						fieldObjectType: Bool!
					}

					extend type ImplementingType implements FourthInterface & ThirdInterface & SecondInterface & BaseInterface {
						fieldOne: Bool!
						fieldTwo: Bool!
						fieldThree: Bool!
						fieldFour: Bool!
					}
				`, Invalid, ImplementTransitiveInterfaces(),
			)
		})

		t.Run("Interface extension implementing interface which also already implements same interface", func(t *testing.T) {
			runDefinitionValidation(t, `
					interface IDType {
					  id: ID!
					}
					
					interface SoftDelete implements IDType {
					  id: ID!
					  deleted: Boolean!
					}
					
					extend interface SoftDelete implements IDType {
					  canBeRecovered: Boolean!
					}
				`, Valid, ImplementTransitiveInterfaces(),
			)
		})

		t.Run("Interface extension implementing interface without body", func(t *testing.T) {
			runDefinitionValidation(t, `
					interface IDType {
					  id: ID!
					}
					
					interface SoftDelete {
					  id: ID!
					  deleted: Boolean!
					}
					
					extend interface SoftDelete implements IDType
				`, Invalid, ImplementTransitiveInterfaces(),
			)
		})

	})
}
