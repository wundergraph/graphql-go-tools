package astnormalization

import "testing"

func TestExtendUnionType(t *testing.T) {
	t.Run("extend simple union type by directive", func(t *testing.T) {
		run(extendUnionTypeDefinition, testDefinition, `
					union Mammal
					extend union Mammal @deprecated(reason: "some reason")
					 `, `
					union Mammal @deprecated(reason: "some reason")
					extend union Mammal @deprecated(reason: "some reason")
					`)
	})
	t.Run("extend simple union type by UnionMemberType", func(t *testing.T) {
		run(extendUnionTypeDefinition, testDefinition, `
					union Mammal
					extend union Mammal = Cat
					 `, `
					union Mammal = Cat
					extend union Mammal = Cat
					`)
	})
	t.Run("extend union by multiple directives and union members", func(t *testing.T) {
		run(extendUnionTypeDefinition, testDefinition, `
					union Mammal
					extend union Mammal @deprecated(reason: "some reason") @skip(if: false) = Cat | Dog
					 `, `
					union Mammal @deprecated(reason: "some reason") @skip(if: false) = Cat | Dog
					extend union Mammal @deprecated(reason: "some reason") @skip(if: false) = Cat | Dog
					`)
	})
}
