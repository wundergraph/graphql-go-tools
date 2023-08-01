package astnormalization

import "testing"

func TestExtendEnumType(t *testing.T) {
	t.Run("extend enum type by directive", func(t *testing.T) {
		run(t, extendEnumTypeDefinition, testDefinition, `
					enum Countries {DE ES NL}
					extend enum Countries @deprecated(reason: "some reason")
					 `, `
					enum Countries @deprecated(reason: "some reason") {DE ES NL} 
					extend enum Countries @deprecated(reason: "some reason")
					`)
	})
	t.Run("extend enum type by enum values", func(t *testing.T) {
		run(t, extendEnumTypeDefinition, testDefinition, `
					enum Countries {DE ES NL}
					extend enum Countries {EN}
					 `, `
					enum Countries {DE ES NL EN}
					extend enum Countries {EN}
					`)
	})
	t.Run("extend enum type by multiple enum values and directives", func(t *testing.T) {
		run(t, extendEnumTypeDefinition, testDefinition, `
					enum Countries {DE ES NL}
					extend enum Countries @deprecated(reason: "some reason") @skip(if: false) {EN IT}
					 `, `
					enum Countries @deprecated(reason: "some reason") @skip(if: false) {DE ES NL EN IT}
					extend enum Countries @deprecated(reason: "some reason") @skip(if: false) {EN IT}
					`)
	})
	t.Run("extend non existent enum type", func(t *testing.T) {
		run(t, extendEnumTypeDefinition, "", `
					extend enum Planet { EARTH MARS }
					extend enum Countries @deprecated(reason: "some reason") @skip(if: false) {EN IT}
					 `, `
					extend enum Planet { EARTH MARS }
					extend enum Countries @deprecated(reason: "some reason") @skip(if: false) {EN IT}
					enum Planet { EARTH MARS }
					enum Countries @deprecated(reason: "some reason") @skip(if: false) {EN IT}
					`)
	})
}
