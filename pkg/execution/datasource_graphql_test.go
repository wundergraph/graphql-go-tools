package execution

import (
	"bytes"
	"context"
	"encoding/json"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jensneuse/abstractlogger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/internal/pkg/unsafeparser"
	"github.com/wundergraph/graphql-go-tools/pkg/astnormalization"
	"github.com/wundergraph/graphql-go-tools/pkg/execution/datasource"
	"github.com/wundergraph/graphql-go-tools/pkg/operationreport"
)

type preSendHttpHookFunc func(ctx datasource.HookContext, req *http.Request)

func (p preSendHttpHookFunc) Execute(ctx datasource.HookContext, req *http.Request) {
	p(ctx, req)
}

type postReceiveHttpHookFunc func(ctx datasource.HookContext, resp *http.Response, body []byte)

func (p postReceiveHttpHookFunc) Execute(ctx datasource.HookContext, resp *http.Response, body []byte) {
	p(ctx, resp, body)
}

var graphqlDataSourceName = "graphql"

func TestGraphqlDataSource_WithPlanning(t *testing.T) {
	type testCase struct {
		definition            string
		operation             datasource.GraphqlRequest
		typeFieldConfigs      []datasource.TypeFieldConfiguration
		hooksFactory          func(t *testing.T) datasource.Hooks
		assertRequestBody     bool
		expectedRequestBodies []string
		upstreamResponses     []string
		expectedResponseBody  string
	}

	run := func(tc testCase) func(t *testing.T) {
		return func(t *testing.T) {
			upstreams := make([]*httptest.Server, len(tc.upstreamResponses))
			for i := 0; i < len(tc.upstreamResponses); i++ {
				if tc.assertRequestBody {
					require.Len(t, tc.expectedRequestBodies, len(tc.upstreamResponses))
				}

				var expectedRequestBody string
				if tc.assertRequestBody {
					expectedRequestBody = tc.expectedRequestBodies[i]
				}

				upstream := upstreamGraphqlServer(t, tc.assertRequestBody, expectedRequestBody, tc.upstreamResponses[i])
				defer upstream.Close()

				upstreams[i] = upstream
			}

			var upstreamURLs []string
			for _, upstream := range upstreams {
				upstreamURLs = append(upstreamURLs, upstream.URL)
			}

			plannerConfig := createPlannerConfigToUpstream(t, upstreamURLs, http.MethodPost, tc.typeFieldConfigs)
			basePlanner, err := datasource.NewBaseDataSourcePlanner([]byte(tc.definition), plannerConfig, abstractlogger.NoopLogger)
			require.NoError(t, err)

			var hooks datasource.Hooks
			if tc.hooksFactory != nil {
				hooks = tc.hooksFactory(t)
			}

			err = basePlanner.RegisterDataSourcePlannerFactory(graphqlDataSourceName, &datasource.GraphQLDataSourcePlannerFactoryFactory{Hooks: hooks})
			require.NoError(t, err)

			definitionDocument := unsafeparser.ParseGraphqlDocumentString(tc.definition)
			operationDocument := unsafeparser.ParseGraphqlDocumentString(tc.operation.Query)

			var report operationreport.Report
			operationDocument.Input.Variables = tc.operation.Variables
			normalizer := astnormalization.NewNormalizer(true, true)
			normalizer.NormalizeOperation(&operationDocument, &definitionDocument, &report)
			require.False(t, report.HasErrors())

			tc.operation.Variables = operationDocument.Input.Variables

			planner := NewPlanner(basePlanner)
			plan := planner.Plan(&operationDocument, &definitionDocument, tc.operation.OperationName, &report)
			require.False(t, report.HasErrors())

			variables, extraArguments := VariablesFromJson(tc.operation.Variables, nil)
			executionContext := Context{
				Context:        context.Background(),
				Variables:      variables,
				ExtraArguments: extraArguments,
			}

			var buf bytes.Buffer
			executor := NewExecutor(nil)
			err = executor.Execute(executionContext, plan, &buf)
			require.NoError(t, err)

			assert.JSONEq(t, tc.expectedResponseBody, buf.String())
		}
	}

	t.Run("should execute a single query without arguments", run(
		testCase{
			definition: countriesSchema,
			operation: datasource.GraphqlRequest{
				OperationName: "",
				Variables:     nil,
				Query:         "{ continents { code name } }",
			},
			typeFieldConfigs: []datasource.TypeFieldConfiguration{
				graphqlTypeFieldConfigContinents,
			},
			assertRequestBody: false,
			upstreamResponses: []string{
				`{ "data": { "continents": [ { "code": "DE", "name": "Germany" } ] } }`,
			},
			expectedResponseBody: `{ "data": { "continents": [ { "code": "DE", "name": "Germany" } ] } }`,
		}),
	)

	t.Run("should execute a single query with arguments", run(
		testCase{
			definition: countriesSchema,
			operation: datasource.GraphqlRequest{
				OperationName: "",
				Variables:     nil,
				Query:         `{ country(code: "DE") { code name } }`,
			},
			typeFieldConfigs: []datasource.TypeFieldConfiguration{
				graphqlTypeFieldConfigCountry,
			},
			assertRequestBody: false,
			upstreamResponses: []string{
				`{ "data": { "country": { "code": "DE", "name": "Germany" } } }`,
			},
			expectedResponseBody: `{ "data": { "country": { "code": "DE", "name": "Germany" } } }`,
		}),
	)

	t.Run("should execute hooks", run(
		testCase{
			definition: countriesSchema,
			operation: datasource.GraphqlRequest{
				OperationName: "",
				Variables:     nil,
				Query:         `{ country(code: "DE") { code name } }`,
			},
			typeFieldConfigs: []datasource.TypeFieldConfiguration{
				graphqlTypeFieldConfigCountry,
			},
			hooksFactory: func(t *testing.T) datasource.Hooks {
				return datasource.Hooks{
					PreSendHttpHook: preSendHttpHookFunc(func(ctx datasource.HookContext, req *http.Request) {
						assert.Equal(t, ctx.TypeName, "Query")
						assert.Equal(t, ctx.FieldName, "country")
						assert.Regexp(t, `http://127.0.0.1:[0-9]+`, req.URL.String())
					}),
					PostReceiveHttpHook: postReceiveHttpHookFunc(func(ctx datasource.HookContext, resp *http.Response, body []byte) {
						assert.Equal(t, ctx.TypeName, "Query")
						assert.Equal(t, ctx.FieldName, "country")
						assert.Equal(t, 200, resp.StatusCode)
						assert.Equal(t, body, []byte(`{ "data": { "country": { "code": "DE", "name": "Germany" } } }`))
					}),
				}
			},
			assertRequestBody: false,
			upstreamResponses: []string{
				`{ "data": { "country": { "code": "DE", "name": "Germany" } } }`,
			},
			expectedResponseBody: `{ "data": { "country": { "code": "DE", "name": "Germany" } } }`,
		}),
	)

	t.Run("should execute a multiple queries in a single query", run(
		testCase{
			definition: countriesSchema,
			operation: datasource.GraphqlRequest{
				OperationName: "",
				Variables:     nil,
				Query:         `{ continents { code name } country(code: "DE") { code name } }`,
			},
			typeFieldConfigs: []datasource.TypeFieldConfiguration{
				graphqlTypeFieldConfigCountry,
				graphqlTypeFieldConfigContinents,
			},
			assertRequestBody: true,
			expectedRequestBodies: []string{
				`{ "operationName": "o", "variables": {"a": "DE"}, "query": "query o($a: ID!){country(code: $a){code name}}" }`,
				`{ "operationName": "o", "variables": {}, "query": "query o {continents {code name}}" }`,
			},
			upstreamResponses: []string{
				`{ "data": { "country": { "code": "DE", "name": "Germany" } } }`,
				`{ "data": { "continents": [ { "code": "DE", "name": "Germany" } ] } }`,
			},
			expectedResponseBody: `{ "data": { "country": { "code": "DE", "name": "Germany" }, "continents": [ { "code": "DE", "name": "Germany" } ] } }`,
		}),
	)

	t.Run("should execute a multiple queries in a single query with same arguments", run(
		testCase{
			definition: countriesSchema,
			operation: datasource.GraphqlRequest{
				OperationName: "",
				Variables:     nil,
				Query:         `{ country(code: "DE") { code name } continent(code: "EU") { code name } }`,
			},
			typeFieldConfigs: []datasource.TypeFieldConfiguration{
				graphqlTypeFieldConfigCountry,
				graphqlTypeFieldConfigContinent,
			},
			assertRequestBody: true,
			expectedRequestBodies: []string{
				`{ "operationName": "o", "variables": {"a": "DE"}, "query": "query o($a: ID!){country(code: $a){code name}}" }`,
				`{ "operationName": "o", "variables": {"b": "EU"}, "query": "query o($b: ID!){continent(code: $b){code name}}" }`,
			},
			upstreamResponses: []string{
				`{ "data": { "country": { "code": "DE", "name": "Germany" } } }`,
				`{ "data": { "continent": { "code": "EU", "name": "Europe" } } }`,
			},
			expectedResponseBody: `{ "data": { "country": { "code": "DE", "name": "Germany" }, "continent": { "code": "EU", "name": "Europe" } } }`,
		}),
	)
}

