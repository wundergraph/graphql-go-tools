package sdlmerge

import "testing"

func TestExtendEnumObjectType(t *testing.T) {
	t.Run("extend simple enum type by field", func(t *testing.T) {
		run(t, newExtendEnumTypeDefinition(), `
			enum Starters {
				BULBASAUR
				CHARMANDER
				SQUIRTLE
			}

			extend enum Starters {
				CHIKORITA
			}
		`, `
			enum Starters {
				BULBASAUR
				CHARMANDER
				SQUIRTLE
				CHIKORITA
			}

			extend enum Starters {
				CHIKORITA
			}
		`)
	})

	t.Run("extend simple enum type by directive", func(t *testing.T) {
		run(t, newExtendEnumTypeDefinition(), `
			enum Starters {
				BULBASAUR
				CHARMANDER
				SQUIRTLE
			}

			extend enum Starters @deprecated(reason: "some reason")
		`, `
			enum Starters @deprecated(reason: "some reason") {
				BULBASAUR
				CHARMANDER
				SQUIRTLE
			}

			extend enum Starters @deprecated(reason: "some reason")
		`)
	})

	t.Run("extend enum type by complex extends", func(t *testing.T) {
		run(t, newExtendEnumTypeDefinition(), `
			enum Starters {
				BULBASAUR
				CHARMANDER
				SQUIRTLE
			}

			extend enum Starters @deprecated(reason: "some reason") @skip(if: false) {
				CHIKORITA
				CYNDAQUIL
			}
		`, `
			enum Starters @deprecated(reason: "some reason") @skip(if: false) {
				BULBASAUR
				CHARMANDER
				SQUIRTLE
				CHIKORITA
				CYNDAQUIL
			}

			extend enum Starters @deprecated(reason: "some reason") @skip(if: false) {
				CHIKORITA
				CYNDAQUIL
			}
		`)
	})

	// When federating, duplicate value types must be identical or the federation will fail.
	// Consequently, when extending, all duplicate value types should also be extended.
	t.Run("Duplicate enums are each extended", func(t *testing.T) {
		run(t, newExtendEnumTypeDefinition(), `
			enum Starters {
				BULBASAUR
				CHARMANDER
				SQUIRTLE
			}

			enum Starters {
				BULBASAUR
				CHARMANDER
				SQUIRTLE
			}

			extend enum Starters @deprecated(reason: "some reason") @skip(if: false) {
				CHIKORITA
				CYNDAQUIL
			}
		`, `
			enum Starters @deprecated(reason: "some reason") @skip(if: false) {
				BULBASAUR
				CHARMANDER
				SQUIRTLE
				CHIKORITA
				CYNDAQUIL
			}

			enum Starters @deprecated(reason: "some reason") @skip(if: false) {
				BULBASAUR
				CHARMANDER
				SQUIRTLE
				CHIKORITA
				CYNDAQUIL
			}

			extend enum Starters @deprecated(reason: "some reason") @skip(if: false) {
				CHIKORITA
				CYNDAQUIL
			}
		`)
	})
}
