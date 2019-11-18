package astnormalization

import "testing"

func TestExtendInputObjectType(t *testing.T) {
	t.Run("extend input object type by directive", func(t *testing.T) {
		run(extendInputObjectTypeDefinition, testDefinition, `
					input DogSize {width: Float height: Float}
					extend input DogSize @deprecated(reason: "some reason")
					 `, `
					input DogSize @deprecated(reason: "some reason") {width: Float height: Float}
					extend input DogSize @deprecated(reason: "some reason")
					`)
	})
	t.Run("extend input object type by input fields definition", func(t *testing.T) {
		run(extendInputObjectTypeDefinition, testDefinition, `
					input DogSize {width: Float height: Float}
					extend input DogSize {breadth: Float}
					 `, `
					input DogSize {width: Float height: Float, breadth: Float}
					extend input DogSize {breadth: Float}
					`)
	})
	t.Run("extend input object type by multiple input fields definition and directives", func(t *testing.T) {
		run(extendInputObjectTypeDefinition, testDefinition, `
					input DogSize {width: Float height: Float}
					extend input DogSize @deprecated(reason: "some reason") @skip(if: false) {breadth: Float weight: Float}
					 `, `
					input DogSize @deprecated(reason: "some reason") @skip(if: false) {width: Float height: Float breadth: Float weight: Float}
					extend input DogSize @deprecated(reason: "some reason") @skip(if: false) {breadth: Float weight: Float}
					`)
	})
}
