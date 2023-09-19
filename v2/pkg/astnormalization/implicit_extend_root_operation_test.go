package astnormalization

import "testing"

func TestImplicitExtendRootOperation(t *testing.T) {
	t.Run("implicit extend root operation from schema", func(_ *testing.T) {
		runManyOnDefinition(t, `
		schema {
			query: QueryName
			mutation: MutationName
		}
		type QueryName @hello {
		}
		type MutationName {
			field: String!
		}
		scalar String
		`, `
		schema { query: QueryName mutation: MutationName }
		extend type QueryName @hello{}
		extend type MutationName { field: String! }
		scalar String
	`, registerNormalizeFunc(implicitExtendRootOperation))
	})
	t.Run("don't implicitly extend empty schema root operation", func(_ *testing.T) {
		runManyOnDefinition(t, `
		schema {
			query: QueryName
		}
		type QueryName {
		}
		`, `
		schema { query: QueryName }
		type QueryName {}
	`, registerNormalizeFunc(implicitExtendRootOperation))
	})
	t.Run("don't implicitly extend empty object root operation", func(_ *testing.T) {
		runManyOnDefinition(t, `
			type Query {}
			type Mutation {
				field: String!
			}
			type Subscription @directive {
			}
			`, `
			type Query {}
			extend type Mutation { field: String! }
			extend type Subscription @directive {}
		`, registerNormalizeFunc(implicitExtendRootOperation))
	})
	t.Run("implicitly extend object root operation with definitions and directives", func(_ *testing.T) {
		runManyOnDefinition(t, `
		type Query {}
		type Mutation {
			field: String!
		}
		type Subscription @directive {
			newUser: ID!
		}
		`, `
		type Query {}
		extend type Mutation { field: String! }
		extend type Subscription @directive { newUser: ID! }
	`, registerNormalizeFunc(implicitExtendRootOperation))
	})

}
