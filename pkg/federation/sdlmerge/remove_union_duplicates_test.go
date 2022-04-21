package sdlmerge

import "testing"

func TestRemoveUnionDuplicates(t *testing.T) {
	t.Run("Input and output are identical when no duplications", func(t *testing.T) {
		run(t, newRemoveDuplicateUnionTypeDefinitionVisitor(), `
					union Types = Grass | Fire | Water
					 `, `
					union Types = Grass | Fire | Water
					`)
	})

	t.Run("Identical same named unions are merged", func(t *testing.T) {
		run(t, newRemoveDuplicateUnionTypeDefinitionVisitor(), `
					union Types = Grass | Fire | Water
	
					union Types = Grass | Fire | Water
					 `, `
					union Types = Grass | Fire | Water
					`)
	})

	t.Run("Same name unions with different values are merged", func(t *testing.T) {
		run(t, newRemoveDuplicateUnionTypeDefinitionVisitor(), `
					union Types = Grass | Fire | Water
	
					union Types = Grass | Fire | Water | Rock
					 `, `
					union Types = Grass | Fire | Water | Rock
					`)
	})

	t.Run("Same named unions with no overlapping values are merged", func(t *testing.T) {
		run(t, newRemoveDuplicateUnionTypeDefinitionVisitor(), `
					union Types = Grass | Fire
	
					union Types = Water | Rock
					 `, `
					union Types = Grass | Fire | Water | Rock
					`)
	})

	t.Run("Same named unions with varying overlapping values are merged", func(t *testing.T) {
		run(t, newRemoveDuplicateUnionTypeDefinitionVisitor(), `
					union Types = Grass | Fire
	
					union Types = Fire | Water
	
					union Types = Rock | Grass
	
					union Types = Water | Fire
					 `, `
					union Types = Grass | Fire | Water | Rock
					`)
	})

	t.Run("Different groups of same named unions are all merged correctly", func(t *testing.T) {
		run(t, newRemoveDuplicateUnionTypeDefinitionVisitor(), `
					union Cities = Cerulean | Saffron

					union Types = Grass | Fire | Rock
	
					union Types = Fire | Water
					
					union Badges = Boulder | Volcano | Earth

					union Cities = Viridian | Saffron | Celadon
	
					union Types = Rock | Grass | Fire | Water
					
					union Badges = Marsh | Soul | Volcano | Thunder | Rainbow | Cascade
					
					union Badges = Volcano | Rainbow | Boulder | Soul
	
					union Types = Water | Fire

					union Badges = Earth | Thunder

					union Cities = Cerulean | Celadon
					 `, `
					union Cities = Cerulean | Saffron | Viridian | Celadon

					union Types = Grass | Fire | Rock | Water

					union Badges = Boulder | Volcano | Earth | Marsh | Soul | Thunder | Rainbow | Cascade
					`)
	})
}
