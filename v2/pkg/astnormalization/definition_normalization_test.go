package astnormalization

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/astprinter"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/internal/unsafeparser"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/operationreport"
)

func TestNormalizeDefinition(t *testing.T) {
	run := func(t *testing.T, definition, expectedOutput string) {
		t.Helper()

		definitionDocument := unsafeparser.ParseGraphqlDocumentString(definition)
		expectedOutputDocument := unsafeparser.ParseGraphqlDocumentString(expectedOutput)

		report := operationreport.Report{}
		normalizer := NewDefinitionNormalizer()
		normalizer.NormalizeDefinition(&definitionDocument, &report)

		if report.HasErrors() {
			t.Fatal(report.Error())
		}

		got := mustString(astprinter.PrintString(&definitionDocument))
		want := mustString(astprinter.PrintString(&expectedOutputDocument))

		assert.Equal(t, want, got)
	}

	t.Run("removes extensions and creates missing types", func(t *testing.T) {
		run(t, typeExtensionsDefinition, `
			schema { query: Query }
			
			type User implements Entity {
				name: String
				id: ID
				age: Int
				type: UserType
				metadata: JSONPayload
			}

			type TrialUser {
				enabled: Boolean
			}
			
			type SubscribedUser {
				subscription: SubscriptionType
			}

			enum SubscriptionType {
				BASIC
				PRO
				ULTIMATE
			}

			scalar JSONPayload

			union UserType = TrialUser | SubscribedUser
			
			type Query {
				findUserByLocation(loc: Location): [User]
			}
			
			interface Entity {
				id: ID
			}
				
			enum Planet {
				EARTH
				MARS
			}
			
			input Location {
				lat: Float 
				lon: Float
				planet: Planet
			}
		`)
	})

	t.Run("removes type extension and includes interfaces when type already has implements interface", func(t *testing.T) {
		run(t, `
			schema { query: Query }
			
			type User implements Named {
				name: String
			}
	
			interface Named {
				name: String
			}

			extend type User implements Entity {
				id: ID
			}
			
			interface Entity {
				id: ID
			}
		`, `
			schema { query: Query }
			
			type User implements Named & Entity {
				name: String
				id: ID
			}
	
			interface Named {
				name: String
			}
			
			interface Entity {
				id: ID
			}
		`)
	})

	t.Run("removes extensions and creates missing schema and root operation types", func(t *testing.T) {
		run(t, extendedRootOperationTypeDefinition, `
			schema {
				query: Query
				mutation: Mutation
				subscription: Subscription
			}
			type Query {
				me: String
			}
			type Mutation {
				increaseTextCounter: String
			}
			type Subscription {
				textCounter: String
			}
		`)
	})

	t.Run("merges schema extension directives into schema definition", func(t *testing.T) {
		run(t, `
			directive @a on SCHEMA
			schema { query: Query }
			extend schema @a
			type Query { me: String }
		`, `
			directive @a on SCHEMA
			schema @a { query: Query }
			type Query { me: String }
		`)
	})

	t.Run("merges schema extension root operation types into schema definition", func(t *testing.T) {
		run(t, `
			directive @a on SCHEMA
			extend schema @a { mutation: Mutation }
			schema { query: Query }
			type Query { me: String }
			type Mutation { increase: String }
		`, `
			directive @a on SCHEMA
			schema @a { query: Query mutation: Mutation }
			type Query { me: String }
			type Mutation { increase: String }
		`)
	})

	t.Run("creates schema definition from orphan schema extension", func(t *testing.T) {
		run(t, `
			directive @a on SCHEMA
			extend schema @a
			type Query { me: String }
		`, `
			directive @a on SCHEMA
			type Query { me: String }
			schema @a { query: Query }
		`)
	})

	t.Run("creates schema definition from orphan schema extension with root operation types", func(t *testing.T) {
		run(t, `
			directive @a on SCHEMA
			extend schema @a { query: Query }
			type Query { me: String }
		`, `
			directive @a on SCHEMA
			type Query { me: String }
			schema @a { query: Query }
		`)
	})

	t.Run("creates empty schema definition from orphan schema extension without root operation types", func(t *testing.T) {
		run(t, `
			directive @a on SCHEMA
			extend schema @a
			type Entity { id: ID! }
		`, `
			directive @a on SCHEMA
			type Entity { id: ID! }
			schema @a {}
		`)
	})
}

