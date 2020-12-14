package plan

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/jensneuse/graphql-go-tools/internal/pkg/unsafeparser"
	"github.com/jensneuse/graphql-go-tools/pkg/astnormalization"
	"github.com/jensneuse/graphql-go-tools/pkg/asttransform"
	"github.com/jensneuse/graphql-go-tools/pkg/astvalidation"
	"github.com/jensneuse/graphql-go-tools/pkg/engine/resolve"
	"github.com/jensneuse/graphql-go-tools/pkg/operationreport"
)

func TestPlanner_Plan(t *testing.T) {
	testLogic := func(definition, operation, operationName string, config Configuration, report *operationreport.Report) Plan {
		def := unsafeparser.ParseGraphqlDocumentString(definition)
		op := unsafeparser.ParseGraphqlDocumentString(operation)
		err := asttransform.MergeDefinitionWithBaseSchema(&def)
		if err != nil {
			t.Fatal(err)
		}
		norm := astnormalization.NewNormalizer(true, true)
		norm.NormalizeOperation(&op, &def, report)
		valid := astvalidation.DefaultOperationValidator()
		valid.Validate(&op, &def, report)
		p := NewPlanner(config)
		return p.Plan(&op, &def, operationName, report)
	}

	test := func(definition, operation, operationName string, expectedPlan Plan, config Configuration) func(t *testing.T) {
		return func(t *testing.T) {
			var report operationreport.Report
			plan := testLogic(definition, operation, operationName, config, &report)
			if report.HasErrors() {
				t.Fatal(report.Error())
			}
			assert.Equal(t, expectedPlan, plan)
		}
	}

	testWithError := func(definition, operation, operationName string, config Configuration) func(t *testing.T) {
		return func(t *testing.T) {
			var report operationreport.Report
			_ = testLogic(definition, operation, operationName, config, &report)
			assert.Error(t, report)
		}
	}

	t.Run("stream & defer Query", test(testDefinition, `
		query MyQuery($id: ID!) @flushInterval(milliSeconds: 100) {
			droid(id: $id){
				name
				aliased: name
				friends @stream {
					name
				}
				friendsWithInitialBatch: friends @stream(initialBatchSize: 5) {
					name
				}
				primaryFunction
				favoriteEpisode @defer
			}
		}
	`, "MyQuery", &SynchronousResponsePlan{
		FlushInterval: 100,
		Response: &resolve.GraphQLResponse{
			Data: &resolve.Object{
				Fields: []*resolve.Field{
					{
						Name: []byte("droid"),
						Value: &resolve.Object{
							Path:     []string{"droid"},
							Nullable: true,
							Fields: []*resolve.Field{
								{
									Name: []byte("name"),
									Value: &resolve.String{
										Path: []string{"name"},
									},
								},
								{
									Name: []byte("aliased"),
									Value: &resolve.String{
										Path: []string{"name"},
									},
								},
								{
									Name: []byte("friends"),
									Stream: &resolve.StreamField{
										InitialBatchSize: 0,
									},
									Value: &resolve.Array{
										Nullable: true,
										Path:     []string{"friends"},
										Item: &resolve.Object{
											Nullable: true,
											Fields: []*resolve.Field{
												{
													Name: []byte("name"),
													Value: &resolve.String{
														Path: []string{"name"},
													},
												},
											},
										},
									},
								},
								{
									Name: []byte("friendsWithInitialBatch"),
									Stream: &resolve.StreamField{
										InitialBatchSize: 5,
									},
									Value: &resolve.Array{
										Nullable: true,
										Path:     []string{"friends"},
										Item: &resolve.Object{
											Nullable: true,
											Fields: []*resolve.Field{
												{
													Name: []byte("name"),
													Value: &resolve.String{
														Path: []string{"name"},
													},
												},
											},
										},
									},
								},
								{
									Name: []byte("primaryFunction"),
									Value: &resolve.String{
										Path: []string{"primaryFunction"},
									},
								},
								{
									Name:  []byte("favoriteEpisode"),
									Defer: &resolve.DeferField{},
									Value: &resolve.String{
										Nullable: true,
										Path:     []string{"favoriteEpisode"},
									},
								},
							},
						},
					},
				},
			},
		},
	}, Configuration{
		DefaultFlushInterval: 0,
	}))

	t.Run("operation selection", func(t *testing.T) {
		t.Run("should successfully plan a single named query by providing an operation name", test(testDefinition, `
				query MyHero {
					hero{
						name
					}
				}
			`, "MyHero", expectedMyHeroPlan, Configuration{},
		))

		t.Run("should successfully plan multiple named queries by providing an operation name", test(testDefinition, `
				query MyDroid($id: ID!) {
					droid(id: $id){
						name
					}
				}
		
				query MyHero {
					hero{
						name
					}
				}
			`, "MyHero", expectedMyHeroPlan, Configuration{},
		))

		t.Run("should successfully plan a single named query without providing an operation name", test(testDefinition, `
				query MyHero {
					hero{
						name
					}
				}
			`, "", expectedMyHeroPlan, Configuration{},
		))

		t.Run("should successfully plan a single unnamed query without providing an operation name", test(testDefinition, `
				{
					hero{
						name
					}
				}
			`, "", expectedMyHeroPlan, Configuration{},
		))

		t.Run("should write into error report when no query with name was found", testWithError(testDefinition, `
				query MyHero {
					hero{
						name
					}
				}
			`, "NoHero", Configuration{},
		))

		t.Run("should write into error report when no operation name was provided on multiple named queries", testWithError(testDefinition, `
				query MyDroid($id: ID!) {
					droid(id: $id){
						name
					}
				}
		
				query MyHero {
					hero{
						name
					}
				}
			`, "", Configuration{},
		))
	})

}

var expectedMyHeroPlan = &SynchronousResponsePlan{
	FlushInterval: 0,
	Response: &resolve.GraphQLResponse{
		Data: &resolve.Object{
			Fields: []*resolve.Field{
				{
					Name: []byte("hero"),
					Value: &resolve.Object{
						Path:     []string{"hero"},
						Nullable: true,
						Fields: []*resolve.Field{
							{
								Name: []byte("name"),
								Value: &resolve.String{
									Path: []string{"name"},
								},
							},
						},
					},
				},
			},
		},
	},
}

const testDefinition = `

directive @defer on FIELD

directive @flushInterval(milliSeconds: Int!) on QUERY | SUBSCRIPTION

directive @stream(initialBatchSize: Int) on FIELD

union SearchResult = Human | Droid | Starship

schema {
    query: Query
    mutation: Mutation
    subscription: Subscription
}

type Query {
    hero: Character
    droid(id: ID!): Droid
    search(name: String!): SearchResult
}

type Mutation {
    createReview(episode: Episode!, review: ReviewInput!): Review
}

type Subscription {
    remainingJedis: Int!
	newReviews: Review
}

input ReviewInput {
    stars: Int!
    commentary: String
}

type Review {
    id: ID!
    stars: Int!
    commentary: String
}

enum Episode {
    NEWHOPE
    EMPIRE
    JEDI
}

interface Character {
    name: String!
    friends: [Character]
}

type Human implements Character {
    name: String!
    height: String!
    friends: [Character]
}

type Droid implements Character {
    name: String!
    primaryFunction: String!
    friends: [Character]
	favoriteEpisode: Episode
}

type Startship {
    name: String!
    length: Float!
}`
