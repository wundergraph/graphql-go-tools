package graphql

import (
	"bytes"
	"context"
	"encoding/json"
	"io/ioutil"
	"net/http"
	"testing"

	"github.com/jensneuse/abstractlogger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jensneuse/graphql-go-tools/pkg/execution/datasource"
	"github.com/jensneuse/graphql-go-tools/pkg/starwars"
)

type testRoundTripper func(req *http.Request) *http.Response

func (t testRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return t(req), nil
}

func TestExecutionEngine_ExecuteWithOptions(t *testing.T) {
	schema := starwarsSchema(t)
	extraVariables := map[string]string{
		"request": `{"Authorization": "Bearer ey123"}`,
	}
	extraVariablesBytes, err := json.Marshal(extraVariables)
	require.NoError(t, err)

	t.Run("execute with custom roundtripper for simple hero query on HttpJsonDatasource", func(t *testing.T) {
		query := starwars.LoadQuery(t, starwars.FileSimpleHeroQuery, nil)
		request := Request{}
		err := UnmarshalRequest(bytes.NewBuffer(query), &request)
		require.NoError(t, err)

		plannerConfig := datasource.PlannerConfiguration{
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
								Host: "example.com",
								URL:  "/",
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

		roundTripper := testRoundTripper(func(req *http.Request) *http.Response {
			assert.Equal(t, "example.com", req.URL.Host)
			assert.Equal(t, "/", req.URL.Path)

			body := bytes.NewBuffer([]byte(`{"hero": {"name": "Luke Skywalker"}}`))
			return &http.Response{StatusCode: 200, Body: ioutil.NopCloser(body)}
		})

		httpJsonOptions := DataSourceHttpJsonOptions{
			HttpClient: &http.Client{
				Transport: roundTripper,
			},
		}

		engine, err := NewExecutionEngine(abstractlogger.NoopLogger, schema, plannerConfig)
		assert.NoError(t, err)

		err = engine.AddHttpJsonDataSourceWithOptions("HttpJsonDataSource", httpJsonOptions)
		assert.NoError(t, err)

		executionRes, err := engine.Execute(context.Background(), &request, ExecutionOptions{ExtraArguments: extraVariablesBytes})
		assert.NoError(t, err)
		assert.Equal(t, `{"data":{"hero":{"name":"Luke Skywalker"}}}`, executionRes.Buffer().String())
	})

	t.Run("execute with custom roundtripper for simple hero query on GraphqlDataSource", func(t *testing.T) {
		query := starwars.LoadQuery(t, starwars.FileSimpleHeroQuery, nil)
		request := Request{}
		err := UnmarshalRequest(bytes.NewBuffer(query), &request)
		require.NoError(t, err)

		plannerConfig := datasource.PlannerConfiguration{
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
								Host: "example.com",
								URL:  "/",
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

		roundTripper := testRoundTripper(func(req *http.Request) *http.Response {
			assert.Equal(t, "example.com", req.URL.Host)
			assert.Equal(t, "/", req.URL.Path)

			body := bytes.NewBuffer([]byte(`{"data":{"hero":{"name":"Luke Skywalker"}}}`))
			return &http.Response{StatusCode: 200, Body: ioutil.NopCloser(body)}
		})

		graphqlOptions := DataSourceGraphqlOptions{
			HttpClient: &http.Client{
				Transport: roundTripper,
			},
		}

		engine, err := NewExecutionEngine(abstractlogger.NoopLogger, schema, plannerConfig)
		assert.NoError(t, err)

		err = engine.AddGraphqlDataSourceWithOptions("GraphqlDataSource", graphqlOptions)
		assert.NoError(t, err)

		executionRes, err := engine.Execute(context.Background(), &request, ExecutionOptions{ExtraArguments: extraVariablesBytes})
		assert.NoError(t, err)
		assert.Equal(t, `{"data":{"hero":{"name":"Luke Skywalker"}}}`, executionRes.Buffer().String())
	})
}
