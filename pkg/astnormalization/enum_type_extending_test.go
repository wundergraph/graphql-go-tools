package astnormalization

import "testing"

func TestExtendEnumType(t *testing.T) {
	t.Run("extend simple union type by directive", func(t *testing.T) {
		run(extendEnumTypeDefinition, testDefinition, `
					enum Countries {DE ES NL}
					extend enum Countries @deprecated(reason: "some reason")
					 `, `
					enum Countries @deprecated(reason: "some reason") {DE ES NL} 
					extend enum Countries @deprecated(reason: "some reason")
					`)
	})
}
