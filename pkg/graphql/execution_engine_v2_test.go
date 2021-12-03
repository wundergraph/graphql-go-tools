package graphql

import (
	"compress/flate"
	"compress/gzip"
	"context"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/jensneuse/abstractlogger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	federationExample "github.com/jensneuse/graphql-go-tools/examples/federation"
	accounts "github.com/jensneuse/graphql-go-tools/examples/federation/accounts/graph"
	products "github.com/jensneuse/graphql-go-tools/examples/federation/products/graph"
	reviews "github.com/jensneuse/graphql-go-tools/examples/federation/reviews/graph"
	"github.com/jensneuse/graphql-go-tools/pkg/engine/datasource/graphql_datasource"
	"github.com/jensneuse/graphql-go-tools/pkg/engine/datasource/httpclient"
	"github.com/jensneuse/graphql-go-tools/pkg/engine/datasource/rest_datasource"
	"github.com/jensneuse/graphql-go-tools/pkg/engine/datasource/staticdatasource"
	"github.com/jensneuse/graphql-go-tools/pkg/engine/plan"
	"github.com/jensneuse/graphql-go-tools/pkg/engine/resolve"
	"github.com/jensneuse/graphql-go-tools/pkg/operationreport"
	"github.com/jensneuse/graphql-go-tools/pkg/starwars"
)

func TestEngineResponseWriter_AsHTTPResponse(t *testing.T) {
	t.Run("no compression", func(t *testing.T) {
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
	})

	t.Run("compression based on content encoding header", func(t *testing.T) {
		rw := NewEngineResultWriter()
		_, err := rw.Write([]byte(`{"key": "value"}`))
		require.NoError(t, err)

		headers := make(http.Header)
		headers.Set("Content-Type", "application/json")

		t.Run("gzip", func(t *testing.T) {
			headers.Set(httpclient.ContentEncodingHeader, "gzip")

			response := rw.AsHTTPResponse(http.StatusOK, headers)
			assert.Equal(t, http.StatusOK, response.StatusCode)
			assert.Equal(t, "application/json", response.Header.Get("Content-Type"))
			assert.Equal(t, "gzip", response.Header.Get(httpclient.ContentEncodingHeader))

			reader, err := gzip.NewReader(response.Body)
			require.NoError(t, err)

			body, err := ioutil.ReadAll(reader)
			require.NoError(t, err)

			assert.Equal(t, `{"key": "value"}`, string(body))
		})

		t.Run("deflate", func(t *testing.T) {
			headers.Set(httpclient.ContentEncodingHeader, "deflate")

			response := rw.AsHTTPResponse(http.StatusOK, headers)
			assert.Equal(t, http.StatusOK, response.StatusCode)
			assert.Equal(t, "application/json", response.Header.Get("Content-Type"))
			assert.Equal(t, "deflate", response.Header.Get(httpclient.ContentEncodingHeader))

			reader := flate.NewReader(response.Body)
			body, err := ioutil.ReadAll(reader)
			require.NoError(t, err)

			assert.Equal(t, `{"key": "value"}`, string(body))
		})
	})
}

func TestWithAdditionalHttpHeaders(t *testing.T) {
	reqHeader := http.Header{
		http.CanonicalHeaderKey("X-Other-Key"):       []string{"x-other-value"},
		http.CanonicalHeaderKey("Date"):              []string{"date-value"},
		http.CanonicalHeaderKey("Host"):              []string{"host-value"},
		http.CanonicalHeaderKey("Sec-WebSocket-Key"): []string{"sec-websocket-value"},
		http.CanonicalHeaderKey("User-Agent"):        []string{"user-agent-value"},
		http.CanonicalHeaderKey("Content-Length"):    []string{"content-length-value"},
	}

	t.Run("should add all headers to request without excluded keys", func(t *testing.T) {
		internalExecutionCtx := &internalExecutionContext{
			resolveContext: &resolve.Context{
				Request: resolve.Request{
					Header: nil,
				},
			},
		}

		optionsFn := WithAdditionalHttpHeaders(reqHeader)
		optionsFn(internalExecutionCtx)

		assert.Equal(t, reqHeader, internalExecutionCtx.resolveContext.Request.Header)
	})

	t.Run("should only add headers that are not excluded", func(t *testing.T) {
		internalExecutionCtx := &internalExecutionContext{
			resolveContext: &resolve.Context{
				Request: resolve.Request{
					Header: nil,
				},
			},
		}

		excludableRuntimeHeaders := []string{
			http.CanonicalHeaderKey("Date"),
			http.CanonicalHeaderKey("Host"),
			http.CanonicalHeaderKey("Sec-WebSocket-Key"),
			http.CanonicalHeaderKey("User-Agent"),
			http.CanonicalHeaderKey("Content-Length"),
		}

		optionsFn := WithAdditionalHttpHeaders(reqHeader, excludableRuntimeHeaders...)
		optionsFn(internalExecutionCtx)

		expectedHeaders := http.Header{
			http.CanonicalHeaderKey("X-Other-Key"): []string{"x-other-value"},
		}
		assert.Equal(t, expectedHeaders, internalExecutionCtx.resolveContext.Request.Header)
	})
}

