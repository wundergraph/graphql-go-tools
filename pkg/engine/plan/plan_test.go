package plan

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/wundergraph/graphql-go-tools/internal/pkg/unsafeparser"
	"github.com/wundergraph/graphql-go-tools/pkg/astnormalization"
	"github.com/wundergraph/graphql-go-tools/pkg/asttransform"
	"github.com/wundergraph/graphql-go-tools/pkg/astvalidation"
	"github.com/wundergraph/graphql-go-tools/pkg/engine/resolve"
	"github.com/wundergraph/graphql-go-tools/pkg/operationreport"
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
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		p := NewPlanner(ctx, config)
		return p.Plan(&op, &def, operationName, report)
	}

	test := func(definition, operation, operationName string, expectedPlan Plan, config Configuration) func(t *testing.T) {
		return func(t *testing.T) {
			t.Helper()

			var report operationreport.Report
			plan := testLogic(definition, operation, operationName, config, &report)
			if report.HasErrors() {
				t.Fatal(report.Error())
			}
			assert.Equal(t, expectedPlan, plan)

			toJson := func(v interface{}) string {
				b := &strings.Builder{}
				e := json.NewEncoder(b)
				e.SetIndent("", " ")
				_ = e.Encode(v)
				return b.String()
			}

			assert.Equal(t, toJson(expectedPlan), toJson(plan))

		}
	}

	testWithError := func(definition, operation, operationName string, config Configuration) func(t *testing.T) {
		return func(t *testing.T) {
			t.Helper()

			var report operationreport.Report
			_ = testLogic(definition, operation, operationName, config, &report)
			assert.True(t, report.HasErrors())
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
						Position: resolve.Position{
							Line:   3,
							Column: 4,
						},
						Value: &resolve.Object{
							Path:     []string{"droid"},
							Nullable: true,
							Fields: []*resolve.Field{
								{
									Name: []byte("name"),
									Value: &resolve.String{
										Path: []string{"name"},
									},
									Position: resolve.Position{
										Line:   4,
										Column: 5,
									},
								},
								{
									Name: []byte("aliased"),
									Value: &resolve.String{
										Path: []string{"name"},
									},
									Position: resolve.Position{
										Line:   5,
										Column: 5,
									},
								},
								{
									Name: []byte("friends"),
									Stream: &resolve.StreamField{
										InitialBatchSize: 0,
									},
									Position: resolve.Position{
										Line:   6,
										Column: 5,
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
													Position: resolve.Position{
														Line:   7,
														Column: 6,
													},
												},
											},
										},
									},
								},
								{
									Name: []byte("friendsWithInitialBatch"),
									Position: resolve.Position{
										Line:   9,
										Column: 5,
									},
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
													Position: resolve.Position{
														Line:   10,
														Column: 6,
													},
												},
											},
										},
									},
								},
								{
									Name: []byte("primaryFunction"),
									Position: resolve.Position{
										Line:   12,
										Column: 5,
									},
									Value: &resolve.String{
										Path: []string{"primaryFunction"},
									},
								},
								{
									Name: []byte("favoriteEpisode"),
									Position: resolve.Position{
										Line:   13,
										Column: 5,
									},
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
		DefaultFlushIntervalMillis: 0,
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

		t.Run("should successfully plan unnamed query with fragment", test(testDefinition, `
				fragment CharacterFields on Character {
					name
				}
				query {
					hero {
						...CharacterFields
					}
				}
			`, "", expectedMyHeroPlanWithFragment, Configuration{},
		))

		t.Run("should successfully plan multiple named queries by providing an operation name", test(testDefinition, `
				query MyHero {
					hero {
						name
					}
				}

				query MyDroid($id: ID!) {
					droid(id: $id){
						name
					}
				}
			`, "MyHero", expectedMyHeroPlan, Configuration{},
		))

		t.Run("should successfully plan a single named query without providing an operation name", test(testDefinition, `
				query MyHero {
					hero {
						name
					}
				}
			`, "", expectedMyHeroPlan, Configuration{},
		))

		t.Run("should successfully plan a single unnamed query without providing an operation name", test(testDefinition, `
				{
					hero {
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
					Position: resolve.Position{
						Line:   3,
						Column: 6,
					},
					Value: &resolve.Object{
						Path:     []string{"hero"},
						Nullable: true,
						Fields: []*resolve.Field{
							{
								Name: []byte("name"),
								Value: &resolve.String{
									Path: []string{"name"},
								},
								Position: resolve.Position{
									Line:   4,
									Column: 7,
								},
							},
						},
					},
				},
			},
		},
	},
}

var expectedMyHeroPlanWithFragment = &SynchronousResponsePlan{
	FlushInterval: 0,
	Response: &resolve.GraphQLResponse{
		Data: &resolve.Object{
			Fields: []*resolve.Field{
				{
					Name: []byte("hero"),
					Position: resolve.Position{
						Line:   6,
						Column: 6,
					},
					Value: &resolve.Object{
						Path:     []string{"hero"},
						Nullable: true,
						Fields: []*resolve.Field{
							{
								Name: []byte("name"),
								Value: &resolve.String{
									Path: []string{"name"},
								},
								Position: resolve.Position{
									Line:   3,
									Column: 6,
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
