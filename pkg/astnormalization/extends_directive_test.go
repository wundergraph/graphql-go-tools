package astnormalization

import "testing"

func TestExtendsDirective(t *testing.T) {
	runNormalizeSubgraphSDL(t, "support extends directive",
		`
		type User @extends {
			field: String!
		}
		`,
		`
		extend type User { field: String! }
	`, registerNormalizeFunc(extendsDirective))
	runNormalizeSubgraphSDL(t, "delete extends directive",
		`
		type User @extends @directiv2 {
			field: String!
		}
		`,
		`
		extend type User @directiv2 { field: String! }
	`, registerNormalizeFunc(extendsDirective))
}
