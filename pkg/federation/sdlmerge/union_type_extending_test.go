package sdlmerge

import "testing"

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

	// When federating, duplicate value types must be identical or the federation will fail.
	// Consequently, when extending, all duplicate value types should also be extended.
	t.Run("Duplicate unions are each extended", func(t *testing.T) {
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

			union Animal = Dog

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

			union Animal = Dog | Bird | Cat

			extend union Animal = Bird | Cat
		`)
	})
}
