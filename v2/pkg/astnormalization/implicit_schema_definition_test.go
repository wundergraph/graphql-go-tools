package astnormalization

import (
	"testing"
)

func TestImplicitSchemaDefinition(t *testing.T) {
	t.Run("should create schema definition implicitly", func(t *testing.T) {
		run(t, implicitSchemaDefinition, "", `
					extend type Query { me: String }
					extend type Mutation { increaseTextCounter: String }
					extend type Subscription { textCounter: String }
					type Query { me: String }
					type Mutation { increaseTextCounter: String }
					type Subscription { textCounter: String }
					 `, `
					schema { query: Query mutation: Mutation subscription: Subscription }
					extend type Query { me: String }
					extend type Mutation { increaseTextCounter: String }
					extend type Subscription { textCounter: String }
					type Query { me: String }
					type Mutation { increaseTextCounter: String }
					type Subscription { textCounter: String }
					`)
	})

	t.Run("should replace empty schema definition with implicit one", func(t *testing.T) {
		run(t, implicitSchemaDefinition, "", `
					schema {}
					extend type Query { me: String }
					extend type Mutation { increaseTextCounter: String }
					extend type Subscription { textCounter: String }
					type Query { me: String }
					type Mutation { increaseTextCounter: String }
					type Subscription { textCounter: String }
					 `, `
					schema { query: Query mutation: Mutation subscription: Subscription }
					extend type Query { me: String }
					extend type Mutation { increaseTextCounter: String }
					extend type Subscription { textCounter: String }
					type Query { me: String }
					type Mutation { increaseTextCounter: String }
					type Subscription { textCounter: String }
					`)
	})

	t.Run("should ignore schema definition if there is already one explicitly defined", func(t *testing.T) {
		run(t, implicitSchemaDefinition, "", `
					schema { query: Query }
					extend type Query { me: String }
					extend type Mutation { increaseTextCounter: String }
					extend type Subscription { textCounter: String }
					type Query { me: String }
					type Mutation { increaseTextCounter: String }
					type Subscription { textCounter: String }
					 `, `
					schema { query: Query }
					extend type Query { me: String }
					extend type Mutation { increaseTextCounter: String }
					extend type Subscription { textCounter: String }
					type Query { me: String }
					type Mutation { increaseTextCounter: String }
					type Subscription { textCounter: String }
					`)
	})
}
