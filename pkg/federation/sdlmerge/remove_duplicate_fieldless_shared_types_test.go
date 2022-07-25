package sdlmerge

import (
	"fmt"
	"testing"
)

func TestRemoveDuplicateFieldlessSharedTypes(t *testing.T) {
	t.Run("Input and output are identical when no duplications", func(t *testing.T) {
		run(t, newRemoveDuplicateFieldlessSharedTypesVisitor(), `
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
		run(t, newRemoveDuplicateFieldlessSharedTypesVisitor(), `
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

	t.Run("Identical same name enums are merged into a single input regardless of value order", func(t *testing.T) {
		run(t, newRemoveDuplicateFieldlessSharedTypesVisitor(), `
			enum Pokemon {
				BULBASAUR,
				CHARMANDER,
				SQUIRTLE,
			}

			enum Pokemon {
				SQUIRTLE,
				CHARMANDER,
				BULBASAUR,
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
		runAndExpectError(t, newRemoveDuplicateFieldlessSharedTypesVisitor(), `
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
		`, NonIdenticalSharedTypeErrorMessage(pokemon))
	})

	t.Run("Empty and populated same name enums return an error", func(t *testing.T) {
		runAndExpectError(t, newRemoveDuplicateFieldlessSharedTypesVisitor(), `
			enum Pokemon {
			}

			enum Pokemon {
				CHARMANDER,
				SQUIRTLE,
			}
		`, NonIdenticalSharedTypeErrorMessage(pokemon))
	})

	t.Run("Empty enums are merged into a single empty enum", func(t *testing.T) {
		run(t, newRemoveDuplicateFieldlessSharedTypesVisitor(), `
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
		runAndExpectError(t, newRemoveDuplicateFieldlessSharedTypesVisitor(), `
			enum Pokemon {
				BULBASAUR,
				CHARMANDER,
			}

			enum Pokemon {
				SQUIRTLE,
				MEW,
			}
		`, NonIdenticalSharedTypeErrorMessage(pokemon))
	})

	t.Run("Same name enums with varying overlapping values return an error", func(t *testing.T) {
		runAndExpectError(t, newRemoveDuplicateFieldlessSharedTypesVisitor(), `
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
		`, NonIdenticalSharedTypeErrorMessage(pokemon))
	})

	t.Run("Different groups of same name enums return an error immediately upon invalidation", func(t *testing.T) {
		runAndExpectError(t, newRemoveDuplicateFieldlessSharedTypesVisitor(), `
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
		`, NonIdenticalSharedTypeErrorMessage(types))
	})

	t.Run("Input and output are identical when no scalar duplications", func(t *testing.T) {
		run(t, newRemoveDuplicateFieldlessSharedTypesVisitor(), `
			scalar DateTime
		`, `
			scalar DateTime
		`)
	})

	t.Run("Same name scalars are removed to leave only one", func(t *testing.T) {
		run(t, newRemoveDuplicateFieldlessSharedTypesVisitor(), `
			scalar DateTime

			scalar DateTime

			scalar DateTime
		`, `
			scalar DateTime
		`)
	})

	t.Run("Any more than one of a same name scalar are removed", func(t *testing.T) {
		run(t, newRemoveDuplicateFieldlessSharedTypesVisitor(), `
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
		run(t, newRemoveDuplicateFieldlessSharedTypesVisitor(), `
			union Types = Grass | Fire | Water
		`, `
			union Types = Grass | Fire | Water
		`)
	})

	t.Run("Identical same name unions are merged", func(t *testing.T) {
		run(t, newRemoveDuplicateFieldlessSharedTypesVisitor(), `
			union Types = Grass | Fire | Water
	
			union Types = Grass | Fire | Water
		`, `
			union Types = Grass | Fire | Water
		`)
	})

	t.Run("Identical same name unions are merged into a single input regardless of value order", func(t *testing.T) {
		run(t, newRemoveDuplicateFieldlessSharedTypesVisitor(), `
			union Types = Grass | Fire | Water
	
			union Types = Water | Grass | Fire
		`, `
			union Types = Grass | Fire | Water
		`)
	})

	t.Run("Same name unions with different values return an error", func(t *testing.T) {
		runAndExpectError(t, newRemoveDuplicateFieldlessSharedTypesVisitor(), `
			union Types = Grass | Fire | Water

			union Types = Grass | Fire | Water | Rock
		`, NonIdenticalSharedTypeErrorMessage(types))
	})

	t.Run("Same name unions with no overlapping values return an error", func(t *testing.T) {
		runAndExpectError(t, newRemoveDuplicateFieldlessSharedTypesVisitor(), `
			union Types = Grass | Fire

			union Types = Water | Rock
		`, NonIdenticalSharedTypeErrorMessage(types))
	})

	t.Run("Same name unions with varying overlapping values return an error", func(t *testing.T) {
		runAndExpectError(t, newRemoveDuplicateFieldlessSharedTypesVisitor(), `
			union Types = Grass | Fire

			union Types = Fire | Water

			union Types = Rock | Grass

			union Types = Water | Fire
		`, NonIdenticalSharedTypeErrorMessage(types))
	})

	t.Run("Different groups of same name unions return an error immediately upon invalidation", func(t *testing.T) {
		runAndExpectError(t, newRemoveDuplicateFieldlessSharedTypesVisitor(), `
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
		`, NonIdenticalSharedTypeErrorMessage(cities))
	})
}

const (
	cities  = "Cities"
	pokemon = "Pokemon"
	types   = "Types"
)

func NonIdenticalSharedTypeErrorMessage(typeName string) string {
	return fmt.Sprintf("the shared type named '%s' must be identical in any subgraphs to federate", typeName)
}
