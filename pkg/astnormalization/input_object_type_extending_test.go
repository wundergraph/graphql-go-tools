package astnormalization

import "testing"

func TestExtendInputObjectType(t *testing.T) {
	t.Run("extend simple input object type by directive", func(t *testing.T) {
		run(extendInputObjectTypeDefinition, testDefinition, `
					input DogSize {width: Float height: Float}
					extend input DogSize @deprecated(reason: "some reason")
					 `, `
					input DogSize @deprecated(reason: "some reason") {width: Float height: Float}
					extend input DogSize @deprecated(reason: "some reason")
					`)
	})
}
