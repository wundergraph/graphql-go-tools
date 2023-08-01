package graphql

import (
	"bytes"
	"context"
	"encoding/json"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/jensneuse/abstractlogger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/pkg/execution/datasource"
	"github.com/wundergraph/graphql-go-tools/pkg/starwars"
)

type testRoundTripper func(req *http.Request) *http.Response

func (t testRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return t(req), nil
}

type roundTripperTestCase struct {
	expectedHost     string
	expectedPath     string
	expectedBody     string
	sendStatusCode   int
	sendResponseBody string
}

func createTestRoundTripper(t *testing.T, testCase roundTripperTestCase) testRoundTripper {
	return func(req *http.Request) *http.Response {
		assert.Equal(t, testCase.expectedHost, req.URL.Host)
		assert.Equal(t, testCase.expectedPath, req.URL.Path)

		if len(testCase.expectedBody) > 0 {
			var receivedBodyBytes []byte
			if req.Body != nil {
				var err error
				receivedBodyBytes, err = ioutil.ReadAll(req.Body)
				require.NoError(t, err)
			}
			require.Equal(t, testCase.expectedBody, string(receivedBodyBytes), "roundTripperTestCase body do not match")
		}

		body := bytes.NewBuffer([]byte(testCase.sendResponseBody))
		return &http.Response{StatusCode: testCase.sendStatusCode, Body: ioutil.NopCloser(body)}
	}
}

