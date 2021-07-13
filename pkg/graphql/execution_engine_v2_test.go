package graphql

import (
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/jensneuse/abstractlogger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jensneuse/graphql-go-tools/pkg/engine/datasource/graphql_datasource"
	"github.com/jensneuse/graphql-go-tools/pkg/engine/datasource/httpclient"
	"github.com/jensneuse/graphql-go-tools/pkg/engine/datasource/rest_datasource"
	"github.com/jensneuse/graphql-go-tools/pkg/engine/datasource/staticdatasource"
	"github.com/jensneuse/graphql-go-tools/pkg/engine/plan"
	"github.com/jensneuse/graphql-go-tools/pkg/engine/resolve"
	"github.com/jensneuse/graphql-go-tools/pkg/engine/subscription"
	"github.com/jensneuse/graphql-go-tools/pkg/engine/subscription/http_polling"
	"github.com/jensneuse/graphql-go-tools/pkg/starwars"
)

func TestNewEngineV2Configuration(t *testing.T) {
	var engineConfig EngineV2Configuration

	t.Run("should create a new engine v2 config", func(t *testing.T) {
		schema, err := NewSchemaFromString(countriesSchema)
		require.NoError(t, err)

		engineConfig = NewEngineV2Configuration(schema)
		assert.Len(t, engineConfig.plannerConfig.DataSources, 0)
		assert.Len(t, engineConfig.plannerConfig.Fields, 0)
		assert.Equal(t, dataLoaderConfig{}, engineConfig.dataLoaderConfig)
	})

	t.Run("should successfully add a data source", func(t *testing.T) {
		ds := plan.DataSourceConfiguration{Custom: []byte("1")}
		engineConfig.AddDataSource(ds)

		assert.Len(t, engineConfig.plannerConfig.DataSources, 1)
		assert.Equal(t, ds, engineConfig.plannerConfig.DataSources[0])
	})

	t.Run("should successfully set all data sources", func(t *testing.T) {
		ds := []plan.DataSourceConfiguration{
			{Custom: []byte("2")},
			{Custom: []byte("3")},
			{Custom: []byte("4")},
		}
		engineConfig.SetDataSources(ds)

		assert.Len(t, engineConfig.plannerConfig.DataSources, 3)
		assert.Equal(t, ds, engineConfig.plannerConfig.DataSources)
	})

	t.Run("should successfully enable single flight loader", func(t *testing.T) {
		engineConfig.EnableSingleFlightLoader(true)
		assert.True(t, engineConfig.dataLoaderConfig.EnableSingleFlightLoader)
	})

	t.Run("should successfully enable data loader", func(t *testing.T) {
		engineConfig.EnableDataLoader(true)
		assert.True(t, engineConfig.dataLoaderConfig.EnableDataLoader)
	})

	t.Run("should successfully add a field config", func(t *testing.T) {
		fieldConfig := plan.FieldConfiguration{FieldName: "a"}
		engineConfig.AddFieldConfiguration(fieldConfig)

		assert.Len(t, engineConfig.plannerConfig.Fields, 1)
		assert.Equal(t, fieldConfig, engineConfig.plannerConfig.Fields[0])
	})

	t.Run("should successfully set all field configs", func(t *testing.T) {
		fieldConfigs := plan.FieldConfigurations{
			{FieldName: "b"},
			{FieldName: "c"},
			{FieldName: "d"},
		}
		engineConfig.SetFieldConfigurations(fieldConfigs)

		assert.Len(t, engineConfig.plannerConfig.Fields, 3)
		assert.Equal(t, fieldConfigs, engineConfig.plannerConfig.Fields)
	})
}

func TestEngineResponseWriter_AsHTTPResponse(t *testing.T) {
	rw := NewEngineResultWriter()
	_, err := rw.Write([]byte(`{"key": "value"}`))
	require.NoError(t, err)

	headers := make(http.Header)
	headers.Set("Content-Type", "application/json")
	response := rw.AsHTTPResponse(http.StatusOK, headers)
	body, err := ioutil.ReadAll(response.Body)
	require.NoError(t, err)

	assert.Equal(t, http.StatusOK, response.StatusCode)
	assert.Equal(t, "application/json", response.Header.Get("Content-Type"))
	assert.Equal(t, `{"key": "value"}`, string(body))
}

type ExecutionEngineV2TestCase struct {
	schema           *Schema
	operation        func(t *testing.T) Request
	dataSources      []plan.DataSourceConfiguration
	fields           plan.FieldConfigurations
	expectedResponse string
}

type executionEngineV2SubscriptionTestCase struct {
	schema            *Schema
	operation         func(t *testing.T) Request
	dataSources       []plan.DataSourceConfiguration
	enableDataLoader  bool
	fields            plan.FieldConfigurations
	streamFactory     func(cancelSubscriptionFn context.CancelFunc) subscription.Stream
	expectedResponses []string
}

