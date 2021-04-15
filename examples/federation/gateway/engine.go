package main

import (
	"github.com/jensneuse/abstractlogger"
	"github.com/jensneuse/graphql-go-tools/pkg/engine/datasource/graphql_datasource"
	"github.com/jensneuse/graphql-go-tools/pkg/engine/datasource/httpclient"
	"github.com/jensneuse/graphql-go-tools/pkg/engine/plan"
	"github.com/jensneuse/graphql-go-tools/pkg/engine/subscription"
	"github.com/jensneuse/graphql-go-tools/pkg/graphql"
)

func createFieldConfigurations() plan.FieldConfigurations {
	return plan.FieldConfigurations{
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
	}
}

func createDataSourceConfiguration(plannerFactory plan.PlannerFactory) []plan.DataSourceConfiguration {
	return []plan.DataSourceConfiguration{
		{
			RootNodes: []plan.TypeField{
				{
					TypeName:   "Query",
					FieldNames: []string{"me"},
				},
				{
					TypeName:   "User",
					FieldNames: []string{"id", "username"},
				},
			},
			ChildNodes: []plan.TypeField{
				{
					TypeName:   "User",
					FieldNames: []string{"id", "username"},
				},
			},
			Custom: graphql_datasource.ConfigJson(graphql_datasource.Configuration{
				Fetch: graphql_datasource.FetchConfiguration{
					URL:    "http://localhost:4001/query",
					Method: "POST",
				},
				Federation: graphql_datasource.FederationConfiguration{
					Enabled: true,
					ServiceSDL: `
extend type Query {
    me: User
}

type User @key(fields: "id") {
    id: ID!
    username: String!
}
`,
				},
			}),
			Factory: plannerFactory,
		},
		{
			RootNodes: []plan.TypeField{
				{
					TypeName:   "Query",
					FieldNames: []string{"topProducts"},
				},
				{
					TypeName:   "Product",
					FieldNames: []string{"upc", "name", "price"},
				},
				{
					TypeName:   "Subscription",
					FieldNames: []string{"updatedPrice"},
				},
			},
			ChildNodes: []plan.TypeField{
				{
					TypeName:   "Product",
					FieldNames: []string{"upc", "name", "price"},
				},
			},
			Custom: graphql_datasource.ConfigJson(graphql_datasource.Configuration{
				Fetch: graphql_datasource.FetchConfiguration{
					URL:    "http://localhost:4002/query",
					Method: "POST",
				},
				Subscription: graphql_datasource.SubscriptionConfiguration{
					URL: "ws://localhost:4002/query",
				},
				Federation: graphql_datasource.FederationConfiguration{
					Enabled: true,
					ServiceSDL: `
extend type Query {
    topProducts(first: Int = 5): [Product]
}

extend type Subscription {
    updatedPrice: Product!
}

type Product @key(fields: "upc") {
    upc: String!
    name: String!
    price: Int!
}`,
				},
			}),
			Factory: plannerFactory,
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
					FieldNames: []string{"body", "author", "product"},
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
					URL:    "http://localhost:4003/query",
					Method: "POST",
				},
				Federation: graphql_datasource.FederationConfiguration{
					Enabled: true,
					ServiceSDL: `
type Review {
    body: String!
    author: User! @provides(fields: "username")
    product: Product!
}

extend type User @key(fields: "id") {
    id: ID! @external
	username: String! @external
    reviews: [Review]
}

extend type Product @key(fields: "upc") {
    upc: String! @external
    reviews: [Review]
}
`,
				},
			}),
			Factory: plannerFactory,
		},
	}
}

func newEngineConfig(schema *graphql.Schema, fieldConfigurations plan.FieldConfigurations, dataSourcesConfigurations []plan.DataSourceConfiguration) graphql.EngineV2Configuration {
	conf := graphql.NewEngineV2Configuration(schema)

	conf.SetFieldConfigurations(fieldConfigurations)
	conf.SetDataSources(dataSourcesConfigurations)

	return conf
}

func newPlannerFactory() *graphql_datasource.Factory {
	return &graphql_datasource.Factory{
		Client: httpclient.NewNetHttpClient(httpclient.DefaultNetHttpClient),
	}
}

func newEngine(logger abstractlogger.Logger, schema *graphql.Schema, subscriptionManager *subscription.Manager) (*graphql.ExecutionEngineV2, error) {
	plannerFactory := newPlannerFactory()
	engineConfig := newEngineConfig(schema, createFieldConfigurations(), createDataSourceConfiguration(plannerFactory))

	engine, err := graphql.NewExecutionEngineV2WithTriggerManagers(logger, engineConfig, subscriptionManager)
	if err != nil {
		return nil, err
	}

	return engine, err
}
