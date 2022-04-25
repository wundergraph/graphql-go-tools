package sdlmerge

import "testing"

func TestRemoveDuplicateFieldlessValueTypes(t *testing.T) {
	t.Run("Input and output are identical when no duplications", func(t *testing.T) {
		run(t, newRemoveDuplicateFieldlessValueTypesVisitor(), `
					enum Pokemon {
						BULBASAUR,
						CHARMANDER,
						SQUIRTLE,
					}
					 `, `
					enum Pokemon {
						BULBASAUR,
						CHARMANDER,
						SQUIRTLE,
					}
					`)
	})

	t.Run("Identical same name enums are merged", func(t *testing.T) {
		run(t, newRemoveDuplicateFieldlessValueTypesVisitor(), `
					enum Pokemon {
						BULBASAUR,
						CHARMANDER,
						SQUIRTLE,
					}

					enum Pokemon {
						BULBASAUR,
						CHARMANDER,
						SQUIRTLE,
					}
					 `, `
					enum Pokemon {
						BULBASAUR,
						CHARMANDER,
						SQUIRTLE,
					}
					`)
	})

	t.Run("Same name enums with different values are merged", func(t *testing.T) {
		run(t, newRemoveDuplicateFieldlessValueTypesVisitor(), `
					enum Pokemon {
						BULBASAUR,
						CHARMANDER,
						SQUIRTLE,
					}

					enum Pokemon {
						BULBASAUR,
						CHARMANDER,
						SQUIRTLE,
						MEW,
					}
					 `, `
					enum Pokemon {
						BULBASAUR,
						CHARMANDER,
						SQUIRTLE,
						MEW,
					}
					`)
	})

	t.Run("Empty enums are merged into populated enums where possible", func(t *testing.T) {
		run(t, newRemoveDuplicateFieldlessValueTypesVisitor(), `
					enum Pokemon {
					}

					enum Pokemon {
						CHARMANDER,
						SQUIRTLE,
					}
					 `, `
					enum Pokemon {
						CHARMANDER,
						SQUIRTLE,
					}
					`)
	})

	t.Run("Empty enums are merged into a single empty enum", func(t *testing.T) {
		run(t, newRemoveDuplicateFieldlessValueTypesVisitor(), `
					enum Pokemon {
					}

					enum Pokemon {
					}
					 `, `
					enum Pokemon {
					}
					`)
	})

	t.Run("Same named enums with no overlapping values are merged", func(t *testing.T) {
		run(t, newRemoveDuplicateFieldlessValueTypesVisitor(), `
					enum Pokemon {
						BULBASAUR,
						CHARMANDER,
					}

					enum Pokemon {
						SQUIRTLE,
						MEW,
					}
					 `, `
					enum Pokemon {
						BULBASAUR,
						CHARMANDER,
						SQUIRTLE,
						MEW,
					}
					`)
	})

	t.Run("Same named enums with varying overlapping values are merged", func(t *testing.T) {
		run(t, newRemoveDuplicateFieldlessValueTypesVisitor(), `
					enum Pokemon {
						BULBASAUR,
						CHARMANDER,
					}

					enum Pokemon {
						CHARMANDER,
						MEW,
					}

					enum Pokemon {
						BULBASAUR,
						MEW,
					}

					enum Pokemon {
						BULBASAUR,
						SQUIRTLE,
					}
					 `, `
					enum Pokemon {
						BULBASAUR,
						CHARMANDER,
						MEW,
						SQUIRTLE,
					}
					`)
	})

	t.Run("Different groups of same named enums are all merged correctly", func(t *testing.T) {
		run(t, newRemoveDuplicateFieldlessValueTypesVisitor(), `
					enum Cities {
						CERULEAN,
						SAFFRON,
					}

					enum Types {
						GRASS,
						FIRE,
						ROCK,
					}

					enum Badges {
					}

					enum Types {
						FIRE,
						WATER,
					}

					enum Badges {
						BOULDER,
						VOLCANO,
						EARTH,
					}

					enum Cities {
						VIRIDIAN,
						SAFFRON,
						CELADON,
					}
					
					enum Types {
						ROCK,
						GRASS,
						FIRE,
						WATER,
					}

					enum Badges {
						MARSH,
						SOUL,
						VOLCANO,
						THUNDER,
						RAINBOW,
						CASCADE,
					}

					enum Badges {
						VOLCANO,
						RAINBOW,
						BOULDER,
						SOUL,
					}

					enum Types {
						WATER,
						FIRE,
					}

					enum Badges {
					}

					enum Badges {
						EARTH,
						THUNDER,
					}
					
					enum Cities {
					}

					enum Cities {
						CERULEAN,
						CELADON,
					}
					 `, `
					enum Cities {
						CERULEAN,
						SAFFRON,
						VIRIDIAN,
						CELADON,
					}

					enum Types {
						GRASS,
						FIRE,
						ROCK,
						WATER,
					}

					enum Badges {
						BOULDER,
						VOLCANO,
						EARTH,
						MARSH,
						SOUL,
						THUNDER,
						RAINBOW,
						CASCADE,
					}
					`)
	})

	t.Run("Input and output are identical when no duplications", func(t *testing.T) {
		run(t, newRemoveDuplicateFieldlessValueTypesVisitor(), `
					scalar DateTime
					 `, `
					scalar DateTime
					`)
	})

	t.Run("Same named scalars are removed to leave only one", func(t *testing.T) {
		run(t, newRemoveDuplicateFieldlessValueTypesVisitor(), `
					scalar DateTime
	
					scalar DateTime

					scalar DateTime
					 `, `
					scalar DateTime
					`)
	})

	t.Run("Any more than one of a same named scalar are removed", func(t *testing.T) {
		run(t, newRemoveDuplicateFieldlessValueTypesVisitor(), `
					scalar DateTime

					scalar BigInt
	
					scalar BigInt
					
					scalar CustomScalar

					scalar DateTime
	
					scalar UniqueScalar
	
					scalar BigInt
					
					scalar CustomScalar
					
					scalar CustomScalar
	
					scalar DateTime

					scalar CustomScalar

					scalar DateTime
					 `, `
					scalar DateTime

					scalar BigInt

					scalar CustomScalar

					scalar UniqueScalar
					`)
	})

	t.Run("Input and output are identical when no duplications", func(t *testing.T) {
		run(t, newRemoveDuplicateFieldlessValueTypesVisitor(), `
					union Types = Grass | Fire | Water
					 `, `
					union Types = Grass | Fire | Water
					`)
	})

	t.Run("Identical same named unions are merged", func(t *testing.T) {
		run(t, newRemoveDuplicateFieldlessValueTypesVisitor(), `
					union Types = Grass | Fire | Water
	
					union Types = Grass | Fire | Water
					 `, `
					union Types = Grass | Fire | Water
					`)
	})

	t.Run("Same name unions with different values are merged", func(t *testing.T) {
		run(t, newRemoveDuplicateFieldlessValueTypesVisitor(), `
					union Types = Grass | Fire | Water
	
					union Types = Grass | Fire | Water | Rock
					 `, `
					union Types = Grass | Fire | Water | Rock
					`)
	})

	t.Run("Same named unions with no overlapping values are merged", func(t *testing.T) {
		run(t, newRemoveDuplicateFieldlessValueTypesVisitor(), `
					union Types = Grass | Fire
	
					union Types = Water | Rock
					 `, `
					union Types = Grass | Fire | Water | Rock
					`)
	})

	t.Run("Same named unions with varying overlapping values are merged", func(t *testing.T) {
		run(t, newRemoveDuplicateFieldlessValueTypesVisitor(), `
					union Types = Grass | Fire
	
					union Types = Fire | Water
	
					union Types = Rock | Grass
	
					union Types = Water | Fire
					 `, `
					union Types = Grass | Fire | Water | Rock
					`)
	})

	t.Run("Different groups of same named unions are all merged correctly", func(t *testing.T) {
		run(t, newRemoveDuplicateFieldlessValueTypesVisitor(), `
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
