package sdlmerge

import (
	"testing"
)

func TestExtendUnionType(t *testing.T) {
	t.Run("extend union types", func(t *testing.T) {
		run(t, newExtendUnionTypeDefinition(), `
			type Dog {
				name: String
			}

			union Animal = Dog
			
			type Cat {
				name: String
			}

			type Bird {
				name: String
			}

			extend union Animal = Bird | Cat
		`, `
			type Dog {
				name: String
			}

			union Animal = Dog | Bird | Cat
			
			type Cat {
				name: String
			}

			type Bird {
				name: String
			}

			extend union Animal = Bird | Cat
		`)
	})

	t.Run("Extending a union that is a shared type returns an error", func(t *testing.T) {
		runAndExpectError(t, newExtendUnionTypeDefinition(), `
			type Dog {
				name: String
			}

			union Animal = Dog
			
			type Cat {
				name: String
			}

			type Bird {
				name: String
			}

			union Animal = Dog

			extend union Animal = Bird | Cat
		`, sharedTypeExtensionErrorMessage("Animal"))
	})

	t.Run("Unresolved union extension orphan returns an error", func(t *testing.T) {
		runAndExpectError(t, newExtendUnionTypeDefinition(), `
			extend union Badges = Boulder
		`, unresolvedExtensionOrphansErrorMessage("Badges"))
	})
}
