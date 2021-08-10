package astnormalization

import "testing"

func TestImplicitExtendRootOperation(t *testing.T) {
	runNormalizeSubgraphSDL(t, "implicit extend root operation from schema",
		`schema {
			query: QueryName
			mutation: MutationName
		}
		type QueryName @hello {
		}
		type MutationName {
			field: String!
		}
		scalar String
		`,
		`
		schema { query: QueryName mutation: MutationName }
		extend type QueryName @hello{}
		extend type MutationName { field: String! }
		scalar String
	`, registerNormalizeFunc(implicitExtendRootOperation))
	runNormalizeSubgraphSDL(t, "don't implicitly extend empty schema root operation",
		`schema {
			query: QueryName
		}
		type QueryName {
		}
		`,
		`
		schema { query: QueryName }
		type QueryName {}
	`, registerNormalizeFunc(implicitExtendRootOperation))
	runNormalizeSubgraphSDL(t, "don't implicitly extend empty object root operation",
		`type Query {}
		type Mutation {
			field: String!
		}
		type Subscription @directive {
		}
		`,
		`
		type Query {}
		extend type Mutation { field: String! }
		extend type Subscription @directive {}
	`, registerNormalizeFunc(implicitExtendRootOperation))
	runNormalizeSubgraphSDL(t, "implicitly extend object root operation with definitions and directives",
		`type Query {}
		type Mutation {
			field: String!
		}
		type Subscription @directive {
			newUser: ID!
		}
		`,
		`
		type Query {}
		extend type Mutation { field: String! }
		extend type Subscription @directive { newUser: ID! }
	`, registerNormalizeFunc(implicitExtendRootOperation))
}
