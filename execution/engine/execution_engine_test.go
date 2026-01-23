package engine

import (
	"bytes"
	"compress/flate"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"sync"
	"testing"

	"github.com/jensneuse/abstractlogger"
	"github.com/sebdah/goldie/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"

	"github.com/wundergraph/graphql-go-tools/execution/federationtesting"
	"github.com/wundergraph/graphql-go-tools/execution/graphql"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/datasource/graphql_datasource"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/datasource/httpclient"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/datasource/staticdatasource"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/operationreport"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/starwars"
)

type customResolver struct{}

func (customResolver) Resolve(_ *resolve.Context, value []byte) ([]byte, error) {
	return []byte("15"), nil
}

func mustSchemaConfig(t *testing.T, federationConfiguration *graphql_datasource.FederationConfiguration, schema string) *graphql_datasource.SchemaConfiguration {
	t.Helper()

	s, err := graphql_datasource.NewSchemaConfiguration(schema, federationConfiguration)
	require.NoError(t, err)
	return s
}

func mustConfiguration(t *testing.T, input graphql_datasource.ConfigurationInput) graphql_datasource.Configuration {
	t.Helper()

	cfg, err := graphql_datasource.NewConfiguration(input)
	require.NoError(t, err)
	return cfg
}

func mustFactory(t testing.TB, httpClient *http.Client) plan.PlannerFactory[graphql_datasource.Configuration] {
	t.Helper()

	factory, err := graphql_datasource.NewFactory(context.Background(), httpClient, graphql_datasource.NewGraphQLSubscriptionClient(httpClient, httpClient, context.Background()))
	require.NoError(t, err)

	return factory
}

func mustFactoryGRPC(t testing.TB, grpcClient grpc.ClientConnInterface) plan.PlannerFactory[graphql_datasource.Configuration] {
	t.Helper()

	factory, err := graphql_datasource.NewFactoryGRPC(context.Background(), grpcClient)
	require.NoError(t, err)

	return factory
}

func mustGraphqlDataSourceConfiguration(t *testing.T, id string, factory plan.PlannerFactory[graphql_datasource.Configuration], metadata *plan.DataSourceMetadata, customConfig graphql_datasource.Configuration) plan.DataSourceConfiguration[graphql_datasource.Configuration] {
	t.Helper()

	cfg, err := plan.NewDataSourceConfiguration[graphql_datasource.Configuration](
		id,
		factory,
		metadata,
		customConfig,
	)
	require.NoError(t, err)

	return cfg
}

