package astnormalization

import "testing"

func TestExtendEnumType(t *testing.T) {
	t.Run("extend simple enum type by directive", func(t *testing.T) {
		run(extendEnumTypeDefinition, testDefinition, `
					enum Countries {DE ES NL}
					extend enum Countries @deprecated(reason: "some reason")
					 `, `
					enum Countries @deprecated(reason: "some reason") {DE ES NL} 
					extend enum Countries @deprecated(reason: "some reason")
					`)
	})
	t.Run("extend simple enum type by enum values", func(t *testing.T) {
		run(extendEnumTypeDefinition, testDefinition, `
					enum Countries {DE ES NL}
					extend enum Countries {EN}
					 `, `
					enum Countries {DE ES NL EN}
					extend enum Countries {EN}
					`)
	})
	t.Run("extend enum type by numtiple enum values and directives", func(t *testing.T) {
		run(extendEnumTypeDefinition, testDefinition, `
					enum Countries {DE ES NL}
					extend enum Countries @deprecated(reason: "some reason") @skip(if: false) {EN IT}
					 `, `
					enum Countries @deprecated(reason: "some reason") @skip(if: false) {DE ES NL EN IT}
					extend enum Countries @deprecated(reason: "some reason") @skip(if: false) {EN IT}
					`)
	})
}
