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
}
