package astnormalization

import "testing"

func TestExtendUnionType(t *testing.T) {
	t.Run("extend union type by directive", func(t *testing.T) {
		run(t, extendUnionTypeDefinition, testDefinition, `
					union Mammal
					extend union Mammal @deprecated(reason: "some reason")
					 `, `
					union Mammal @deprecated(reason: "some reason")
					extend union Mammal @deprecated(reason: "some reason")
					`)
	})
	t.Run("extend union type by UnionMemberType", func(t *testing.T) {
		run(t, extendUnionTypeDefinition, testDefinition, `
					union Mammal
					extend union Mammal = Cat
					 `, `
					union Mammal = Cat
					extend union Mammal = Cat
					`)
	})
	t.Run("extend union type by multiple UnionMemberTypes", func(t *testing.T) {
		run(t, extendUnionTypeDefinition, testDefinition, `
					union Mammal
					extend union Mammal = Cat | Dog
					 `, `
					union Mammal = Cat | Dog
					extend union Mammal = Cat | Dog
					`)
	})
	t.Run("extend union by multiple directives and union members", func(t *testing.T) {
		run(t, extendUnionTypeDefinition, testDefinition, `
					union Mammal
					extend union Mammal @deprecated(reason: "some reason") @skip(if: false) = Cat | Dog
					 `, `
					union Mammal @deprecated(reason: "some reason") @skip(if: false) = Cat | Dog
					extend union Mammal @deprecated(reason: "some reason") @skip(if: false) = Cat | Dog
					`)
	})
	t.Run("extend union type which already has union member", func(t *testing.T) {
		run(t, extendUnionTypeDefinition, testDefinition, `
					union Mammal = Cat
					extend union Mammal @deprecated(reason: "some reason") = Dog
					 `, `
					union Mammal @deprecated(reason: "some reason") = Cat | Dog
					extend union Mammal @deprecated(reason: "some reason") = Dog
					`)
	})
	t.Run("extend non-existent union", func(t *testing.T) {
		run(t, extendUnionTypeDefinition, testDefinition, `
					extend union Response = SuccessResponse | ErrorResponse
					extend union Mammal @deprecated(reason: "some reason") = Dog
					 `, `
					extend union Response = SuccessResponse | ErrorResponse
					extend union Mammal @deprecated(reason: "some reason") = Dog
					union Response = SuccessResponse | ErrorResponse
					union Mammal @deprecated(reason: "some reason") = Dog
					`)
	})
}
