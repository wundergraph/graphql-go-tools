package astnormalization

import "testing"

func TestExtendsDirective(t *testing.T) {
	t.Run("support extends directive", func(_ *testing.T) {
		runManyOnDefinition(
			`
		type User @extends {
			field: String!
		}
		`,
			`
		extend type User { field: String! }
	`, registerNormalizeFunc(extendsDirective))
	})
	t.Run("delete extends directive", func(_ *testing.T) {
		runManyOnDefinition(
			`
		type User @extends @directiv2 {
			field: String!
		}
		`,
			`
		extend type User @directiv2 { field: String! }
	`, registerNormalizeFunc(extendsDirective))
	})
}