func upstreamGraphqlServer(t *testing.T, assertRequestBody bool, expectedRequestBody string, response string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.NotNil(t, r.Body)

		bodyBytes, err := ioutil.ReadAll(r.Body)
		require.NoError(t, err)

		if assertRequestBody {
			isEqual := assert.JSONEq(t, expectedRequestBody, string(bodyBytes))
			if !isEqual {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
		}

		_, err = w.Write([]byte(response))
		require.NoError(t, err)
	}))
}

func createPlannerConfigToUpstream(t *testing.T, upstreamURL []string, method string, typeFieldConfigs []datasource.TypeFieldConfiguration) datasource.PlannerConfiguration {
	require.Len(t, upstreamURL, len(typeFieldConfigs))

	for i := 0; i < len(typeFieldConfigs); i++ {
		typeFieldConfigs[i].DataSource.Config = jsonRawMessagify(map[string]interface{}{
			"url":    upstreamURL[i],
			"method": method,
		})
	}

	return datasource.PlannerConfiguration{
		TypeFieldConfigurations: typeFieldConfigs,
	}
}

var graphqlTypeFieldConfigContinents = datasource.TypeFieldConfiguration{
	TypeName:  "Query",
	FieldName: "continents",
	Mapping: &datasource.MappingConfiguration{
		Disabled: false,
		Path:     "continents",
	},
	DataSource: datasource.SourceConfig{
		Name: graphqlDataSourceName,
	},
}

var graphqlTypeFieldConfigContinent = datasource.TypeFieldConfiguration{
	TypeName:  "Query",
	FieldName: "continent",
	Mapping: &datasource.MappingConfiguration{
		Disabled: false,
		Path:     "continent",
	},
	DataSource: datasource.SourceConfig{
		Name: graphqlDataSourceName,
	},
}

var graphqlTypeFieldConfigCountry = datasource.TypeFieldConfiguration{
	TypeName:  "Query",
	FieldName: "country",
	Mapping: &datasource.MappingConfiguration{
		Disabled: false,
		Path:     "country",
	},
	DataSource: datasource.SourceConfig{
		Name: graphqlDataSourceName,
	},
}

func jsonRawMessagify(any interface{}) []byte {
	out, _ := json.Marshal(any)
	return out
}