func TestEngineResponseWriter_AsHTTPResponse(t *testing.T) {
	t.Run("no compression", func(t *testing.T) {
		rw := graphql.NewEngineResultWriter()
		_, err := rw.Write([]byte(`{"key": "value"}`))
		require.NoError(t, err)

		headers := make(http.Header)
		headers.Set("Content-Type", "application/json")
		response := rw.AsHTTPResponse(http.StatusOK, headers)
		defer response.Body.Close()
		body, err := io.ReadAll(response.Body)
		require.NoError(t, err)

		assert.Equal(t, http.StatusOK, response.StatusCode)
		assert.Equal(t, "application/json", response.Header.Get("Content-Type"))
		assert.Equal(t, `{"key": "value"}`, string(body))
	})

	t.Run("compression based on content encoding header", func(t *testing.T) {
		rw := graphql.NewEngineResultWriter()
		_, err := rw.Write([]byte(`{"key": "value"}`))
		require.NoError(t, err)

		headers := make(http.Header)
		headers.Set("Content-Type", "application/json")

		t.Run("gzip", func(t *testing.T) {
			headers.Set(httpclient.ContentEncodingHeader, "gzip")

			response := rw.AsHTTPResponse(http.StatusOK, headers)
			defer response.Body.Close()
			assert.Equal(t, http.StatusOK, response.StatusCode)
			assert.Equal(t, "application/json", response.Header.Get("Content-Type"))
			assert.Equal(t, "gzip", response.Header.Get(httpclient.ContentEncodingHeader))

			reader, err := gzip.NewReader(response.Body)
			require.NoError(t, err)

			body, err := io.ReadAll(reader)
			require.NoError(t, err)

			assert.Equal(t, `{"key": "value"}`, string(body))
		})

		t.Run("deflate", func(t *testing.T) {
			headers.Set(httpclient.ContentEncodingHeader, "deflate")

			response := rw.AsHTTPResponse(http.StatusOK, headers)
			defer response.Body.Close()
			assert.Equal(t, http.StatusOK, response.StatusCode)
			assert.Equal(t, "application/json", response.Header.Get("Content-Type"))
			assert.Equal(t, "deflate", response.Header.Get(httpclient.ContentEncodingHeader))

			reader := flate.NewReader(response.Body)
			body, err := io.ReadAll(reader)
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
		c := resolve.NewContext(context.Background())
		c.Request = resolve.Request{
			Header: nil,
		}

		internalExecutionCtx := &internalExecutionContext{
			resolveContext: c,
		}

		optionsFn := WithAdditionalHttpHeaders(reqHeader)
		optionsFn(internalExecutionCtx)

		assert.Equal(t, reqHeader, internalExecutionCtx.resolveContext.Request.Header)
	})

	t.Run("should only add headers that are not excluded", func(t *testing.T) {
		c := resolve.NewContext(context.Background())
		c.Request = resolve.Request{
			Header: nil,
		}

		internalExecutionCtx := &internalExecutionContext{
			resolveContext: c,
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

type ExecutionEngineTestCase struct {
	schema           *graphql.Schema
	operation        func(t *testing.T) graphql.Request
	dataSources      []plan.DataSource
	fields           plan.FieldConfigurations
	engineOptions    []ExecutionOptions
	customResolveMap map[string]resolve.CustomResolve
	skipReason       string
	indentJSON       bool

	expectedResponse     string
	expectedJSONResponse string
	expectedFixture      string
	expectedStaticCost   int
}

type _executionTestOptions struct {
	resolvableOptions resolve.ResolvableOptions

	apolloRouterCompatibilitySubrequestHTTPError bool
	propagateFetchReasons                        bool
	validateRequiredExternalFields               bool
	computeStaticCost                            bool
}

type executionTestOptions func(*_executionTestOptions)

func withValueCompletion() executionTestOptions {
	return func(options *_executionTestOptions) {
		options.resolvableOptions = resolve.ResolvableOptions{
			ApolloCompatibilityValueCompletionInExtensions: true,
		}
	}
}

func withFetchReasons() executionTestOptions {
	return func(options *_executionTestOptions) {
		options.propagateFetchReasons = true
	}
}

func validateRequiredExternalFields() executionTestOptions {
	return func(options *_executionTestOptions) {
		options.validateRequiredExternalFields = true
	}
}

func computeStaticCost() executionTestOptions {
	return func(options *_executionTestOptions) {
		options.computeStaticCost = true
	}
}

func TestExecutionEngine_Execute(t *testing.T) {
	run := func(testCase ExecutionEngineTestCase, withError bool, expectedErrorMessage string, options ...executionTestOptions) func(t *testing.T) {
		t.Helper()

		return func(t *testing.T) {
			t.Helper()

			if testCase.skipReason != "" {
				t.Skip(testCase.skipReason)
			}

			engineConf := NewConfiguration(testCase.schema)
			engineConf.SetDataSources(testCase.dataSources)
			engineConf.SetFieldConfigurations(testCase.fields)
			engineConf.SetCustomResolveMap(testCase.customResolveMap)

			engineConf.plannerConfig.Debug = plan.DebugConfiguration{
				// PrintOperationTransformations: true,
				// PrintPlanningPaths:            true,
				// PrintNodeSuggestions:          true,
				// PrintQueryPlans:               true,
				// ConfigurationVisitor:          true,
				// PlanningVisitor:               true,
				// DatasourceVisitor:             true,
			}

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			var opts _executionTestOptions
			for _, option := range options {
				option(&opts)
			}
			engineConf.plannerConfig.BuildFetchReasons = opts.propagateFetchReasons
			engineConf.plannerConfig.ValidateRequiredExternalFields = opts.validateRequiredExternalFields
			engineConf.plannerConfig.ComputeStaticCost = opts.computeStaticCost
			engineConf.plannerConfig.StaticCostDefaultListSize = 10
			resolveOpts := resolve.ResolverOptions{
				MaxConcurrency:    1024,
				ResolvableOptions: opts.resolvableOptions,
				ApolloRouterCompatibilitySubrequestHTTPError: opts.apolloRouterCompatibilitySubrequestHTTPError,
				PropagateFetchReasons:                        opts.propagateFetchReasons,
				ValidateRequiredExternalFields:               opts.validateRequiredExternalFields,
			}
			engine, err := NewExecutionEngine(ctx, abstractlogger.Noop{}, engineConf, resolveOpts)
			require.NoError(t, err)

			operation := testCase.operation(t)
			resultWriter := graphql.NewEngineResultWriter()
			execCtx, execCtxCancel := context.WithCancel(context.Background())
			defer execCtxCancel()
			err = engine.Execute(execCtx, &operation, &resultWriter, testCase.engineOptions...)
			actualResponse := resultWriter.String()

			if testCase.indentJSON {
				dst := new(bytes.Buffer)
				require.NoError(t, json.Indent(dst, []byte(actualResponse), "", "  "))
				actualResponse = dst.String()
			}

			if testCase.expectedFixture != "" {
				g := goldie.New(t, goldie.WithFixtureDir("testdata"), goldie.WithNameSuffix(".json"))
				g.Assert(t, testCase.expectedFixture, []byte(actualResponse))
				return
			}

			if withError {
				require.Error(t, err)
				if expectedErrorMessage != "" {
					assert.Contains(t, err.Error(), expectedErrorMessage)
				}
			} else {
				require.NoError(t, err)
			}

			if testCase.expectedJSONResponse != "" {
				assert.JSONEq(t, testCase.expectedJSONResponse, actualResponse)
			}

			if testCase.expectedResponse != "" {
				assert.Equal(t, testCase.expectedResponse, actualResponse)
			}

			if testCase.expectedStaticCost != 0 {
				lastPlan := engine.lastPlan
				require.NotNil(t, lastPlan)
				costCalc := lastPlan.GetStaticCostCalculator()
				gotCost := costCalc.GetStaticCost()
				// fmt.Println(costCalc.DebugPrint())
				require.Equal(t, testCase.expectedStaticCost, gotCost)
			}

		}
	}

	runWithAndCompareError := func(testCase ExecutionEngineTestCase, expectedErrorMessage string, options ...executionTestOptions) func(t *testing.T) {
		t.Helper()

		return run(testCase, true, expectedErrorMessage, options...)
	}

	runWithoutError := func(testCase ExecutionEngineTestCase, options ...executionTestOptions) func(t *testing.T) {
		t.Helper()

		return run(testCase, false, "", options...)
	}

	t.Run("apollo router compatibility subrequest HTTP error enabled", runWithoutError(
		ExecutionEngineTestCase{
			schema:    graphql.StarwarsSchema(t),
			operation: graphql.LoadStarWarsQuery(starwars.FileSimpleHeroQuery, nil),
			dataSources: []plan.DataSource{
				mustGraphqlDataSourceConfiguration(t,
					"id",
					mustFactory(t,
						testNetHttpClient(t, roundTripperTestCase{
							expectedHost:     "example.com",
							expectedPath:     "/",
							expectedBody:     "",
							sendResponseBody: `{"errors":[{"message":"Unknown access token","extensions":{"code":"UNAUTHENTICATED"}}]}`,
							sendStatusCode:   403,
						}),
					),
					&plan.DataSourceMetadata{
						RootNodes: []plan.TypeField{
							{
								TypeName:   "Query",
								FieldNames: []string{"hero"},
							},
						},
						ChildNodes: []plan.TypeField{
							{
								TypeName:   "Character",
								FieldNames: []string{"name"},
							},
						},
					},
					mustConfiguration(t, graphql_datasource.ConfigurationInput{
						Fetch: &graphql_datasource.FetchConfiguration{
							URL:    "https://example.com/",
							Method: "GET",
						},
						SchemaConfiguration: mustSchemaConfig(
							t,
							nil,
							string(graphql.StarwarsSchema(t).RawSchema()),
						),
					}),
				),
			},
			expectedJSONResponse: `{
				"data": { "hero": null },
				"errors": [
					{
						"message": "HTTP fetch failed from 'id': 403: Forbidden",
						"path": [],
						"extensions": {
							"code": "SUBREQUEST_HTTP_ERROR",
							"service": "id",
							"reason": "403: Forbidden",
							"http": {
								"status": 403
							}
						}
					},
					{
						"message": "Failed to fetch from Subgraph 'id'."
					}
				]
			}`,
		},
		func(eto *_executionTestOptions) {
			eto.apolloRouterCompatibilitySubrequestHTTPError = true
		},
	))

	t.Run("apollo router compatibility subrequest HTTP error disabled", runWithoutError(
		ExecutionEngineTestCase{
			schema:    graphql.StarwarsSchema(t),
			operation: graphql.LoadStarWarsQuery(starwars.FileSimpleHeroQuery, nil),
			dataSources: []plan.DataSource{
				mustGraphqlDataSourceConfiguration(t,
					"id",
					mustFactory(t,
						testNetHttpClient(t, roundTripperTestCase{
							expectedHost:     "example.com",
							expectedPath:     "/",
							expectedBody:     "",
							sendResponseBody: `{"errors":[{"message":"Unknown access token","extensions":{"code":"UNAUTHENTICATED"}}]}`,
							sendStatusCode:   403,
						}),
					),
					&plan.DataSourceMetadata{
						RootNodes: []plan.TypeField{
							{
								TypeName:   "Query",
								FieldNames: []string{"hero"},
							},
						},
						ChildNodes: []plan.TypeField{
							{
								TypeName:   "Character",
								FieldNames: []string{"name"},
							},
						},
					},
					mustConfiguration(t, graphql_datasource.ConfigurationInput{
						Fetch: &graphql_datasource.FetchConfiguration{
							URL:    "https://example.com/",
							Method: "GET",
						},
						SchemaConfiguration: mustSchemaConfig(
							t,
							nil,
							string(graphql.StarwarsSchema(t).RawSchema()),
						),
					}),
				),
			},
			expectedJSONResponse: `{
				"errors": [
					{
						"message": "Failed to fetch from Subgraph 'id'."
					}
				],
				"data": {
					"hero": null
				}
			}`,
		},
		func(eto *_executionTestOptions) {
			eto.apolloRouterCompatibilitySubrequestHTTPError = false
		},
	))

	t.Run("apollo router compatibility subrequest HTTP error enabled and non-error http status", runWithoutError(
		ExecutionEngineTestCase{
			schema:    graphql.StarwarsSchema(t),
			operation: graphql.LoadStarWarsQuery(starwars.FileSimpleHeroQuery, nil),
			dataSources: []plan.DataSource{
				mustGraphqlDataSourceConfiguration(t,
					"id",
					mustFactory(t,
						testNetHttpClient(t, roundTripperTestCase{
							expectedHost:     "example.com",
							expectedPath:     "/",
							expectedBody:     "",
							sendResponseBody: `{"errors":[{"message":"Unknown access token","extensions":{"code":"UNAUTHENTICATED"}}]}`,
							sendStatusCode:   200,
						}),
					),
					&plan.DataSourceMetadata{
						RootNodes: []plan.TypeField{
							{
								TypeName:   "Query",
								FieldNames: []string{"hero"},
							},
						},
						ChildNodes: []plan.TypeField{
							{
								TypeName:   "Character",
								FieldNames: []string{"name"},
							},
						},
					},
					mustConfiguration(t, graphql_datasource.ConfigurationInput{
						Fetch: &graphql_datasource.FetchConfiguration{
							URL:    "https://example.com/",
							Method: "GET",
						},
						SchemaConfiguration: mustSchemaConfig(
							t,
							nil,
							string(graphql.StarwarsSchema(t).RawSchema()),
						),
					}),
				),
			},
			expectedJSONResponse: `{
				"errors": [
					{
						"message": "Failed to fetch from Subgraph 'id'."
					}
				],
				"data": {
					"hero": null
				}
			}`,
		},
		func(eto *_executionTestOptions) {
			eto.apolloRouterCompatibilitySubrequestHTTPError = true
		},
	))

	t.Run("introspection", func(t *testing.T) {
		schema := graphql.StarwarsSchema(t)

		t.Run("execute type introspection query", runWithoutError(
			ExecutionEngineTestCase{
				schema: schema,
				operation: func(t *testing.T) graphql.Request {
					return graphql.Request{
						OperationName: "myIntrospection",
						Query: `
							query myIntrospection(){
								q: __type(name: "Query") {
									name
									kind
									fields {
										name
									}
								}
								h: __type(name: "Human") {
									name
									fields {
										name
									}
								}
							}
						`,
					}
				},
				expectedResponse: `{"data":{"q":{"name":"Query","kind":"OBJECT","fields":[{"name":"droid"},{"name":"search"},{"name":"searchResults"}]},"h":{"name":"Human","fields":[{"name":"name"},{"name":"friends"}]}}}`,
			},
		))

		t.Run("execute type introspection query for input - without deprecated inputFields", runWithoutError(
			ExecutionEngineTestCase{
				schema: schema,
				operation: func(t *testing.T) graphql.Request {
					return graphql.Request{
						OperationName: "myIntrospection",
						Query: `
							query myIntrospection(){
								__type(name: "ReviewInput") {
									name
									kind
									inputFields {
										name
									}
								}
							}
						`,
					}
				},
				expectedResponse: `{"data":{"__type":{"name":"ReviewInput","kind":"INPUT_OBJECT","inputFields":[{"name":"stars"}]}}}`,
			},
		))

		t.Run("execute type introspection query for input - include deprecated inputFields", runWithoutError(
			ExecutionEngineTestCase{
				schema: schema,
				operation: func(t *testing.T) graphql.Request {
					return graphql.Request{
						OperationName: "myIntrospection",
						Query: `
							query myIntrospection(){
								__type(name: "ReviewInput") {
									name
									kind
									inputFields(includeDeprecated: true) {
										name
									}
								}
							}
						`,
					}
				},
				expectedResponse: `{"data":{"__type":{"name":"ReviewInput","kind":"INPUT_OBJECT","inputFields":[{"name":"stars"},{"name":"commentary"}]}}}`,
			},
		))

		t.Run("execute type introspection query with typenames", runWithoutError(
			ExecutionEngineTestCase{
				schema: schema,
				operation: func(t *testing.T) graphql.Request {
					return graphql.Request{
						OperationName: "myIntrospection",
						Query: `
							query myIntrospection(){
								q: __type(name: "Query") {
									__typename
									name
									kind
									fields {
										__typename
										name
									}
								}
								h: __type(name: "Human") {
									name
									fields {
										name
									}
								}
							}
						`,
					}
				},
				expectedResponse: `{"data":{"q":{"__typename":"__Type","name":"Query","kind":"OBJECT","fields":[{"__typename":"__Field","name":"droid"},{"__typename":"__Field","name":"search"},{"__typename":"__Field","name":"searchResults"}]},"h":{"name":"Human","fields":[{"name":"name"},{"name":"friends"}]}}}`,
			},
		))

		t.Run("execute type introspection query for not existing type", runWithoutError(
			ExecutionEngineTestCase{
				schema: schema,
				operation: func(t *testing.T) graphql.Request {
					return graphql.Request{
						OperationName: "myIntrospection",
						Query: `
							query myIntrospection(){
								__type(name: "NotExisting") {
									name
									kind
									fields {
										name
									}
								}
							}
						`,
					}
				},
				expectedResponse: `{"data":{"__type":null}}`,
			},
		))

		t.Run("execute type introspection query with deprecated fields", runWithoutError(
			ExecutionEngineTestCase{
				schema: schema,
				operation: func(t *testing.T) graphql.Request {
					return graphql.Request{
						OperationName: "myIntrospection",
						Query: `query myIntrospection(){
							__type(name: "Query") {
								name
								kind
								fields(includeDeprecated: true) {
									name
								}
							}
						}`,
					}
				},
				expectedResponse: `{"data":{"__type":{"name":"Query","kind":"OBJECT","fields":[{"name":"hero"},{"name":"droid"},{"name":"search"},{"name":"searchResults"}]}}}`,
			},
		))

		t.Run("execute query typename", runWithoutError(
			ExecutionEngineTestCase{
				schema: schema,
				operation: func(t *testing.T) graphql.Request {
					return graphql.Request{
						OperationName: "my",
						Query: `query my {
							__typename
						}`,
					}
				},
				expectedResponse: `{"data":{"__typename":"Query"}}`,
			},
		))

		t.Run("execute query typename + hero", runWithoutError(
			ExecutionEngineTestCase{
				schema: schema,
				dataSources: []plan.DataSource{
					mustGraphqlDataSourceConfiguration(t,
						"id",
						mustFactory(t,
							testNetHttpClient(t, roundTripperTestCase{
								expectedHost:     "example.com",
								expectedPath:     "/",
								expectedBody:     "",
								sendResponseBody: `{"data":{"hero":{"name":"Luke Skywalker"}}}`,
								sendStatusCode:   200,
							}),
						),
						&plan.DataSourceMetadata{
							RootNodes: []plan.TypeField{
								{
									TypeName:   "Query",
									FieldNames: []string{"hero"},
								},
								{
									TypeName:   "Character",
									FieldNames: []string{"name"},
								},
							},
						},
						mustConfiguration(t, graphql_datasource.ConfigurationInput{
							Fetch: &graphql_datasource.FetchConfiguration{
								URL:    "https://example.com/",
								Method: "GET",
							},
							SchemaConfiguration: mustSchemaConfig(
								t,
								nil,
								string(schema.RawSchema()),
							),
						}),
					),
				},
				operation: func(t *testing.T) graphql.Request {
					return graphql.Request{
						OperationName: "my",
						Query: `query my {
							__typename
							hero {
								name
							}
						}`,
					}
				},
				expectedResponse: `{"data":{"__typename":"Query","hero":{"name":"Luke Skywalker"}}}`,
			},
		))

		t.Run("execute mutation typename", runWithoutError(
			ExecutionEngineTestCase{
				schema: schema,
				operation: func(t *testing.T) graphql.Request {
					return graphql.Request{
						OperationName: "my",
						Query: `mutation my {
							__typename
						}`,
					}
				},
				expectedResponse: `{"data":{"__typename":"Mutation"}}`,
			},
		))

		t.Run("execute full introspection query", runWithoutError(
			ExecutionEngineTestCase{
				schema: schema,
				operation: func(t *testing.T) graphql.Request {
					return graphql.StarwarsRequestForQuery(t, starwars.FileIntrospectionQuery)
				},
				expectedFixture: "full_introspection",
				indentJSON:      true,
			},
		))

		t.Run("execute full introspection query include deprecated: true", runWithoutError(
			ExecutionEngineTestCase{
				schema: schema,
				operation: func(t *testing.T) graphql.Request {
					return graphql.StarwarsRequestForQuery(t, starwars.FileIntrospectionQueryIncludeDeprecated)
				},
				expectedFixture: "full_introspection_with_deprecated",
				indentJSON:      true,
			},
		))

		t.Run("execute full introspection query with typenames", runWithoutError(
			ExecutionEngineTestCase{
				schema: schema,
				operation: func(t *testing.T) graphql.Request {
					return graphql.StarwarsRequestForQuery(t, starwars.FileIntrospectionQueryWithTypenames)
				},
				expectedFixture: "full_introspection_with_typenames",
				indentJSON:      true,
			},
		))
	})

	t.Run("execute simple hero operation with graphql data source", runWithoutError(
		ExecutionEngineTestCase{
			schema:    graphql.StarwarsSchema(t),
			operation: graphql.LoadStarWarsQuery(starwars.FileSimpleHeroQuery, nil),
			dataSources: []plan.DataSource{
				mustGraphqlDataSourceConfiguration(t,
					"id",
					mustFactory(t,
						testNetHttpClient(t, roundTripperTestCase{
							expectedHost:     "example.com",
							expectedPath:     "/",
							expectedBody:     "",
							sendResponseBody: `{"data":{"hero":{"name":"Luke Skywalker"}}}`,
							sendStatusCode:   200,
						}),
					),
					&plan.DataSourceMetadata{
						RootNodes: []plan.TypeField{
							{
								TypeName:   "Query",
								FieldNames: []string{"hero"},
							},
						},
						ChildNodes: []plan.TypeField{
							{
								TypeName:   "Character",
								FieldNames: []string{"name"},
							},
						},
					},
					mustConfiguration(t, graphql_datasource.ConfigurationInput{
						Fetch: &graphql_datasource.FetchConfiguration{
							URL:    "https://example.com/",
							Method: "GET",
						},
						SchemaConfiguration: mustSchemaConfig(
							t,
							nil,
							string(graphql.StarwarsSchema(t).RawSchema()),
						),
					}),
				),
			},
			fields:           []plan.FieldConfiguration{},
			expectedResponse: `{"data":{"hero":{"name":"Luke Skywalker"}}}`,
		},
	))

	t.Run("operation on interface, subgraph expects fetch reasons for all implementing types", runWithoutError(
		ExecutionEngineTestCase{
			schema:    graphql.StarwarsSchema(t),
			operation: graphql.LoadStarWarsQuery(starwars.FileSimpleHeroQuery, nil),
			dataSources: []plan.DataSource{
				mustGraphqlDataSourceConfiguration(t,
					"id",
					mustFactory(t,
						testNetHttpClient(t, roundTripperTestCase{
							expectedHost:     "example.com",
							expectedPath:     "/",
							expectedBody:     `{"query":"{hero {name}}","extensions":{"fetch_reasons":[{"typename":"Character","field":"name","by_user":true},{"typename":"Droid","field":"name","by_user":true},{"typename":"Human","field":"name","by_user":true}]}}`,
							sendResponseBody: `{"data":{"hero":{"name":"Luke Skywalker"}}}`,
							sendStatusCode:   200,
						}),
					),
					&plan.DataSourceMetadata{
						RootNodes: []plan.TypeField{
							{
								TypeName:   "Query",
								FieldNames: []string{"hero"},
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
						ChildNodes: []plan.TypeField{
							{
								TypeName:   "Character",
								FieldNames: []string{"name", "friends"},
								// An interface field implicitly marks all the implementing types.
								FetchReasonFields: []string{"name"},
							},
						},
					},
					mustConfiguration(t, graphql_datasource.ConfigurationInput{
						Fetch: &graphql_datasource.FetchConfiguration{
							URL:    "https://example.com/",
							Method: "POST",
						},
						SchemaConfiguration: mustSchemaConfig(
							t,
							nil,
							string(graphql.StarwarsSchema(t).RawSchema()),
						),
					}),
				),
			},
			fields:           []plan.FieldConfiguration{},
			expectedResponse: `{"data":{"hero":{"name":"Luke Skywalker"}}}`,
		},
		withFetchReasons(),
	))

	t.Run("operation on interface, subgraph expects fetch reasons for one implementing type", runWithoutError(
		ExecutionEngineTestCase{
			schema:    graphql.StarwarsSchema(t),
			operation: graphql.LoadStarWarsQuery(starwars.FileSimpleHeroQuery, nil),
			dataSources: []plan.DataSource{
				mustGraphqlDataSourceConfiguration(t,
					"id",
					mustFactory(t,
						testNetHttpClient(t, roundTripperTestCase{
							expectedHost:     "example.com",
							expectedPath:     "/",
							expectedBody:     `{"query":"{hero {name}}","extensions":{"fetch_reasons":[{"typename":"Droid","field":"name","by_user":true}]}}`,
							sendResponseBody: `{"data":{"hero":{"name":"Droid Number 6"}}}`,
							sendStatusCode:   200,
						}),
					),
					&plan.DataSourceMetadata{
						RootNodes: []plan.TypeField{
							{
								TypeName:   "Query",
								FieldNames: []string{"hero"},
							},
							{
								TypeName:   "Human",
								FieldNames: []string{"name", "height", "friends"},
							},
							{
								TypeName:   "Droid",
								FieldNames: []string{"name", "primaryFunction", "friends"},
								// Only for this field propagate the fetch reasons,
								// even if a user has asked for the interface in the query.
								FetchReasonFields: []string{"name"},
							},
						},
						ChildNodes: []plan.TypeField{
							{
								TypeName:   "Character",
								FieldNames: []string{"name", "friends"},
							},
						},
					},
					mustConfiguration(t, graphql_datasource.ConfigurationInput{
						Fetch: &graphql_datasource.FetchConfiguration{
							URL:    "https://example.com/",
							Method: "POST",
						},
						SchemaConfiguration: mustSchemaConfig(
							t,
							nil,
							string(graphql.StarwarsSchema(t).RawSchema()),
						),
					}),
				),
			},
			fields:           []plan.FieldConfiguration{},
			expectedResponse: `{"data":{"hero":{"name":"Droid Number 6"}}}`,
		},
		withFetchReasons(),
	))

	t.Run("operation on interface, interface and object marked, subgraph expects fetch reasons for one implementing type", runWithoutError(
		ExecutionEngineTestCase{
			schema:    graphql.StarwarsSchema(t),
			operation: graphql.LoadStarWarsQuery(starwars.FileSimpleHeroQuery, nil),
			dataSources: []plan.DataSource{
				mustGraphqlDataSourceConfiguration(t,
					"id",
					mustFactory(t,
						testNetHttpClient(t, roundTripperTestCase{
							expectedHost:     "example.com",
							expectedPath:     "/",
							expectedBody:     `{"query":"{hero {name}}","extensions":{"fetch_reasons":[{"typename":"Character","field":"name","by_user":true},{"typename":"Droid","field":"name","by_user":true},{"typename":"Human","field":"name","by_user":true}]}}`,
							sendResponseBody: `{"data":{"hero":{"name":"Droid Number 6"}}}`,
							sendStatusCode:   200,
						}),
					),
					&plan.DataSourceMetadata{
						RootNodes: []plan.TypeField{
							{
								TypeName:   "Query",
								FieldNames: []string{"hero"},
							},
							{
								TypeName:   "Human",
								FieldNames: []string{"name", "height", "friends"},
							},
							{
								TypeName:          "Droid",
								FieldNames:        []string{"name", "primaryFunction", "friends"},
								FetchReasonFields: []string{"name"}, // implementing is marked
							},
						},
						ChildNodes: []plan.TypeField{
							{
								TypeName:          "Character",
								FieldNames:        []string{"name", "friends"},
								FetchReasonFields: []string{"name"}, // interface is marked
							},
						},
					},
					mustConfiguration(t, graphql_datasource.ConfigurationInput{
						Fetch: &graphql_datasource.FetchConfiguration{
							URL:    "https://example.com/",
							Method: "POST",
						},
						SchemaConfiguration: mustSchemaConfig(
							t,
							nil,
							string(graphql.StarwarsSchema(t).RawSchema()),
						),
					}),
				),
			},
			fields:           []plan.FieldConfiguration{},
			expectedResponse: `{"data":{"hero":{"name":"Droid Number 6"}}}`,
		},
		withFetchReasons(),
	))

	t.Run("operation on fragment, subgraph expects fetch reasons for one implementing type", runWithoutError(
		ExecutionEngineTestCase{
			schema: graphql.StarwarsSchema(t),
			operation: func(t *testing.T) graphql.Request {
				return graphql.Request{
					Query: `query {
						hero {
							...humanFields
						}
					}
					fragment humanFields on Human {
						name
						height
					}`,
				}
			},
			dataSources: []plan.DataSource{
				mustGraphqlDataSourceConfiguration(t,
					"id",
					mustFactory(t,
						testNetHttpClient(t, roundTripperTestCase{
							expectedHost:     "example.com",
							expectedPath:     "/",
							expectedBody:     `{"query":"{hero {__typename ... on Human {name height}}}","extensions":{"fetch_reasons":[{"typename":"Human","field":"name","by_user":true}]}}`,
							sendResponseBody: `{"data":{"hero":{"__typename": "Human", "name":"Luke Skywalker", "height": "1.99"}}}`,
							sendStatusCode:   200,
						}),
					),
					&plan.DataSourceMetadata{
						RootNodes: []plan.TypeField{
							{
								TypeName:   "Query",
								FieldNames: []string{"hero"},
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
						ChildNodes: []plan.TypeField{
							{
								TypeName:   "Character",
								FieldNames: []string{"name", "friends"},
								// Interface is marked, and we should propagate reasons if
								// a user has asked for the fragment on concrete type
								// that is not marked for fetch reasons.
								FetchReasonFields: []string{"name"},
							},
						},
					},
					mustConfiguration(t, graphql_datasource.ConfigurationInput{
						Fetch: &graphql_datasource.FetchConfiguration{
							URL:    "https://example.com/",
							Method: "POST",
						},
						SchemaConfiguration: mustSchemaConfig(
							t,
							nil,
							string(graphql.StarwarsSchema(t).RawSchema()),
						),
					}),
				),
			},
			fields:           []plan.FieldConfiguration{},
			expectedResponse: `{"data":{"hero":{"name":"Luke Skywalker","height":"1.99"}}}`,
		},
		withFetchReasons(),
	))
	t.Run("operation on fragment, subgraph expects fetch reasons for interface and implementing types without dupes", runWithoutError(
		ExecutionEngineTestCase{
			schema: graphql.StarwarsSchema(t),
			operation: func(t *testing.T) graphql.Request {
				return graphql.Request{
					Query: `query {
						hero {
							...humanFields
							name
						}
					}
					fragment humanFields on Human {
						name
					}`,
				}
			},
			dataSources: []plan.DataSource{
				mustGraphqlDataSourceConfiguration(t,
					"id",
					mustFactory(t,
						testNetHttpClient(t, roundTripperTestCase{
							expectedHost:     "example.com",
							expectedPath:     "/",
							expectedBody:     `{"query":"{hero {__typename ... on Human {name} name}}","extensions":{"fetch_reasons":[{"typename":"Character","field":"name","by_user":true},{"typename":"Droid","field":"name","by_user":true},{"typename":"Human","field":"name","by_user":true}]}}`,
							sendResponseBody: `{"data":{"hero":{"__typename": "Human", "name":"Luke Skywalker"}}}`,
							sendStatusCode:   200,
						}),
					),
					&plan.DataSourceMetadata{
						RootNodes: []plan.TypeField{
							{
								TypeName:   "Query",
								FieldNames: []string{"hero"},
							},
							{
								TypeName:          "Human",
								FieldNames:        []string{"name", "height", "friends"},
								FetchReasonFields: []string{"name"}, // implementing is marked
							},
							{
								TypeName:          "Droid",
								FieldNames:        []string{"name", "primaryFunction", "friends"},
								FetchReasonFields: []string{"name"}, // implementing is marked
							},
						},
						ChildNodes: []plan.TypeField{
							{
								TypeName:          "Character",
								FieldNames:        []string{"name", "friends"},
								FetchReasonFields: []string{"name"}, // interface is marked
							},
						},
					},
					mustConfiguration(t, graphql_datasource.ConfigurationInput{
						Fetch: &graphql_datasource.FetchConfiguration{
							URL:    "https://example.com/",
							Method: "POST",
						},
						SchemaConfiguration: mustSchemaConfig(
							t,
							nil,
							string(graphql.StarwarsSchema(t).RawSchema()),
						),
					}),
				),
			},
			fields:           []plan.FieldConfiguration{},
			expectedResponse: `{"data":{"hero":{"name":"Luke Skywalker"}}}`,
		},
		withFetchReasons(),
	))

	t.Run("execute simple hero operation with graphql data source and empty errors list", runWithoutError(
		ExecutionEngineTestCase{
			schema:    graphql.StarwarsSchema(t),
			operation: graphql.LoadStarWarsQuery(starwars.FileSimpleHeroQuery, nil),
			dataSources: []plan.DataSource{
				mustGraphqlDataSourceConfiguration(t,
					"id",
					mustFactory(t,
						testNetHttpClient(t, roundTripperTestCase{
							expectedHost:     "example.com",
							expectedPath:     "/",
							expectedBody:     "",
							sendResponseBody: `{"data":{"hero":{"name":"Luke Skywalker"}}, "errors": []}`,
							sendStatusCode:   200,
						}),
					),
					&plan.DataSourceMetadata{
						RootNodes: []plan.TypeField{
							{
								TypeName:   "Query",
								FieldNames: []string{"hero"},
							},
						},
						ChildNodes: []plan.TypeField{
							{
								TypeName:   "Character",
								FieldNames: []string{"name"},
							},
						},
					},
					mustConfiguration(t, graphql_datasource.ConfigurationInput{
						Fetch: &graphql_datasource.FetchConfiguration{
							URL:    "https://example.com/",
							Method: "GET",
						},
						SchemaConfiguration: mustSchemaConfig(
							t,
							nil,
							string(graphql.StarwarsSchema(t).RawSchema()),
						),
					}),
				),
			},
			fields:           []plan.FieldConfiguration{},
			expectedResponse: `{"data":{"hero":{"name":"Luke Skywalker"}}}`,
		},
	))

	t.Run("execute the correct operation when sending multiple queries", runWithoutError(
		ExecutionEngineTestCase{
			schema: graphql.StarwarsSchema(t),
			operation: func(t *testing.T) graphql.Request {
				request := graphql.LoadStarWarsQuery(starwars.FileMultiQueries, nil)(t)
				request.OperationName = "SingleHero"
				return request
			},
			dataSources: []plan.DataSource{
				mustGraphqlDataSourceConfiguration(t,
					"id",
					mustFactory(t,
						testNetHttpClient(t, roundTripperTestCase{
							expectedHost:     "example.com",
							expectedPath:     "/",
							expectedBody:     "",
							sendResponseBody: `{"data":{"hero":{"name":"Luke Skywalker"}}}`,
							sendStatusCode:   200,
						}),
					),
					&plan.DataSourceMetadata{
						RootNodes: []plan.TypeField{
							{
								TypeName:   "Query",
								FieldNames: []string{"hero"},
							},
						},
						ChildNodes: []plan.TypeField{
							{
								TypeName:   "Character",
								FieldNames: []string{"name"},
							},
						},
					},
					mustConfiguration(t, graphql_datasource.ConfigurationInput{
						Fetch: &graphql_datasource.FetchConfiguration{
							URL:    "https://example.com/",
							Method: "GET",
						},
						SchemaConfiguration: mustSchemaConfig(
							t,
							nil,
							string(graphql.StarwarsSchema(t).RawSchema()),
						),
					}),
				),
			},
			fields:           []plan.FieldConfiguration{},
			expectedResponse: `{"data":{"hero":{"name":"Luke Skywalker"}}}`,
		},
	))

	schemaWithCustomScalar, _ := graphql.NewSchemaFromString(`
    scalar Long
    type Asset {
      id: Long!
    }
    type Query {
      asset: Asset
    }
  `)
	t.Run("query with custom scalar", runWithoutError(
		ExecutionEngineTestCase{
			schema: schemaWithCustomScalar,
			operation: func(t *testing.T) graphql.Request {
				return graphql.Request{
					Query: `{asset{id}}`,
				}
			},
			dataSources: []plan.DataSource{
				mustGraphqlDataSourceConfiguration(t,
					"id",
					mustFactory(t,
						testNetHttpClient(t, roundTripperTestCase{
							expectedHost:     "example.com",
							expectedPath:     "/",
							expectedBody:     "",
							sendResponseBody: `{"data":{"asset":{"id":1}}}`,
							sendStatusCode:   200,
						}),
					),
					&plan.DataSourceMetadata{
						RootNodes: []plan.TypeField{
							{
								TypeName:   "Query",
								FieldNames: []string{"asset"},
							},
						},
						ChildNodes: []plan.TypeField{
							{
								TypeName:   "Asset",
								FieldNames: []string{"id"},
							},
						},
					},
					mustConfiguration(t, graphql_datasource.ConfigurationInput{
						Fetch: &graphql_datasource.FetchConfiguration{
							URL:    "https://example.com/",
							Method: "GET",
						},
						SchemaConfiguration: mustSchemaConfig(
							t,
							nil,
							string(schemaWithCustomScalar.RawSchema()),
						),
					}),
				),
			},
			customResolveMap: map[string]resolve.CustomResolve{
				"Long": &customResolver{},
			},
			expectedResponse: `{"data":{"asset":{"id":15}}}`,
		},
	))

	t.Run("execute operation with variables for arguments", runWithoutError(
		ExecutionEngineTestCase{
			schema:    graphql.StarwarsSchema(t),
			operation: graphql.LoadStarWarsQuery(starwars.FileDroidWithArgAndVarQuery, map[string]interface{}{"droidID": "R2D2"}),
			dataSources: []plan.DataSource{
				mustGraphqlDataSourceConfiguration(t,
					"id",
					mustFactory(t,
						testNetHttpClient(t, roundTripperTestCase{
							expectedHost:     "example.com",
							expectedPath:     "/",
							expectedBody:     "",
							sendResponseBody: `{"data":{"droid":{"name":"R2D2"}}}`,
							sendStatusCode:   200,
						}),
					),
					&plan.DataSourceMetadata{
						RootNodes: []plan.TypeField{
							{
								TypeName:   "Query",
								FieldNames: []string{"droid"},
							},
						},
						ChildNodes: []plan.TypeField{
							{
								TypeName:   "Droid",
								FieldNames: []string{"name"},
							},
						},
					},
					mustConfiguration(t, graphql_datasource.ConfigurationInput{
						Fetch: &graphql_datasource.FetchConfiguration{
							URL:    "https://example.com/",
							Method: "GET",
						},
						SchemaConfiguration: mustSchemaConfig(
							t,
							nil,
							string(graphql.StarwarsSchema(t).RawSchema()),
						),
					}),
				),
			},
			fields: []plan.FieldConfiguration{
				{
					TypeName:  "Query",
					FieldName: "droid",
					Path:      []string{"droid"},
					Arguments: []plan.ArgumentConfiguration{
						{
							Name:         "id",
							SourceType:   plan.FieldArgumentSource,
							RenderConfig: plan.RenderArgumentAsGraphQLValue,
						},
					},
				},
			},
			expectedResponse: `{"data":{"droid":{"name":"R2D2"}}}`,
		},
	))

	t.Run("execute operation with array input type", runWithoutError(ExecutionEngineTestCase{
		schema: heroWithArgumentSchema(t),
		operation: func(t *testing.T) graphql.Request {
			return graphql.Request{
				OperationName: "MyHeroes",
				Variables: stringify(map[string]interface{}{
					"heroNames": []string{"Luke Skywalker", "R2-D2"},
				}),
				Query: `query MyHeroes($heroNames: [String!]!){
						heroes(names: $heroNames)
					}`,
			}
		},
		dataSources: []plan.DataSource{
			mustGraphqlDataSourceConfiguration(t,
				"id",
				mustFactory(t,
					testNetHttpClient(t, roundTripperTestCase{
						expectedHost:     "example.com",
						expectedPath:     "/",
						expectedBody:     `{"query":"query($heroNames: [String!]!){heroes(names: $heroNames)}","variables":{"heroNames":["Luke Skywalker","R2-D2"]}}`,
						sendResponseBody: `{"data":{"heroes":["Human","Droid"]}}`,
						sendStatusCode:   200,
					}),
				),
				&plan.DataSourceMetadata{
					RootNodes: []plan.TypeField{
						{TypeName: "Query", FieldNames: []string{"heroes"}},
					},
				},
				mustConfiguration(t, graphql_datasource.ConfigurationInput{
					Fetch: &graphql_datasource.FetchConfiguration{
						URL:    "https://example.com/",
						Method: "POST",
					},
					SchemaConfiguration: mustSchemaConfig(
						t,
						nil,
						string(heroWithArgumentSchema(t).RawSchema()),
					),
				}),
			),
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

	t.Run("execute operation with null and omitted input variables", runWithoutError(ExecutionEngineTestCase{
		schema: func(t *testing.T) *graphql.Schema {
			t.Helper()
			schema := `
			type Query {
				heroes(names: [String!], height: String): [String!]
			}`
			parseSchema, err := graphql.NewSchemaFromString(schema)
			require.NoError(t, err)
			return parseSchema
		}(t),
		operation: func(t *testing.T) graphql.Request {
			return graphql.Request{
				OperationName: "MyHeroes",
				Variables:     []byte(`{"height": null}`),
				Query: `query MyHeroes($heroNames: [String!], $height: String){
						heroes(names: $heroNames, height: $height)
					}`,
			}
		},
		dataSources: []plan.DataSource{
			mustGraphqlDataSourceConfiguration(t,
				"id",
				mustFactory(t,
					testNetHttpClient(t, roundTripperTestCase{
						expectedHost:     "example.com",
						expectedPath:     "/",
						expectedBody:     `{"query":"query($heroNames: [String!], $height: String){heroes(names: $heroNames, height: $height)}","variables":{"height":null}}`,
						sendResponseBody: `{"data":{"heroes":[]}}`,
						sendStatusCode:   200,
					}),
				),
				&plan.DataSourceMetadata{
					RootNodes: []plan.TypeField{
						{TypeName: "Query", FieldNames: []string{"heroes"}},
					},
				},
				mustConfiguration(t, graphql_datasource.ConfigurationInput{
					Fetch: &graphql_datasource.FetchConfiguration{
						URL:    "https://example.com/",
						Method: "POST",
					},
					SchemaConfiguration: mustSchemaConfig(
						t,
						nil,
						`type Query { heroes(names: [String!], height: String): [String!] }`,
					),
				}),
			),
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
					{
						Name:       "height",
						SourceType: plan.FieldArgumentSource,
					},
				},
			},
		},
		expectedResponse: `{"data":{"heroes":[]}}`,
	}))

	t.Run("execute operation with null variable on required type", runWithAndCompareError(ExecutionEngineTestCase{
		schema: func(t *testing.T) *graphql.Schema {
			t.Helper()
			schema := `
			type Query {
				hero(name: String!): String!
			}`
			parseSchema, err := graphql.NewSchemaFromString(schema)
			require.NoError(t, err)
			return parseSchema
		}(t),
		operation: func(t *testing.T) graphql.Request {
			return graphql.Request{
				OperationName: "MyHero",
				Variables:     []byte(`{"heroName": null}`),
				Query: `query MyHero($heroName: String!){
						hero(name: $heroName)
					}`,
			}
		},
		dataSources: []plan.DataSource{
			mustGraphqlDataSourceConfiguration(t,
				"id",
				mustFactory(t, http.DefaultClient),
				&plan.DataSourceMetadata{
					RootNodes: []plan.TypeField{
						{TypeName: "Query", FieldNames: []string{"hero"}},
					},
				},
				mustConfiguration(t, graphql_datasource.ConfigurationInput{
					Fetch: &graphql_datasource.FetchConfiguration{
						URL:    "https://example.com/",
						Method: "POST",
					},
					SchemaConfiguration: mustSchemaConfig(
						t,
						nil,
						`type Query { hero(name: String!): String! }`,
					),
				}),
			),
		},
		fields: []plan.FieldConfiguration{
			{
				TypeName:  "Query",
				FieldName: "hero",
				Path:      []string{"hero"},
				Arguments: []plan.ArgumentConfiguration{
					{
						Name:       "name",
						SourceType: plan.FieldArgumentSource,
					},
				},
			},
		},
	},
		`Variable "$heroName" got invalid value null; Expected non-nullable type "String!" not to be null.`,
	))

	t.Run("execute operation with all fields skipped", runWithoutError(ExecutionEngineTestCase{
		schema: func(t *testing.T) *graphql.Schema {
			t.Helper()
			schema := `
			type Query {
				hero(name: String!): String!
			}`
			parseSchema, err := graphql.NewSchemaFromString(schema)
			require.NoError(t, err)
			return parseSchema
		}(t),
		operation: func(t *testing.T) graphql.Request {
			return graphql.Request{
				OperationName: "MyHero",
				Variables:     []byte(`{"heroName": null}`),
				Query: `query MyHero($heroName: String!){
						hero(name: $heroName) @skip(if: true)
					}`,
			}
		},
		dataSources: []plan.DataSource{
			mustGraphqlDataSourceConfiguration(t,
				"id",
				mustFactory(t,
					testNetHttpClient(t, roundTripperTestCase{
						expectedHost:     "example.com",
						expectedPath:     "/",
						expectedBody:     "",
						sendResponseBody: `{"data":{"__internal__typename_placeholder":"Query"}}`,
						sendStatusCode:   200,
					}),
				),
				&plan.DataSourceMetadata{
					RootNodes: []plan.TypeField{
						{TypeName: "Query", FieldNames: []string{"hero"}},
					},
				},
				mustConfiguration(t, graphql_datasource.ConfigurationInput{
					Fetch: &graphql_datasource.FetchConfiguration{
						URL:    "https://example.com/",
						Method: "POST",
					},
					SchemaConfiguration: mustSchemaConfig(
						t,
						nil,
						`type Query { hero(name: String!): String! }`,
					),
				}),
			),
		},
		fields: []plan.FieldConfiguration{
			{
				TypeName:  "Query",
				FieldName: "hero",
				Path:      []string{"hero"},
				Arguments: []plan.ArgumentConfiguration{
					{
						Name:       "name",
						SourceType: plan.FieldArgumentSource,
					},
				},
			},
		},
		expectedResponse: `{"data":{}}`,
	}))

	t.Run("execute operation and apply input coercion for lists without variables", runWithoutError(ExecutionEngineTestCase{
		schema: graphql.InputCoercionForListSchema(t),
		operation: func(t *testing.T) graphql.Request {
			return graphql.Request{
				OperationName: "",
				Variables:     stringify(map[string]interface{}{}),
				Query: `query{
						charactersByIds(ids: 1) {
							name
						}
					}`,
			}
		},
		dataSources: []plan.DataSource{
			mustGraphqlDataSourceConfiguration(t,
				"id",
				mustFactory(t,
					testNetHttpClient(t, roundTripperTestCase{
						expectedHost:     "example.com",
						expectedPath:     "/",
						expectedBody:     `{"query":"query($a: [Int]){charactersByIds(ids: $a){name}}","variables":{"a":[1]}}`,
						sendResponseBody: `{"data":{"charactersByIds":[{"name": "Luke"}]}}`,
						sendStatusCode:   200,
					}),
				),
				&plan.DataSourceMetadata{
					RootNodes: []plan.TypeField{
						{
							TypeName:   "Query",
							FieldNames: []string{"charactersByIds"},
						},
					},
					ChildNodes: []plan.TypeField{
						{
							TypeName:   "Character",
							FieldNames: []string{"name"},
						},
					},
				},
				mustConfiguration(t, graphql_datasource.ConfigurationInput{
					Fetch: &graphql_datasource.FetchConfiguration{
						URL:    "https://example.com/",
						Method: "POST",
					},
					SchemaConfiguration: mustSchemaConfig(
						t,
						nil,
						string(graphql.InputCoercionForListSchema(t).RawSchema()),
					),
				}),
			),
		},
		fields: []plan.FieldConfiguration{
			{
				TypeName:  "Query",
				FieldName: "charactersByIds",
				Path:      []string{"charactersByIds"},
				Arguments: []plan.ArgumentConfiguration{
					{
						Name:         "ids",
						SourceType:   plan.FieldArgumentSource,
						RenderConfig: plan.RenderArgumentAsGraphQLValue,
					},
				},
			},
		},
		expectedResponse: `{"data":{"charactersByIds":[{"name":"Luke"}]}}`,
	}))

	t.Run("execute operation and apply input coercion for lists with variable extraction", runWithoutError(ExecutionEngineTestCase{
		schema: graphql.InputCoercionForListSchema(t),
		operation: func(t *testing.T) graphql.Request {
			return graphql.Request{
				OperationName: "",
				Variables: stringify(map[string]interface{}{
					"ids": 1,
				}),
				Query: `query($ids: [Int]) { charactersByIds(ids: $ids) { name } }`,
			}
		},
		dataSources: []plan.DataSource{
			mustGraphqlDataSourceConfiguration(t,
				"id",
				mustFactory(t,
					testNetHttpClient(t, roundTripperTestCase{
						expectedHost:     "example.com",
						expectedPath:     "/",
						expectedBody:     `{"query":"query($ids: [Int]){charactersByIds(ids: $ids){name}}","variables":{"ids":[1]}}`,
						sendResponseBody: `{"data":{"charactersByIds":[{"name": "Luke"}]}}`,
						sendStatusCode:   200,
					}),
				),
				&plan.DataSourceMetadata{
					RootNodes: []plan.TypeField{
						{
							TypeName:   "Query",
							FieldNames: []string{"charactersByIds"},
						},
					},
					ChildNodes: []plan.TypeField{
						{
							TypeName:   "Character",
							FieldNames: []string{"name"},
						},
					},
				},
				mustConfiguration(t, graphql_datasource.ConfigurationInput{
					Fetch: &graphql_datasource.FetchConfiguration{
						URL:    "https://example.com/",
						Method: "POST",
					},
					SchemaConfiguration: mustSchemaConfig(
						t,
						nil,
						string(graphql.InputCoercionForListSchema(t).RawSchema()),
					),
				}),
			),
		},
		fields: []plan.FieldConfiguration{
			{
				TypeName:  "Query",
				FieldName: "charactersByIds",
				Path:      []string{"charactersByIds"},
				Arguments: []plan.ArgumentConfiguration{
					{
						Name:         "ids",
						SourceType:   plan.FieldArgumentSource,
						RenderConfig: plan.RenderArgumentAsGraphQLValue,
					},
				},
			},
		},
		expectedResponse: `{"data":{"charactersByIds":[{"name":"Luke"}]}}`,
	}))

	t.Run("execute operation with arguments", runWithoutError(
		ExecutionEngineTestCase{
			schema:    graphql.StarwarsSchema(t),
			operation: graphql.LoadStarWarsQuery(starwars.FileDroidWithArgQuery, nil),
			dataSources: []plan.DataSource{
				mustGraphqlDataSourceConfiguration(t,
					"id",
					mustFactory(t,
						testNetHttpClient(t, roundTripperTestCase{
							expectedHost:     "example.com",
							expectedPath:     "/",
							expectedBody:     "",
							sendResponseBody: `{"data":{"droid":{"name":"R2D2"}}}`,
							sendStatusCode:   200,
						}),
					),
					&plan.DataSourceMetadata{
						RootNodes: []plan.TypeField{
							{
								TypeName:   "Query",
								FieldNames: []string{"droid"},
							},
						},
						ChildNodes: []plan.TypeField{
							{
								TypeName:   "Droid",
								FieldNames: []string{"name"},
							},
						},
					},
					mustConfiguration(t, graphql_datasource.ConfigurationInput{
						Fetch: &graphql_datasource.FetchConfiguration{
							URL:    "https://example.com/",
							Method: "GET",
						},
						SchemaConfiguration: mustSchemaConfig(
							t,
							nil,
							string(graphql.StarwarsSchema(t).RawSchema()),
						),
					}),
				),
			},
			fields: []plan.FieldConfiguration{
				{
					TypeName:  "Query",
					FieldName: "droid",
					Arguments: []plan.ArgumentConfiguration{
						{
							Name:         "id",
							SourceType:   plan.FieldArgumentSource,
							RenderConfig: plan.RenderArgumentAsGraphQLValue,
						},
					},
				},
			},
			expectedResponse: `{"data":{"droid":{"name":"R2D2"}}}`,
		},
	))

	t.Run("execute operation with default arguments", func(t *testing.T) {
		t.Run("query variables with default value", runWithoutError(
			ExecutionEngineTestCase{
				schema: heroWithArgumentSchema(t),
				operation: func(t *testing.T) graphql.Request {
					return graphql.Request{
						OperationName: "queryVariables",
						Variables:     nil,
						Query: `query queryVariables($name: String! = "R2D2", $nameOptional: String = "R2D2") {
						  hero(name: $name)
 						  hero2: hero(name: $nameOptional)
						}`,
					}
				},
				dataSources: []plan.DataSource{
					mustGraphqlDataSourceConfiguration(t,
						"id",
						mustFactory(t,
							testNetHttpClient(t, roundTripperTestCase{
								expectedHost:     "example.com",
								expectedPath:     "/",
								expectedBody:     `{"query":"query($name: String!, $nameOptional: String){hero(name: $name) hero2: hero(name: $nameOptional)}","variables":{"nameOptional":"R2D2","name":"R2D2"}}`,
								sendResponseBody: `{"data":{"hero":"R2D2","hero2":"R2D2"}}`,
								sendStatusCode:   200,
							}),
						),
						&plan.DataSourceMetadata{
							RootNodes: []plan.TypeField{
								{TypeName: "Query", FieldNames: []string{"hero"}},
							},
						},
						mustConfiguration(t, graphql_datasource.ConfigurationInput{
							Fetch: &graphql_datasource.FetchConfiguration{
								URL:    "https://example.com/",
								Method: "GET",
							},
							SchemaConfiguration: mustSchemaConfig(
								t,
								nil,
								string(heroWithArgumentSchema(t).RawSchema()),
							),
						}),
					),
				},
				fields: []plan.FieldConfiguration{
					{
						TypeName:  "Query",
						FieldName: "hero",
						Path:      []string{"hero"},
						Arguments: []plan.ArgumentConfiguration{
							{
								Name:       "name",
								SourceType: plan.FieldArgumentSource,
							},
						},
					},
				},
				expectedResponse: `{"data":{"hero":"R2D2","hero2":"R2D2"}}`,
			},
		))

		t.Run("query variables with default value when args provided", runWithoutError(
			ExecutionEngineTestCase{
				schema: heroWithArgumentSchema(t),
				operation: func(t *testing.T) graphql.Request {
					return graphql.Request{
						OperationName: "queryVariables",
						Variables: stringify(map[string]interface{}{
							"name":         "Luke",
							"nameOptional": "Skywalker",
						}),
						Query: `query queryVariables($name: String! = "R2D2", $nameOptional: String = "R2D2") {
						  hero(name: $name)
 						  hero2: hero(name: $nameOptional)
						}`,
					}
				},
				dataSources: []plan.DataSource{
					mustGraphqlDataSourceConfiguration(t,
						"id",
						mustFactory(t,
							testNetHttpClient(t, roundTripperTestCase{
								expectedHost:     "example.com",
								expectedPath:     "/",
								expectedBody:     `{"query":"query($name: String!, $nameOptional: String){hero(name: $name) hero2: hero(name: $nameOptional)}","variables":{"nameOptional":"Skywalker","name":"Luke"}}`,
								sendResponseBody: `{"data":{"hero":"R2D2","hero2":"R2D2"}}`,
								sendStatusCode:   200,
							}),
						),
						&plan.DataSourceMetadata{
							RootNodes: []plan.TypeField{
								{TypeName: "Query", FieldNames: []string{"hero"}},
							},
						},
						mustConfiguration(t, graphql_datasource.ConfigurationInput{
							Fetch: &graphql_datasource.FetchConfiguration{
								URL:    "https://example.com/",
								Method: "GET",
							},
							SchemaConfiguration: mustSchemaConfig(
								t,
								nil,
								string(heroWithArgumentSchema(t).RawSchema()),
							),
						}),
					),
				},
				fields: []plan.FieldConfiguration{
					{
						TypeName:  "Query",
						FieldName: "hero",
						Path:      []string{"hero"},
						Arguments: []plan.ArgumentConfiguration{
							{
								Name:       "name",
								SourceType: plan.FieldArgumentSource,
							},
						},
					},
				},
				expectedResponse: `{"data":{"hero":"R2D2","hero2":"R2D2"}}`,
			},
		))

		t.Run("query variables with default values for fields with required and optional args", runWithoutError(
			ExecutionEngineTestCase{
				schema: heroWithArgumentSchema(t),
				operation: func(t *testing.T) graphql.Request {
					return graphql.Request{
						OperationName: "queryVariables",
						Variables:     nil,
						Query: `query queryVariables($name: String! = "R2D2", $nameOptional: String = "R2D2") {
						  hero: heroDefault(name: $name)
 						  hero2: heroDefault(name: $nameOptional)
						  hero3: heroDefaultRequired(name: $name)
 						  hero4: heroDefaultRequired(name: $nameOptional)
						}`,
					}
				},
				dataSources: []plan.DataSource{
					mustGraphqlDataSourceConfiguration(t,
						"id",
						mustFactory(t,
							testNetHttpClient(t, roundTripperTestCase{
								expectedHost:     "example.com",
								expectedPath:     "/",
								expectedBody:     `{"query":"query($name: String!, $nameOptional: String!){hero: heroDefault(name: $name) hero2: heroDefault(name: $nameOptional) hero3: heroDefaultRequired(name: $name) hero4: heroDefaultRequired(name: $nameOptional)}","variables":{"nameOptional":"R2D2","name":"R2D2"}}`,
								sendResponseBody: `{"data":{"hero":"R2D2","hero2":"R2D2","hero3":"R2D2","hero4":"R2D2"}}`,
								sendStatusCode:   200,
							}),
						),
						&plan.DataSourceMetadata{
							RootNodes: []plan.TypeField{
								{TypeName: "Query", FieldNames: []string{"heroDefault", "heroDefaultRequired"}},
							},
						},
						mustConfiguration(t, graphql_datasource.ConfigurationInput{
							Fetch: &graphql_datasource.FetchConfiguration{
								URL:    "https://example.com/",
								Method: "GET",
							},
							SchemaConfiguration: mustSchemaConfig(
								t,
								nil,
								string(heroWithArgumentSchema(t).RawSchema()),
							),
						}),
					),
				},
				fields: []plan.FieldConfiguration{
					{
						TypeName:  "Query",
						FieldName: "heroDefault",
						Path:      []string{"heroDefault"},
						Arguments: []plan.ArgumentConfiguration{
							{
								Name:       "name",
								SourceType: plan.FieldArgumentSource,
							},
						},
					},
					{
						TypeName:  "Query",
						FieldName: "heroDefaultRequired",
						Path:      []string{"heroDefaultRequired"},
						Arguments: []plan.ArgumentConfiguration{
							{
								Name:       "name",
								SourceType: plan.FieldArgumentSource,
							},
						},
					},
				},
				expectedResponse: `{"data":{"hero":"R2D2","hero2":"R2D2","hero3":"R2D2","hero4":"R2D2"}}`,
			},
		))

		t.Run("query fields with default value", runWithoutError(
			ExecutionEngineTestCase{
				schema: heroWithArgumentSchema(t),
				operation: func(t *testing.T) graphql.Request {
					return graphql.Request{
						OperationName: "fieldArgs",
						Variables:     nil,
						Query: `query fieldArgs {
						  heroDefault
 						  heroDefaultRequired
						}`,
					}
				},
				dataSources: []plan.DataSource{
					mustGraphqlDataSourceConfiguration(t,
						"id",
						mustFactory(t,
							testNetHttpClient(t, roundTripperTestCase{
								expectedHost:     "example.com",
								expectedPath:     "/",
								expectedBody:     `{"query":"{heroDefault heroDefaultRequired}"}`,
								sendResponseBody: `{"data":{"heroDefault":"R2D2","heroDefaultRequired":"R2D2"}}`,
								sendStatusCode:   200,
							}),
						),
						&plan.DataSourceMetadata{
							RootNodes: []plan.TypeField{
								{TypeName: "Query", FieldNames: []string{"heroDefault", "heroDefaultRequired"}},
							},
						},
						mustConfiguration(t, graphql_datasource.ConfigurationInput{
							Fetch: &graphql_datasource.FetchConfiguration{
								URL:    "https://example.com/",
								Method: "GET",
							},
							SchemaConfiguration: mustSchemaConfig(
								t,
								nil,
								string(heroWithArgumentSchema(t).RawSchema()),
							),
						}),
					),
				},
				fields: []plan.FieldConfiguration{
					{
						TypeName:  "Query",
						FieldName: "heroDefault",
						Path:      []string{"heroDefault"},
						Arguments: []plan.ArgumentConfiguration{
							{
								Name:       "name",
								SourceType: plan.FieldArgumentSource,
							},
						},
					},
					{
						TypeName:  "Query",
						FieldName: "heroDefaultRequired",
						Path:      []string{"heroDefaultRequired"},
						Arguments: []plan.ArgumentConfiguration{
							{
								Name:       "name",
								SourceType: plan.FieldArgumentSource,
							},
						},
					},
				},
				expectedResponse: `{"data":{"heroDefault":"R2D2","heroDefaultRequired":"R2D2"}}`,
			},
		))

	})

	t.Run("execute query with data source on field with interface return type", runWithoutError(
		ExecutionEngineTestCase{
			schema: graphql.CreateCountriesSchema(t),
			operation: func(t *testing.T) graphql.Request {
				return graphql.Request{
					OperationName: "",
					Variables:     nil,
					Query:         `{ codeType { code ...on Country { name } } }`,
				}
			},
			dataSources: []plan.DataSource{
				mustGraphqlDataSourceConfiguration(t,
					"id",
					mustFactory(t,
						testNetHttpClient(t, roundTripperTestCase{
							expectedHost:     "example.com",
							expectedPath:     "/",
							expectedBody:     `{"query":"{codeType {code __typename ... on Country {name}}}"}`,
							sendResponseBody: `{"data":{"codeType":{"__typename":"Country","code":"de","name":"Germany"}}}`,
							sendStatusCode:   200,
						}),
					),
					&plan.DataSourceMetadata{
						RootNodes: []plan.TypeField{
							{TypeName: "Query", FieldNames: []string{"codeType"}},
						},
						ChildNodes: []plan.TypeField{
							{
								TypeName:   "Continent",
								FieldNames: []string{"code", "name", "countries"},
							},
							{
								TypeName:   "Country",
								FieldNames: []string{"code", "name", "native", "phone", "continent", "capital", "currency", "languages", "emoji", "emojiU", "states"},
							},
							{
								TypeName:   "Language",
								FieldNames: []string{"code", "name", "native", "rtl"},
							},
							{
								TypeName:   "State",
								FieldNames: []string{"code", "name", "country"},
							},
							{
								TypeName:   "CodeNameType",
								FieldNames: []string{"code", "name"},
							},
							{
								TypeName:   "CodeType",
								FieldNames: []string{"code"},
							},
						},
					},
					mustConfiguration(t, graphql_datasource.ConfigurationInput{
						Fetch: &graphql_datasource.FetchConfiguration{
							URL:    "https://example.com/",
							Method: "GET",
						},
						SchemaConfiguration: mustSchemaConfig(
							t,
							nil,
							graphql.CountriesSchema,
						),
					}),
				),
			},
			fields:           []plan.FieldConfiguration{},
			expectedResponse: `{"data":{"codeType":{"code":"de","name":"Germany"}}}`,
		},
	))

	t.Run("Spreading a fragment on an invalid type returns ErrInvalidFragmentSpread", runWithAndCompareError(
		ExecutionEngineTestCase{
			schema:    graphql.StarwarsSchema(t),
			operation: graphql.LoadStarWarsQuery(starwars.FileInvalidFragmentsQuery, nil),
			dataSources: []plan.DataSource{
				mustGraphqlDataSourceConfiguration(t,
					"id",
					mustFactory(t,
						testNetHttpClient(t, roundTripperTestCase{
							expectedHost:     "example.com",
							expectedPath:     "/",
							expectedBody:     "",
							sendResponseBody: `{"data":{"droid":{"name":"R2D2"}}}`,
							sendStatusCode:   200,
						}),
					),
					&plan.DataSourceMetadata{
						RootNodes: []plan.TypeField{
							{
								TypeName:   "Query",
								FieldNames: []string{"droid"},
							},
						},
						ChildNodes: []plan.TypeField{
							{
								TypeName:   "Droid",
								FieldNames: []string{"name"},
							},
						},
					},
					mustConfiguration(t, graphql_datasource.ConfigurationInput{
						Fetch: &graphql_datasource.FetchConfiguration{
							URL:    "https://example.com/",
							Method: "GET",
						},
						SchemaConfiguration: mustSchemaConfig(
							t,
							nil,
							string(graphql.StarwarsSchema(t).RawSchema()),
						),
					}),
				),
			},
			fields: []plan.FieldConfiguration{
				{
					TypeName:  "Query",
					FieldName: "droid",
					Arguments: []plan.ArgumentConfiguration{
						{
							Name:         "id",
							SourceType:   plan.FieldArgumentSource,
							RenderConfig: plan.RenderArgumentAsGraphQLValue,
						},
					},
				},
			},
			expectedResponse: ``,
		},
		"fragment spread: fragment reviewFields must be spread on type Review and not type Droid, locations: [], path: [query,droid]",
	))

	t.Run("execute the correct operation when sending multiple queries", runWithoutError(
		ExecutionEngineTestCase{
			schema: graphql.StarwarsSchema(t),
			operation: func(t *testing.T) graphql.Request {
				request := graphql.LoadStarWarsQuery(starwars.FileInterfaceFragmentsOnUnion, nil)(t)
				request.OperationName = "SearchResults"
				return request
			},
			dataSources: []plan.DataSource{
				mustGraphqlDataSourceConfiguration(t,
					"id",
					mustFactory(t,
						testNetHttpClient(t, roundTripperTestCase{
							expectedHost:     "example.com",
							expectedPath:     "/",
							expectedBody:     "",
							sendResponseBody: `{"data":{"searchResults":[{"name":"Luke Skywalker"},{"length":13.37}]}}`,
							sendStatusCode:   200,
						}),
					),
					&plan.DataSourceMetadata{
						RootNodes: []plan.TypeField{
							{
								TypeName:   "Query",
								FieldNames: []string{"searchResults"},
							},
						},
						ChildNodes: []plan.TypeField{
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
							{
								TypeName:   "Character",
								FieldNames: []string{"name"},
							},
							{
								TypeName:   "Vehicle",
								FieldNames: []string{"length"},
							},
						},
					},
					mustConfiguration(t, graphql_datasource.ConfigurationInput{
						Fetch: &graphql_datasource.FetchConfiguration{
							URL:    "https://example.com/",
							Method: "GET",
						},
						SchemaConfiguration: mustSchemaConfig(
							t,
							nil,
							string(graphql.StarwarsSchema(t).RawSchema()),
						),
					}),
				),
			},
			fields:           []plan.FieldConfiguration{},
			expectedResponse: `{"data":{"searchResults":[{},{}]}}`,
		},
	))

	t.Run("invalid and inaccessible enum values", func(t *testing.T) {
		schema, err := graphql.NewSchemaFromString(enumSDL)
		require.NoError(t, err)

		t.Run("invalid non-nullable enum input", run(
			ExecutionEngineTestCase{
				schema: schema,
				operation: func(t *testing.T) graphql.Request {
					return graphql.Request{
						OperationName: "Enum",
						Query: `query Enum($enum: Enum!) {
							enum(enum: $enum)
						}`,
						Variables: []byte(`{"enum":"INVALID"}`),
					}
				},
				dataSources: []plan.DataSource{
					mustGraphqlDataSourceConfiguration(t,
						"id",
						mustFactory(t,
							testNetHttpClient(t, roundTripperTestCase{
								expectedHost:     "example.com",
								expectedPath:     "/",
								expectedBody:     "",
								sendResponseBody: `{"data":{"enum":"A"}}`,
								sendStatusCode:   200,
							}),
						),
						&plan.DataSourceMetadata{
							RootNodes: []plan.TypeField{
								{
									TypeName:   "Query",
									FieldNames: []string{"enum"},
								},
							},
						},
						mustConfiguration(t, graphql_datasource.ConfigurationInput{
							Fetch: &graphql_datasource.FetchConfiguration{
								URL:    "https://example.com/",
								Method: "GET",
							},
							SchemaConfiguration: mustSchemaConfig(
								t,
								nil,
								enumSDL,
							),
						}),
					),
				},
				fields: []plan.FieldConfiguration{
					{
						TypeName:  "Query",
						FieldName: "enum",
						Path:      []string{"enum"},
						Arguments: []plan.ArgumentConfiguration{
							{
								Name:         "enum",
								SourceType:   plan.FieldArgumentSource,
								RenderConfig: plan.RenderArgumentAsGraphQLValue,
							},
						},
					},
				},
				expectedResponse: ``,
			}, true, `Variable "$enum" got invalid value "INVALID"; Value "INVALID" does not exist in "Enum" enum.`,
		))

		t.Run("nested invalid non-nullable enum input", run(
			ExecutionEngineTestCase{
				schema: schema,
				operation: func(t *testing.T) graphql.Request {
					return graphql.Request{
						OperationName: "Enum",
						Query: `query Enum($enum: Enum!) {
							nestedEnums {
								enum(enum: $enum)
							}
						}`,
						Variables: []byte(`{"enum":"INVALID"}`),
					}
				},
				dataSources: []plan.DataSource{
					mustGraphqlDataSourceConfiguration(t,
						"id",
						mustFactory(t,
							testNetHttpClient(t, roundTripperTestCase{
								expectedHost:     "example.com",
								expectedPath:     "/",
								expectedBody:     "",
								sendResponseBody: `{"data":{"nestedEnums":{"enum":"A"}}}`,
								sendStatusCode:   200,
							}),
						),
						&plan.DataSourceMetadata{
							RootNodes: []plan.TypeField{
								{
									TypeName:   "Query",
									FieldNames: []string{"nestedEnums"},
								},
							},
							ChildNodes: []plan.TypeField{
								{
									TypeName:   "Object",
									FieldNames: []string{"enum"},
								},
							},
						},
						mustConfiguration(t, graphql_datasource.ConfigurationInput{
							Fetch: &graphql_datasource.FetchConfiguration{
								URL:    "https://example.com/",
								Method: "GET",
							},
							SchemaConfiguration: mustSchemaConfig(
								t,
								nil,
								enumSDL,
							),
						}),
					),
				},
				fields: []plan.FieldConfiguration{
					{
						TypeName:  "Object",
						FieldName: "enum",
						Path:      []string{"enum"},
						Arguments: []plan.ArgumentConfiguration{
							{
								Name:         "enum",
								SourceType:   plan.FieldArgumentSource,
								RenderConfig: plan.RenderArgumentAsGraphQLValue,
							},
						},
					},
				},
				expectedResponse: ``,
			}, true, `Variable "$enum" got invalid value "INVALID"; Value "INVALID" does not exist in "Enum" enum.`,
		))

		t.Run("invalid nullable enum input", run(
			ExecutionEngineTestCase{
				schema: schema,
				operation: func(t *testing.T) graphql.Request {
					return graphql.Request{
						OperationName: "Enum",
						Query: `query Enum($enum: Enum) {
							nullableEnum(enum: $enum)
						}`,
						Variables: []byte(`{"enum":"INVALID"}`),
					}
				},
				dataSources: []plan.DataSource{
					mustGraphqlDataSourceConfiguration(t,
						"id",
						mustFactory(t,
							testNetHttpClient(t, roundTripperTestCase{
								expectedHost:     "example.com",
								expectedPath:     "/",
								expectedBody:     "",
								sendResponseBody: `{"data":{"enum":"INVALID"}}`,
								sendStatusCode:   200,
							}),
						),
						&plan.DataSourceMetadata{
							RootNodes: []plan.TypeField{
								{
									TypeName:   "Query",
									FieldNames: []string{"nullableEnum"},
								},
							},
						},
						mustConfiguration(t, graphql_datasource.ConfigurationInput{
							Fetch: &graphql_datasource.FetchConfiguration{
								URL:    "https://example.com/",
								Method: "GET",
							},
							SchemaConfiguration: mustSchemaConfig(
								t,
								nil,
								enumSDL,
							),
						}),
					),
				},
				fields: []plan.FieldConfiguration{
					{
						TypeName:  "Query",
						FieldName: "nullableEnum",
						Path:      []string{"nullableEnum"},
						Arguments: []plan.ArgumentConfiguration{
							{
								Name:         "enum",
								SourceType:   plan.FieldArgumentSource,
								RenderConfig: plan.RenderArgumentAsGraphQLValue,
							},
						},
					},
				},
				expectedResponse: ``,
			}, true, `Variable "$enum" got invalid value "INVALID"; Value "INVALID" does not exist in "Enum" enum.`,
		))

		t.Run("nested invalid nullable enum input", run(
			ExecutionEngineTestCase{
				schema: schema,
				operation: func(t *testing.T) graphql.Request {
					return graphql.Request{
						OperationName: "Enum",
						Query: `query Enum($enum: Enum) {
							nestedEnums {
								nullableEnum(enum: $enum)
							}
						}`,
						Variables: []byte(`{"enum":"INVALID"}`),
					}
				},
				dataSources: []plan.DataSource{
					mustGraphqlDataSourceConfiguration(t,
						"id",
						mustFactory(t,
							testNetHttpClient(t, roundTripperTestCase{
								expectedHost:     "example.com",
								expectedPath:     "/",
								expectedBody:     "",
								sendResponseBody: `{"data":{"nestedEnums":{"nullableEnum":"A"}}}`,
								sendStatusCode:   200,
							}),
						),
						&plan.DataSourceMetadata{
							RootNodes: []plan.TypeField{
								{
									TypeName:   "Query",
									FieldNames: []string{"nestedEnums"},
								},
							},
							ChildNodes: []plan.TypeField{
								{
									TypeName:   "Object",
									FieldNames: []string{"nullableEnum"},
								},
							},
						},
						mustConfiguration(t, graphql_datasource.ConfigurationInput{
							Fetch: &graphql_datasource.FetchConfiguration{
								URL:    "https://example.com/",
								Method: "GET",
							},
							SchemaConfiguration: mustSchemaConfig(
								t,
								nil,
								enumSDL,
							),
						}),
					),
				},
				fields: []plan.FieldConfiguration{
					{
						TypeName:  "Object",
						FieldName: "nullableEnum",
						Path:      []string{"nullableEnum"},
						Arguments: []plan.ArgumentConfiguration{
							{
								Name:         "enum",
								SourceType:   plan.FieldArgumentSource,
								RenderConfig: plan.RenderArgumentAsGraphQLValue,
							},
						},
					},
				},
				expectedResponse: ``,
			}, true, `Variable "$enum" got invalid value "INVALID"; Value "INVALID" does not exist in "Enum" enum.`,
		))

		t.Run("invalid non-string enum value", runWithoutError(
			ExecutionEngineTestCase{
				schema: schema,
				operation: func(t *testing.T) graphql.Request {
					return graphql.Request{
						OperationName: "Enum",
						Query: `query Enum {
							nullableEnum
						}`,
					}
				},
				dataSources: []plan.DataSource{
					mustGraphqlDataSourceConfiguration(t,
						"id",
						mustFactory(t,
							testNetHttpClient(t, roundTripperTestCase{
								expectedHost:     "example.com",
								expectedPath:     "/",
								expectedBody:     "",
								sendResponseBody: `{"data":{"nullableEnum":1}}`,
								sendStatusCode:   200,
							}),
						),
						&plan.DataSourceMetadata{
							RootNodes: []plan.TypeField{
								{
									TypeName:   "Query",
									FieldNames: []string{"nullableEnum"},
								},
							},
						},
						mustConfiguration(t, graphql_datasource.ConfigurationInput{
							Fetch: &graphql_datasource.FetchConfiguration{
								URL:    "https://example.com/",
								Method: "GET",
							},
							SchemaConfiguration: mustSchemaConfig(
								t,
								nil,
								enumSDL,
							),
						}),
					),
				},
				fields:           []plan.FieldConfiguration{},
				expectedResponse: `{"errors":[{"message":"Enum \"Enum\" cannot represent value: 1","path":["nullableEnum"],"extensions":{"code":"INTERNAL_SERVER_ERROR"}}],"data":null}`,
			},
		))

		t.Run("nested invalid non-string enum value", runWithoutError(
			ExecutionEngineTestCase{
				schema: schema,
				operation: func(t *testing.T) graphql.Request {
					return graphql.Request{
						OperationName: "Enum",
						Query: `query Enum($enum: Enum) {
							nestedEnums {
								nullableEnum(enum: $enum)
							}
						}`,
						Variables: []byte(`{"enum":"A"}`),
					}
				},
				dataSources: []plan.DataSource{
					mustGraphqlDataSourceConfiguration(t,
						"id",
						mustFactory(t,
							testNetHttpClient(t, roundTripperTestCase{
								expectedHost:     "example.com",
								expectedPath:     "/",
								expectedBody:     "",
								sendResponseBody: `{"data":{"nestedEnums":{"nullableEnum":1}}}`,
								sendStatusCode:   200,
							}),
						),
						&plan.DataSourceMetadata{
							RootNodes: []plan.TypeField{
								{
									TypeName:   "Query",
									FieldNames: []string{"nestedEnums"},
								},
							},
							ChildNodes: []plan.TypeField{
								{
									TypeName:   "Object",
									FieldNames: []string{"nullableEnum"},
								},
							},
						},
						mustConfiguration(t, graphql_datasource.ConfigurationInput{
							Fetch: &graphql_datasource.FetchConfiguration{
								URL:    "https://example.com/",
								Method: "GET",
							},
							SchemaConfiguration: mustSchemaConfig(
								t,
								nil,
								enumSDL,
							),
						}),
					),
				},
				fields: []plan.FieldConfiguration{
					{
						TypeName:  "Object",
						FieldName: "nullableEnum",
						Path:      []string{"nullableEnum"},
						Arguments: []plan.ArgumentConfiguration{
							{
								Name:         "enum",
								SourceType:   plan.FieldArgumentSource,
								RenderConfig: plan.RenderArgumentAsGraphQLValue,
							},
						},
					},
				},
				expectedResponse: `{"errors":[{"message":"Enum \"Enum\" cannot represent value: 1","path":["nestedEnums","nullableEnum"],"extensions":{"code":"INTERNAL_SERVER_ERROR"}}],"data":null}`,
			},
		))

		t.Run("invalid non-nullable enum value", runWithoutError(
			ExecutionEngineTestCase{
				schema: schema,
				operation: func(t *testing.T) graphql.Request {
					return graphql.Request{
						OperationName: "Enum",
						Query: `query Enum($enum: Enum!) {
							enum(enum: $enum)
						}`,
						Variables: []byte(`{"enum":"A"}`),
					}
				},
				dataSources: []plan.DataSource{
					mustGraphqlDataSourceConfiguration(t,
						"id",
						mustFactory(t,
							testNetHttpClient(t, roundTripperTestCase{
								expectedHost:     "example.com",
								expectedPath:     "/",
								expectedBody:     "",
								sendResponseBody: `{"data":{"enum":"INVALID"}}`,
								sendStatusCode:   200,
							}),
						),
						&plan.DataSourceMetadata{
							RootNodes: []plan.TypeField{
								{
									TypeName:   "Query",
									FieldNames: []string{"enum"},
								},
							},
						},
						mustConfiguration(t, graphql_datasource.ConfigurationInput{
							Fetch: &graphql_datasource.FetchConfiguration{
								URL:    "https://example.com/",
								Method: "GET",
							},
							SchemaConfiguration: mustSchemaConfig(
								t,
								nil,
								enumSDL,
							),
						}),
					),
				},
				fields: []plan.FieldConfiguration{
					{
						TypeName:  "Query",
						FieldName: "enum",
						Path:      []string{"enum"},
						Arguments: []plan.ArgumentConfiguration{
							{
								Name:         "enum",
								SourceType:   plan.FieldArgumentSource,
								RenderConfig: plan.RenderArgumentAsGraphQLValue,
							},
						},
					},
				},
				expectedResponse: `{"errors":[{"message":"Enum \"Enum\" cannot represent value: \"INVALID\"","path":["enum"],"extensions":{"code":"INTERNAL_SERVER_ERROR"}}],"data":null}`,
			},
		))

		t.Run("nested invalid non-nullable enum value", runWithoutError(
			ExecutionEngineTestCase{
				schema: schema,
				operation: func(t *testing.T) graphql.Request {
					return graphql.Request{
						OperationName: "Enum",
						Query: `query Enum($enum: Enum!) {
							nestedEnums {
								enum(enum: $enum)
							}
						}`,
						Variables: []byte(`{"enum":"A"}`),
					}
				},
				dataSources: []plan.DataSource{
					mustGraphqlDataSourceConfiguration(t,
						"id",
						mustFactory(t,
							testNetHttpClient(t, roundTripperTestCase{
								expectedHost:     "example.com",
								expectedPath:     "/",
								expectedBody:     "",
								sendResponseBody: `{"data":{"nestedEnums":{"enum":"INVALID"}}}`,
								sendStatusCode:   200,
							}),
						),
						&plan.DataSourceMetadata{
							RootNodes: []plan.TypeField{
								{
									TypeName:   "Query",
									FieldNames: []string{"nestedEnums"},
								},
							},
							ChildNodes: []plan.TypeField{
								{
									TypeName:   "Object",
									FieldNames: []string{"enum"},
								},
							},
						},
						mustConfiguration(t, graphql_datasource.ConfigurationInput{
							Fetch: &graphql_datasource.FetchConfiguration{
								URL:    "https://example.com/",
								Method: "GET",
							},
							SchemaConfiguration: mustSchemaConfig(
								t,
								nil,
								enumSDL,
							),
						}),
					),
				},
				fields: []plan.FieldConfiguration{
					{
						TypeName:  "Object",
						FieldName: "enum",
						Path:      []string{"enum"},
						Arguments: []plan.ArgumentConfiguration{
							{
								Name:         "enum",
								SourceType:   plan.FieldArgumentSource,
								RenderConfig: plan.RenderArgumentAsGraphQLValue,
							},
						},
					},
				},
				expectedResponse: `{"errors":[{"message":"Enum \"Enum\" cannot represent value: \"INVALID\"","path":["nestedEnums","enum"],"extensions":{"code":"INTERNAL_SERVER_ERROR"}}],"data":null}`,
			},
		))

		t.Run("invalid nullable enum value", runWithoutError(
			ExecutionEngineTestCase{
				schema: schema,
				operation: func(t *testing.T) graphql.Request {
					return graphql.Request{
						OperationName: "Enum",
						Query: `query Enum($enum: Enum!) {
							nullableEnum(enum: $enum)
						}`,
						Variables: []byte(`{"enum":"A"}`),
					}
				},
				dataSources: []plan.DataSource{
					mustGraphqlDataSourceConfiguration(t,
						"id",
						mustFactory(t,
							testNetHttpClient(t, roundTripperTestCase{
								expectedHost:     "example.com",
								expectedPath:     "/",
								expectedBody:     "",
								sendResponseBody: `{"data":{"nullableEnum":"INVALID"}}`,
								sendStatusCode:   200,
							}),
						),
						&plan.DataSourceMetadata{
							RootNodes: []plan.TypeField{
								{
									TypeName:   "Query",
									FieldNames: []string{"nullableEnum"},
								},
							},
						},
						mustConfiguration(t, graphql_datasource.ConfigurationInput{
							Fetch: &graphql_datasource.FetchConfiguration{
								URL:    "https://example.com/",
								Method: "GET",
							},
							SchemaConfiguration: mustSchemaConfig(
								t,
								nil,
								enumSDL,
							),
						}),
					),
				},
				fields: []plan.FieldConfiguration{
					{
						TypeName:  "Query",
						FieldName: "nullableEnum",
						Path:      []string{"nullableEnum"},
						Arguments: []plan.ArgumentConfiguration{
							{
								Name:         "enum",
								SourceType:   plan.FieldArgumentSource,
								RenderConfig: plan.RenderArgumentAsGraphQLValue,
							},
						},
					},
				},
				expectedResponse: `{"errors":[{"message":"Enum \"Enum\" cannot represent value: \"INVALID\"","path":["nullableEnum"],"extensions":{"code":"INTERNAL_SERVER_ERROR"}}],"data":{"nullableEnum":null}}`,
			},
		))

		t.Run("nested invalid nullable enum value", runWithoutError(
			ExecutionEngineTestCase{
				schema: schema,
				operation: func(t *testing.T) graphql.Request {
					return graphql.Request{
						OperationName: "Enum",
						Query: `query Enum($enum: Enum) {
							nestedEnums {
								nullableEnum(enum: $enum)
							}
						}`,
						Variables: []byte(`{"enum":"A"}`),
					}
				},
				dataSources: []plan.DataSource{
					mustGraphqlDataSourceConfiguration(t,
						"id",
						mustFactory(t,
							testNetHttpClient(t, roundTripperTestCase{
								expectedHost:     "example.com",
								expectedPath:     "/",
								expectedBody:     "",
								sendResponseBody: `{"data":{"nestedEnums":{"nullableEnum":"INVALID"}}}`,
								sendStatusCode:   200,
							}),
						),
						&plan.DataSourceMetadata{
							RootNodes: []plan.TypeField{
								{
									TypeName:   "Query",
									FieldNames: []string{"nestedEnums"},
								},
							},
							ChildNodes: []plan.TypeField{
								{
									TypeName:   "Object",
									FieldNames: []string{"nullableEnum"},
								},
							},
						},
						mustConfiguration(t, graphql_datasource.ConfigurationInput{
							Fetch: &graphql_datasource.FetchConfiguration{
								URL:    "https://example.com/",
								Method: "GET",
							},
							SchemaConfiguration: mustSchemaConfig(
								t,
								nil,
								enumSDL,
							),
						}),
					),
				},
				fields: []plan.FieldConfiguration{
					{
						TypeName:  "Object",
						FieldName: "nullableEnum",
						Path:      []string{"nullableEnum"},
						Arguments: []plan.ArgumentConfiguration{
							{
								Name:         "enum",
								SourceType:   plan.FieldArgumentSource,
								RenderConfig: plan.RenderArgumentAsGraphQLValue,
							},
						},
					},
				},
				expectedResponse: `{"errors":[{"message":"Enum \"Enum\" cannot represent value: \"INVALID\"","path":["nestedEnums","nullableEnum"],"extensions":{"code":"INTERNAL_SERVER_ERROR"}}],"data":{"nestedEnums":{"nullableEnum":null}}}`,
			},
		))

		t.Run("invalid non-nullable enum value returned by list", runWithoutError(
			ExecutionEngineTestCase{
				schema: schema,
				operation: func(t *testing.T) graphql.Request {
					return graphql.Request{
						OperationName: "Enums",
						Query: `query Enums {
							enums
						}`,
					}
				},
				dataSources: []plan.DataSource{
					mustGraphqlDataSourceConfiguration(t,
						"id",
						mustFactory(t,
							testNetHttpClient(t, roundTripperTestCase{
								expectedHost:     "example.com",
								expectedPath:     "/",
								expectedBody:     "",
								sendResponseBody: `{"data":{"enums":["A","B","INVALID"]}}`,
								sendStatusCode:   200,
							}),
						),
						&plan.DataSourceMetadata{
							RootNodes: []plan.TypeField{
								{
									TypeName:   "Query",
									FieldNames: []string{"enums"},
								},
							},
						},
						mustConfiguration(t, graphql_datasource.ConfigurationInput{
							Fetch: &graphql_datasource.FetchConfiguration{
								URL:    "https://example.com/",
								Method: "GET",
							},
							SchemaConfiguration: mustSchemaConfig(
								t,
								nil,
								enumSDL,
							),
						}),
					),
				},
				fields:           []plan.FieldConfiguration{},
				expectedResponse: `{"errors":[{"message":"Enum \"Enum\" cannot represent value: \"INVALID\"","path":["enums",2],"extensions":{"code":"INTERNAL_SERVER_ERROR"}}],"data":null}`,
			},
		))

		t.Run("nested invalid non-nullable enum value returned by list", runWithoutError(
			ExecutionEngineTestCase{
				schema: schema,
				operation: func(t *testing.T) graphql.Request {
					return graphql.Request{
						OperationName: "Enums",
						Query: `query Enums {
							nestedEnums {
								enums
							}
						}`,
					}
				},
				dataSources: []plan.DataSource{
					mustGraphqlDataSourceConfiguration(t,
						"id",
						mustFactory(t,
							testNetHttpClient(t, roundTripperTestCase{
								expectedHost:     "example.com",
								expectedPath:     "/",
								expectedBody:     "",
								sendResponseBody: `{"data":{"nestedEnums":{"enums":["A","B","INVALID"]}}}`,
								sendStatusCode:   200,
							}),
						),
						&plan.DataSourceMetadata{
							RootNodes: []plan.TypeField{
								{
									TypeName:   "Query",
									FieldNames: []string{"nestedEnums"},
								},
							},
							ChildNodes: []plan.TypeField{
								{
									TypeName:   "Object",
									FieldNames: []string{"enums"},
								},
							},
						},
						mustConfiguration(t, graphql_datasource.ConfigurationInput{
							Fetch: &graphql_datasource.FetchConfiguration{
								URL:    "https://example.com/",
								Method: "GET",
							},
							SchemaConfiguration: mustSchemaConfig(
								t,
								nil,
								enumSDL,
							),
						}),
					),
				},
				fields:           []plan.FieldConfiguration{},
				expectedResponse: `{"errors":[{"message":"Enum \"Enum\" cannot represent value: \"INVALID\"","path":["nestedEnums","enums",2],"extensions":{"code":"INTERNAL_SERVER_ERROR"}}],"data":null}`,
			},
		))

		t.Run("invalid nullable enum value returned by list", runWithoutError(
			ExecutionEngineTestCase{
				schema: schema,
				operation: func(t *testing.T) graphql.Request {
					return graphql.Request{
						OperationName: "Enums",
						Query: `query Enums {
							nullableEnums
						}`,
					}
				},
				dataSources: []plan.DataSource{
					mustGraphqlDataSourceConfiguration(t,
						"id",
						mustFactory(t,
							testNetHttpClient(t, roundTripperTestCase{
								expectedHost:     "example.com",
								expectedPath:     "/",
								expectedBody:     "",
								sendResponseBody: `{"data":{"nullableEnums":["A","INVALID","B"]}}`,
								sendStatusCode:   200,
							}),
						),
						&plan.DataSourceMetadata{
							RootNodes: []plan.TypeField{
								{
									TypeName:   "Query",
									FieldNames: []string{"nullableEnums"},
								},
							},
						},
						mustConfiguration(t, graphql_datasource.ConfigurationInput{
							Fetch: &graphql_datasource.FetchConfiguration{
								URL:    "https://example.com/",
								Method: "GET",
							},
							SchemaConfiguration: mustSchemaConfig(
								t,
								nil,
								enumSDL,
							),
						}),
					),
				},
				fields:           []plan.FieldConfiguration{},
				expectedResponse: `{"errors":[{"message":"Enum \"Enum\" cannot represent value: \"INVALID\"","path":["nullableEnums",1],"extensions":{"code":"INTERNAL_SERVER_ERROR"}}],"data":{"nullableEnums":["A",null,"B"]}}`,
			},
		))

		t.Run("nested invalid nullable enum value returned by list", runWithoutError(
			ExecutionEngineTestCase{
				schema: schema,
				operation: func(t *testing.T) graphql.Request {
					return graphql.Request{
						OperationName: "Enums",
						Query: `query Enums {
							nestedEnums {
								nullableEnums
							}
						}`,
					}
				},
				dataSources: []plan.DataSource{
					mustGraphqlDataSourceConfiguration(t,
						"id",
						mustFactory(t,
							testNetHttpClient(t, roundTripperTestCase{
								expectedHost:     "example.com",
								expectedPath:     "/",
								expectedBody:     "",
								sendResponseBody: `{"data":{"nestedEnums":{"nullableEnums":["A","INVALID","B"]}}}`,
								sendStatusCode:   200,
							}),
						),
						&plan.DataSourceMetadata{
							RootNodes: []plan.TypeField{
								{
									TypeName:   "Query",
									FieldNames: []string{"nestedEnums"},
								},
							},
							ChildNodes: []plan.TypeField{
								{
									TypeName:   "Object",
									FieldNames: []string{"nullableEnums"},
								},
							},
						},
						mustConfiguration(t, graphql_datasource.ConfigurationInput{
							Fetch: &graphql_datasource.FetchConfiguration{
								URL:    "https://example.com/",
								Method: "GET",
							},
							SchemaConfiguration: mustSchemaConfig(
								t,
								nil,
								enumSDL,
							),
						}),
					),
				},
				fields:           []plan.FieldConfiguration{},
				expectedResponse: `{"errors":[{"message":"Enum \"Enum\" cannot represent value: \"INVALID\"","path":["nestedEnums","nullableEnums",1],"extensions":{"code":"INTERNAL_SERVER_ERROR"}}],"data":{"nestedEnums":{"nullableEnums":["A",null,"B"]}}}`,
			},
		))

		t.Run("inaccessible non-nullable enum input", run(
			ExecutionEngineTestCase{
				schema: schema,
				operation: func(t *testing.T) graphql.Request {
					return graphql.Request{
						OperationName: "Enum",
						Query: `query Enum($enum: Enum!) {
							enum(enum: $enum)
						}`,
						Variables: []byte(`{"enum":"INACCESSIBLE"}`),
					}
				},
				dataSources: []plan.DataSource{
					mustGraphqlDataSourceConfiguration(t,
						"id",
						mustFactory(t,
							testNetHttpClient(t, roundTripperTestCase{
								expectedHost:     "example.com",
								expectedPath:     "/",
								expectedBody:     "",
								sendResponseBody: `{"data":{"enum":"A"}}`,
								sendStatusCode:   200,
							}),
						),
						&plan.DataSourceMetadata{
							RootNodes: []plan.TypeField{
								{
									TypeName:   "Query",
									FieldNames: []string{"enum"},
								},
							},
						},
						mustConfiguration(t, graphql_datasource.ConfigurationInput{
							Fetch: &graphql_datasource.FetchConfiguration{
								URL:    "https://example.com/",
								Method: "GET",
							},
							SchemaConfiguration: mustSchemaConfig(
								t,
								nil,
								enumSDL,
							),
						}),
					),
				},
				fields: []plan.FieldConfiguration{
					{
						TypeName:  "Query",
						FieldName: "enum",
						Path:      []string{"enum"},
						Arguments: []plan.ArgumentConfiguration{
							{
								Name:         "enum",
								SourceType:   plan.FieldArgumentSource,
								RenderConfig: plan.RenderArgumentAsGraphQLValue,
							},
						},
					},
				},
				expectedResponse: ``,
			}, true, `Variable "$enum" got invalid value "INACCESSIBLE"; Value "INACCESSIBLE" does not exist in "Enum" enum.`,
		))

		t.Run("nested inaccessible non-nullable enum input", run(
			ExecutionEngineTestCase{
				schema: schema,
				operation: func(t *testing.T) graphql.Request {
					return graphql.Request{
						OperationName: "Enum",
						Query: `query Enum($enum: Enum!) {
							nestedEnums {
								enum(enum: $enum)
							}
						}`,
						Variables: []byte(`{"enum":"INACCESSIBLE"}`),
					}
				},
				dataSources: []plan.DataSource{
					mustGraphqlDataSourceConfiguration(t,
						"id",
						mustFactory(t,
							testNetHttpClient(t, roundTripperTestCase{
								expectedHost:     "example.com",
								expectedPath:     "/",
								expectedBody:     "",
								sendResponseBody: `{"data":{"nestedEnums":{"enum":"A"}}}`,
								sendStatusCode:   200,
							}),
						),
						&plan.DataSourceMetadata{
							RootNodes: []plan.TypeField{
								{
									TypeName:   "Query",
									FieldNames: []string{"nestedEnums"},
								},
							},
							ChildNodes: []plan.TypeField{
								{
									TypeName:   "Object",
									FieldNames: []string{"enum"},
								},
							},
						},
						mustConfiguration(t, graphql_datasource.ConfigurationInput{
							Fetch: &graphql_datasource.FetchConfiguration{
								URL:    "https://example.com/",
								Method: "GET",
							},
							SchemaConfiguration: mustSchemaConfig(
								t,
								nil,
								enumSDL,
							),
						}),
					),
				},
				fields: []plan.FieldConfiguration{
					{
						TypeName:  "Object",
						FieldName: "enum",
						Path:      []string{"enum"},
						Arguments: []plan.ArgumentConfiguration{
							{
								Name:         "enum",
								SourceType:   plan.FieldArgumentSource,
								RenderConfig: plan.RenderArgumentAsGraphQLValue,
							},
						},
					},
				},
				expectedResponse: ``,
			}, true, `Variable "$enum" got invalid value "INACCESSIBLE"; Value "INACCESSIBLE" does not exist in "Enum" enum.`,
		))

		t.Run("inaccessible nullable enum input", run(
			ExecutionEngineTestCase{
				schema: schema,
				operation: func(t *testing.T) graphql.Request {
					return graphql.Request{
						OperationName: "Enum",
						Query: `query Enum($enum: Enum) {
							nullableEnum(enum: $enum)
						}`,
						Variables: []byte(`{"enum":"INACCESSIBLE"}`),
					}
				},
				dataSources: []plan.DataSource{
					mustGraphqlDataSourceConfiguration(t,
						"id",
						mustFactory(t,
							testNetHttpClient(t, roundTripperTestCase{
								expectedHost:     "example.com",
								expectedPath:     "/",
								expectedBody:     "",
								sendResponseBody: `{"data":{"enum":"INVALID"}}`,
								sendStatusCode:   200,
							}),
						),
						&plan.DataSourceMetadata{
							RootNodes: []plan.TypeField{
								{
									TypeName:   "Query",
									FieldNames: []string{"nullableEnum"},
								},
							},
						},
						mustConfiguration(t, graphql_datasource.ConfigurationInput{
							Fetch: &graphql_datasource.FetchConfiguration{
								URL:    "https://example.com/",
								Method: "GET",
							},
							SchemaConfiguration: mustSchemaConfig(
								t,
								nil,
								enumSDL,
							),
						}),
					),
				},
				fields: []plan.FieldConfiguration{
					{
						TypeName:  "Query",
						FieldName: "nullableEnum",
						Path:      []string{"nullableEnum"},
						Arguments: []plan.ArgumentConfiguration{
							{
								Name:         "enum",
								SourceType:   plan.FieldArgumentSource,
								RenderConfig: plan.RenderArgumentAsGraphQLValue,
							},
						},
					},
				},
				expectedResponse: ``,
			}, true, `Variable "$enum" got invalid value "INACCESSIBLE"; Value "INACCESSIBLE" does not exist in "Enum" enum.`,
		))

		t.Run("nested inaccessible nullable enum input", run(
			ExecutionEngineTestCase{
				schema: schema,
				operation: func(t *testing.T) graphql.Request {
					return graphql.Request{
						OperationName: "Enum",
						Query: `query Enum($enum: Enum) {
							nestedEnums {
								nullableEnum(enum: $enum)
							}
						}`,
						Variables: []byte(`{"enum":"INACCESSIBLE"}`),
					}
				},
				dataSources: []plan.DataSource{
					mustGraphqlDataSourceConfiguration(t,
						"id",
						mustFactory(t,
							testNetHttpClient(t, roundTripperTestCase{
								expectedHost:     "example.com",
								expectedPath:     "/",
								expectedBody:     "",
								sendResponseBody: `{"data":{"nestedEnums":{"nullableEnum":"A"}}}`,
								sendStatusCode:   200,
							}),
						),
						&plan.DataSourceMetadata{
							RootNodes: []plan.TypeField{
								{
									TypeName:   "Query",
									FieldNames: []string{"nestedEnums"},
								},
							},
							ChildNodes: []plan.TypeField{
								{
									TypeName:   "Object",
									FieldNames: []string{"nullableEnum"},
								},
							},
						},
						mustConfiguration(t, graphql_datasource.ConfigurationInput{
							Fetch: &graphql_datasource.FetchConfiguration{
								URL:    "https://example.com/",
								Method: "GET",
							},
							SchemaConfiguration: mustSchemaConfig(
								t,
								nil,
								enumSDL,
							),
						}),
					),
				},
				fields: []plan.FieldConfiguration{
					{
						TypeName:  "Object",
						FieldName: "nullableEnum",
						Path:      []string{"nullableEnum"},
						Arguments: []plan.ArgumentConfiguration{
							{
								Name:         "enum",
								SourceType:   plan.FieldArgumentSource,
								RenderConfig: plan.RenderArgumentAsGraphQLValue,
							},
						},
					},
				},
				expectedResponse: ``,
			}, true, `Variable "$enum" got invalid value "INACCESSIBLE"; Value "INACCESSIBLE" does not exist in "Enum" enum.`,
		))

		t.Run("inaccessible non-nullable enum value", runWithoutError(
			ExecutionEngineTestCase{
				schema: schema,
				operation: func(t *testing.T) graphql.Request {
					return graphql.Request{
						OperationName: "Enum",
						Query: `query Enum($enum: Enum!) {
							enum(enum: $enum)
						}`,
						Variables: []byte(`{"enum":"A"}`),
					}
				},
				dataSources: []plan.DataSource{
					mustGraphqlDataSourceConfiguration(t,
						"id",
						mustFactory(t,
							testNetHttpClient(t, roundTripperTestCase{
								expectedHost:     "example.com",
								expectedPath:     "/",
								expectedBody:     "",
								sendResponseBody: `{"data":{"enum":"INACCESSIBLE"}}`,
								sendStatusCode:   200,
							}),
						),
						&plan.DataSourceMetadata{
							RootNodes: []plan.TypeField{
								{
									TypeName:   "Query",
									FieldNames: []string{"enum"},
								},
							},
						},
						mustConfiguration(t, graphql_datasource.ConfigurationInput{
							Fetch: &graphql_datasource.FetchConfiguration{
								URL:    "https://example.com/",
								Method: "GET",
							},
							SchemaConfiguration: mustSchemaConfig(
								t,
								&graphql_datasource.FederationConfiguration{
									Enabled:    true,
									ServiceSDL: enumSDL,
								},
								enumSDL,
							),
						}),
					),
				},
				fields: []plan.FieldConfiguration{
					{
						TypeName:  "Query",
						FieldName: "enum",
						Path:      []string{"enum"},
						Arguments: []plan.ArgumentConfiguration{
							{
								Name:         "enum",
								SourceType:   plan.FieldArgumentSource,
								RenderConfig: plan.RenderArgumentAsGraphQLValue,
							},
						},
					},
				},
				expectedResponse: `{"errors":[{"message":"Invalid value found for field Query.enum.","path":["enum"],"extensions":{"code":"INVALID_GRAPHQL"}}],"data":null}`,
			},
		))

		t.Run("inaccessible non-nullable enum value apollo compatibility mode", runWithoutError(
			ExecutionEngineTestCase{
				schema: schema,
				operation: func(t *testing.T) graphql.Request {
					return graphql.Request{
						OperationName: "Enum",
						Query: `query Enum($enum: Enum!) {
							enum(enum: $enum)
						}`,
						Variables: []byte(`{"enum":"A"}`),
					}
				},
				dataSources: []plan.DataSource{
					mustGraphqlDataSourceConfiguration(t,
						"id",
						mustFactory(t,
							testNetHttpClient(t, roundTripperTestCase{
								expectedHost:     "example.com",
								expectedPath:     "/",
								expectedBody:     "",
								sendResponseBody: `{"data":{"enum":"INACCESSIBLE"}}`,
								sendStatusCode:   200,
							}),
						),
						&plan.DataSourceMetadata{
							RootNodes: []plan.TypeField{
								{
									TypeName:   "Query",
									FieldNames: []string{"enum"},
								},
							},
						},
						mustConfiguration(t, graphql_datasource.ConfigurationInput{
							Fetch: &graphql_datasource.FetchConfiguration{
								URL:    "https://example.com/",
								Method: "GET",
							},
							SchemaConfiguration: mustSchemaConfig(
								t,
								&graphql_datasource.FederationConfiguration{
									Enabled:    true,
									ServiceSDL: enumSDL,
								},
								enumSDL,
							),
						}),
					),
				},
				fields: []plan.FieldConfiguration{
					{
						TypeName:  "Query",
						FieldName: "enum",
						Path:      []string{"enum"},
						Arguments: []plan.ArgumentConfiguration{
							{
								Name:         "enum",
								SourceType:   plan.FieldArgumentSource,
								RenderConfig: plan.RenderArgumentAsGraphQLValue,
							},
						},
					},
				},
				expectedResponse: `{"data":null,"extensions":{"valueCompletion":[{"message":"Invalid value found for field Query.enum.","path":["enum"],"extensions":{"code":"INVALID_GRAPHQL"}}]}}`,
			},
			withValueCompletion(),
		))

		t.Run("nested inaccessible non-nullable enum value", runWithoutError(
			ExecutionEngineTestCase{
				schema: schema,
				operation: func(t *testing.T) graphql.Request {
					return graphql.Request{
						OperationName: "Enum",
						Query: `query Enum($enum: Enum!) {
							nestedEnums {
								enum(enum: $enum)
							}
						}`,
						Variables: []byte(`{"enum":"A"}`),
					}
				},
				dataSources: []plan.DataSource{
					mustGraphqlDataSourceConfiguration(t,
						"id",
						mustFactory(t,
							testNetHttpClient(t, roundTripperTestCase{
								expectedHost:     "example.com",
								expectedPath:     "/",
								expectedBody:     "",
								sendResponseBody: `{"data":{"nestedEnums":{"enum":"INACCESSIBLE"}}}`,
								sendStatusCode:   200,
							}),
						),
						&plan.DataSourceMetadata{
							RootNodes: []plan.TypeField{
								{
									TypeName:   "Query",
									FieldNames: []string{"nestedEnums"},
								},
							},
							ChildNodes: []plan.TypeField{
								{
									TypeName:   "Object",
									FieldNames: []string{"enum"},
								},
							},
						},
						mustConfiguration(t, graphql_datasource.ConfigurationInput{
							Fetch: &graphql_datasource.FetchConfiguration{
								URL:    "https://example.com/",
								Method: "GET",
							},
							SchemaConfiguration: mustSchemaConfig(
								t,
								&graphql_datasource.FederationConfiguration{
									Enabled:    true,
									ServiceSDL: enumSDL,
								},
								enumSDL,
							),
						}),
					),
				},
				fields: []plan.FieldConfiguration{
					{
						TypeName:  "Object",
						FieldName: "enum",
						Path:      []string{"enum"},
						Arguments: []plan.ArgumentConfiguration{
							{
								Name:         "enum",
								SourceType:   plan.FieldArgumentSource,
								RenderConfig: plan.RenderArgumentAsGraphQLValue,
							},
						},
					},
				},
				expectedResponse: `{"errors":[{"message":"Invalid value found for field Object.enum.","path":["nestedEnums","enum"],"extensions":{"code":"INVALID_GRAPHQL"}}],"data":null}`,
			},
		))

		t.Run("nested inaccessible non-nullable enum value apollo compatibility mode", runWithoutError(
			ExecutionEngineTestCase{
				schema: schema,
				operation: func(t *testing.T) graphql.Request {
					return graphql.Request{
						OperationName: "Enum",
						Query: `query Enum($enum: Enum!) {
							nestedEnums {
								enum(enum: $enum)
							}
						}`,
						Variables: []byte(`{"enum":"A"}`),
					}
				},
				dataSources: []plan.DataSource{
					mustGraphqlDataSourceConfiguration(t,
						"id",
						mustFactory(t,
							testNetHttpClient(t, roundTripperTestCase{
								expectedHost:     "example.com",
								expectedPath:     "/",
								expectedBody:     "",
								sendResponseBody: `{"data":{"nestedEnums":{"enum":"INACCESSIBLE"}}}`,
								sendStatusCode:   200,
							}),
						),
						&plan.DataSourceMetadata{
							RootNodes: []plan.TypeField{
								{
									TypeName:   "Query",
									FieldNames: []string{"nestedEnums"},
								},
							},
							ChildNodes: []plan.TypeField{
								{
									TypeName:   "Object",
									FieldNames: []string{"enum"},
								},
							},
						},
						mustConfiguration(t, graphql_datasource.ConfigurationInput{
							Fetch: &graphql_datasource.FetchConfiguration{
								URL:    "https://example.com/",
								Method: "GET",
							},
							SchemaConfiguration: mustSchemaConfig(
								t,
								&graphql_datasource.FederationConfiguration{
									Enabled:    true,
									ServiceSDL: enumSDL,
								},
								enumSDL,
							),
						}),
					),
				},
				fields: []plan.FieldConfiguration{
					{
						TypeName:  "Object",
						FieldName: "enum",
						Path:      []string{"enum"},
						Arguments: []plan.ArgumentConfiguration{
							{
								Name:         "enum",
								SourceType:   plan.FieldArgumentSource,
								RenderConfig: plan.RenderArgumentAsGraphQLValue,
							},
						},
					},
				},
				expectedResponse: `{"data":null,"extensions":{"valueCompletion":[{"message":"Invalid value found for field Object.enum.","path":["nestedEnums","enum"],"extensions":{"code":"INVALID_GRAPHQL"}}]}}`,
			},
			withValueCompletion(),
		))

		t.Run("inaccessible nullable enum value", runWithoutError(
			ExecutionEngineTestCase{
				schema: schema,
				operation: func(t *testing.T) graphql.Request {
					return graphql.Request{
						OperationName: "Enum",
						Query: `query Enum($enum: Enum!) {
							nullableEnum(enum: $enum)
						}`,
						Variables: []byte(`{"enum":"A"}`),
					}
				},
				dataSources: []plan.DataSource{
					mustGraphqlDataSourceConfiguration(t,
						"id",
						mustFactory(t,
							testNetHttpClient(t, roundTripperTestCase{
								expectedHost:     "example.com",
								expectedPath:     "/",
								expectedBody:     "",
								sendResponseBody: `{"data":{"nullableEnum":"INACCESSIBLE"}}`,
								sendStatusCode:   200,
							}),
						),
						&plan.DataSourceMetadata{
							RootNodes: []plan.TypeField{
								{
									TypeName:   "Query",
									FieldNames: []string{"nullableEnum"},
								},
							},
						},
						mustConfiguration(t, graphql_datasource.ConfigurationInput{
							Fetch: &graphql_datasource.FetchConfiguration{
								URL:    "https://example.com/",
								Method: "GET",
							},
							SchemaConfiguration: mustSchemaConfig(
								t,
								&graphql_datasource.FederationConfiguration{
									Enabled:    true,
									ServiceSDL: enumSDL,
								},
								enumSDL,
							),
						}),
					),
				},
				fields: []plan.FieldConfiguration{
					{
						TypeName:  "Query",
						FieldName: "nullableEnum",
						Path:      []string{"nullableEnum"},
						Arguments: []plan.ArgumentConfiguration{
							{
								Name:         "enum",
								SourceType:   plan.FieldArgumentSource,
								RenderConfig: plan.RenderArgumentAsGraphQLValue,
							},
						},
					},
				},
				expectedResponse: `{"errors":[{"message":"Invalid value found for field Query.nullableEnum.","path":["nullableEnum"],"extensions":{"code":"INVALID_GRAPHQL"}}],"data":{"nullableEnum":null}}`,
			},
		))

		t.Run("inaccessible nullable enum value apollo compatibility mode", runWithoutError(
			ExecutionEngineTestCase{
				schema: schema,
				operation: func(t *testing.T) graphql.Request {
					return graphql.Request{
						OperationName: "Enum",
						Query: `query Enum($enum: Enum!) {
							nullableEnum(enum: $enum)
						}`,
						Variables: []byte(`{"enum":"A"}`),
					}
				},
				dataSources: []plan.DataSource{
					mustGraphqlDataSourceConfiguration(t,
						"id",
						mustFactory(t,
							testNetHttpClient(t, roundTripperTestCase{
								expectedHost:     "example.com",
								expectedPath:     "/",
								expectedBody:     "",
								sendResponseBody: `{"data":{"nullableEnum":"INACCESSIBLE"}}`,
								sendStatusCode:   200,
							}),
						),
						&plan.DataSourceMetadata{
							RootNodes: []plan.TypeField{
								{
									TypeName:   "Query",
									FieldNames: []string{"nullableEnum"},
								},
							},
						},
						mustConfiguration(t, graphql_datasource.ConfigurationInput{
							Fetch: &graphql_datasource.FetchConfiguration{
								URL:    "https://example.com/",
								Method: "GET",
							},
							SchemaConfiguration: mustSchemaConfig(
								t,
								&graphql_datasource.FederationConfiguration{
									Enabled:    true,
									ServiceSDL: enumSDL,
								},
								enumSDL,
							),
						}),
					),
				},
				fields: []plan.FieldConfiguration{
					{
						TypeName:  "Query",
						FieldName: "nullableEnum",
						Path:      []string{"nullableEnum"},
						Arguments: []plan.ArgumentConfiguration{
							{
								Name:         "enum",
								SourceType:   plan.FieldArgumentSource,
								RenderConfig: plan.RenderArgumentAsGraphQLValue,
							},
						},
					},
				},
				expectedResponse: `{"data":{"nullableEnum":null},"extensions":{"valueCompletion":[{"message":"Invalid value found for field Query.nullableEnum.","path":["nullableEnum"],"extensions":{"code":"INVALID_GRAPHQL"}}]}}`,
			},
			withValueCompletion(),
		))

		t.Run("nested inaccessible nullable enum value", runWithoutError(
			ExecutionEngineTestCase{
				schema: schema,
				operation: func(t *testing.T) graphql.Request {
					return graphql.Request{
						OperationName: "Enum",
						Query: `query Enum {
							nestedEnums {
								nullableEnum
							}
						}`,
					}
				},
				dataSources: []plan.DataSource{
					mustGraphqlDataSourceConfiguration(t,
						"id",
						mustFactory(t,
							testNetHttpClient(t, roundTripperTestCase{
								expectedHost:     "example.com",
								expectedPath:     "/",
								expectedBody:     "",
								sendResponseBody: `{"data":{"nestedEnums":{"nullableEnum":"INACCESSIBLE"}}}`,
								sendStatusCode:   200,
							}),
						),
						&plan.DataSourceMetadata{
							RootNodes: []plan.TypeField{
								{
									TypeName:   "Query",
									FieldNames: []string{"nestedEnums"},
								},
							},
							ChildNodes: []plan.TypeField{
								{
									TypeName:   "Object",
									FieldNames: []string{"nullableEnum"},
								},
							},
						},
						mustConfiguration(t, graphql_datasource.ConfigurationInput{
							Fetch: &graphql_datasource.FetchConfiguration{
								URL:    "https://example.com/",
								Method: "GET",
							},
							SchemaConfiguration: mustSchemaConfig(
								t,
								&graphql_datasource.FederationConfiguration{
									Enabled:    true,
									ServiceSDL: enumSDL,
								},
								enumSDL,
							),
						}),
					),
				},
				fields:           []plan.FieldConfiguration{},
				expectedResponse: `{"errors":[{"message":"Invalid value found for field Object.nullableEnum.","path":["nestedEnums","nullableEnum"],"extensions":{"code":"INVALID_GRAPHQL"}}],"data":{"nestedEnums":{"nullableEnum":null}}}`,
			},
		))

		t.Run("nested inaccessible nullable enum value apollo compatibility mode", runWithoutError(
			ExecutionEngineTestCase{
				schema: schema,
				operation: func(t *testing.T) graphql.Request {
					return graphql.Request{
						OperationName: "Enum",
						Query: `query Enum {
							nestedEnums {
								nullableEnum
							}
						}`,
					}
				},
				dataSources: []plan.DataSource{
					mustGraphqlDataSourceConfiguration(t,
						"id",
						mustFactory(t,
							testNetHttpClient(t, roundTripperTestCase{
								expectedHost:     "example.com",
								expectedPath:     "/",
								expectedBody:     "",
								sendResponseBody: `{"data":{"nestedEnums":{"nullableEnum":"INACCESSIBLE"}}}`,
								sendStatusCode:   200,
							}),
						),
						&plan.DataSourceMetadata{
							RootNodes: []plan.TypeField{
								{
									TypeName:   "Query",
									FieldNames: []string{"nestedEnums"},
								},
							},
							ChildNodes: []plan.TypeField{
								{
									TypeName:   "Object",
									FieldNames: []string{"nullableEnum"},
								},
							},
						},
						mustConfiguration(t, graphql_datasource.ConfigurationInput{
							Fetch: &graphql_datasource.FetchConfiguration{
								URL:    "https://example.com/",
								Method: "GET",
							},
							SchemaConfiguration: mustSchemaConfig(
								t,
								&graphql_datasource.FederationConfiguration{
									Enabled:    true,
									ServiceSDL: enumSDL,
								},
								enumSDL,
							),
						}),
					),
				},
				fields:           []plan.FieldConfiguration{},
				expectedResponse: `{"data":{"nestedEnums":{"nullableEnum":null}},"extensions":{"valueCompletion":[{"message":"Invalid value found for field Object.nullableEnum.","path":["nestedEnums","nullableEnum"],"extensions":{"code":"INVALID_GRAPHQL"}}]}}`,
			},
			withValueCompletion(),
		))

		t.Run("inaccessible non-nullable enum value returned by list", runWithoutError(
			ExecutionEngineTestCase{
				schema: schema,
				operation: func(t *testing.T) graphql.Request {
					return graphql.Request{
						OperationName: "Enums",
						Query: `query Enums {
							enums
						}`,
					}
				},
				dataSources: []plan.DataSource{
					mustGraphqlDataSourceConfiguration(t,
						"id",
						mustFactory(t,
							testNetHttpClient(t, roundTripperTestCase{
								expectedHost:     "example.com",
								expectedPath:     "/",
								expectedBody:     "",
								sendResponseBody: `{"data":{"enums":["INACCESSIBLE","A","B"]}}`,
								sendStatusCode:   200,
							}),
						),
						&plan.DataSourceMetadata{
							RootNodes: []plan.TypeField{
								{
									TypeName:   "Query",
									FieldNames: []string{"enums"},
								},
							},
						},
						mustConfiguration(t, graphql_datasource.ConfigurationInput{
							Fetch: &graphql_datasource.FetchConfiguration{
								URL:    "https://example.com/",
								Method: "GET",
							},
							SchemaConfiguration: mustSchemaConfig(
								t,
								&graphql_datasource.FederationConfiguration{
									Enabled:    true,
									ServiceSDL: enumSDL,
								},
								enumSDL,
							),
						}),
					),
				},
				fields:           []plan.FieldConfiguration{},
				expectedResponse: `{"errors":[{"message":"Invalid value found for array element of type Enum at index 0.","path":["enums",0],"extensions":{"code":"INVALID_GRAPHQL"}}],"data":null}`,
			},
		))

		t.Run("inaccessible non-nullable enum value returned by list apollo compatibility mode", runWithoutError(
			ExecutionEngineTestCase{
				schema: schema,
				operation: func(t *testing.T) graphql.Request {
					return graphql.Request{
						OperationName: "Enums",
						Query: `query Enums {
							enums
						}`,
					}
				},
				dataSources: []plan.DataSource{
					mustGraphqlDataSourceConfiguration(t,
						"id",
						mustFactory(t,
							testNetHttpClient(t, roundTripperTestCase{
								expectedHost:     "example.com",
								expectedPath:     "/",
								expectedBody:     "",
								sendResponseBody: `{"data":{"enums":["INACCESSIBLE","A","B"]}}`,
								sendStatusCode:   200,
							}),
						),
						&plan.DataSourceMetadata{
							RootNodes: []plan.TypeField{
								{
									TypeName:   "Query",
									FieldNames: []string{"enums"},
								},
							},
						},
						mustConfiguration(t, graphql_datasource.ConfigurationInput{
							Fetch: &graphql_datasource.FetchConfiguration{
								URL:    "https://example.com/",
								Method: "GET",
							},
							SchemaConfiguration: mustSchemaConfig(
								t,
								&graphql_datasource.FederationConfiguration{
									Enabled:    true,
									ServiceSDL: enumSDL,
								},
								enumSDL,
							),
						}),
					),
				},
				fields:           []plan.FieldConfiguration{},
				expectedResponse: `{"data":null,"extensions":{"valueCompletion":[{"message":"Invalid value found for array element of type Enum at index 0.","path":["enums",0],"extensions":{"code":"INVALID_GRAPHQL"}}]}}`,
			},
			withValueCompletion(),
		))

		t.Run("nested inaccessible non-nullable enum value returned by list", runWithoutError(
			ExecutionEngineTestCase{
				schema: schema,
				operation: func(t *testing.T) graphql.Request {
					return graphql.Request{
						OperationName: "Enums",
						Query: `query Enums {
							nestedEnums {
								enums
							}
						}`,
					}
				},
				dataSources: []plan.DataSource{
					mustGraphqlDataSourceConfiguration(t,
						"id",
						mustFactory(t,
							testNetHttpClient(t, roundTripperTestCase{
								expectedHost:     "example.com",
								expectedPath:     "/",
								expectedBody:     "",
								sendResponseBody: `{"data":{"nestedEnums":{"enums":["A","INACCESSIBLE","B"]}}}`,
								sendStatusCode:   200,
							}),
						),
						&plan.DataSourceMetadata{
							RootNodes: []plan.TypeField{
								{
									TypeName:   "Query",
									FieldNames: []string{"nestedEnums"},
								},
							},
							ChildNodes: []plan.TypeField{
								{
									TypeName:   "Object",
									FieldNames: []string{"enums"},
								},
							},
						},
						mustConfiguration(t, graphql_datasource.ConfigurationInput{
							Fetch: &graphql_datasource.FetchConfiguration{
								URL:    "https://example.com/",
								Method: "GET",
							},
							SchemaConfiguration: mustSchemaConfig(
								t,
								&graphql_datasource.FederationConfiguration{
									Enabled:    true,
									ServiceSDL: enumSDL,
								},
								enumSDL,
							),
						}),
					),
				},
				fields:           []plan.FieldConfiguration{},
				expectedResponse: `{"errors":[{"message":"Invalid value found for array element of type Enum at index 1.","path":["nestedEnums","enums",1],"extensions":{"code":"INVALID_GRAPHQL"}}],"data":null}`,
			},
		))

		t.Run("nested inaccessible non-nullable enum value returned by list apollo compatibility mode", runWithoutError(
			ExecutionEngineTestCase{
				schema: schema,
				operation: func(t *testing.T) graphql.Request {
					return graphql.Request{
						OperationName: "Enums",
						Query: `query Enums {
							nestedEnums {
								enums
							}
						}`,
					}
				},
				dataSources: []plan.DataSource{
					mustGraphqlDataSourceConfiguration(t,
						"id",
						mustFactory(t,
							testNetHttpClient(t, roundTripperTestCase{
								expectedHost:     "example.com",
								expectedPath:     "/",
								expectedBody:     "",
								sendResponseBody: `{"data":{"nestedEnums":{"enums":["A","B","INACCESSIBLE"]}}}`,
								sendStatusCode:   200,
							}),
						),
						&plan.DataSourceMetadata{
							RootNodes: []plan.TypeField{
								{
									TypeName:   "Query",
									FieldNames: []string{"nestedEnums"},
								},
							},
							ChildNodes: []plan.TypeField{
								{
									TypeName:   "Object",
									FieldNames: []string{"enums"},
								},
							},
						},
						mustConfiguration(t, graphql_datasource.ConfigurationInput{
							Fetch: &graphql_datasource.FetchConfiguration{
								URL:    "https://example.com/",
								Method: "GET",
							},
							SchemaConfiguration: mustSchemaConfig(
								t,
								&graphql_datasource.FederationConfiguration{
									Enabled:    true,
									ServiceSDL: enumSDL,
								},
								enumSDL,
							),
						}),
					),
				},
				fields:           []plan.FieldConfiguration{},
				expectedResponse: `{"data":null,"extensions":{"valueCompletion":[{"message":"Invalid value found for array element of type Enum at index 2.","path":["nestedEnums","enums",2],"extensions":{"code":"INVALID_GRAPHQL"}}]}}`,
			},
			withValueCompletion(),
		))

		t.Run("inaccessible nullable enum value returned by list", runWithoutError(
			ExecutionEngineTestCase{
				schema: schema,
				operation: func(t *testing.T) graphql.Request {
					return graphql.Request{
						OperationName: "Enums",
						Query: `query Enums {
							nullableEnums
						}`,
					}
				},
				dataSources: []plan.DataSource{
					mustGraphqlDataSourceConfiguration(t,
						"id",
						mustFactory(t,
							testNetHttpClient(t, roundTripperTestCase{
								expectedHost:     "example.com",
								expectedPath:     "/",
								expectedBody:     "",
								sendResponseBody: `{"data":{"nullableEnums":["INACCESSIBLE","A","B"]}}`,
								sendStatusCode:   200,
							}),
						),
						&plan.DataSourceMetadata{
							RootNodes: []plan.TypeField{
								{
									TypeName:   "Query",
									FieldNames: []string{"nullableEnums"},
								},
							},
						},
						mustConfiguration(t, graphql_datasource.ConfigurationInput{
							Fetch: &graphql_datasource.FetchConfiguration{
								URL:    "https://example.com/",
								Method: "GET",
							},
							SchemaConfiguration: mustSchemaConfig(
								t,
								&graphql_datasource.FederationConfiguration{
									Enabled:    true,
									ServiceSDL: enumSDL,
								},
								enumSDL,
							),
						}),
					),
				},
				fields:           []plan.FieldConfiguration{},
				expectedResponse: `{"errors":[{"message":"Invalid value found for array element of type Enum at index 0.","path":["nullableEnums",0],"extensions":{"code":"INVALID_GRAPHQL"}}],"data":{"nullableEnums":[null,"A","B"]}}`,
			},
		))

		t.Run("inaccessible nullable enum value returned by list apollo compatibility mode", runWithoutError(
			ExecutionEngineTestCase{
				schema: schema,
				operation: func(t *testing.T) graphql.Request {
					return graphql.Request{
						OperationName: "Enums",
						Query: `query Enums {
							nullableEnums
						}`,
					}
				},
				dataSources: []plan.DataSource{
					mustGraphqlDataSourceConfiguration(t,
						"id",
						mustFactory(t,
							testNetHttpClient(t, roundTripperTestCase{
								expectedHost:     "example.com",
								expectedPath:     "/",
								expectedBody:     "",
								sendResponseBody: `{"data":{"nullableEnums":["INACCESSIBLE","A","B"]}}`,
								sendStatusCode:   200,
							}),
						),
						&plan.DataSourceMetadata{
							RootNodes: []plan.TypeField{
								{
									TypeName:   "Query",
									FieldNames: []string{"nullableEnums"},
								},
							},
						},
						mustConfiguration(t, graphql_datasource.ConfigurationInput{
							Fetch: &graphql_datasource.FetchConfiguration{
								URL:    "https://example.com/",
								Method: "GET",
							},
							SchemaConfiguration: mustSchemaConfig(
								t,
								&graphql_datasource.FederationConfiguration{
									Enabled:    true,
									ServiceSDL: enumSDL,
								},
								enumSDL,
							),
						}),
					),
				},
				fields:           []plan.FieldConfiguration{},
				expectedResponse: `{"data":{"nullableEnums":[null,"A","B"]},"extensions":{"valueCompletion":[{"message":"Invalid value found for array element of type Enum at index 0.","path":["nullableEnums",0],"extensions":{"code":"INVALID_GRAPHQL"}}]}}`,
			},
			withValueCompletion(),
		))

		t.Run("nested inaccessible nullable enum value returned by list", runWithoutError(
			ExecutionEngineTestCase{
				schema: schema,
				operation: func(t *testing.T) graphql.Request {
					return graphql.Request{
						OperationName: "Enums",
						Query: `query Enums {
							nestedEnums {
								nullableEnums
							}
						}`,
					}
				},
				dataSources: []plan.DataSource{
					mustGraphqlDataSourceConfiguration(t,
						"id",
						mustFactory(t,
							testNetHttpClient(t, roundTripperTestCase{
								expectedHost:     "example.com",
								expectedPath:     "/",
								expectedBody:     "",
								sendResponseBody: `{"data":{"nestedEnums":{"nullableEnums":["A","INACCESSIBLE","B"]}}}`,
								sendStatusCode:   200,
							}),
						),
						&plan.DataSourceMetadata{
							RootNodes: []plan.TypeField{
								{
									TypeName:   "Query",
									FieldNames: []string{"nestedEnums"},
								},
							},
							ChildNodes: []plan.TypeField{
								{
									TypeName:   "Object",
									FieldNames: []string{"nullableEnums"},
								},
							},
						},
						mustConfiguration(t, graphql_datasource.ConfigurationInput{
							Fetch: &graphql_datasource.FetchConfiguration{
								URL:    "https://example.com/",
								Method: "GET",
							},
							SchemaConfiguration: mustSchemaConfig(
								t,
								&graphql_datasource.FederationConfiguration{
									Enabled:    true,
									ServiceSDL: enumSDL,
								},
								enumSDL,
							),
						}),
					),
				},
				fields:           []plan.FieldConfiguration{},
				expectedResponse: `{"errors":[{"message":"Invalid value found for array element of type Enum at index 1.","path":["nestedEnums","nullableEnums",1],"extensions":{"code":"INVALID_GRAPHQL"}}],"data":{"nestedEnums":{"nullableEnums":["A",null,"B"]}}}`,
			},
		))

		t.Run("nested inaccessible nullable enum value returned by list apollo compatibility mode", runWithoutError(
			ExecutionEngineTestCase{
				schema: schema,
				operation: func(t *testing.T) graphql.Request {
					return graphql.Request{
						OperationName: "Enums",
						Query: `query Enums {
							nestedEnums {
								nullableEnums
							}
						}`,
					}
				},
				dataSources: []plan.DataSource{
					mustGraphqlDataSourceConfiguration(t,
						"id",
						mustFactory(t,
							testNetHttpClient(t, roundTripperTestCase{
								expectedHost:     "example.com",
								expectedPath:     "/",
								expectedBody:     "",
								sendResponseBody: `{"data":{"nestedEnums":{"nullableEnums":["A","B","INACCESSIBLE"]}}}`,
								sendStatusCode:   200,
							}),
						),
						&plan.DataSourceMetadata{
							RootNodes: []plan.TypeField{
								{
									TypeName:   "Query",
									FieldNames: []string{"nestedEnums"},
								},
							},
							ChildNodes: []plan.TypeField{
								{
									TypeName:   "Object",
									FieldNames: []string{"nullableEnums"},
								},
							},
						},
						mustConfiguration(t, graphql_datasource.ConfigurationInput{
							Fetch: &graphql_datasource.FetchConfiguration{
								URL:    "https://example.com/",
								Method: "GET",
							},
							SchemaConfiguration: mustSchemaConfig(
								t,
								&graphql_datasource.FederationConfiguration{
									Enabled:    true,
									ServiceSDL: enumSDL,
								},
								enumSDL,
							),
						}),
					),
				},
				fields:           []plan.FieldConfiguration{},
				expectedResponse: `{"data":{"nestedEnums":{"nullableEnums":["A","B",null]}},"extensions":{"valueCompletion":[{"message":"Invalid value found for array element of type Enum at index 2.","path":["nestedEnums","nullableEnums",2],"extensions":{"code":"INVALID_GRAPHQL"}}]}}`,
			},
			withValueCompletion(),
		))
	})

	t.Run("variables", func(t *testing.T) {
		t.Run("operation with optional input fields", func(t *testing.T) {
			schemaString := `
				type Query {
					field(arg: Input): String
				}

				input Input {
					optional: String
					required: String!
				}`
			schema, err := graphql.NewSchemaFromString(schemaString)
			require.NoError(t, err)

			t.Run("optional value provided", runWithoutError(
				ExecutionEngineTestCase{
					schema: schema,
					operation: func(t *testing.T) graphql.Request {
						return graphql.Request{
							OperationName: "queryVariables",
							Variables:     []byte(`{"optional":"optionalValue","required":"requiredValue"}`),
							Query: `query queryVariables($optional: String, $required: String!) {
										field(arg: {optional: $optional, required: $required})
									}`,
						}
					},
					dataSources: []plan.DataSource{
						mustGraphqlDataSourceConfiguration(t,
							"id",
							mustFactory(t,
								testNetHttpClient(t, roundTripperTestCase{
									expectedHost:     "example.com",
									expectedPath:     "/",
									expectedBody:     `{"query":"query($a: Input){field(arg: $a)}","variables":{"a":{"optional":"optionalValue","required":"requiredValue"}}}`,
									sendResponseBody: `{"data":{"field":"response"}}`,
									sendStatusCode:   200,
								}),
							),
							&plan.DataSourceMetadata{
								RootNodes: []plan.TypeField{
									{TypeName: "Query", FieldNames: []string{"field"}},
								},
							},
							mustConfiguration(t, graphql_datasource.ConfigurationInput{
								Fetch: &graphql_datasource.FetchConfiguration{
									URL:    "https://example.com/",
									Method: "GET",
								},
								SchemaConfiguration: mustSchemaConfig(
									t,
									nil,
									schemaString,
								),
							}),
						),
					},
					fields: []plan.FieldConfiguration{
						{
							TypeName:  "Query",
							FieldName: "field",
							Path:      []string{"field"},
							Arguments: []plan.ArgumentConfiguration{
								{
									Name:       "arg",
									SourceType: plan.FieldArgumentSource,
								},
							},
						},
					},
					expectedResponse: `{"data":{"field":"response"}}`,
				},
			))

			t.Run("optional value ommited", runWithoutError(
				ExecutionEngineTestCase{
					schema: schema,
					operation: func(t *testing.T) graphql.Request {
						return graphql.Request{
							OperationName: "queryVariables",
							Variables:     []byte(`{"required":"requiredValue"}`),
							Query: `query queryVariables($optional: String, $required: String!) {
										field(arg: {optional: $optional, required: $required})
									}`,
						}
					},
					dataSources: []plan.DataSource{
						mustGraphqlDataSourceConfiguration(t,
							"id",
							mustFactory(t,
								testNetHttpClient(t, roundTripperTestCase{
									expectedHost:     "example.com",
									expectedPath:     "/",
									expectedBody:     `{"query":"query($a: Input){field(arg: $a)}","variables":{"a":{"required":"requiredValue"}}}`,
									sendResponseBody: `{"data":{"field":"response"}}`,
									sendStatusCode:   200,
								}),
							),
							&plan.DataSourceMetadata{
								RootNodes: []plan.TypeField{
									{TypeName: "Query", FieldNames: []string{"field"}},
								},
							},
							mustConfiguration(t, graphql_datasource.ConfigurationInput{
								Fetch: &graphql_datasource.FetchConfiguration{
									URL:    "https://example.com/",
									Method: "GET",
								},
								SchemaConfiguration: mustSchemaConfig(
									t,
									nil,
									schemaString,
								),
							}),
						),
					},
					fields: []plan.FieldConfiguration{
						{
							TypeName:  "Query",
							FieldName: "field",
							Path:      []string{"field"},
							Arguments: []plan.ArgumentConfiguration{
								{
									Name:       "arg",
									SourceType: plan.FieldArgumentSource,
								},
							},
						},
					},
					expectedResponse: `{"data":{"field":"response"}}`,
				},
			))
		})
	})

	t.Run("execute operation with nested fetch on one of the types", func(t *testing.T) {

		definition := `
			type User implements Node {
				id: ID!
				title: String!
				some: User!
			}

			type Admin implements Node {
				id: ID!
				title: String!
				some: User!
			}

			interface Node {
				id: ID!
				title: String!
				some: User!
			}

			type Query {
				accounts: [Node!]!
			}`

		firstSubgraphSDL := `	
				type User implements Node @key(fields: "id") {
					id: ID!
					title: String! @external
					some: User!
				}

				type Admin implements Node @key(fields: "id") {
					id: ID!
					title: String! @external
					some: User!
				}

				interface Node {
					id: ID!
					title: String!
					some: User!
				}

				type Query {
					accounts: [Node!]!
				}
			`
		secondSubgraphSDL := `
				type User @key(fields: "id") {
					id: ID!
					name: String!
					title: String!
				}

				type Admin @key(fields: "id") {
					id: ID!
					adminName: String!
					title: String!
				}
			`

		type makeDataSourceOpts struct {
			expectFetchReasons bool
			includeCostConfig  bool
		}

		makeDataSource := func(t *testing.T, opts makeDataSourceOpts) []plan.DataSource {
			var expectedBody1 string
			var expectedBody2 string
			if !opts.expectFetchReasons {
				expectedBody1 = `{"query":"{accounts {__typename ... on User {some {__typename id}} ... on Admin {some {__typename id}}}}"}`
			} else {
				expectedBody1 = `{"query":"{accounts {__typename ... on User {some {__typename id}} ... on Admin {some {__typename id}}}}","extensions":{"fetch_reasons":[{"typename":"Admin","field":"some","by_user":true},{"typename":"User","field":"id","by_subgraphs":["id-2"],"by_user":true,"is_key":true}]}}`
			}
			expectedBody2 = `{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {__typename title}}}","variables":{"representations":[{"__typename":"User","id":"1"},{"__typename":"User","id":"3"}]}}`

			// Cost config for DS1 (first subgraph): accounts service
			var ds1CostConfig *plan.DataSourceCostConfig
			if opts.includeCostConfig {
				ds1CostConfig = &plan.DataSourceCostConfig{
					Weights: map[plan.FieldCoordinate]*plan.FieldWeight{
						{TypeName: "Query", FieldName: "accounts"}: {HasWeight: true, Weight: 5},
						{TypeName: "User", FieldName: "some"}:      {HasWeight: true, Weight: 2},
						{TypeName: "Admin", FieldName: "some"}:     {HasWeight: true, Weight: 3},
					},
					ListSizes: map[plan.FieldCoordinate]*plan.FieldListSize{
						{TypeName: "Query", FieldName: "accounts"}: {AssumedSize: 3},
					},
				}
			}

			// Cost config for DS2 (second subgraph): extends User/Admin with title
			var ds2CostConfig *plan.DataSourceCostConfig
			if opts.includeCostConfig {
				ds2CostConfig = &plan.DataSourceCostConfig{
					Weights: map[plan.FieldCoordinate]*plan.FieldWeight{
						{TypeName: "User", FieldName: "name"}:       {HasWeight: true, Weight: 2},
						{TypeName: "User", FieldName: "title"}:      {HasWeight: true, Weight: 4},
						{TypeName: "Admin", FieldName: "adminName"}: {HasWeight: true, Weight: 3},
						{TypeName: "Admin", FieldName: "title"}:     {HasWeight: true, Weight: 5},
					},
				}
			}

			return []plan.DataSource{
				mustGraphqlDataSourceConfiguration(t,
					"id-1",
					mustFactory(t,
						testNetHttpClient(t, roundTripperTestCase{
							expectedHost:     "first",
							expectedPath:     "/",
							expectedBody:     expectedBody1,
							sendResponseBody: `{"data":{"accounts":[{"__typename":"User","some":{"__typename":"User","id":"1"}},{"__typename":"Admin","some":{"__typename":"User","id":"2"}},{"__typename":"User","some":{"__typename":"User","id":"3"}}]}}`,
							sendStatusCode:   200,
						}),
					),
					&plan.DataSourceMetadata{
						RootNodes: []plan.TypeField{
							{
								TypeName:   "Query",
								FieldNames: []string{"accounts"},
							},
							{
								TypeName:          "User",
								FieldNames:        []string{"id", "some"},
								FetchReasonFields: []string{"id"},
							},
							{
								TypeName:          "Admin",
								FieldNames:        []string{"id", "some"},
								FetchReasonFields: []string{"some"},
							},
						},
						ChildNodes: []plan.TypeField{
							{
								TypeName:   "Node",
								FieldNames: []string{"id", "title", "some"},
							},
						},
						CostConfig: ds1CostConfig,
						FederationMetaData: plan.FederationMetaData{
							Keys: plan.FederationFieldConfigurations{
								{
									TypeName:     "User",
									SelectionSet: "id",
								},
								{
									TypeName:     "Admin",
									SelectionSet: "id",
								},
							},
						},
					},
					mustConfiguration(t, graphql_datasource.ConfigurationInput{
						Fetch: &graphql_datasource.FetchConfiguration{
							URL:    "https://first/",
							Method: "POST",
						},
						SchemaConfiguration: mustSchemaConfig(
							t,
							&graphql_datasource.FederationConfiguration{
								Enabled:    true,
								ServiceSDL: firstSubgraphSDL,
							},
							firstSubgraphSDL,
						),
					}),
				),
				mustGraphqlDataSourceConfiguration(t,
					"id-2",
					mustFactory(t,
						testNetHttpClient(t, roundTripperTestCase{
							expectedHost:     "second",
							expectedPath:     "/",
							expectedBody:     expectedBody2,
							sendResponseBody: `{"data":{"_entities":[{"__typename":"User","title":"User1"},{"__typename":"User","title":"User3"}]}}`,
							sendStatusCode:   200,
						}),
					),
					&plan.DataSourceMetadata{
						RootNodes: []plan.TypeField{
							{
								TypeName:   "User",
								FieldNames: []string{"id", "name", "title"},
							},
							{
								TypeName:   "Admin",
								FieldNames: []string{"id", "adminName", "title"},
							},
						},
						CostConfig: ds2CostConfig,
						FederationMetaData: plan.FederationMetaData{
							Keys: plan.FederationFieldConfigurations{
								{
									TypeName:     "User",
									SelectionSet: "id",
								},
								{
									TypeName:     "Admin",
									SelectionSet: "id",
								},
							},
						},
					},
					mustConfiguration(t, graphql_datasource.ConfigurationInput{
						Fetch: &graphql_datasource.FetchConfiguration{
							URL:    "https://second/",
							Method: "POST",
						},
						SchemaConfiguration: mustSchemaConfig(
							t,
							&graphql_datasource.FederationConfiguration{
								Enabled:    true,
								ServiceSDL: secondSubgraphSDL,
							},
							secondSubgraphSDL,
						),
					}),
				),
			}
		}

		t.Run("run", runWithoutError(ExecutionEngineTestCase{
			schema: func(t *testing.T) *graphql.Schema {
				t.Helper()
				parseSchema, err := graphql.NewSchemaFromString(definition)
				require.NoError(t, err)
				return parseSchema
			}(t),
			operation: func(t *testing.T) graphql.Request {
				return graphql.Request{
					OperationName: "Accounts",
					Query: `
						query Accounts {
							accounts {
								... on User {
									some {
										title
									}
								}
								... on Admin {
									some {
										__typename
										id
									}
								}
							}
						}`,
				}
			},
			dataSources:      makeDataSource(t, makeDataSourceOpts{expectFetchReasons: false}),
			expectedResponse: `{"data":{"accounts":[{"some":{"title":"User1"}},{"some":{"__typename":"User","id":"2"}},{"some":{"title":"User3"}}]}}`,
		}))

		t.Run("run with extension", runWithoutError(
			ExecutionEngineTestCase{
				schema: func(t *testing.T) *graphql.Schema {
					t.Helper()
					parseSchema, err := graphql.NewSchemaFromString(definition)
					require.NoError(t, err)
					return parseSchema
				}(t),
				operation: func(t *testing.T) graphql.Request {
					return graphql.Request{
						OperationName: "Accounts",
						Query: `
							query Accounts {
								accounts {
									... on User {
										some {
											title
										}
									}
									... on Admin {
										some {
											__typename
											id
										}
									}
								}
							}`,
					}
				},
				dataSources:      makeDataSource(t, makeDataSourceOpts{expectFetchReasons: true}),
				expectedResponse: `{"data":{"accounts":[{"some":{"title":"User1"}},{"some":{"__typename":"User","id":"2"}},{"some":{"title":"User3"}}]}}`,
			},
			withFetchReasons(),
		))

		t.Run("run with static cost computation", runWithoutError(
			ExecutionEngineTestCase{
				schema: func(t *testing.T) *graphql.Schema {
					t.Helper()
					parseSchema, err := graphql.NewSchemaFromString(definition)
					require.NoError(t, err)
					return parseSchema
				}(t),
				operation: func(t *testing.T) graphql.Request {
					return graphql.Request{
						OperationName: "Accounts",
						Query: `
							query Accounts {
								accounts {
									... on User {
										some {
											title
										}
									}
									... on Admin {
										some {
											__typename
											id
										}
									}
								}
							}`,
					}
				},
				dataSources:      makeDataSource(t, makeDataSourceOpts{includeCostConfig: true}),
				expectedResponse: `{"data":{"accounts":[{"some":{"title":"User1"}},{"some":{"__typename":"User","id":"2"}},{"some":{"title":"User3"}}]}}`,
				// Cost breakdown with federation:
				// Query.accounts: fieldCost=5, multiplier=3 (listSize)
				//   accounts returns interface [Node!]! with implementing types [User, Admin]
				//
				// Children (per interface member type):
				//   User.some: User: fieldCost=3 (DS1:2 + DS2:1 summed)
				//     User.title: 4 (DS2, resolved via _entities federation)
				//   cost = 3 + 4 = 7
				//
				//   Admin.some: User: fieldCost=3 (DS1 only)
				//   cost = 3
				//
				// Children total = 7 + 3 = 10
				// (is it possible to improve accuracy here by using the largest fragment instead of the sum?)
				// Total = (5 + 10) * 3 = 45
				expectedStaticCost: 45,
			},
			computeStaticCost(),
		))
	})

	t.Run("validation of optional @requires dependencies", func(t *testing.T) {

		t.Run("execute operation with @requires and @external", func(t *testing.T) {
			definition := `
				type User {
					id: ID!
					title: String
					full: String
				}
				type Query {
					accounts: [User!]!
				}`
			firstSubgraphSDL := `
				type User @key(fields: "id") {
					id: ID!
					title: String @external
					full: String @requires(fields: "title")
				}
				type Query {
					accounts: [User!]!
				}`
			secondSubgraphSDL := `
				type User @key(fields: "id") {
					id: ID!
					title: String
				}`

			datasources := []plan.DataSource{
				mustGraphqlDataSourceConfiguration(t,
					"id-1",
					mustFactory(t,
						testConditionalNetHttpClient(t, conditionalTestCase{
							expectedHost: "first",
							expectedPath: "/",
							responses: map[string]sendResponse{
								`{"query":"{accounts {id __typename}}"}`: {
									statusCode: 200,
									body:       `{"data":{"accounts":[{"__typename":"User","id":"1"},{"__typename":"User","id":"3"}]}}`,
								},
								`{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {__typename full}}}","variables":{"representations":[{"__typename":"User","title":"User1","id":"1"},{"__typename":"User","title":"User3","id":"3"}]}}`: {
									statusCode: 200,
									body:       `{"data":{"_entities":[{"__typename":"User","full":"User1 full"},{"__typename":"User","full":"User3 full"}]}}`,
								},
							},
						}),
					),
					&plan.DataSourceMetadata{
						RootNodes: []plan.TypeField{
							{
								TypeName:   "Query",
								FieldNames: []string{"accounts"},
							},
							{
								TypeName:           "User",
								FieldNames:         []string{"id", "full"},
								ExternalFieldNames: []string{"title"},
							},
						},
						FederationMetaData: plan.FederationMetaData{
							Keys: plan.FederationFieldConfigurations{
								{
									TypeName:     "User",
									SelectionSet: "id",
								},
							},
							Requires: plan.FederationFieldConfigurations{
								{
									TypeName:     "User",
									FieldName:    "full",
									SelectionSet: "title",
								},
							},
						},
					},
					mustConfiguration(t, graphql_datasource.ConfigurationInput{
						Fetch: &graphql_datasource.FetchConfiguration{
							URL:    "https://first/",
							Method: "POST",
						},
						SchemaConfiguration: mustSchemaConfig(
							t,
							&graphql_datasource.FederationConfiguration{
								Enabled:    true,
								ServiceSDL: firstSubgraphSDL,
							},
							firstSubgraphSDL,
						),
					}),
				),
				mustGraphqlDataSourceConfiguration(t,
					"id-2",
					mustFactory(t,
						testNetHttpClient(t, roundTripperTestCase{
							expectedHost:     "second",
							expectedPath:     "/",
							expectedBody:     `{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {__typename title}}}","variables":{"representations":[{"__typename":"User","id":"1"},{"__typename":"User","id":"3"}]}}`,
							sendResponseBody: `{"data":{"_entities":[{"__typename":"User","title":"User1"},{"__typename":"User","title":"User3"}]}}`,
							sendStatusCode:   200,
						}),
					),
					&plan.DataSourceMetadata{
						RootNodes: []plan.TypeField{
							{
								TypeName:   "User",
								FieldNames: []string{"id", "title"},
							},
						},
						FederationMetaData: plan.FederationMetaData{
							Keys: plan.FederationFieldConfigurations{
								{
									TypeName:     "User",
									SelectionSet: "id",
								},
							},
						},
					},
					mustConfiguration(t, graphql_datasource.ConfigurationInput{
						Fetch: &graphql_datasource.FetchConfiguration{
							URL:    "https://second/",
							Method: "POST",
						},
						SchemaConfiguration: mustSchemaConfig(
							t,
							&graphql_datasource.FederationConfiguration{
								Enabled:    true,
								ServiceSDL: secondSubgraphSDL,
							},
							secondSubgraphSDL,
						),
					}),
				),
			}

			t.Run("run", runWithoutError(ExecutionEngineTestCase{
				schema: func(t *testing.T) *graphql.Schema {
					t.Helper()
					parseSchema, err := graphql.NewSchemaFromString(definition)
					require.NoError(t, err)
					return parseSchema
				}(t),
				operation: func(t *testing.T) graphql.Request {
					return graphql.Request{
						OperationName: "Accounts",
						Query: `
							query Accounts {
								accounts {
									... on User {
										id
										full
									}
								}
							}`,
					}
				},
				dataSources: datasources,
				expectedJSONResponse: `{"data":{"accounts":[
					{"id":"1","full":"User1 full"},
					{"id":"3","full":"User3 full"}
				]}}`,
			}))
		})

		t.Run("do not validate non-nullable @requires dependencies", func(t *testing.T) {
			definition := `
				type Query {
					accounts: [User!]!
				}
				type User {
					id: ID!
					title: String!
					full: String
				}`
			firstSubgraphSDL := `
				type Query {
					accounts: [User!]!
				}
				type User @key(fields: "id") {
					id: ID!
					title: String! @external
					full: String @requires(fields: "title")
				}`
			secondSubgraphSDL := `
				type User @key(fields: "id") {
					id: ID!
					title: String!
				}`
			// Expected:
			// fetch (id-1) returns all,
			// fetch (id-2) returns broken data and maybe an error,
			// fetch (id-1) made with partial data, returns all
			datasources := []plan.DataSource{
				mustGraphqlDataSourceConfiguration(t,
					"id-1",
					mustFactory(t,
						testConditionalNetHttpClient(t, conditionalTestCase{
							expectedHost: "first",
							expectedPath: "/",
							responses: map[string]sendResponse{
								`{"query":"{accounts {id __typename}}"}`: {
									statusCode: 200,
									body:       `{"data":{"accounts":[{"__typename":"User","id":"1"},{"__typename":"User","id":"3"}]}}`,
								},
								`{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {__typename full}}}","variables":{"representations":[{"__typename":"User","title":"User3","id":"3"}]}}`: {
									statusCode: 200,
									body:       `{"data":{"_entities":[{"__typename":"User","full":"User3 full"}]}}`,
								},
							},
						}),
					),
					&plan.DataSourceMetadata{
						RootNodes: []plan.TypeField{
							{
								TypeName:   "Query",
								FieldNames: []string{"accounts"},
							},
							{
								TypeName:           "User",
								FieldNames:         []string{"id", "full"},
								ExternalFieldNames: []string{"title"},
							},
						},
						FederationMetaData: plan.FederationMetaData{
							Keys: plan.FederationFieldConfigurations{
								{
									TypeName:     "User",
									SelectionSet: "id",
								},
							},
							Requires: plan.FederationFieldConfigurations{
								{
									TypeName:     "User",
									FieldName:    "full",
									SelectionSet: "title",
								},
							},
						},
					},
					mustConfiguration(t, graphql_datasource.ConfigurationInput{
						Fetch: &graphql_datasource.FetchConfiguration{
							URL:    "https://first/",
							Method: "POST",
						},
						SchemaConfiguration: mustSchemaConfig(
							t,
							&graphql_datasource.FederationConfiguration{
								Enabled:    true,
								ServiceSDL: firstSubgraphSDL,
							},
							firstSubgraphSDL,
						),
					}),
				),
				mustGraphqlDataSourceConfiguration(t,
					"id-2",
					mustFactory(t,
						testNetHttpClient(t, roundTripperTestCase{
							expectedHost:     "second",
							expectedPath:     "/",
							expectedBody:     `{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {__typename title}}}","variables":{"representations":[{"__typename":"User","id":"1"},{"__typename":"User","id":"3"}]}}`,
							sendResponseBody: `{"data":{"_entities":[{"__typename":"User","title":null},{"__typename":"User","title":"User3"}]},"errors":[{"message":"Cannot provide value","locations":[{"line":1,"column":30}],"path":["_entities",0,"title"]}]}`,
							sendStatusCode:   200,
						}),
					),
					&plan.DataSourceMetadata{
						RootNodes: []plan.TypeField{
							{
								TypeName:   "User",
								FieldNames: []string{"id", "title"},
							},
						},
						FederationMetaData: plan.FederationMetaData{
							Keys: plan.FederationFieldConfigurations{
								{
									TypeName:     "User",
									SelectionSet: "id",
								},
							},
						},
					},
					mustConfiguration(t, graphql_datasource.ConfigurationInput{
						Fetch: &graphql_datasource.FetchConfiguration{
							URL:    "https://second/",
							Method: "POST",
						},
						SchemaConfiguration: mustSchemaConfig(
							t,
							&graphql_datasource.FederationConfiguration{
								Enabled:    true,
								ServiceSDL: secondSubgraphSDL,
							},
							secondSubgraphSDL,
						),
					}),
				),
			}

			t.Run("run", runWithoutError(ExecutionEngineTestCase{
				schema: func(t *testing.T) *graphql.Schema {
					t.Helper()
					parseSchema, err := graphql.NewSchemaFromString(definition)
					require.NoError(t, err)
					return parseSchema
				}(t),
				operation: func(t *testing.T) graphql.Request {
					return graphql.Request{
						OperationName: "Accounts",
						Query: `
							query Accounts {
								accounts {
									... on User {
										id
										full
									}
								}
							}`,
					}
				},
				dataSources: datasources,
				// The error message is not affected by the handling of optional "requires" fields.
				expectedJSONResponse: `{"data":{
					"accounts":[
						{"id":"1","full":null},
						{"id":"3","full":"User3 full"}
					]},"errors":[
						{"message":"Failed to fetch from Subgraph 'id-2' at Path 'accounts'."}
					]}`,
			}, withFetchReasons(), validateRequiredExternalFields()))
		})

		t.Run("validate nullable @requires dependencies", func(t *testing.T) {
			definition := `
				type Query {
					accounts: [User!]!
				}
				type User {
					id: ID!
					title: String
					full: String
				}`
			firstSubgraphSDL := `
				type Query {
					accounts: [User!]!
				}
				type User @key(fields: "id") {
					id: ID!
					title: String @external
					full: String @requires(fields: "title")
				}`
			secondSubgraphSDL := `
				type User @key(fields: "id") {
					id: ID!
					title: String
				}`
			// Expected:
			// fetch (id-1) returns all,
			// fetch (id-2) returns partial data and error,
			// fetch (id-1) made with partial data, returns all
			datasources := []plan.DataSource{
				mustGraphqlDataSourceConfiguration(t,
					"id-1",
					mustFactory(t,
						testConditionalNetHttpClient(t, conditionalTestCase{
							expectedHost: "first",
							expectedPath: "/",
							responses: map[string]sendResponse{
								`{"query":"{accounts {id __typename}}"}`: {
									statusCode: 200,
									body:       `{"data":{"accounts":[{"__typename":"User","id":"1"},{"__typename":"User","id":"3"}]}}`,
								},
								`{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {__typename full}}}","variables":{"representations":[{"__typename":"User","title":"User3","id":"3"}]}}`: {
									statusCode: 200,
									body:       `{"data":{"_entities":[{"__typename":"User","full":"User3 full"}]}}`,
								},
							},
						}),
					),
					&plan.DataSourceMetadata{
						RootNodes: []plan.TypeField{
							{
								TypeName:   "Query",
								FieldNames: []string{"accounts"},
							},
							{
								TypeName:           "User",
								FieldNames:         []string{"id", "full"},
								ExternalFieldNames: []string{"title"},
							},
						},
						FederationMetaData: plan.FederationMetaData{
							Keys: plan.FederationFieldConfigurations{
								{
									TypeName:     "User",
									SelectionSet: "id",
								},
							},
							Requires: plan.FederationFieldConfigurations{
								{
									TypeName:     "User",
									FieldName:    "full",
									SelectionSet: "title",
								},
							},
						},
					},
					mustConfiguration(t, graphql_datasource.ConfigurationInput{
						Fetch: &graphql_datasource.FetchConfiguration{
							URL:    "https://first/",
							Method: "POST",
						},
						SchemaConfiguration: mustSchemaConfig(
							t,
							&graphql_datasource.FederationConfiguration{
								Enabled:    true,
								ServiceSDL: firstSubgraphSDL,
							},
							firstSubgraphSDL,
						),
					}),
				),
				mustGraphqlDataSourceConfiguration(t,
					"id-2",
					mustFactory(t,
						testNetHttpClient(t, roundTripperTestCase{
							expectedHost:     "second",
							expectedPath:     "/",
							expectedBody:     `{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {__typename title}}}","variables":{"representations":[{"__typename":"User","id":"1"},{"__typename":"User","id":"3"}]}}`,
							sendResponseBody: `{"data":{"_entities":[{"__typename":"User","title":null},{"__typename":"User","title":"User3"}]},"errors":[{"message":"Cannot provide value","locations":[{"line":1,"column":30}],"path":["_entities",0,"title"]}]}`,
							sendStatusCode:   200,
						}),
					),
					&plan.DataSourceMetadata{
						RootNodes: []plan.TypeField{
							{
								TypeName:   "User",
								FieldNames: []string{"id", "title"},
							},
						},
						FederationMetaData: plan.FederationMetaData{
							Keys: plan.FederationFieldConfigurations{
								{
									TypeName:     "User",
									SelectionSet: "id",
								},
							},
						},
					},
					mustConfiguration(t, graphql_datasource.ConfigurationInput{
						Fetch: &graphql_datasource.FetchConfiguration{
							URL:    "https://second/",
							Method: "POST",
						},
						SchemaConfiguration: mustSchemaConfig(
							t,
							&graphql_datasource.FederationConfiguration{
								Enabled:    true,
								ServiceSDL: secondSubgraphSDL,
							},
							secondSubgraphSDL,
						),
					}),
				),
			}

			t.Run("run", runWithoutError(ExecutionEngineTestCase{
				schema: func(t *testing.T) *graphql.Schema {
					t.Helper()
					parseSchema, err := graphql.NewSchemaFromString(definition)
					require.NoError(t, err)
					return parseSchema
				}(t),
				operation: func(t *testing.T) graphql.Request {
					return graphql.Request{
						OperationName: "Accounts",
						Query: `
					query Accounts {
						accounts {
							... on User {
								id
								full
							}
						}
					}`,
					}
				},
				dataSources: datasources,
				expectedJSONResponse: `{"data":{
					"accounts":[
						{"id":"1","full":null},
						{"id":"3","full":"User3 full"}
					]},"errors":[
						{"message":"Failed to obtain field dependencies from Subgraph 'id-2' at Path 'accounts'."},
						{"message":"Failed to fetch from Subgraph 'id-2' at Path 'accounts'."}
					]}`,
			}, withFetchReasons(), validateRequiredExternalFields()))
		})

		t.Run("validate nested nullable @requires dependencies", func(t *testing.T) {
			definition := `
				type Query {
					accounts: [User!]!
				}
				type User {
					id: ID!
					nested: Nested!
					complex: String
				}
				type Nested {
					property: String
					name: String
				}`
			firstSubgraphSDL := `
				type Query {
					accounts: [User!]!
				}
				type User @key(fields: "id") {
					id: ID!
					nested: Nested! @external
					complex: String @requires(fields: "nested { property name }")
				}
				type Nested {
					property: String @external
					name: String @external
				}`
			secondSubgraphSDL := `
				type User @key(fields: "id") {
					id: ID!
					nested: Nested!
				}
				type Nested {
					property: String
					name: String
				}`

			// expected fetches:
			// id-1, fetch returns all,
			// id-2, fetch returns partial data and error,
			// id-1, fetch made with partial data, returns all
			datasources := []plan.DataSource{
				mustGraphqlDataSourceConfiguration(t,
					"id-1",
					mustFactory(t,
						testConditionalNetHttpClient(t, conditionalTestCase{
							expectedHost: "first",
							expectedPath: "/",
							responses: map[string]sendResponse{
								`{"query":"{accounts {id __typename}}"}`: {
									statusCode: 200,
									body:       `{"data":{"accounts":[{"__typename":"User","id":"1"},{"__typename":"User","id":"2"},{"__typename":"User","id":"3"},{"__typename":"User","id":"4"}]}}`,
								},
								`{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {__typename complex}}}","variables":{"representations":[{"__typename":"User","nested":{"property":"property1","name":"name1"},"id":"1"},{"__typename":"User","nested":{"property":"property4","name":"name4"},"id":"4"}]}}`: {
									statusCode: 200,
									body:       `{"data":{"_entities":[{"__typename":"User","complex":"complex1"},{"__typename":"User","complex":"complex4"}]}}`,
								},
							},
						}),
					),
					&plan.DataSourceMetadata{
						RootNodes: []plan.TypeField{
							{
								TypeName:   "Query",
								FieldNames: []string{"accounts"},
							},
							{
								TypeName:           "User",
								FieldNames:         []string{"id", "complex"},
								ExternalFieldNames: []string{"nested"},
							},
						},
						ChildNodes: []plan.TypeField{
							{
								TypeName:           "Nested",
								ExternalFieldNames: []string{"property", "name"},
							},
						},
						FederationMetaData: plan.FederationMetaData{
							Keys: plan.FederationFieldConfigurations{
								{
									TypeName:     "User",
									SelectionSet: "id",
								},
							},
							Requires: plan.FederationFieldConfigurations{
								{
									TypeName:     "User",
									FieldName:    "complex",
									SelectionSet: "nested { property name }",
								},
							},
						},
					},
					mustConfiguration(t, graphql_datasource.ConfigurationInput{
						Fetch: &graphql_datasource.FetchConfiguration{
							URL:    "https://first/",
							Method: "POST",
						},
						SchemaConfiguration: mustSchemaConfig(
							t,
							&graphql_datasource.FederationConfiguration{
								Enabled:    true,
								ServiceSDL: firstSubgraphSDL,
							},
							firstSubgraphSDL,
						),
					}),
				),
				mustGraphqlDataSourceConfiguration(t,
					"id-2",
					mustFactory(t,
						testNetHttpClient(t, roundTripperTestCase{
							expectedHost: "second",
							expectedPath: "/",
							expectedBody: `{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {__typename nested {__typename property name}}}}","variables":{"representations":[{"__typename":"User","id":"1"},{"__typename":"User","id":"2"},{"__typename":"User","id":"3"},{"__typename":"User","id":"4"}]}}`,
							sendResponseBody: `{"data":{"_entities":[
								{"__typename":"User","nested":{"__typename":"Nested","property":"property1","name":"name1"}},
								{"__typename":"User","nested":{"__typename":"Nested","property":null,"name":"name2"}},
								{"__typename":"User","nested":{"__typename":"Nested","property":"property3","name":null}},
								{"__typename":"User","nested":{"__typename":"Nested","property":"property4","name":"name4"}}
							]},"errors":[
								{"message":"Cannot provide value","locations":[{"line":1,"column":30}],"path":["_entities",1,"nested","property"]},
								{"message":"Cannot provide value","locations":[{"line":1,"column":30}],"path":["_entities",2,"nested","name"]}
							]}`,
							sendStatusCode: 200,
						}),
					),
					&plan.DataSourceMetadata{
						RootNodes: []plan.TypeField{
							{
								TypeName:   "User",
								FieldNames: []string{"id", "nested"},
							},
						},
						FederationMetaData: plan.FederationMetaData{
							Keys: plan.FederationFieldConfigurations{
								{
									TypeName:     "User",
									SelectionSet: "id",
								},
							},
						},
						ChildNodes: []plan.TypeField{
							{
								TypeName:   "Nested",
								FieldNames: []string{"property", "name"},
							},
						},
					},
					mustConfiguration(t, graphql_datasource.ConfigurationInput{
						Fetch: &graphql_datasource.FetchConfiguration{
							URL:    "https://second/",
							Method: "POST",
						},
						SchemaConfiguration: mustSchemaConfig(
							t,
							&graphql_datasource.FederationConfiguration{
								Enabled:    true,
								ServiceSDL: secondSubgraphSDL,
							},
							secondSubgraphSDL,
						),
					}),
				),
			}

			t.Run("run", runWithoutError(ExecutionEngineTestCase{
				schema: func(t *testing.T) *graphql.Schema {
					t.Helper()
					parseSchema, err := graphql.NewSchemaFromString(definition)
					require.NoError(t, err)
					return parseSchema
				}(t),
				operation: func(t *testing.T) graphql.Request {
					return graphql.Request{
						OperationName: "Accounts",
						Query: `
							query Accounts {
								accounts {
									... on User {
										id
										complex
									}
								}
							}`,
					}
				},
				dataSources: datasources,
				expectedJSONResponse: `{"data":{
					"accounts":[
						{"id":"1","complex":"complex1"},
						{"id":"2","complex":null},
						{"id":"3","complex":null},
						{"id":"4","complex":"complex4"}
					]},"errors":[
						{"message":"Failed to obtain field dependencies from Subgraph 'id-2' at Path 'accounts'."},
						{"message":"Failed to fetch from Subgraph 'id-2' at Path 'accounts'."}
					]}`,
			}, withFetchReasons(), validateRequiredExternalFields()))
		})
	})

	t.Run("static cost computation", func(t *testing.T) {
		t.Run("common on star wars scheme", func(t *testing.T) {
			rootNodes := []plan.TypeField{
				{TypeName: "Query", FieldNames: []string{"hero", "droid"}},
				{TypeName: "Human", FieldNames: []string{"name", "height", "friends"}},
				{TypeName: "Droid", FieldNames: []string{"name", "primaryFunction", "friends"}},
			}
			childNodes := []plan.TypeField{
				{TypeName: "Character", FieldNames: []string{"name", "friends"}},
			}
			customConfig := mustConfiguration(t, graphql_datasource.ConfigurationInput{
				Fetch: &graphql_datasource.FetchConfiguration{
					URL:    "https://example.com/",
					Method: "GET",
				},
				SchemaConfiguration: mustSchemaConfig(
					t,
					nil,
					string(graphql.StarwarsSchema(t).RawSchema()),
				),
			})

			t.Run("droid with weighted plain fields", runWithoutError(
				ExecutionEngineTestCase{
					schema: graphql.StarwarsSchema(t),
					operation: func(t *testing.T) graphql.Request {
						return graphql.Request{
							Query: `{
								droid(id: "R2D2") {
									name
									primaryFunction
								}
							}`,
						}
					},
					dataSources: []plan.DataSource{
						mustGraphqlDataSourceConfiguration(t, "id",
							mustFactory(t,
								testNetHttpClient(t, roundTripperTestCase{
									expectedHost: "example.com", expectedPath: "/", expectedBody: "",
									sendResponseBody: `{"data":{"droid":{"name":"R2D2","primaryFunction":"no"}}}`,
									sendStatusCode:   200,
								}),
							),
							&plan.DataSourceMetadata{
								RootNodes:  rootNodes,
								ChildNodes: childNodes,
								CostConfig: &plan.DataSourceCostConfig{
									Weights: map[plan.FieldCoordinate]*plan.FieldWeight{
										{TypeName: "Droid", FieldName: "name"}: {HasWeight: true, Weight: 17},
									},
								}},
							customConfig,
						),
					},
					fields: []plan.FieldConfiguration{
						{
							TypeName: "Query", FieldName: "droid",
							Arguments: []plan.ArgumentConfiguration{
								{
									Name:         "id",
									SourceType:   plan.FieldArgumentSource,
									RenderConfig: plan.RenderArgumentAsGraphQLValue,
								},
							},
						},
					},
					expectedResponse:   `{"data":{"droid":{"name":"R2D2","primaryFunction":"no"}}}`,
					expectedStaticCost: 18, // Query.droid (1) + droid.name (17)
				},
				computeStaticCost(),
			))

			t.Run("droid with weighted plain fields and an argument", runWithoutError(
				ExecutionEngineTestCase{
					schema: graphql.StarwarsSchema(t),
					operation: func(t *testing.T) graphql.Request {
						return graphql.Request{
							Query: `{
								droid(id: "R2D2") {
									name
									primaryFunction
								}
							}`,
						}
					},
					dataSources: []plan.DataSource{
						mustGraphqlDataSourceConfiguration(t, "id",
							mustFactory(t,
								testNetHttpClient(t, roundTripperTestCase{
									expectedHost: "example.com", expectedPath: "/", expectedBody: "",
									sendResponseBody: `{"data":{"droid":{"name":"R2D2","primaryFunction":"no"}}}`,
									sendStatusCode:   200,
								}),
							),
							&plan.DataSourceMetadata{
								RootNodes:  rootNodes,
								ChildNodes: childNodes,
								CostConfig: &plan.DataSourceCostConfig{
									Weights: map[plan.FieldCoordinate]*plan.FieldWeight{
										{TypeName: "Query", FieldName: "droid"}: {
											ArgumentWeights: map[string]int{"id": 3},
											HasWeight:       false,
										},
										{TypeName: "Droid", FieldName: "name"}: {HasWeight: true, Weight: 17},
									},
								}},
							customConfig,
						),
					},
					fields: []plan.FieldConfiguration{
						{
							TypeName: "Query", FieldName: "droid",
							Arguments: []plan.ArgumentConfiguration{
								{
									Name:         "id",
									SourceType:   plan.FieldArgumentSource,
									RenderConfig: plan.RenderArgumentAsGraphQLValue,
								},
							},
						},
					},
					expectedResponse:   `{"data":{"droid":{"name":"R2D2","primaryFunction":"no"}}}`,
					expectedStaticCost: 21, // Query.droid (1) + Query.droid.id (3) + droid.name (17)
				},
				computeStaticCost(),
			))

			t.Run("hero field has weight (returns interface) and with concrete fragment", runWithoutError(
				ExecutionEngineTestCase{
					schema: graphql.StarwarsSchema(t),
					operation: func(t *testing.T) graphql.Request {
						return graphql.Request{
							Query: `{ 
								hero { 
									name 
									... on Human { height }
								}
							}`,
						}
					},
					dataSources: []plan.DataSource{
						mustGraphqlDataSourceConfiguration(t, "id",
							mustFactory(t,
								testNetHttpClient(t, roundTripperTestCase{
									expectedHost: "example.com", expectedPath: "/", expectedBody: "",
									sendResponseBody: `{"data":{"hero":{"__typename":"Human","name":"Luke Skywalker","height":"12"}}}`,
									sendStatusCode:   200,
								}),
							),
							&plan.DataSourceMetadata{RootNodes: rootNodes, ChildNodes: childNodes, CostConfig: &plan.DataSourceCostConfig{
								Weights: map[plan.FieldCoordinate]*plan.FieldWeight{
									{TypeName: "Query", FieldName: "hero"}:   {HasWeight: true, Weight: 2},
									{TypeName: "Human", FieldName: "height"}: {HasWeight: true, Weight: 3},
									{TypeName: "Human", FieldName: "name"}:   {HasWeight: true, Weight: 7},
									{TypeName: "Droid", FieldName: "name"}:   {HasWeight: true, Weight: 17},
								},
								Types: map[string]int{
									"Human": 13,
								},
							}},
							customConfig,
						),
					},
					expectedResponse:   `{"data":{"hero":{"name":"Luke Skywalker","height":"12"}}}`,
					expectedStaticCost: 22, // Query.hero (2) + Human.height (3) + Droid.name (17=max(7, 17))
				},
				computeStaticCost(),
			))

			t.Run("hero field has no weight (returns interface) and with concrete fragment", runWithoutError(
				ExecutionEngineTestCase{
					schema: graphql.StarwarsSchema(t),
					operation: func(t *testing.T) graphql.Request {
						return graphql.Request{
							Query: `{ 
								hero { name }
							}`,
						}
					},
					dataSources: []plan.DataSource{
						mustGraphqlDataSourceConfiguration(t, "id",
							mustFactory(t,
								testNetHttpClient(t, roundTripperTestCase{
									expectedHost: "example.com", expectedPath: "/", expectedBody: "",
									sendResponseBody: `{"data":{"hero":{"__typename":"Human","name":"Luke Skywalker"}}}`,
									sendStatusCode:   200,
								}),
							),
							&plan.DataSourceMetadata{RootNodes: rootNodes, ChildNodes: childNodes, CostConfig: &plan.DataSourceCostConfig{
								Weights: map[plan.FieldCoordinate]*plan.FieldWeight{
									{TypeName: "Human", FieldName: "name"}: {HasWeight: true, Weight: 7},
									{TypeName: "Droid", FieldName: "name"}: {HasWeight: true, Weight: 17},
								},
								Types: map[string]int{
									"Human": 13,
									"Droid": 11,
								},
							}},
							customConfig,
						),
					},
					expectedResponse:   `{"data":{"hero":{"name":"Luke Skywalker"}}}`,
					expectedStaticCost: 30, // Query.Human (13) + Droid.name (17=max(7, 17))
				},
				computeStaticCost(),
			))

			t.Run("query hero without assumedSize on friends", runWithoutError(
				ExecutionEngineTestCase{
					schema: graphql.StarwarsSchema(t),
					operation: func(t *testing.T) graphql.Request {
						return graphql.Request{
							Query: `{ 
								hero {
									friends {
										...on Droid { name primaryFunction }
										...on Human { name height }
									}
								}
							}`,
						}
					},
					dataSources: []plan.DataSource{
						mustGraphqlDataSourceConfiguration(t, "id",
							mustFactory(t,
								testNetHttpClient(t, roundTripperTestCase{
									expectedHost: "example.com", expectedPath: "/", expectedBody: "",
									sendResponseBody: `{"data":{"hero":{"__typename":"Human","friends":[
									{"__typename":"Human","name":"Luke Skywalker","height":"12"},
									{"__typename":"Droid","name":"R2DO","primaryFunction":"joke"}
								]}}}`,
									sendStatusCode: 200,
								}),
							),
							&plan.DataSourceMetadata{
								RootNodes:  rootNodes,
								ChildNodes: childNodes,
								CostConfig: &plan.DataSourceCostConfig{
									Weights: map[plan.FieldCoordinate]*plan.FieldWeight{
										{TypeName: "Human", FieldName: "height"}: {HasWeight: true, Weight: 1},
										{TypeName: "Human", FieldName: "name"}:   {HasWeight: true, Weight: 2},
										{TypeName: "Droid", FieldName: "name"}:   {HasWeight: true, Weight: 2},
									},
									Types: map[string]int{
										"Human": 7,
										"Droid": 5,
									},
								},
							},
							customConfig,
						),
					},
					expectedResponse:   `{"data":{"hero":{"friends":[{"name":"Luke Skywalker","height":"12"},{"name":"R2DO","primaryFunction":"joke"}]}}}`,
					expectedStaticCost: 127, // Query.hero(max(7,5))+10*(Human(max(7,5))+Human.name(2)+Human.height(1)+Droid.name(2))
				},
				computeStaticCost(),
			))

			t.Run("query hero with assumedSize on friends", runWithoutError(
				ExecutionEngineTestCase{
					schema: graphql.StarwarsSchema(t),
					operation: func(t *testing.T) graphql.Request {
						return graphql.Request{
							Query: `{ 
								hero {
									friends {
										...on Droid { name primaryFunction }
										...on Human { name height }
									}
								}
							}`,
						}
					},
					dataSources: []plan.DataSource{
						mustGraphqlDataSourceConfiguration(t, "id",
							mustFactory(t,
								testNetHttpClient(t, roundTripperTestCase{
									expectedHost: "example.com", expectedPath: "/", expectedBody: "",
									sendResponseBody: `{"data":{"hero":{"__typename":"Human","friends":[
									{"__typename":"Human","name":"Luke Skywalker","height":"12"},
									{"__typename":"Droid","name":"R2DO","primaryFunction":"joke"}
								]}}}`,
									sendStatusCode: 200,
								}),
							),
							&plan.DataSourceMetadata{
								RootNodes:  rootNodes,
								ChildNodes: childNodes,
								CostConfig: &plan.DataSourceCostConfig{
									Weights: map[plan.FieldCoordinate]*plan.FieldWeight{
										{TypeName: "Human", FieldName: "height"}: {HasWeight: true, Weight: 1},
										{TypeName: "Human", FieldName: "name"}:   {HasWeight: true, Weight: 2},
										{TypeName: "Droid", FieldName: "name"}:   {HasWeight: true, Weight: 2},
									},
									ListSizes: map[plan.FieldCoordinate]*plan.FieldListSize{
										{TypeName: "Human", FieldName: "friends"}: {AssumedSize: 5},
										{TypeName: "Droid", FieldName: "friends"}: {AssumedSize: 20},
									},
									Types: map[string]int{
										"Human": 7,
										"Droid": 5,
									},
								},
							},
							customConfig,
						),
					},
					expectedResponse:   `{"data":{"hero":{"friends":[{"name":"Luke Skywalker","height":"12"},{"name":"R2DO","primaryFunction":"joke"}]}}}`,
					expectedStaticCost: 247, // Query.hero(max(7,5))+ 20 * (7+2+2+1)
					// We pick maximum on every path independently. This is to reveal the upper boundary.
					// Query.hero: picked maximum weight (Human=7) out of two types (Human, Droid)
					// Query.hero.friends: the max possible weight (7) is for implementing class Human
					// of the returned type of Character; the multiplier picked for the Droid since
					// it is the maximum possible value - we considered the enclosing type that contains it.
				},
				computeStaticCost(),
			))

			t.Run("query hero with assumedSize on friends and weight defined", runWithoutError(
				ExecutionEngineTestCase{
					schema: graphql.StarwarsSchema(t),
					operation: func(t *testing.T) graphql.Request {
						return graphql.Request{
							Query: `{ 
							hero {
								friends {
									...on Droid { name primaryFunction }
									...on Human { name height }
								}
							}
						}`,
						}
					},
					dataSources: []plan.DataSource{
						mustGraphqlDataSourceConfiguration(t, "id",
							mustFactory(t,
								testNetHttpClient(t, roundTripperTestCase{
									expectedHost: "example.com", expectedPath: "/", expectedBody: "",
									sendResponseBody: `{"data":{"hero":{"__typename":"Human","friends":[
									{"__typename":"Human","name":"Luke Skywalker","height":"12"},
									{"__typename":"Droid","name":"R2DO","primaryFunction":"joke"}
								]}}}`,
									sendStatusCode: 200,
								}),
							),
							&plan.DataSourceMetadata{
								RootNodes:  rootNodes,
								ChildNodes: childNodes,
								CostConfig: &plan.DataSourceCostConfig{
									Weights: map[plan.FieldCoordinate]*plan.FieldWeight{
										{TypeName: "Human", FieldName: "friends"}: {HasWeight: true, Weight: 3},
										{TypeName: "Droid", FieldName: "friends"}: {HasWeight: true, Weight: 4},
										{TypeName: "Human", FieldName: "height"}:  {HasWeight: true, Weight: 1},
										{TypeName: "Human", FieldName: "name"}:    {HasWeight: true, Weight: 2},
										{TypeName: "Droid", FieldName: "name"}:    {HasWeight: true, Weight: 2},
									},
									ListSizes: map[plan.FieldCoordinate]*plan.FieldListSize{
										{TypeName: "Human", FieldName: "friends"}: {AssumedSize: 5},
										{TypeName: "Droid", FieldName: "friends"}: {AssumedSize: 20},
									},
									Types: map[string]int{
										"Human": 7,
										"Droid": 5,
									},
								},
							},
							customConfig,
						),
					},
					expectedResponse:   `{"data":{"hero":{"friends":[{"name":"Luke Skywalker","height":"12"},{"name":"R2DO","primaryFunction":"joke"}]}}}`,
					expectedStaticCost: 187, // Query.hero(max(7,5))+ 20 * (4+2+2+1)
				},
				computeStaticCost(),
			))

			t.Run("query hero with empty cost structures", runWithoutError(
				ExecutionEngineTestCase{
					schema: graphql.StarwarsSchema(t),
					operation: func(t *testing.T) graphql.Request {
						return graphql.Request{
							Query: `{ 
								hero {
									friends {
										...on Droid { name primaryFunction }
										...on Human { name height }
									}
								}
							}`,
						}
					},
					dataSources: []plan.DataSource{
						mustGraphqlDataSourceConfiguration(t, "id",
							mustFactory(t,
								testNetHttpClient(t, roundTripperTestCase{
									expectedHost: "example.com", expectedPath: "/", expectedBody: "",
									sendResponseBody: `{"data":{"hero":{"__typename":"Human","friends":[
									{"__typename":"Human","name":"Luke Skywalker","height":"12"},
									{"__typename":"Droid","name":"R2DO","primaryFunction":"joke"}
								]}}}`,
									sendStatusCode: 200,
								}),
							),
							&plan.DataSourceMetadata{
								RootNodes:  rootNodes,
								ChildNodes: childNodes,
								CostConfig: &plan.DataSourceCostConfig{},
							},
							customConfig,
						),
					},
					expectedResponse:   `{"data":{"hero":{"friends":[{"name":"Luke Skywalker","height":"12"},{"name":"R2DO","primaryFunction":"joke"}]}}}`,
					expectedStaticCost: 11, // Query.hero(max(1,1))+ 10 * 1
				},
				computeStaticCost(),
			))

			t.Run("named fragment on interface", runWithoutError(
				ExecutionEngineTestCase{
					schema: graphql.StarwarsSchema(t),
					operation: func(t *testing.T) graphql.Request {
						return graphql.Request{
							Query: `
								fragment CharacterFields on Character {
									name
									friends { name }
								}
								{ hero { ...CharacterFields } }
								`,
						}
					},
					dataSources: []plan.DataSource{
						mustGraphqlDataSourceConfiguration(t, "id",
							mustFactory(t,
								testNetHttpClient(t, roundTripperTestCase{
									expectedHost:     "example.com",
									expectedPath:     "/",
									expectedBody:     "",
									sendResponseBody: `{"data":{"hero":{"__typename":"Human","name":"Luke","friends":[{"name":"Leia"}]}}}`,
									sendStatusCode:   200,
								}),
							),
							&plan.DataSourceMetadata{
								RootNodes:  rootNodes,
								ChildNodes: childNodes,
								CostConfig: &plan.DataSourceCostConfig{
									Weights: map[plan.FieldCoordinate]*plan.FieldWeight{
										{TypeName: "Query", FieldName: "hero"}: {HasWeight: true, Weight: 2},
										{TypeName: "Human", FieldName: "name"}: {HasWeight: true, Weight: 3},
										{TypeName: "Droid", FieldName: "name"}: {HasWeight: true, Weight: 5},
									},
									ListSizes: map[plan.FieldCoordinate]*plan.FieldListSize{
										{TypeName: "Human", FieldName: "friends"}: {AssumedSize: 4},
										{TypeName: "Droid", FieldName: "friends"}: {AssumedSize: 6},
									},
									Types: map[string]int{
										"Human": 2,
										"Droid": 3,
									},
								},
							},
							customConfig,
						),
					},
					expectedResponse: `{"data":{"hero":{"name":"Luke","friends":[{"name":"Leia"}]}}}`,
					// Cost calculation:
					// Query.hero: 2
					// Character.name: max(Human.name=3, Droid.name=5) = 5
					//   friends listSize: max(4, 6) = 6
					//   Character type: max(Human=2, Droid=3) = 3
					//   name: max(Human.name=3, Droid.name=5) = 5
					// Total: 2 + 5 + 6 * (3 + 5)
					expectedStaticCost: 55,
				},
				computeStaticCost(),
			))

			t.Run("named fragment with concrete type", runWithoutError(
				ExecutionEngineTestCase{
					schema: graphql.StarwarsSchema(t),
					operation: func(t *testing.T) graphql.Request {
						return graphql.Request{
							Query: `
								fragment HumanFields on Human {
									name
									height
								}
								{ hero { ...HumanFields } }
								`,
						}
					},
					dataSources: []plan.DataSource{
						mustGraphqlDataSourceConfiguration(t, "id",
							mustFactory(t,
								testNetHttpClient(t, roundTripperTestCase{
									expectedHost:     "example.com",
									expectedPath:     "/",
									expectedBody:     "",
									sendResponseBody: `{"data":{"hero":{"__typename":"Human","name":"Luke","height":"1.72"}}}`,
									sendStatusCode:   200,
								}),
							),
							&plan.DataSourceMetadata{
								RootNodes:  rootNodes,
								ChildNodes: childNodes,
								CostConfig: &plan.DataSourceCostConfig{
									Weights: map[plan.FieldCoordinate]*plan.FieldWeight{
										{TypeName: "Query", FieldName: "hero"}:   {HasWeight: true, Weight: 2},
										{TypeName: "Human", FieldName: "name"}:   {HasWeight: true, Weight: 3},
										{TypeName: "Human", FieldName: "height"}: {HasWeight: true, Weight: 7},
										{TypeName: "Droid", FieldName: "name"}:   {HasWeight: true, Weight: 5},
									},
									Types: map[string]int{
										"Human": 1,
										"Droid": 1,
									},
								},
							},
							customConfig,
						),
					},
					expectedResponse: `{"data":{"hero":{"name":"Luke","height":"1.72"}}}`,
					// Total: 2 + 3 + 7
					expectedStaticCost: 12,
				},
				computeStaticCost(),
			))

		})

		t.Run("union types", func(t *testing.T) {
			unionSchema := `
			type Query {
			   search(term: String!): [SearchResult!]
			}
			union SearchResult = User | Post | Comment
			type User @key(fields: "id") {
			  id: ID!
			  name: String!
			  email: String!
			}
			type Post @key(fields: "id") {
			  id: ID!
			  title: String!
			  body: String!
			}
			type Comment @key(fields: "id") {
			  id: ID!
			  text: String!
			}
			`
			schema, err := graphql.NewSchemaFromString(unionSchema)
			require.NoError(t, err)

			rootNodes := []plan.TypeField{
				{TypeName: "Query", FieldNames: []string{"search"}},
				{TypeName: "User", FieldNames: []string{"id", "name", "email"}},
				{TypeName: "Post", FieldNames: []string{"id", "title", "body"}},
				{TypeName: "Comment", FieldNames: []string{"id", "text"}},
			}
			childNodes := []plan.TypeField{}
			customConfig := mustConfiguration(t, graphql_datasource.ConfigurationInput{
				Fetch: &graphql_datasource.FetchConfiguration{
					URL:    "https://example.com/",
					Method: "GET",
				},
				SchemaConfiguration: mustSchemaConfig(t, nil, unionSchema),
			})
			fieldConfig := []plan.FieldConfiguration{
				{
					TypeName:  "Query",
					FieldName: "search",
					Path:      []string{"search"},
					Arguments: []plan.ArgumentConfiguration{
						{Name: "term", SourceType: plan.FieldArgumentSource, RenderConfig: plan.RenderArgumentAsGraphQLValue},
					},
				},
			}

			t.Run("union with all member types", runWithoutError(
				ExecutionEngineTestCase{
					schema: schema,
					operation: func(t *testing.T) graphql.Request {
						return graphql.Request{
							Query: `{
							  search(term: "test") {
							    ... on User { name email }
							    ... on Post { title body }
							    ... on Comment { text }
							  }
							}`,
						}
					},
					dataSources: []plan.DataSource{
						mustGraphqlDataSourceConfiguration(t, "id",
							mustFactory(t,
								testNetHttpClient(t, roundTripperTestCase{
									expectedHost:     "example.com",
									expectedPath:     "/",
									expectedBody:     "",
									sendResponseBody: `{"data":{"search":[{"__typename":"User","name":"John","email":"john@test.com"}]}}`,
									sendStatusCode:   200,
								}),
							),
							&plan.DataSourceMetadata{
								RootNodes:  rootNodes,
								ChildNodes: childNodes,
								CostConfig: &plan.DataSourceCostConfig{
									Weights: map[plan.FieldCoordinate]*plan.FieldWeight{
										{TypeName: "User", FieldName: "name"}:    {HasWeight: true, Weight: 2},
										{TypeName: "User", FieldName: "email"}:   {HasWeight: true, Weight: 3},
										{TypeName: "Post", FieldName: "title"}:   {HasWeight: true, Weight: 4},
										{TypeName: "Post", FieldName: "body"}:    {HasWeight: true, Weight: 5},
										{TypeName: "Comment", FieldName: "text"}: {HasWeight: true, Weight: 1},
									},
									ListSizes: map[plan.FieldCoordinate]*plan.FieldListSize{
										{TypeName: "Query", FieldName: "search"}: {AssumedSize: 5},
									},
									Types: map[string]int{
										"User":    2,
										"Post":    3,
										"Comment": 1,
									},
								},
							},
							customConfig,
						),
					},
					fields:           fieldConfig,
					expectedResponse: `{"data":{"search":[{"name":"John","email":"john@test.com"}]}}`,
					// search listSize: 10
					// For each SearchResult, use max across all union members:
					//   Type weight: max(User=2, Post=3, Comment=1) = 3
					//   Fields: all fields from all fragments are counted
					//     (2 + 3) + (4 + 5) + (1) = 15
					// TODO: this is not correct, we should pick a maximum sum among types implementing union.
					//  9 should be used instead of 15
					// Total: 5 * (3 + 15)
					expectedStaticCost: 90,
				},
				computeStaticCost(),
			))

			t.Run("union with weighted search field", runWithoutError(
				ExecutionEngineTestCase{
					schema: schema,
					operation: func(t *testing.T) graphql.Request {
						return graphql.Request{
							Query: `{
							  search(term: "test") {
							    ... on User { name }
							    ... on Post { title }
							  }
							}`,
						}
					},
					dataSources: []plan.DataSource{
						mustGraphqlDataSourceConfiguration(t, "id",
							mustFactory(t,
								testNetHttpClient(t, roundTripperTestCase{
									expectedHost:     "example.com",
									expectedPath:     "/",
									expectedBody:     "",
									sendResponseBody: `{"data":{"search":[{"__typename":"User","name":"John"}]}}`,
									sendStatusCode:   200,
								}),
							),
							&plan.DataSourceMetadata{
								RootNodes:  rootNodes,
								ChildNodes: childNodes,
								CostConfig: &plan.DataSourceCostConfig{
									Weights: map[plan.FieldCoordinate]*plan.FieldWeight{
										{TypeName: "User", FieldName: "name"}:  {HasWeight: true, Weight: 2},
										{TypeName: "Post", FieldName: "title"}: {HasWeight: true, Weight: 5},
									},
									ListSizes: map[plan.FieldCoordinate]*plan.FieldListSize{
										{TypeName: "Query", FieldName: "search"}: {AssumedSize: 3},
									},
									Types: map[string]int{
										"User": 6,
										"Post": 10,
									},
								},
							},
							customConfig,
						),
					},
					fields:           fieldConfig,
					expectedResponse: `{"data":{"search":[{"name":"John"}]}}`,
					// Query.search: max(User=10, Post=6)
					// search listSize: 3
					// Union members:
					//   All fields from all fragments: User.name(2) + Post.title(5)
					// Total: 3 * (10+2+5)
					// TODO: we might correct this by counting only members of one implementing types
					//  of a union when fragments are used.
					expectedStaticCost: 51,
				},
				computeStaticCost(),
			))
		})

		t.Run("listSize", func(t *testing.T) {
			listSchema := `
			type Query {
			   items(first: Int, last: Int): [Item!] 
			}
			type Item @key(fields: "id") {
			  id: ID
			} 
			`
			schemaSlicing, err := graphql.NewSchemaFromString(listSchema)
			require.NoError(t, err)
			rootNodes := []plan.TypeField{
				{TypeName: "Query", FieldNames: []string{"items"}},
				{TypeName: "Item", FieldNames: []string{"id"}},
			}
			childNodes := []plan.TypeField{}
			customConfig := mustConfiguration(t, graphql_datasource.ConfigurationInput{
				Fetch: &graphql_datasource.FetchConfiguration{
					URL:    "https://example.com/",
					Method: "GET",
				},
				SchemaConfiguration: mustSchemaConfig(t, nil, listSchema),
			})
			fieldConfig := []plan.FieldConfiguration{
				{
					TypeName:  "Query",
					FieldName: "items",
					Path:      []string{"items"},
					Arguments: []plan.ArgumentConfiguration{
						{
							Name:         "first",
							SourceType:   plan.FieldArgumentSource,
							RenderConfig: plan.RenderArgumentAsGraphQLValue,
						},
						{
							Name:         "last",
							SourceType:   plan.FieldArgumentSource,
							RenderConfig: plan.RenderArgumentAsGraphQLValue,
						},
					},
				},
			}
			t.Run("multiple slicing arguments as literals", runWithoutError(
				ExecutionEngineTestCase{
					schema: schemaSlicing,
					operation: func(t *testing.T) graphql.Request {
						return graphql.Request{
							Query: `query MultipleSlicingArguments {
							  items(first: 5, last: 12) { id }
							}`,
						}
					},
					dataSources: []plan.DataSource{
						mustGraphqlDataSourceConfiguration(t, "id",
							mustFactory(t,
								testNetHttpClient(t, roundTripperTestCase{
									expectedHost: "example.com", expectedPath: "/", expectedBody: "",
									sendResponseBody: `{"data":{"items":[ {"id":"2"}, {"id":"3"} ]}}`,
									sendStatusCode:   200,
								}),
							),
							&plan.DataSourceMetadata{
								RootNodes:  rootNodes,
								ChildNodes: childNodes,
								CostConfig: &plan.DataSourceCostConfig{
									Weights: map[plan.FieldCoordinate]*plan.FieldWeight{
										{TypeName: "Item", FieldName: "id"}: {HasWeight: true, Weight: 1},
									},
									ListSizes: map[plan.FieldCoordinate]*plan.FieldListSize{
										{TypeName: "Query", FieldName: "items"}: {
											AssumedSize:      8,
											SlicingArguments: []string{"first", "last"},
										},
									},
									Types: map[string]int{
										"Item": 3,
									},
								},
							},
							customConfig,
						),
					},
					fields:             fieldConfig,
					expectedResponse:   `{"data":{"items":[{"id":"2"},{"id":"3"}]}}`,
					expectedStaticCost: 48, // slicingArgument(12) * (Item(3)+Item.id(1))
				},
				computeStaticCost(),
			))
			t.Run("slicing argument as a variable", runWithoutError(
				ExecutionEngineTestCase{
					schema: schemaSlicing,
					operation: func(t *testing.T) graphql.Request {
						return graphql.Request{
							Query: `query SlicingWithVariable($limit: Int!) {
							  items(first: $limit) { id }
							}`,
							Variables: []byte(`{"limit": 25}`),
						}
					},
					dataSources: []plan.DataSource{
						mustGraphqlDataSourceConfiguration(t, "id",
							mustFactory(t,
								testNetHttpClient(t, roundTripperTestCase{
									expectedHost: "example.com", expectedPath: "/", expectedBody: "",
									sendResponseBody: `{"data":{"items":[ {"id":"2"}, {"id":"3"} ]}}`,
									sendStatusCode:   200,
								}),
							),
							&plan.DataSourceMetadata{
								RootNodes:  rootNodes,
								ChildNodes: childNodes,
								CostConfig: &plan.DataSourceCostConfig{
									Weights: map[plan.FieldCoordinate]*plan.FieldWeight{
										{TypeName: "Item", FieldName: "id"}: {HasWeight: true, Weight: 1},
									},
									ListSizes: map[plan.FieldCoordinate]*plan.FieldListSize{
										{TypeName: "Query", FieldName: "items"}: {
											AssumedSize:      8,
											SlicingArguments: []string{"first", "last"},
										},
									},
									Types: map[string]int{
										"Item": 3,
									},
								},
							},
							customConfig,
						),
					},
					fields:             fieldConfig,
					expectedResponse:   `{"data":{"items":[{"id":"2"},{"id":"3"}]}}`,
					expectedStaticCost: 100, // slicingArgument($limit=25) * (Item(3)+Item.id(1))
				},
				computeStaticCost(),
			))
			t.Run("slicing argument not provided falls back to assumedSize", runWithoutError(
				ExecutionEngineTestCase{
					schema: schemaSlicing,
					operation: func(t *testing.T) graphql.Request {
						return graphql.Request{
							Query: `query NoSlicingArg {
							  items { id }
							}`,
							// No slicing arguments provided - should fall back to assumedSize
						}
					},
					dataSources: []plan.DataSource{
						mustGraphqlDataSourceConfiguration(t, "id",
							mustFactory(t,
								testNetHttpClient(t, roundTripperTestCase{
									expectedHost: "example.com", expectedPath: "/", expectedBody: "",
									sendResponseBody: `{"data":{"items":[{"id":"1"},{"id":"2"}]}}`,
									sendStatusCode:   200,
								}),
							),
							&plan.DataSourceMetadata{
								RootNodes:  rootNodes,
								ChildNodes: childNodes,
								CostConfig: &plan.DataSourceCostConfig{
									Weights: map[plan.FieldCoordinate]*plan.FieldWeight{
										{TypeName: "Item", FieldName: "id"}: {HasWeight: true, Weight: 1},
									},
									ListSizes: map[plan.FieldCoordinate]*plan.FieldListSize{
										{TypeName: "Query", FieldName: "items"}: {
											AssumedSize:      15,
											SlicingArguments: []string{"first", "last"},
										},
									},
									Types: map[string]int{
										"Item": 2,
									},
								},
							},
							customConfig,
						),
					},
					fields:             fieldConfig,
					expectedResponse:   `{"data":{"items":[{"id":"1"},{"id":"2"}]}}`,
					expectedStaticCost: 45, // Total: 15 * (2 + 1)
				},
				computeStaticCost(),
			))
			t.Run("zero slicing argument falls back to assumedSize", runWithoutError(
				ExecutionEngineTestCase{
					schema: schemaSlicing,
					operation: func(t *testing.T) graphql.Request {
						return graphql.Request{
							Query: `query ZeroSlicing {
							  items(first: 0) { id }
							}`,
						}
					},
					dataSources: []plan.DataSource{
						mustGraphqlDataSourceConfiguration(t, "id",
							mustFactory(t,
								testNetHttpClient(t, roundTripperTestCase{
									expectedHost: "example.com", expectedPath: "/", expectedBody: "",
									sendResponseBody: `{"data":{"items":[]}}`,
									sendStatusCode:   200,
								}),
							),
							&plan.DataSourceMetadata{
								RootNodes:  rootNodes,
								ChildNodes: childNodes,
								CostConfig: &plan.DataSourceCostConfig{
									Weights: map[plan.FieldCoordinate]*plan.FieldWeight{
										{TypeName: "Item", FieldName: "id"}: {HasWeight: true, Weight: 1},
									},
									ListSizes: map[plan.FieldCoordinate]*plan.FieldListSize{
										{TypeName: "Query", FieldName: "items"}: {
											AssumedSize:      20,
											SlicingArguments: []string{"first", "last"},
										},
									},
									Types: map[string]int{
										"Item": 2,
									},
								},
							},
							customConfig,
						),
					},
					fields:             fieldConfig,
					expectedResponse:   `{"data":{"items":[]}}`,
					expectedStaticCost: 60, // 20 * (2 + 1)
				},
				computeStaticCost(),
			))
			t.Run("negative slicing argument falls back to assumedSize", runWithoutError(
				ExecutionEngineTestCase{
					schema: schemaSlicing,
					operation: func(t *testing.T) graphql.Request {
						return graphql.Request{
							Query: `query NegativeSlicing {
							  items(first: -5) { id }
							}`,
						}
					},
					dataSources: []plan.DataSource{
						mustGraphqlDataSourceConfiguration(t, "id",
							mustFactory(t,
								testNetHttpClient(t, roundTripperTestCase{
									expectedHost: "example.com", expectedPath: "/", expectedBody: "",
									sendResponseBody: `{"data":{"items":[]}}`,
									sendStatusCode:   200,
								}),
							),
							&plan.DataSourceMetadata{
								RootNodes:  rootNodes,
								ChildNodes: childNodes,
								CostConfig: &plan.DataSourceCostConfig{
									Weights: map[plan.FieldCoordinate]*plan.FieldWeight{
										{TypeName: "Item", FieldName: "id"}: {HasWeight: true, Weight: 1},
									},
									ListSizes: map[plan.FieldCoordinate]*plan.FieldListSize{
										{TypeName: "Query", FieldName: "items"}: {
											AssumedSize:      25,
											SlicingArguments: []string{"first", "last"},
										},
									},
									Types: map[string]int{
										"Item": 2,
									},
								},
							},
							customConfig,
						),
					},
					fields:             fieldConfig,
					expectedResponse:   `{"data":{"items":[]}}`,
					expectedStaticCost: 75, //  25 * (2 + 1)
				},
				computeStaticCost(),
			))

		})

		t.Run("nested lists with compounding multipliers", func(t *testing.T) {
			nestedSchema := `
			type Query {
			   users(first: Int): [User!]
			}
			type User @key(fields: "id") {
			  id: ID!
			  posts(first: Int): [Post!]
			}
			type Post @key(fields: "id") {
			  id: ID!
			  comments(first: Int): [Comment!]
			}
			type Comment @key(fields: "id") {
			  id: ID!
			  text: String!
			}
			`
			schemaNested, err := graphql.NewSchemaFromString(nestedSchema)
			require.NoError(t, err)

			rootNodes := []plan.TypeField{
				{TypeName: "Query", FieldNames: []string{"users"}},
				{TypeName: "User", FieldNames: []string{"id", "posts"}},
				{TypeName: "Post", FieldNames: []string{"id", "comments"}},
				{TypeName: "Comment", FieldNames: []string{"id", "text"}},
			}
			childNodes := []plan.TypeField{}
			customConfig := mustConfiguration(t, graphql_datasource.ConfigurationInput{
				Fetch: &graphql_datasource.FetchConfiguration{
					URL:    "https://example.com/",
					Method: "GET",
				},
				SchemaConfiguration: mustSchemaConfig(t, nil, nestedSchema),
			})
			fieldConfig := []plan.FieldConfiguration{
				{
					TypeName: "Query", FieldName: "users", Path: []string{"users"},
					Arguments: []plan.ArgumentConfiguration{
						{Name: "first", SourceType: plan.FieldArgumentSource, RenderConfig: plan.RenderArgumentAsGraphQLValue},
					},
				},
				{
					TypeName: "User", FieldName: "posts", Path: []string{"posts"},
					Arguments: []plan.ArgumentConfiguration{
						{Name: "first", SourceType: plan.FieldArgumentSource, RenderConfig: plan.RenderArgumentAsGraphQLValue},
					},
				},
				{
					TypeName: "Post", FieldName: "comments", Path: []string{"comments"},
					Arguments: []plan.ArgumentConfiguration{
						{Name: "first", SourceType: plan.FieldArgumentSource, RenderConfig: plan.RenderArgumentAsGraphQLValue},
					},
				},
			}

			t.Run("nested lists with slicing arguments", runWithoutError(
				ExecutionEngineTestCase{
					schema: schemaNested,
					operation: func(t *testing.T) graphql.Request {
						return graphql.Request{
							Query: `{
							  users(first: 10) {
							    posts(first: 5) {
							      comments(first: 3) { text }
							    }
							  }
							}`,
						}
					},
					dataSources: []plan.DataSource{
						mustGraphqlDataSourceConfiguration(t, "id",
							mustFactory(t,
								testNetHttpClient(t, roundTripperTestCase{
									expectedHost:     "example.com",
									expectedPath:     "/",
									expectedBody:     "",
									sendResponseBody: `{"data":{"users":[{"posts":[{"comments":[{"text":"hello"}]}]}]}}`,
									sendStatusCode:   200,
								}),
							),
							&plan.DataSourceMetadata{
								RootNodes:  rootNodes,
								ChildNodes: childNodes,
								CostConfig: &plan.DataSourceCostConfig{
									Weights: map[plan.FieldCoordinate]*plan.FieldWeight{
										{TypeName: "Comment", FieldName: "text"}: {HasWeight: true, Weight: 1},
									},
									ListSizes: map[plan.FieldCoordinate]*plan.FieldListSize{
										{TypeName: "Query", FieldName: "users"}: {
											AssumedSize:      100,
											SlicingArguments: []string{"first"},
										},
										{TypeName: "User", FieldName: "posts"}: {
											AssumedSize:      50,
											SlicingArguments: []string{"first"},
										},
										{TypeName: "Post", FieldName: "comments"}: {
											AssumedSize:      20,
											SlicingArguments: []string{"first"},
										},
									},
									Types: map[string]int{
										"User":    4,
										"Post":    3,
										"Comment": 2,
									},
								},
							},
							customConfig,
						),
					},
					fields:           fieldConfig,
					expectedResponse: `{"data":{"users":[{"posts":[{"comments":[{"text":"hello"}]}]}]}}`,
					// Cost calculation:
					// users(first:10): multiplier 10
					//   User type weight: 4
					//   posts(first:5): multiplier 5
					//     Post type weight: 3
					//     comments(first:3): multiplier 3
					//       Comment type weight: 2
					//       text weight: 1
					// Total: 10 * (4 + 5 * (3 + 3 * (2 + 1)))
					expectedStaticCost: 640,
				},
				computeStaticCost(),
			))

			t.Run("nested lists fallback to assumedSize when slicing arg not provided", runWithoutError(
				ExecutionEngineTestCase{
					schema: schemaNested,
					operation: func(t *testing.T) graphql.Request {
						return graphql.Request{
							Query: `{
							  users(first: 2) {
							    posts {
							      comments(first: 4) { text }
							    }
							  }
							}`,
						}
					},
					dataSources: []plan.DataSource{
						mustGraphqlDataSourceConfiguration(t, "id",
							mustFactory(t,
								testNetHttpClient(t, roundTripperTestCase{
									expectedHost:     "example.com",
									expectedPath:     "/",
									expectedBody:     "",
									sendResponseBody: `{"data":{"users":[{"posts":[{"comments":[{"text":"hi"}]}]}]}}`,
									sendStatusCode:   200,
								}),
							),
							&plan.DataSourceMetadata{
								RootNodes:  rootNodes,
								ChildNodes: childNodes,
								CostConfig: &plan.DataSourceCostConfig{
									Weights: map[plan.FieldCoordinate]*plan.FieldWeight{
										{TypeName: "Comment", FieldName: "text"}: {HasWeight: true, Weight: 1},
									},
									ListSizes: map[plan.FieldCoordinate]*plan.FieldListSize{
										{TypeName: "Query", FieldName: "users"}: {
											AssumedSize:      100,
											SlicingArguments: []string{"first"},
										},
										{TypeName: "User", FieldName: "posts"}: {
											AssumedSize: 50, // no slicing arg, should use this
										},
										{TypeName: "Post", FieldName: "comments"}: {
											AssumedSize:      20,
											SlicingArguments: []string{"first"},
										},
									},
									Types: map[string]int{
										"User":    4,
										"Post":    3,
										"Comment": 2,
									},
								},
							},
							customConfig,
						),
					},
					fields:           fieldConfig,
					expectedResponse: `{"data":{"users":[{"posts":[{"comments":[{"text":"hi"}]}]}]}}`,
					// Cost calculation:
					// users(first:2): multiplier 2
					//   User type weight: 4
					//   posts (no arg): assumedSize 50
					//     Post type weight: 3
					//     comments(first:4): multiplier 4
					//       Comment type weight: 2
					//       text weight: 1
					// Total: 2 * (4 + 50 * (3 + 4 * (2 + 1)))
					expectedStaticCost: 1508,
				},
				computeStaticCost(),
			))
		})

	})
}

func testNetHttpClient(t *testing.T, testCase roundTripperTestCase) *http.Client {
	t.Helper()

	defaultClient := httpclient.DefaultNetHttpClient
	return &http.Client{
		Transport:     createTestRoundTripper(t, testCase),
		CheckRedirect: defaultClient.CheckRedirect,
		Jar:           defaultClient.Jar,
		Timeout:       defaultClient.Timeout,
	}
}

func testConditionalNetHttpClient(t *testing.T, testCase conditionalTestCase) *http.Client {
	t.Helper()

	defaultClient := httpclient.DefaultNetHttpClient
	return &http.Client{
		Transport:     createConditionalTestRoundTripper(t, testCase),
		CheckRedirect: defaultClient.CheckRedirect,
		Jar:           defaultClient.Jar,
		Timeout:       defaultClient.Timeout,
	}
}

func TestExecutionEngine_GetCachedPlan(t *testing.T) {
	schema, err := graphql.NewSchemaFromString(testSubscriptionDefinition)
	require.NoError(t, err)

	gqlRequest := graphql.Request{
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

	differentGqlRequest := graphql.Request{
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

	engineConfig := NewConfiguration(schema)
	engineConfig.SetDataSources([]plan.DataSource{
		mustGraphqlDataSourceConfiguration(t,
			"id",
			mustFactory(t, http.DefaultClient),
			&plan.DataSourceMetadata{
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
			},
			mustConfiguration(t, graphql_datasource.ConfigurationInput{
				Subscription: &graphql_datasource.SubscriptionConfiguration{
					URL: "http://localhost:8080",
				},
				SchemaConfiguration: mustSchemaConfig(
					t,
					nil,
					testSubscriptionDefinition,
				),
			}),
		),
	})

	engine, err := NewExecutionEngine(context.Background(), abstractlogger.NoopLogger, engineConfig, resolve.ResolverOptions{
		MaxConcurrency: 1024,
	})
	require.NoError(t, err)

	t.Run("should reuse cached plan", func(t *testing.T) {
		t.Cleanup(engine.executionPlanCache.Purge)
		require.Equal(t, 0, engine.executionPlanCache.Len())

		firstInternalExecCtx := newInternalExecutionContext()
		firstInternalExecCtx.resolveContext.Request.Header = http.Header{
			http.CanonicalHeaderKey("Authorization"): []string{"123abc"},
		}

		report := operationreport.Report{}
		cachedPlan, _ := engine.getCachedPlan(firstInternalExecCtx, gqlRequest.Document(), schema.Document(), gqlRequest.OperationName, &report)
		_, oldestCachedPlan, _ := engine.executionPlanCache.GetOldest()
		assert.False(t, report.HasErrors())
		assert.Equal(t, 1, engine.executionPlanCache.Len())
		assert.Equal(t, cachedPlan, oldestCachedPlan.(*plan.SubscriptionResponsePlan))

		secondInternalExecCtx := newInternalExecutionContext()
		secondInternalExecCtx.resolveContext.Request.Header = http.Header{
			http.CanonicalHeaderKey("Authorization"): []string{"123abc"},
		}

		cachedPlan, _ = engine.getCachedPlan(secondInternalExecCtx, gqlRequest.Document(), schema.Document(), gqlRequest.OperationName, &report)
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
		cachedPlan, _ := engine.getCachedPlan(firstInternalExecCtx, gqlRequest.Document(), schema.Document(), gqlRequest.OperationName, &report)
		_, oldestCachedPlan, _ := engine.executionPlanCache.GetOldest()
		assert.False(t, report.HasErrors())
		assert.Equal(t, 1, engine.executionPlanCache.Len())
		assert.Equal(t, cachedPlan, oldestCachedPlan.(*plan.SubscriptionResponsePlan))

		secondInternalExecCtx := newInternalExecutionContext()
		secondInternalExecCtx.resolveContext.Request.Header = http.Header{
			http.CanonicalHeaderKey("Authorization"): []string{"xyz098"},
		}

		cachedPlan, _ = engine.getCachedPlan(secondInternalExecCtx, differentGqlRequest.Document(), schema.Document(), differentGqlRequest.OperationName, &report)
		_, oldestCachedPlan, _ = engine.executionPlanCache.GetOldest()
		assert.False(t, report.HasErrors())
		assert.Equal(t, 2, engine.executionPlanCache.Len())
		assert.NotEqual(t, cachedPlan, oldestCachedPlan.(*plan.SubscriptionResponsePlan))
	})
}

func BenchmarkIntrospection(b *testing.B) {
	schema := graphql.StarwarsSchema(b)
	engineConf := NewConfiguration(schema)

	// Read expected response from goldie fixture
	expectedResponseIndented, err := os.ReadFile("testdata/full_introspection.json")
	require.NoError(b, err)

	// Minify the JSON to match the engine output format (preserve key order)
	var buf bytes.Buffer
	err = json.Compact(&buf, expectedResponseIndented)
	require.NoError(b, err)
	expectedResponse := buf.Bytes()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	type benchCase struct {
		engine *ExecutionEngine
		writer *graphql.EngineResultWriter
	}
	newEngine := func() *ExecutionEngine {
		engine, err := NewExecutionEngine(ctx, abstractlogger.NoopLogger, engineConf, resolve.ResolverOptions{
			MaxConcurrency: 1024,
		})
		require.NoError(b, err)

		return engine
	}

	newBenchCase := func() *benchCase {
		writer := graphql.NewEngineResultWriter()
		return &benchCase{
			engine: newEngine(),
			writer: &writer,
		}
	}

	ctx = context.Background()
	req := graphql.StarwarsRequestForQuery(b, starwars.FileIntrospectionQuery)

	writer := graphql.NewEngineResultWriter()
	engine := newEngine()
	require.NoError(b, engine.Execute(ctx, &req, &writer))
	require.Equal(b, string(expectedResponse), writer.String())

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
			require.NoError(b, bc.engine.Execute(ctx, &req, bc.writer))
			require.True(b, bytes.Equal(expectedResponse, bc.writer.Bytes()))
			// no point to use JSONEq here because JSONEq would contaminate results
			pool.Put(bc)
		}
	})

}

func BenchmarkExecutionEngine(b *testing.B) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	type benchCase struct {
		engine *ExecutionEngine
		writer *graphql.EngineResultWriter
	}
	newEngine := func() *ExecutionEngine {
		schema, err := graphql.NewSchemaFromString(`type Query { hello: String}`)
		require.NoError(b, err)

		dsCfg, err := plan.NewDataSourceConfiguration[staticdatasource.Configuration](
			"id",
			&staticdatasource.Factory[staticdatasource.Configuration]{},
			&plan.DataSourceMetadata{
				RootNodes: []plan.TypeField{
					{TypeName: "Query", FieldNames: []string{"hello"}},
				},
			},
			staticdatasource.Configuration{
				Data: `{"hello": "world"}`,
			},
		)
		require.NoError(b, err)

		engineConf := NewConfiguration(schema)
		engineConf.SetDataSources([]plan.DataSource{
			dsCfg,
		})
		engineConf.SetFieldConfigurations([]plan.FieldConfiguration{
			{
				TypeName:  "Query",
				FieldName: "hello",
			},
		})

		engine, err := NewExecutionEngine(ctx, abstractlogger.NoopLogger, engineConf, resolve.ResolverOptions{
			MaxConcurrency: 1024,
		})
		require.NoError(b, err)

		return engine
	}

	newBenchCase := func() *benchCase {
		writer := graphql.NewEngineResultWriter()
		return &benchCase{
			engine: newEngine(),
			writer: &writer,
		}
	}

	ctx = context.Background()
	req := graphql.Request{
		Query: "{hello}",
	}

	writer := graphql.NewEngineResultWriter()
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
			if err := bc.engine.Execute(ctx, &req, bc.writer); err != nil {
				b.Fatal(err)
			}
			pool.Put(bc)
		}
	})

}

func newFederationEngineStaticConfig(ctx context.Context, setup *federationtesting.FederationSetup) (engine *ExecutionEngine, schema *graphql.Schema, err error) {
	accountsSDL, err := federationtesting.LoadTestingSubgraphSDL(federationtesting.UpstreamAccounts)
	if err != nil {
		return
	}

	productsSDL, err := federationtesting.LoadTestingSubgraphSDL(federationtesting.UpstreamProducts)
	if err != nil {
		return
	}

	reviewsSDL, err := federationtesting.LoadTestingSubgraphSDL(federationtesting.UpstreamReviews)
	if err != nil {
		return
	}

	subscriptionClient := graphql_datasource.NewGraphQLSubscriptionClient(
		httpclient.DefaultNetHttpClient,
		httpclient.DefaultNetHttpClient,
		ctx,
	)

	graphqlFactory, err := graphql_datasource.NewFactory(ctx, httpclient.DefaultNetHttpClient, subscriptionClient)
	if err != nil {
		return
	}

	accountsSchemaConfiguration, err := graphql_datasource.NewSchemaConfiguration(
		string(accountsSDL),
		&graphql_datasource.FederationConfiguration{
			Enabled:    true,
			ServiceSDL: string(accountsSDL),
		},
	)
	if err != nil {
		return
	}

	accountsConfiguration, err := graphql_datasource.NewConfiguration(graphql_datasource.ConfigurationInput{
		Fetch: &graphql_datasource.FetchConfiguration{
			URL:    setup.AccountsUpstreamServer.URL,
			Method: http.MethodPost,
		},
		SchemaConfiguration: accountsSchemaConfiguration,
	})
	if err != nil {
		return
	}

	accountsDataSource, err := plan.NewDataSourceConfiguration[graphql_datasource.Configuration](
		"accounts",
		graphqlFactory,
		&plan.DataSourceMetadata{
			RootNodes: []plan.TypeField{
				{
					TypeName:   "Query",
					FieldNames: []string{"me", "identifiable", "histories", "cat"},
				},
				{
					TypeName:   "User",
					FieldNames: []string{"id", "username", "history", "realName"},
				},
				{
					TypeName:   "Product",
					FieldNames: []string{"upc"},
				},
			},
			ChildNodes: []plan.TypeField{
				{
					TypeName:   "Cat",
					FieldNames: []string{"name"},
				},
				{
					TypeName:   "Identifiable",
					FieldNames: []string{"id"},
				},
				{
					TypeName:   "Info",
					FieldNames: []string{"quantity"},
				},
				{
					TypeName:   "Purchase",
					FieldNames: []string{"product", "wallet", "quantity"},
				},
				{
					TypeName:   "Store",
					FieldNames: []string{"location"},
				},
				{
					TypeName:   "Sale",
					FieldNames: []string{"product", "rating", "location"},
				},
				{
					TypeName:   "Wallet",
					FieldNames: []string{"currency", "amount"},
				},
				{
					TypeName:   "WalletType1",
					FieldNames: []string{"currency", "amount", "specialField1"},
				},
				{
					TypeName:   "WalletType2",
					FieldNames: []string{"currency", "amount", "specialField2"},
				},
				{
					TypeName:   "Namer",
					FieldNames: []string{"name"},
				},
				{
					TypeName:   "A",
					FieldNames: []string{"name"},
				},
				{
					TypeName:   "B",
					FieldNames: []string{"name"},
				},
			},
			FederationMetaData: plan.FederationMetaData{
				Keys: plan.FederationFieldConfigurations{
					{
						TypeName:     "User",
						SelectionSet: "id",
					},
					{
						TypeName:     "Product",
						SelectionSet: "upc",
					},
				},
			},
		},
		accountsConfiguration,
	)
	if err != nil {
		return
	}

	productsSchemaConfiguration, err := graphql_datasource.NewSchemaConfiguration(
		string(productsSDL),
		&graphql_datasource.FederationConfiguration{
			Enabled:    true,
			ServiceSDL: string(productsSDL),
		},
	)
	if err != nil {
		return
	}

	productsConfiguration, err := graphql_datasource.NewConfiguration(graphql_datasource.ConfigurationInput{
		Fetch: &graphql_datasource.FetchConfiguration{
			URL:    setup.ProductsUpstreamServer.URL,
			Method: http.MethodPost,
		},
		Subscription: &graphql_datasource.SubscriptionConfiguration{
			URL: setup.ProductsUpstreamServer.URL,
		},
		SchemaConfiguration: productsSchemaConfiguration,
	})
	if err != nil {
		return
	}

	productsDataSource, err := plan.NewDataSourceConfiguration[graphql_datasource.Configuration](
		"products",
		graphqlFactory,
		&plan.DataSourceMetadata{
			RootNodes: []plan.TypeField{
				{
					TypeName:   "Query",
					FieldNames: []string{"topProducts"},
				},
				{
					TypeName:   "Product",
					FieldNames: []string{"upc", "name", "price", "inStock"},
				},
				{
					TypeName:   "Subscription",
					FieldNames: []string{"updatedPrice", "updateProductPrice"},
				},
				{
					TypeName:   "Mutation",
					FieldNames: []string{"setPrice"},
				},
			},
			FederationMetaData: plan.FederationMetaData{
				Keys: plan.FederationFieldConfigurations{
					{
						TypeName:     "Product",
						SelectionSet: "upc",
					},
				},
			},
		},
		productsConfiguration,
	)
	if err != nil {
		return
	}

	reviewsSchemaConfiguration, err := graphql_datasource.NewSchemaConfiguration(
		string(reviewsSDL),
		&graphql_datasource.FederationConfiguration{
			Enabled:    true,
			ServiceSDL: string(reviewsSDL),
		},
	)
	if err != nil {
		return
	}

	reviewsConfiguration, err := graphql_datasource.NewConfiguration(graphql_datasource.ConfigurationInput{
		Fetch: &graphql_datasource.FetchConfiguration{
			URL:    setup.ReviewsUpstreamServer.URL,
			Method: http.MethodPost,
		},
		Subscription: &graphql_datasource.SubscriptionConfiguration{
			URL: setup.ReviewsUpstreamServer.URL,
		},
		SchemaConfiguration: reviewsSchemaConfiguration,
	})
	if err != nil {
		return
	}

	reviewsDataSource, err := plan.NewDataSourceConfiguration[graphql_datasource.Configuration](
		"reviews",
		graphqlFactory,
		&plan.DataSourceMetadata{
			RootNodes: []plan.TypeField{
				{
					TypeName:   "Query",
					FieldNames: []string{"me", "cat"},
				},
				{
					TypeName:   "User",
					FieldNames: []string{"id", "reviews", "realName"},
				},
				{
					TypeName:   "Product",
					FieldNames: []string{"upc", "reviews"},
				},
				{
					TypeName:   "Mutation",
					FieldNames: []string{"addReview"},
				},
			},
			ChildNodes: []plan.TypeField{
				{
					TypeName:   "Cat",
					FieldNames: []string{"name"},
				},
				{
					TypeName:   "Comment",
					FieldNames: []string{"upc", "body"},
				},
				{
					TypeName:   "Review",
					FieldNames: []string{"body", "author", "product", "attachments"},
				},
				{
					TypeName:   "Question",
					FieldNames: []string{"upc", "body"},
				},
				{
					TypeName:   "Rating",
					FieldNames: []string{"upc", "body", "score"},
				},
				{
					TypeName:   "Rating",
					FieldNames: []string{"upc", "body", "score"},
				},
				{
					TypeName:   "Video",
					FieldNames: []string{"upc", "size"},
				},
			},
			FederationMetaData: plan.FederationMetaData{
				Keys: plan.FederationFieldConfigurations{
					{
						TypeName:     "User",
						SelectionSet: "id",
					},
					{
						TypeName:     "Product",
						SelectionSet: "upc",
					},
				},
			},
		},
		reviewsConfiguration,
	)
	if err != nil {
		return
	}

	fieldConfigs := plan.FieldConfigurations{
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
		{
			TypeName:  "Subscription",
			FieldName: "updateProductPrice",
			Arguments: []plan.ArgumentConfiguration{
				{
					Name:       "upc",
					SourceType: plan.FieldArgumentSource,
				},
			},
		},
		{
			TypeName:  "Mutation",
			FieldName: "addReview",
			Arguments: []plan.ArgumentConfiguration{
				{
					Name:       "authorID",
					SourceType: plan.FieldArgumentSource,
				},
				{
					Name:       "upc",
					SourceType: plan.FieldArgumentSource,
				},
				{
					Name:       "review",
					SourceType: plan.FieldArgumentSource,
				},
			},
		},
	}

	schema, err = federationSchema()
	if err != nil {
		return
	}

	engineConfig := NewConfiguration(schema)
	engineConfig.AddDataSource(accountsDataSource)
	engineConfig.AddDataSource(productsDataSource)
	engineConfig.AddDataSource(reviewsDataSource)
	engineConfig.SetFieldConfigurations(fieldConfigs)

	engineConfig.plannerConfig.Debug = plan.DebugConfiguration{
		PrintOperationTransformations: false,
		PrintPlanningPaths:            false,
		PrintQueryPlans:               false,
		ConfigurationVisitor:          false,
		PlanningVisitor:               false,
		DatasourceVisitor:             false,
	}

	engine, err = NewExecutionEngine(ctx, abstractlogger.Noop{}, engineConfig, resolve.ResolverOptions{
		MaxConcurrency: 1024,
	})
	if err != nil {
		return
	}

	return
}

// nolint
func federationSchema() (*graphql.Schema, error) {
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

	return graphql.NewSchemaFromString(rawSchema)
}

func newPollingUpstreamHandler() http.Handler {
	counter := 0
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		counter++
		respBody := fmt.Sprintf(`{"counter":%d}`, counter)
		_, _ = w.Write([]byte(respBody))
	})
}

const testSubscriptionDefinition = `
type Subscription {
	lastRegisteredUser: User
	liveUserCount: Int!
}

type User {
	id: ID!
	username: String!
	email: String!
}
`

const testSubscriptionLastRegisteredUserOperation = `
subscription LastRegisteredUser {
	lastRegisteredUser {
		id
		username
		email
	}
}
`

const testSubscriptionLiveUserCountOperation = `
subscription LiveUserCount {
	liveUserCount
}
`

const enumSDL = `
	type Query {
		enum(enum: Enum!): Enum!
		enums: [Enum!]!
		nullableEnum(enum: Enum): Enum
		nullableEnums: [Enum]!
		nestedEnums: Object!
	}

	enum Enum {
		A
		B
		INACCESSIBLE @inaccessible
	}

	type Object {
		enum(enum: Enum!): Enum!
		nullableEnum(enum: Enum): Enum
		enums: [Enum!]!
		nullableEnums: [Enum]!
	}
`
