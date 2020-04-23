package graphqldatasource

import (
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	. "github.com/jensneuse/graphql-go-tools/pkg/engine/datasourcetesting"
	"github.com/jensneuse/graphql-go-tools/pkg/engine/plan"
	"github.com/jensneuse/graphql-go-tools/pkg/engine/resolve"
)

// TODO: next steps -> add tests for GraphQL DS Load, then complete GraphQL DS Plan test, next implement GraphQL Planner

/*
operation MyQuery($id: ID!){
	droid(id: $id){
		name
		aliased: name
		friends {
			name
		}
		primaryFunction
	}
}
*/

func TestGraphQLDataSourcePlanning(t *testing.T) {
	t.Run("simple named Query", RunTest(testDefinition, `
		query MyQuery($id: ID!){
			droid(id: $id){
				name
			}
		}
	`, "MyQuery", &plan.SynchronousResponsePlan{
		Response: resolve.GraphQLResponse{
			Data: &resolve.Object{
				Fetch: &resolve.SingleFetch{
					DataSource: &Source{
						Client: http.Client{
							Timeout: time.Second * 10,
						},
					},
					BufferId:   0,
					Input:      []byte(`{"url":"https://swapi.com/graphql","query":"query($id: ID!){droid(id: $id){name}}","variables":{"id":$$0$$}}`),
					Variables: []resolve.Variable{
						&resolve.ContextVariable{
							Path: []string{"id"},
						},
					},
				},
				FieldSets: []resolve.FieldSet{
					{
						Fields: []resolve.Field{
							{
								Name: []byte("droid"),
								Value: &resolve.Object{
									FieldSets: []resolve.FieldSet{
										{
											Fields: []resolve.Field{
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
					},
				},
			},
		},
	}, plan.Configuration{
		DataSources: []plan.DataSourceConfiguration{
			{
				TypeName:   "Query",
				FieldNames: []string{"droid"},
				Attributes: []plan.DataSourceAttribute{
					{
						Key:   "url",
						Value: []byte("https://swapi.com/graphql"),
					},
					{
						Key: "arguments",
						Value: ArgumentsConfigJSON(ArgumentsConfig{
							Fields: []FieldConfig{
								{
									FieldName: "droid",
									Arguments: []Argument{
										{
											Name:       "id",
											Source:     Field,
											SourcePath: []string{"id"},
										},
									},
								},
							},
						}),
					},
				},
				DataSourcePlanner: &Planner{},
			},
		},
	}))
}

func TestGraphQLDataSourceExecution(t *testing.T) {
	test := func(ctx func() context.Context, input func(server *httptest.Server) string, serverHandler func(t *testing.T) http.HandlerFunc, result func(t *testing.T, bufPair *resolve.BufPair, err error)) func(t *testing.T) {
		return func(t *testing.T) {
			server := httptest.NewServer(serverHandler(t))
			defer server.Close()
			source := &Source{}
			bufPair := &resolve.BufPair{
				Data:   &bytes.Buffer{},
				Errors: &bytes.Buffer{},
			}
			err := source.Load(ctx(), []byte(input(server)), bufPair)
			result(t, bufPair, err)
		}
	}

	t.Run("simple", test(func() context.Context {
		return context.Background()
	}, func(server *httptest.Server) string {
		return fmt.Sprintf(`{"url":"%s","operation":"query($id: ID!){droid(id: $id){name}}","variables":{"id":1}}`, server.URL)
	}, func(t *testing.T) http.HandlerFunc {
		return func(writer http.ResponseWriter, request *http.Request) {
			body, err := ioutil.ReadAll(request.Body)
			assert.NoError(t, err)
			assert.Equal(t, `{"operation":"query($id: ID!){droid(id: $id){name}}","variables":{"id":1}}`, string(body))
			assert.Equal(t, request.Method, http.MethodPost)
			_, err = writer.Write([]byte(`{"data":{"droid":{"name":"r2d2"}}"}`))
			assert.NoError(t, err)
		}
	}, func(t *testing.T, bufPair *resolve.BufPair, err error) {
		assert.NoError(t, err)
		assert.Equal(t, `{"droid":{"name":"r2d2"}}`, bufPair.Data.String())
		assert.Equal(t, false, bufPair.HasErrors())
	}))
}

const testDefinition = `
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
}

type Startship {
    name: String!
    length: Float!
}`
