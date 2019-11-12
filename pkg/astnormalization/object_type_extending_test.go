package astnormalization

import "testing"

func TestExtendObjectType(t *testing.T) {
	t.Run("extend simple object type by field", func(t *testing.T) {
		run(extendObjectTypeDefinition, testDefinition, `
					type Dog {
						name: String
					}
					extend type Dog {
						favoriteToy: String
					}
					 `, `
					type Dog {
						name: String
						favoriteToy: string
					}
					extend type Dog {
						favoriteToy: String
					}
					`)
	})
	t.Run("extend simple object type by directive", func(t *testing.T) {
		run(extendObjectTypeDefinition, testDefinition, `
					type Cat {
						name: String
					}
					extend type Cat @deprecated("not as cool as dogs")
					 `, `
					type Cat @deprecated("not as cool as dogs") {
						name: String
					}
					extend type Cat @deprecated("not as cool as dogs")
					`)
	})
}
