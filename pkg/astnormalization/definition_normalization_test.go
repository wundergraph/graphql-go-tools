package astnormalization

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/wundergraph/graphql-go-tools/internal/pkg/unsafeparser"
	"github.com/wundergraph/graphql-go-tools/pkg/astprinter"
	"github.com/wundergraph/graphql-go-tools/pkg/operationreport"
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
			}`,
		)
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
			}`, `
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
			}`,
		)
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
			}`,
		)
	})
}
