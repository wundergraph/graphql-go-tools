package astnormalization

import (
	"testing"
)

func TestNormalizeSubgraphSDL(t *testing.T) {
	t.Run("support both extends directive and implicit extend keyword", func(_ *testing.T) {
		runManyOnDefinition(t, `
		type User @extends {
			field: String!
		}
		type Query @extends {}
		`, `
		extend type User { field: String! } extend type Query @extends {}
	`, registerNormalizeFunc(implicitExtendRootOperation), registerNormalizeFunc(extendsDirective))
	})
	t.Run("support both extends directive and implicit extend keyword in schema", func(_ *testing.T) {
		runManyOnDefinition(t, `
		schema {
			query: AQuery
		}
		type User @extends @directiv2 {
			field: String!
		}
		type AQuery @key {
			field: String
		}
		`, `
		schema {
			query: AQuery
		}
		extend type User @directiv2 { field: String! }
		extend type AQuery @key {
			field: String
		}
	`, registerNormalizeFunc(implicitExtendRootOperation), registerNormalizeFunc(extendsDirective))
	})
}
