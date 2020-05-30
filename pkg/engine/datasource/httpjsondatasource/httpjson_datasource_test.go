package httpjsondatasource

import (
	"context"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/jensneuse/graphql-go-tools/pkg/engine/datasource"
	"github.com/jensneuse/graphql-go-tools/pkg/engine/datasourcetesting"
	"github.com/jensneuse/graphql-go-tools/pkg/engine/plan"
	"github.com/jensneuse/graphql-go-tools/pkg/engine/resolve"
)

const (
	schema = `
		type Query {
			friend: Friend
			withArgument(id: String!, name: String): Friend
		}

		type Friend {
			name: String
			pet: Pet
		}

		type Pet {
			id: String
			name: String
		}
	`

	simpleOperation = `
		query {
			friend {
				name
			}
		}
	`
	nestedOperation = `
		query {
			friend {
				name
				pet {
					id
					name
				}
			}
		}
	`

	argumentOperation = `
		query ArgumentQuery($idVariable: String!) {
			withArgument(id: $idVariable, name: "foo") {
				name
			}
		}
	`
)

func TestFastHttpJsonDataSourcePlanning(t *testing.T) {
	t.Run("get request", datasourcetesting.RunTest(schema, nestedOperation, "",
		&plan.SynchronousResponsePlan{
			Response: resolve.GraphQLResponse{
				Data: &resolve.Object{
					Fetch: &resolve.SingleFetch{
						BufferId: 0,
						Input:    []byte(`{"method":"GET","url":"https://example.com/friend"}`),
						DataSource: &Source{
							client: NewPlanner(nil).clientOrDefault(),
						},
					},
					FieldSets: []resolve.FieldSet{
						{
							BufferID:  0,
							HasBuffer: true,
							Fields: []resolve.Field{
								{
									Name: []byte("friend"),
									Value: &resolve.Object{
										Fetch: &resolve.SingleFetch{
											BufferId: 1,
											Input:    []byte(`{"method":"GET","url":"https://example.com/friend/$$0$$/pet"}`),
											DataSource: &Source{
												client: NewPlanner(nil).clientOrDefault(),
											},
											Variables: resolve.NewVariables(
												&resolve.ObjectVariable{
													Path: []string{"name"},
												},
											),
										},
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
											{
												HasBuffer: true,
												BufferID:  1,
												Fields: []resolve.Field{
													{
														Name: []byte("pet"),
														Value: &resolve.Object{
															FieldSets: []resolve.FieldSet{
																{
																	Fields: []resolve.Field{
																		{
																			Name: []byte("id"),
																			Value: &resolve.String{
																				Path: []string{"id"},
																			},
																		},
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
							},
						},
					},
				},
			},
		},
		plan.Configuration{
			DataSourceConfigurations: []plan.DataSourceConfiguration{
				{
					TypeName:   "Query",
					FieldNames: []string{"friend"},
					Attributes: []plan.DataSourceAttribute{
						{
							Key:   "path",
							Value: []byte("https://example.com/friend"),
						},
						{
							Key:   "method",
							Value: []byte("GET"),
						},
					},
					DataSourcePlanner: &Planner{},
				},
				{
					TypeName:   "Friend",
					FieldNames: []string{"pet"},
					Attributes: []plan.DataSourceAttribute{
						{
							Key:   "path",
							Value: []byte("https://example.com/friend/{{ .object.name }}/pet"),
						},
						{
							Key:   "method",
							Value: []byte("GET"),
						},
					},
					DataSourcePlanner: &Planner{},
				},
			},
			FieldMappings: []plan.FieldMapping{
				{
					TypeName:              "Query",
					FieldName:             "friend",
					DisableDefaultMapping: true,
				},
				{
					TypeName:              "Friend",
					FieldName:             "pet",
					DisableDefaultMapping: true,
				},
			},
		},
	))
	t.Run("get request with argument", datasourcetesting.RunTest(schema, argumentOperation, "ArgumentQuery",
		&plan.SynchronousResponsePlan{
			Response: resolve.GraphQLResponse{
				Data: &resolve.Object{
					Fetch: &resolve.SingleFetch{
						BufferId: 0,
						Input:    []byte(`{"method":"GET","url":"https://example.com/$$0$$/foo"}`),
						DataSource: &Source{
							client: NewPlanner(nil).clientOrDefault(),
						},
						Variables: resolve.NewVariables(
							&resolve.ContextVariable{
								Path: []string{"idVariable"},
							},
						),
					},
					FieldSets: []resolve.FieldSet{
						{
							BufferID:  0,
							HasBuffer: true,
							Fields: []resolve.Field{
								{
									Name: []byte("withArgument"),
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
		},
		plan.Configuration{
			DataSourceConfigurations: []plan.DataSourceConfiguration{
				{
					TypeName:   "Query",
					FieldNames: []string{"withArgument"},
					Attributes: []plan.DataSourceAttribute{
						{
							Key:   "path",
							Value: []byte("https://example.com/{{ .arguments.id }}/{{ .arguments.name }}"),
						},
						{
							Key:   "method",
							Value: []byte("GET"),
						},
					},
					DataSourcePlanner: &Planner{},
				},
			},
			FieldMappings: []plan.FieldMapping{
				{
					TypeName:              "Query",
					FieldName:             "withArgument",
					DisableDefaultMapping: true,
				},
			},
		},
	))
	t.Run("post request with body", datasourcetesting.RunTest(schema, simpleOperation, "",
		&plan.SynchronousResponsePlan{
			Response: resolve.GraphQLResponse{
				Data: &resolve.Object{
					Fetch: &resolve.SingleFetch{
						BufferId: 0,
						Input:    []byte(`{"body":{"foo":"bar"},"method":"POST","url":"https://example.com/friend"}`),
						DataSource: &Source{
							client: NewPlanner(nil).clientOrDefault(),
						},
						Variables:            resolve.Variables{},
						DisallowSingleFlight: true,
					},
					FieldSets: []resolve.FieldSet{
						{
							BufferID:  0,
							HasBuffer: true,
							Fields: []resolve.Field{
								{
									Name: []byte("friend"),
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
		},
		plan.Configuration{
			DataSourceConfigurations: []plan.DataSourceConfiguration{
				{
					TypeName:   "Query",
					FieldNames: []string{"friend"},
					Attributes: []plan.DataSourceAttribute{
						{
							Key:   "base_url",
							Value: []byte("https://example.com"),
						},
						{
							Key:   "path",
							Value: []byte("/friend"),
						},
						{
							Key:   "method",
							Value: []byte("POST"),
						},
						{
							Key:   "body",
							Value: []byte(`{"foo":"bar"}`),
						},
					},
					DataSourcePlanner: &Planner{},
				},
			},
			FieldMappings: []plan.FieldMapping{
				{
					TypeName:              "Query",
					FieldName:             "friend",
					DisableDefaultMapping: true,
				},
			},
		},
	))
	t.Run("get request with headers", datasourcetesting.RunTest(schema, simpleOperation, "",
		&plan.SynchronousResponsePlan{
			Response: resolve.GraphQLResponse{
				Data: &resolve.Object{
					Fetch: &resolve.SingleFetch{
						BufferId: 0,
						Input:    []byte(`{"headers":{"Authorization":"Bearer 123","X-API-Key":"456"},"method":"GET","url":"https://example.com/friend"}`),
						DataSource: &Source{
							client: NewPlanner(nil).clientOrDefault(),
						},
						Variables: resolve.Variables{},
					},
					FieldSets: []resolve.FieldSet{
						{
							BufferID:  0,
							HasBuffer: true,
							Fields: []resolve.Field{
								{
									Name: []byte("friend"),
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
		},
		plan.Configuration{
			DataSourceConfigurations: []plan.DataSourceConfiguration{
				{
					TypeName:   "Query",
					FieldNames: []string{"friend"},
					Attributes: []plan.DataSourceAttribute{
						{
							Key:   "base_url",
							Value: []byte("https://example.com"),
						},
						{
							Key:   "path",
							Value: []byte("/friend"),
						},
						{
							Key:   "method",
							Value: []byte("GET"),
						},
						{
							Key:   "headers",
							Value: []byte(`{"Authorization":"Bearer 123","X-API-Key":"456"}`),
						},
					},
					DataSourcePlanner: &Planner{},
				},
			},
			FieldMappings: []plan.FieldMapping{
				{
					TypeName:              "Query",
					FieldName:             "friend",
					DisableDefaultMapping: true,
				},
			},
		},
	))
	t.Run("get request with query", datasourcetesting.RunTest(schema, argumentOperation, "ArgumentQuery",
		&plan.SynchronousResponsePlan{
			Response: resolve.GraphQLResponse{
				Data: &resolve.Object{
					Fetch: &resolve.SingleFetch{
						BufferId: 0,
						Input:    []byte(`{"query_params":[{"name":"static","value":"staticValue"},{"name":"name","value":"foo"},{"name":"id","value":"$$0$$"}],"method":"GET","url":"https://example.com/friend"}`),
						DataSource: &Source{
							client: NewPlanner(nil).clientOrDefault(),
						},
						Variables: resolve.NewVariables(
							&resolve.ContextVariable{
								Path: []string{"idVariable"},
							},
						),
					},
					FieldSets: []resolve.FieldSet{
						{
							BufferID:  0,
							HasBuffer: true,
							Fields: []resolve.Field{
								{
									Name: []byte("withArgument"),
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
		},
		plan.Configuration{
			DataSourceConfigurations: []plan.DataSourceConfiguration{
				{
					TypeName:   "Query",
					FieldNames: []string{"withArgument"},
					Attributes: []plan.DataSourceAttribute{
						{
							Key:   "base_url",
							Value: []byte("https://example.com"),
						},
						{
							Key:   "path",
							Value: []byte("/friend"),
						},
						{
							Key:   "method",
							Value: []byte("GET"),
						},
						{
							Key: "query_params",
							Value: NewQueryValues(
								QueryValue{
									Name:  "static",
									Value: "staticValue",
								},
								QueryValue{
									Name:  "name",
									Value: "{{ .arguments.name }}",
								},
								QueryValue{
									Name:  "id",
									Value: "{{ .arguments.id }}",
								},
							),
						},
					},
					DataSourcePlanner: &Planner{},
				},
			},
			FieldMappings: []plan.FieldMapping{
				{
					TypeName:              "Query",
					FieldName:             "withArgument",
					DisableDefaultMapping: true,
				},
			},
		},
	))
}

func TestHttpJsonDataSource_Load(t *testing.T) {

	runTests := func(t *testing.T, source *Source) {
		t.Run("simple get", func(t *testing.T) {

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, r.Method, http.MethodGet)
				_, _ = w.Write([]byte(`ok`))
			}))

			defer server.Close()

			input := []byte(fmt.Sprintf(`{"method":"GET","url":"%s"}`, server.URL))
			pair := resolve.NewBufPair()
			err := source.Load(context.Background(), input, pair)
			assert.NoError(t, err)
			assert.Equal(t, `ok`, pair.Data.String())
		})
		t.Run("get with query parameters", func(t *testing.T) {

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, r.Method, http.MethodGet)
				fooQueryParam := r.URL.Query().Get("foo")
				assert.Equal(t, fooQueryParam, "bar")
				doubleQueryParam := r.URL.Query()["double"]
				assert.Len(t, doubleQueryParam, 2)
				assert.Equal(t, "first", doubleQueryParam[0])
				assert.Equal(t, "second", doubleQueryParam[1])
				_, _ = w.Write([]byte(`ok`))
			}))

			defer server.Close()

			input := []byte(fmt.Sprintf(`{"query_params":[{"name":"foo","value":"bar"},{"name":"double","value":"first"},{"name":"double","value":"second"}],"method":"GET","url":"%s"}`, server.URL))
			pair := resolve.NewBufPair()
			err := source.Load(context.Background(), input, pair)
			assert.NoError(t, err)
			assert.Equal(t, `ok`, pair.Data.String())
		})
		t.Run("get with headers", func(t *testing.T) {

			authorization := "Bearer 123"
			xApiKey := "456"

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, r.Method, http.MethodGet)
				assert.Equal(t, authorization, r.Header.Get("Authorization"))
				assert.Equal(t, xApiKey, r.Header.Get("X-API-KEY"))
				_, _ = w.Write([]byte(`ok`))
			}))

			defer server.Close()

			input := []byte(fmt.Sprintf(`{"method":"GET","url":"%s","headers":{"Authorization":"%s","X-API-KEY":"%s"}}`, server.URL, authorization, xApiKey))
			pair := resolve.NewBufPair()
			err := source.Load(context.Background(), input, pair)
			assert.NoError(t, err)
			assert.Equal(t, `ok`, pair.Data.String())
		})
		t.Run("post with body", func(t *testing.T) {

			body := `{"foo":"bar"}`

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, http.MethodPost, r.Method)
				actualBody, err := ioutil.ReadAll(r.Body)
				assert.NoError(t, err)
				assert.Equal(t, string(actualBody), body)
				_, _ = w.Write([]byte(`ok`))
			}))

			defer server.Close()

			input := []byte(fmt.Sprintf(`{"method":"POST","url":"%s","body":%s}`, server.URL, body))
			pair := resolve.NewBufPair()
			err := source.Load(context.Background(), input, pair)
			assert.NoError(t, err)
			assert.Equal(t, `ok`, pair.Data.String())
		})
	}

	t.Run("net/http", func(t *testing.T) {
		source := &Source{
			client: datasource.NewNetHttpClient(datasource.DefaultNetHttpClient),
		}
		runTests(t, source)
	})
	t.Run("fasthttp", func(t *testing.T) {
		source := &Source{
			client: datasource.NewFastHttpClient(datasource.DefaultFastHttpClient),
		}
		runTests(t, source)
	})
}
