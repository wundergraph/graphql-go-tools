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
	"sync"
	"testing"

	"github.com/jensneuse/abstractlogger"
	"github.com/sebdah/goldie/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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
	expectedResponse string
	expectedFixture  string
	customResolveMap map[string]resolve.CustomResolve
	skipReason       string
	indentJSON       bool
}

func TestExecutionEngine_Execute(t *testing.T) {
	run := func(testCase ExecutionEngineTestCase, withError bool, expectedErrorMessage string) func(t *testing.T) {
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
				// PrintOperationTransformations:    true,
				// PrintPlanningPaths:               true,
				// PrintQueryPlans:                  true,
				// ConfigurationVisitor:             true,
				// PlanningVisitor:                  true,
				// DatasourceVisitor:                true,
			}

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			engine, err := NewExecutionEngine(ctx, abstractlogger.Noop{}, engineConf, resolve.ResolverOptions{
				MaxConcurrency: 1024,
			})
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

			assert.Equal(t, testCase.expectedResponse, actualResponse)
			if withError {
				assert.Error(t, err)
				if expectedErrorMessage != "" {
					assert.Contains(t, err.Error(), expectedErrorMessage)
				}
			} else {
				assert.NoError(t, err)
			}
		}
	}

	runWithAndCompareError := func(testCase ExecutionEngineTestCase, expectedErrorMessage string) func(t *testing.T) {
		return run(testCase, true, expectedErrorMessage)
	}

	runWithoutError := func(testCase ExecutionEngineTestCase) func(t *testing.T) {
		return run(testCase, false, "")
	}

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
		cachedPlan := engine.getCachedPlan(firstInternalExecCtx, gqlRequest.Document(), schema.Document(), gqlRequest.OperationName, &report)
		_, oldestCachedPlan, _ := engine.executionPlanCache.GetOldest()
		assert.False(t, report.HasErrors())
		assert.Equal(t, 1, engine.executionPlanCache.Len())
		assert.Equal(t, cachedPlan, oldestCachedPlan.(*plan.SubscriptionResponsePlan))

		secondInternalExecCtx := newInternalExecutionContext()
		secondInternalExecCtx.resolveContext.Request.Header = http.Header{
			http.CanonicalHeaderKey("Authorization"): []string{"123abc"},
		}

		cachedPlan = engine.getCachedPlan(secondInternalExecCtx, gqlRequest.Document(), schema.Document(), gqlRequest.OperationName, &report)
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
		cachedPlan := engine.getCachedPlan(firstInternalExecCtx, gqlRequest.Document(), schema.Document(), gqlRequest.OperationName, &report)
		_, oldestCachedPlan, _ := engine.executionPlanCache.GetOldest()
		assert.False(t, report.HasErrors())
		assert.Equal(t, 1, engine.executionPlanCache.Len())
		assert.Equal(t, cachedPlan, oldestCachedPlan.(*plan.SubscriptionResponsePlan))

		secondInternalExecCtx := newInternalExecutionContext()
		secondInternalExecCtx.resolveContext.Request.Header = http.Header{
			http.CanonicalHeaderKey("Authorization"): []string{"xyz098"},
		}

		cachedPlan = engine.getCachedPlan(secondInternalExecCtx, differentGqlRequest.Document(), schema.Document(), differentGqlRequest.OperationName, &report)
		_, oldestCachedPlan, _ = engine.executionPlanCache.GetOldest()
		assert.False(t, report.HasErrors())
		assert.Equal(t, 2, engine.executionPlanCache.Len())
		assert.NotEqual(t, cachedPlan, oldestCachedPlan.(*plan.SubscriptionResponsePlan))
	})
}

