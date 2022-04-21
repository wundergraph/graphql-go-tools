package sdlmerge

import "testing"

func TestRemoveEnumDuplicates(t *testing.T) {
	t.Run("Input and output are identical when no duplications", func(t *testing.T) {
		run(t, newRemoveDuplicateEnumTypeDefinitionVisitor(), `
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
		run(t, newRemoveDuplicateEnumTypeDefinitionVisitor(), `
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
		run(t, newRemoveDuplicateEnumTypeDefinitionVisitor(), `
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
		run(t, newRemoveDuplicateEnumTypeDefinitionVisitor(), `
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
		run(t, newRemoveDuplicateEnumTypeDefinitionVisitor(), `
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
		run(t, newRemoveDuplicateEnumTypeDefinitionVisitor(), `
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
		run(t, newRemoveDuplicateEnumTypeDefinitionVisitor(), `
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
		run(t, newRemoveDuplicateEnumTypeDefinitionVisitor(), `
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
}
