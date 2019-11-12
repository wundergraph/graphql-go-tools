package astnormalization

import "testing"

func TestExtendObjectType(t *testing.T) {
	t.Run("simple", func(t *testing.T) {
		run(extendObjectType, testDefinition, `
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
}
