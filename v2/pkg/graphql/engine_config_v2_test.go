package graphql

import (
	"context"
	"net/http"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/astparser"
	graphqlDataSource "github.com/wundergraph/graphql-go-tools/v2/pkg/engine/datasource/graphql_datasource"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan"
)

func TestNewEngineV2Configuration(t *testing.T) {
	var engineConfig EngineV2Configuration

	t.Run("should create a new engine v2 config", func(t *testing.T) {
		schema, err := NewSchemaFromString(countriesSchema)
		require.NoError(t, err)

		engineConfig = NewEngineV2Configuration(schema)
		assert.Len(t, engineConfig.plannerConfig.DataSources, 0)
		assert.Len(t, engineConfig.plannerConfig.Fields, 0)
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

func TestGraphQLDataSourceV2Generator_Generate(t *testing.T) {
	client := &http.Client{}
	streamingClient := &http.Client{}

	doc, report := astparser.ParseGraphqlDocumentString(graphqlGeneratorSchema)
	require.Falsef(t, report.HasErrors(), "document parser report has errors")

	expectedRootNodes := plan.TypeFields{
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
	}
	expectedChildNodes := plan.TypeFields{
		{
			TypeName:   "User",
			FieldNames: []string{"id", "name", "age", "language"},
		},
		{
			TypeName:   "Language",
			FieldNames: []string{"code", "name"},
		},
	}

	t.Run("without subscription configuration", func(t *testing.T) {
		dataSourceConfig := graphqlDataSource.Configuration{
			Fetch: graphqlDataSource.FetchConfiguration{
				URL:    "http://localhost:8080",
				Method: http.MethodGet,
				Header: map[string][]string{
					"Authorization": {"123abc"},
				},
			},
		}

		expectedDataSourceFactory := &graphqlDataSource.Factory{
			HTTPClient:         client,
			StreamingClient:    streamingClient,
			SubscriptionClient: mockSubscriptionClient,
		}

		dataSource, err := newGraphQLDataSourceV2Generator(&doc).Generate(
			dataSourceConfig,
			client,
			WithDataSourceV2GeneratorSubscriptionClientFactory(&MockSubscriptionClientFactory{}),
		)
		require.NoError(t, err)

		assert.Equal(t, expectedRootNodes, dataSource.RootNodes)
		assert.Equal(t, expectedChildNodes, dataSource.ChildNodes)
		assert.Equal(t, expectedDataSourceFactory, dataSource.Factory)
	})

	t.Run("with subscription configuration (SSE)", func(t *testing.T) {
		dataSourceConfig := graphqlDataSource.Configuration{
			Fetch: graphqlDataSource.FetchConfiguration{
				URL:    "http://localhost:8080",
				Method: http.MethodGet,
				Header: map[string][]string{
					"Authorization": {"123abc"},
				},
			},
			Subscription: graphqlDataSource.SubscriptionConfiguration{
				UseSSE: true,
			},
		}

		expectedDataSourceFactory := &graphqlDataSource.Factory{
			HTTPClient:         client,
			StreamingClient:    streamingClient,
			SubscriptionClient: mockSubscriptionClient,
		}

		dataSource, err := newGraphQLDataSourceV2Generator(&doc).Generate(
			dataSourceConfig,
			client,
			WithDataSourceV2GeneratorSubscriptionConfiguration(streamingClient, SubscriptionTypeSSE),
			WithDataSourceV2GeneratorSubscriptionClientFactory(&MockSubscriptionClientFactory{}),
		)
		require.NoError(t, err)

		assert.Equal(t, expectedRootNodes, dataSource.RootNodes)
		assert.Equal(t, expectedChildNodes, dataSource.ChildNodes)
		assert.Equal(t, expectedDataSourceFactory, dataSource.Factory)
	})

}

func TestGraphqlFieldConfigurationsV2Generator_Generate(t *testing.T) {
	schema, err := NewSchemaFromString(graphqlGeneratorSchema)
	require.NoError(t, err)

	t.Run("should generate field configs without predefined field configs", func(t *testing.T) {
		fieldConfigurations := newGraphQLFieldConfigsV2Generator(schema).Generate()
		sort.Slice(fieldConfigurations, func(i, j int) bool { // make the resulting slice deterministic again
			return fieldConfigurations[i].TypeName < fieldConfigurations[j].TypeName
		})

		expectedFieldConfigurations := plan.FieldConfigurations{
			{
				TypeName:  "Mutation",
				FieldName: "addUser",
				Arguments: []plan.ArgumentConfiguration{
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
				Arguments: []plan.ArgumentConfiguration{
					{
						Name:       "representations",
						SourceType: plan.FieldArgumentSource,
					},
				},
			},
		}

		assert.Equal(t, expectedFieldConfigurations, fieldConfigurations)
	})

	t.Run("should generate field configs with predefined field configs", func(t *testing.T) {
		predefinedFieldConfigs := plan.FieldConfigurations{
			{
				TypeName:       "User",
				FieldName:      "name",
				RequiresFields: []string{"id"},
			},
			{
				TypeName:  "Query",
				FieldName: "_entities",
				Arguments: []plan.ArgumentConfiguration{
					{
						Name:       "representations",
						SourceType: plan.FieldArgumentSource,
					},
				},
			},
		}

		fieldConfigurations := newGraphQLFieldConfigsV2Generator(schema).Generate(predefinedFieldConfigs...)
		sort.Slice(fieldConfigurations, func(i, j int) bool { // make the resulting slice deterministic again
			return fieldConfigurations[i].TypeName < fieldConfigurations[j].TypeName
		})

		expectedFieldConfigurations := plan.FieldConfigurations{
			{
				TypeName:  "Mutation",
				FieldName: "addUser",
				Arguments: []plan.ArgumentConfiguration{
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
				Arguments: []plan.ArgumentConfiguration{
					{
						Name:       "representations",
						SourceType: plan.FieldArgumentSource,
					},
				},
			},
			{
				TypeName:       "User",
				FieldName:      "name",
				RequiresFields: []string{"id"},
			},
		}

		assert.Equal(t, expectedFieldConfigurations, fieldConfigurations)
	})

}

func TestEngineV2Configuration_EnableSingleFlight(t *testing.T) {
	schema, err := NewSchemaFromString(graphqlGeneratorSchema)
	require.NoError(t, err)

	conf := NewEngineV2Configuration(schema)
	require.Falsef(t, conf.dataLoaderConfig.EnableSingleFlightLoader, "single flight is not disabled by default")

	conf.EnableSingleFlight(true)
	assert.True(t, conf.dataLoaderConfig.EnableSingleFlightLoader)

	conf.EnableSingleFlight(false)
	assert.False(t, conf.dataLoaderConfig.EnableSingleFlightLoader)
}

var mockSubscriptionClient = &graphqlDataSource.SubscriptionClient{}

type MockSubscriptionClientFactory struct{}

func (m *MockSubscriptionClientFactory) NewSubscriptionClient(httpClient, streamingClient *http.Client, engineCtx context.Context, options ...graphqlDataSource.Options) graphqlDataSource.GraphQLSubscriptionClient {
	return mockSubscriptionClient
}

var graphqlGeneratorSchema = `scalar _Any
	union _Entity = User

	type Query {
		me: User!
		_entities(representations: [_Any!]!): [_Entity]!
	}

	type Mutation {
		addUser(name: String!, age: Int!, language: Language!): User!
	}

	type Subscription {
		userCount: Int!
	}

	type User {
		id: ID!
		name: String!
		age: Int!
		language: Language!
	}

	type Language {
		code: String!
		name: String!
	}
`
