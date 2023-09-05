package graphql

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/astparser"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astprinter"
	graphqlDataSource "github.com/wundergraph/graphql-go-tools/v2/pkg/engine/datasource/graphql_datasource"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan"
)

func TestEngineConfigV2Factory_EngineV2Configuration(t *testing.T) {
	runWithoutError := func(
		t *testing.T,
		httpClient *http.Client,
		streamingClient *http.Client,
		dataSourceConfigs []graphqlDataSource.Configuration,
		baseSchema string,
		expectedConfigFactory func(t *testing.T, baseSchema string) EngineV2Configuration,
	) {
		doc, report := astparser.ParseGraphqlDocumentString(baseSchema)
		if report.HasErrors() {
			require.Fail(t, report.Error())
		}
		printedBaseSchema, err := astprinter.PrintString(&doc, nil)
		require.NoError(t, err)

		engineConfigV2Factory := NewFederationEngineConfigFactory(
			dataSourceConfigs,
			WithFederationHttpClient(httpClient),
			WithFederationStreamingClient(streamingClient),
			WithFederationSubscriptionClientFactory(&MockSubscriptionClientFactory{}),
		)
		config, err := engineConfigV2Factory.EngineV2Configuration()
		assert.NoError(t, err)
		assert.Equal(t, expectedConfigFactory(t, printedBaseSchema), config)
	}

	httpClient := &http.Client{}
	streamingClient := &http.Client{}

	t.Run("should create engine V2 configuration", func(t *testing.T) {
		runWithoutError(t, httpClient, streamingClient, []graphqlDataSource.Configuration{
			{
				Fetch: graphqlDataSource.FetchConfiguration{
					URL: "http://user.service",
				},
				Federation: graphqlDataSource.FederationConfiguration{
					Enabled:    true,
					ServiceSDL: accountSchema,
				},
			},
			{
				Fetch: graphqlDataSource.FetchConfiguration{
					URL: "http://product.service",
				},
				Federation: graphqlDataSource.FederationConfiguration{
					Enabled:    true,
					ServiceSDL: productSchema,
				},
			},
			{
				Fetch: graphqlDataSource.FetchConfiguration{
					URL: "http://review.service",
				},
				Federation: graphqlDataSource.FederationConfiguration{
					Enabled:    true,
					ServiceSDL: reviewSchema,
				},
				Subscription: graphqlDataSource.SubscriptionConfiguration{
					UseSSE: true,
				},
			},
		}, baseFederationSchema, func(t *testing.T, baseSchema string) EngineV2Configuration {
			schema, err := NewSchemaFromString(baseSchema)
			require.NoError(t, err)

			conf := NewEngineV2Configuration(schema)
			conf.SetFieldConfigurations(plan.FieldConfigurations{
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
			})

			conf.SetDataSources([]plan.DataSourceConfiguration{
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
					Custom: graphqlDataSource.ConfigJson(graphqlDataSource.Configuration{
						Fetch: graphqlDataSource.FetchConfiguration{
							URL: "http://user.service",
						},
						Federation: graphqlDataSource.FederationConfiguration{
							Enabled:    true,
							ServiceSDL: accountSchema,
						},
					}),
					Factory: &graphqlDataSource.Factory{
						HTTPClient:         httpClient,
						StreamingClient:    streamingClient,
						SubscriptionClient: mockSubscriptionClient,
					},
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
					},
					ChildNodes: []plan.TypeField{
						{
							TypeName:   "Product",
							FieldNames: []string{"upc", "name", "price"},
						},
					},
					Custom: graphqlDataSource.ConfigJson(graphqlDataSource.Configuration{
						Fetch: graphqlDataSource.FetchConfiguration{
							URL: "http://product.service",
						},
						Federation: graphqlDataSource.FederationConfiguration{
							Enabled:    true,
							ServiceSDL: productSchema,
						},
					}),
					Factory: &graphqlDataSource.Factory{
						HTTPClient:         httpClient,
						StreamingClient:    streamingClient,
						SubscriptionClient: mockSubscriptionClient,
					},
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
							TypeName:   "Product",
							FieldNames: []string{"reviews", "upc"},
						},
						{
							TypeName:   "User",
							FieldNames: []string{"reviews", "id", "username"},
						},
					},
					Factory: &graphqlDataSource.Factory{
						HTTPClient:         httpClient,
						StreamingClient:    streamingClient,
						SubscriptionClient: mockSubscriptionClient,
					},
					Custom: graphqlDataSource.ConfigJson(graphqlDataSource.Configuration{
						Fetch: graphqlDataSource.FetchConfiguration{
							URL: "http://review.service",
						},
						Federation: graphqlDataSource.FederationConfiguration{
							Enabled:    true,
							ServiceSDL: reviewSchema,
						},
						Subscription: graphqlDataSource.SubscriptionConfiguration{
							UseSSE: true,
						},
					}),
				},
			})

			return conf
		})
	})
}

const (
	accountSchema = `
		extend type Query {
			me: User
		} 
		type User @key(fields: "id"){ 
			id: ID! 
			username: String!
		}`
	productSchema = `
		extend type Query {
			topProducts(first: Int = 5): [Product]
		} 
		type Product @key(fields: "upc") {
			upc: String!
			name: String!
			price: Int!
		}`
	reviewSchema = `
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
			upc: String!@external 
			reviews: [Review] 
		}`
	baseFederationSchema = `
	type Query {
		me: User
		topProducts(first: Int = 5): [Product]
	}
	type User {
		id: ID!
		username: String!
		reviews: [Review]
	}
	type Product {
		upc: String!
		name: String!
		price: Int!
		reviews: [Review]
	}
	type Review {
		body: String!
		author: User!
		product: Product!
	}
`
)
