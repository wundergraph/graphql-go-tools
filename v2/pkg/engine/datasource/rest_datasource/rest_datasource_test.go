package rest_datasource

import (
	"github.com/TykTechnologies/graphql-go-tools/v2/pkg/ast"
	"github.com/TykTechnologies/graphql-go-tools/v2/pkg/engine/datasourcetesting"
	"github.com/TykTechnologies/graphql-go-tools/v2/pkg/engine/plan"
	"github.com/TykTechnologies/graphql-go-tools/v2/pkg/engine/resolve"
	"github.com/buger/jsonparser"
	"github.com/stretchr/testify/assert"
	"net/http"
	"testing"
)

const (
	schema = `
		type Query {
			friend: Friend
			withArgument(id: String!, name: String, optional: String): Friend
			withArrayArguments(names: [String]): Friend
			withIntArgument(limit: Int): Friend
			withStringArgument(name: String!): Friend
		}

		type Subscription {
			friend: Friend
			withArgument(id: String!, name: String, optional: String): Friend
			withArrayArguments(names: [String]): Friend
		}

		input InputFriend {
			name: String!
		}

		type Mutation {
			createFriend(friend: InputFriend!): Friend
		}

		type Friend {
			name: String
			pet: Pet
			phone(name: String!): String
			hasArg(limit: Int!): String
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

	// nolint
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

	intArgumentOperation = `
		query ArgumentQuery {
			withIntArgument(limit: 10) {
				name
			}
		}
	`

	intArgumentOperationNonNullableInt = `
query ArgumentQuery($in: Int!) {
  			withIntArgument(limit: $in) {
    			name
  			}
		}
`

	stringArgumentOperationNonNullableString = `
query ArgumentQuery($in: String!) {
  			withStringArgument(name: $in) {
    			name
  			}
		}
`

	createFriendOperation = `
		mutation CreateFriend($friendVariable: InputFriend!) {
			createFriend(friend: $friendVariable) {
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
						FetchID: 0,
						FetchConfiguration: resolve.FetchConfiguration{
							Input:      `{"method":"GET","url":"https://example.com/friend"}`,
							DataSource: &Source{},
							//TODO check the extra fields of this type if they are needed
						},
						DataSourceIdentifier: []byte("rest_datasource.Source"),
					},
					Fields: []*resolve.Field{
						{
							Name: []byte("friend"),
							Value: &resolve.Object{
								Nullable: true,
								Fetch: &resolve.SingleFetch{
									FetchID: 1,
									FetchConfiguration: resolve.FetchConfiguration{
										Input:      `{"method":"GET","url":"https://example.com/friend/$$0$$/pet"}`,
										DataSource: &Source{},
										Variables: resolve.NewVariables(&resolve.ObjectVariable{
											Path:     []string{"name"},
											Renderer: resolve.NewPlainVariableRenderer(),
										}),
									},
									DataSourceIdentifier: []byte("rest_datasource.Source"),
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
										Name: []byte("pet"),
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
					ChildNodes: []plan.TypeField{
						{
							TypeName:   "Friend",
							FieldNames: []string{"name"},
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
					ChildNodes: []plan.TypeField{
						{
							TypeName:   "Pet",
							FieldNames: []string{"id", "name"},
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
			DisableResolveFieldPositions: true,
		},
	))
	t.Run("get request with argument", datasourcetesting.RunTest(schema, argumentOperation, "ArgumentQuery",
		&plan.SynchronousResponsePlan{
			Response: &resolve.GraphQLResponse{
				Data: &resolve.Object{
					Fetch: &resolve.SingleFetch{
						FetchConfiguration: resolve.FetchConfiguration{
							Input:      `{"method":"GET","url":"https://example.com/$$0$$/$$1$$"}`,
							DataSource: &Source{},
							Variables: resolve.NewVariables(
								&resolve.ContextVariable{
									Path:     []string{"idVariable"},
									Renderer: resolve.NewPlainVariableRendererWithValidation(`{"type":["string"]}`),
								},
								&resolve.ContextVariable{
									Path:     []string{"a"},
									Renderer: resolve.NewPlainVariableRendererWithValidation(`{"type":["string","null"]}`),
								}),
						},
						FetchID:              0,
						DataSourceIdentifier: []byte("rest_datasource.Source"),
					},
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
		plan.Configuration{
			DataSources: []plan.DataSourceConfiguration{
				{
					RootNodes: []plan.TypeField{
						{
							TypeName:   "Query",
							FieldNames: []string{"withArgument"},
						},
					},
					ChildNodes: []plan.TypeField{
						{
							TypeName:   "Friend",
							FieldNames: []string{"name"},
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
			DisableResolveFieldPositions: true,
		},
	))
	t.Run("mutation with nested argument", datasourcetesting.RunTest(schema, createFriendOperation, "CreateFriend",
		&plan.SynchronousResponsePlan{
			Response: &resolve.GraphQLResponse{
				Data: &resolve.Object{
					Fetch: &resolve.SingleFetch{
						FetchConfiguration: resolve.FetchConfiguration{
							Input:      `{"body":"{"friend":{"name":"$$0$$"}}","method":"POST","url":"https://example.com/$$0$$"}`,
							DataSource: &Source{},
							Variables: resolve.NewVariables(
								&resolve.ContextVariable{
									Path:     []string{"friend", "name"},
									Renderer: resolve.NewPlainVariableRendererWithValidation(`{"type":["string"]}`),
								},
							),
						},
						FetchID:              0,
						DataSourceIdentifier: []byte("rest_datasource.Source"),
					},
					Fields: []*resolve.Field{
						{
							Name: []byte("createFriend"),
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
							TypeName:   "Mutation",
							FieldNames: []string{"createFriend"},
						},
					},
					ChildNodes: []plan.TypeField{
						{
							TypeName:   "Friend",
							FieldNames: []string{"name"},
						},
					},
					Custom: ConfigJSON(Configuration{
						Fetch: FetchConfiguration{
							URL:    "https://example.com/{{ .arguments.friend.name }}",
							Method: "POST",
							Body:   "{\"friend\":{\"name\":\"{{ .arguments.friend.name }}\"}}",
						},
					}),
					Factory: &Factory{},
				},
			},
			Fields: []plan.FieldConfiguration{
				{
					TypeName:              "Mutation",
					FieldName:             "createFriend",
					DisableDefaultMapping: true,
				},
			},
			DisableResolveFieldPositions: true,
		},
	))
	t.Run("post request with nested JSON body", datasourcetesting.RunTest(authSchema, `
			mutation Login ($phoneNumber: String! $a: String) {
				Login: postPasswordlessStart(
					postPasswordlessStartInput: {
						applicationId: $a
						loginId: $phoneNumber
					}
				) {
					code
				}
			}
	`, "Login",
		&plan.SynchronousResponsePlan{
			Response: &resolve.GraphQLResponse{
				Data: &resolve.Object{
					Fetch: &resolve.SingleFetch{
						FetchConfiguration: resolve.FetchConfiguration{
							Input:      `{"body":{"applicationId":$$0$$,"loginId":$$1$$},"method":"POST","url":"https://example.com/passwordless_start"}`,
							DataSource: &Source{},
							Variables: resolve.NewVariables(
								&resolve.ContextVariable{
									Path:     []string{"a"},
									Renderer: resolve.NewJSONVariableRendererWithValidation(`{"type":["string","null"]}`),
								},
								&resolve.ContextVariable{
									Path:     []string{"phoneNumber"},
									Renderer: resolve.NewJSONVariableRendererWithValidation(`{"type":["string","null"]}`),
								},
							),
						},
						FetchID:              0,
						DataSourceIdentifier: []byte("rest_datasource.Source"),
					},
					Fields: []*resolve.Field{
						{
							Name: []byte("Login"),
							Value: &resolve.Object{
								Nullable: true,
								Fields: []*resolve.Field{
									{
										Name: []byte("code"),
										Value: &resolve.String{
											Path:     []string{"code"},
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
							TypeName:   "Mutation",
							FieldNames: []string{"postPasswordlessStart"},
						},
					},
					ChildNodes: []plan.TypeField{
						{
							TypeName:   "PostPasswordlessStart",
							FieldNames: []string{"code"},
						},
					},
					Custom: ConfigJSON(Configuration{
						Fetch: FetchConfiguration{
							URL:    "https://example.com/passwordless_start",
							Method: "POST",
							Body:   "{{ .arguments.postPasswordlessStartInput }}",
						},
					}),
					Factory: &Factory{},
				},
			},
			Fields: []plan.FieldConfiguration{
				{
					TypeName:              "Mutation",
					FieldName:             "postPasswordlessStart",
					DisableDefaultMapping: true,
				},
			},
			DisableResolveFieldPositions: true,
		},
	))
	t.Run("get request with duplicated argument and alias", datasourcetesting.RunTest(schema, duplicatedArgumentOperationWithAlias, "ArgumentQuery",
		&plan.SynchronousResponsePlan{
			Response: &resolve.GraphQLResponse{
				Data: &resolve.Object{
					Fetch: &resolve.MultiFetch{
						Fetches: []*resolve.SingleFetch{
							{
								FetchConfiguration: resolve.FetchConfiguration{
									Input:      `{"method":"GET","url":"https://example.com/$$0$$/$$1$$"}`,
									DataSource: &Source{},
									Variables: resolve.NewVariables(
										&resolve.ContextVariable{
											Path:     []string{"idVariable"},
											Renderer: resolve.NewPlainVariableRendererWithValidation(`{"type":["string"]}`),
										},
										&resolve.ContextVariable{
											Path:     []string{"a"},
											Renderer: resolve.NewPlainVariableRendererWithValidation(`{"type":["string","null"]}`),
										},
									),
								},
								FetchID:              0,
								DataSourceIdentifier: []byte("rest_datasource.Source"),
							},
							{
								FetchConfiguration: resolve.FetchConfiguration{
									Input:      `{"method":"GET","url":"https://example.com/$$0$$/$$1$$"}`,
									DataSource: &Source{},
									Variables: resolve.NewVariables(
										&resolve.ContextVariable{
											Path:     []string{"idVariable"},
											Renderer: resolve.NewPlainVariableRendererWithValidation(`{"type":["string"]}`),
										},
										&resolve.ContextVariable{
											Path:     []string{"d"},
											Renderer: resolve.NewPlainVariableRendererWithValidation(`{"type":["string","null"]}`),
										},
									),
								},
								FetchID:              3,
								DataSourceIdentifier: []byte("rest_datasource.Source"),
							},
						},
					},
					Fields: []*resolve.Field{
						{
							Name: []byte("withArgument"),
							Value: &resolve.Object{
								Nullable: true,
								Fetch: &resolve.MultiFetch{
									Fetches: []*resolve.SingleFetch{
										{
											FetchConfiguration: resolve.FetchConfiguration{
												Input:      `{"method":"GET","url":"https://example.com/friends/phone/$$0$$"}`,
												DataSource: &Source{},
												Variables: resolve.NewVariables(
													&resolve.ContextVariable{
														Path:     []string{"b"},
														Renderer: resolve.NewPlainVariableRendererWithValidation(`{"type":["string"]}`),
													},
												),
											},
											FetchID:              1,
											DataSourceIdentifier: []byte("rest_datasource.Source"),
										},
										{
											FetchID: 2,
											FetchConfiguration: resolve.FetchConfiguration{
												Input:      `{"method":"GET","url":"https://example.com/friends/phone/$$0$$"}`,
												DataSource: &Source{},
												Variables: resolve.NewVariables(
													&resolve.ContextVariable{
														Path:     []string{"c"},
														Renderer: resolve.NewPlainVariableRendererWithValidation(`{"type":["string"]}`),
													},
												),
											},
											DataSourceIdentifier: []byte("rest_datasource.Source"),
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
										Name: []byte("homePhone"),
										Value: &resolve.String{
											Path:     []string{"phone"},
											Nullable: true,
										},
									},
									{
										Name: []byte("officePhone"),
										Value: &resolve.String{
											Path:     []string{"phone"},
											Nullable: true,
										},
									},
								},
							},
						},
						{
							Name: []byte("aliased"),
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
					ChildNodes: []plan.TypeField{
						{
							TypeName:   "Friend",
							FieldNames: []string{"name"},
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
			DisableResolveFieldPositions: true,
		},
		datasourcetesting.WithCheckFuncs(
			func(t *testing.T, op ast.Document, actualPlan plan.Plan) {
				assert.Equal(t, `{"d":"bar","c":"office","b":"home","a":"foo"}`, string(op.Input.Variables))
			}),
	))
	t.Run("get request with argument using templates with and without spaces", datasourcetesting.RunTest(schema, argumentWithoutVariablesOperation, "ArgumentWithoutVariablesQuery",
		&plan.SynchronousResponsePlan{
			Response: &resolve.GraphQLResponse{
				Data: &resolve.Object{
					Fetch: &resolve.SingleFetch{
						FetchID: 0,
						FetchConfiguration: resolve.FetchConfiguration{
							Input:      `{"method":"GET","url":"https://example.com/$$0$$/$$1$$"}`,
							DataSource: &Source{},
							Variables: resolve.NewVariables(
								&resolve.ContextVariable{
									Path:     []string{"a"},
									Renderer: resolve.NewPlainVariableRendererWithValidation(`{"type":["string"]}`),
								},
								&resolve.ContextVariable{
									Path:     []string{"b"},
									Renderer: resolve.NewPlainVariableRendererWithValidation(`{"type":["string","null"]}`),
								},
							),
						},
						DataSourceIdentifier: []byte("rest_datasource.Source"),
					},
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
		plan.Configuration{
			DataSources: []plan.DataSourceConfiguration{
				{
					RootNodes: []plan.TypeField{
						{
							TypeName:   "Query",
							FieldNames: []string{"withArgument"},
						},
					},
					ChildNodes: []plan.TypeField{
						{
							TypeName:   "Friend",
							FieldNames: []string{"name"},
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
			DisableResolveFieldPositions: true,
		},
	))
	t.Run("post request with body", datasourcetesting.RunTest(schema, simpleOperation, "",
		&plan.SynchronousResponsePlan{
			Response: &resolve.GraphQLResponse{
				Data: &resolve.Object{
					Fetch: &resolve.SingleFetch{
						FetchID: 0,
						FetchConfiguration: resolve.FetchConfiguration{
							Input:      `{"body":{"foo":"bar"},"method":"POST","url":"https://example.com/friend"}`,
							DataSource: &Source{},
						},
						DataSourceIdentifier: []byte("rest_datasource.Source"),
					},
					Fields: []*resolve.Field{
						{
							Name: []byte("friend"),
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
					ChildNodes: []plan.TypeField{
						{
							TypeName:   "Friend",
							FieldNames: []string{"name"},
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
			DisableResolveFieldPositions: true,
		},
	))
	t.Run("get request with headers", datasourcetesting.RunTest(schema, simpleOperation, "",
		&plan.SynchronousResponsePlan{
			Response: &resolve.GraphQLResponse{
				Data: &resolve.Object{
					Fetch: &resolve.SingleFetch{
						FetchID: 0,
						FetchConfiguration: resolve.FetchConfiguration{
							Input:      `{"header":{"Authorization":["Bearer 123"],"Invalid-Template":["{{ request.headers.Authorization }}"],"Token":["Bearer $$0$$"],"X-API-Key":["456"]},"method":"GET","url":"https://example.com/friend"}`,
							DataSource: &Source{},
							Variables: []resolve.Variable{
								&resolve.HeaderVariable{
									Path: []string{"Authorization"},
								},
							},
						},
						DataSourceIdentifier: []byte("rest_datasource.Source"),
					},
					Fields: []*resolve.Field{
						{
							Name: []byte("friend"),
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
					ChildNodes: []plan.TypeField{
						{
							TypeName:   "Friend",
							FieldNames: []string{"name"},
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
			DisableResolveFieldPositions: true,
		},
	))
	t.Run("get request with query", datasourcetesting.RunTest(schema, argumentOperation, "ArgumentQuery",
		&plan.SynchronousResponsePlan{
			Response: &resolve.GraphQLResponse{
				Data: &resolve.Object{
					Fetch: &resolve.SingleFetch{
						FetchID: 0,
						FetchConfiguration: resolve.FetchConfiguration{
							Input:      `{"query_params":[{"name":"static","value":"staticValue"},{"name":"static","value":"secondStaticValue"},{"name":"name","value":"$$0$$"},{"name":"id","value":"$$1$$"}],"method":"GET","url":"https://example.com/friend"}`,
							DataSource: &Source{},
							Variables: resolve.NewVariables(
								&resolve.ContextVariable{
									Path:     []string{"a"},
									Renderer: resolve.NewPlainVariableRendererWithValidation(`{"type":["string","null"]}`),
								},
								&resolve.ContextVariable{
									Path:     []string{"idVariable"},
									Renderer: resolve.NewPlainVariableRendererWithValidation(`{"type":["string"]}`),
								},
							),
						},
						DataSourceIdentifier: []byte("rest_datasource.Source"),
					},
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
		plan.Configuration{
			DataSources: []plan.DataSourceConfiguration{
				{
					RootNodes: []plan.TypeField{
						{
							TypeName:   "Query",
							FieldNames: []string{"withArgument"},
						},
					},
					ChildNodes: []plan.TypeField{
						{
							TypeName:   "Friend",
							FieldNames: []string{"name"},
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
					TypeName:  "Query",
					FieldName: "withArgument",
					Arguments: []plan.ArgumentConfiguration{
						{
							Name:       "id",
							SourceType: plan.FieldArgumentSource,
						},
						{
							Name:       "input",
							SourceType: plan.FieldArgumentSource,
						},
					},
					DisableDefaultMapping: true,
				},
			},
			DisableResolveFieldPositions: true,
		},
	))
	t.Run("get request with array query", datasourcetesting.RunTest(schema, arrayArgumentOperation, "ArgumentQuery",
		&plan.SynchronousResponsePlan{
			Response: &resolve.GraphQLResponse{
				Data: &resolve.Object{
					Fetch: &resolve.SingleFetch{
						FetchID: 0,
						FetchConfiguration: resolve.FetchConfiguration{
							Input:      `{"query_params":[{"name":"names","value":$$0$$}],"method":"GET","url":"https://example.com/friend"}`,
							DataSource: &Source{},
							Variables: resolve.NewVariables(
								&resolve.ContextVariable{
									Path:     []string{"a"},
									Renderer: resolve.NewPlainVariableRendererWithValidation(`{"type":["array","null"],"items":{"type":["string","null"]}}`),
								},
							),
						},
						DataSourceIdentifier: []byte("rest_datasource.Source"),
					},
					Fields: []*resolve.Field{
						{
							Name: []byte("withArrayArguments"),
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
					ChildNodes: []plan.TypeField{
						{
							TypeName:   "Friend",
							FieldNames: []string{"name"},
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
			DisableResolveFieldPositions: true,
		},
	))
	t.Run("get request with int argument query param", datasourcetesting.RunTest(schema, intArgumentOperation, "ArgumentQuery",
		&plan.SynchronousResponsePlan{
			Response: &resolve.GraphQLResponse{
				Data: &resolve.Object{
					Fetch: &resolve.SingleFetch{
						FetchID: 0,
						FetchConfiguration: resolve.FetchConfiguration{
							Input:      `{"query_params":[{"name":"limit","value":$$0$$}],"method":"GET","url":"https://example.com/friend"}`,
							DataSource: &Source{},
							Variables: resolve.NewVariables(
								&resolve.ContextVariable{
									Path:     []string{"a"},
									Renderer: resolve.NewPlainVariableRendererWithValidation(`{"type":["integer","null"]}`),
								},
							),
						},
						DataSourceIdentifier: []byte("rest_datasource.Source"),
					},
					Fields: []*resolve.Field{
						{
							Name: []byte("withIntArgument"),
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
							FieldNames: []string{"withIntArgument"},
						},
					},
					ChildNodes: []plan.TypeField{
						{
							TypeName:   "Friend",
							FieldNames: []string{"name"},
						},
					},
					Custom: ConfigJSON(Configuration{
						Fetch: FetchConfiguration{
							URL:    "https://example.com/friend",
							Method: "GET",
							Query: []QueryConfiguration{
								{
									Name:  "limit",
									Value: "{{ .arguments.limit }}",
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
					FieldName:             "withIntArgument",
					DisableDefaultMapping: true,
				},
			},
			DisableResolveFieldPositions: true,
		},
	))
	t.Run("get request with non null int as query param", datasourcetesting.RunTest(schema, intArgumentOperationNonNullableInt, "ArgumentQuery",
		&plan.SynchronousResponsePlan{
			Response: &resolve.GraphQLResponse{
				Data: &resolve.Object{
					Fetch: &resolve.SingleFetch{
						FetchID: 0,
						FetchConfiguration: resolve.FetchConfiguration{
							Input:      `{"query_params":[{"name":"limit","value":$$0$$}],"method":"GET","url":"https://example.com/friend"}`,
							DataSource: &Source{},
							Variables: resolve.NewVariables(
								&resolve.ContextVariable{
									Path:     []string{"in"},
									Renderer: resolve.NewPlainVariableRendererWithValidation(`{"type":["integer"]}`),
								},
							),
						},
						DataSourceIdentifier: []byte("rest_datasource.Source"),
					},
					Fields: []*resolve.Field{
						{
							Name: []byte("withIntArgument"),
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
							FieldNames: []string{"withIntArgument"},
						},
					},
					ChildNodes: []plan.TypeField{
						{
							TypeName:   "Friend",
							FieldNames: []string{"name"},
						},
					},
					Custom: ConfigJSON(Configuration{
						Fetch: FetchConfiguration{
							URL:    "https://example.com/friend",
							Method: "GET",
							Query: []QueryConfiguration{
								{
									Name:  "limit",
									Value: "{{ .arguments.limit }}",
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
					FieldName:             "withIntArgument",
					DisableDefaultMapping: true,
				},
			},
			DisableResolveFieldPositions: true,
		},
	))
	t.Run("get request with non null string as query param", datasourcetesting.RunTest(schema, stringArgumentOperationNonNullableString, "ArgumentQuery",
		&plan.SynchronousResponsePlan{
			Response: &resolve.GraphQLResponse{
				Data: &resolve.Object{
					Fetch: &resolve.SingleFetch{
						FetchID: 0,
						FetchConfiguration: resolve.FetchConfiguration{
							Input:      `{"query_params":[{"name":"name","value":"$$0$$"}],"method":"GET","url":"https://example.com/friend"}`,
							DataSource: &Source{},
							Variables: resolve.NewVariables(
								&resolve.ContextVariable{
									Path:     []string{"in"},
									Renderer: resolve.NewPlainVariableRendererWithValidation(`{"type":["string"]}`),
								},
							),
						},
						DataSourceIdentifier: []byte("rest_datasource.Source"),
					},
					Fields: []*resolve.Field{
						{
							Name: []byte("withStringArgument"),
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
							FieldNames: []string{"withStringArgument"},
						},
					},
					ChildNodes: []plan.TypeField{
						{
							TypeName:   "Friend",
							FieldNames: []string{"name"},
						},
					},
					Custom: ConfigJSON(Configuration{
						Fetch: FetchConfiguration{
							URL:    "https://example.com/friend",
							Method: "GET",
							Query: []QueryConfiguration{
								{
									Name:  "name",
									Value: "{{ .arguments.name }}",
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
					FieldName:             "withStringArgument",
					DisableDefaultMapping: true,
				},
			},
			DisableResolveFieldPositions: true,
		},
	))
	t.Run("get request with array query", datasourcetesting.RunTest(schema, arrayArgumentOperation, "ArgumentQuery",
		&plan.SynchronousResponsePlan{
			Response: &resolve.GraphQLResponse{
				Data: &resolve.Object{
					Fetch: &resolve.SingleFetch{
						FetchID: 0,
						FetchConfiguration: resolve.FetchConfiguration{
							Input:      `{"method":"GET","url":"https://example.com/friend/$$0$$"}`,
							DataSource: &Source{},
							Variables: resolve.NewVariables(
								&resolve.ContextVariable{
									Path:     []string{"a"},
									Renderer: resolve.NewCSVVariableRenderer(resolve.JsonRootType{Value: jsonparser.String}),
								},
							),
						},
						DataSourceIdentifier: []byte("rest_datasource.Source"),
					},
					Fields: []*resolve.Field{
						{
							Name: []byte("withArrayArguments"),
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
					ChildNodes: []plan.TypeField{
						{
							TypeName:   "Friend",
							FieldNames: []string{"name"},
						},
					},
					Custom: ConfigJSON(Configuration{
						Fetch: FetchConfiguration{
							URL:    "https://example.com/friend/{{ .arguments.names }}",
							Method: "GET",
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
					Arguments: []plan.ArgumentConfiguration{
						{
							Name:         "names",
							RenderConfig: plan.RenderArgumentAsArrayCSV,
						},
					},
				},
			},
			DisableResolveFieldPositions: true,
		},
	))
}

//func TestHttpJsonDataSource_Load(t *testing.T) {
//	runTests := func(t *testing.T, source *Source) {
//		t.Run("simple get", func(t *testing.T) {
//			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
//				assert.Equal(t, r.Method, http.MethodGet)
//				_, _ = w.Write([]byte(`ok`))
//			}))
//
//			defer server.Close()
//
//			input := []byte(fmt.Sprintf(`{"method":"GET","url":"%s"}`, server.URL))
//			b := &strings.Builder{}
//			require.NoError(t, source.Load(context.Background(), input, b))
//			assert.Equal(t, `ok`, b.String())
//		})
//		t.Run("get with query parameters", func(t *testing.T) {
//			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
//				assert.Equal(t, r.Method, http.MethodGet)
//				fooQueryParam := r.URL.Query().Get("foo")
//				assert.Equal(t, fooQueryParam, "bar")
//				doubleQueryParam := r.URL.Query()["double"]
//				assert.Len(t, doubleQueryParam, 2)
//				assert.Equal(t, "first", doubleQueryParam[0])
//				assert.Equal(t, "second", doubleQueryParam[1])
//				_, _ = w.Write([]byte(`ok`))
//			}))
//
//			defer server.Close()
//
//			input := []byte(fmt.Sprintf(`{"query_params":[{"name":"foo","value":"bar"},{"name":"double","value":"first"},{"name":"double","value":"second"}],"method":"GET","url":"%s"}`, server.URL))
//			b := &strings.Builder{}
//			require.NoError(t, source.Load(context.Background(), input, b))
//			assert.Equal(t, `ok`, b.String())
//		})
//		t.Run("get with headers", func(t *testing.T) {
//			authorization := "Bearer 123"
//			xApiKey := "456"
//
//			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
//				assert.Equal(t, r.Method, http.MethodGet)
//				assert.Equal(t, authorization, r.Header.Get("Authorization"))
//				assert.Equal(t, xApiKey, r.Header.Get("X-API-KEY"))
//				assert.Equal(t, []string{"one", "two"}, r.Header["Multi"])
//				assert.Equal(t, "x,y", r.Header.Get("MultiComma"))
//				_, notExists := r.Header[http.CanonicalHeaderKey("NotExists")]
//				assert.False(t, notExists)
//
//				_, _ = w.Write([]byte(`ok`))
//			}))
//
//			defer server.Close()
//
//			input := []byte(fmt.Sprintf(`{"method":"GET","url":"%s","header":{"Multi":["one", "two"],"MultiComma":["x,y"],"Authorization":["Bearer 123"],"Token":["%s"],"X-API-Key":["%s"],"NotExists":[""]}}`, server.URL, authorization, xApiKey))
//			b := &strings.Builder{}
//			require.NoError(t, source.Load(context.Background(), input, b))
//			assert.Equal(t, `ok`, b.String())
//		})
//		t.Run("post with body", func(t *testing.T) {
//			body := `{"foo":"bar"}`
//
//			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
//				assert.Equal(t, http.MethodPost, r.Method)
//				actualBody, err := io.ReadAll(r.Body)
//				assert.NoError(t, err)
//				assert.Equal(t, string(actualBody), body)
//				_, _ = w.Write([]byte(`ok`))
//			}))
//
//			defer server.Close()
//
//			input := []byte(fmt.Sprintf(`{"method":"POST","url":"%s","body":%s}`, server.URL, body))
//			b := &strings.Builder{}
//			require.NoError(t, source.Load(context.Background(), input, b))
//			assert.Equal(t, `ok`, b.String())
//		})
//	}
//
//	t.Run("net/http", func(t *testing.T) {
//		source := &Source{
//			client: http.DefaultClient,
//		}
//		runTests(t, source)
//	})
//}

const authSchema = `
type Mutation {
  postPasswordlessStart(postPasswordlessStartInput: postPasswordlessStartInput): PostPasswordlessStart
  postPasswordlessStartList(postPasswordlessStartInput: [postPasswordlessStartInput]): PostPasswordlessStart
  postPasswordlessLogin(postPasswordlessLoginInput: postPasswordlessLoginInput): PostPasswordlessLogin
}

type PostPasswordlessStart {
  code: String
}

input postPasswordlessStartInput {
  applicationId: String
  loginId: String
  nested: postPasswordlessStartInput
}

type PostPasswordlessLogin {
  refreshToken: String
  token: String
  user: User
}

type User {
  username: String
  verified: Boolean
  firstName: String
  lastName: String
  email: String
  mobilePhone: String
  timezone: String
}

input postPasswordlessLoginInput {
  code: String
  ipAddress: String
  metaData: MetaDataInput
}

input MetaDataInput {
  device: DeviceInput
}

input DeviceInput {
  name: String
}
`
