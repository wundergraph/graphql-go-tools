package sdlmerge

import (
	"testing"
)

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

	t.Run("Same name enums with different values return an error", func(t *testing.T) {
		runAndExpectError(t, newRemoveDuplicateFieldlessValueTypesVisitor(), `
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
		`, FederatingFieldlessValueTypeErrorMessage(pokemon))
	})

	t.Run("Empty and populated same name enums return an error", func(t *testing.T) {
		runAndExpectError(t, newRemoveDuplicateFieldlessValueTypesVisitor(), `
			enum Pokemon {
			}

			enum Pokemon {
				CHARMANDER,
				SQUIRTLE,
			}
		`, FederatingFieldlessValueTypeErrorMessage(pokemon))
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

	t.Run("Same name enums with no overlapping values return an error", func(t *testing.T) {
		runAndExpectError(t, newRemoveDuplicateFieldlessValueTypesVisitor(), `
			enum Pokemon {
				BULBASAUR,
				CHARMANDER,
			}

			enum Pokemon {
				SQUIRTLE,
				MEW,
			}
		`, FederatingFieldlessValueTypeErrorMessage(pokemon))
	})

	t.Run("Same name enums with varying overlapping values return an error", func(t *testing.T) {
		runAndExpectError(t, newRemoveDuplicateFieldlessValueTypesVisitor(), `
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
		`, FederatingFieldlessValueTypeErrorMessage(pokemon))
	})

	t.Run("Different groups of same name enums return an error immediately upon invalidation", func(t *testing.T) {
		runAndExpectError(t, newRemoveDuplicateFieldlessValueTypesVisitor(), `
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
		`, FederatingFieldlessValueTypeErrorMessage(types))
	})

	t.Run("Input and output are identical when no scalar duplications", func(t *testing.T) {
		run(t, newRemoveDuplicateFieldlessValueTypesVisitor(), `
			scalar DateTime
		`, `
			scalar DateTime
		`)
	})

	t.Run("Same name scalars are removed to leave only one", func(t *testing.T) {
		run(t, newRemoveDuplicateFieldlessValueTypesVisitor(), `
			scalar DateTime

			scalar DateTime

			scalar DateTime
		`, `
			scalar DateTime
		`)
	})

	t.Run("Any more than one of a same name scalar are removed", func(t *testing.T) {
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

	t.Run("Identical same name unions are merged", func(t *testing.T) {
		run(t, newRemoveDuplicateFieldlessValueTypesVisitor(), `
			union Types = Grass | Fire | Water
	
			union Types = Grass | Fire | Water
		`, `
			union Types = Grass | Fire | Water
		`)
	})

	t.Run("Same name unions with different values return an error", func(t *testing.T) {
		runAndExpectError(t, newRemoveDuplicateFieldlessValueTypesVisitor(), `
			union Types = Grass | Fire | Water

			union Types = Grass | Fire | Water | Rock
		`, FederatingFieldlessValueTypeErrorMessage(types))
	})

	t.Run("Same name unions with no overlapping values return an error", func(t *testing.T) {
		runAndExpectError(t, newRemoveDuplicateFieldlessValueTypesVisitor(), `
			union Types = Grass | Fire

			union Types = Water | Rock
		`, FederatingFieldlessValueTypeErrorMessage(types))
	})

	t.Run("Same name unions with varying overlapping values return an error", func(t *testing.T) {
		runAndExpectError(t, newRemoveDuplicateFieldlessValueTypesVisitor(), `
			union Types = Grass | Fire

			union Types = Fire | Water

			union Types = Rock | Grass

			union Types = Water | Fire
		`, FederatingFieldlessValueTypeErrorMessage(types))
	})

	t.Run("Different groups of same name unions return an error immediately upon invalidation", func(t *testing.T) {
		runAndExpectError(t, newRemoveDuplicateFieldlessValueTypesVisitor(), `
			union Cities = Cerulean | Saffron

			union Types = Grass | Fire | Rock

			union Badges = Boulder | Volcano | Earth

			union Cities = Viridian | Saffron | Celadon

			union Types = Rock | Grass | Fire | Water
			
			union Badges = Marsh | Soul | Volcano | Thunder | Rainbow | Cascade
			
			union Badges = Volcano | Rainbow | Boulder | Soul

			union Types = Water | Fire

			union Badges = Earth | Thunder

			union Cities = Cerulean | Celadon
		`, FederatingFieldlessValueTypeErrorMessage(cities))
	})
}

const (
	cities  = "Cities"
	pokemon = "Pokemon"
	types   = "Types"
)