func TestNormalizeSubgraphDefinition(t *testing.T) {
	run := func(t *testing.T, definition, expectedOutput string) {
		t.Helper()

		definitionDocument := unsafeparser.ParseGraphqlDocumentString(definition)
		expectedOutputDocument := unsafeparser.ParseGraphqlDocumentString(expectedOutput)

		report := operationreport.Report{}
		normalizer := NewSubgraphDefinitionNormalizer()
		normalizer.NormalizeDefinition(&definitionDocument, &report)

		if report.HasErrors() {
			t.Fatal(report.Error())
		}

		got := mustString(astprinter.PrintString(&definitionDocument))
		want := mustString(astprinter.PrintString(&expectedOutputDocument))

		assert.Equal(t, want, got)
	}

	t.Run("Extension orphans are not deleted", func(t *testing.T) {
		run(t, `
			extend type Rival {
				version: Version!
			}

			enum Badge {
				BOULDER
				SOUL
			}

			extend enum Version {
				SILVER
			}

			extend input Deposit {
				quantity: Int!
			}

			extend interface GymLeader {
				badge: Badge!
			}
			
			type Pokemon {
				name: String!
			}
			
			extend interface Trainer {
				age: Int!
			}

			union Types = Water | Fire
	
			extend input Move {
				name: String
			}

			input Deposit {
				item: String!
			}
			
			extend enum Badge {
				EARTH
			}
			
			extend union Berry = Oran
	
			extend type Pokemon {
				types: Types!
			}
			
			extend union Types = Grass
			
			interface Trainer {
				name: String!
			}
		`, `
			extend type Rival {
				version: Version!
			}

			enum Badge {
				BOULDER
				SOUL
				EARTH
			}

			extend enum Version {
				SILVER
			}

			extend interface GymLeader {
				badge: Badge!
			}

			type Pokemon {
				name: String!
				types: Types!
			}

			union Types = Water | Fire | Grass

			extend input Move {
				name: String
			}

			input Deposit {
				item: String!
				quantity: Int!
			}
			
			extend union Berry = Oran

			interface Trainer {
				name: String!
				age: Int!
			}
		`)
	})
}

func TestNormalizeSubgraphDefinitionSchemaExtension(t *testing.T) {
	run := func(t *testing.T, definition, expectedOutput string) {
		t.Helper()

		definitionDocument := unsafeparser.ParseGraphqlDocumentString(definition)
		expectedOutputDocument := unsafeparser.ParseGraphqlDocumentString(expectedOutput)

		report := operationreport.Report{}
		normalizer := NewSubgraphDefinitionNormalizer()
		normalizer.NormalizeDefinition(&definitionDocument, &report)

		if report.HasErrors() {
			t.Fatal(report.Error())
		}

		got := mustString(astprinter.PrintString(&definitionDocument))
		want := mustString(astprinter.PrintString(&expectedOutputDocument))

		assert.Equal(t, want, got)
	}

	t.Run("merges schema extension into schema definition", func(t *testing.T) {
		run(t, `
			directive @a on SCHEMA
			schema { query: Query }
			extend schema @a
			type Query { me: String }
		`, `
			directive @a on SCHEMA
			schema @a { query: Query }
			type Query { me: String }
		`)
	})

	t.Run("keeps orphan schema extension", func(t *testing.T) {
		run(t, `
			directive @a on SCHEMA
			extend schema @a
			type Entity { id: ID! }
		`, `
			directive @a on SCHEMA
			extend schema @a
			type Entity { id: ID! }
		`)
	})
}