func BenchmarkIntrospection(b *testing.B) {
	schema := graphql.StarwarsSchema(b)
	engineConf := NewConfiguration(schema)

	expectedResponse := []byte(`{"data":{"__schema":{"queryType":{"name":"Query"},"mutationType":{"name":"Mutation"},"subscriptionType":{"name":"Subscription"},"types":[{"kind":"UNION","name":"SearchResult","description":"","fields":null,"inputFields":[],"interfaces":[],"enumValues":null,"possibleTypes":[{"kind":"OBJECT","name":"Human","ofType":null},{"kind":"OBJECT","name":"Droid","ofType":null},{"kind":"OBJECT","name":"Starship","ofType":null}]},{"kind":"OBJECT","name":"Query","description":"","fields":[{"name":"hero","description":"","args":[],"type":{"kind":"INTERFACE","name":"Character","ofType":null},"isDeprecated":true,"deprecationReason":"No longer supported"},{"name":"droid","description":"","args":[{"name":"id","description":"","type":{"kind":"NON_NULL","name":null,"ofType":{"kind":"SCALAR","name":"ID","ofType":null}},"defaultValue":null}],"type":{"kind":"OBJECT","name":"Droid","ofType":null},"isDeprecated":false,"deprecationReason":null},{"name":"search","description":"","args":[{"name":"name","description":"","type":{"kind":"NON_NULL","name":null,"ofType":{"kind":"SCALAR","name":"String","ofType":null}},"defaultValue":null}],"type":{"kind":"UNION","name":"SearchResult","ofType":null},"isDeprecated":false,"deprecationReason":null},{"name":"searchResults","description":"","args":[],"type":{"kind":"LIST","name":null,"ofType":{"kind":"UNION","name":"SearchResult","ofType":null}},"isDeprecated":false,"deprecationReason":null}],"inputFields":[],"interfaces":[],"enumValues":null,"possibleTypes":[]},{"kind":"OBJECT","name":"Mutation","description":"","fields":[{"name":"createReview","description":"","args":[{"name":"episode","description":"","type":{"kind":"NON_NULL","name":null,"ofType":{"kind":"ENUM","name":"Episode","ofType":null}},"defaultValue":null},{"name":"review","description":"","type":{"kind":"NON_NULL","name":null,"ofType":{"kind":"INPUT_OBJECT","name":"ReviewInput","ofType":null}},"defaultValue":null}],"type":{"kind":"OBJECT","name":"Review","ofType":null},"isDeprecated":false,"deprecationReason":null}],"inputFields":[],"interfaces":[],"enumValues":null,"possibleTypes":[]},{"kind":"OBJECT","name":"Subscription","description":"","fields":[{"name":"remainingJedis","description":"","args":[],"type":{"kind":"NON_NULL","name":null,"ofType":{"kind":"SCALAR","name":"Int","ofType":null}},"isDeprecated":false,"deprecationReason":null}],"inputFields":[],"interfaces":[],"enumValues":null,"possibleTypes":[]},{"kind":"INPUT_OBJECT","name":"ReviewInput","description":"","fields":null,"inputFields":[{"name":"stars","description":"","type":{"kind":"NON_NULL","name":null,"ofType":{"kind":"SCALAR","name":"Int","ofType":null}},"defaultValue":null},{"name":"commentary","description":"","type":{"kind":"SCALAR","name":"String","ofType":null},"defaultValue":null}],"interfaces":[],"enumValues":null,"possibleTypes":[]},{"kind":"OBJECT","name":"Review","description":"","fields":[{"name":"id","description":"","args":[],"type":{"kind":"NON_NULL","name":null,"ofType":{"kind":"SCALAR","name":"ID","ofType":null}},"isDeprecated":false,"deprecationReason":null},{"name":"stars","description":"","args":[],"type":{"kind":"NON_NULL","name":null,"ofType":{"kind":"SCALAR","name":"Int","ofType":null}},"isDeprecated":false,"deprecationReason":null},{"name":"commentary","description":"","args":[],"type":{"kind":"SCALAR","name":"String","ofType":null},"isDeprecated":false,"deprecationReason":null}],"inputFields":[],"interfaces":[],"enumValues":null,"possibleTypes":[]},{"kind":"ENUM","name":"Episode","description":"","fields":null,"inputFields":[],"interfaces":[],"enumValues":[{"name":"NEWHOPE","description":"","isDeprecated":false,"deprecationReason":null},{"name":"EMPIRE","description":"","isDeprecated":false,"deprecationReason":null},{"name":"JEDI","description":"","isDeprecated":true,"deprecationReason":"No longer supported"}],"possibleTypes":[]},{"kind":"INTERFACE","name":"Character","description":"","fields":[{"name":"name","description":"","args":[],"type":{"kind":"NON_NULL","name":null,"ofType":{"kind":"SCALAR","name":"String","ofType":null}},"isDeprecated":false,"deprecationReason":null},{"name":"friends","description":"","args":[],"type":{"kind":"LIST","name":null,"ofType":{"kind":"INTERFACE","name":"Character","ofType":null}},"isDeprecated":false,"deprecationReason":null}],"inputFields":[],"interfaces":[],"enumValues":null,"possibleTypes":[{"kind":"OBJECT","name":"Human","ofType":null},{"kind":"OBJECT","name":"Droid","ofType":null}]},{"kind":"OBJECT","name":"Human","description":"","fields":[{"name":"name","description":"","args":[],"type":{"kind":"NON_NULL","name":null,"ofType":{"kind":"SCALAR","name":"String","ofType":null}},"isDeprecated":false,"deprecationReason":null},{"name":"height","description":"","args":[],"type":{"kind":"NON_NULL","name":null,"ofType":{"kind":"SCALAR","name":"String","ofType":null}},"isDeprecated":true,"deprecationReason":"No longer supported"},{"name":"friends","description":"","args":[],"type":{"kind":"LIST","name":null,"ofType":{"kind":"INTERFACE","name":"Character","ofType":null}},"isDeprecated":false,"deprecationReason":null}],"inputFields":[],"interfaces":[{"kind":"INTERFACE","name":"Character","ofType":null}],"enumValues":null,"possibleTypes":[]},{"kind":"OBJECT","name":"Droid","description":"","fields":[{"name":"name","description":"","args":[],"type":{"kind":"NON_NULL","name":null,"ofType":{"kind":"SCALAR","name":"String","ofType":null}},"isDeprecated":false,"deprecationReason":null},{"name":"primaryFunction","description":"","args":[],"type":{"kind":"NON_NULL","name":null,"ofType":{"kind":"SCALAR","name":"String","ofType":null}},"isDeprecated":false,"deprecationReason":null},{"name":"friends","description":"","args":[],"type":{"kind":"LIST","name":null,"ofType":{"kind":"INTERFACE","name":"Character","ofType":null}},"isDeprecated":false,"deprecationReason":null}],"inputFields":[],"interfaces":[{"kind":"INTERFACE","name":"Character","ofType":null}],"enumValues":null,"possibleTypes":[]},{"kind":"INTERFACE","name":"Vehicle","description":"","fields":[{"name":"length","description":"","args":[],"type":{"kind":"NON_NULL","name":null,"ofType":{"kind":"SCALAR","name":"Float","ofType":null}},"isDeprecated":false,"deprecationReason":null}],"inputFields":[],"interfaces":[],"enumValues":null,"possibleTypes":[{"kind":"OBJECT","name":"Starship","ofType":null}]},{"kind":"OBJECT","name":"Starship","description":"","fields":[{"name":"name","description":"","args":[],"type":{"kind":"NON_NULL","name":null,"ofType":{"kind":"SCALAR","name":"String","ofType":null}},"isDeprecated":false,"deprecationReason":null},{"name":"length","description":"","args":[],"type":{"kind":"NON_NULL","name":null,"ofType":{"kind":"SCALAR","name":"Float","ofType":null}},"isDeprecated":false,"deprecationReason":null}],"inputFields":[],"interfaces":[{"kind":"INTERFACE","name":"Vehicle","ofType":null}],"enumValues":null,"possibleTypes":[]},{"kind":"SCALAR","name":"Int","description":"The 'Int' scalar type represents non-fractional signed whole numeric values. Int can represent values between -(2^31) and 2^31 - 1.","fields":null,"inputFields":[],"interfaces":[],"enumValues":null,"possibleTypes":[]},{"kind":"SCALAR","name":"Float","description":"The 'Float' scalar type represents signed double-precision fractional values as specified by [IEEE 754](http://en.wikipedia.org/wiki/IEEE_floating_point).","fields":null,"inputFields":[],"interfaces":[],"enumValues":null,"possibleTypes":[]},{"kind":"SCALAR","name":"String","description":"The 'String' scalar type represents textual data, represented as UTF-8 character sequences. The String type is most often used by GraphQL to represent free-form human-readable text.","fields":null,"inputFields":[],"interfaces":[],"enumValues":null,"possibleTypes":[]},{"kind":"SCALAR","name":"Boolean","description":"The 'Boolean' scalar type represents 'true' or 'false' .","fields":null,"inputFields":[],"interfaces":[],"enumValues":null,"possibleTypes":[]},{"kind":"SCALAR","name":"ID","description":"The 'ID' scalar type represents a unique identifier, often used to refetch an object or as key for a cache. The ID type appears in a JSON response as a String; however, it is not intended to be human-readable. When expected as an input type, any string (such as '4') or integer (such as 4) input value will be accepted as an ID.","fields":null,"inputFields":[],"interfaces":[],"enumValues":null,"possibleTypes":[]}],"directives":[{"name":"include","description":"Directs the executor to include this field or fragment only when the argument is true.","locations":["FIELD","FRAGMENT_SPREAD","INLINE_FRAGMENT"],"args":[{"name":"if","description":"Included when true.","type":{"kind":"NON_NULL","name":null,"ofType":{"kind":"SCALAR","name":"Boolean","ofType":null}},"defaultValue":null}]},{"name":"skip","description":"Directs the executor to skip this field or fragment when the argument is true.","locations":["FIELD","FRAGMENT_SPREAD","INLINE_FRAGMENT"],"args":[{"name":"if","description":"Skipped when true.","type":{"kind":"NON_NULL","name":null,"ofType":{"kind":"SCALAR","name":"Boolean","ofType":null}},"defaultValue":null}]},{"name":"deprecated","description":"Marks an element of a GraphQL schema as no longer supported.","locations":["FIELD_DEFINITION","ENUM_VALUE"],"args":[{"name":"reason","description":"Explains why this element was deprecated, usually also including a suggestion\n    for how to access supported similar data. Formatted in\n    [Markdown](https://daringfireball.net/projects/markdown/).","type":{"kind":"SCALAR","name":"String","ofType":null},"defaultValue":"\"No longer supported\""}]}]}}}`)

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
	require.Equal(b, expectedResponse, writer.Bytes())

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
			if !bytes.Equal(expectedResponse, bc.writer.Bytes()) {
				require.Equal(b, string(expectedResponse), bc.writer.String())
			}

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
				Data: `"world"`,
			},
		)
		require.NoError(b, err)

		engineConf := NewConfiguration(schema)
		engineConf.SetDataSources([]plan.DataSource{
			dsCfg,
		})
		engineConf.SetFieldConfigurations([]plan.FieldConfiguration{
			{
				TypeName:              "Query",
				FieldName:             "hello",
				DisableDefaultMapping: true,
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
			_ = bc.engine.Execute(ctx, &req, bc.writer)
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
