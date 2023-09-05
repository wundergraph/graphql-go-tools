package graphql

import (
	"net/http"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	graphqlDataSource "github.com/wundergraph/graphql-go-tools/v2/pkg/engine/datasource/graphql_datasource"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan"
)

func TestProxyEngineConfigFactory_EngineV2Configuration(t *testing.T) {
	schema, err := NewSchemaFromString(graphqlGeneratorSchema)
	require.NoError(t, err)

	client := &http.Client{}
	streamingClient := &http.Client{}

	expectedFieldConfigs := plan.FieldConfigurations{
		{
			TypeName:  "Mutation",
			FieldName: "addUser",
			Arguments: plan.ArgumentsConfigurations{
				{
					Name:       "name",
					SourceType: plan.FieldArgumentSource,
				},
				{
					Name:       "age",
					SourceType: plan.FieldArgumentSource,
				},
				{
					Name:       "language",
					SourceType: plan.FieldArgumentSource,
				},
			},
		},
		{
			TypeName:  "Query",
			FieldName: "_entities",
			Arguments: plan.ArgumentsConfigurations{
				{
					Name:       "representations",
					SourceType: plan.FieldArgumentSource,
				},
			},
		},
	}

	t.Run("engine config with unknown subscription type", func(t *testing.T) {
		upstreamConfig := ProxyUpstreamConfig{
			URL:    "http://localhost:8080",
			Method: http.MethodGet,
			StaticHeaders: map[string][]string{
				"Authorization": {"123abc"},
			},
		}

		configFactory := NewProxyEngineConfigFactory(
			schema,
			upstreamConfig,
			WithProxyHttpClient(client),
			WithProxyStreamingClient(streamingClient),
			WithProxySubscriptionClientFactory(&MockSubscriptionClientFactory{}),
		)
		config, err := configFactory.EngineV2Configuration()
		if !assert.NoError(t, err) {
			return
		}

		expectedDataSource := plan.DataSourceConfiguration{
			RootNodes: []plan.TypeField{
				{
					TypeName:   "Query",
					FieldNames: []string{"me", "_entities"},
				},
				{
					TypeName:   "Mutation",
					FieldNames: []string{"addUser"},
				},
				{
					TypeName:   "Subscription",
					FieldNames: []string{"userCount"},
				},
			},
			ChildNodes: []plan.TypeField{
				{
					TypeName:   "User",
					FieldNames: []string{"id", "name", "age", "language"},
				},
				{
					TypeName:   "Language",
					FieldNames: []string{"code", "name"},
				},
			},
			Factory: &graphqlDataSource.Factory{
				HTTPClient:         client,
				StreamingClient:    streamingClient,
				SubscriptionClient: mockSubscriptionClient,
			},
			Custom: graphqlDataSource.ConfigJson(graphqlDataSource.Configuration{
				Fetch: graphqlDataSource.FetchConfiguration{
					URL:    "http://localhost:8080",
					Method: "GET",
					Header: map[string][]string{
						"Authorization": {"123abc"},
					},
				},
				Subscription: graphqlDataSource.SubscriptionConfiguration{
					URL:    "http://localhost:8080",
					UseSSE: false,
				},
			}),
		}

		expectedConfig := NewEngineV2Configuration(schema)
		expectedConfig.AddDataSource(expectedDataSource)
		expectedConfig.SetFieldConfigurations(expectedFieldConfigs)
		sortFieldConfigurations(config.FieldConfigurations())

		assert.Equal(t, expectedConfig, config)
	})

	t.Run("engine config with specific WS subscription type", func(t *testing.T) {
		upstreamConfig := ProxyUpstreamConfig{
			URL:    "http://localhost:8080",
			Method: http.MethodGet,
			StaticHeaders: map[string][]string{
				"Authorization": {"123abc"},
			},
			SubscriptionType: SubscriptionTypeGraphQLTransportWS,
		}

		configFactory := NewProxyEngineConfigFactory(
			schema,
			upstreamConfig,
			WithProxyHttpClient(client),
			WithProxyStreamingClient(streamingClient),
			WithProxySubscriptionClientFactory(&MockSubscriptionClientFactory{}),
		)
		config, err := configFactory.EngineV2Configuration()
		if !assert.NoError(t, err) {
			return
		}

		expectedDataSource := plan.DataSourceConfiguration{
			RootNodes: []plan.TypeField{
				{
					TypeName:   "Query",
					FieldNames: []string{"me", "_entities"},
				},
				{
					TypeName:   "Mutation",
					FieldNames: []string{"addUser"},
				},
				{
					TypeName:   "Subscription",
					FieldNames: []string{"userCount"},
				},
			},
			ChildNodes: []plan.TypeField{
				{
					TypeName:   "User",
					FieldNames: []string{"id", "name", "age", "language"},
				},
				{
					TypeName:   "Language",
					FieldNames: []string{"code", "name"},
				},
			},
			Factory: &graphqlDataSource.Factory{
				HTTPClient:         client,
				StreamingClient:    streamingClient,
				SubscriptionClient: mockSubscriptionClient,
			},
			Custom: graphqlDataSource.ConfigJson(graphqlDataSource.Configuration{
				Fetch: graphqlDataSource.FetchConfiguration{
					URL:    "http://localhost:8080",
					Method: "GET",
					Header: map[string][]string{
						"Authorization": {"123abc"},
					},
				},
				Subscription: graphqlDataSource.SubscriptionConfiguration{
					URL:    "http://localhost:8080",
					UseSSE: false,
				},
			}),
		}

		expectedConfig := NewEngineV2Configuration(schema)
		expectedConfig.AddDataSource(expectedDataSource)
		expectedConfig.SetFieldConfigurations(expectedFieldConfigs)
		sortFieldConfigurations(config.FieldConfigurations())

		assert.Equal(t, expectedConfig, config)
	})

	t.Run("engine config with SSE subscription type", func(t *testing.T) {
		upstreamConfig := ProxyUpstreamConfig{
			URL:    "http://localhost:8080",
			Method: http.MethodGet,
			StaticHeaders: map[string][]string{
				"Authorization": {"123abc"},
			},
			SubscriptionType: SubscriptionTypeSSE,
		}

		configFactory := NewProxyEngineConfigFactory(
			schema,
			upstreamConfig,
			WithProxyHttpClient(client),
			WithProxyStreamingClient(streamingClient),
			WithProxySubscriptionClientFactory(&MockSubscriptionClientFactory{}),
		)
		config, err := configFactory.EngineV2Configuration()
		if !assert.NoError(t, err) {
			return
		}

		expectedDataSource := plan.DataSourceConfiguration{
			RootNodes: []plan.TypeField{
				{
					TypeName:   "Query",
					FieldNames: []string{"me", "_entities"},
				},
				{
					TypeName:   "Mutation",
					FieldNames: []string{"addUser"},
				},
				{
					TypeName:   "Subscription",
					FieldNames: []string{"userCount"},
				},
			},
			ChildNodes: []plan.TypeField{
				{
					TypeName:   "User",
					FieldNames: []string{"id", "name", "age", "language"},
				},
				{
					TypeName:   "Language",
					FieldNames: []string{"code", "name"},
				},
			},
			Factory: &graphqlDataSource.Factory{
				HTTPClient:         client,
				StreamingClient:    streamingClient,
				SubscriptionClient: mockSubscriptionClient,
			},
			Custom: graphqlDataSource.ConfigJson(graphqlDataSource.Configuration{
				Fetch: graphqlDataSource.FetchConfiguration{
					URL:    "http://localhost:8080",
					Method: "GET",
					Header: map[string][]string{
						"Authorization": {"123abc"},
					},
				},
				Subscription: graphqlDataSource.SubscriptionConfiguration{
					URL:    "http://localhost:8080",
					UseSSE: true,
				},
			}),
		}

		expectedConfig := NewEngineV2Configuration(schema)
		expectedConfig.AddDataSource(expectedDataSource)
		expectedConfig.SetFieldConfigurations(expectedFieldConfigs)
		sortFieldConfigurations(config.FieldConfigurations())

		assert.Equal(t, expectedConfig, config)
	})

}

// sortFieldConfigurations makes field configurations deterministic for testing
func sortFieldConfigurations(fieldConfigs []plan.FieldConfiguration) {
	sort.Slice(fieldConfigs, func(i, j int) bool {
		return fieldConfigs[i].TypeName < fieldConfigs[j].TypeName
	})
}