func TestExecutionEngineV2_Execute(t *testing.T) {
	run := func(testCase ExecutionEngineV2TestCase, withError bool) func(t *testing.T) {
		return func(t *testing.T) {
			engineConf := NewEngineV2Configuration(testCase.schema)
			engineConf.SetDataSources(testCase.dataSources)
			engineConf.SetFieldConfigurations(testCase.fields)
			closer := make(chan struct{})
			defer close(closer)
			engine, err := NewExecutionEngineV2(abstractlogger.Noop{}, engineConf, closer)
			require.NoError(t, err)

			operation := testCase.operation(t)
			resultWriter := NewEngineResultWriter()
			err = engine.Execute(context.Background(), &operation, &resultWriter)

			assert.Equal(t, testCase.expectedResponse, resultWriter.String())

			if withError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		}
	}

	runWithError := func(testCase ExecutionEngineV2TestCase) func(t *testing.T) {
		return run(testCase, true)
	}

	runWithoutError := func(testCase ExecutionEngineV2TestCase) func(t *testing.T) {
		return run(testCase, false)
	}

	t.Run("execute with empty request object should not panic", runWithError(
		ExecutionEngineV2TestCase{
			schema: starwarsSchema(t),
			operation: func(t *testing.T) Request {
				return Request{}
			},
			dataSources: []plan.DataSourceConfiguration{
				{
					RootNodes: []plan.TypeField{
						{TypeName: "Query", FieldNames: []string{"hero"}},
					},
					Factory: &rest_datasource.Factory{},
					Custom: rest_datasource.ConfigJSON(rest_datasource.Configuration{
						Fetch: rest_datasource.FetchConfiguration{
							URL:    "https://example.com/",
							Method: "GET",
						},
					}),
				},
			},
			fields:           []plan.FieldConfiguration{},
			expectedResponse: "",
		},
	))

	t.Run("execute simple hero operation with rest data source", runWithoutError(
		ExecutionEngineV2TestCase{
			schema:    starwarsSchema(t),
			operation: loadStarWarsQuery(starwars.FileSimpleHeroQuery, nil),
			dataSources: []plan.DataSourceConfiguration{
				{
					RootNodes: []plan.TypeField{
						{TypeName: "Query", FieldNames: []string{"hero"}},
					},
					Factory: &rest_datasource.Factory{
						Client: testNetHttpClient(t, roundTripperTestCase{
							expectedHost:     "example.com",
							expectedPath:     "/",
							expectedBody:     "",
							sendResponseBody: `{"hero": {"name": "Luke Skywalker"}}`,
							sendStatusCode:   200,
						}),
					},
					Custom: rest_datasource.ConfigJSON(rest_datasource.Configuration{
						Fetch: rest_datasource.FetchConfiguration{
							URL:    "https://example.com/",
							Method: "GET",
						},
					}),
				},
			},
			fields:           []plan.FieldConfiguration{},
			expectedResponse: `{"data":{"hero":{"name":"Luke Skywalker"}}}`,
		},
	))

	t.Run("execute with header injection", runWithoutError(
		ExecutionEngineV2TestCase{
			schema: starwarsSchema(t),
			operation: func(t *testing.T) Request {
				request := loadStarWarsQuery(starwars.FileSimpleHeroQuery, nil)(t)
				request.request.Header = map[string][]string{
					"Authorization": {"foo"},
				}
				return request
			},
			dataSources: []plan.DataSourceConfiguration{
				{
					RootNodes: []plan.TypeField{
						{TypeName: "Query", FieldNames: []string{"hero"}},
					},
					Factory: &rest_datasource.Factory{
						Client: testNetHttpClient(t, roundTripperTestCase{
							expectedHost:     "example.com",
							expectedPath:     "/foo",
							expectedBody:     "",
							sendResponseBody: `{"hero": {"name": "Luke Skywalker"}}`,
							sendStatusCode:   200,
						}),
					},
					Custom: rest_datasource.ConfigJSON(rest_datasource.Configuration{
						Fetch: rest_datasource.FetchConfiguration{
							URL:    "https://example.com/{{ .request.headers.Authorization }}",
							Method: "GET",
						},
					}),
				},
			},
			fields:           []plan.FieldConfiguration{},
			expectedResponse: `{"data":{"hero":{"name":"Luke Skywalker"}}}`,
		},
	))

	t.Run("execute simple hero operation with graphql data source", runWithoutError(
		ExecutionEngineV2TestCase{
			schema:    starwarsSchema(t),
			operation: loadStarWarsQuery(starwars.FileSimpleHeroQuery, nil),
			dataSources: []plan.DataSourceConfiguration{
				{
					RootNodes: []plan.TypeField{
						{TypeName: "Query", FieldNames: []string{"hero"}},
					},
					Factory: &graphql_datasource.Factory{
						Client: testNetHttpClient(t, roundTripperTestCase{
							expectedHost:     "example.com",
							expectedPath:     "/",
							expectedBody:     "",
							sendResponseBody: `{"data":{"hero":{"name":"Luke Skywalker"}}}`,
							sendStatusCode:   200,
						}),
					},
					Custom: graphql_datasource.ConfigJson(graphql_datasource.Configuration{
						Fetch: graphql_datasource.FetchConfiguration{
							URL:    "https://example.com/",
							Method: "GET",
						},
					}),
				},
			},
			fields:           []plan.FieldConfiguration{},
			expectedResponse: `{"data":{"hero":{"name":"Luke Skywalker"}}}`,
		},
	))

	t.Run("execute the correct operation when sending multiple queries", runWithoutError(
		ExecutionEngineV2TestCase{
			schema: starwarsSchema(t),
			operation: func(t *testing.T) Request {
				request := loadStarWarsQuery(starwars.FileMultiQueries, nil)(t)
				request.OperationName = "SingleHero"
				return request
			},
			dataSources: []plan.DataSourceConfiguration{
				{
					RootNodes: []plan.TypeField{
						{TypeName: "Query", FieldNames: []string{"hero"}},
					},
					Factory: &graphql_datasource.Factory{
						Client: testNetHttpClient(t, roundTripperTestCase{
							expectedHost:     "example.com",
							expectedPath:     "/",
							expectedBody:     "",
							sendResponseBody: `{"data":{"hero":{"name":"Luke Skywalker"}}}`,
							sendStatusCode:   200,
						}),
					},
					Custom: graphql_datasource.ConfigJson(graphql_datasource.Configuration{
						Fetch: graphql_datasource.FetchConfiguration{
							URL:    "https://example.com/",
							Method: "GET",
						},
					}),
				},
			},
			fields:           []plan.FieldConfiguration{},
			expectedResponse: `{"data":{"hero":{"name":"Luke Skywalker"}}}`,
		},
	))

	t.Run("execute operation with variables for arguments", runWithoutError(
		ExecutionEngineV2TestCase{
			schema:    starwarsSchema(t),
			operation: loadStarWarsQuery(starwars.FileDroidWithArgAndVarQuery, map[string]interface{}{"droidID": "R2D2"}),
			dataSources: []plan.DataSourceConfiguration{
				{
					RootNodes: []plan.TypeField{
						{TypeName: "Query", FieldNames: []string{"droid"}},
					},
					Factory: &graphql_datasource.Factory{
						Client: testNetHttpClient(t, roundTripperTestCase{
							expectedHost:     "example.com",
							expectedPath:     "/",
							expectedBody:     "",
							sendResponseBody: `{"data":{"droid":{"name":"R2D2"}}}`,
							sendStatusCode:   200,
						}),
					},
					Custom: graphql_datasource.ConfigJson(graphql_datasource.Configuration{
						Fetch: graphql_datasource.FetchConfiguration{
							URL:    "https://example.com/",
							Method: "GET",
						},
					}),
				},
			},
			fields:           []plan.FieldConfiguration{},
			expectedResponse: `{"data":{"droid":{"name":"R2D2"}}}`,
		},
	))

	t.Run("execute operation with arguments", runWithoutError(
		ExecutionEngineV2TestCase{
			schema:    starwarsSchema(t),
			operation: loadStarWarsQuery(starwars.FileDroidWithArgQuery, nil),
			dataSources: []plan.DataSourceConfiguration{
				{
					RootNodes: []plan.TypeField{
						{TypeName: "Query", FieldNames: []string{"droid"}},
					},
					Factory: &graphql_datasource.Factory{
						Client: testNetHttpClient(t, roundTripperTestCase{
							expectedHost:     "example.com",
							expectedPath:     "/",
							expectedBody:     "",
							sendResponseBody: `{"data":{"droid":{"name":"R2D2"}}}`,
							sendStatusCode:   200,
						}),
					},
					Custom: graphql_datasource.ConfigJson(graphql_datasource.Configuration{
						Fetch: graphql_datasource.FetchConfiguration{
							URL:    "https://example.com/",
							Method: "GET",
						},
					}),
				},
			},
			fields:           []plan.FieldConfiguration{},
			expectedResponse: `{"data":{"droid":{"name":"R2D2"}}}`,
		},
	))

	t.Run("execute single mutation with arguments on document with multiple operations", runWithoutError(
		ExecutionEngineV2TestCase{
			schema: moviesSchema(t),
			operation: func(t *testing.T) Request {
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
			dataSources: []plan.DataSourceConfiguration{
				{
					RootNodes: []plan.TypeField{
						{TypeName: "Mutation", FieldNames: []string{"addToWatchlistWithInput"}},
					},
					Factory: &rest_datasource.Factory{
						Client: testNetHttpClient(t, roundTripperTestCase{
							expectedHost:     "example.com",
							expectedPath:     "/",
							expectedBody:     "",
							sendResponseBody: `{"added_movie":{"id":2, "name": "Episode V – The Empire Strikes Back", "year": 1980}}`,
							sendStatusCode:   200,
						}),
					},
					Custom: rest_datasource.ConfigJSON(rest_datasource.Configuration{
						Fetch: rest_datasource.FetchConfiguration{
							URL:    "https://example.com/",
							Method: "GET",
						},
					}),
				},
			},
			fields: []plan.FieldConfiguration{
				{
					TypeName:              "Mutation",
					FieldName:             "addToWatchlistWithInput",
					DisableDefaultMapping: false,
					Path:                  []string{"added_movie"},
				},
			},
			expectedResponse: `{"data":{"addToWatchlistWithInput":{"id":2,"name":"Episode V – The Empire Strikes Back","year":1980}}}`,
		},
	))

	t.Run("execute operation with rest data source and arguments", runWithoutError(
		ExecutionEngineV2TestCase{
			schema: heroWithArgumentSchema(t),
			operation: func(t *testing.T) Request {
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
			dataSources: []plan.DataSourceConfiguration{
				{
					RootNodes: []plan.TypeField{
						{TypeName: "Query", FieldNames: []string{"hero"}},
					},
					Factory: &rest_datasource.Factory{
						Client: testNetHttpClient(t, roundTripperTestCase{
							expectedHost:     "example.com",
							expectedPath:     "/",
							expectedBody:     `{ "name": "Luke Skywalker" }`,
							sendResponseBody: `{"race": "Human"}`,
							sendStatusCode:   200,
						}),
					},
					Custom: rest_datasource.ConfigJSON(rest_datasource.Configuration{
						Fetch: rest_datasource.FetchConfiguration{
							URL:    "https://example.com/",
							Method: "POST",
							Body:   `{ "name": "{{ .arguments.name }}" }`,
						},
					}),
				},
			},
			fields: []plan.FieldConfiguration{
				{
					TypeName:              "Query",
					FieldName:             "hero",
					DisableDefaultMapping: false,
					Path:                  []string{"race"},
				},
			},
			expectedResponse: `{"data":{"hero":"Human"}}`,
		},
	))

	t.Run("execute operation with rest data source and arguments in url", runWithoutError(
		ExecutionEngineV2TestCase{
			schema: heroWithArgumentSchema(t),
			operation: func(t *testing.T) Request {
				return Request{
					OperationName: "MyHero",
					Variables: stringify(map[string]interface{}{
						"heroName": "luke",
					}),
					Query: `query MyHero($heroName: String){
						hero(name: $heroName)
					}`,
				}
			},
			dataSources: []plan.DataSourceConfiguration{
				{
					RootNodes: []plan.TypeField{
						{TypeName: "Query", FieldNames: []string{"hero"}},
					},
					Factory: &rest_datasource.Factory{
						Client: testNetHttpClient(t, roundTripperTestCase{
							expectedHost:     "example.com",
							expectedPath:     "/name/luke",
							expectedBody:     "",
							sendResponseBody: `{"race": "Human"}`,
							sendStatusCode:   200,
						}),
					},
					Custom: rest_datasource.ConfigJSON(rest_datasource.Configuration{
						Fetch: rest_datasource.FetchConfiguration{
							URL:    "https://example.com/name/{{.arguments.name}}",
							Method: "POST",
							Body:   "",
						},
					}),
				},
			},
			fields: []plan.FieldConfiguration{
				{
					TypeName:              "Query",
					FieldName:             "hero",
					DisableDefaultMapping: false,
					Path:                  []string{"race"},
				},
			},
			expectedResponse: `{"data":{"hero":"Human"}}`,
		},
	))

	runSubscription := func(testCase executionEngineV2SubscriptionTestCase, withError bool) func(t *testing.T) {
		return func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			subscriptionCtx, subscriptionCancel := context.WithCancel(ctx)
			defer subscriptionCancel()

			engineConf := NewEngineV2Configuration(testCase.schema)
			engineConf.SetDataSources(testCase.dataSources)
			engineConf.SetFieldConfigurations(testCase.fields)
			engineConf.EnableDataLoader(testCase.enableDataLoader)
			closer := make(chan struct{})
			defer close(closer)
			engine, err := NewExecutionEngineV2(abstractlogger.Noop{}, engineConf, closer)
			require.NoError(t, err)

			triggerManager := subscription.NewManager(testCase.streamFactory(subscriptionCancel))
			engine.WithTriggerManager(triggerManager)
			triggerManager.Run(ctx.Done())

			operation := testCase.operation(t)

			var messages []string
			resultWriter := NewEngineResultWriter()
			resultWriter.SetFlushCallback(func(data []byte) {
				messages = append(messages, string(data))
			})

			err = engine.Execute(subscriptionCtx, &operation, &resultWriter)

			assert.Equal(t, testCase.expectedResponses, messages)

			if withError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		}
	}

	runSubscriptionWithoutError := func(testCase executionEngineV2SubscriptionTestCase) func(t *testing.T) {
		return runSubscription(testCase, false)
	}

	t.Run("execute subscription operation with graphql data source", runSubscriptionWithoutError(executionEngineV2SubscriptionTestCase{
		schema: starwarsSchema(t),
		operation: func(t *testing.T) Request {
			request := loadStarWarsQuery(starwars.FileRemainingJedisSubscription, nil)(t)
			return request
		},
		dataSources: []plan.DataSourceConfiguration{
			{
				RootNodes: []plan.TypeField{
					{TypeName: "Subscription", FieldNames: []string{"remainingJedis"}},
				},
				Factory: &graphql_datasource.Factory{},
				Custom: graphql_datasource.ConfigJson(graphql_datasource.Configuration{
					Subscription: graphql_datasource.SubscriptionConfiguration{
						URL: "wss://swapi.com/graphql",
					},
				}),
			},
		},
		fields: []plan.FieldConfiguration{},
		streamFactory: func(cancelSubscriptionFn context.CancelFunc) subscription.Stream {
			stream := subscription.NewStreamStub([]byte("graphql_websocket_subscription"), context.Background().Done())
			go func() {
				stream.SendMessage(`{"url":"wss://swapi.com/graphql","body":{"query":"subscription{remainingJedis}"}}`, []byte(`{"remainingJedis":1}`))
				time.Sleep(5 * time.Millisecond)
				stream.SendMessage(`{"url":"wss://swapi.com/graphql","body":{"query":"subscription{remainingJedis}"}}`, []byte(`{"remainingJedis":2}`))
				time.Sleep(5 * time.Millisecond)
				cancelSubscriptionFn()
			}()

			return stream
		},
		expectedResponses: []string{`{"data":{"remainingJedis":1}}`, `{"data":{"remainingJedis":2}}`},
	}))

	t.Run("execute subscription with rest data source", runSubscriptionWithoutError(executionEngineV2SubscriptionTestCase{
		schema: starwarsSchema(t),
		operation: func(t *testing.T) Request {
			request := loadStarWarsQuery(starwars.FileRemainingJedisSubscription, nil)(t)
			return request
		},
		dataSources: []plan.DataSourceConfiguration{
			{
				RootNodes: []plan.TypeField{
					{TypeName: "Subscription", FieldNames: []string{"remainingJedis"}},
				},
				Factory: &rest_datasource.Factory{
					Client: testNetHttpClient(t, roundTripperTestCase{
						expectedHost:     "example.com",
						expectedPath:     "/",
						expectedBody:     "",
						sendResponseBody: `{"remainingJedis":1}`,
						sendStatusCode:   200,
					}),
				},
				Custom: rest_datasource.ConfigJSON(rest_datasource.Configuration{
					Fetch: rest_datasource.FetchConfiguration{
						URL:    "https://example.com/",
						Method: "GET",
						Body:   "",
					},
					Subscription: rest_datasource.SubscriptionConfiguration{
						PollingIntervalMillis:   5,
						SkipPublishSameResponse: true,
					},
				}),
			},
		},
		fields: []plan.FieldConfiguration{},
		streamFactory: func(cancelSubscriptionFn context.CancelFunc) subscription.Stream {
			var callNum int

			stream := http_polling.New(testHttpClientDecorator(func() httpclient.Client {
				callNum++

				return testNetHttpClient(t, roundTripperTestCase{
					expectedHost:     "example.com",
					expectedPath:     "/",
					expectedBody:     "",
					sendResponseBody: fmt.Sprintf(`{"remainingJedis":%d}`, callNum),
					sendStatusCode:   200,
				})
			}))

			go func() {
				time.Sleep(15 * time.Millisecond)
				cancelSubscriptionFn()
			}()

			return stream
		},
		expectedResponses: []string{`{"data":{"remainingJedis":1}}`, `{"data":{"remainingJedis":2}}`},
	}))

	t.Run("execute subscription with graphql federation data source", func(t *testing.T) {
		batchFactory := graphql_datasource.NewBatchFactory()
		runSubscriptionWithoutError(executionEngineV2SubscriptionTestCase{
			schema: federationSchema(t),
			operation: func(t *testing.T) Request {
				return Request{
					Query: `
					subscription UpdatePrice {
						updatedPrice {
							upc
							name
							price
							reviews {
								body
								author {
									name	
									username
								}
							}
						}
					}
				`,
					OperationName: "UpdatePrice",
				}
			},
			dataSources: []plan.DataSourceConfiguration{
				{
					RootNodes: []plan.TypeField{
						{
							TypeName:   "Query",
							FieldNames: []string{"me"},
						},
						{
							TypeName:   "User",
							FieldNames: []string{"id", "name", "username"},
						},
					},
					ChildNodes: []plan.TypeField{
						{
							TypeName:   "User",
							FieldNames: []string{"id", "name", "username"},
						},
					},
					Custom: graphql_datasource.ConfigJson(graphql_datasource.Configuration{
						Fetch: graphql_datasource.FetchConfiguration{
							URL:    "http://user.service/",
							Method: "GET",
						},
						Federation: graphql_datasource.FederationConfiguration{
							Enabled:    true,
							ServiceSDL: `extend type Query { me: User } type User @key(fields: "id") { id: ID! name: String username: String }`,
						},
					}),
					Factory: &graphql_datasource.Factory{
						BatchFactory: batchFactory,
						Client: testNetHttpClient(t, roundTripperTestCase{
							expectedHost:     "user.service",
							expectedPath:     "/",
							expectedBody:     `{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {name}}}","variables":{"representations":[{"id":"1234","__typename":"User"}]}}`,
							sendResponseBody: `{"data":{"_entities":[{"name": "Name 1234"}]}}`,
							sendStatusCode:   200,
						}),
					},
				},
				{
					RootNodes: []plan.TypeField{
						{
							TypeName:   "Query",
							FieldNames: []string{"topProducts"},
						},
						{
							TypeName:   "Product",
							FieldNames: []string{"upc", "name", "price", "weight"},
						},
						{
							TypeName:   "Subscription",
							FieldNames: []string{"updatedPrice"},
						},
						{
							TypeName:   "Mutation",
							FieldNames: []string{"setPrice"},
						},
					},
					ChildNodes: []plan.TypeField{
						{
							TypeName:   "Product",
							FieldNames: []string{"upc", "name", "price", "weight"},
						},
					},
					Custom: graphql_datasource.ConfigJson(graphql_datasource.Configuration{
						Fetch: graphql_datasource.FetchConfiguration{
							URL:    "http://product.service",
							Method: "GET",
						},
						Subscription: graphql_datasource.SubscriptionConfiguration{
							URL: "ws://product.service",
						},
						Federation: graphql_datasource.FederationConfiguration{
							Enabled: true,
							ServiceSDL: `
extend	type Query {
  topProducts(first: Int = 5): [Product]
}

type Product @key(fields: "upc") {
  upc: String!
  name: String
  price: Int
  weight: Int
}

extend type Subscription  {
  updatedPrice: Product!
}

extend type Mutation {
  setPrice(upc: String!, price: Int!): Product
}`,
						},
					}),
					Factory: &graphql_datasource.Factory{
						BatchFactory: batchFactory,
						Client: testNetHttpClient(t, roundTripperTestCase{
							expectedHost:     "product.service",
							expectedPath:     "/",
							expectedBody:     "",
							sendResponseBody: ``,
							sendStatusCode:   200,
						}),
					},
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
							FieldNames: []string{"id", "body", "author", "product"},
						},
						{
							TypeName:   "User",
							FieldNames: []string{"id", "username", "reviews"},
						},
						{
							TypeName:   "Product",
							FieldNames: []string{"upc", "reviews"},
						},
					},
					Custom: graphql_datasource.ConfigJson(graphql_datasource.Configuration{
						Fetch: graphql_datasource.FetchConfiguration{
							URL:    "http://review.service/",
							Method: "GET",
						},
						Federation: graphql_datasource.FederationConfiguration{
							Enabled: true,
							ServiceSDL: `
type Review {
  id: ID!
  body: String
  author: User @provides(fields: "username")
  product: Product
}

extend type User  @key(fields: "id") {
  id: ID! @external
  username: String @external
  reviews: [Review]
}

extend type Product @key(fields: "upc") {
  upc: String! @external
  reviews: [Review]
}`,
						},
					}),
					Factory: &graphql_datasource.Factory{
						BatchFactory: batchFactory,
						Client: testNetHttpClient(t, roundTripperTestCase{
							expectedHost:     "review.service",
							expectedPath:     "/",
							expectedBody:     `{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Product {reviews {body author {username id}}}}}","variables":{"representations":[{"upc":"top-1","__typename":"Product"}]}}`,
							sendResponseBody: `{"data": {"_entities": [{"reviews": [{"body": "A highly effective form of birth control.","author": {"username": "User 1234","id": 1234}}]}]}}`,
							sendStatusCode:   200,
						}),
					},
				},
			},
			fields: plan.FieldConfigurations{
				{
					TypeName:       "User",
					FieldName:      "name",
					RequiresFields: []string{"id"},
				},
				{
					TypeName:       "User",
					FieldName:      "username",
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
					FieldName:      "weight",
					RequiresFields: []string{"upc"},
				},
				{
					TypeName:       "User",
					FieldName:      "reviews",
					RequiresFields: []string{"id"},
				},
				{
					TypeName:       "Product",
					FieldName:      "reviews",
					RequiresFields: []string{"upc"},
				},
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
					TypeName:  "Mutation",
					FieldName: "setPrice",
					Arguments: []plan.ArgumentConfiguration{
						{
							Name:       "upc",
							SourceType: plan.FieldArgumentSource,
						},
						{
							Name:       "price",
							SourceType: plan.FieldArgumentSource,
						},
					},
				},
			},
			streamFactory: func(cancelSubscriptionFn context.CancelFunc) subscription.Stream {
				stream := subscription.NewStreamStub([]byte("graphql_websocket_subscription"), context.Background().Done())

				go func() {
					stream.SendMessage(
						`{"url":"ws://product.service","body":{"query":"subscription{updatedPrice {upc name price}}"}}`,
						[]byte(`{"updatedPrice":{"upc": "top-1", "name": "Trilby", "price": 11}}`))
					time.Sleep(5 * time.Millisecond)
					stream.SendMessage(
						`{"url":"ws://product.service","body":{"query":"subscription{updatedPrice {upc name price}}"}}`,
						[]byte(`{"updatedPrice":{"upc": "top-1", "name": "Trilby", "price": 15}}`))
					time.Sleep(5 * time.Millisecond)
					cancelSubscriptionFn()
				}()

				return stream
			},
			expectedResponses: []string{
				`{"data":{"updatedPrice":{"upc":"top-1","name":"Trilby","price":11,"reviews":[{"body":"A highly effective form of birth control.","author":{"name":"Name 1234","username":"User 1234"}}]}}}`,
				`{"data":{"updatedPrice":{"upc":"top-1","name":"Trilby","price":15,"reviews":[{"body":"A highly effective form of birth control.","author":{"name":"Name 1234","username":"User 1234"}}]}}}`,
			},
		})
	})
	t.Run("execute subscription with graphql federation data source and enabled data loader", func(t *testing.T) {
		batchFactory := graphql_datasource.NewBatchFactory()
		runSubscriptionWithoutError(executionEngineV2SubscriptionTestCase{
			schema: federationSchema(t),
			enableDataLoader: true,
			operation: func(t *testing.T) Request {
				return Request{
					Query: `
					subscription UpdatePrice {
						updatedPrice {
							upc
							name
							price
							reviews {
								body
								author {
									name	
									username
								}
							}
						}
					}
				`,
					OperationName: "UpdatePrice",
				}
			},
			dataSources: []plan.DataSourceConfiguration{
				{
					RootNodes: []plan.TypeField{
						{
							TypeName:   "Query",
							FieldNames: []string{"me"},
						},
						{
							TypeName:   "User",
							FieldNames: []string{"id", "name", "username"},
						},
					},
					ChildNodes: []plan.TypeField{
						{
							TypeName:   "User",
							FieldNames: []string{"id", "name", "username"},
						},
					},
					Custom: graphql_datasource.ConfigJson(graphql_datasource.Configuration{
						Fetch: graphql_datasource.FetchConfiguration{
							URL:    "http://user.service/",
							Method: "GET",
						},
						Federation: graphql_datasource.FederationConfiguration{
							Enabled:    true,
							ServiceSDL: `extend type Query { me: User } type User @key(fields: "id") { id: ID! name: String username: String }`,
						},
					}),
					Factory: &graphql_datasource.Factory{
						BatchFactory: batchFactory,
						Client: testNetHttpClient(t, roundTripperTestCase{
							expectedHost:     "user.service",
							expectedPath:     "/",
							expectedBody:     `{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {name}}}","variables":{"representations":[{"id":"1234","__typename":"User"},{"id":"4321","__typename":"User"}]}}`,
							sendResponseBody: `{"data":{"_entities":[{"name": "Name 1234"},{"name": "Name 4321"}]}}`,
							sendStatusCode:   200,
						}),
					},
				},
				{
					RootNodes: []plan.TypeField{
						{
							TypeName:   "Query",
							FieldNames: []string{"topProducts"},
						},
						{
							TypeName:   "Product",
							FieldNames: []string{"upc", "name", "price", "weight"},
						},
						{
							TypeName:   "Subscription",
							FieldNames: []string{"updatedPrice"},
						},
						{
							TypeName:   "Mutation",
							FieldNames: []string{"setPrice"},
						},
					},
					ChildNodes: []plan.TypeField{
						{
							TypeName:   "Product",
							FieldNames: []string{"upc", "name", "price", "weight"},
						},
					},
					Custom: graphql_datasource.ConfigJson(graphql_datasource.Configuration{
						Fetch: graphql_datasource.FetchConfiguration{
							URL:    "http://product.service",
							Method: "GET",
						},
						Subscription: graphql_datasource.SubscriptionConfiguration{
							URL: "ws://product.service",
						},
						Federation: graphql_datasource.FederationConfiguration{
							Enabled: true,
							ServiceSDL: `
extend	type Query {
  topProducts(first: Int = 5): [Product]
}

type Product @key(fields: "upc") {
  upc: String!
  name: String
  price: Int
  weight: Int
}

extend type Subscription  {
  updatedPrice: Product!
}

extend type Mutation {
  setPrice(upc: String!, price: Int!): Product
}`,
						},
					}),
					Factory: &graphql_datasource.Factory{
						BatchFactory: batchFactory,
						Client: testNetHttpClient(t, roundTripperTestCase{
							expectedHost:     "product.service",
							expectedPath:     "/",
							expectedBody:     "",
							sendResponseBody: ``,
							sendStatusCode:   200,
						}),
					},
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
							FieldNames: []string{"id", "body", "author", "product"},
						},
						{
							TypeName:   "User",
							FieldNames: []string{"id", "username", "reviews"},
						},
						{
							TypeName:   "Product",
							FieldNames: []string{"upc", "reviews"},
						},
					},
					Custom: graphql_datasource.ConfigJson(graphql_datasource.Configuration{
						Fetch: graphql_datasource.FetchConfiguration{
							URL:    "http://review.service/",
							Method: "GET",
						},
						Federation: graphql_datasource.FederationConfiguration{
							Enabled: true,
							ServiceSDL: `
type Review {
  id: ID!
  body: String
  author: User @provides(fields: "username")
  product: Product
}

extend type User  @key(fields: "id") {
  id: ID! @external
  username: String @external
  reviews: [Review]
}

extend type Product @key(fields: "upc") {
  upc: String! @external
  reviews: [Review]
}`,
						},
					}),
					Factory: &graphql_datasource.Factory{
						BatchFactory: batchFactory,
						Client: testNetHttpClient(t, roundTripperTestCase{
							expectedHost:     "review.service",
							expectedPath:     "/",
							expectedBody:     `{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Product {reviews {body author {username id}}}}}","variables":{"representations":[{"upc":"top-1","__typename":"Product"}]}}`,
							sendResponseBody: `{"data": {"_entities": [{"reviews": [{"body": "A highly effective form of birth control.","author": {"username": "User 1234","id": 1234}},{"body": "Fedoras are one of the most fashionable hats around and can look great with a variety of outfits.","author": {"username": "User 4321","id": 4321}}]}]}}`,
							sendStatusCode:   200,
						}),
					},
				},
			},
			fields: plan.FieldConfigurations{
				{
					TypeName:       "User",
					FieldName:      "name",
					RequiresFields: []string{"id"},
				},
				{
					TypeName:       "User",
					FieldName:      "username",
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
					FieldName:      "weight",
					RequiresFields: []string{"upc"},
				},
				{
					TypeName:       "User",
					FieldName:      "reviews",
					RequiresFields: []string{"id"},
				},
				{
					TypeName:       "Product",
					FieldName:      "reviews",
					RequiresFields: []string{"upc"},
				},
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
					TypeName:  "Mutation",
					FieldName: "setPrice",
					Arguments: []plan.ArgumentConfiguration{
						{
							Name:       "upc",
							SourceType: plan.FieldArgumentSource,
						},
						{
							Name:       "price",
							SourceType: plan.FieldArgumentSource,
						},
					},
				},
			},
			streamFactory: func(cancelSubscriptionFn context.CancelFunc) subscription.Stream {
				stream := subscription.NewStreamStub([]byte("graphql_websocket_subscription"), context.Background().Done())

				go func() {
					stream.SendMessage(
						`{"url":"ws://product.service","body":{"query":"subscription{updatedPrice {upc name price}}"}}`,
						[]byte(`{"updatedPrice":{"upc": "top-1", "name": "Trilby", "price": 11}}`))
					time.Sleep(5 * time.Millisecond)
					stream.SendMessage(
						`{"url":"ws://product.service","body":{"query":"subscription{updatedPrice {upc name price}}"}}`,
						[]byte(`{"updatedPrice":{"upc": "top-1", "name": "Trilby", "price": 15}}`))
					time.Sleep(5 * time.Millisecond)
					cancelSubscriptionFn()
				}()

				return stream
			},
			expectedResponses: []string{
				`{"data":{"updatedPrice":{"upc":"top-1","name":"Trilby","price":11,"reviews":[{"body":"A highly effective form of birth control.","author":{"name":"Name 1234","username":"User 1234"}},{"body":"Fedoras are one of the most fashionable hats around and can look great with a variety of outfits.","author":{"name":"Name 4321","username":"User 4321"}}]}}}`,
				`{"data":{"updatedPrice":{"upc":"top-1","name":"Trilby","price":15,"reviews":[{"body":"A highly effective form of birth control.","author":{"name":"Name 1234","username":"User 1234"}},{"body":"Fedoras are one of the most fashionable hats around and can look great with a variety of outfits.","author":{"name":"Name 4321","username":"User 4321"}}]}}}`,
			},
		})
	})
}

