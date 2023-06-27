package astnormalization

import "testing"

func TestExtendsDirective(t *testing.T) {
	t.Run("@extends on object type definition", func(t *testing.T) {
		t.Run("support extends directive", func(_ *testing.T) {
			runManyOnDefinition(t, `
				type User @extends {
					field: String!
				}
				`, `
					extend type User { field: String! }
				`, extendsDirective,
			)
		})

		t.Run("delete extends directive", func(_ *testing.T) {
			runManyOnDefinition(t, `
				type User @extends @otherDirective {
					field: String!
				}
				`, `
					extend type User @otherDirective { field: String! }
				`, extendsDirective,
			)
		})
	})

	t.Run("@extends on interface type definition", func(t *testing.T) {
		t.Run("support extends directive", func(_ *testing.T) {
			runManyOnDefinition(t, `
				interface Vehicle @extends {
					speed: Int!
				}
				`, `
					extend interface Vehicle { speed: Int! }
				`, extendsDirective,
			)
		})

		t.Run("delete extends directive", func(_ *testing.T) {
			runManyOnDefinition(t, `
				interface Vehicle @extends @otherDirective {
					speed: Int!
				}
				`, `
					extend interface Vehicle @otherDirective { speed: Int! }
				`, extendsDirective,
			)
		})
	})
}