type ExecutionEngineV2TestCase struct {
	schema           *Schema
	operation        func(t *testing.T) Request
	dataSources      []plan.DataSourceConfiguration
	fields           plan.FieldConfigurations
	engineOptions    []ExecutionOptionsV2
	expectedResponse string
}

func TestExecutionEngineV2_Execute(t *testing.T) {
	run := func(testCase ExecutionEngineV2TestCase, withError bool) func(t *testing.T) {
		return func(t *testing.T) {
			engineConf := NewEngineV2Configuration(testCase.schema)
			engineConf.SetDataSources(testCase.dataSources)
			engineConf.SetFieldConfigurations(testCase.fields)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			engine, err := NewExecutionEngineV2(ctx, abstractlogger.Noop{}, engineConf)
			require.NoError(t, err)

			operation := testCase.operation(t)
			resultWriter := NewEngineResultWriter()
			execCtx, execCtxCancel := context.WithCancel(context.Background())
			defer execCtxCancel()
			err = engine.Execute(execCtx, &operation, &resultWriter, testCase.engineOptions...)

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

	t.Run("execute with additional headers", runWithoutError(
		ExecutionEngineV2TestCase{
			schema: starwarsSchema(t),
			operation: func(t *testing.T) Request {
				request := loadStarWarsQuery(starwars.FileSimpleHeroQuery, nil)(t)
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
							URL:    "https://example.com/{{ .request.headers.X-Other-Header }}",
							Method: "GET",
						},
					}),
				},
			},
			fields: []plan.FieldConfiguration{},
			engineOptions: []ExecutionOptionsV2{
				WithAdditionalHttpHeaders(http.Header{
					"X-Other-Header": []string{"foo"},
				}),
			},
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
						HTTPClient: testNetHttpClient(t, roundTripperTestCase{
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
						HTTPClient: testNetHttpClient(t, roundTripperTestCase{
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
						HTTPClient: testNetHttpClient(t, roundTripperTestCase{
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

	t.Run("execute operation with array input type", runWithoutError(ExecutionEngineV2TestCase{
		schema: heroWithArgumentSchema(t),
		operation: func(t *testing.T) Request {
			return Request{
				OperationName: "MyHeroes",
				Variables: stringify(map[string]interface{}{
					"heroNames": []string{"Luke Skywalker", "R2-D2"},
				}),
				Query: `query MyHeroes($heroNames: [String!]!){
						heroes(names: $heroNames)
					}`,
			}
		},
		dataSources: []plan.DataSourceConfiguration{
			{
				RootNodes: []plan.TypeField{
					{TypeName: "Query", FieldNames: []string{"heroes"}},
				},
				Factory: &graphql_datasource.Factory{
					HTTPClient: testNetHttpClient(t, roundTripperTestCase{
						expectedHost:     "example.com",
						expectedPath:     "/",
						expectedBody:     `{"query":"query($heroNames: [String!]!){heroes(names: $heroNames)}","variables":{"heroNames":["Luke Skywalker","R2-D2"]}}`,
						sendResponseBody: `{"data":{"heroes":["Human","Droid"]}}`,
						sendStatusCode:   200,
					}),
				},
				Custom: graphql_datasource.ConfigJson(graphql_datasource.Configuration{
					Fetch: graphql_datasource.FetchConfiguration{
						URL:    "https://example.com/",
						Method: "POST",
					},
				}),
			},
		},
		fields: []plan.FieldConfiguration{
			{
				TypeName:  "Query",
				FieldName: "heroes",
				Path:      []string{"heroes"},
				Arguments: []plan.ArgumentConfiguration{
					{
						Name:       "names",
						SourceType: plan.FieldArgumentSource,
					},
				},
			},
		},
		expectedResponse: `{"data":{"heroes":["Human","Droid"]}}`,
	}))

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
						HTTPClient: testNetHttpClient(t, roundTripperTestCase{
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
							Body:   `{ "name": {{ .arguments.name }} }`,
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
					Arguments: []plan.ArgumentConfiguration{
						{
							Name:         "name",
							RenderConfig: plan.RenderArgumentAsJSONValue,
						},
					},
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
							URL:    "https://example.com/name/{{ .arguments.name }}",
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
					Arguments: []plan.ArgumentConfiguration{
						{
							Name:         "name",
							RenderConfig: plan.RenderArgumentDefault,
						},
					},
				},
			},
			expectedResponse: `{"data":{"hero":"Human"}}`,
		},
	))
}

func TestExecutionEngineV2_FederationAndSubscription_IntegrationTest(t *testing.T) {

	runIntegration := func(t *testing.T, enableDataLoader bool, secondRun bool) {
		t.Helper()
		ctx, cancelFn := context.WithCancel(context.Background())
		setup := newFederationSetup()
		t.Cleanup(func() {
			cancelFn()
			setup.accountsUpstreamServer.Close()
			setup.productsUpstreamServer.Close()
			setup.reviewsUpstreamServer.Close()
			setup.pollingUpstreamServer.Close()
		})

		engine, schema, err := newFederationEngine(ctx, setup, enableDataLoader)
		require.NoError(t, err)

		t.Run("should successfully execute a federation operation", func(t *testing.T) {
			gqlRequest := &Request{
				OperationName: "",
				Variables:     nil,
				Query:         federationExample.QueryReviewsOfMe,
			}

			validationResult, err := gqlRequest.ValidateForSchema(schema)
			require.NoError(t, err)
			require.True(t, validationResult.Valid)

			execCtx, execCtxCancelFn := context.WithCancel(context.Background())
			defer execCtxCancelFn()

			resultWriter := NewEngineResultWriter()
			err = engine.Execute(execCtx, gqlRequest, &resultWriter)
			if assert.NoError(t, err) {
				assert.Equal(t,
					`{"data":{"me":{"reviews":[{"body":"A highly effective form of birth control.","product":{"upc":"top-1","name":"Trilby","price":11}},{"body":"Fedoras are one of the most fashionable hats around and can look great with a variety of outfits.","product":{"upc":"top-2","name":"Fedora","price":22}}]}}}`,
					resultWriter.String(),
				)
			}
		})

		t.Run("should successfully execute a federation subscription", func(t *testing.T) {
			query := `
subscription UpdatedPrice {
  updatedPrice {
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
}`

			gqlRequest := &Request{
				OperationName: "",
				Variables:     nil,
				Query:         query,
			}

			validationResult, err := gqlRequest.ValidateForSchema(schema)
			require.NoError(t, err)
			require.True(t, validationResult.Valid)

			execCtx, execCtxCancelFn := context.WithCancel(context.Background())
			defer execCtxCancelFn()

			message := make(chan string)
			resultWriter := NewEngineResultWriter()
			resultWriter.SetFlushCallback(func(data []byte) {
				message <- string(data)
			})

			go func() {
				err := engine.Execute(execCtx, gqlRequest, &resultWriter)
				assert.NoError(t, err)
			}()

			if assert.NoError(t, err) {
				assert.Eventuallyf(t, func() bool {
					msg := `{"data":{"updatedPrice":{"name":"Trilby","price":%d,"reviews":[{"body":"A highly effective form of birth control.","author":{"id":"1234","username":"User 1234"}}]}}}`
					price := 10
					if secondRun {
						price += 2
					}

					firstMessage := <-message
					expectedFirstMessage := fmt.Sprintf(msg, price)
					assert.Equal(t, expectedFirstMessage, firstMessage)

					secondMessage := <-message
					expectedSecondMessage := fmt.Sprintf(msg, price+1)
					assert.Equal(t, expectedSecondMessage, secondMessage)
					return true
				}, time.Second, 10*time.Millisecond, "did not receive expected messages")
			}
		})

		/* Uncomment when polling subscriptions are ready:

		t.Run("should successfully subscribe to rest data source", func(t *testing.T) {
			gqlRequest := &Request{
				OperationName: "",
				Variables:     nil,
				Query:         "subscription Counter { counter }",
			}

			validationResult, err := gqlRequest.ValidateForSchema(setup.schema)
			require.NoError(t, err)
			require.True(t, validationResult.Valid)

			execCtx, execCtxCancelFn := context.WithCancel(context.Background())
			defer execCtxCancelFn()

			message := make(chan string)
			resultWriter := NewEngineResultWriter()
			resultWriter.SetFlushCallback(func(data []byte) {
				fmt.Println(string(data))
				message <- string(data)
			})

			err = setup.engine.Execute(execCtx, gqlRequest, &resultWriter)
			assert.NoError(t, err)

			if assert.NoError(t, err) {
				assert.Eventuallyf(t, func() bool {
					firstMessage := <-message
					assert.Equal(t, `{"data":{"counter":1}}`, firstMessage)
					secondMessage := <-message
					assert.Equal(t, `{"data":{"counter":2}}`, secondMessage)
					return true
				}, time.Second, 10*time.Millisecond, "did not receive expected messages")
			}
		})
		*/

	}

	t.Run("federation", func(t *testing.T) {
		runIntegration(t, false, false)
	})

	t.Run("federation with data loader enabled", func(t *testing.T) {
		runIntegration(t, true, true)
	})
}

func testNetHttpClient(t *testing.T, testCase roundTripperTestCase) *http.Client {
	defaultClient := httpclient.DefaultNetHttpClient
	return &http.Client{
		Transport:     createTestRoundTripper(t, testCase),
		CheckRedirect: defaultClient.CheckRedirect,
		Jar:           defaultClient.Jar,
		Timeout:       defaultClient.Timeout,
	}
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
					HTTPClient: testNetHttpClient(t, roundTripperTestCase{
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

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	engine, err := NewExecutionEngineV2(ctx, abstractlogger.Noop{}, engineConf)
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

func TestExecutionEngineV2_GetCachedPlan(t *testing.T) {
	schema, err := NewSchemaFromString(testSubscriptionDefinition)
	require.NoError(t, err)

	gqlRequest := Request{
		OperationName: "LastRegisteredUser",
		Variables:     nil,
		Query:         testSubscriptionLastRegisteredUserOperation,
	}

	validationResult, err := gqlRequest.ValidateForSchema(schema)
	require.NoError(t, err)
	require.True(t, validationResult.Valid)

	normalizationResult, err := gqlRequest.Normalize(schema)
	require.NoError(t, err)
	require.True(t, normalizationResult.Successful)

	differentGqlRequest := Request{
		OperationName: "LiveUserCount",
		Variables:     nil,
		Query:         testSubscriptionLiveUserCountOperation,
	}

	validationResult, err = differentGqlRequest.ValidateForSchema(schema)
	require.NoError(t, err)
	require.True(t, validationResult.Valid)

	normalizationResult, err = differentGqlRequest.Normalize(schema)
	require.NoError(t, err)
	require.True(t, normalizationResult.Successful)

	engineConfig := NewEngineV2Configuration(schema)
	engineConfig.SetDataSources([]plan.DataSourceConfiguration{
		{
			RootNodes: []plan.TypeField{
				{
					TypeName:   "Subscription",
					FieldNames: []string{"lastRegisteredUser", "liveUserCount"},
				},
			},
			ChildNodes: []plan.TypeField{
				{
					TypeName:   "User",
					FieldNames: []string{"id", "username", "email"},
				},
			},
			Factory: &graphql_datasource.Factory{},
			Custom: graphql_datasource.ConfigJson(graphql_datasource.Configuration{
				Subscription: graphql_datasource.SubscriptionConfiguration{
					URL: "http://localhost:8080",
				},
			}),
		},
	})

	engine, err := NewExecutionEngineV2(context.Background(), abstractlogger.NoopLogger, engineConfig)
	require.NoError(t, err)

	t.Run("should reuse cached plan", func(t *testing.T) {
		t.Cleanup(engine.executionPlanCache.Purge)
		require.Equal(t, 0, engine.executionPlanCache.Len())

		firstInternalExecCtx := newInternalExecutionContext()
		firstInternalExecCtx.resolveContext.Request.Header = http.Header{
			http.CanonicalHeaderKey("Authorization"): []string{"123abc"},
		}

		report := operationreport.Report{}
		cachedPlan := engine.getCachedPlan(firstInternalExecCtx, &gqlRequest.document, &schema.document, gqlRequest.OperationName, &report)
		_, oldestCachedPlan, _ := engine.executionPlanCache.GetOldest()
		assert.False(t, report.HasErrors())
		assert.Equal(t, 1, engine.executionPlanCache.Len())
		assert.Equal(t, cachedPlan, oldestCachedPlan.(*plan.SubscriptionResponsePlan))

		secondInternalExecCtx := newInternalExecutionContext()
		secondInternalExecCtx.resolveContext.Request.Header = http.Header{
			http.CanonicalHeaderKey("Authorization"): []string{"123abc"},
		}

		cachedPlan = engine.getCachedPlan(secondInternalExecCtx, &gqlRequest.document, &schema.document, gqlRequest.OperationName, &report)
		_, oldestCachedPlan, _ = engine.executionPlanCache.GetOldest()
		assert.False(t, report.HasErrors())
		assert.Equal(t, 1, engine.executionPlanCache.Len())
		assert.Equal(t, cachedPlan, oldestCachedPlan.(*plan.SubscriptionResponsePlan))
	})

	t.Run("should create new plan and cache it", func(t *testing.T) {
		t.Cleanup(engine.executionPlanCache.Purge)
		require.Equal(t, 0, engine.executionPlanCache.Len())

		firstInternalExecCtx := newInternalExecutionContext()
		firstInternalExecCtx.resolveContext.Request.Header = http.Header{
			http.CanonicalHeaderKey("Authorization"): []string{"123abc"},
		}

		report := operationreport.Report{}
		cachedPlan := engine.getCachedPlan(firstInternalExecCtx, &gqlRequest.document, &schema.document, gqlRequest.OperationName, &report)
		_, oldestCachedPlan, _ := engine.executionPlanCache.GetOldest()
		assert.False(t, report.HasErrors())
		assert.Equal(t, 1, engine.executionPlanCache.Len())
		assert.Equal(t, cachedPlan, oldestCachedPlan.(*plan.SubscriptionResponsePlan))

		secondInternalExecCtx := newInternalExecutionContext()
		secondInternalExecCtx.resolveContext.Request.Header = http.Header{
			http.CanonicalHeaderKey("Authorization"): []string{"xyz098"},
		}

		cachedPlan = engine.getCachedPlan(secondInternalExecCtx, &differentGqlRequest.document, &schema.document, differentGqlRequest.OperationName, &report)
		_, oldestCachedPlan, _ = engine.executionPlanCache.GetOldest()
		assert.False(t, report.HasErrors())
		assert.Equal(t, 2, engine.executionPlanCache.Len())
		assert.NotEqual(t, cachedPlan, oldestCachedPlan.(*plan.SubscriptionResponsePlan))
	})
}

func BenchmarkExecutionEngineV2(b *testing.B) {

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

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

		engine, err := NewExecutionEngineV2(ctx, abstractlogger.NoopLogger, engineConf)
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

	ctx = context.Background()
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

type federationSetup struct {
	accountsUpstreamServer *httptest.Server
	productsUpstreamServer *httptest.Server
	reviewsUpstreamServer  *httptest.Server
	pollingUpstreamServer  *httptest.Server
}

func newFederationSetup() *federationSetup {
	return &federationSetup{
		accountsUpstreamServer: httptest.NewServer(accounts.GraphQLEndpointHandler(accounts.TestOptions)),
		productsUpstreamServer: httptest.NewServer(products.GraphQLEndpointHandler(products.TestOptions)),
		reviewsUpstreamServer:  httptest.NewServer(reviews.GraphQLEndpointHandler(reviews.TestOptions)),
		pollingUpstreamServer:  httptest.NewServer(newPollingUpstreamHandler()),
	}
}

func newFederationEngine(ctx context.Context, setup *federationSetup, enableDataLoader bool) (engine *ExecutionEngineV2, schema *Schema, err error) {
	accountsSDL, err := federationExample.LoadSDLFromExamplesDirectoryWithinPkg(federationExample.UpstreamAccounts)
	if err != nil {
		return
	}

	productsSDL, err := federationExample.LoadSDLFromExamplesDirectoryWithinPkg(federationExample.UpstreamProducts)
	if err != nil {
		return
	}

	reviewsSDL, err := federationExample.LoadSDLFromExamplesDirectoryWithinPkg(federationExample.UpstreamReviews)
	if err != nil {
		return
	}

	batchFactory := graphql_datasource.NewBatchFactory()

	accountsDataSource := plan.DataSourceConfiguration{
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
				URL:    setup.accountsUpstreamServer.URL,
				Method: http.MethodPost,
			},
			Federation: graphql_datasource.FederationConfiguration{
				Enabled:    true,
				ServiceSDL: string(accountsSDL),
			},
		}),
		Factory: &graphql_datasource.Factory{
			BatchFactory: batchFactory,
			HTTPClient:   httpclient.DefaultNetHttpClient,
		},
	}

	productsDataSource := plan.DataSourceConfiguration{
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
				URL:    setup.productsUpstreamServer.URL,
				Method: http.MethodPost,
			},
			Subscription: graphql_datasource.SubscriptionConfiguration{
				URL: setup.productsUpstreamServer.URL,
			},
			Federation: graphql_datasource.FederationConfiguration{
				Enabled:    true,
				ServiceSDL: string(productsSDL),
			},
		}),
		Factory: &graphql_datasource.Factory{
			BatchFactory: batchFactory,
			HTTPClient:   httpclient.DefaultNetHttpClient,
		},
	}

	reviewsDataSource := plan.DataSourceConfiguration{
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
				URL:    setup.reviewsUpstreamServer.URL,
				Method: http.MethodPost,
			},
			Subscription: graphql_datasource.SubscriptionConfiguration{
				URL: setup.reviewsUpstreamServer.URL,
			},
			Federation: graphql_datasource.FederationConfiguration{
				Enabled:    true,
				ServiceSDL: string(reviewsSDL),
			},
		}),
		Factory: &graphql_datasource.Factory{
			BatchFactory: batchFactory,
			HTTPClient:   httpclient.DefaultNetHttpClient,
		},
	}

	pollingDataSource := plan.DataSourceConfiguration{
		RootNodes: []plan.TypeField{
			{
				TypeName:   "Subscription",
				FieldNames: []string{"counter"},
			},
		},
		ChildNodes: nil,
		Factory: &rest_datasource.Factory{
			Client: httpclient.DefaultNetHttpClient,
		},
		Custom: rest_datasource.ConfigJSON(rest_datasource.Configuration{
			Fetch: rest_datasource.FetchConfiguration{
				URL:    setup.pollingUpstreamServer.URL,
				Method: http.MethodPost,
			},
			Subscription: rest_datasource.SubscriptionConfiguration{
				PollingIntervalMillis:   10,
				SkipPublishSameResponse: true,
			},
		}),
	}

	fieldConfigs := plan.FieldConfigurations{
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
	}

	schema, err = federationSchema()
	if err != nil {
		return
	}

	engineConfig := NewEngineV2Configuration(schema)
	engineConfig.AddDataSource(accountsDataSource)
	engineConfig.AddDataSource(productsDataSource)
	engineConfig.AddDataSource(reviewsDataSource)
	engineConfig.AddDataSource(pollingDataSource)
	engineConfig.SetFieldConfigurations(fieldConfigs)
	engineConfig.EnableDataLoader(enableDataLoader)

	engine, err = NewExecutionEngineV2(ctx, abstractlogger.Noop{}, engineConfig)
	if err != nil {
		return
	}

	return
}

// nolint
func federationSchema() (*Schema, error) {
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
	counter: Int!
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

	return NewSchemaFromString(rawSchema)
}

func newPollingUpstreamHandler() http.Handler {
	counter := 0
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		counter++
		respBody := fmt.Sprintf(`{"counter":%d}`, counter)
		_, _ = w.Write([]byte(respBody))
	})
}