func testNetHttpClient(t *testing.T, testCase roundTripperTestCase) httpclient.Client {
	return httpclient.NewNetHttpClient(&http.Client{
		Transport: createTestRoundTripper(t, testCase),
	})
}

type testHttpClientDecorator func() httpclient.Client

func (t testHttpClientDecorator) Do(ctx context.Context, requestInput []byte, out io.Writer) (err error) {
	httpClient := t()
	return httpClient.Do(ctx, requestInput, out)
}

type beforeFetchHook struct {
	input string
}

func (b *beforeFetchHook) OnBeforeFetch(ctx resolve.HookContext, input []byte) {
	b.input += string(input)
}

type afterFetchHook struct {
	data string
	err  string
}

func (a *afterFetchHook) OnData(ctx resolve.HookContext, output []byte, singleFlight bool) {
	a.data += string(output)
}

func (a *afterFetchHook) OnError(ctx resolve.HookContext, output []byte, singleFlight bool) {
	a.err += string(output)
}

func TestExecutionWithOptions(t *testing.T) {

	closer := make(chan struct{})
	defer close(closer)

	testCase := ExecutionEngineV2TestCase{
		schema:    starwarsSchema(t),
		operation: loadStarWarsQuery(starwars.FileSimpleHeroQuery, nil),
		dataSources: []plan.DataSourceConfiguration{
			{
				RootNodes: []plan.TypeField{
					{TypeName: "Query", FieldNames: []string{"hero"}},
				},
				Factory: &graphql_datasource.Factory{
					Client: testNetHttpClient(t, roundTripperTestCase{
						expectedHost:     "example.com",
						expectedPath:     "/",
						expectedBody:     "",
						sendResponseBody: `{"data":{"hero":{"name":"Luke Skywalker"}}}`,
						sendStatusCode:   200,
					}),
				},
				Custom: graphql_datasource.ConfigJson(graphql_datasource.Configuration{
					Fetch: graphql_datasource.FetchConfiguration{
						URL:    "https://example.com/",
						Method: "GET",
					},
				}),
			},
		},
		fields:           []plan.FieldConfiguration{},
		expectedResponse: `{"data":{"hero":{"name":"Luke Skywalker"}}}`,
	}

	engineConf := NewEngineV2Configuration(testCase.schema)
	engineConf.SetDataSources(testCase.dataSources)
	engineConf.SetFieldConfigurations(testCase.fields)

	engine, err := NewExecutionEngineV2(abstractlogger.Noop{}, engineConf, closer)
	require.NoError(t, err)

	before := &beforeFetchHook{}
	after := &afterFetchHook{}

	operation := testCase.operation(t)
	resultWriter := NewEngineResultWriter()
	err = engine.Execute(context.Background(), &operation, &resultWriter, WithBeforeFetchHook(before), WithAfterFetchHook(after))

	assert.Equal(t, `{"method":"GET","url":"https://example.com/","body":{"query":"{hero}"}}`, before.input)
	assert.Equal(t, `{"hero":{"name":"Luke Skywalker"}}`, after.data)
	assert.Equal(t, "", after.err)
	assert.NoError(t, err)
}