func TestExecutionEngine_ExecuteWithOptions(t *testing.T) {
	type testCase struct {
		schema            *Schema
		request           func(t *testing.T) Request
		plannerConfig     datasource.PlannerConfiguration
		roundTripper      testRoundTripper
		preExecutionTasks func(t *testing.T, request Request, schema *Schema, engine *ExecutionEngine) // optional
		expectedResponse  string
	}

	run := func(tc testCase, hasExecutionError bool) func(t *testing.T) {
		return func(t *testing.T) {
			request := tc.request(t)

			extraVariables := map[string]string{
				"request": `{"Authorization": "Bearer ey123"}`,
			}
			extraVariablesBytes, err := json.Marshal(extraVariables)
			require.NoError(t, err)

			engine, err := NewExecutionEngine(abstractlogger.NoopLogger, tc.schema, tc.plannerConfig)
			assert.NoError(t, err)

			switch tc.plannerConfig.TypeFieldConfigurations[0].DataSource.Name {
			case "HttpJsonDataSource":
				httpJsonOptions := DataSourceHttpJsonOptions{
					HttpClient: &http.Client{
						Transport: tc.roundTripper,
					},
				}

				err = engine.AddHttpJsonDataSourceWithOptions("HttpJsonDataSource", httpJsonOptions)
				assert.NoError(t, err)
			case "GraphqlDataSource":
				graphqlOptions := DataSourceGraphqlOptions{
					HttpClient: &http.Client{
						Transport: tc.roundTripper,
					},
				}

				err = engine.AddGraphqlDataSourceWithOptions("GraphqlDataSource", graphqlOptions)
				assert.NoError(t, err)
			}

			if tc.preExecutionTasks != nil {
				tc.preExecutionTasks(t, request, tc.schema, engine)
			}

			executionRes, err := engine.Execute(context.Background(), &request, ExecutionOptions{ExtraArguments: extraVariablesBytes})

			if hasExecutionError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			assert.Equal(t, tc.expectedResponse, executionRes.Buffer().String())
		}
	}

	runWithoutError := func(tc testCase) func(t *testing.T) {
		return run(tc, false)
	}

	runWithError := func(tc testCase) func(t *testing.T) {
		return run(tc, true)
	}

	t.Run("execute with custom roundtripper for simple hero query on HttpJsonDatasource", runWithoutError(testCase{
		schema:        starwarsSchema(t),
		request:       loadStarWarsQuery(starwars.FileSimpleHeroQuery, nil),
		plannerConfig: heroHttpJsonPlannerConfig,
		roundTripper: createTestRoundTripper(t, roundTripperTestCase{
			expectedHost:     "example.com",
			expectedPath:     "/",
			expectedBody:     "",
			sendResponseBody: `{"hero": {"name": "Luke Skywalker"}}`,
			sendStatusCode:   200,
		}),
		expectedResponse: `{"data":{"hero":{"name":"Luke Skywalker"}}}`,
	}))

	t.Run("execute with empty request object should not panic", runWithError(testCase{
		schema: starwarsSchema(t),
		request: func(t *testing.T) Request {
			return Request{}
		},
		plannerConfig: heroHttpJsonPlannerConfig,
		roundTripper: createTestRoundTripper(t, roundTripperTestCase{
			expectedHost:     "example.com",
			expectedPath:     "/",
			expectedBody:     "",
			sendResponseBody: `{"hero": {"name": "Luke Skywalker"}}`,
			sendStatusCode:   200,
		}),
		expectedResponse: "",
	}))

	t.Run("execute with custom roundtripper for simple hero query on GraphqlDataSource", runWithoutError(testCase{
		schema:        starwarsSchema(t),
		request:       loadStarWarsQuery(starwars.FileSimpleHeroQuery, nil),
		plannerConfig: heroGraphqlDataSource,
		roundTripper: createTestRoundTripper(t, roundTripperTestCase{
			expectedHost:     "example.com",
			expectedPath:     "/",
			expectedBody:     "",
			sendResponseBody: `{"data":{"hero":{"name":"Luke Skywalker"}}}`,
			sendStatusCode:   200,
		}),
		expectedResponse: `{"data":{"hero":{"name":"Luke Skywalker"}}}`,
	}))

	t.Run("execute the correct query when sending multiple queries", runWithoutError(testCase{
		schema: starwarsSchema(t),
		request: func(t *testing.T) Request {
			request := loadStarWarsQuery(starwars.FileMultiQueries, nil)(t)
			request.OperationName = "SingleHero"
			return request
		},
		plannerConfig: heroGraphqlDataSource,
		roundTripper: createTestRoundTripper(t, roundTripperTestCase{
			expectedHost:     "example.com",
			expectedPath:     "/",
			expectedBody:     "",
			sendResponseBody: `{"data":{"hero":{"name":"Luke Skywalker"}}}`,
			sendStatusCode:   200,
		}),
		expectedResponse: `{"data":{"hero":{"name":"Luke Skywalker"}}}`,
	}))

	t.Run("execute query with variables for arguments", runWithoutError(testCase{
		schema:        starwarsSchema(t),
		request:       loadStarWarsQuery(starwars.FileDroidWithArgAndVarQuery, map[string]interface{}{"droidID": "R2D2"}),
		plannerConfig: droidGraphqlDataSource,
		roundTripper: createTestRoundTripper(t, roundTripperTestCase{
			expectedHost:     "example.com",
			expectedPath:     "/",
			expectedBody:     "",
			sendResponseBody: `{"data":{"droid":{"name":"R2D2"}}}`,
			sendStatusCode:   200,
		}),
		preExecutionTasks: normalizeAndValidatePreExecutionTasks,
		expectedResponse:  `{"data":{"droid":{"name":"R2D2"}}}`,
	}))

	t.Run("execute query with arguments", runWithoutError(testCase{
		schema:        starwarsSchema(t),
		request:       loadStarWarsQuery(starwars.FileDroidWithArgQuery, nil),
		plannerConfig: droidGraphqlDataSource,
		roundTripper: createTestRoundTripper(t, roundTripperTestCase{
			expectedHost:     "example.com",
			expectedPath:     "/",
			expectedBody:     "",
			sendResponseBody: `{"data":{"droid":{"name":"R2D2"}}}`,
			sendStatusCode:   200,
		}),
		preExecutionTasks: normalizeAndValidatePreExecutionTasks,
		expectedResponse:  `{"data":{"droid":{"name":"R2D2"}}}`,
	}))

	t.Run("execute single mutation with arguments on document with multiple operations", runWithoutError(testCase{
		schema: moviesSchema(t),
		request: func(t *testing.T) Request {
			return Request{
				OperationName: "AddWithInput",
				Variables:     nil,
				Query: `mutation AddToWatchlist {
						  addToWatchlist(movieID:3) {
							id
							name
							year
						  }
						}
						
						
						mutation AddWithInput {
						  addToWatchlistWithInput(input: {id: 2}) {
							id
							name
							year
						  }
						}`,
			}
		},
		plannerConfig: movieHttpJsonDataSource,
		roundTripper: createTestRoundTripper(t, roundTripperTestCase{
			expectedHost:     "example.com",
			expectedPath:     "/",
			expectedBody:     "",
			sendResponseBody: `{"added_movie":{"id":2, "name": "Episode V – The Empire Strikes Back", "year": 1980}}`,
			sendStatusCode:   200,
		}),
		preExecutionTasks: normalizeAndValidatePreExecutionTasks,
		expectedResponse:  `{"data":{"addToWatchlistWithInput":{"id":2,"name":"Episode V – The Empire Strikes Back","year":1980}}}`,
	}))

	t.Run("execute operation with rest data source and arguments", runWithoutError(testCase{
		schema: heroWithArgumentSchema(t),
		request: func(t *testing.T) Request {
			return Request{
				OperationName: "MyHero",
				Variables: stringify(map[string]interface{}{
					"heroName": "Luke Skywalker",
				}),
				Query: `query MyHero($heroName: String){
					hero(name: $heroName)
				}`,
			}
		},
		plannerConfig: heroWithArgumentHttpJsonDataSource,
		roundTripper: createTestRoundTripper(t, roundTripperTestCase{
			expectedHost:     "example.com",
			expectedPath:     "/",
			expectedBody:     `{ "name": "Luke Skywalker" }`,
			sendResponseBody: `{"race": "Human"}`,
			sendStatusCode:   200,
		}),
		expectedResponse: `{"data":{"hero":"Human"}}`,
	}))

	t.Run("execute query and apply input coercion for lists", runWithoutError(testCase{
		schema: inputCoercionForListSchema(t),
		request: func(t *testing.T) Request {
			return Request{
				OperationName: "charactersByIds",
				Variables: stringify(map[string]interface{}{
					"ids": 1,
				}),
				Query: `query($ids: [Int]) {charactersByIds(ids: $ids) { name }}`,
			}
		},
		plannerConfig: inputCoercionHttpJsonDataSource,
		roundTripper: createTestRoundTripper(t, roundTripperTestCase{
			expectedHost:     "example.com",
			expectedPath:     "/",
			expectedBody:     `{ "ids": [1] }`,
			sendResponseBody: `{"charactersByIds":[{"name": "Luke"}]}`,
			sendStatusCode:   200,
		}),
		preExecutionTasks: normalizeAndValidatePreExecutionTasks,
		expectedResponse:  `{"data":{"charactersByIds":[{"name":"Luke"}]}}`,
	}))

	t.Run("execute query and apply input coercion for lists with inline integer value", runWithoutError(testCase{
		schema: inputCoercionForListSchema(t),
		request: func(t *testing.T) Request {
			return Request{
				OperationName: "charactersByIds",
				Variables:     stringify(map[string]interface{}{"ids": 1}),
				// the library would fail to parse the query without input coercion.
				Query: `query($ids: [Int]) {charactersByIds(ids: $ids) { name }}`,
			}
		},
		plannerConfig: inputCoercionHttpJsonDataSource,
		roundTripper: createTestRoundTripper(t, roundTripperTestCase{
			expectedHost:     "example.com",
			expectedPath:     "/",
			expectedBody:     `{ "ids": [1] }`,
			sendResponseBody: `{"charactersByIds":[{"name": "Luke"}]}`,
			sendStatusCode:   200,
		}),
		preExecutionTasks: normalizeAndValidatePreExecutionTasks,
		expectedResponse:  `{"data":{"charactersByIds":[{"name":"Luke"}]}}`,
	}))
}

