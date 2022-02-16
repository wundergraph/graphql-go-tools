package graphql_datasource

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jensneuse/graphql-go-tools/examples/chat"
	. "github.com/jensneuse/graphql-go-tools/pkg/engine/datasourcetesting"
	"github.com/jensneuse/graphql-go-tools/pkg/engine/plan"
	"github.com/jensneuse/graphql-go-tools/pkg/engine/resolve"
)

func TestGraphQLDataSource(t *testing.T) {
	t.Run("simple named Query", RunTest(starWarsSchema, `
		query MyQuery($id: ID!){
			droid(id: $id){
				name
				aliased: name
				friends {
					name
				}
				primaryFunction
			}
			hero {
				name
			}
			stringList
			nestedStringList
		}
	`, "MyQuery", &plan.SynchronousResponsePlan{
		Response: &resolve.GraphQLResponse{
			Data: &resolve.Object{
				Fetch: &resolve.SingleFetch{
					DataSource: &Source{},
					BufferId:   0,
					Input:      `{"method":"POST","url":"https://swapi.com/graphql","header":{"Authorization":["$$1$$"],"Invalid-Template":["{{ request.headers.Authorization }}"]},"body":{"query":"query($id: ID!){droid(id: $id){name aliased: name friends {name} primaryFunction} hero {name} stringList nestedStringList}","variables":{"id":$$0$$}}}`,
					Variables: resolve.NewVariables(
						&resolve.ContextVariable{
							Path:     []string{"id"},
							Renderer: resolve.NewJSONVariableRendererWithValidation(`{"type":["string","integer"]}`),
						},
						&resolve.HeaderVariable{
							Path: []string{"Authorization"},
						},
					),
					DataSourceIdentifier:  []byte("graphql_datasource.Source"),
					ProcessResponseConfig: resolve.ProcessResponseConfig{ExtractGraphqlResponse: true},
				},
				Fields: []*resolve.Field{
					{
						HasBuffer: true,
						BufferID:  0,
						Name:      []byte("droid"),
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
										Path: []string{"aliased"},
									},
									Position: resolve.Position{
										Line:   5,
										Column: 5,
									},
								},
								{
									Name: []byte("friends"),
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
									Name: []byte("primaryFunction"),
									Value: &resolve.String{
										Path: []string{"primaryFunction"},
									},
									Position: resolve.Position{
										Line:   9,
										Column: 5,
									},
								},
							},
						},
					},
					{
						HasBuffer: true,
						BufferID:  0,
						Name:      []byte("hero"),
						Position: resolve.Position{
							Line:   11,
							Column: 4,
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
										Line:   12,
										Column: 5,
									},
								},
							},
						},
					},
					{
						HasBuffer: true,
						BufferID:  0,
						Name:      []byte("stringList"),
						Position: resolve.Position{
							Line:   14,
							Column: 4,
						},
						Value: &resolve.Array{
							Nullable: true,
							Item: &resolve.String{
								Nullable: true,
							},
						},
					},
					{
						HasBuffer: true,
						BufferID:  0,
						Name:      []byte("nestedStringList"),
						Position: resolve.Position{
							Line:   15,
							Column: 4,
						},
						Value: &resolve.Array{
							Nullable: true,
							Path:     []string{"nestedStringList"},
							Item: &resolve.String{
								Nullable: true,
							},
						},
					},
				},
			},
		},
	}, plan.Configuration{
		DataSources: []plan.DataSourceConfiguration{
			{
				RootNodes: []plan.TypeField{
					{
						TypeName:   "Query",
						FieldNames: []string{"droid", "hero", "stringList", "nestedStringList"},
					},
				},
				ChildNodes: []plan.TypeField{
					{
						TypeName:   "Character",
						FieldNames: []string{"name", "friends"},
					},
					{
						TypeName:   "Human",
						FieldNames: []string{"name", "height", "friends"},
					},
					{
						TypeName:   "Droid",
						FieldNames: []string{"name", "primaryFunction", "friends"},
					},
				},
				Factory: &Factory{},
				Custom: ConfigJson(Configuration{
					Fetch: FetchConfiguration{
						URL: "https://swapi.com/graphql",
						Header: http.Header{
							"Authorization":    []string{"{{ .request.headers.Authorization }}"},
							"Invalid-Template": []string{"{{ request.headers.Authorization }}"},
						},
					},
				}),
			},
		},
		Fields: []plan.FieldConfiguration{
			{
				TypeName:  "Query",
				FieldName: "droid",
				Arguments: []plan.ArgumentConfiguration{
					{
						Name:       "id",
						SourceType: plan.FieldArgumentSource,
					},
				},
			},
			{
				TypeName:              "Query",
				FieldName:             "stringList",
				DisableDefaultMapping: true,
			},
			{
				TypeName:  "Query",
				FieldName: "nestedStringList",
				Path:      []string{"nestedStringList"},
			},
		},
	}))
	t.Run("selections on interface type", RunTest(interfaceSelectionSchema, `
		query MyQuery {
			user {
				id
				displayName
			}
		}
	`, "MyQuery", &plan.SynchronousResponsePlan{
		Response: &resolve.GraphQLResponse{
			Data: &resolve.Object{
				Fetch: &resolve.SingleFetch{
					DataSource:            &Source{},
					BufferId:              0,
					Input:                 `{"method":"POST","url":"https://swapi.com/graphql","body":{"query":"{user {id displayName}}"}}`,
					DataSourceIdentifier:  []byte("graphql_datasource.Source"),
					ProcessResponseConfig: resolve.ProcessResponseConfig{ExtractGraphqlResponse: true},
				},
				Fields: []*resolve.Field{
					{
						HasBuffer: true,
						BufferID:  0,
						Name:      []byte("user"),
						Position: resolve.Position{
							Line:   3,
							Column: 4,
						},
						Value: &resolve.Object{
							Path:     []string{"user"},
							Nullable: true,
							Fields: []*resolve.Field{
								{
									Name: []byte("id"),
									Value: &resolve.String{
										Path: []string{"id"},
									},
									Position: resolve.Position{
										Line:   4,
										Column: 5,
									},
								},
								{
									Name: []byte("displayName"),
									Value: &resolve.String{
										Path: []string{"displayName"},
									},
									Position: resolve.Position{
										Line:   5,
										Column: 5,
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
				RootNodes: []plan.TypeField{
					{
						TypeName:   "Query",
						FieldNames: []string{"user"},
					},
				},
				ChildNodes: []plan.TypeField{
					{
						TypeName:   "User",
						FieldNames: []string{"id", "displayName", "isLoggedIn"},
					},
					{
						TypeName:   "RegisteredUser",
						FieldNames: []string{"id", "displayName", "isLoggedIn"},
					},
				},
				Factory: &Factory{},
				Custom: ConfigJson(Configuration{
					Fetch: FetchConfiguration{
						URL: "https://swapi.com/graphql",
					},
				}),
			},
		},
		Fields: []plan.FieldConfiguration{},
	}))
	t.Run("skip directive with variable", RunTest(interfaceSelectionSchema, `
		query MyQuery ($skip: Boolean!) {
			user {
				id
				displayName @skip(if: $skip)
			}
		}
	`, "MyQuery", &plan.SynchronousResponsePlan{
		Response: &resolve.GraphQLResponse{
			Data: &resolve.Object{
				Fetch: &resolve.SingleFetch{
					DataSource:            &Source{},
					BufferId:              0,
					Input:                 `{"method":"POST","url":"https://swapi.com/graphql","body":{"query":"query($skip: Boolean!){user {id displayName @skip(if: $skip)}}","variables":{"skip":$$0$$}}}`,
					DataSourceIdentifier:  []byte("graphql_datasource.Source"),
					ProcessResponseConfig: resolve.ProcessResponseConfig{ExtractGraphqlResponse: true},
					Variables: resolve.NewVariables(
						&resolve.ContextVariable{
							Path:     []string{"skip"},
							Renderer: resolve.NewJSONVariableRendererWithValidation(`{"type":["boolean"]}`),
						},
					),
				},
				Fields: []*resolve.Field{
					{
						HasBuffer: true,
						BufferID:  0,
						Name:      []byte("user"),
						Position: resolve.Position{
							Line:   3,
							Column: 4,
						},
						Value: &resolve.Object{
							Path:     []string{"user"},
							Nullable: true,
							Fields: []*resolve.Field{
								{
									Name: []byte("id"),
									Value: &resolve.String{
										Path: []string{"id"},
									},
									Position: resolve.Position{
										Line:   4,
										Column: 5,
									},
								},
								{
									Name: []byte("displayName"),
									Value: &resolve.String{
										Path: []string{"displayName"},
									},
									Position: resolve.Position{
										Line:   5,
										Column: 5,
									},
									SkipDirectiveDefined: true,
									SkipVariableName:     "skip",
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
				RootNodes: []plan.TypeField{
					{
						TypeName:   "Query",
						FieldNames: []string{"user"},
					},
				},
				ChildNodes: []plan.TypeField{
					{
						TypeName:   "User",
						FieldNames: []string{"id", "displayName", "isLoggedIn"},
					},
					{
						TypeName:   "RegisteredUser",
						FieldNames: []string{"id", "displayName", "isLoggedIn"},
					},
				},
				Factory: &Factory{},
				Custom: ConfigJson(Configuration{
					Fetch: FetchConfiguration{
						URL: "https://swapi.com/graphql",
					},
				}),
			},
		},
		Fields: []plan.FieldConfiguration{},
	}))
	t.Run("skip directive on __typename", RunTest(interfaceSelectionSchema, `
		query MyQuery ($skip: Boolean!) {
			user {
				id
				displayName
				__typename @skip(if: $skip)
			}
		}
	`, "MyQuery", &plan.SynchronousResponsePlan{
		Response: &resolve.GraphQLResponse{
			Data: &resolve.Object{
				Fetch: &resolve.SingleFetch{
					DataSource:            &Source{},
					BufferId:              0,
					Input:                 `{"method":"POST","url":"https://swapi.com/graphql","body":{"query":"{user {__typename id displayName}}"}}`,
					DataSourceIdentifier:  []byte("graphql_datasource.Source"),
					ProcessResponseConfig: resolve.ProcessResponseConfig{ExtractGraphqlResponse: true},
				},
				Fields: []*resolve.Field{
					{
						HasBuffer: true,
						BufferID:  0,
						Name:      []byte("user"),
						Position: resolve.Position{
							Line:   3,
							Column: 4,
						},
						Value: &resolve.Object{
							Path:     []string{"user"},
							Nullable: true,
							Fields: []*resolve.Field{
								{
									Name: []byte("id"),
									Value: &resolve.String{
										Path: []string{"id"},
									},
									Position: resolve.Position{
										Line:   4,
										Column: 5,
									},
								},
								{
									Name: []byte("displayName"),
									Value: &resolve.String{
										Path: []string{"displayName"},
									},
									Position: resolve.Position{
										Line:   5,
										Column: 5,
									},
								},
								{
									Name: []byte("__typename"),
									Value: &resolve.String{
										Path: []string{"__typename"},
									},
									Position: resolve.Position{
										Line:   6,
										Column: 5,
									},
									SkipDirectiveDefined: true,
									SkipVariableName:     "skip",
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
				RootNodes: []plan.TypeField{
					{
						TypeName:   "Query",
						FieldNames: []string{"user"},
					},
				},
				ChildNodes: []plan.TypeField{
					{
						TypeName:   "User",
						FieldNames: []string{"id", "displayName", "isLoggedIn"},
					},
					{
						TypeName:   "RegisteredUser",
						FieldNames: []string{"id", "displayName", "isLoggedIn"},
					},
				},
				Factory: &Factory{},
				Custom: ConfigJson(Configuration{
					Fetch: FetchConfiguration{
						URL: "https://swapi.com/graphql",
					},
				}),
			},
		},
		Fields: []plan.FieldConfiguration{},
	}))
	t.Run("skip directive on an inline fragment", RunTest(interfaceSelectionSchema, `
		query MyQuery ($skip: Boolean!) {
			user {
				... @skip(if: $skip) {
					id
					displayName
				}
			}
		}
	`, "MyQuery", &plan.SynchronousResponsePlan{
		Response: &resolve.GraphQLResponse{
			Data: &resolve.Object{
				Fetch: &resolve.SingleFetch{
					DataSource:            &Source{},
					BufferId:              0,
					Input:                 `{"method":"POST","url":"https://swapi.com/graphql","body":{"query":"query($skip: Boolean!){user {... @skip(if: $skip){id displayName}}}","variables":{"skip":$$0$$}}}`,
					DataSourceIdentifier:  []byte("graphql_datasource.Source"),
					ProcessResponseConfig: resolve.ProcessResponseConfig{ExtractGraphqlResponse: true},
					Variables: resolve.NewVariables(
						&resolve.ContextVariable{
							Path:     []string{"skip"},
							Renderer: resolve.NewJSONVariableRendererWithValidation(`{"type":["boolean"]}`),
						},
					),
				},
				Fields: []*resolve.Field{
					{
						HasBuffer: true,
						BufferID:  0,
						Name:      []byte("user"),
						Position: resolve.Position{
							Line:   3,
							Column: 4,
						},
						Value: &resolve.Object{
							Path:     []string{"user"},
							Nullable: true,
							Fields: []*resolve.Field{
								{
									Name: []byte("id"),
									Value: &resolve.String{
										Path: []string{"id"},
									},
									Position: resolve.Position{
										Line:   5,
										Column: 6,
									},
									SkipDirectiveDefined: true,
									SkipVariableName:     "skip",
								},
								{
									Name: []byte("displayName"),
									Value: &resolve.String{
										Path: []string{"displayName"},
									},
									Position: resolve.Position{
										Line:   6,
										Column: 6,
									},
									SkipDirectiveDefined: true,
									SkipVariableName:     "skip",
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
				RootNodes: []plan.TypeField{
					{
						TypeName:   "Query",
						FieldNames: []string{"user"},
					},
				},
				ChildNodes: []plan.TypeField{
					{
						TypeName:   "User",
						FieldNames: []string{"id", "displayName", "isLoggedIn"},
					},
					{
						TypeName:   "RegisteredUser",
						FieldNames: []string{"id", "displayName", "isLoggedIn"},
					},
				},
				Factory: &Factory{},
				Custom: ConfigJson(Configuration{
					Fetch: FetchConfiguration{
						URL: "https://swapi.com/graphql",
					},
				}),
			},
		},
		Fields: []plan.FieldConfiguration{},
	}))
	t.Run("include directive on an inline fragment", RunTest(interfaceSelectionSchema, `
		query MyQuery ($include: Boolean!) {
			user {
				... @include(if: $include) {
					id
					displayName
				}
			}
		}
	`, "MyQuery", &plan.SynchronousResponsePlan{
		Response: &resolve.GraphQLResponse{
			Data: &resolve.Object{
				Fetch: &resolve.SingleFetch{
					DataSource:            &Source{},
					BufferId:              0,
					Input:                 `{"method":"POST","url":"https://swapi.com/graphql","body":{"query":"query($include: Boolean!){user {... @include(if: $include){id displayName}}}","variables":{"include":$$0$$}}}`,
					DataSourceIdentifier:  []byte("graphql_datasource.Source"),
					ProcessResponseConfig: resolve.ProcessResponseConfig{ExtractGraphqlResponse: true},
					Variables: resolve.NewVariables(
						&resolve.ContextVariable{
							Path:     []string{"include"},
							Renderer: resolve.NewJSONVariableRendererWithValidation(`{"type":["boolean"]}`),
						},
					),
				},
				Fields: []*resolve.Field{
					{
						HasBuffer: true,
						BufferID:  0,
						Name:      []byte("user"),
						Position: resolve.Position{
							Line:   3,
							Column: 4,
						},
						Value: &resolve.Object{
							Path:     []string{"user"},
							Nullable: true,
							Fields: []*resolve.Field{
								{
									Name: []byte("id"),
									Value: &resolve.String{
										Path: []string{"id"},
									},
									Position: resolve.Position{
										Line:   5,
										Column: 6,
									},
									IncludeDirectiveDefined: true,
									IncludeVariableName:     "include",
								},
								{
									Name: []byte("displayName"),
									Value: &resolve.String{
										Path: []string{"displayName"},
									},
									Position: resolve.Position{
										Line:   6,
										Column: 6,
									},
									IncludeDirectiveDefined: true,
									IncludeVariableName:     "include",
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
				RootNodes: []plan.TypeField{
					{
						TypeName:   "Query",
						FieldNames: []string{"user"},
					},
				},
				ChildNodes: []plan.TypeField{
					{
						TypeName:   "User",
						FieldNames: []string{"id", "displayName", "isLoggedIn"},
					},
					{
						TypeName:   "RegisteredUser",
						FieldNames: []string{"id", "displayName", "isLoggedIn"},
					},
				},
				Factory: &Factory{},
				Custom: ConfigJson(Configuration{
					Fetch: FetchConfiguration{
						URL: "https://swapi.com/graphql",
					},
				}),
			},
		},
		Fields: []plan.FieldConfiguration{},
	}))
	t.Run("skip directive with inline value true", RunTest(interfaceSelectionSchema, `
		query MyQuery {
			user {
				id
				displayName @skip(if: true)
			}
		}
	`, "MyQuery", &plan.SynchronousResponsePlan{
		Response: &resolve.GraphQLResponse{
			Data: &resolve.Object{
				Fetch: &resolve.SingleFetch{
					DataSource:            &Source{},
					BufferId:              0,
					Input:                 `{"method":"POST","url":"https://swapi.com/graphql","body":{"query":"{user {id}}"}}`,
					DataSourceIdentifier:  []byte("graphql_datasource.Source"),
					ProcessResponseConfig: resolve.ProcessResponseConfig{ExtractGraphqlResponse: true},
				},
				Fields: []*resolve.Field{
					{
						HasBuffer: true,
						BufferID:  0,
						Name:      []byte("user"),
						Position: resolve.Position{
							Line:   3,
							Column: 4,
						},
						Value: &resolve.Object{
							Path:     []string{"user"},
							Nullable: true,
							Fields: []*resolve.Field{
								{
									Name: []byte("id"),
									Value: &resolve.String{
										Path: []string{"id"},
									},
									Position: resolve.Position{
										Line:   4,
										Column: 5,
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
				RootNodes: []plan.TypeField{
					{
						TypeName:   "Query",
						FieldNames: []string{"user"},
					},
				},
				ChildNodes: []plan.TypeField{
					{
						TypeName:   "User",
						FieldNames: []string{"id", "displayName", "isLoggedIn"},
					},
					{
						TypeName:   "RegisteredUser",
						FieldNames: []string{"id", "displayName", "isLoggedIn"},
					},
				},
				Factory: &Factory{},
				Custom: ConfigJson(Configuration{
					Fetch: FetchConfiguration{
						URL: "https://swapi.com/graphql",
					},
				}),
			},
		},
		Fields: []plan.FieldConfiguration{},
	}))
	t.Run("skip directive with inline value false", RunTest(interfaceSelectionSchema, `
		query MyQuery {
			user {
				id
				displayName @skip(if: false)
			}
		}
	`, "MyQuery", &plan.SynchronousResponsePlan{
		Response: &resolve.GraphQLResponse{
			Data: &resolve.Object{
				Fetch: &resolve.SingleFetch{
					DataSource:            &Source{},
					BufferId:              0,
					Input:                 `{"method":"POST","url":"https://swapi.com/graphql","body":{"query":"{user {id displayName}}"}}`,
					DataSourceIdentifier:  []byte("graphql_datasource.Source"),
					ProcessResponseConfig: resolve.ProcessResponseConfig{ExtractGraphqlResponse: true},
				},
				Fields: []*resolve.Field{
					{
						HasBuffer: true,
						BufferID:  0,
						Name:      []byte("user"),
						Position: resolve.Position{
							Line:   3,
							Column: 4,
						},
						Value: &resolve.Object{
							Path:     []string{"user"},
							Nullable: true,
							Fields: []*resolve.Field{
								{
									Name: []byte("id"),
									Value: &resolve.String{
										Path: []string{"id"},
									},
									Position: resolve.Position{
										Line:   4,
										Column: 5,
									},
								},
								{
									Name: []byte("displayName"),
									Value: &resolve.String{
										Path: []string{"displayName"},
									},
									Position: resolve.Position{
										Line:   5,
										Column: 5,
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
				RootNodes: []plan.TypeField{
					{
						TypeName:   "Query",
						FieldNames: []string{"user"},
					},
				},
				ChildNodes: []plan.TypeField{
					{
						TypeName:   "User",
						FieldNames: []string{"id", "displayName", "isLoggedIn"},
					},
					{
						TypeName:   "RegisteredUser",
						FieldNames: []string{"id", "displayName", "isLoggedIn"},
					},
				},
				Factory: &Factory{},
				Custom: ConfigJson(Configuration{
					Fetch: FetchConfiguration{
						URL: "https://swapi.com/graphql",
					},
				}),
			},
		},
		Fields: []plan.FieldConfiguration{},
	}))
	t.Run("include directive with variable", RunTest(interfaceSelectionSchema, `
		query MyQuery ($include: Boolean!) {
			user {
				id
				displayName @include(if: $include)
			}
		}
	`, "MyQuery", &plan.SynchronousResponsePlan{
		Response: &resolve.GraphQLResponse{
			Data: &resolve.Object{
				Fetch: &resolve.SingleFetch{
					DataSource:            &Source{},
					BufferId:              0,
					Input:                 `{"method":"POST","url":"https://swapi.com/graphql","body":{"query":"query($include: Boolean!){user {id displayName @include(if: $include)}}","variables":{"include":$$0$$}}}`,
					DataSourceIdentifier:  []byte("graphql_datasource.Source"),
					ProcessResponseConfig: resolve.ProcessResponseConfig{ExtractGraphqlResponse: true},
					Variables: resolve.NewVariables(
						&resolve.ContextVariable{
							Path:     []string{"include"},
							Renderer: resolve.NewJSONVariableRendererWithValidation(`{"type":["boolean"]}`),
						},
					),
				},
				Fields: []*resolve.Field{
					{
						HasBuffer: true,
						BufferID:  0,
						Name:      []byte("user"),
						Position: resolve.Position{
							Line:   3,
							Column: 4,
						},
						Value: &resolve.Object{
							Path:     []string{"user"},
							Nullable: true,
							Fields: []*resolve.Field{
								{
									Name: []byte("id"),
									Value: &resolve.String{
										Path: []string{"id"},
									},
									Position: resolve.Position{
										Line:   4,
										Column: 5,
									},
								},
								{
									Name: []byte("displayName"),
									Value: &resolve.String{
										Path: []string{"displayName"},
									},
									Position: resolve.Position{
										Line:   5,
										Column: 5,
									},
									IncludeDirectiveDefined: true,
									IncludeVariableName:     "include",
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
				RootNodes: []plan.TypeField{
					{
						TypeName:   "Query",
						FieldNames: []string{"user"},
					},
				},
				ChildNodes: []plan.TypeField{
					{
						TypeName:   "User",
						FieldNames: []string{"id", "displayName", "isLoggedIn"},
					},
					{
						TypeName:   "RegisteredUser",
						FieldNames: []string{"id", "displayName", "isLoggedIn"},
					},
				},
				Factory: &Factory{},
				Custom: ConfigJson(Configuration{
					Fetch: FetchConfiguration{
						URL: "https://swapi.com/graphql",
					},
				}),
			},
		},
		Fields: []plan.FieldConfiguration{},
	}))
	t.Run("include directive with inline value true", RunTest(interfaceSelectionSchema, `
		query MyQuery {
			user {
				id
				displayName @include(if: true)
			}
		}
	`, "MyQuery", &plan.SynchronousResponsePlan{
		Response: &resolve.GraphQLResponse{
			Data: &resolve.Object{
				Fetch: &resolve.SingleFetch{
					DataSource:            &Source{},
					BufferId:              0,
					Input:                 `{"method":"POST","url":"https://swapi.com/graphql","body":{"query":"{user {id displayName}}"}}`,
					DataSourceIdentifier:  []byte("graphql_datasource.Source"),
					ProcessResponseConfig: resolve.ProcessResponseConfig{ExtractGraphqlResponse: true},
				},
				Fields: []*resolve.Field{
					{
						HasBuffer: true,
						BufferID:  0,
						Name:      []byte("user"),
						Position: resolve.Position{
							Line:   3,
							Column: 4,
						},
						Value: &resolve.Object{
							Path:     []string{"user"},
							Nullable: true,
							Fields: []*resolve.Field{
								{
									Name: []byte("id"),
									Value: &resolve.String{
										Path: []string{"id"},
									},
									Position: resolve.Position{
										Line:   4,
										Column: 5,
									},
								},
								{
									Name: []byte("displayName"),
									Value: &resolve.String{
										Path: []string{"displayName"},
									},
									Position: resolve.Position{
										Line:   5,
										Column: 5,
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
				RootNodes: []plan.TypeField{
					{
						TypeName:   "Query",
						FieldNames: []string{"user"},
					},
				},
				ChildNodes: []plan.TypeField{
					{
						TypeName:   "User",
						FieldNames: []string{"id", "displayName", "isLoggedIn"},
					},
					{
						TypeName:   "RegisteredUser",
						FieldNames: []string{"id", "displayName", "isLoggedIn"},
					},
				},
				Factory: &Factory{},
				Custom: ConfigJson(Configuration{
					Fetch: FetchConfiguration{
						URL: "https://swapi.com/graphql",
					},
				}),
			},
		},
		Fields: []plan.FieldConfiguration{},
	}))
	t.Run("include directive with inline value false", RunTest(interfaceSelectionSchema, `
		query MyQuery {
			user {
				id
				displayName @include(if: false)
			}
		}
	`, "MyQuery", &plan.SynchronousResponsePlan{
		Response: &resolve.GraphQLResponse{
			Data: &resolve.Object{
				Fetch: &resolve.SingleFetch{
					DataSource:            &Source{},
					BufferId:              0,
					Input:                 `{"method":"POST","url":"https://swapi.com/graphql","body":{"query":"{user {id}}"}}`,
					DataSourceIdentifier:  []byte("graphql_datasource.Source"),
					ProcessResponseConfig: resolve.ProcessResponseConfig{ExtractGraphqlResponse: true},
				},
				Fields: []*resolve.Field{
					{
						HasBuffer: true,
						BufferID:  0,
						Name:      []byte("user"),
						Position: resolve.Position{
							Line:   3,
							Column: 4,
						},
						Value: &resolve.Object{
							Path:     []string{"user"},
							Nullable: true,
							Fields: []*resolve.Field{
								{
									Name: []byte("id"),
									Value: &resolve.String{
										Path: []string{"id"},
									},
									Position: resolve.Position{
										Line:   4,
										Column: 5,
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
				RootNodes: []plan.TypeField{
					{
						TypeName:   "Query",
						FieldNames: []string{"user"},
					},
				},
				ChildNodes: []plan.TypeField{
					{
						TypeName:   "User",
						FieldNames: []string{"id", "displayName", "isLoggedIn"},
					},
					{
						TypeName:   "RegisteredUser",
						FieldNames: []string{"id", "displayName", "isLoggedIn"},
					},
				},
				Factory: &Factory{},
				Custom: ConfigJson(Configuration{
					Fetch: FetchConfiguration{
						URL: "https://swapi.com/graphql",
					},
				}),
			},
		},
		Fields: []plan.FieldConfiguration{},
	}))
	t.Run("selections on interface type with object type interface", RunTest(interfaceSelectionSchema, `
		query MyQuery {
			user {
				id
				displayName
				... on RegisteredUser {
					hasVerifiedEmail
				}
			}
		}
	`, "MyQuery", &plan.SynchronousResponsePlan{
		Response: &resolve.GraphQLResponse{
			Data: &resolve.Object{
				Fetch: &resolve.SingleFetch{
					DataSource:            &Source{},
					BufferId:              0,
					Input:                 `{"method":"POST","url":"https://swapi.com/graphql","body":{"query":"{user {id displayName __typename ... on RegisteredUser {hasVerifiedEmail}}}"}}`,
					DataSourceIdentifier:  []byte("graphql_datasource.Source"),
					ProcessResponseConfig: resolve.ProcessResponseConfig{ExtractGraphqlResponse: true},
				},
				Fields: []*resolve.Field{
					{
						HasBuffer: true,
						BufferID:  0,
						Name:      []byte("user"),
						Position: resolve.Position{
							Line:   3,
							Column: 4,
						},
						Value: &resolve.Object{
							Path:     []string{"user"},
							Nullable: true,
							Fields: []*resolve.Field{
								{
									Name: []byte("id"),
									Value: &resolve.String{
										Path: []string{"id"},
									},
									Position: resolve.Position{
										Line:   4,
										Column: 5,
									},
								},
								{
									Name: []byte("displayName"),
									Value: &resolve.String{
										Path: []string{"displayName"},
									},
									Position: resolve.Position{
										Line:   5,
										Column: 5,
									},
								},
								{
									Name: []byte("hasVerifiedEmail"),
									Value: &resolve.Boolean{
										Path: []string{"hasVerifiedEmail"},
									},
									Position: resolve.Position{
										Line:   7,
										Column: 6,
									},
									OnTypeName: []byte("RegisteredUser"),
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
				RootNodes: []plan.TypeField{
					{
						TypeName:   "Query",
						FieldNames: []string{"user"},
					},
				},
				ChildNodes: []plan.TypeField{
					{
						TypeName:   "User",
						FieldNames: []string{"id", "displayName", "isLoggedIn"},
					},
					{
						TypeName:   "RegisteredUser",
						FieldNames: []string{"id", "displayName", "isLoggedIn", "hasVerifiedEmail"},
					},
				},
				Factory: &Factory{},
				Custom: ConfigJson(Configuration{
					Fetch: FetchConfiguration{
						URL: "https://swapi.com/graphql",
					},
				}),
			},
		},
		Fields: []plan.FieldConfiguration{},
	}))
	t.Run("exported field", RunTest(starWarsSchemaWithExportDirective, `
		query MyQuery($id: ID! $heroName: String!){
			droid(id: $id){
				name
				aliased: name
				friends {
					name
				}
				primaryFunction
			}
			hero {
				name @export(as: "heroName")
			}
			search(name: $heroName) {
				... on Droid {
					primaryFunction
				}
			}
			stringList
			nestedStringList
		}
	`, "MyQuery", &plan.SynchronousResponsePlan{
		Response: &resolve.GraphQLResponse{
			Data: &resolve.Object{
				Fetch: &resolve.SingleFetch{
					DataSource: &Source{},
					BufferId:   0,
					Input:      `{"method":"POST","url":"https://swapi.com/graphql","header":{"Authorization":["$$2$$"],"Invalid-Template":["{{ request.headers.Authorization }}"]},"body":{"query":"query($id: ID!, $heroName: String!){droid(id: $id){name aliased: name friends {name} primaryFunction} hero {name} search(name: $heroName){__typename ... on Droid {primaryFunction}} stringList nestedStringList}","variables":{"heroName":$$1$$,"id":$$0$$}}}`,
					Variables: resolve.NewVariables(
						&resolve.ContextVariable{
							Path:     []string{"id"},
							Renderer: resolve.NewJSONVariableRendererWithValidation(`{"type":["string","integer"]}`),
						},
						&resolve.ContextVariable{
							Path:     []string{"heroName"},
							Renderer: resolve.NewJSONVariableRendererWithValidation(`{"type":["string"]}`),
						},
						&resolve.HeaderVariable{
							Path: []string{"Authorization"},
						},
					),
					DataSourceIdentifier:  []byte("graphql_datasource.Source"),
					ProcessResponseConfig: resolve.ProcessResponseConfig{ExtractGraphqlResponse: true},
				},
				Fields: []*resolve.Field{
					{
						HasBuffer: true,
						BufferID:  0,
						Name:      []byte("droid"),
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
										Path: []string{"aliased"},
									},
									Position: resolve.Position{
										Line:   5,
										Column: 5,
									},
								},
								{
									Name: []byte("friends"),
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
									Name: []byte("primaryFunction"),
									Value: &resolve.String{
										Path: []string{"primaryFunction"},
									},
									Position: resolve.Position{
										Line:   9,
										Column: 5,
									},
								},
							},
						},
					},
					{
						HasBuffer: true,
						BufferID:  0,
						Name:      []byte("hero"),
						Position: resolve.Position{
							Line:   11,
							Column: 4,
						},
						Value: &resolve.Object{
							Path:     []string{"hero"},
							Nullable: true,
							Fields: []*resolve.Field{
								{
									Name: []byte("name"),
									Value: &resolve.String{
										Path: []string{"name"},
										Export: &resolve.FieldExport{
											Path:     []string{"heroName"},
											AsString: true,
										},
									},
									Position: resolve.Position{
										Line:   12,
										Column: 5,
									},
								},
							},
						},
					},
					{
						HasBuffer: true,
						BufferID:  0,
						Name:      []byte("search"),
						Position: resolve.Position{
							Line:   14,
							Column: 4,
						},
						Value: &resolve.Object{
							Nullable: true,
							Path:     []string{"search"},
							Fields: []*resolve.Field{
								{
									Name: []byte("primaryFunction"),
									Value: &resolve.String{
										Path: []string{"primaryFunction"},
									},
									Position: resolve.Position{
										Line:   16,
										Column: 6,
									},
									OnTypeName: []byte("Droid"),
								},
							},
						},
					},
					{
						HasBuffer: true,
						BufferID:  0,
						Name:      []byte("stringList"),
						Position: resolve.Position{
							Line:   19,
							Column: 4,
						},
						Value: &resolve.Array{
							Nullable: true,
							Item: &resolve.String{
								Nullable: true,
							},
						},
					},
					{
						HasBuffer: true,
						BufferID:  0,
						Name:      []byte("nestedStringList"),
						Position: resolve.Position{
							Line:   20,
							Column: 4,
						},
						Value: &resolve.Array{
							Nullable: true,
							Path:     []string{"nestedStringList"},
							Item: &resolve.String{
								Nullable: true,
							},
						},
					},
				},
			},
		},
	}, plan.Configuration{
		DataSources: []plan.DataSourceConfiguration{
			{
				RootNodes: []plan.TypeField{
					{
						TypeName:   "Query",
						FieldNames: []string{"droid", "hero", "stringList", "nestedStringList", "search"},
					},
				},
				ChildNodes: []plan.TypeField{
					{
						TypeName:   "Character",
						FieldNames: []string{"name", "friends"},
					},
					{
						TypeName:   "Human",
						FieldNames: []string{"name", "height", "friends"},
					},
					{
						TypeName:   "Droid",
						FieldNames: []string{"name", "primaryFunction", "friends"},
					},
				},
				Factory: &Factory{},
				Custom: ConfigJson(Configuration{
					Fetch: FetchConfiguration{
						URL: "https://swapi.com/graphql",
						Header: http.Header{
							"Authorization":    []string{"{{ .request.headers.Authorization }}"},
							"Invalid-Template": []string{"{{ request.headers.Authorization }}"},
						},
					},
					UpstreamSchema: starWarsSchema,
				}),
			},
		},
		Fields: []plan.FieldConfiguration{
			{
				TypeName:  "Query",
				FieldName: "droid",
				Arguments: []plan.ArgumentConfiguration{
					{
						Name:       "id",
						SourceType: plan.FieldArgumentSource,
					},
				},
			},
			{
				TypeName:              "Query",
				FieldName:             "stringList",
				DisableDefaultMapping: true,
			},
			{
				TypeName:  "Query",
				FieldName: "nestedStringList",
				Path:      []string{"nestedStringList"},
			},
			{
				TypeName:  "Query",
				FieldName: "search",
				Path:      []string{"search"},
				Arguments: []plan.ArgumentConfiguration{
					{
						Name:       "name",
						SourceType: plan.FieldArgumentSource,
					},
				},
			},
		},
	}))
	t.Run("Query with renamed root fields", RunTest(renamedStarWarsSchema, `
		query MyQuery($id: ID! $input: SearchInput_api! @api_onVariable $options: JSON_api) @otherapi_undefined @api_onOperation {
			api_droid(id: $id){
				name @api_format
				aliased: name
				friends {
					name
				}
				primaryFunction
			}
			api_hero {
				name
				... on Human_api {
					height
				}
			}
			api_stringList
			renamed: api_nestedStringList
			api_search(name: "r2d2") {
				... on Droid_api {
					primaryFunction
				}
			}
			api_searchWithInput(input: $input) {
				... on Droid_api {
					primaryFunction
				}
			}
			withOptions: api_searchWithInput(input: {
				options: $options
			}) {
				... on Droid_api {
					primaryFunction
				}
			}
		}
	`, "MyQuery", &plan.SynchronousResponsePlan{
		Response: &resolve.GraphQLResponse{
			Data: &resolve.Object{
				Fetch: &resolve.SingleFetch{
					DataSource: &Source{},
					BufferId:   0,
					Input:      `{"method":"POST","url":"https://swapi.com/graphql","header":{"Authorization":["$$3$$"],"Invalid-Template":["{{ request.headers.Authorization }}"]},"body":{"query":"query($id: ID!, $input: SearchInput! @onVariable, $options: JSON)@onOperation {api_droid: droid(id: $id){name @format aliased: name friends {name} primaryFunction} api_hero: hero {name __typename ... on Human {height}} api_stringList: stringList renamed: nestedStringList api_search: search {__typename ... on Droid {primaryFunction}} api_searchWithInput: searchWithInput(input: $input){__typename ... on Droid {primaryFunction}} withOptions: searchWithInput(input: {options: $options}){__typename ... on Droid {primaryFunction}}}","variables":{"options":$$2$$,"input":$$1$$,"id":$$0$$}}}`,
					Variables: resolve.NewVariables(
						&resolve.ContextVariable{
							Path:     []string{"id"},
							Renderer: resolve.NewJSONVariableRendererWithValidation(`{"type":["string","integer"]}`),
						},
						&resolve.ContextVariable{
							Path:     []string{"input"},
							Renderer: resolve.NewJSONVariableRendererWithValidation(`{"type":["object"],"properties":{"name":{"type":["string","null"]},"options":{}},"additionalProperties":false}`),
						},
						&resolve.ContextVariable{
							Path:     []string{"options"},
							Renderer: resolve.NewJSONVariableRendererWithValidation(`{}`),
						},
						&resolve.HeaderVariable{
							Path: []string{"Authorization"},
						},
					),
					DataSourceIdentifier:  []byte("graphql_datasource.Source"),
					ProcessResponseConfig: resolve.ProcessResponseConfig{ExtractGraphqlResponse: true},
				},
				Fields: []*resolve.Field{
					{
						HasBuffer: true,
						BufferID:  0,
						Name:      []byte("api_droid"),
						Position: resolve.Position{
							Line:   3,
							Column: 4,
						},
						Value: &resolve.Object{
							Path:     []string{"api_droid"},
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
										Path: []string{"aliased"},
									},
									Position: resolve.Position{
										Line:   5,
										Column: 5,
									},
								},
								{
									Name: []byte("friends"),
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
									Name: []byte("primaryFunction"),
									Value: &resolve.String{
										Path: []string{"primaryFunction"},
									},
									Position: resolve.Position{
										Line:   9,
										Column: 5,
									},
								},
							},
						},
					},
					{
						HasBuffer: true,
						BufferID:  0,
						Name:      []byte("api_hero"),
						Position: resolve.Position{
							Line:   11,
							Column: 4,
						},
						Value: &resolve.Object{
							Path:     []string{"api_hero"},
							Nullable: true,
							Fields: []*resolve.Field{
								{
									Name: []byte("name"),
									Value: &resolve.String{
										Path: []string{"name"},
									},
									Position: resolve.Position{
										Line:   12,
										Column: 5,
									},
								},
								{
									Name: []byte("height"),
									Value: &resolve.String{
										Path: []string{"height"},
									},
									Position: resolve.Position{
										Line:   14,
										Column: 6,
									},
									OnTypeName: []byte("Human"),
								},
							},
						},
					},
					{
						HasBuffer: true,
						BufferID:  0,
						Name:      []byte("api_stringList"),
						Position: resolve.Position{
							Line:   17,
							Column: 4,
						},
						Value: &resolve.Array{
							Nullable: true,
							Path:     []string{"api_stringList"},
							Item: &resolve.String{
								Nullable: true,
							},
						},
					},
					{
						HasBuffer: true,
						BufferID:  0,
						Name:      []byte("renamed"),
						Position: resolve.Position{
							Line:   18,
							Column: 4,
						},
						Value: &resolve.Array{
							Nullable: true,
							Path:     []string{"renamed"},
							Item: &resolve.String{
								Nullable: true,
							},
						},
					},
					{
						HasBuffer: true,
						BufferID:  0,
						Name:      []byte("api_search"),
						Position: resolve.Position{
							Line:   19,
							Column: 4,
						},
						Value: &resolve.Object{
							Nullable: true,
							Path:     []string{"api_search"},
							Fields: []*resolve.Field{
								{
									Name: []byte("primaryFunction"),
									Value: &resolve.String{
										Path: []string{"primaryFunction"},
									},
									Position: resolve.Position{
										Line:   21,
										Column: 6,
									},
									OnTypeName: []byte("Droid"),
								},
							},
						},
					},
					{
						HasBuffer: true,
						BufferID:  0,
						Name:      []byte("api_searchWithInput"),
						Position: resolve.Position{
							Line:   24,
							Column: 4,
						},
						Value: &resolve.Object{
							Nullable: true,
							Path:     []string{"api_searchWithInput"},
							Fields: []*resolve.Field{
								{
									Name: []byte("primaryFunction"),
									Value: &resolve.String{
										Path: []string{"primaryFunction"},
									},
									Position: resolve.Position{
										Line:   26,
										Column: 6,
									},
									OnTypeName: []byte("Droid"),
								},
							},
						},
					},
					{
						HasBuffer: true,
						BufferID:  0,
						Name:      []byte("withOptions"),
						Position: resolve.Position{
							Line:   29,
							Column: 4,
						},
						Value: &resolve.Object{
							Nullable: true,
							Path:     []string{"withOptions"},
							Fields: []*resolve.Field{
								{
									Name: []byte("primaryFunction"),
									Value: &resolve.String{
										Path: []string{"primaryFunction"},
									},
									Position: resolve.Position{
										Line:   33,
										Column: 6,
									},
									OnTypeName: []byte("Droid"),
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
				RootNodes: []plan.TypeField{
					{
						TypeName:   "Query",
						FieldNames: []string{"api_droid", "api_hero", "api_stringList", "api_nestedStringList", "api_search", "api_searchWithInput"},
					},
				},
				ChildNodes: []plan.TypeField{
					{
						TypeName:   "Character_api",
						FieldNames: []string{"name", "friends"},
					},
					{
						TypeName:   "Human_api",
						FieldNames: []string{"name", "height", "friends"},
					},
					{
						TypeName:   "Droid_api",
						FieldNames: []string{"name", "primaryFunction", "friends"},
					},
					{
						TypeName:   "SearchResult_api",
						FieldNames: []string{"name", "height", "primaryFunction", "friends"},
					},
				},
				Directives: []plan.DirectiveConfiguration{
					{
						DirectiveName: "api_format",
						RenameTo:      "format",
					},
					{
						DirectiveName: "api_onOperation",
						RenameTo:      "onOperation",
					},
					{
						DirectiveName: "api_onVariable",
						RenameTo:      "onVariable",
					},
				},
				Factory: &Factory{},
				Custom: ConfigJson(Configuration{
					Fetch: FetchConfiguration{
						URL: "https://swapi.com/graphql",
						Header: http.Header{
							"Authorization":    []string{"{{ .request.headers.Authorization }}"},
							"Invalid-Template": []string{"{{ request.headers.Authorization }}"},
						},
					},
					UpstreamSchema: starWarsSchema,
				}),
			},
		},
		Fields: []plan.FieldConfiguration{
			{
				TypeName:  "Query",
				FieldName: "api_droid",
				Arguments: []plan.ArgumentConfiguration{
					{
						Name:       "id",
						SourceType: plan.FieldArgumentSource,
					},
				},
				Path: []string{"droid"},
			},
			{
				TypeName:  "Query",
				FieldName: "api_hero",
				Path:      []string{"hero"},
			},
			{
				TypeName:  "Query",
				FieldName: "api_stringList",
				Path:      []string{"stringList"},
			},
			{
				TypeName:  "Query",
				FieldName: "api_nestedStringList",
				Path:      []string{"nestedStringList"},
			},
			{
				TypeName:  "Query",
				FieldName: "api_search",
				Path:      []string{"search"},
			},
			{
				TypeName:  "Query",
				FieldName: "api_searchWithInput",
				Path:      []string{"searchWithInput"},
				Arguments: []plan.ArgumentConfiguration{
					{
						Name:       "input",
						SourceType: plan.FieldArgumentSource,
					},
				},
			},
		},
		Types: []plan.TypeConfiguration{
			{
				TypeName: "Human_api",
				RenameTo: "Human",
			},
			{
				TypeName: "Droid_api",
				RenameTo: "Droid",
			},
			{
				TypeName: "SearchResult_api",
				RenameTo: "SearchResult",
			},
			{
				TypeName: "SearchInput_api",
				RenameTo: "SearchInput",
			},
			{
				TypeName: "JSON_api",
				RenameTo: "JSON",
			},
		},
	}))
	t.Run("Query with array input", RunTest(subgraphTestSchema, `
		query($representations: [_Any!]!) {
			_entities(representations: $representations){
				... on Product {
					reviews {
						body 
						author {
							username 
							id
						}
					}
				}
			}
		}
	`, "", &plan.SynchronousResponsePlan{
		Response: &resolve.GraphQLResponse{
			Data: &resolve.Object{
				Fetch: &resolve.SingleFetch{
					DataSource: &Source{},
					BufferId:   0,
					Input:      `{"method":"POST","url":"https://subgraph-reviews/query","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){__typename ... on Product {reviews {body author {username id}}}}}","variables":{"representations":$$0$$}}}`,
					Variables: resolve.NewVariables(
						&resolve.ContextVariable{
							Path:     []string{"representations"},
							Renderer: resolve.NewJSONVariableRendererWithValidation(`{"type":["array"],"items":{"type":["object"],"additionalProperties":true}}`),
						},
					),
					DataSourceIdentifier:  []byte("graphql_datasource.Source"),
					ProcessResponseConfig: resolve.ProcessResponseConfig{ExtractGraphqlResponse: true},
				},
				Fields: []*resolve.Field{
					{
						HasBuffer: true,
						BufferID:  0,
						Name:      []byte("_entities"),
						Position: resolve.Position{
							Line:   3,
							Column: 4,
						},
						Value: &resolve.Array{
							Path:     []string{"_entities"},
							Nullable: false,
							Item: &resolve.Object{
								Nullable: true,
								Path:     nil,
								Fields: []*resolve.Field{
									{
										Name: []byte("reviews"),
										Value: &resolve.Array{
											Path:                []string{"reviews"},
											Nullable:            true,
											ResolveAsynchronous: false,
											Item: &resolve.Object{
												Nullable: true,
												Path:     nil,
												Fields: []*resolve.Field{
													{
														Name: []byte("body"),
														Value: &resolve.String{
															Path:     []string{"body"},
															Nullable: false,
														},
														Position: resolve.Position{
															Line:   6,
															Column: 7,
														},
													},
													{
														Name: []byte("author"),
														Value: &resolve.Object{
															Nullable: false,
															Path:     []string{"author"},
															Fields: []*resolve.Field{
																{
																	Name: []byte("username"),
																	Value: &resolve.String{
																		Path:     []string{"username"},
																		Nullable: false,
																	},
																	Position: resolve.Position{
																		Line:   8,
																		Column: 8,
																	},
																},
																{
																	Name: []byte("id"),
																	Value: &resolve.String{
																		Path:     []string{"id"},
																		Nullable: false,
																	},
																	Position: resolve.Position{
																		Line:   9,
																		Column: 8,
																	},
																},
															},
														},
														Position: resolve.Position{
															Line:   7,
															Column: 7,
														},
													},
												},
											},
										},
										Position: resolve.Position{
											Line:   5,
											Column: 6,
										},
										OnTypeName: []byte("Product"),
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
				RootNodes: []plan.TypeField{
					{
						TypeName:   "Query",
						FieldNames: []string{"_entities", "_service"},
					},
				},
				ChildNodes: []plan.TypeField{
					{
						TypeName:   "_Service",
						FieldNames: []string{"sdl"},
					},
					{
						TypeName:   "Entity",
						FieldNames: []string{"findProductByUpc", "findUserByID"},
					},
					{
						TypeName:   "Product",
						FieldNames: []string{"upc", "reviews"},
					},
					{
						TypeName:   "Review",
						FieldNames: []string{"body", "author", "product"},
					},
					{
						TypeName:   "User",
						FieldNames: []string{"id", "username", "reviews"},
					},
				},
				Factory: &Factory{},
				Custom: ConfigJson(Configuration{
					Fetch: FetchConfiguration{
						URL: "https://subgraph-reviews/query",
					},
				}),
			},
		},
		Fields: []plan.FieldConfiguration{
			{
				TypeName:  "Query",
				FieldName: "_entities",
				Arguments: []plan.ArgumentConfiguration{
					{
						Name:       "representations",
						SourceType: plan.FieldArgumentSource,
					},
				},
			},
			{
				TypeName:  "Entity",
				FieldName: "findProductByUpc",
				Arguments: []plan.ArgumentConfiguration{
					{
						Name:       "upc",
						SourceType: plan.FieldArgumentSource,
					},
				},
			},
			{
				TypeName:  "Entity",
				FieldName: "findUserByID",
				Arguments: []plan.ArgumentConfiguration{
					{
						Name:       "id",
						SourceType: plan.FieldArgumentSource,
					},
				},
			},
		},
	}))

	t.Run("Query with ID array input", runTestOnTestDefinition(`
		query Droids($droidIDs: [ID!]!) {
			droids(ids: $droidIDs) {
				name
				primaryFunction
			}
		}`, "Droids",
		&plan.SynchronousResponsePlan{
			Response: &resolve.GraphQLResponse{
				Data: &resolve.Object{
					Fetch: &resolve.SingleFetch{
						BufferId:   0,
						Input:      `{"method":"POST","url":"https://swapi.com/graphql","body":{"query":"query($droidIDs: [ID!]!){droids(ids: $droidIDs){name primaryFunction}}","variables":{"droidIDs":$$0$$}}}`,
						DataSource: &Source{},
						Variables: resolve.NewVariables(
							&resolve.ContextVariable{
								Path:     []string{"droidIDs"},
								Renderer: resolve.NewJSONVariableRendererWithValidation(`{"type":["array"],"items":{"type":["string","integer"]}}`),
							},
						),
						DataSourceIdentifier:  []byte("graphql_datasource.Source"),
						ProcessResponseConfig: resolve.ProcessResponseConfig{ExtractGraphqlResponse: true},
					},
					Fields: []*resolve.Field{
						{
							Name: []byte("droids"),
							Value: &resolve.Array{
								Path:     []string{"droids"},
								Nullable: true,
								Item: &resolve.Object{
									Nullable: true,
									Path:     nil,
									Fields: []*resolve.Field{
										{
											Name: []byte("name"),
											Value: &resolve.String{
												Path:     []string{"name"},
												Nullable: false,
											},
											Position: resolve.Position{
												Line:   4,
												Column: 5,
											},
										},
										{
											Name: []byte("primaryFunction"),
											Value: &resolve.String{
												Path:     []string{"primaryFunction"},
												Nullable: false,
											},
											Position: resolve.Position{
												Line:   5,
												Column: 5,
											},
										},
									},
								},
								Stream: resolve.Stream{},
							},
							Position: resolve.Position{
								Line:   3,
								Column: 4,
							},
							HasBuffer: true,
							BufferID:  0,
						},
					},
				},
			},
		}))

	t.Run("Query with ID input", runTestOnTestDefinition(`
		query Droid($droidID: ID!) {
			droid(id: $droidID) {
				name
				primaryFunction
			}
		}`, "Droid",
		&plan.SynchronousResponsePlan{
			Response: &resolve.GraphQLResponse{
				Data: &resolve.Object{
					Fetch: &resolve.SingleFetch{
						BufferId:   0,
						Input:      `{"method":"POST","url":"https://swapi.com/graphql","body":{"query":"query($droidID: ID!){droid(id: $droidID){name primaryFunction}}","variables":{"droidID":$$0$$}}}`,
						DataSource: &Source{},
						Variables: resolve.NewVariables(
							&resolve.ContextVariable{
								Path:     []string{"droidID"},
								Renderer: resolve.NewJSONVariableRendererWithValidation(`{"type":["string","integer"]}`),
							},
						),
						DataSourceIdentifier:  []byte("graphql_datasource.Source"),
						ProcessResponseConfig: resolve.ProcessResponseConfig{ExtractGraphqlResponse: true},
					},
					Fields: []*resolve.Field{
						{
							Name: []byte("droid"),
							Value: &resolve.Object{
								Nullable: true,
								Path:     []string{"droid"},
								Fields: []*resolve.Field{
									{
										Name: []byte("name"),
										Value: &resolve.String{
											Path:     []string{"name"},
											Nullable: false,
										},
										Position: resolve.Position{
											Line:   4,
											Column: 5,
										},
									},
									{
										Name: []byte("primaryFunction"),
										Value: &resolve.String{
											Path:     []string{"primaryFunction"},
											Nullable: false,
										},
										Position: resolve.Position{
											Line:   5,
											Column: 5,
										},
									},
								},
							},
							Position: resolve.Position{
								Line:   3,
								Column: 4,
							},
							HasBuffer: true,
							BufferID:  0,
						},
					},
				},
			},
		}))

	t.Run("Query with Date input aka scalar", runTestOnTestDefinition(`
		query HeroByBirthdate($birthdate: Date!) {
			heroByBirthdate(birthdate: $birthdate) {
				name
			}
		}`, "HeroByBirthdate",
		&plan.SynchronousResponsePlan{
			Response: &resolve.GraphQLResponse{
				Data: &resolve.Object{
					Fetch: &resolve.SingleFetch{
						BufferId:   0,
						Input:      `{"method":"POST","url":"https://swapi.com/graphql","body":{"query":"query($birthdate: Date!){heroByBirthdate(birthdate: $birthdate){name}}","variables":{"birthdate":$$0$$}}}`,
						DataSource: &Source{},
						Variables: resolve.NewVariables(
							&resolve.ContextVariable{
								Path:     []string{"birthdate"},
								Renderer: resolve.NewJSONVariableRendererWithValidation(`{}`),
							},
						),
						DataSourceIdentifier:  []byte("graphql_datasource.Source"),
						ProcessResponseConfig: resolve.ProcessResponseConfig{ExtractGraphqlResponse: true},
					},
					Fields: []*resolve.Field{
						{
							Name: []byte("heroByBirthdate"),
							Value: &resolve.Object{
								Nullable: true,
								Path:     []string{"heroByBirthdate"},
								Fields: []*resolve.Field{
									{
										Name: []byte("name"),
										Value: &resolve.String{
											Path:     []string{"name"},
											Nullable: false,
										},
										Position: resolve.Position{
											Line:   4,
											Column: 5,
										},
									},
								},
							},
							Position: resolve.Position{
								Line:   3,
								Column: 4,
							},
							HasBuffer: true,
							BufferID:  0,
						},
					},
				},
			},
		}))

	t.Run("simple mutation", RunTest(`
		type Mutation {
			addFriend(name: String!):Friend!
		}
		type Friend {
			id: ID!
			name: String!
		}
	`,
		`mutation AddFriend($name: String!){ addFriend(name: $name){ id name } }`,
		"AddFriend",
		&plan.SynchronousResponsePlan{
			Response: &resolve.GraphQLResponse{
				Data: &resolve.Object{
					Fetch: &resolve.SingleFetch{
						BufferId:   0,
						Input:      `{"method":"POST","url":"https://service.one","body":{"query":"mutation($name: String!){addFriend(name: $name){id name}}","variables":{"name":$$0$$}}}`,
						DataSource: &Source{},
						Variables: resolve.NewVariables(
							&resolve.ContextVariable{
								Path:     []string{"name"},
								Renderer: resolve.NewJSONVariableRendererWithValidation(`{"type":["string"]}`),
							},
						),
						DisallowSingleFlight:  true,
						DataSourceIdentifier:  []byte("graphql_datasource.Source"),
						ProcessResponseConfig: resolve.ProcessResponseConfig{ExtractGraphqlResponse: true},
					},
					Fields: []*resolve.Field{
						{
							BufferID:  0,
							HasBuffer: true,
							Name:      []byte("addFriend"),
							Position: resolve.Position{
								Line:   1,
								Column: 37,
							},
							Value: &resolve.Object{
								Fields: []*resolve.Field{
									{
										Name: []byte("id"),
										Value: &resolve.String{
											Path: []string{"id"},
										},
										Position: resolve.Position{
											Line:   1,
											Column: 61,
										},
									},
									{
										Name: []byte("name"),
										Value: &resolve.String{
											Path: []string{"name"},
										},
										Position: resolve.Position{
											Line:   1,
											Column: 64,
										},
									},
								},
							},
						},
					},
				},
			},
		},
		plan.Configuration{
			DataSources: []plan.DataSourceConfiguration{
				{
					RootNodes: []plan.TypeField{
						{
							TypeName:   "Mutation",
							FieldNames: []string{"addFriend"},
						},
					},
					ChildNodes: []plan.TypeField{
						{
							TypeName:   "Friend",
							FieldNames: []string{"id", "name"},
						},
					},
					Custom: ConfigJson(Configuration{
						Fetch: FetchConfiguration{
							URL: "https://service.one",
						},
					}),
					Factory: &Factory{},
				},
			},
			Fields: []plan.FieldConfiguration{
				{
					TypeName:              "Mutation",
					FieldName:             "addFriend",
					DisableDefaultMapping: true,
					Arguments: []plan.ArgumentConfiguration{
						{
							Name:       "name",
							SourceType: plan.FieldArgumentSource,
						},
					},
				},
			},
		},
	))

	t.Run("nested resolvers of same upstream", RunTest(`
		type Query {
			foo(bar: String):Baz
		}
		type Baz {
			bar(bal: String):String
		}
		`,
		`
		query NestedQuery {
			foo(bar: "baz") {
				bar(bal: "bat")
			}
		}
		`,
		"NestedQuery",
		&plan.SynchronousResponsePlan{
			Response: &resolve.GraphQLResponse{
				Data: &resolve.Object{
					Fetch: &resolve.SingleFetch{
						BufferId:   0,
						Input:      `{"method":"POST","url":"https://foo.service","body":{"query":"query($a: String, $b: String){foo(bar: $a){bar(bal: $b)}}","variables":{"b":$$1$$,"a":$$0$$}}}`,
						DataSource: &Source{},
						Variables: resolve.NewVariables(
							&resolve.ContextVariable{
								Path:     []string{"a"},
								Renderer: resolve.NewJSONVariableRendererWithValidation(`{"type":["string","null"]}`),
							},
							&resolve.ContextVariable{
								Path:     []string{"b"},
								Renderer: resolve.NewJSONVariableRendererWithValidation(`{"type":["string","null"]}`),
							},
						),
						DisallowSingleFlight:  false,
						DataSourceIdentifier:  []byte("graphql_datasource.Source"),
						ProcessResponseConfig: resolve.ProcessResponseConfig{ExtractGraphqlResponse: true},
					},
					Fields: []*resolve.Field{
						{
							BufferID:  0,
							HasBuffer: true,
							Name:      []byte("foo"),
							Position: resolve.Position{
								Line:   3,
								Column: 4,
							},
							Value: &resolve.Object{
								Nullable: true,
								Path:     []string{"foo"},
								Fields: []*resolve.Field{
									{
										Name: []byte("bar"),
										Value: &resolve.String{
											Nullable: true,
											Path:     []string{"bar"},
										},
										Position: resolve.Position{
											Line:   4,
											Column: 5,
										},
									},
								},
							},
						},
					},
				},
			},
		},
		plan.Configuration{
			DataSources: []plan.DataSourceConfiguration{
				{
					RootNodes: []plan.TypeField{
						{
							TypeName:   "Query",
							FieldNames: []string{"foo"},
						},
					},
					ChildNodes: []plan.TypeField{
						{
							TypeName:   "Baz",
							FieldNames: []string{"bar"},
						},
					},
					Factory: &Factory{},
					Custom: ConfigJson(Configuration{
						Fetch: FetchConfiguration{
							URL: "https://foo.service",
						},
					}),
				},
			},
			Fields: []plan.FieldConfiguration{
				{
					TypeName:  "Query",
					FieldName: "foo",
					Arguments: []plan.ArgumentConfiguration{
						{
							Name:       "bar",
							SourceType: plan.FieldArgumentSource,
						},
					},
				},
				{
					TypeName:  "Baz",
					FieldName: "bar",
					Arguments: []plan.ArgumentConfiguration{
						{
							Name:       "bal",
							SourceType: plan.FieldArgumentSource,
						},
					},
				},
			},
		},
	))

	t.Run("same upstream with alias in query", RunTest(
		countriesSchema,
		`
		query QueryWithAlias {
			country(code: "AD") {
				name
			}
			alias: country(code: "AE") {
				name
            }
		}
		`,
		"QueryWithAlias",
		&plan.SynchronousResponsePlan{
			Response: &resolve.GraphQLResponse{
				Data: &resolve.Object{
					Fetch: &resolve.SingleFetch{
						BufferId:   0,
						Input:      `{"method":"POST","url":"https://countries.service","body":{"query":"query($a: ID!, $b: ID!){country(code: $a){name} alias: country(code: $b){name}}","variables":{"b":$$1$$,"a":$$0$$}}}`,
						DataSource: &Source{},
						Variables: resolve.NewVariables(
							&resolve.ContextVariable{
								Path:     []string{"a"},
								Renderer: resolve.NewJSONVariableRendererWithValidation(`{"type":["string","integer"]}`),
							},
							&resolve.ContextVariable{
								Path:     []string{"b"},
								Renderer: resolve.NewJSONVariableRendererWithValidation(`{"type":["string","integer"]}`),
							},
						),
						DisallowSingleFlight:  false,
						DataSourceIdentifier:  []byte("graphql_datasource.Source"),
						ProcessResponseConfig: resolve.ProcessResponseConfig{ExtractGraphqlResponse: true},
					},
					Fields: []*resolve.Field{
						{
							BufferID:  0,
							HasBuffer: true,
							Name:      []byte("country"),
							Position: resolve.Position{
								Line:   3,
								Column: 4,
							},
							Value: &resolve.Object{
								Nullable: true,
								Path:     []string{"country"},
								Fields: []*resolve.Field{
									{
										Name: []byte("name"),
										Value: &resolve.String{
											Nullable: false,
											Path:     []string{"name"},
										},
										Position: resolve.Position{
											Line:   4,
											Column: 5,
										},
									},
								},
							},
						},
						{
							BufferID:  0,
							HasBuffer: true,
							Name:      []byte("alias"),
							Position: resolve.Position{
								Line:   6,
								Column: 4,
							},
							Value: &resolve.Object{
								Nullable: true,
								Path:     []string{"alias"},
								Fields: []*resolve.Field{
									{
										Name: []byte("name"),
										Value: &resolve.String{
											Nullable: false,
											Path:     []string{"name"},
										},
										Position: resolve.Position{
											Line:   7,
											Column: 5,
										},
									},
								},
							},
						},
					},
				},
			},
		},
		plan.Configuration{
			DataSources: []plan.DataSourceConfiguration{
				{
					RootNodes: []plan.TypeField{
						{
							TypeName:   "Query",
							FieldNames: []string{"country", "countryAlias"},
						},
					},
					ChildNodes: []plan.TypeField{
						{
							TypeName:   "Country",
							FieldNames: []string{"name", "code"},
						},
					},
					Factory: &Factory{},
					Custom: ConfigJson(Configuration{
						Fetch: FetchConfiguration{
							URL: "https://countries.service",
						},
					}),
				},
			},
			Fields: []plan.FieldConfiguration{
				{
					TypeName:  "Query",
					FieldName: "country",
					Path:      []string{"country"},
					Arguments: []plan.ArgumentConfiguration{
						{
							Name:       "code",
							SourceType: plan.FieldArgumentSource,
						},
					},
				},
				{
					TypeName:  "Query",
					FieldName: "countryAlias",
					Path:      []string{"country"},
					Arguments: []plan.ArgumentConfiguration{
						{
							Name:       "code",
							SourceType: plan.FieldArgumentSource,
						},
					},
				},
			},
		},
	))

	t.Run("same upstream with alias in schema", RunTest(
		countriesSchema,
		`
		query QueryWithSchemaAlias {
			country(code: "AD") {
				name
			}
			countryAlias(code: "AE") {
				name
            }
		}
		`,
		"QueryWithSchemaAlias",
		&plan.SynchronousResponsePlan{
			Response: &resolve.GraphQLResponse{
				Data: &resolve.Object{
					Fetch: &resolve.SingleFetch{
						BufferId:   0,
						Input:      `{"method":"POST","url":"https://countries.service","body":{"query":"query($a: ID!, $b: ID!){country(code: $a){name} countryAlias: country(code: $b){name}}","variables":{"b":$$1$$,"a":$$0$$}}}`,
						DataSource: &Source{},
						Variables: resolve.NewVariables(
							&resolve.ContextVariable{
								Path:     []string{"a"},
								Renderer: resolve.NewJSONVariableRendererWithValidation(`{"type":["string","integer"]}`),
							},
							&resolve.ContextVariable{
								Path:     []string{"b"},
								Renderer: resolve.NewJSONVariableRendererWithValidation(`{"type":["string","integer"]}`),
							},
						),
						DisallowSingleFlight:  false,
						DataSourceIdentifier:  []byte("graphql_datasource.Source"),
						ProcessResponseConfig: resolve.ProcessResponseConfig{ExtractGraphqlResponse: true},
					},
					Fields: []*resolve.Field{
						{
							BufferID:  0,
							HasBuffer: true,
							Name:      []byte("country"),
							Position: resolve.Position{
								Line:   3,
								Column: 4,
							},
							Value: &resolve.Object{
								Nullable: true,
								Path:     []string{"country"},
								Fields: []*resolve.Field{
									{
										Name: []byte("name"),
										Value: &resolve.String{
											Nullable: false,
											Path:     []string{"name"},
										},
										Position: resolve.Position{
											Line:   4,
											Column: 5,
										},
									},
								},
							},
						},
						{
							BufferID:  0,
							HasBuffer: true,
							Name:      []byte("countryAlias"),
							Position: resolve.Position{
								Line:   6,
								Column: 4,
							},
							Value: &resolve.Object{
								Nullable: true,
								Path:     []string{"countryAlias"},
								Fields: []*resolve.Field{
									{
										Name: []byte("name"),
										Value: &resolve.String{
											Nullable: false,
											Path:     []string{"name"},
										},
										Position: resolve.Position{
											Line:   7,
											Column: 5,
										},
									},
								},
							},
						},
					},
				},
			},
		},
		plan.Configuration{
			DataSources: []plan.DataSourceConfiguration{
				{
					RootNodes: []plan.TypeField{
						{
							TypeName:   "Query",
							FieldNames: []string{"country", "countryAlias"},
						},
					},
					ChildNodes: []plan.TypeField{
						{
							TypeName:   "Country",
							FieldNames: []string{"name", "code"},
						},
					},
					Factory: &Factory{},
					Custom: ConfigJson(Configuration{
						Fetch: FetchConfiguration{
							URL: "https://countries.service",
						},
					}),
				},
			},
			Fields: []plan.FieldConfiguration{
				{
					TypeName:  "Query",
					FieldName: "country",
					Path:      []string{"country"},
					Arguments: []plan.ArgumentConfiguration{
						{
							Name:       "code",
							SourceType: plan.FieldArgumentSource,
						},
					},
				},
				{
					TypeName:  "Query",
					FieldName: "countryAlias",
					Path:      []string{"country"},
					Arguments: []plan.ArgumentConfiguration{
						{
							Name:       "code",
							SourceType: plan.FieldArgumentSource,
						},
					},
				},
			},
		},
	))

	nestedGraphQLEngineFactory := &Factory{}
	t.Run("nested graphql engines", RunTest(`
		type Query {
			serviceOne(serviceOneArg: String): ServiceOneResponse
			anotherServiceOne(anotherServiceOneArg: Int): ServiceOneResponse
			reusingServiceOne(reusingServiceOneArg: String): ServiceOneResponse
			serviceTwo(serviceTwoArg: Boolean): ServiceTwoResponse
			secondServiceTwo(secondServiceTwoArg: Float): ServiceTwoResponse
		}
		type ServiceOneResponse {
			fieldOne: String!
			countries: [Country!]!
		}
		type ServiceTwoResponse {
			fieldTwo: String
			serviceOneField: String
			serviceOneResponse: ServiceOneResponse
		}
		type Country {
			name: String!
        }
	`, `
		query NestedQuery ($firstArg: String, $secondArg: Boolean, $thirdArg: Int, $fourthArg: Float){
			serviceOne(serviceOneArg: $firstArg) {
				fieldOne
				countries {
					name
				}
			}
			serviceTwo(serviceTwoArg: $secondArg){
				fieldTwo
				serviceOneResponse {
					fieldOne
				}
			}
			anotherServiceOne(anotherServiceOneArg: $thirdArg){
				fieldOne
			}
			secondServiceTwo(secondServiceTwoArg: $fourthArg){
				fieldTwo
				serviceOneField
			}
			reusingServiceOne(reusingServiceOneArg: $firstArg){
				fieldOne
			}
		}
	`, "NestedQuery",
		&plan.SynchronousResponsePlan{
			Response: &resolve.GraphQLResponse{
				Data: &resolve.Object{
					Fetch: &resolve.ParallelFetch{
						Fetches: []resolve.Fetch{
							&resolve.SingleFetch{
								BufferId:   0,
								Input:      `{"method":"POST","url":"https://service.one","body":{"query":"query($firstArg: String, $thirdArg: Int){serviceOne(serviceOneArg: $firstArg){fieldOne} anotherServiceOne(anotherServiceOneArg: $thirdArg){fieldOne} reusingServiceOne(reusingServiceOneArg: $firstArg){fieldOne}}","variables":{"thirdArg":$$1$$,"firstArg":$$0$$}}}`,
								DataSource: &Source{},
								Variables: resolve.NewVariables(
									&resolve.ContextVariable{
										Path:     []string{"firstArg"},
										Renderer: resolve.NewJSONVariableRendererWithValidation(`{"type":["string","null"]}`),
									},
									&resolve.ContextVariable{
										Path:     []string{"thirdArg"},
										Renderer: resolve.NewJSONVariableRendererWithValidation(`{"type":["integer","null"]}`),
									},
								),
								DataSourceIdentifier:  []byte("graphql_datasource.Source"),
								ProcessResponseConfig: resolve.ProcessResponseConfig{ExtractGraphqlResponse: true},
							},
							&resolve.SingleFetch{
								BufferId:   2,
								Input:      `{"method":"POST","url":"https://service.two","body":{"query":"query($secondArg: Boolean, $fourthArg: Float){serviceTwo(serviceTwoArg: $secondArg){fieldTwo serviceOneField} secondServiceTwo(secondServiceTwoArg: $fourthArg){fieldTwo serviceOneField}}","variables":{"fourthArg":$$1$$,"secondArg":$$0$$}}}`,
								DataSource: &Source{},
								Variables: resolve.NewVariables(
									&resolve.ContextVariable{
										Path:     []string{"secondArg"},
										Renderer: resolve.NewJSONVariableRendererWithValidation(`{"type":["boolean","null"]}`),
									},
									&resolve.ContextVariable{
										Path:     []string{"fourthArg"},
										Renderer: resolve.NewJSONVariableRendererWithValidation(`{"type":["number","null"]}`),
									},
								),
								DataSourceIdentifier:  []byte("graphql_datasource.Source"),
								ProcessResponseConfig: resolve.ProcessResponseConfig{ExtractGraphqlResponse: true},
							},
						},
					},
					Fields: []*resolve.Field{
						{
							BufferID:  0,
							HasBuffer: true,
							Name:      []byte("serviceOne"),
							Position: resolve.Position{
								Line:   3,
								Column: 4,
							},
							Value: &resolve.Object{
								Nullable: true,
								Path:     []string{"serviceOne"},

								Fetch: &resolve.SingleFetch{
									BufferId:              1,
									DataSource:            &Source{},
									Input:                 `{"method":"POST","url":"https://country.service","body":{"query":"{countries {name}}"}}`,
									DataSourceIdentifier:  []byte("graphql_datasource.Source"),
									ProcessResponseConfig: resolve.ProcessResponseConfig{ExtractGraphqlResponse: true},
								},

								Fields: []*resolve.Field{
									{
										Name: []byte("fieldOne"),
										Value: &resolve.String{
											Path: []string{"fieldOne"},
										},
										Position: resolve.Position{
											Line:   4,
											Column: 5,
										},
									},
									{
										Name:      []byte("countries"),
										HasBuffer: true,
										BufferID:  1,
										Position: resolve.Position{
											Line:   5,
											Column: 5,
										},
										Value: &resolve.Array{
											Path: []string{"countries"},
											Item: &resolve.Object{
												Fields: []*resolve.Field{
													{
														Name: []byte("name"),
														Value: &resolve.String{
															Path: []string{"name"},
														},
														Position: resolve.Position{
															Line:   6,
															Column: 6,
														},
													},
												},
											},
										},
									},
								},
							},
						},
						{
							HasBuffer: true,
							BufferID:  2,
							Name:      []byte("serviceTwo"),
							Position: resolve.Position{
								Line:   9,
								Column: 4,
							},
							Value: &resolve.Object{
								Nullable: true,
								Path:     []string{"serviceTwo"},
								Fetch: &resolve.SingleFetch{
									BufferId:   3,
									DataSource: &Source{},
									Input:      `{"method":"POST","url":"https://service.one","body":{"query":"query($a: String){serviceOneResponse: serviceOne(serviceOneArg: $a){fieldOne}}","variables":{"a":$$0$$}}}`,
									Variables: resolve.NewVariables(
										&resolve.ObjectVariable{
											Path:     []string{"serviceOneField"},
											Renderer: resolve.NewJSONVariableRendererWithValidation(`{"type":["string","null"]}`),
										},
									),
									DataSourceIdentifier:  []byte("graphql_datasource.Source"),
									ProcessResponseConfig: resolve.ProcessResponseConfig{ExtractGraphqlResponse: true},
								},
								Fields: []*resolve.Field{
									{
										Name: []byte("fieldTwo"),
										Value: &resolve.String{
											Nullable: true,
											Path:     []string{"fieldTwo"},
										},
										Position: resolve.Position{
											Line:   10,
											Column: 5,
										},
									},
									{
										HasBuffer: true,
										BufferID:  3,
										Name:      []byte("serviceOneResponse"),
										Position: resolve.Position{
											Line:   11,
											Column: 5,
										},
										Value: &resolve.Object{
											Nullable: true,
											Path:     []string{"serviceOneResponse"},
											Fields: []*resolve.Field{
												{
													Name: []byte("fieldOne"),
													Value: &resolve.String{
														Path: []string{"fieldOne"},
													},
													Position: resolve.Position{
														Line:   12,
														Column: 6,
													},
												},
											},
										},
									},
								},
							},
						},
						{
							HasBuffer: true,
							BufferID:  0,
							Name:      []byte("anotherServiceOne"),
							Position: resolve.Position{
								Line:   15,
								Column: 4,
							},
							Value: &resolve.Object{
								Nullable: true,
								Path:     []string{"anotherServiceOne"},
								Fields: []*resolve.Field{
									{
										Name: []byte("fieldOne"),
										Value: &resolve.String{
											Path: []string{"fieldOne"},
										},
										Position: resolve.Position{
											Line:   16,
											Column: 5,
										},
									},
								},
							},
						},
						{
							BufferID:  2,
							HasBuffer: true,
							Name:      []byte("secondServiceTwo"),
							Position: resolve.Position{
								Line:   18,
								Column: 4,
							},
							Value: &resolve.Object{
								Nullable: true,
								Path:     []string{"secondServiceTwo"},
								Fields: []*resolve.Field{
									{
										Name: []byte("fieldTwo"),
										Value: &resolve.String{
											Path:     []string{"fieldTwo"},
											Nullable: true,
										},
										Position: resolve.Position{
											Line:   19,
											Column: 5,
										},
									},
									{
										Name: []byte("serviceOneField"),
										Value: &resolve.String{
											Path:     []string{"serviceOneField"},
											Nullable: true,
										},
										Position: resolve.Position{
											Line:   20,
											Column: 5,
										},
									},
								},
							},
						},
						{
							BufferID:  0,
							HasBuffer: true,
							Name:      []byte("reusingServiceOne"),
							Position: resolve.Position{
								Line:   22,
								Column: 4,
							},
							Value: &resolve.Object{
								Nullable: true,
								Path:     []string{"reusingServiceOne"},
								Fields: []*resolve.Field{
									{
										Name: []byte("fieldOne"),
										Value: &resolve.String{
											Path: []string{"fieldOne"},
										},
										Position: resolve.Position{
											Line:   23,
											Column: 5,
										},
									},
								},
							},
						},
					},
				},
			},
		},
		plan.Configuration{
			DataSources: []plan.DataSourceConfiguration{
				{
					RootNodes: []plan.TypeField{
						{
							TypeName:   "Query",
							FieldNames: []string{"serviceOne", "anotherServiceOne", "reusingServiceOne"},
						},
						{
							TypeName:   "ServiceTwoResponse",
							FieldNames: []string{"serviceOneResponse"},
						},
					},
					ChildNodes: []plan.TypeField{
						{
							TypeName:   "ServiceOneResponse",
							FieldNames: []string{"fieldOne"},
						},
					},
					Custom: ConfigJson(Configuration{
						Fetch: FetchConfiguration{
							URL: "https://service.one",
						},
					}),
					Factory: nestedGraphQLEngineFactory,
				},
				{
					RootNodes: []plan.TypeField{
						{
							TypeName:   "Query",
							FieldNames: []string{"serviceTwo", "secondServiceTwo"},
						},
					},
					ChildNodes: []plan.TypeField{
						{
							TypeName:   "ServiceTwoResponse",
							FieldNames: []string{"fieldTwo", "serviceOneField"},
						},
					},
					Custom: ConfigJson(Configuration{
						Fetch: FetchConfiguration{
							URL: "https://service.two",
						},
					}),
					Factory: nestedGraphQLEngineFactory,
				},
				{
					RootNodes: []plan.TypeField{
						{
							TypeName:   "ServiceOneResponse",
							FieldNames: []string{"countries"},
						},
					},
					ChildNodes: []plan.TypeField{
						{
							TypeName:   "Country",
							FieldNames: []string{"name"},
						},
					},
					Custom: ConfigJson(Configuration{
						Fetch: FetchConfiguration{
							URL: "https://country.service",
						},
					}),
					Factory: nestedGraphQLEngineFactory,
				},
			},
			Fields: []plan.FieldConfiguration{
				{
					TypeName:       "ServiceTwoResponse",
					FieldName:      "serviceOneResponse",
					Path:           []string{"serviceOne"},
					RequiresFields: []string{"serviceOneField"},
					Arguments: []plan.ArgumentConfiguration{
						{
							Name:       "serviceOneArg",
							SourceType: plan.ObjectFieldSource,
							SourcePath: []string{"serviceOneField"},
						},
					},
				},
				{
					TypeName:  "Query",
					FieldName: "serviceTwo",
					Arguments: []plan.ArgumentConfiguration{
						{
							Name:       "serviceTwoArg",
							SourceType: plan.FieldArgumentSource,
						},
					},
				},
				{
					TypeName:  "Query",
					FieldName: "secondServiceTwo",
					Arguments: []plan.ArgumentConfiguration{
						{
							Name:       "secondServiceTwoArg",
							SourceType: plan.FieldArgumentSource,
						},
					},
				},
				{
					TypeName:  "Query",
					FieldName: "serviceOne",
					Arguments: []plan.ArgumentConfiguration{
						{
							Name:       "serviceOneArg",
							SourceType: plan.FieldArgumentSource,
						},
					},
				},
				{
					TypeName:  "Query",
					FieldName: "reusingServiceOne",
					Arguments: []plan.ArgumentConfiguration{
						{
							Name:       "reusingServiceOneArg",
							SourceType: plan.FieldArgumentSource,
						},
					},
				},
				{
					TypeName:  "Query",
					FieldName: "anotherServiceOne",
					Arguments: []plan.ArgumentConfiguration{
						{
							Name:       "anotherServiceOneArg",
							SourceType: plan.FieldArgumentSource,
						},
					},
				},
			},
		},
	))

	t.Run("mutation with variables in array object argument", RunTest(
		todoSchema,
		`mutation AddTask($title: String!, $completed: Boolean!, $name: String! @fromClaim(name: "sub")) {
					  addTask(input: [{title: $title, completed: $completed, user: {name: $name}}]){
						task {
						  id
						  title
						  completed
						}
					  }
					}`,
		"AddTask",
		&plan.SynchronousResponsePlan{
			Response: &resolve.GraphQLResponse{
				Data: &resolve.Object{
					Fetch: &resolve.SingleFetch{
						BufferId:   0,
						Input:      `{"method":"POST","url":"https://graphql.service","body":{"query":"mutation($title: String!, $completed: Boolean!, $name: String!){addTask(input: [{title: $title,completed: $completed,user: {name: $name}}]){task {id title completed}}}","variables":{"name":$$2$$,"completed":$$1$$,"title":$$0$$}}}`,
						DataSource: &Source{},
						Variables: resolve.NewVariables(
							&resolve.ContextVariable{
								Path:     []string{"title"},
								Renderer: resolve.NewJSONVariableRendererWithValidation(`{"type":["string"]}`),
							},
							&resolve.ContextVariable{
								Path:     []string{"completed"},
								Renderer: resolve.NewJSONVariableRendererWithValidation(`{"type":["boolean"]}`),
							},
							&resolve.ContextVariable{
								Path:     []string{"name"},
								Renderer: resolve.NewJSONVariableRendererWithValidation(`{"type":["string","null"]}`),
							},
						),
						DisallowSingleFlight:  true,
						DataSourceIdentifier:  []byte("graphql_datasource.Source"),
						ProcessResponseConfig: resolve.ProcessResponseConfig{ExtractGraphqlResponse: true},
					},
					Fields: []*resolve.Field{
						{
							HasBuffer: true,
							BufferID:  0,
							Name:      []byte("addTask"),
							Position: resolve.Position{
								Line:   2,
								Column: 8,
							},
							Value: &resolve.Object{
								Path:     []string{"addTask"},
								Nullable: true,
								Fields: []*resolve.Field{
									{
										Name: []byte("task"),
										Position: resolve.Position{
											Line:   3,
											Column: 7,
										},
										Value: &resolve.Array{
											Nullable: true,
											Path:     []string{"task"},
											Item: &resolve.Object{
												Nullable: true,
												Fields: []*resolve.Field{
													{
														Name: []byte("id"),
														Value: &resolve.String{
															Path: []string{"id"},
														},
														Position: resolve.Position{
															Line:   4,
															Column: 9,
														},
													},
													{
														Name: []byte("title"),
														Value: &resolve.String{
															Path: []string{"title"},
														},
														Position: resolve.Position{
															Line:   5,
															Column: 9,
														},
													},
													{
														Name: []byte("completed"),
														Value: &resolve.Boolean{
															Path: []string{"completed"},
														},
														Position: resolve.Position{
															Line:   6,
															Column: 9,
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
		},
		plan.Configuration{
			DataSources: []plan.DataSourceConfiguration{
				{
					RootNodes: []plan.TypeField{
						{
							TypeName:   "Mutation",
							FieldNames: []string{"addTask"},
						},
					},
					ChildNodes: []plan.TypeField{
						{
							TypeName:   "AddTaskPayload",
							FieldNames: []string{"task"},
						},
						{
							TypeName:   "Task",
							FieldNames: []string{"id", "title", "completed"},
						},
					},
					Custom: ConfigJson(Configuration{
						Fetch: FetchConfiguration{
							URL: "https://graphql.service",
						},
					}),
					Factory: &Factory{},
				},
			},
			Fields: []plan.FieldConfiguration{
				{
					TypeName:  "Mutation",
					FieldName: "addTask",
					Arguments: []plan.ArgumentConfiguration{
						{
							Name:       "input",
							SourceType: plan.FieldArgumentSource,
						},
					},
				},
			},
		},
	))

	t.Run("inline object value with arguments", RunTest(`
			schema {
				mutation: Mutation
			}
			type Mutation {
				createUser(input: CreateUserInput!): CreateUser
			}
			input CreateUserInput {
				user: UserInput
			}
			input UserInput {
				id: String
				username: String
			}
			type CreateUser {
				user: User
			}
			type User {
				id: String
				username: String
				createdDate: String
			}
			directive @fromClaim(name: String) on VARIABLE_DEFINITION
			`, `
			mutation Register($name: String $id: String @fromClaim(name: "sub")) {
			  createUser(input: {user: {id: $id username: $name}}){
				user {
				  id
				  username
				  createdDate
				}
			  }
			}`,
		"Register",
		&plan.SynchronousResponsePlan{
			Response: &resolve.GraphQLResponse{
				Data: &resolve.Object{
					Fetch: &resolve.SingleFetch{
						BufferId:   0,
						Input:      `{"method":"POST","url":"https://user.service","body":{"query":"mutation($id: String, $name: String){createUser(input: {user: {id: $id,username: $name}}){user {id username createdDate}}}","variables":{"name":$$1$$,"id":$$0$$}}}`,
						DataSource: &Source{},
						Variables: resolve.NewVariables(
							&resolve.ContextVariable{
								Path:     []string{"id"},
								Renderer: resolve.NewJSONVariableRendererWithValidation(`{"type":["string","null"]}`),
							},
							&resolve.ContextVariable{
								Path:     []string{"name"},
								Renderer: resolve.NewJSONVariableRendererWithValidation(`{"type":["string","null"]}`),
							},
						),
						DisallowSingleFlight:  true,
						DataSourceIdentifier:  []byte("graphql_datasource.Source"),
						ProcessResponseConfig: resolve.ProcessResponseConfig{ExtractGraphqlResponse: true},
					},
					Fields: []*resolve.Field{
						{
							BufferID:  0,
							HasBuffer: true,
							Name:      []byte("createUser"),
							Position: resolve.Position{
								Line:   3,
								Column: 6,
							},
							Value: &resolve.Object{
								Nullable: true,
								Path:     []string{"createUser"},
								Fields: []*resolve.Field{
									{
										Name: []byte("user"),
										Position: resolve.Position{
											Line:   4,
											Column: 5,
										},
										Value: &resolve.Object{
											Path:     []string{"user"},
											Nullable: true,
											Fields: []*resolve.Field{
												{
													Name: []byte("id"),
													Value: &resolve.String{
														Path:     []string{"id"},
														Nullable: true,
													},
													Position: resolve.Position{
														Line:   5,
														Column: 7,
													},
												},
												{
													Name: []byte("username"),
													Value: &resolve.String{
														Path:     []string{"username"},
														Nullable: true,
													},
													Position: resolve.Position{
														Line:   6,
														Column: 7,
													},
												},
												{
													Name: []byte("createdDate"),
													Value: &resolve.String{
														Path:     []string{"createdDate"},
														Nullable: true,
													},
													Position: resolve.Position{
														Line:   7,
														Column: 7,
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
		plan.Configuration{
			DataSources: []plan.DataSourceConfiguration{
				{
					RootNodes: []plan.TypeField{
						{
							TypeName:   "Mutation",
							FieldNames: []string{"createUser"},
						},
					},
					ChildNodes: []plan.TypeField{
						{
							TypeName:   "CreateUser",
							FieldNames: []string{"user"},
						},
						{
							TypeName:   "User",
							FieldNames: []string{"id", "username", "createdDate"},
						},
					},
					Custom: ConfigJson(Configuration{
						Fetch: FetchConfiguration{
							URL: "https://user.service",
						},
					}),
					Factory: &Factory{},
				},
			},
			Fields: []plan.FieldConfiguration{
				{
					TypeName:  "Mutation",
					FieldName: "createUser",
					Arguments: []plan.ArgumentConfiguration{
						{
							Name:       "input",
							SourceType: plan.FieldArgumentSource,
						},
					},
				},
			},
		},
	))

	t.Run("mutation with union response", RunTest(wgSchema, `
		mutation CreateNamespace($name: String! $personal: Boolean!) {
			__typename
			namespaceCreate(input: {name: $name, personal: $personal}){
				__typename
				... on NamespaceCreated {
					namespace {
						id
						name
					}
				}
				... on Error {
					code
					message
				}
			}
		}`, "CreateNamespace",
		&plan.SynchronousResponsePlan{
			Response: &resolve.GraphQLResponse{
				Data: &resolve.Object{
					Fetch: &resolve.SingleFetch{
						BufferId:   0,
						Input:      `{"method":"POST","url":"http://api.com","body":{"query":"mutation($name: String!, $personal: Boolean!){__typename namespaceCreate(input: {name: $name,personal: $personal}){__typename ... on NamespaceCreated {namespace {id name}} ... on Error {code message}}}","variables":{"personal":$$1$$,"name":$$0$$}}}`,
						DataSource: &Source{},
						Variables: resolve.NewVariables(
							&resolve.ContextVariable{
								Path:     []string{"name"},
								Renderer: resolve.NewJSONVariableRendererWithValidation(`{"type":["string"]}`),
							},
							&resolve.ContextVariable{
								Path:     []string{"personal"},
								Renderer: resolve.NewJSONVariableRendererWithValidation(`{"type":["boolean"]}`),
							},
						),
						DisallowSingleFlight:  true,
						DataSourceIdentifier:  []byte("graphql_datasource.Source"),
						ProcessResponseConfig: resolve.ProcessResponseConfig{ExtractGraphqlResponse: true},
					},
					Fields: []*resolve.Field{
						{
							Name: []byte("__typename"),
							Value: &resolve.String{
								Path:     []string{"__typename"},
								Nullable: false,
							},
							Position: resolve.Position{
								Line:   3,
								Column: 4,
							},
						},
						{
							Name:      []byte("namespaceCreate"),
							HasBuffer: true,
							BufferID:  0,
							Position: resolve.Position{
								Line:   4,
								Column: 4,
							},
							Value: &resolve.Object{
								Path: []string{"namespaceCreate"},
								Fields: []*resolve.Field{
									{
										Name: []byte("__typename"),
										Value: &resolve.String{
											Path:     []string{"__typename"},
											Nullable: false,
										},
										Position: resolve.Position{
											Line:   5,
											Column: 5,
										},
									},
									{
										OnTypeName: []byte("NamespaceCreated"),
										Name:       []byte("namespace"),
										Position: resolve.Position{
											Line:   7,
											Column: 6,
										},
										Value: &resolve.Object{
											Path: []string{"namespace"},
											Fields: []*resolve.Field{
												{
													Name: []byte("id"),
													Value: &resolve.String{
														Path:     []string{"id"},
														Nullable: false,
													},
													Position: resolve.Position{
														Line:   8,
														Column: 7,
													},
												},
												{
													Name: []byte("name"),
													Value: &resolve.String{
														Path:     []string{"name"},
														Nullable: false,
													},
													Position: resolve.Position{
														Line:   9,
														Column: 7,
													},
												},
											},
										},
									},
									{
										OnTypeName: []byte("Error"),
										Name:       []byte("code"),
										Value: &resolve.String{
											Path: []string{"code"},
										},
										Position: resolve.Position{
											Line:   13,
											Column: 6,
										},
									},
									{
										OnTypeName: []byte("Error"),
										Name:       []byte("message"),
										Value: &resolve.String{
											Path: []string{"message"},
										},
										Position: resolve.Position{
											Line:   14,
											Column: 6,
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
					RootNodes: []plan.TypeField{
						{
							TypeName: "Mutation",
							FieldNames: []string{
								"namespaceCreate",
							},
						},
					},
					ChildNodes: []plan.TypeField{
						{
							TypeName: "NamespaceCreated",
							FieldNames: []string{
								"namespace",
							},
						},
						{
							TypeName:   "Namespace",
							FieldNames: []string{"id", "name"},
						},
						{
							TypeName:   "Error",
							FieldNames: []string{"code", "message"},
						},
					},
					Custom: ConfigJson(Configuration{
						Fetch: FetchConfiguration{
							URL:    "http://api.com",
							Method: "POST",
						},
						Subscription: SubscriptionConfiguration{
							URL: "ws://api.com",
						},
					}),
					Factory: &Factory{},
				},
			},
			Fields: []plan.FieldConfiguration{
				{
					TypeName:  "Mutation",
					FieldName: "namespaceCreate",
					Arguments: []plan.ArgumentConfiguration{
						{
							Name:       "input",
							SourceType: plan.FieldArgumentSource,
						},
					},
					DisableDefaultMapping: false,
					Path:                  []string{},
				},
			},
			DefaultFlushIntervalMillis: 500,
		}))
	factory := &Factory{
		HTTPClient: http.DefaultClient,
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	t.Run("subscription", runTestOnTestDefinition(`
		subscription RemainingJedis {
			remainingJedis
		}
	`, "RemainingJedis", &plan.SubscriptionResponsePlan{
		Response: &resolve.GraphQLSubscription{
			Trigger: resolve.GraphQLSubscriptionTrigger{
				Input: []byte(`{"url":"wss://swapi.com/graphql","body":{"query":"subscription{remainingJedis}"}}`),
				Source: &SubscriptionSource{
					NewWebSocketGraphQLSubscriptionClient(http.DefaultClient, ctx),
				},
			},
			Response: &resolve.GraphQLResponse{
				Data: &resolve.Object{
					Fields: []*resolve.Field{
						{
							Name: []byte("remainingJedis"),
							Position: resolve.Position{
								Line:   3,
								Column: 4,
							},
							Value: &resolve.Integer{
								Path:     []string{"remainingJedis"},
								Nullable: false,
							},
						},
					},
				},
			},
		},
	}, testWithFactory(factory)))

	t.Run("subscription with variables", RunTest(`
		type Subscription {
			foo(bar: String): Int!
 		}
`, `
		subscription SubscriptionWithVariables {
			foo(bar: "baz")
		}
	`, "SubscriptionWithVariables", &plan.SubscriptionResponsePlan{
		Response: &resolve.GraphQLSubscription{
			Trigger: resolve.GraphQLSubscriptionTrigger{
				Input: []byte(`{"url":"wss://swapi.com/graphql","body":{"query":"subscription($a: String){foo(bar: $a)}","variables":{"a":$$0$$}}}`),
				Variables: resolve.NewVariables(
					&resolve.ContextVariable{
						Path:     []string{"a"},
						Renderer: resolve.NewJSONVariableRendererWithValidation(`{"type":["string","null"]}`),
					},
				),
				Source: &SubscriptionSource{
					client: NewWebSocketGraphQLSubscriptionClient(http.DefaultClient, ctx),
				},
			},
			Response: &resolve.GraphQLResponse{
				Data: &resolve.Object{
					Fields: []*resolve.Field{
						{
							Name: []byte("foo"),
							Position: resolve.Position{
								Line:   3,
								Column: 4,
							},
							Value: &resolve.Integer{
								Path:     []string{"foo"},
								Nullable: false,
							},
						},
					},
				},
			},
		},
	}, plan.Configuration{
		DataSources: []plan.DataSourceConfiguration{
			{
				RootNodes: []plan.TypeField{
					{
						TypeName:   "Subscription",
						FieldNames: []string{"foo"},
					},
				},
				Custom: ConfigJson(Configuration{
					Subscription: SubscriptionConfiguration{
						URL: "wss://swapi.com/graphql",
					},
				}),
				Factory: factory,
			},
		},
		Fields: []plan.FieldConfiguration{
			{
				TypeName:  "Subscription",
				FieldName: "foo",
				Arguments: []plan.ArgumentConfiguration{
					{
						Name:       "bar",
						SourceType: plan.FieldArgumentSource,
					},
				},
			},
		},
	}))

	batchFactory := NewBatchFactory()
	federationFactory := &Factory{BatchFactory: batchFactory}
	t.Run("federation", RunTest(federationTestSchema,
		`	query MyReviews {
						me {
							id
							username
							reviews {
								body
								author {
									id
									username
								}	
								product {
									name
									price
									reviews {
										body
										author {
											id
											username
										}
									}
								}
							}
						}
					}`,
		"MyReviews",
		&plan.SynchronousResponsePlan{
			Response: &resolve.GraphQLResponse{
				Data: &resolve.Object{
					Fetch: &resolve.SingleFetch{
						BufferId:              0,
						Input:                 `{"method":"POST","url":"http://user.service","body":{"query":"{me {id username}}"}}`,
						DataSource:            &Source{},
						DataSourceIdentifier:  []byte("graphql_datasource.Source"),
						ProcessResponseConfig: resolve.ProcessResponseConfig{ExtractGraphqlResponse: true},
					},
					Fields: []*resolve.Field{
						{
							HasBuffer: true,
							BufferID:  0,
							Name:      []byte("me"),
							Position: resolve.Position{
								Line:   2,
								Column: 7,
							},
							Value: &resolve.Object{
								Fetch: &resolve.BatchFetch{
									Fetch: &resolve.SingleFetch{
										BufferId: 1,
										Input:    `{"method":"POST","url":"http://review.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {reviews {body author {id username} product {upc}}}}}","variables":{"representations":[{"id":$$0$$,"__typename":"User"}]}}}`,
										Variables: resolve.NewVariables(
											&resolve.ObjectVariable{
												Path:     []string{"id"},
												Renderer: resolve.NewJSONVariableRendererWithValidation(`{"type":["string","integer"]}`),
											},
										),
										DataSource:           &Source{},
										DataSourceIdentifier: []byte("graphql_datasource.Source"),
										ProcessResponseConfig: resolve.ProcessResponseConfig{
											ExtractGraphqlResponse:    true,
											ExtractFederationEntities: true,
										},
									},
									BatchFactory: batchFactory,
								},
								Path:     []string{"me"},
								Nullable: true,
								Fields: []*resolve.Field{
									{
										Name: []byte("id"),
										Value: &resolve.String{
											Path: []string{"id"},
										},
										Position: resolve.Position{
											Line:   3,
											Column: 8,
										},
									},
									{
										Name: []byte("username"),
										Value: &resolve.String{
											Path: []string{"username"},
										},
										Position: resolve.Position{
											Line:   4,
											Column: 8,
										},
									},
									{
										HasBuffer: true,
										BufferID:  1,
										Name:      []byte("reviews"),
										Position: resolve.Position{
											Line:   5,
											Column: 8,
										},
										Value: &resolve.Array{
											Path:     []string{"reviews"},
											Nullable: true,
											Item: &resolve.Object{
												Nullable: true,
												Fields: []*resolve.Field{
													{
														Name: []byte("body"),
														Value: &resolve.String{
															Path: []string{"body"},
														},
														Position: resolve.Position{
															Line:   6,
															Column: 9,
														},
													},
													{
														Name: []byte("author"),
														Position: resolve.Position{
															Line:   7,
															Column: 9,
														},
														Value: &resolve.Object{
															Path: []string{"author"},
															Fields: []*resolve.Field{
																{
																	Name: []byte("id"),
																	Value: &resolve.String{
																		Path: []string{"id"},
																	},
																	Position: resolve.Position{
																		Line:   8,
																		Column: 10,
																	},
																},
																{
																	Name: []byte("username"),
																	Value: &resolve.String{
																		Path: []string{"username"},
																	},
																	Position: resolve.Position{
																		Line:   9,
																		Column: 10,
																	},
																},
															},
														},
													},
													{
														Name: []byte("product"),
														Position: resolve.Position{
															Line:   11,
															Column: 9,
														},
														Value: &resolve.Object{
															Path: []string{"product"},
															Fetch: &resolve.ParallelFetch{
																Fetches: []resolve.Fetch{
																	&resolve.BatchFetch{
																		Fetch: &resolve.SingleFetch{
																			BufferId:   2,
																			Input:      `{"method":"POST","url":"http://product.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Product {name price}}}","variables":{"representations":[{"upc":$$0$$,"__typename":"Product"}]}}}`,
																			DataSource: &Source{},
																			Variables: resolve.NewVariables(
																				&resolve.ObjectVariable{
																					Path:     []string{"upc"},
																					Renderer: resolve.NewJSONVariableRendererWithValidation(`{"type":["string"]}`),
																				},
																			),
																			DataSourceIdentifier: []byte("graphql_datasource.Source"),
																			ProcessResponseConfig: resolve.ProcessResponseConfig{
																				ExtractGraphqlResponse:    true,
																				ExtractFederationEntities: true,
																			},
																		},
																		BatchFactory: batchFactory,
																	},
																	&resolve.BatchFetch{
																		Fetch: &resolve.SingleFetch{
																			BufferId: 3,
																			Input:    `{"method":"POST","url":"http://review.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Product {reviews {body author {id username}}}}}","variables":{"representations":[{"upc":$$0$$,"__typename":"Product"}]}}}`,
																			Variables: resolve.NewVariables(
																				&resolve.ObjectVariable{
																					Path:     []string{"upc"},
																					Renderer: resolve.NewJSONVariableRendererWithValidation(`{"type":["string"]}`),
																				},
																			),
																			DataSource:           &Source{},
																			DataSourceIdentifier: []byte("graphql_datasource.Source"),
																			ProcessResponseConfig: resolve.ProcessResponseConfig{
																				ExtractGraphqlResponse:    true,
																				ExtractFederationEntities: true,
																			},
																		},
																		BatchFactory: batchFactory,
																	},
																},
															},
															Fields: []*resolve.Field{
																{
																	HasBuffer: true,
																	BufferID:  2,
																	Name:      []byte("name"),
																	Value: &resolve.String{
																		Path: []string{"name"},
																	},
																	Position: resolve.Position{
																		Line:   12,
																		Column: 10,
																	},
																},
																{
																	HasBuffer: true,
																	BufferID:  2,
																	Name:      []byte("price"),
																	Value: &resolve.Integer{
																		Path: []string{"price"},
																	},
																	Position: resolve.Position{
																		Line:   13,
																		Column: 10,
																	},
																},
																{
																	HasBuffer: true,
																	BufferID:  3,
																	Name:      []byte("reviews"),
																	Position: resolve.Position{
																		Line:   14,
																		Column: 10,
																	},
																	Value: &resolve.Array{
																		Nullable: true,
																		Path:     []string{"reviews"},
																		Item: &resolve.Object{
																			Nullable: true,
																			Fields: []*resolve.Field{
																				{
																					Name: []byte("body"),
																					Value: &resolve.String{
																						Path: []string{"body"},
																					},
																					Position: resolve.Position{
																						Line:   15,
																						Column: 11,
																					},
																				},
																				{
																					Name: []byte("author"),
																					Position: resolve.Position{
																						Line:   16,
																						Column: 11,
																					},
																					Value: &resolve.Object{
																						Path: []string{"author"},
																						Fields: []*resolve.Field{
																							{
																								Name: []byte("id"),
																								Value: &resolve.String{
																									Path: []string{"id"},
																								},
																								Position: resolve.Position{
																									Line:   17,
																									Column: 12,
																								},
																							},
																							{
																								Name: []byte("username"),
																								Value: &resolve.String{
																									Path: []string{"username"},
																								},
																								Position: resolve.Position{
																									Line:   18,
																									Column: 12,
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
		plan.Configuration{
			DataSources: []plan.DataSourceConfiguration{
				{
					RootNodes: []plan.TypeField{
						{
							TypeName:   "Query",
							FieldNames: []string{"me"},
						},
					},
					ChildNodes: []plan.TypeField{
						{
							TypeName:   "User",
							FieldNames: []string{"id", "username"},
						},
					},
					Custom: ConfigJson(Configuration{
						Fetch: FetchConfiguration{
							URL: "http://user.service",
						},
						Federation: FederationConfiguration{
							Enabled:    true,
							ServiceSDL: "extend type Query {me: User} type User @key(fields: \"id\"){ id: ID! username: String!}",
						},
					}),
					Factory: federationFactory,
				},
				{
					RootNodes: []plan.TypeField{
						{
							TypeName:   "Query",
							FieldNames: []string{"topProducts"},
						},
						{
							TypeName:   "Subscription",
							FieldNames: []string{"updatedPrice"},
						},
						{
							TypeName:   "Product",
							FieldNames: []string{"upc", "name", "price"},
						},
					},
					ChildNodes: []plan.TypeField{
						{
							TypeName:   "Product",
							FieldNames: []string{"upc", "name", "price"},
						},
					},
					Custom: ConfigJson(Configuration{
						Fetch: FetchConfiguration{
							URL: "http://product.service",
						},
						Subscription: SubscriptionConfiguration{
							URL: "ws://product.service",
						},
						Federation: FederationConfiguration{
							Enabled:    true,
							ServiceSDL: "extend type Query {topProducts(first: Int = 5): [Product]} type Product @key(fields: \"upc\") {upc: String! price: Int!}",
						},
					}),
					Factory: federationFactory,
				},
				{
					RootNodes: []plan.TypeField{
						{
							TypeName:   "User",
							FieldNames: []string{"reviews"},
						},
						{
							TypeName:   "Product",
							FieldNames: []string{"reviews"},
						},
					},
					ChildNodes: []plan.TypeField{
						{
							TypeName:   "Review",
							FieldNames: []string{"body", "author", "product"},
						},
						{
							TypeName:   "User",
							FieldNames: []string{"id", "username"},
						},
						{
							TypeName:   "Product",
							FieldNames: []string{"upc"},
						},
					},
					Factory: federationFactory,
					Custom: ConfigJson(Configuration{
						Fetch: FetchConfiguration{
							URL: "http://review.service",
						},
						Federation: FederationConfiguration{
							Enabled:    true,
							ServiceSDL: "type Review { body: String! author: User! @provides(fields: \"username\") product: Product! } extend type User @key(fields: \"id\") { id: ID! @external reviews: [Review] } extend type Product @key(fields: \"upc\") { upc: String! @external reviews: [Review] }",
						},
					}),
				},
			},
			Fields: []plan.FieldConfiguration{
				{
					TypeName:  "Query",
					FieldName: "topProducts",
					Arguments: []plan.ArgumentConfiguration{
						{
							Name:       "first",
							SourceType: plan.FieldArgumentSource,
						},
					},
				},
				{
					TypeName:       "User",
					FieldName:      "reviews",
					RequiresFields: []string{"id"},
				},
				{
					TypeName:       "Product",
					FieldName:      "name",
					RequiresFields: []string{"upc"},
				},
				{
					TypeName:       "Product",
					FieldName:      "price",
					RequiresFields: []string{"upc"},
				},
				{
					TypeName:       "Product",
					FieldName:      "reviews",
					RequiresFields: []string{"upc"},
				},
			},
		}))

	t.Run("federation with variables", RunTest(federationTestSchema,
		`	query MyReviews($publicOnly: Boolean!, $someSkipCondition: Boolean!) {
						me {
							reviews {
								body
								notes @skip(if: $someSkipCondition)
								likes(filterToPublicOnly: $publicOnly)
							}
						}
					}`,
		"MyReviews",
		&plan.SynchronousResponsePlan{
			Response: &resolve.GraphQLResponse{
				Data: &resolve.Object{
					Fetch: &resolve.SingleFetch{
						BufferId:              0,
						Input:                 `{"method":"POST","url":"http://user.service","body":{"query":"{me {id}}"}}`,
						DataSource:            &Source{},
						DataSourceIdentifier:  []byte("graphql_datasource.Source"),
						ProcessResponseConfig: resolve.ProcessResponseConfig{ExtractGraphqlResponse: true},
					},
					Fields: []*resolve.Field{
						{
							HasBuffer: true,
							BufferID:  0,
							Name:      []byte("me"),
							Position: resolve.Position{
								Line:   2,
								Column: 7,
							},
							Value: &resolve.Object{
								Fetch: &resolve.BatchFetch{
									Fetch: &resolve.SingleFetch{
										BufferId: 1,
										Input:    `{"method":"POST","url":"http://review.service","body":{"query":"query($representations: [_Any!]!, $someSkipCondition: Boolean!, $publicOnly: Boolean!){_entities(representations: $representations){... on User {reviews {body notes @skip(if: $someSkipCondition) likes(filterToPublicOnly: $publicOnly)}}}}","variables":{"publicOnly":$$2$$,"someSkipCondition":$$1$$,"representations":[{"id":$$0$$,"__typename":"User"}]}}}`,
										Variables: resolve.NewVariables(
											&resolve.ObjectVariable{
												Path:     []string{"id"},
												Renderer: resolve.NewJSONVariableRendererWithValidation(`{"type":["string","integer"]}`),
											},
											&resolve.ContextVariable{
												Path:     []string{"someSkipCondition"},
												Renderer: resolve.NewJSONVariableRendererWithValidation(`{"type":["boolean"]}`),
											},
											&resolve.ContextVariable{
												Path:     []string{"publicOnly"},
												Renderer: resolve.NewJSONVariableRendererWithValidation(`{"type":["boolean","null"]}`),
											},
										),
										DataSource:           &Source{},
										DataSourceIdentifier: []byte("graphql_datasource.Source"),
										ProcessResponseConfig: resolve.ProcessResponseConfig{
											ExtractGraphqlResponse:    true,
											ExtractFederationEntities: true,
										},
									},
									BatchFactory: batchFactory,
								},
								Path:     []string{"me"},
								Nullable: true,
								Fields: []*resolve.Field{
									{
										HasBuffer: true,
										BufferID:  1,
										Name:      []byte("reviews"),
										Position: resolve.Position{
											Line:   3,
											Column: 8,
										},
										Value: &resolve.Array{
											Path:     []string{"reviews"},
											Nullable: true,
											Item: &resolve.Object{
												Nullable: true,
												Fields: []*resolve.Field{
													{
														Name: []byte("body"),
														Value: &resolve.String{
															Path: []string{"body"},
														},
														Position: resolve.Position{
															Line:   4,
															Column: 9,
														},
													},
													{
														Name: []byte("notes"),
														Value: &resolve.String{
															Path:     []string{"notes"},
															Nullable: true,
														},
														Position: resolve.Position{
															Line:   5,
															Column: 9,
														},
														SkipDirectiveDefined: true,
														SkipVariableName:     "someSkipCondition",
													},
													{
														Name: []byte("likes"),
														Value: &resolve.String{
															Path: []string{"likes"},
														},
														Position: resolve.Position{
															Line:   6,
															Column: 9,
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
		},
		plan.Configuration{
			DataSources: []plan.DataSourceConfiguration{
				{
					RootNodes: []plan.TypeField{
						{
							TypeName:   "Query",
							FieldNames: []string{"me"},
						},
					},
					ChildNodes: []plan.TypeField{
						{
							TypeName:   "User",
							FieldNames: []string{"id"},
						},
					},
					Custom: ConfigJson(Configuration{
						Fetch: FetchConfiguration{
							URL: "http://user.service",
						},
						Federation: FederationConfiguration{
							Enabled:    true,
							ServiceSDL: "extend type Query {me: User} type User @key(fields: \"id\"){ id: ID! }",
						},
					}),
					Factory: federationFactory,
				},
				{
					RootNodes: []plan.TypeField{
						{
							TypeName:   "User",
							FieldNames: []string{"reviews"},
						},
					},
					ChildNodes: []plan.TypeField{
						{
							TypeName:   "Review",
							FieldNames: []string{"body", "notes", "likes"},
						},
						{
							TypeName:   "User",
							FieldNames: []string{"id"},
						},
					},
					Factory: federationFactory,
					Custom: ConfigJson(Configuration{
						Fetch: FetchConfiguration{
							URL: "http://review.service",
						},
						Federation: FederationConfiguration{
							Enabled:    true,
							ServiceSDL: "type Review { body: String! notes: String likes(filterToPublicOnly: Boolean): Int! } extend type User @key(fields: \"id\") { id: ID! @external reviews: [Review] }",
						},
					}),
				},
			},
			Fields: []plan.FieldConfiguration{
				{
					TypeName:       "User",
					FieldName:      "reviews",
					RequiresFields: []string{"id"},
				},
				{
					TypeName:  "Review",
					FieldName: "likes",
					Arguments: []plan.ArgumentConfiguration{
						{
							Name:       "filterToPublicOnly",
							SourceType: plan.FieldArgumentSource,
						},
					},
				},
			},
		}))

	t.Run("federation with variables and renamed types", RunTest(federationTestSchema,
		`	query MyReviews($publicOnly: Boolean!, $someSkipCondition: Boolean!) {
						me {
							reviews {
								body
								notes @skip(if: $someSkipCondition)
								likes(filterToPublicOnly: $publicOnly)
							}
						}
					}`,
		"MyReviews",
		&plan.SynchronousResponsePlan{
			Response: &resolve.GraphQLResponse{
				Data: &resolve.Object{
					Fetch: &resolve.SingleFetch{
						BufferId:              0,
						Input:                 `{"method":"POST","url":"http://user.service","body":{"query":"{me {id}}"}}`,
						DataSource:            &Source{},
						DataSourceIdentifier:  []byte("graphql_datasource.Source"),
						ProcessResponseConfig: resolve.ProcessResponseConfig{ExtractGraphqlResponse: true},
					},
					Fields: []*resolve.Field{
						{
							HasBuffer: true,
							BufferID:  0,
							Name:      []byte("me"),
							Position: resolve.Position{
								Line:   2,
								Column: 7,
							},
							Value: &resolve.Object{
								Fetch: &resolve.BatchFetch{
									Fetch: &resolve.SingleFetch{
										BufferId: 1,
										Input:    `{"method":"POST","url":"http://review.service","body":{"query":"query($representations: [_Any!]!, $someSkipCondition: XBoolean!, $publicOnly: XBoolean!){_entities(representations: $representations){... on User {reviews {body notes @skip(if: $someSkipCondition) likes(filterToPublicOnly: $publicOnly)}}}}","variables":{"publicOnly":$$2$$,"someSkipCondition":$$1$$,"representations":[{"id":$$0$$,"__typename":"User"}]}}}`,
										Variables: resolve.NewVariables(
											&resolve.ObjectVariable{
												Path:     []string{"id"},
												Renderer: resolve.NewJSONVariableRendererWithValidation(`{"type":["string","integer"]}`),
											},
											&resolve.ContextVariable{
												Path:     []string{"someSkipCondition"},
												Renderer: resolve.NewJSONVariableRendererWithValidation(`{"type":["boolean"]}`),
											},
											&resolve.ContextVariable{
												Path:     []string{"publicOnly"},
												Renderer: resolve.NewJSONVariableRendererWithValidation(`{"type":["boolean","null"]}`),
											},
										),
										DataSource:           &Source{},
										DataSourceIdentifier: []byte("graphql_datasource.Source"),
										ProcessResponseConfig: resolve.ProcessResponseConfig{
											ExtractGraphqlResponse:    true,
											ExtractFederationEntities: true,
										},
									},
									BatchFactory: batchFactory,
								},
								Path:     []string{"me"},
								Nullable: true,
								Fields: []*resolve.Field{
									{
										HasBuffer: true,
										BufferID:  1,
										Name:      []byte("reviews"),
										Position: resolve.Position{
											Line:   3,
											Column: 8,
										},
										Value: &resolve.Array{
											Path:     []string{"reviews"},
											Nullable: true,
											Item: &resolve.Object{
												Nullable: true,
												Fields: []*resolve.Field{
													{
														Name: []byte("body"),
														Value: &resolve.String{
															Path: []string{"body"},
														},
														Position: resolve.Position{
															Line:   4,
															Column: 9,
														},
													},
													{
														Name: []byte("notes"),
														Value: &resolve.String{
															Path:     []string{"notes"},
															Nullable: true,
														},
														Position: resolve.Position{
															Line:   5,
															Column: 9,
														},
														SkipDirectiveDefined: true,
														SkipVariableName:     "someSkipCondition",
													},
													{
														Name: []byte("likes"),
														Value: &resolve.String{
															Path: []string{"likes"},
														},
														Position: resolve.Position{
															Line:   6,
															Column: 9,
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
		},
		plan.Configuration{
			DataSources: []plan.DataSourceConfiguration{
				{
					RootNodes: []plan.TypeField{
						{
							TypeName:   "Query",
							FieldNames: []string{"me"},
						},
					},
					ChildNodes: []plan.TypeField{
						{
							TypeName:   "User",
							FieldNames: []string{"id"},
						},
					},
					Custom: ConfigJson(Configuration{
						Fetch: FetchConfiguration{
							URL: "http://user.service",
						},
						Federation: FederationConfiguration{
							Enabled:    true,
							ServiceSDL: "extend type Query {me: User} type User @key(fields: \"id\"){ id: ID! }",
						},
					}),
					Factory: federationFactory,
				},
				{
					RootNodes: []plan.TypeField{
						{
							TypeName:   "User",
							FieldNames: []string{"reviews"},
						},
					},
					ChildNodes: []plan.TypeField{
						{
							TypeName:   "Review",
							FieldNames: []string{"body", "notes", "likes"},
						},
						{
							TypeName:   "User",
							FieldNames: []string{"id"},
						},
					},
					Factory: federationFactory,
					Custom: ConfigJson(Configuration{
						Fetch: FetchConfiguration{
							URL: "http://review.service",
						},
						Federation: FederationConfiguration{
							Enabled:    true,
							ServiceSDL: "type Review { body: String! notes: String likes(filterToPublicOnly: Boolean): Int! } extend type User @key(fields: \"id\") { id: ID! @external reviews: [Review] }",
						},
					}),
				},
			},
			Fields: []plan.FieldConfiguration{
				{
					TypeName:       "User",
					FieldName:      "reviews",
					RequiresFields: []string{"id"},
				},
				{
					TypeName:  "Review",
					FieldName: "likes",
					Arguments: []plan.ArgumentConfiguration{
						{
							Name:       "filterToPublicOnly",
							SourceType: plan.FieldArgumentSource,
						},
					},
				},
			},
			Types: []plan.TypeConfiguration{
				{
					TypeName: "Boolean",
					RenameTo: "XBoolean",
				},
			},
		}))

	t.Run("federated entity with requires", RunTest(requiredFieldTestSchema,
		`	query QueryWithRequiredFields {
						serviceOne {
							serviceTwoFieldOne  # @requires(fields: "serviceOneFieldOne")
							serviceTwoFieldTwo  # @requires(fields: "serviceOneFieldTwo")
						}
					}`,
		"QueryWithRequiredFields",
		&plan.SynchronousResponsePlan{
			Response: &resolve.GraphQLResponse{
				Data: &resolve.Object{
					Fetch: &resolve.SingleFetch{
						BufferId: 0,
						// Should fetch the federation key as well as all the required fields.
						Input:                 `{"method":"POST","url":"http://one.service","body":{"query":"{serviceOne {id serviceOneFieldOne serviceOneFieldTwo}}"}}`,
						DataSource:            &Source{},
						DataSourceIdentifier:  []byte("graphql_datasource.Source"),
						ProcessResponseConfig: resolve.ProcessResponseConfig{ExtractGraphqlResponse: true},
					},
					Fields: []*resolve.Field{
						{
							HasBuffer: true,
							BufferID:  0,
							Name:      []byte("serviceOne"),
							Position: resolve.Position{
								Line:   2,
								Column: 7,
							},
							Value: &resolve.Object{
								Fetch: &resolve.BatchFetch{
									Fetch: &resolve.SingleFetch{
										BufferId: 1,
										// The required fields are present in the representations.
										Input: `{"method":"POST","url":"http://two.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on ServiceOneType {serviceTwoFieldOne serviceTwoFieldTwo}}}","variables":{"representations":[{"serviceOneFieldTwo":$$2$$,"serviceOneFieldOne":$$1$$,"id":$$0$$,"__typename":"ServiceOneType"}]}}}`,
										Variables: resolve.NewVariables(
											&resolve.ObjectVariable{
												Path:     []string{"id"},
												Renderer: resolve.NewJSONVariableRendererWithValidation(`{"type":["string","integer"]}`),
											},
											&resolve.ObjectVariable{
												Path:     []string{"serviceOneFieldOne"},
												Renderer: resolve.NewJSONVariableRendererWithValidation(`{"type":["string"]}`),
											},
											&resolve.ObjectVariable{
												Path:     []string{"serviceOneFieldTwo"},
												Renderer: resolve.NewJSONVariableRendererWithValidation(`{"type":["string"]}`),
											},
										),
										DataSource:           &Source{},
										DataSourceIdentifier: []byte("graphql_datasource.Source"),
										ProcessResponseConfig: resolve.ProcessResponseConfig{
											ExtractGraphqlResponse:    true,
											ExtractFederationEntities: true,
										},
									},
									BatchFactory: batchFactory,
								},
								Path:     []string{"serviceOne"},
								Nullable: true,
								Fields: []*resolve.Field{
									{
										HasBuffer: true,
										BufferID:  1,
										Name:      []byte("serviceTwoFieldOne"),
										Position: resolve.Position{
											Line:   3,
											Column: 8,
										},
										Value: &resolve.String{
											Path: []string{"serviceTwoFieldOne"},
										},
									},
									{
										HasBuffer: true,
										BufferID:  1,
										Name:      []byte("serviceTwoFieldTwo"),
										Position: resolve.Position{
											Line:   4,
											Column: 8,
										},
										Value: &resolve.String{
											Path: []string{"serviceTwoFieldTwo"},
										},
									},
								},
							},
						},
					},
				},
			},
		},
		plan.Configuration{
			DataSources: []plan.DataSourceConfiguration{
				{
					RootNodes: []plan.TypeField{
						{
							TypeName:   "Query",
							FieldNames: []string{"serviceOne"},
						},
					},
					ChildNodes: []plan.TypeField{
						{
							TypeName:   "ServiceOneType",
							FieldNames: []string{"id", "serviceOneFieldOne", "serviceOneFieldTwo"},
						},
					},
					Custom: ConfigJson(Configuration{
						Fetch: FetchConfiguration{
							URL: "http://one.service",
						},
						Federation: FederationConfiguration{
							Enabled:    true,
							ServiceSDL: "extend type Query {serviceOne: ServiceOneType} type ServiceOneType @key(fields: \"id\"){ id: ID! serviceOneFieldOne: String! serviceOneFieldTwo: String!}",
						},
					}),
					Factory: federationFactory,
				},
				{
					RootNodes: []plan.TypeField{
						{
							TypeName:   "ServiceOneType",
							FieldNames: []string{"serviceTwoFieldOne", "serviceTwoFieldTwo"},
						},
					},
					ChildNodes: []plan.TypeField{
						{
							TypeName:   "ServiceOneType",
							FieldNames: []string{"id", "serviceOneFieldOne", "serviceOneFieldTwo"},
						},
					},
					Custom: ConfigJson(Configuration{
						Fetch: FetchConfiguration{
							URL: "http://two.service",
						},
						Federation: FederationConfiguration{
							Enabled:    true,
							ServiceSDL: "extend type ServiceOneType @key(fields: \"id\") { id: ID! @external serviceOneFieldOne: String! @external serviceOneFieldTwo: String! @external serviceTwoFieldOne: String! @requires(fields: \"serviceOneFieldOne\") serviceTwoFieldTwo: String! @requires(fields: \"serviceOneFieldTwo\")}",
						},
					}),
					Factory: federationFactory,
				},
			},
			Fields: []plan.FieldConfiguration{
				{
					TypeName:       "ServiceOneType",
					FieldName:      "serviceTwoFieldOne",
					RequiresFields: []string{"id", "serviceOneFieldOne"},
				},
				{
					TypeName:       "ServiceOneType",
					FieldName:      "serviceTwoFieldTwo",
					RequiresFields: []string{"id", "serviceOneFieldTwo"},
				},
			},
		}))

	t.Run("federation with renamed schema", RunTest(renamedFederationTestSchema,
		`	query MyReviews {
						api_me {
							id
							username
							reviews {
								body
								author {
									id
									username
								}	
								product {
									name
									price
									reviews {
										body
										author {
											id
											username
										}
									}
								}
							}
						}
					}`,
		"MyReviews",
		&plan.SynchronousResponsePlan{
			Response: &resolve.GraphQLResponse{
				Data: &resolve.Object{
					Fetch: &resolve.SingleFetch{
						BufferId:              0,
						Input:                 `{"method":"POST","url":"http://user.service","body":{"query":"{api_me: me {id username}}"}}`,
						DataSource:            &Source{},
						DataSourceIdentifier:  []byte("graphql_datasource.Source"),
						ProcessResponseConfig: resolve.ProcessResponseConfig{ExtractGraphqlResponse: true},
					},
					Fields: []*resolve.Field{
						{
							HasBuffer: true,
							BufferID:  0,
							Name:      []byte("api_me"),
							Position: resolve.Position{
								Line:   2,
								Column: 7,
							},
							Value: &resolve.Object{
								Fetch: &resolve.BatchFetch{
									Fetch: &resolve.SingleFetch{
										BufferId: 1,
										Input:    `{"method":"POST","url":"http://review.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {reviews {body author {id username} product {upc}}}}}","variables":{"representations":[{"id":$$0$$,"__typename":"User"}]}}}`,
										Variables: resolve.NewVariables(
											&resolve.ObjectVariable{
												Path:     []string{"id"},
												Renderer: resolve.NewJSONVariableRendererWithValidation(`{"type":["string","integer"]}`),
											},
										),
										DataSource:           &Source{},
										DataSourceIdentifier: []byte("graphql_datasource.Source"),
										ProcessResponseConfig: resolve.ProcessResponseConfig{
											ExtractGraphqlResponse:    true,
											ExtractFederationEntities: true,
										},
									},
									BatchFactory: batchFactory,
								},
								Path:     []string{"api_me"},
								Nullable: true,
								Fields: []*resolve.Field{
									{
										Name: []byte("id"),
										Value: &resolve.String{
											Path: []string{"id"},
										},
										Position: resolve.Position{
											Line:   3,
											Column: 8,
										},
									},
									{
										Name: []byte("username"),
										Value: &resolve.String{
											Path: []string{"username"},
										},
										Position: resolve.Position{
											Line:   4,
											Column: 8,
										},
									},
									{
										HasBuffer: true,
										BufferID:  1,
										Name:      []byte("reviews"),
										Position: resolve.Position{
											Line:   5,
											Column: 8,
										},
										Value: &resolve.Array{
											Path:     []string{"reviews"},
											Nullable: true,
											Item: &resolve.Object{
												Nullable: true,
												Fields: []*resolve.Field{
													{
														Name: []byte("body"),
														Value: &resolve.String{
															Path: []string{"body"},
														},
														Position: resolve.Position{
															Line:   6,
															Column: 9,
														},
													},
													{
														Name: []byte("author"),
														Position: resolve.Position{
															Line:   7,
															Column: 9,
														},
														Value: &resolve.Object{
															Path: []string{"author"},
															Fields: []*resolve.Field{
																{
																	Name: []byte("id"),
																	Value: &resolve.String{
																		Path: []string{"id"},
																	},
																	Position: resolve.Position{
																		Line:   8,
																		Column: 10,
																	},
																},
																{
																	Name: []byte("username"),
																	Value: &resolve.String{
																		Path: []string{"username"},
																	},
																	Position: resolve.Position{
																		Line:   9,
																		Column: 10,
																	},
																},
															},
														},
													},
													{
														Name: []byte("product"),
														Position: resolve.Position{
															Line:   11,
															Column: 9,
														},
														Value: &resolve.Object{
															Path: []string{"product"},
															Fetch: &resolve.ParallelFetch{
																Fetches: []resolve.Fetch{
																	&resolve.BatchFetch{
																		Fetch: &resolve.SingleFetch{
																			BufferId:   2,
																			Input:      `{"method":"POST","url":"http://product.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Product {name price}}}","variables":{"representations":[{"upc":$$0$$,"__typename":"Product"}]}}}`,
																			DataSource: &Source{},
																			Variables: resolve.NewVariables(
																				&resolve.ObjectVariable{
																					Path:     []string{"upc"},
																					Renderer: resolve.NewJSONVariableRendererWithValidation(`{"type":["string"]}`),
																				},
																			),
																			DataSourceIdentifier: []byte("graphql_datasource.Source"),
																			ProcessResponseConfig: resolve.ProcessResponseConfig{
																				ExtractGraphqlResponse:    true,
																				ExtractFederationEntities: true,
																			},
																		},
																		BatchFactory: batchFactory,
																	},
																	&resolve.BatchFetch{
																		Fetch: &resolve.SingleFetch{
																			BufferId: 3,
																			Input:    `{"method":"POST","url":"http://review.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Product {reviews {body author {id username}}}}}","variables":{"representations":[{"upc":$$0$$,"__typename":"Product"}]}}}`,
																			Variables: resolve.NewVariables(
																				&resolve.ObjectVariable{
																					Path:     []string{"upc"},
																					Renderer: resolve.NewJSONVariableRendererWithValidation(`{"type":["string"]}`),
																				},
																			),
																			DataSource:           &Source{},
																			DataSourceIdentifier: []byte("graphql_datasource.Source"),
																			ProcessResponseConfig: resolve.ProcessResponseConfig{
																				ExtractGraphqlResponse:    true,
																				ExtractFederationEntities: true,
																			},
																		},
																		BatchFactory: batchFactory,
																	},
																},
															},
															Fields: []*resolve.Field{
																{
																	HasBuffer: true,
																	BufferID:  2,
																	Name:      []byte("name"),
																	Value: &resolve.String{
																		Path: []string{"name"},
																	},
																	Position: resolve.Position{
																		Line:   12,
																		Column: 10,
																	},
																},
																{
																	HasBuffer: true,
																	BufferID:  2,
																	Name:      []byte("price"),
																	Value: &resolve.Integer{
																		Path: []string{"price"},
																	},
																	Position: resolve.Position{
																		Line:   13,
																		Column: 10,
																	},
																},
																{
																	HasBuffer: true,
																	BufferID:  3,
																	Name:      []byte("reviews"),
																	Position: resolve.Position{
																		Line:   14,
																		Column: 10,
																	},
																	Value: &resolve.Array{
																		Nullable: true,
																		Path:     []string{"reviews"},
																		Item: &resolve.Object{
																			Nullable: true,
																			Fields: []*resolve.Field{
																				{
																					Name: []byte("body"),
																					Value: &resolve.String{
																						Path: []string{"body"},
																					},
																					Position: resolve.Position{
																						Line:   15,
																						Column: 11,
																					},
																				},
																				{
																					Name: []byte("author"),
																					Position: resolve.Position{
																						Line:   16,
																						Column: 11,
																					},
																					Value: &resolve.Object{
																						Path: []string{"author"},
																						Fields: []*resolve.Field{
																							{
																								Name: []byte("id"),
																								Value: &resolve.String{
																									Path: []string{"id"},
																								},
																								Position: resolve.Position{
																									Line:   17,
																									Column: 12,
																								},
																							},
																							{
																								Name: []byte("username"),
																								Value: &resolve.String{
																									Path: []string{"username"},
																								},
																								Position: resolve.Position{
																									Line:   18,
																									Column: 12,
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
		plan.Configuration{
			DataSources: []plan.DataSourceConfiguration{
				{
					RootNodes: []plan.TypeField{
						{
							TypeName:   "Query",
							FieldNames: []string{"api_me"},
						},
					},
					ChildNodes: []plan.TypeField{
						{
							TypeName:   "User_api",
							FieldNames: []string{"id", "username"},
						},
					},
					Custom: ConfigJson(Configuration{
						Fetch: FetchConfiguration{
							URL: "http://user.service",
						},
						Federation: FederationConfiguration{
							Enabled:    true,
							ServiceSDL: "extend type Query {me: User} type User @key(fields: \"id\"){ id: ID! username: String!}",
						},
						UpstreamSchema: federationTestSchema,
					}),
					Factory: federationFactory,
				},
				{
					RootNodes: []plan.TypeField{
						{
							TypeName:   "Query",
							FieldNames: []string{"api_topProducts"},
						},
						{
							TypeName:   "Subscription",
							FieldNames: []string{"api_updatedPrice"},
						},
						{
							TypeName:   "Product_api",
							FieldNames: []string{"upc", "name", "price"},
						},
					},
					ChildNodes: []plan.TypeField{
						{
							TypeName:   "Product_api",
							FieldNames: []string{"upc", "name", "price"},
						},
					},
					Custom: ConfigJson(Configuration{
						Fetch: FetchConfiguration{
							URL: "http://product.service",
						},
						Subscription: SubscriptionConfiguration{
							URL: "ws://product.service",
						},
						Federation: FederationConfiguration{
							Enabled:    true,
							ServiceSDL: "extend type Query {topProducts(first: Int = 5): [Product]} type Product @key(fields: \"upc\") {upc: String! price: Int!}",
						},
						UpstreamSchema: federationTestSchema,
					}),
					Factory: federationFactory,
				},
				{
					RootNodes: []plan.TypeField{
						{
							TypeName:   "User_api",
							FieldNames: []string{"reviews"},
						},
						{
							TypeName:   "Product_api",
							FieldNames: []string{"reviews"},
						},
					},
					ChildNodes: []plan.TypeField{
						{
							TypeName:   "Review_api",
							FieldNames: []string{"body", "author", "product"},
						},
						{
							TypeName:   "User_api",
							FieldNames: []string{"id", "username"},
						},
						{
							TypeName:   "Product_api",
							FieldNames: []string{"upc"},
						},
					},
					Factory: federationFactory,
					Custom: ConfigJson(Configuration{
						Fetch: FetchConfiguration{
							URL: "http://review.service",
						},
						Federation: FederationConfiguration{
							Enabled:    true,
							ServiceSDL: "type Review { body: String! author: User! @provides(fields: \"username\") product: Product! } extend type User @key(fields: \"id\") { id: ID! @external reviews: [Review] } extend type Product @key(fields: \"upc\") { upc: String! @external reviews: [Review] }",
						},
						UpstreamSchema: federationTestSchema,
					}),
				},
			},
			Fields: []plan.FieldConfiguration{
				{
					TypeName:  "Query",
					FieldName: "topProducts_api",
					Arguments: []plan.ArgumentConfiguration{
						{
							Name:       "first",
							SourceType: plan.FieldArgumentSource,
						},
					},
					Path: []string{"topProducts"},
				},
				{
					TypeName:  "Query",
					FieldName: "api_me",
					Path:      []string{"me"},
				},
				{
					TypeName:       "User_api",
					FieldName:      "reviews",
					RequiresFields: []string{"id"},
				},
				{
					TypeName:       "Product_api",
					FieldName:      "name",
					RequiresFields: []string{"upc"},
				},
				{
					TypeName:       "Product_api",
					FieldName:      "price",
					RequiresFields: []string{"upc"},
				},
				{
					TypeName:       "Product_api",
					FieldName:      "reviews",
					RequiresFields: []string{"upc"},
				},
			},
			Types: []plan.TypeConfiguration{
				{
					TypeName: "User_api",
					RenameTo: "User",
				},
				{
					TypeName: "Product_api",
					RenameTo: "Product",
				},
				{
					TypeName: "Review_api",
					RenameTo: "Review",
				},
			},
		}))
}

var errSubscriptionClientFail = errors.New("subscription client fail error")

type FailingSubscriptionClient struct{}

func (f FailingSubscriptionClient) Subscribe(ctx context.Context, options GraphQLSubscriptionOptions, next chan<- []byte) error {
	return errSubscriptionClientFail
}

func TestSubscriptionSource_Start(t *testing.T) {
	chatServer := httptest.NewServer(chat.GraphQLEndpointHandler())
	defer chatServer.Close()

	sendChatMessage := func(t *testing.T, username, message string) {
		time.Sleep(200 * time.Millisecond)
		httpClient := http.Client{}
		req, err := http.NewRequest(
			http.MethodPost,
			chatServer.URL,
			bytes.NewBufferString(fmt.Sprintf(`{"variables": {}, "operationName": "SendMessage", "query": "mutation SendMessage { post(roomName: \"#test\", username: \"%s\", text: \"%s\") { id } }"}`, username, message)),
		)
		require.NoError(t, err)

		req.Header.Set("Content-Type", "application/json")
		resp, err := httpClient.Do(req)
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, resp.StatusCode)
	}

	chatServerSubscriptionOptions := func(t *testing.T, body string) []byte {
		var gqlBody GraphQLBody
		_ = json.Unmarshal([]byte(body), &gqlBody)
		options := GraphQLSubscriptionOptions{
			URL:    chatServer.URL,
			Body:   gqlBody,
			Header: nil,
		}

		optionsBytes, err := json.Marshal(options)
		require.NoError(t, err)

		return optionsBytes
	}

	newSubscriptionSource := func(ctx context.Context) SubscriptionSource {
		httpClient := http.Client{}
		subscriptionSource := SubscriptionSource{client: NewWebSocketGraphQLSubscriptionClient(&httpClient, ctx)}
		return subscriptionSource
	}

	t.Run("should return error when input is invalid", func(t *testing.T) {
		source := SubscriptionSource{client: FailingSubscriptionClient{}}
		err := source.Start(context.Background(), []byte(`{"url": "", "body": "", "header": null}`), nil)
		assert.Error(t, err)
	})

	t.Run("should return error when subscription client returns an error", func(t *testing.T) {
		source := SubscriptionSource{client: FailingSubscriptionClient{}}
		err := source.Start(context.Background(), []byte(`{"url": "", "body": {}, "header": null}`), nil)
		assert.Error(t, err)
		assert.Equal(t, resolve.ErrUnableToResolve, err)
	})

	t.Run("invalid json: should stop before sending to upstream", func(t *testing.T) {
		next := make(chan []byte)
		ctx := context.Background()
		defer ctx.Done()

		source := newSubscriptionSource(ctx)
		chatSubscriptionOptions := chatServerSubscriptionOptions(t, `{"variables": {}, "extensions": {}, "operationName": "LiveMessages", "query": "subscription LiveMessages { messageAdded(roomName: "#test") { text createdBy } }"}`)
		err := source.Start(ctx, chatSubscriptionOptions, next)
		require.ErrorIs(t, err, resolve.ErrUnableToResolve)
	})

	t.Run("invalid syntax (roomNam)", func(t *testing.T) {
		next := make(chan []byte)
		ctx := context.Background()
		defer ctx.Done()

		source := newSubscriptionSource(ctx)
		chatSubscriptionOptions := chatServerSubscriptionOptions(t, `{"variables": {}, "extensions": {}, "operationName": "LiveMessages", "query": "subscription LiveMessages { messageAdded(roomNam: \"#test\") { text createdBy } }"}`)
		err := source.Start(ctx, chatSubscriptionOptions, next)
		require.NoError(t, err)

		msg, ok := <-next
		assert.True(t, ok)
		assert.Equal(t, `{"errors":[{"message":"Unknown argument \"roomNam\" on field \"messageAdded\" of type \"Subscription\". Did you mean \"roomName\"?","locations":[{"line":1,"column":29}],"extensions":{"code":"GRAPHQL_VALIDATION_FAILED"}},{"message":"Field \"messageAdded\" argument \"roomName\" of type \"String!\" is required but not provided.","locations":[{"line":1,"column":29}],"extensions":{"code":"GRAPHQL_VALIDATION_FAILED"}}]}`, string(msg))
		_, ok = <-next
		assert.False(t, ok)
	})

	t.Run("should close connection on stop message", func(t *testing.T) {
		next := make(chan []byte)
		subscriptionLifecycle, cancelSubscription := context.WithCancel(context.Background())
		resolverLifecycle, cancelResolver := context.WithCancel(context.Background())
		defer cancelResolver()

		source := newSubscriptionSource(resolverLifecycle)
		chatSubscriptionOptions := chatServerSubscriptionOptions(t, `{"variables": {}, "extensions": {}, "operationName": "LiveMessages", "query": "subscription LiveMessages { messageAdded(roomName: \"#test\") { text createdBy } }"}`)
		err := source.Start(subscriptionLifecycle, chatSubscriptionOptions, next)
		require.NoError(t, err)

		username := "myuser"
		message := "hello world!"
		go sendChatMessage(t, username, message)

		nextBytes := <-next
		assert.Equal(t, `{"data":{"messageAdded":{"text":"hello world!","createdBy":"myuser"}}}`, string(nextBytes))
		cancelSubscription()
		_, ok := <-next
		assert.False(t, ok)
	})

	t.Run("should successfully subscribe with chat example", func(t *testing.T) {
		next := make(chan []byte)
		ctx := context.Background()
		defer ctx.Done()

		source := newSubscriptionSource(ctx)
		chatSubscriptionOptions := chatServerSubscriptionOptions(t, `{"variables": {}, "extensions": {}, "operationName": "LiveMessages", "query": "subscription LiveMessages { messageAdded(roomName: \"#test\") { text createdBy } }"}`)
		err := source.Start(ctx, chatSubscriptionOptions, next)
		require.NoError(t, err)

		username := "myuser"
		message := "hello world!"
		go sendChatMessage(t, username, message)

		nextBytes := <-next
		assert.Equal(t, `{"data":{"messageAdded":{"text":"hello world!","createdBy":"myuser"}}}`, string(nextBytes))
	})
}

type _fakeDataSource struct {
	data              []byte
	artificialLatency time.Duration
}

func (f *_fakeDataSource) Load(ctx context.Context, input []byte, w io.Writer) (err error) {
	if f.artificialLatency != 0 {
		time.Sleep(f.artificialLatency)
	}
	_, err = w.Write(f.data)
	return
}

func FakeDataSource(data string) *_fakeDataSource {
	return &_fakeDataSource{
		data: []byte(data),
	}
}

type runTestOnTestDefinitionOptions func(planConfig *plan.Configuration, extraChecks *[]CheckFunc)

func testWithFactory(factory *Factory) runTestOnTestDefinitionOptions {
	return func(planConfig *plan.Configuration, extraChecks *[]CheckFunc) {
		for _, ds := range planConfig.DataSources {
			ds.Factory = factory
		}
	}
}

// nolint:deadcode,unused
func testWithExtraChecks(extraChecks ...CheckFunc) runTestOnTestDefinitionOptions {
	return func(planConfig *plan.Configuration, availableChecks *[]CheckFunc) {
		*availableChecks = append(*availableChecks, extraChecks...)
	}
}

func runTestOnTestDefinition(operation, operationName string, expectedPlan plan.Plan, options ...runTestOnTestDefinitionOptions) func(t *testing.T) {
	extraChecks := make([]CheckFunc, 0)
	config := plan.Configuration{
		DataSources: []plan.DataSourceConfiguration{
			{
				RootNodes: []plan.TypeField{
					{
						TypeName:   "Query",
						FieldNames: []string{"hero", "heroByBirthdate", "droid", "droids", "search", "stringList", "nestedStringList"},
					},
					{
						TypeName:   "Mutation",
						FieldNames: []string{"createReview"},
					},
					{
						TypeName:   "Subscription",
						FieldNames: []string{"remainingJedis"},
					},
				},
				ChildNodes: []plan.TypeField{
					{
						TypeName:   "Review",
						FieldNames: []string{"id", "stars", "commentary"},
					},
					{
						TypeName:   "Character",
						FieldNames: []string{"name", "friends"},
					},
					{
						TypeName:   "Human",
						FieldNames: []string{"name", "height", "friends"},
					},
					{
						TypeName:   "Droid",
						FieldNames: []string{"name", "primaryFunction", "friends"},
					},
					{
						TypeName:   "Starship",
						FieldNames: []string{"name", "length"},
					},
				},
				Custom: ConfigJson(Configuration{
					Fetch: FetchConfiguration{
						URL:    "https://swapi.com/graphql",
						Method: "POST",
					},
					Subscription: SubscriptionConfiguration{
						URL: "wss://swapi.com/graphql",
					},
				}),
				Factory: &Factory{},
			},
		},
		Fields: []plan.FieldConfiguration{
			{
				TypeName:  "Query",
				FieldName: "heroByBirthdate",
				Arguments: []plan.ArgumentConfiguration{
					{
						Name:       "birthdate",
						SourceType: plan.FieldArgumentSource,
					},
				},
			},
			{
				TypeName:  "Query",
				FieldName: "droid",
				Arguments: []plan.ArgumentConfiguration{
					{
						Name:       "id",
						SourceType: plan.FieldArgumentSource,
					},
				},
			},
			{
				TypeName:  "Query",
				FieldName: "droids",
				Arguments: []plan.ArgumentConfiguration{
					{
						Name:       "ids",
						SourceType: plan.FieldArgumentSource,
					},
				},
			},
			{
				TypeName:  "Query",
				FieldName: "search",
				Arguments: []plan.ArgumentConfiguration{
					{
						Name:       "name",
						SourceType: plan.FieldArgumentSource,
					},
				},
			},
		},
	}

	for _, opt := range options {
		opt(&config, &extraChecks)
	}

	return RunTest(testDefinition, operation, operationName, expectedPlan, config, extraChecks...)
}

func BenchmarkFederationBatching(b *testing.B) {
	userService := FakeDataSource(`{"data":{"me": {"id": "1234","username": "Me","__typename": "User"}}}`)
	reviewsService := FakeDataSource(`{"data":{"_entities":[{"reviews": [{"body": "A highly effective form of birth control.","product": {"upc": "top-1","__typename": "Product"}},{"body": "Fedoras are one of the most fashionable hats around and can look great with a variety of outfits.","product": {"upc": "top-2","__typename": "Product"}}]}]}}`)
	productsService := FakeDataSource(`{"data":{"_entities":[{"name": "Trilby"},{"name": "Fedora"}]}}`)

	reviewBatchFactory := NewBatchFactory()
	productBatchFactory := NewBatchFactory()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	resolver := resolve.New(ctx, resolve.NewFetcher(true), true)

	preparedPlan := &resolve.GraphQLResponse{
		Data: &resolve.Object{
			Fetch: &resolve.SingleFetch{
				BufferId: 0,
				InputTemplate: resolve.InputTemplate{
					Segments: []resolve.TemplateSegment{
						{
							Data:        []byte(`{"method":"POST","url":"http://localhost:4001","body":{"query":"{me {id username}}"}}`),
							SegmentType: resolve.StaticSegmentType,
						},
					},
				},
				DataSource: userService,
				ProcessResponseConfig: resolve.ProcessResponseConfig{
					ExtractGraphqlResponse: true,
				},
			},
			Fields: []*resolve.Field{
				{
					HasBuffer: true,
					BufferID:  0,
					Name:      []byte("me"),
					Value: &resolve.Object{
						Fetch: &resolve.BatchFetch{
							Fetch: &resolve.SingleFetch{
								BufferId: 1,
								InputTemplate: resolve.InputTemplate{
									Segments: []resolve.TemplateSegment{
										{
											Data:        []byte(`{"method":"POST","url":"http://localhost:4002","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {reviews {body product {upc __typename}}}}}","variables":{"representations":[{"id":"`),
											SegmentType: resolve.StaticSegmentType,
										},
										{
											SegmentType:        resolve.VariableSegmentType,
											VariableKind:       resolve.ObjectVariableKind,
											VariableSourcePath: []string{"id"},
											Renderer:           resolve.NewJSONVariableRendererWithValidation(`{"type":["number"]}`),
										},
										{
											Data:        []byte(`","__typename":"User"}]}}}`),
											SegmentType: resolve.StaticSegmentType,
										},
									},
								},
								DataSource: reviewsService,
								ProcessResponseConfig: resolve.ProcessResponseConfig{
									ExtractGraphqlResponse:    true,
									ExtractFederationEntities: true,
								},
							},
							BatchFactory: reviewBatchFactory,
						},
						Path:     []string{"me"},
						Nullable: true,
						Fields: []*resolve.Field{
							{
								Name: []byte("id"),
								Value: &resolve.String{
									Path: []string{"id"},
								},
							},
							{
								Name: []byte("username"),
								Value: &resolve.String{
									Path: []string{"username"},
								},
							},
							{

								HasBuffer: true,
								BufferID:  1,
								Name:      []byte("reviews"),
								Value: &resolve.Array{
									Path:     []string{"reviews"},
									Nullable: true,
									Item: &resolve.Object{
										Nullable: true,
										Fields: []*resolve.Field{
											{
												Name: []byte("body"),
												Value: &resolve.String{
													Path: []string{"body"},
												},
											},
											{
												Name: []byte("product"),
												Value: &resolve.Object{
													Path: []string{"product"},
													Fetch: &resolve.BatchFetch{
														Fetch: &resolve.SingleFetch{
															BufferId:   2,
															DataSource: productsService,
															InputTemplate: resolve.InputTemplate{
																Segments: []resolve.TemplateSegment{
																	{
																		Data:        []byte(`{"method":"POST","url":"http://localhost:4003","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Product {name}}}","variables":{"representations":[{"upc":`),
																		SegmentType: resolve.StaticSegmentType,
																	},
																	{
																		SegmentType:        resolve.VariableSegmentType,
																		VariableKind:       resolve.ObjectVariableKind,
																		VariableSourcePath: []string{"upc"},
																		Renderer:           resolve.NewJSONVariableRendererWithValidation(`{"type":["string"]}`),
																	},
																	{
																		Data:        []byte(`,"__typename":"Product"}]}}}`),
																		SegmentType: resolve.StaticSegmentType,
																	},
																},
															},
															ProcessResponseConfig: resolve.ProcessResponseConfig{
																ExtractGraphqlResponse:    true,
																ExtractFederationEntities: true,
															},
														},
														BatchFactory: productBatchFactory,
													},
													Fields: []*resolve.Field{
														{
															Name: []byte("upc"),
															Value: &resolve.String{
																Path: []string{"upc"},
															},
														},
														{
															HasBuffer: true,
															BufferID:  2,
															Name:      []byte("name"),
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
			},
		},
	}

	var err error
	expected := []byte(`{"data":{"me":{"id":"1234","username":"Me","reviews":[{"body":"A highly effective form of birth control.","product":{"upc":"top-1","name":"Trilby"}},{"body":"Fedoras are one of the most fashionable hats around and can look great with a variety of outfits.","product":{"upc":"top-2","name":"Fedora"}}]}}}`)

	pool := sync.Pool{
		New: func() interface{} {
			return bytes.NewBuffer(make([]byte, 0, 1024))
		},
	}

	ctxPool := sync.Pool{
		New: func() interface{} {
			return resolve.NewContext(context.Background())
		},
	}

	b.ReportAllocs()
	b.SetBytes(int64(len(expected)))
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			// _ = resolver.ResolveGraphQLResponse(ctx, plan, nil, ioutil.Discard)
			ctx := ctxPool.Get().(*resolve.Context)
			buf := pool.Get().(*bytes.Buffer)
			err = resolver.ResolveGraphQLResponse(ctx, preparedPlan, nil, buf)
			if err != nil {
				b.Fatal(err)
			}
			if !bytes.Equal(expected, buf.Bytes()) {
				b.Fatalf("want:\n%s\ngot:\n%s\n", string(expected), buf.String())
			}

			buf.Reset()
			pool.Put(buf)

			ctx.Free()
			ctxPool.Put(ctx)
		}
	})
}

const interfaceSelectionSchema = `

scalar String
scalar Boolean

schema {
	query: Query
}

type Query {
	user: User
}

interface User {
    id: ID!
    displayName: String!
    isLoggedIn: Boolean!
}

type RegisteredUser implements User {
    id: ID!
    displayName: String!
    isLoggedIn: Boolean!
	hasVerifiedEmail: Boolean!
}
`

const starWarsSchema = `
union SearchResult = Human | Droid | Starship

directive @format on FIELD
directive @onOperation on QUERY
directive @onVariable on VARIABLE_DEFINITION

schema {
    query: Query
    mutation: Mutation
    subscription: Subscription
}

type Query {
    hero: Character
    droid(id: ID!): Droid
    search(name: String!): SearchResult
    searchWithInput(input: SearchInput!): SearchResult
	stringList: [String]
	nestedStringList: [String]
}

input SearchInput {
	name: String
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

const starWarsSchemaWithExportDirective = `
union SearchResult = Human | Droid | Starship

directive @format on FIELD
directive @onOperation on QUERY
directive @onVariable on VARIABLE_DEFINITION

directive @export (
	as: String!
) on FIELD

schema {
    query: Query
    mutation: Mutation
    subscription: Subscription
}

type Query {
    hero: Character
    droid(id: ID!): Droid
    search(name: String!): SearchResult
    searchWithInput(input: SearchInput!): SearchResult
	stringList: [String]
	nestedStringList: [String]
}

input SearchInput {
	name: String
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

const renamedStarWarsSchema = `
union SearchResult_api = Human_api | Droid_api | Starship_api

directive @api_format on FIELD
directive @otherapi_undefined on QUERY
directive @api_onOperation on QUERY
directive @api_onVariable on VARIABLE_DEFINITION

scalar JSON_api

schema {
    query: Query
    mutation: Mutation
    subscription: Subscription
}

type Query {
    api_hero: Character_api
    api_droid(id: ID!): Droid_api
    api_search(name: String!): SearchResult_api
    api_searchWithInput(input: SearchInput_api!): SearchResult_api
	api_stringList: [String]
	api_nestedStringList: [String]
}

input SearchInput_api {
	name: String
	options: JSON_api
}

type Mutation {
	createReview(episode: Episode_api!, review: ReviewInput_api!): Review_api
}

type Subscription {
    remainingJedis: Int!
}

input ReviewInput_api {
    stars: Int!
    commentary: String
}

type Review_api {
    id: ID!
    stars: Int!
    commentary: String
}

enum Episode_api {
    NEWHOPE
    EMPIRE
    JEDI
}

interface Character_api {
    name: String!
    friends: [Character_api]
}

type Human_api implements Character_api {
    name: String!
    height: String!
    friends: [Character_api]
}

type Droid_api implements Character_api {
    name: String!
    primaryFunction: String!
    friends: [Character_api]
}

type Startship_api {
    name: String!
    length: Float!
}`

const todoSchema = `

schema {
	query: Query
	mutation: Mutation
}

scalar ID
scalar String
scalar Boolean

""""""
scalar DateTime

""""""
enum DgraphIndex {
  """"""
  int
  """"""
  float
  """"""
  bool
  """"""
  hash
  """"""
  exact
  """"""
  term
  """"""
  fulltext
  """"""
  trigram
  """"""
  regexp
  """"""
  year
  """"""
  month
  """"""
  day
  """"""
  hour
}

""""""
input DateTimeFilter {
  """"""
  eq: DateTime
  """"""
  le: DateTime
  """"""
  lt: DateTime
  """"""
  ge: DateTime
  """"""
  gt: DateTime
}

""""""
input StringHashFilter {
  """"""
  eq: String
}

""""""
type UpdateTaskPayload {
  """"""
  task(filter: TaskFilter, order: TaskOrder, first: Int, offset: Int): [Task]
  """"""
  numUids: Int
}

""""""
type Subscription {
  """"""
  getTask(id: ID!): Task
  """"""
  queryTask(filter: TaskFilter, order: TaskOrder, first: Int, offset: Int): [Task]
  """"""
  getUser(username: String!): User
  """"""
  queryUser(filter: UserFilter, order: UserOrder, first: Int, offset: Int): [User]
}

""""""
input FloatFilter {
  """"""
  eq: Float
  """"""
  le: Float
  """"""
  lt: Float
  """"""
  ge: Float
  """"""
  gt: Float
}

""""""
input StringTermFilter {
  """"""
  allofterms: String
  """"""
  anyofterms: String
}

""""""
type DeleteTaskPayload {
  """"""
  task(filter: TaskFilter, order: TaskOrder, first: Int, offset: Int): [Task]
  """"""
  msg: String
  """"""
  numUids: Int
}

""""""
type Mutation {
  """"""
  addTask(input: [AddTaskInput!]!): AddTaskPayload
  """"""
  updateTask(input: UpdateTaskInput!): UpdateTaskPayload
  """"""
  deleteTask(filter: TaskFilter!): DeleteTaskPayload
  """"""
  addUser(input: [AddUserInput!]!): AddUserPayload
  """"""
  updateUser(input: UpdateUserInput!): UpdateUserPayload
  """"""
  deleteUser(filter: UserFilter!): DeleteUserPayload
}

""""""
enum HTTPMethod {
  """"""
  GET
  """"""
  POST
  """"""
  PUT
  """"""
  PATCH
  """"""
  DELETE
}

""""""
type DeleteUserPayload {
  """"""
  user(filter: UserFilter, order: UserOrder, first: Int, offset: Int): [User]
  """"""
  msg: String
  """"""
  numUids: Int
}

""""""
input TaskFilter {
  """"""
  id: [ID!]
  """"""
  title: StringFullTextFilter
  """"""
  completed: Boolean
  """"""
  and: TaskFilter
  """"""
  or: TaskFilter
  """"""
  not: TaskFilter
}

""""""
type UpdateUserPayload {
  """"""
  user(filter: UserFilter, order: UserOrder, first: Int, offset: Int): [User]
  """"""
  numUids: Int
}

""""""
input TaskRef {
  """"""
  id: ID
  """"""
  title: String
  """"""
  completed: Boolean
  """"""
  user: UserRef
}

""""""
input UserFilter {
  """"""
  username: StringHashFilter
  """"""
  name: StringExactFilter
  """"""
  and: UserFilter
  """"""
  or: UserFilter
  """"""
  not: UserFilter
}

""""""
input UserOrder {
  """"""
  asc: UserOrderable
  """"""
  desc: UserOrderable
  """"""
  then: UserOrder
}

""""""
input AuthRule {
  """"""
  and: [AuthRule]
  """"""
  or: [AuthRule]
  """"""
  not: AuthRule
  """"""
  rule: String
}

""""""
type AddTaskPayload {
  """"""
  task(filter: TaskFilter, order: TaskOrder, first: Int, offset: Int): [Task]
  """"""
  numUids: Int
}

""""""
type AddUserPayload {
  """"""
  user(filter: UserFilter, order: UserOrder, first: Int, offset: Int): [User]
  """"""
  numUids: Int
}

""""""
type Task {
  """"""
  id: ID!
  """"""
  title: String!
  """"""
  completed: Boolean!
  """"""
  user(filter: UserFilter): User!
}

""""""
input IntFilter {
  """"""
  eq: Int
  """"""
  le: Int
  """"""
  lt: Int
  """"""
  ge: Int
  """"""
  gt: Int
}

""""""
input StringExactFilter {
  """"""
  eq: String
  """"""
  le: String
  """"""
  lt: String
  """"""
  ge: String
  """"""
  gt: String
}

""""""
enum UserOrderable {
  """"""
  username
  """"""
  name
}

""""""
input AddTaskInput {
  """"""
  title: String!
  """"""
  completed: Boolean!
  """"""
  user: UserRef!
}

""""""
input TaskPatch {
  """"""
  title: String
  """"""
  completed: Boolean
  """"""
  user: UserRef
}

""""""
input UserRef {
  """"""
  username: String
  """"""
  name: String
  """"""
  tasks: [TaskRef]
}

""""""
input StringFullTextFilter {
  """"""
  alloftext: String
  """"""
  anyoftext: String
}

""""""
enum TaskOrderable {
  """"""
  title
}

""""""
input UpdateTaskInput {
  """"""
  filter: TaskFilter!
  """"""
  set: TaskPatch
  """"""
  remove: TaskPatch
}

""""""
input UserPatch {
  """"""
  name: String
  """"""
  tasks: [TaskRef]
}

""""""
type Query {
  """"""
  getTask(id: ID!): Task
  """"""
  queryTask(filter: TaskFilter, order: TaskOrder, first: Int, offset: Int): [Task]
  """"""
  getUser(username: String!): User
  """"""
  queryUser(filter: UserFilter, order: UserOrder, first: Int, offset: Int): [User]
}

""""""
type User {
  """"""
  username: String!
  """"""
  name: String
  """"""
  tasks(filter: TaskFilter, order: TaskOrder, first: Int, offset: Int): [Task]
}

""""""
enum Mode {
  """"""
  BATCH
  """"""
  SINGLE
}

""""""
input CustomHTTP {
  """"""
  url: String!
  """"""
  method: HTTPMethod!
  """"""
  body: String
  """"""
  graphql: String
  """"""
  mode: Mode
  """"""
  forwardHeaders: [String!]
  """"""
  secretHeaders: [String!]
  """"""
  introspectionHeaders: [String!]
  """"""
  skipIntrospection: Boolean
}

""""""
input StringRegExpFilter {
  """"""
  regexp: String
}

""""""
input AddUserInput {
  """"""
  username: String!
  """"""
  name: String
  """"""
  tasks: [TaskRef]
}

""""""
input TaskOrder {
  """"""
  asc: TaskOrderable
  """"""
  desc: TaskOrderable
  """"""
  then: TaskOrder
}

""""""
input UpdateUserInput {
  """"""
  filter: UserFilter!
  """"""
  set: UserPatch
  """"""
  remove: UserPatch
}
"""
The @cache directive caches the response server side and sets cache control headers according to the configuration.
With this setting you can reduce the load on your backend systems for operations that get hit a lot while data doesn't change that frequently. 
"""
directive @cache(
  """maxAge defines the maximum time in seconds a response will be understood 'fresh', defaults to 300 (5 minutes)"""
  maxAge: Int! = 300
  """
  vary defines the headers to append to the cache key
  In addition to all possible headers you can also select a custom claim for authenticated requests
  Examples: 'jwt.sub', 'jwt.team' to vary the cache key based on 'sub' or 'team' fields on the jwt. 
  """
  vary: [String]! = []
) on QUERY

"""The @auth directive lets you configure auth for a given operation"""
directive @auth(
  """disable explicitly disables authentication for the annotated operation"""
  disable: Boolean! = false
) on QUERY | MUTATION | SUBSCRIPTION

"""The @fromClaim directive overrides a variable from a select claim in the jwt"""
directive @fromClaim(
  """
  name is the name of the claim you want to use for the variable
  examples: sub, team, custom.nested.claim
  """
  name: String!
) on VARIABLE_DEFINITION
`

const testDefinition = `
union SearchResult = Human | Droid | Starship
scalar Date

schema {
    query: Query
    mutation: Mutation
    subscription: Subscription
}

type Query {
    hero: Character
	heroByBirthdate(birthdate: Date): Character
    droid(id: ID!): Droid
	droids(ids: [ID!]!): [Droid]
    search(name: String!): SearchResult
	stringList: [String]
	nestedStringList: [String]
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

type Starship {
    name: String!
    length: Float!
}`

const federationTestSchema = `
scalar String
scalar Int
scalar ID

schema {
	query: Query
}

type Product {
  upc: String!
  name: String!
  price: Int!
  reviews: [Review]
}

type Query {
  me: User
  topProducts(first: Int = 5): [Product]
}

type Review {
  body: String!
  author: User!
  product: Product!
  notes: String
  likes(filterToPublicOnly: Boolean): Int!
}

type User {
  id: ID!
  username: String!
  reviews: [Review]
}
`

const renamedFederationTestSchema = `
scalar String
scalar Int
scalar ID

schema {
	query: Query
}

type Product_api {
  upc: String!
  name: String!
  price: Int!
  reviews: [Review_api]
}

type Query {
  api_me: User_api
  api_topProducts(first: Int = 5): [Product_api]
}

type Review_api {
  body: String!
  author: User_api!
  product: Product_api!
}

type User_api {
  id: ID!
  username: String!
  reviews: [Review_api]
}
`

const requiredFieldTestSchema = `
scalar String
scalar ID

schema {
	query: Query
}

type Query {
  serviceOne: ServiceOneType
}

type ServiceOneType {
  id: ID!
  serviceOneFieldOne: String!
  serviceOneFieldTwo: String!

  serviceTwoFieldOne: String!
  serviceTwoFieldTwo: String!
}
`

const subgraphTestSchema = `
directive @external on FIELD_DEFINITION
directive @requires(fields: _FieldSet!) on FIELD_DEFINITION
directive @provides(fields: _FieldSet!) on FIELD_DEFINITION
directive @key(fields: _FieldSet!) on OBJECT | INTERFACE
directive @extends on OBJECT

scalar _Any
union _Entity = Product | User
scalar _FieldSet

type _Service {
  sdl: String
}

type Entity {
  findProductByUpc(upc: String!): Product!
  findUserByID(id: ID!): User!
}

type Product {
  upc: String!
  reviews: [Review]
}

type Query {
  _entities(representations: [_Any!]!): [_Entity]!
  _service: _Service!
}

type Review {
  body: String!
  author: User!
  product: Product!
}

type User {
  id: ID!
  username: String!
  reviews: [Review]
}
`

const countriesSchema = `
scalar String
scalar Int
scalar ID

schema {
	query: Query
}

type Country {
  name: String!
  code: ID!
}

type Query {
  country(code: ID!): Country
  countryAlias(code: ID!): Country
}
`

const wgSchema = `union DeleteEnvironmentResponse = Success | Error

type Query {
  user: User
  edges: [Edge!]!
  admin_Config: AdminConfigResponse!
}

type NamespaceMemberRemoved {
  namespace: Namespace!
}

type NamespaceMemberAdded {
  namespace: Namespace!
}

union DeleteNamespaceResponse = Success | Error

enum Membership {
  owner
  maintainer
  viewer
  guest
}

input CreateAccessToken {
  name: String!
}

type ApiCreated {
  api: Api!
}

scalar Time

union NamespaceRemoveMemberResponse = NamespaceMemberRemoved | Error

enum EnvironmentKind {
  Personal
  Team
  Business
}

type Edge {
  id: ID!
  name: String!
  location: String!
}

type NamespaceCreated {
  namespace: Namespace!
}

union UpdateEnvironmentResponse = EnvironmentUpdated | Error

type Deployment {
  id: ID!
  name: String!
  config: JSON!
  environments: [Environment!]!
}

type Error {
  code: ErrorCode!
  message: String!
}

type Mutation {
  accessTokenCreate(input: CreateAccessToken!): CreateAccessTokenResponse!
  accessTokenDelete(input: DeleteAccessToken!): DeleteAccessTokenResponse!
  apiCreate(input: CreateApi!): CreateApiResponse!
  apiUpdate(input: UpdateApi!): UpdateApiResponse!
  apiDelete(input: DeleteApi!): DeleteApiResponse!
  deploymentCreateOrUpdate(input: CreateOrUpdateDeployment!): CreateOrUpdateDeploymentResponse!
  deploymentDelete(input: DeleteDeployment!): DeleteDeploymentResponse!
  environmentCreate(input: CreateEnvironment!): CreateEnvironmentResponse!
  environmentUpdate(input: UpdateEnvironment!): UpdateEnvironmentResponse!
  environmentDelete(input: DeleteEnvironment!): DeleteEnvironmentResponse!
  namespaceCreate(input: CreateNamespace!): CreateNamespaceResponse!
  namespaceDelete(input: DeleteNamespace!): DeleteNamespaceResponse!
  namespaceAddMember(input: NamespaceAddMember!): NamespaceAddMemberResponse!
  namespaceRemoveMember(input: NamespaceRemoveMember!): NamespaceRemoveMemberResponse!
  namespaceUpdateMembership(input: NamespaceUpdateMembership!): NamespaceUpdateMembershipResponse!
  admin_setWunderNodeImageTag(imageTag: String!): AdminConfigResponse!
}

type AccessToken {
  id: ID!
  name: String!
  createdAt: Time!
}

type EnvironmentCreated {
  environment: Environment!
}

type DeploymentUpdated {
  deployment: Deployment!
}

enum ErrorCode {
  Internal
  AuthenticationRequired
  Unauthorized
  NotFound
  Conflict
  UserAlreadyHasPersonalNamespace
  TeamPlanInPersonalNamespace
  InvalidName
  UnableToDeployEnvironment
  InvalidWunderGraphConfig
  ApiEnvironmentNamespaceMismatch
  UnableToUpdateEdgesOnPersonalEnvironment
}

input CreateEnvironment {
  namespace: ID!
  name: String
  primary: Boolean!
  kind: EnvironmentKind!
  edges: [ID!]
}

type Environment {
  id: ID!
  name: String
  primary: Boolean!
  kind: EnvironmentKind!
  edges: [Edge!]
  primaryHostName: String!
  hostNames: [String!]!
}

type DeploymentCreated {
  deployment: Deployment!
}

union AdminConfigResponse = Error | AdminConfig

input CreateNamespace {
  name: String!
  personal: Boolean!
}

input NamespaceUpdateMembership {
  namespaceID: ID!
  memberID: ID!
  newMembership: Membership!
}

union DeleteApiResponse = Success | Error

type ApiUpdated {
  api: Api!
}

input DeleteDeployment {
  deploymentID: ID!
}

input NamespaceRemoveMember {
  namespaceID: ID!
  memberID: ID!
}

union NamespaceUpdateMembershipResponse = NamespaceMembershipUpdated | Error

type User {
  id: ID!
  name: String!
  email: String!
  namespaces: [Namespace!]!
  accessTokens: [AccessToken!]!
}

input DeleteApi {
  id: ID!
}

type NamespaceMembershipUpdated {
  namespace: Namespace!
}

type EnvironmentUpdated {
  environment: Environment!
}

union CreateNamespaceResponse = NamespaceCreated | Error

type Namespace {
  id: ID!
  name: String!
  members: [Member!]!
  apis: [Api!]!
  environments: [Environment!]!
  personal: Boolean!
}

input UpdateEnvironment {
  environmentID: ID!
  edgeIDs: [ID!]
}

input DeleteEnvironment {
  environmentID: ID!
}

enum ApiVisibility {
  public
  private
  namespace
}

type Member {
  user: User!
  membership: Membership!
}

union DeleteAccessTokenResponse = Success | Error

input CreateApi {
  apiName: String!
  namespaceID: String!
  visibility: ApiVisibility!
  markdownDescription: String!
}

union CreateApiResponse = ApiCreated | Error

union CreateEnvironmentResponse = EnvironmentCreated | Error

union UpdateApiResponse = ApiUpdated | Error

input CreateOrUpdateDeployment {
  apiID: ID!
  name: String
  config: JSON!
  environmentIDs: [ID!]!
}

union CreateOrUpdateDeploymentResponse = DeploymentCreated | DeploymentUpdated | Error

union CreateAccessTokenResponse = AccessTokenCreated | Error

input DeleteAccessToken {
  id: ID!
}

type AdminConfig {
  WunderNodeImageTag: String!
}

input UpdateApi {
  id: ID!
  apiName: String!
  config: JSON!
  visibility: ApiVisibility!
  markdownDescription: String!
}

type Success {
  message: String!
}

scalar JSON

input NamespaceAddMember {
  namespaceID: ID!
  newMemberEmail: String!
  membership: Membership
}

input DeleteNamespace {
  namespaceID: ID!
}

type AccessTokenCreated {
  token: String!
  accessToken: AccessToken!
}

union NamespaceAddMemberResponse = NamespaceMemberAdded | Error

type Api {
  id: ID!
  name: String!
  visibility: ApiVisibility!
  deployments: [Deployment!]!
  markdownDescription: String!
}

union DeleteDeploymentResponse = Success | Error
`
