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
}