func normalizeAndValidatePreExecutionTasks(t *testing.T, request Request, schema *Schema, engine *ExecutionEngine) {
	normalizationResult, err := request.Normalize(schema)
	require.NoError(t, err)
	require.True(t, normalizationResult.Successful)

	validationResult, err := request.ValidateForSchema(schema)
	require.NoError(t, err)
	require.True(t, validationResult.Valid)
}

func stringify(any interface{}) []byte {
	out, _ := json.Marshal(any)
	return out
}

func stringPtr(str string) *string {
	return &str
}

func moviesSchema(t *testing.T) *Schema {
	schemaString := `
type Movie {
  id: Int!
  name: String!
  year: Int!
}

type Mutation {
  addToWatchlist(movieID: Int!): Movie
  addToWatchlistWithInput(input: WatchlistInput!): Movie
}

type Query {
  default: String
}

input WatchlistInput {
  id: Int!
}`
	schema, err := NewSchemaFromString(schemaString)
	require.NoError(t, err)
	return schema
}

func heroWithArgumentSchema(t *testing.T) *Schema {
	schemaString := `
		type Query {
			hero(name: String): String
			heroDefault(name: String = "Any"): String
			heroDefaultRequired(name: String! = "AnyRequired"): String
			heroes(names: [String!]!): [String!]
		}`

	schema, err := NewSchemaFromString(schemaString)
	require.NoError(t, err)
	return schema
}

