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
	test := func(definition, operation, operationName string, expectedPlan Plan, config Configuration) func(t *testing.T) {
		return func(t *testing.T) {
			def := unsafeparser.ParseGraphqlDocumentString(definition)
			op := unsafeparser.ParseGraphqlDocumentString(operation)
			err := asttransform.MergeDefinitionWithBaseSchema(&def)
			if err != nil {
				t.Fatal(err)
			}
			norm := astnormalization.NewNormalizer(true, true)
			var report operationreport.Report
			norm.NormalizeOperation(&op, &def, &report)
			valid := astvalidation.DefaultOperationValidator()
			valid.Validate(&op, &def, &report)
			p := NewPlanner(config)
			plan := p.Plan(&op, &def, operationName, &report)
			if report.HasErrors() {
				t.Fatal(report.Error())
			}
			assert.Equal(t, expectedPlan, plan)
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
