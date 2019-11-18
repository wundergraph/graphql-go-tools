package astnormalization

import "testing"

func TestExtendScalarType(t *testing.T) {
	t.Run("extend simple scalar type", func(t *testing.T) {
		run(extendScalarTypeDefinition, testDefinition, `
					scalar Coordinates
					extend scalar Coordinates @deprecated(reason: "some reason")
					 `, `
					scalar Coordinates @deprecated(reason: "some reason")
					extend scalar Coordinates @deprecated(reason: "some reason")
					`)
	})
	t.Run("extend scalar type by multiple directives", func(t *testing.T) {
		run(extendScalarTypeDefinition, testDefinition, `
					scalar Coordinates
					extend scalar Coordinates @deprecated(reason: "some reason") @skip(if: false)
					 `, `
					scalar Coordinates @deprecated(reason: "some reason") @skip(if: false)
					extend scalar Coordinates @deprecated(reason: "some reason") @skip(if: false)
					`)
	})
}