func BenchmarkExecutionEngineV2(b *testing.B) {

	closer := make(chan struct{})
	defer close(closer)

	type benchCase struct {
		engine *ExecutionEngineV2
		writer *EngineResultWriter
	}

	newEngine := func() *ExecutionEngineV2 {
		schema, err := NewSchemaFromString(`type Query { hello: String}`)
		require.NoError(b, err)

		engineConf := NewEngineV2Configuration(schema)
		engineConf.SetDataSources([]plan.DataSourceConfiguration{
			{
				RootNodes: []plan.TypeField{
					{TypeName: "Query", FieldNames: []string{"hello"}},
				},
				Factory: &staticdatasource.Factory{},
				Custom: staticdatasource.ConfigJSON(staticdatasource.Configuration{
					Data: "world",
				}),
			},
		})
		engineConf.SetFieldConfigurations([]plan.FieldConfiguration{
			{
				TypeName:              "Query",
				FieldName:             "hello",
				DisableDefaultMapping: true,
			},
		})

		engine, err := NewExecutionEngineV2(abstractlogger.NoopLogger, engineConf, closer)
		require.NoError(b, err)

		return engine
	}

	newBenchCase := func() *benchCase {
		writer := NewEngineResultWriter()
		return &benchCase{
			engine: newEngine(),
			writer: &writer,
		}
	}

	ctx := context.Background()
	req := Request{
		Query: "{hello}",
	}

	writer := NewEngineResultWriter()
	engine := newEngine()
	require.NoError(b, engine.Execute(ctx, &req, &writer))
	require.Equal(b, "{\"data\":{\"hello\":\"world\"}}", writer.String())

	pool := sync.Pool{
		New: func() interface{} {
			return newBenchCase()
		},
	}

	b.SetBytes(int64(writer.Len()))
	b.ResetTimer()
	b.ReportAllocs()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			bc := pool.Get().(*benchCase)
			bc.writer.Reset()
			_ = bc.engine.Execute(ctx, &req, bc.writer)
			pool.Put(bc)
		}
	})

}

func federationSchema(t *testing.T) *Schema {
	rawSchema := `
type Query {
	me: User
	topProducts(first: Int = 5): [Product]
}
		
type Mutation {
	setPrice(upc: String!, price: Int!): Product
} 

type Subscription {
	updatedPrice: Product!
}
		
type User {
	id: ID!
	name: String
	username: String
	reviews: [Review]
}

type Product {
	upc: String!
	name: String
	price: Int
	weight: Int
	reviews: [Review]
}

type Review {
	id: ID!
	body: String
	author: User
	product: Product
}
`

	schema, err := NewSchemaFromString(rawSchema)
	require.NoError(t, err)

	return schema
}