func TestExampleExecutionEngine_Concatenation(t *testing.T) {

	schema, err := NewSchemaFromString(`
		schema { query: Query }
		type Query { friend: Friend }
		type Friend { firstName: String lastName: String fullName: String }
	`)

	assert.NoError(t, err)

	friendServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"firstName":"Jens","lastName":"Neuse"}`))
	}))

	defer friendServer.Close()

	pipelineConcat := `
	{
		"steps": [
			{
				"kind": "JSON",
				"config": {
					"template": "{\"fullName\":\"{{ .firstName }} {{ .lastName }}\"}"
				}
			}
  		]
	}`

	plannerConfig := datasource.PlannerConfiguration{
		TypeFieldConfigurations: []datasource.TypeFieldConfiguration{
			{
				TypeName:  "query",
				FieldName: "friend",
				Mapping: &datasource.MappingConfiguration{
					Disabled: true,
				},
				DataSource: datasource.SourceConfig{
					Name: "HttpJsonDataSource",
					Config: stringify(datasource.HttpJsonDataSourceConfig{
						URL:    friendServer.URL,
						Method: stringPtr("GET"),
					}),
				},
			},
			{
				TypeName:  "Friend",
				FieldName: "fullName",
				DataSource: datasource.SourceConfig{
					Name: "FriendFullName",
					Config: stringify(datasource.PipelineDataSourceConfig{
						ConfigString: stringPtr(pipelineConcat),
						InputJSON:    `{"firstName":"{{ .object.firstName }}","lastName":"{{ .object.lastName }}"}`,
					}),
				},
			},
		},
	}

	engine, err := NewExecutionEngine(abstractlogger.NoopLogger, schema, plannerConfig)
	assert.NoError(t, err)
	err = engine.AddHttpJsonDataSource("HttpJsonDataSource")
	assert.NoError(t, err)
	err = engine.AddDataSource("FriendFullName", datasource.PipelineDataSourcePlannerFactoryFactory{})
	assert.NoError(t, err)

	request := &Request{
		Query: `query { friend { firstName lastName fullName }}`,
	}

	executionRes, err := engine.Execute(context.Background(), request, ExecutionOptions{})
	assert.NoError(t, err)

	expected := `{"data":{"friend":{"firstName":"Jens","lastName":"Neuse","fullName":"Jens Neuse"}}}`
	actual := executionRes.Buffer().String()
	assert.Equal(t, expected, actual)
}

func BenchmarkExecutionEngine(b *testing.B) {

	newEngine := func() *ExecutionEngine {
		schema, err := NewSchemaFromString(`type Query { hello: String}`)
		assert.NoError(b, err)
		plannerConfig := datasource.PlannerConfiguration{
			TypeFieldConfigurations: []datasource.TypeFieldConfiguration{
				{
					TypeName:  "query",
					FieldName: "hello",
					Mapping: &datasource.MappingConfiguration{
						Disabled: true,
					},
					DataSource: datasource.SourceConfig{
						Name: "HelloDataSource",
						Config: stringify(datasource.StaticDataSourceConfig{
							Data: "world",
						}),
					},
				},
			},
		}
		engine, err := NewExecutionEngine(abstractlogger.NoopLogger, schema, plannerConfig)
		assert.NoError(b, err)
		assert.NoError(b, engine.AddDataSource("HelloDataSource", datasource.StaticDataSourcePlannerFactoryFactory{}))
		return engine
	}

	ctx := context.Background()
	req := &Request{
		Query: "{hello}",
	}
	out := bytes.Buffer{}
	err := newEngine().ExecuteWithWriter(ctx, req, &out, ExecutionOptions{})
	assert.NoError(b, err)
	assert.Equal(b, "{\"data\":{\"hello\":\"world\"}}", out.String())

	pool := sync.Pool{
		New: func() interface{} {
			return newEngine()
		},
	}

	b.SetBytes(int64(out.Len()))
	b.ResetTimer()
	b.ReportAllocs()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			engine := pool.Get().(*ExecutionEngine)
			_ = engine.ExecuteWithWriter(ctx, req, ioutil.Discard, ExecutionOptions{})
			pool.Put(engine)
		}
	})
}

var heroHttpJsonPlannerConfig = datasource.PlannerConfiguration{
	TypeFieldConfigurations: []datasource.TypeFieldConfiguration{
		{
			TypeName:  "Query",
			FieldName: "hero",
			Mapping: &datasource.MappingConfiguration{
				Disabled: false,
				Path:     "hero",
			},
			DataSource: datasource.SourceConfig{
				Name: "HttpJsonDataSource",
				Config: func() []byte {
					data, _ := json.Marshal(datasource.HttpJsonDataSourceConfig{
						URL: "example.com/",
						Method: func() *string {
							method := "GET"
							return &method
						}(),
						DefaultTypeName: func() *string {
							typeName := "Hero"
							return &typeName
						}(),
					})
					return data
				}(),
			},
		},
	},
}

var movieHttpJsonDataSource = datasource.PlannerConfiguration{
	TypeFieldConfigurations: []datasource.TypeFieldConfiguration{
		{
			TypeName:  "Mutation",
			FieldName: "addToWatchlistWithInput",
			Mapping: &datasource.MappingConfiguration{
				Disabled: false,
				Path:     "added_movie",
			},
			DataSource: datasource.SourceConfig{
				Name: "HttpJsonDataSource",
				Config: func() []byte {
					data, _ := json.Marshal(datasource.HttpJsonDataSourceConfig{
						URL: "example.com/",
						Method: func() *string {
							method := "GET"
							return &method
						}(),
						DefaultTypeName: func() *string {
							typeName := "Movie"
							return &typeName
						}(),
					})
					return data
				}(),
			},
		},
	},
}

var inputCoercionHttpJsonDataSource = datasource.PlannerConfiguration{
	TypeFieldConfigurations: []datasource.TypeFieldConfiguration{
		{
			TypeName:  "Query",
			FieldName: "charactersByIds",
			Mapping: &datasource.MappingConfiguration{
				Disabled: false,
				Path:     "charactersByIds",
			},
			DataSource: datasource.SourceConfig{
				Name: "HttpJsonDataSource",
				Config: func() []byte {
					data, _ := json.Marshal(datasource.HttpJsonDataSourceConfig{
						URL: "example.com/",
						Method: func() *string {
							method := "GET"
							return &method
						}(),
						Body: stringPtr(`{ "ids": {{ .arguments.ids }} }`),
						DefaultTypeName: func() *string {
							typeName := "Character"
							return &typeName
						}(),
					})
					return data
				}(),
			},
		},
	},
}

var heroWithArgumentHttpJsonDataSource = datasource.PlannerConfiguration{
	TypeFieldConfigurations: []datasource.TypeFieldConfiguration{
		{
			TypeName:  "Query",
			FieldName: "hero",
			Mapping: &datasource.MappingConfiguration{
				Disabled: false,
				Path:     "race",
			},
			DataSource: datasource.SourceConfig{
				Name: "HttpJsonDataSource",
				Config: func() []byte {
					data, _ := json.Marshal(datasource.HttpJsonDataSourceConfig{
						URL: "example.com/",
						Method: func() *string {
							method := "GET"
							return &method
						}(),
						Body: stringPtr(`{ "name": "{{ .arguments.name }}" }`),
						DefaultTypeName: func() *string {
							typeName := "Hero"
							return &typeName
						}(),
					})
					return data
				}(),
			},
		},
	},
}

var heroGraphqlDataSource = datasource.PlannerConfiguration{
	TypeFieldConfigurations: []datasource.TypeFieldConfiguration{
		{
			TypeName:  "query",
			FieldName: "hero",
			Mapping: &datasource.MappingConfiguration{
				Disabled: false,
				Path:     "hero",
			},
			DataSource: datasource.SourceConfig{
				Name: "GraphqlDataSource",
				Config: func() []byte {
					data, _ := json.Marshal(datasource.GraphQLDataSourceConfig{
						URL: "example.com/",
						Method: func() *string {
							method := "GET"
							return &method
						}(),
					})
					return data
				}(),
			},
		},
	},
}

var droidGraphqlDataSource = datasource.PlannerConfiguration{
	TypeFieldConfigurations: []datasource.TypeFieldConfiguration{
		{
			TypeName:  "query",
			FieldName: "droid",
			Mapping: &datasource.MappingConfiguration{
				Disabled: false,
				Path:     "droid",
			},
			DataSource: datasource.SourceConfig{
				Name: "GraphqlDataSource",
				Config: func() []byte {
					data, _ := json.Marshal(datasource.GraphQLDataSourceConfig{
						URL: "example.com/",
						Method: func() *string {
							method := "GET"
							return &method
						}(),
					})
					return data
				}(),
			},
		},
	},
}
