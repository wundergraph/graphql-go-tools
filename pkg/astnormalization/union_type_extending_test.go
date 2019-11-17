package astnormalization

import "testing"

func TestExtendUnionType(t *testing.T) {
	t.Run("extend simple union type by UnionMemberType", func(t *testing.T) {
		run(extendUnionTypeDefinition, testDefinition, `
					union Mammal
					extend union Mammal @deprecated(reason: "some reason")
					 `, `
					union Mammal @deprecated(reason: "some reason")
					extend union Mammal @deprecated(reason: "some reason")
					`)
	})
}
