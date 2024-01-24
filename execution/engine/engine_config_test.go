package engine

import (
	"context"
	"net/http"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/execution/graphql"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astparser"
	graphqlDataSource "github.com/wundergraph/graphql-go-tools/v2/pkg/engine/datasource/graphql_datasource"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan"
)

func TestNewEngineConfiguration(t *testing.T) {
	var engineConfig Configuration

	t.Run("should create a new engine v2 config", func(t *testing.T) {
		schema, err := graphql.NewSchemaFromString(graphql.CountriesSchema)
		require.NoError(t, err)

		engineConfig = NewConfiguration(schema)
		assert.Len(t, engineConfig.plannerConfig.DataSources, 0)
		assert.Len(t, engineConfig.plannerConfig.Fields, 0)
	})

	t.Run("should successfully add a data source", func(t *testing.T) {
		ds, _ := plan.NewDataSourceConfiguration[any]("1", nil, nil, []byte("1"))
		engineConfig.AddDataSource(ds)

		assert.Len(t, engineConfig.plannerConfig.DataSources, 1)
		assert.Equal(t, ds, engineConfig.plannerConfig.DataSources[0])
	})

	t.Run("should successfully set all data sources", func(t *testing.T) {
		one, _ := plan.NewDataSourceConfiguration[any]("1", nil, nil, []byte("1"))
		two, _ := plan.NewDataSourceConfiguration[any]("2", nil, nil, []byte("2"))
		three, _ := plan.NewDataSourceConfiguration[any]("3", nil, nil, []byte("3"))
		ds := []plan.DataSource{
			one,
			two,
			three,
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

func TestGraphQLDataSourceGenerator_Generate(t *testing.T) {
	client := &http.Client{}
	streamingClient := &http.Client{}
	engineCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

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
		dataSourceConfig := mustConfiguration(t, graphqlDataSource.ConfigurationInput{
			Fetch: &graphqlDataSource.FetchConfiguration{
				URL:    "http://localhost:8080",
				Method: http.MethodGet,
				Header: map[string][]string{
					"Authorization": {"123abc"},
				},
			},
			SchemaConfiguration: mustSchemaConfig(t,
				nil,
				graphqlGeneratorSchema,
			),
		})

		dataSource, err := newGraphQLDataSourceGenerator(engineCtx, &doc).Generate(
			"test",
			dataSourceConfig,
			client,
			WithDataSourceGeneratorSubscriptionClientFactory(&MockSubscriptionClientFactory{}),
		)
		require.NoError(t, err)

		ds, ok := dataSource.(plan.NodesAccess)
		require.True(t, ok)

		assert.Equal(t, expectedRootNodes, ds.ListRootNodes())
		assert.Equal(t, expectedChildNodes, ds.ListChildNodes())
	})

	t.Run("with subscription configuration (SSE)", func(t *testing.T) {
		dataSourceConfig := mustConfiguration(t, graphqlDataSource.ConfigurationInput{
			Fetch: &graphqlDataSource.FetchConfiguration{
				URL:    "http://localhost:8080",
				Method: http.MethodGet,
				Header: map[string][]string{
					"Authorization": {"123abc"},
				},
			},
			Subscription: &graphqlDataSource.SubscriptionConfiguration{
				UseSSE: true,
			},
			SchemaConfiguration: mustSchemaConfig(t,
				nil,
				graphqlGeneratorSchema,
			),
		})

		dataSource, err := newGraphQLDataSourceGenerator(engineCtx, &doc).Generate(
			"test",
			dataSourceConfig,
			client,
			WithDataSourceGeneratorSubscriptionConfiguration(streamingClient, SubscriptionTypeSSE),
			WithDataSourceGeneratorSubscriptionClientFactory(&MockSubscriptionClientFactory{}),
		)
		require.NoError(t, err)

		ds, ok := dataSource.(plan.NodesAccess)
		require.True(t, ok)

		assert.Equal(t, expectedRootNodes, ds.ListRootNodes())
		assert.Equal(t, expectedChildNodes, ds.ListChildNodes())
	})

}

func TestGraphqlFieldConfigurationsGenerator_Generate(t *testing.T) {
	schema, err := graphql.NewSchemaFromString(graphqlGeneratorSchema)
	require.NoError(t, err)

	t.Run("should generate field configs without predefined field configs", func(t *testing.T) {
		fieldConfigurations := newGraphQLFieldConfigsGenerator(schema).Generate()
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
				TypeName:  "User",
				FieldName: "name",
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

		fieldConfigurations := newGraphQLFieldConfigsGenerator(schema).Generate(predefinedFieldConfigs...)
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
				TypeName:  "User",
				FieldName: "name",
			},
		}

		assert.Equal(t, expectedFieldConfigurations, fieldConfigurations)
	})

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
