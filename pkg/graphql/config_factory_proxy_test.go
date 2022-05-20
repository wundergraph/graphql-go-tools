package graphql

import (
	"net/http"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	graphqlDataSource "github.com/wundergraph/graphql-go-tools/pkg/engine/datasource/graphql_datasource"
	"github.com/wundergraph/graphql-go-tools/pkg/engine/plan"
)

func TestProxyEngineConfigFactory_EngineV2Configuration(t *testing.T) {
	schema, err := NewSchemaFromString(graphqlGeneratorSchema)
	require.NoError(t, err)

	client := &http.Client{}
	upstreamConfig := ProxyUpstreamConfig{
		URL:    "http://localhost:8080",
		Method: http.MethodGet,
		StaticHeaders: map[string][]string{
			"Authorization": {"123abc"},
		},
	}
	batchFactory := graphqlDataSource.NewBatchFactory()
	configFactory := NewProxyEngineConfigFactory(schema, upstreamConfig, batchFactory, WithProxyHttpClient(client))
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
			HTTPClient:   client,
			BatchFactory: batchFactory,
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
				URL: "http://localhost:8080",
			},
		}),
	}

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

	expectedConfig := NewEngineV2Configuration(schema)
	expectedConfig.AddDataSource(expectedDataSource)
	expectedConfig.SetFieldConfigurations(expectedFieldConfigs)

	sort.Slice(config.plannerConfig.Fields, func(i, j int) bool { // make slice deterministic again
		return config.plannerConfig.Fields[i].TypeName < config.plannerConfig.Fields[j].TypeName
	})

	assert.Equal(t, expectedConfig, config)
}
