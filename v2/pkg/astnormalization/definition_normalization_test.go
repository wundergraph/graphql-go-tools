package astnormalization

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/wundergraph/graphql-go-tools/v2/internal/pkg/unsafeparser"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astprinter"
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

		got := mustString(astprinter.PrintString(&definitionDocument, nil))
		want := mustString(astprinter.PrintString(&expectedOutputDocument, nil))

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

		got := mustString(astprinter.PrintString(&definitionDocument, nil))
		want := mustString(astprinter.PrintString(&expectedOutputDocument, nil))

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
