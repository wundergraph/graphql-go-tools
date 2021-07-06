package rest_datasource

import (
	"context"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/jensneuse/graphql-go-tools/pkg/ast"
	"github.com/jensneuse/graphql-go-tools/pkg/engine/datasource/httpclient"
	"github.com/jensneuse/graphql-go-tools/pkg/engine/datasourcetesting"
	"github.com/jensneuse/graphql-go-tools/pkg/engine/plan"
	"github.com/jensneuse/graphql-go-tools/pkg/engine/resolve"
)

const (
	schema = `
		type Query {
			friend: Friend
			withArgument(id: String!, name: String, optional: String): Friend
			withArrayArguments(names: [String]): Friend
		}

		type Subscription {
			friend: Friend
			withArgument(id: String!, name: String, optional: String): Friend
			withArrayArguments(names: [String]): Friend
		}

		type Friend {
			name: String
			pet: Pet
			phone(name: String!): String
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

	duplicatedArgumentOperationWithAlias = `
		query ArgumentQuery($idVariable: String!) {
			withArgument(id: $idVariable, name: "foo") {
				name
				homePhone: phone(name: "home")
				officePhone: phone(name: "office")
			}

			aliased: withArgument(id: $idVariable, name: "bar") {
				name
			}
		}
	`

	argumentWithoutVariablesOperation = `
		query ArgumentWithoutVariablesQuery {
			withArgument(id: "123abc", name: "foo") {
				name
			}
		}
	`

	argumentSubscription = `
		subscription ArgumentQuery($idVariable: String!) {
			withArgument(id: $idVariable, name: "foo") {
				name
			}
		}
	`

	arrayArgumentOperation = `
		query ArgumentQuery {
			withArrayArguments(names: ["foo","bar"]) {
				name
			}
		}
	`
)

func TestFastHttpJsonDataSourcePlanning(t *testing.T) {
	t.Run("get request", datasourcetesting.RunTest(schema, nestedOperation, "",
		&plan.SynchronousResponsePlan{
			Response: &resolve.GraphQLResponse{
				Data: &resolve.Object{
					Fetch: &resolve.SingleFetch{
						BufferId:   0,
						Input:      `{"method":"GET","url":"https://example.com/friend"}`,
						DataSource: &Source{},
					},
					Fields: []*resolve.Field{
						{
							BufferID:  0,
							HasBuffer: true,
							Name:      []byte("friend"),
							Value: &resolve.Object{
								Nullable: true,
								Fetch: &resolve.SingleFetch{
									BufferId:   1,
									Input:      `{"method":"GET","url":"https://example.com/friend/$$0$$/pet"}`,
									DataSource: &Source{},
									Variables: resolve.NewVariables(
										&resolve.ObjectVariable{
											Path: []string{"name"},
										},
									),
								},
								Fields: []*resolve.Field{
									{
										Name: []byte("name"),
										Value: &resolve.String{
											Path:     []string{"name"},
											Nullable: true,
										},
									},
									{
										HasBuffer: true,
										BufferID:  1,
										Name:      []byte("pet"),
										Value: &resolve.Object{
											Nullable: true,
											Fields: []*resolve.Field{
												{
													Name: []byte("id"),
													Value: &resolve.String{
														Path:     []string{"id"},
														Nullable: true,
													},
												},
												{
													Name: []byte("name"),
													Value: &resolve.String{
														Path:     []string{"name"},
														Nullable: true,
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
							FieldNames: []string{"friend"},
						},
					},
					Custom: ConfigJSON(Configuration{
						Fetch: FetchConfiguration{
							URL:    "https://example.com/friend",
							Method: "GET",
						},
					}),
					Factory: &Factory{},
				},
				{
					RootNodes: []plan.TypeField{
						{
							TypeName:   "Friend",
							FieldNames: []string{"pet"},
						},
					},
					Custom: ConfigJSON(Configuration{
						Fetch: FetchConfiguration{
							URL:    "https://example.com/friend/{{ .object.name }}/pet",
							Method: "GET",
						},
					}),
					Factory: &Factory{},
				},
			},
			Fields: []plan.FieldConfiguration{
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
			Response: &resolve.GraphQLResponse{
				Data: &resolve.Object{
					Fetch: &resolve.SingleFetch{
						BufferId:   0,
						Input:      `{"method":"GET","url":"https://example.com/$$0$$/$$1$$"}`,
						DataSource: &Source{},
						Variables: resolve.NewVariables(
							&resolve.ContextVariable{
								Path: []string{"idVariable"},
							},
							&resolve.ContextVariable{
								Path: []string{"a"},
							},
						),
					},
					Fields: []*resolve.Field{
						{
							BufferID:  0,
							HasBuffer: true,
							Name:      []byte("withArgument"),
							Value: &resolve.Object{
								Nullable: true,
								Fields: []*resolve.Field{
									{
										Name: []byte("name"),
										Value: &resolve.String{
											Path:     []string{"name"},
											Nullable: true,
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
							FieldNames: []string{"withArgument"},
						},
					},
					Custom: ConfigJSON(Configuration{
						Fetch: FetchConfiguration{
							URL:    "https://example.com/{{ .arguments.id }}/{{ .arguments.name }}",
							Method: "GET",
						},
					}),
					Factory: &Factory{},
				},
			},
			Fields: []plan.FieldConfiguration{
				{
					TypeName:              "Query",
					FieldName:             "withArgument",
					DisableDefaultMapping: true,
				},
			},
		},
	))
	t.Run("get request with duplicated argument and alias", datasourcetesting.RunTest(schema, duplicatedArgumentOperationWithAlias, "ArgumentQuery",
		&plan.SynchronousResponsePlan{
			Response: &resolve.GraphQLResponse{
				Data: &resolve.Object{
					Fetch: &resolve.ParallelFetch{
						Fetches: []resolve.Fetch{
							&resolve.SingleFetch{
								BufferId:   0,
								Input:      `{"method":"GET","url":"https://example.com/$$0$$/$$1$$"}`,
								DataSource: &Source{},
								Variables: resolve.NewVariables(
									&resolve.ContextVariable{
										Path: []string{"idVariable"},
									},
									&resolve.ContextVariable{
										Path: []string{"a"},
									},
								),
							},
							&resolve.SingleFetch{
								BufferId:   3,
								Input:      `{"method":"GET","url":"https://example.com/$$0$$/$$1$$"}`,
								DataSource: &Source{},
								Variables: resolve.NewVariables(
									&resolve.ContextVariable{
										Path: []string{"idVariable"},
									},
									&resolve.ContextVariable{
										Path: []string{"d"},
									},
								),
							},
						},
					},
					Fields: []*resolve.Field{
						{
							BufferID:  0,
							HasBuffer: true,
							Name:      []byte("withArgument"),
							Value: &resolve.Object{
								Nullable: true,
								Fetch: &resolve.ParallelFetch{
									Fetches: []resolve.Fetch{
										&resolve.SingleFetch{
											BufferId:   1,
											Input:      `{"method":"GET","url":"https://example.com/friends/phone/$$0$$"}`,
											DataSource: &Source{},
											Variables: resolve.NewVariables(
												&resolve.ContextVariable{
													Path: []string{"b"},
												},
											),
										},
										&resolve.SingleFetch{
											BufferId:   2,
											Input:      `{"method":"GET","url":"https://example.com/friends/phone/$$0$$"}`,
											DataSource: &Source{},
											Variables: resolve.NewVariables(
												&resolve.ContextVariable{
													Path: []string{"c"},
												},
											),
										},
									},
								},
								Fields: []*resolve.Field{
									{
										Name: []byte("name"),
										Value: &resolve.String{
											Path:     []string{"name"},
											Nullable: true,
										},
									},
									{
										BufferID:  1,
										HasBuffer: true,
										Name:      []byte("homePhone"),
										Value: &resolve.String{
											Path:     []string{"phone"},
											Nullable: true,
										},
									},
									{
										BufferID:  2,
										HasBuffer: true,
										Name:      []byte("officePhone"),
										Value: &resolve.String{
											Path:     []string{"phone"},
											Nullable: true,
										},
									},
								},
							},
						},
						{
							BufferID:  3,
							HasBuffer: true,
							Name:      []byte("aliased"),
							Value: &resolve.Object{
								Nullable: true,
								Fields: []*resolve.Field{
									{
										Name: []byte("name"),
										Value: &resolve.String{
											Path:     []string{"name"},
											Nullable: true,
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
							FieldNames: []string{"withArgument"},
						},
					},
					Custom: ConfigJSON(Configuration{
						Fetch: FetchConfiguration{
							URL:    "https://example.com/{{ .arguments.id }}/{{ .arguments.name }}",
							Method: "GET",
						},
					}),
					Factory: &Factory{},
				},
				{
					RootNodes: []plan.TypeField{
						{
							TypeName:   "Friend",
							FieldNames: []string{"phone"},
						},
					},
					Custom: ConfigJSON(Configuration{
						Fetch: FetchConfiguration{
							URL:    "https://example.com/friends/phone/{{ .arguments.name }}",
							Method: "GET",
						},
					}),
					Factory: &Factory{},
				},
			},
			Fields: []plan.FieldConfiguration{
				{
					TypeName:              "Query",
					FieldName:             "withArgument",
					DisableDefaultMapping: true,
				},
			},
		},
		func(t *testing.T, op ast.Document, actualPlan plan.Plan) {
			assert.Equal(t, `{"d":"bar","c":"office","b":"home","a":"foo"}`, string(op.Input.Variables))
		},
	))
	t.Run("get request with argument using templates with and without spaces", datasourcetesting.RunTest(schema, argumentWithoutVariablesOperation, "ArgumentWithoutVariablesQuery",
		&plan.SynchronousResponsePlan{
			Response: &resolve.GraphQLResponse{
				Data: &resolve.Object{
					Fetch: &resolve.SingleFetch{
						BufferId:   0,
						Input:      `{"method":"GET","url":"https://example.com/$$0$$/$$1$$"}`,
						DataSource: &Source{},
						Variables: resolve.NewVariables(
							&resolve.ContextVariable{
								Path: []string{"a"},
							},
							&resolve.ContextVariable{
								Path: []string{"b"},
							},
						),
					},
					Fields: []*resolve.Field{
						{
							BufferID:  0,
							HasBuffer: true,
							Name:      []byte("withArgument"),
							Value: &resolve.Object{
								Nullable: true,
								Fields: []*resolve.Field{
									{
										Name: []byte("name"),
										Value: &resolve.String{
											Path:     []string{"name"},
											Nullable: true,
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
							FieldNames: []string{"withArgument"},
						},
					},
					Custom: ConfigJSON(Configuration{
						Fetch: FetchConfiguration{
							URL:    "https://example.com/{{.arguments.id}}/{{   .arguments.name   }}",
							Method: "GET",
						},
					}),
					Factory: &Factory{},
				},
			},
			Fields: []plan.FieldConfiguration{
				{
					TypeName:              "Query",
					FieldName:             "withArgument",
					DisableDefaultMapping: true,
				},
			},
		},
	))
	t.Run("pulling subscription get request with argument", datasourcetesting.RunTest(schema, argumentSubscription, "ArgumentQuery",
		&plan.SubscriptionResponsePlan{
			Response: resolve.GraphQLSubscription{
				Trigger: resolve.GraphQLSubscriptionTrigger{
					Input:     `{"interval":1000,"request_input":{"method":"GET","url":"https://example.com/$$0$$/$$1$$"},"skip_publish_same_response":true}`,
					ManagerID: []byte("http_polling_stream"),
					Variables: resolve.NewVariables(
						&resolve.ContextVariable{
							Path: []string{"idVariable"},
						},
						&resolve.ContextVariable{
							Path: []string{"a"},
						},
					),
				},
				Response: &resolve.GraphQLResponse{
					Data: &resolve.Object{
						Fields: []*resolve.Field{
							{
								Name: []byte("withArgument"),
								Value: &resolve.Object{
									Nullable: true,
									Fields: []*resolve.Field{
										{
											Name: []byte("name"),
											Value: &resolve.String{
												Path:     []string{"name"},
												Nullable: true,
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
							TypeName:   "Subscription",
							FieldNames: []string{"withArgument"},
						},
					},
					Custom: ConfigJSON(Configuration{
						Fetch: FetchConfiguration{
							URL:    "https://example.com/{{ .arguments.id }}/{{ .arguments.name }}",
							Method: "GET",
						},
						Subscription: SubscriptionConfiguration{
							PollingIntervalMillis:   1000,
							SkipPublishSameResponse: true,
						},
					}),
					Factory: &Factory{},
				},
			},
			Fields: []plan.FieldConfiguration{
				{
					TypeName:              "Subscription",
					FieldName:             "withArgument",
					DisableDefaultMapping: true,
				},
			},
		},
	))
	t.Run("post request with body", datasourcetesting.RunTest(schema, simpleOperation, "",
		&plan.SynchronousResponsePlan{
			Response: &resolve.GraphQLResponse{
				Data: &resolve.Object{
					Fetch: &resolve.SingleFetch{
						BufferId:             0,
						Input:                `{"body":{"foo":"bar"},"method":"POST","url":"https://example.com/friend"}`,
						DataSource:           &Source{},
						DisallowSingleFlight: true,
					},
					Fields: []*resolve.Field{
						{
							BufferID:  0,
							HasBuffer: true,
							Name:      []byte("friend"),
							Value: &resolve.Object{
								Nullable: true,
								Fields: []*resolve.Field{
									{
										Name: []byte("name"),
										Value: &resolve.String{
											Path:     []string{"name"},
											Nullable: true,
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
							FieldNames: []string{"friend"},
						},
					},
					Custom: ConfigJSON(Configuration{
						Fetch: FetchConfiguration{
							URL:    "https://example.com/friend",
							Method: "POST",
							Body:   "{\"foo\":\"bar\"}",
						},
					}),
					Factory: &Factory{},
				},
			},
			Fields: []plan.FieldConfiguration{
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
			Response: &resolve.GraphQLResponse{
				Data: &resolve.Object{
					Fetch: &resolve.SingleFetch{
						BufferId:   0,
						Input:      `{"header":{"Authorization":["Bearer 123"],"Invalid-Template":["{{ request.headers.Authorization }}"],"Token":["Bearer $$0$$"],"X-API-Key":["456"]},"method":"GET","url":"https://example.com/friend"}`,
						DataSource: &Source{},
						Variables: []resolve.Variable{
							&resolve.HeaderVariable{
								Path: []string{"Authorization"},
							},
						},
					},
					Fields: []*resolve.Field{
						{
							BufferID:  0,
							HasBuffer: true,
							Name:      []byte("friend"),
							Value: &resolve.Object{
								Nullable: true,
								Fields: []*resolve.Field{
									{
										Name: []byte("name"),
										Value: &resolve.String{
											Path:     []string{"name"},
											Nullable: true,
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
							FieldNames: []string{"friend"},
						},
					},
					Custom: ConfigJSON(Configuration{
						Fetch: FetchConfiguration{
							URL:    "https://example.com/friend",
							Method: "GET",
							Header: http.Header{
								"Authorization":    []string{"Bearer 123"},
								"X-API-Key":        []string{"456"},
								"Token":            []string{"Bearer {{ .request.headers.Authorization }}"},
								"Invalid-Template": []string{"{{ request.headers.Authorization }}"},
							},
						},
					}),
					Factory: &Factory{},
				},
			},
			Fields: []plan.FieldConfiguration{
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
			Response: &resolve.GraphQLResponse{
				Data: &resolve.Object{
					Fetch: &resolve.SingleFetch{
						BufferId:   0,
						Input:      `{"query_params":[{"name":"static","value":"staticValue"},{"name":"static","value":"secondStaticValue"},{"name":"name","value":"$$0$$"},{"name":"id","value":"$$1$$"}],"method":"GET","url":"https://example.com/friend"}`,
						DataSource: &Source{},
						Variables: resolve.NewVariables(
							&resolve.ContextVariable{
								Path: []string{"a"},
							},
							&resolve.ContextVariable{
								Path: []string{"idVariable"},
							},
						),
					},
					Fields: []*resolve.Field{
						{
							BufferID:  0,
							HasBuffer: true,
							Name:      []byte("withArgument"),
							Value: &resolve.Object{
								Nullable: true,
								Fields: []*resolve.Field{
									{
										Name: []byte("name"),
										Value: &resolve.String{
											Path:     []string{"name"},
											Nullable: true,
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
							FieldNames: []string{"withArgument"},
						},
					},
					Custom: ConfigJSON(Configuration{
						Fetch: FetchConfiguration{
							URL:    "https://example.com/friend",
							Method: "GET",
							Query: []QueryConfiguration{
								{
									Name:  "static",
									Value: "staticValue",
								},
								{
									Name:  "static",
									Value: "secondStaticValue",
								},
								{
									Name:  "name",
									Value: "{{ .arguments.name }}",
								},
								{
									Name:  "id",
									Value: "{{ .arguments.id }}",
								},
								{
									Name:  "optional",
									Value: "{{ .arguments.optional }}",
								},
							},
						},
					}),
					Factory: &Factory{},
				},
			},
			Fields: []plan.FieldConfiguration{
				{
					TypeName:              "Query",
					FieldName:             "withArgument",
					DisableDefaultMapping: true,
				},
			},
		},
	))
	t.Run("get request with array query", datasourcetesting.RunTest(schema, arrayArgumentOperation, "ArgumentQuery",
		&plan.SynchronousResponsePlan{
			Response: &resolve.GraphQLResponse{
				Data: &resolve.Object{
					Fetch: &resolve.SingleFetch{
						BufferId:   0,
						Input:      `{"query_params":[{"name":"names","value":"$$0$$"}],"method":"GET","url":"https://example.com/friend"}`,
						DataSource: &Source{},
						Variables: resolve.NewVariables(
							&resolve.ContextVariable{
								Path: []string{"a"},
							},
						),
					},
					Fields: []*resolve.Field{
						{
							BufferID:  0,
							HasBuffer: true,
							Name:      []byte("withArrayArguments"),
							Value: &resolve.Object{
								Nullable: true,
								Fields: []*resolve.Field{
									{
										Name: []byte("name"),
										Value: &resolve.String{
											Path:     []string{"name"},
											Nullable: true,
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
							FieldNames: []string{"withArrayArguments"},
						},
					},
					Custom: ConfigJSON(Configuration{
						Fetch: FetchConfiguration{
							URL:    "https://example.com/friend",
							Method: "GET",
							Query: []QueryConfiguration{
								{
									Name:  "names",
									Value: "{{ .arguments.names }}",
								},
							},
						},
					}),
					Factory: &Factory{},
				},
			},
			Fields: []plan.FieldConfiguration{
				{
					TypeName:              "Query",
					FieldName:             "withArrayArguments",
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
				assert.Equal(t, []string{"one", "two"}, r.Header["Multi"])
				assert.Equal(t, "x,y", r.Header.Get("MultiComma"))

				_, _ = w.Write([]byte(`ok`))
			}))

			defer server.Close()

			input := []byte(fmt.Sprintf(`{"method":"GET","url":"%s","header":{"Multi":["one", "two"],"MultiComma":["x,y"],"Authorization":["Bearer 123"],"Token":["%s"],"X-API-Key":["%s"]}}`, server.URL, authorization, xApiKey))
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
			client: httpclient.NewNetHttpClient(httpclient.DefaultNetHttpClient),
		}
		runTests(t, source)
	})
	t.Run("fasthttp", func(t *testing.T) {
		source := &Source{
			client: httpclient.NewFastHttpClient(httpclient.DefaultFastHttpClient),
		}
		runTests(t, source)
	})
}
