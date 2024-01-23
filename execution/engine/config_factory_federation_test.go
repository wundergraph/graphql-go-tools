package engine

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/execution/graphql"
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
		subgraphs []SubgraphConfig,
		baseSchema string,
		expectedConfigFactory func(t *testing.T, baseSchema string) EngineV2Configuration,
	) {
		doc, report := astparser.ParseGraphqlDocumentString(baseSchema)
		if report.HasErrors() {
			require.Fail(t, report.Error())
		}
		printedBaseSchema, err := astprinter.PrintStringIndent(&doc, nil, "  ")
		require.NoError(t, err)

		engineConfigV2Factory := NewFederationEngineConfigFactory(
			subgraphs,
			WithFederationHttpClient(httpClient),
			WithFederationStreamingClient(streamingClient),
			WithFederationSubscriptionClientFactory(&MockSubscriptionClientFactory{}),
		)
		config, err := engineConfigV2Factory.BuildEngineConfiguration()
		assert.NoError(t, err)
		expected := expectedConfigFactory(t, printedBaseSchema)
		assert.Equal(t, expected.plannerConfig, config.plannerConfig)
	}

	httpClient := &http.Client{}
	streamingClient := &http.Client{}

	t.Run("should create engine V2 configuration", func(t *testing.T) {
		runWithoutError(t, httpClient, streamingClient, []SubgraphConfig{
			{
				Name: "account",
				URL:  "http://user.service",
				SDL:  accountSchema,
			},
			{
				Name: "product",
				URL:  "http://product.service",
				SDL:  productSchema,
			},
			{
				Name: "review",
				URL:  "http://review.service",
				SDL:  reviewSchema,
			},
		}, baseFederationSchema, func(t *testing.T, baseSchema string) EngineV2Configuration {
			schema, err := graphql.NewSchemaFromString(baseSchema)
			require.NoError(t, err)

			conf := NewEngineV2Configuration(schema)
			conf.SetFieldConfigurations(plan.FieldConfigurations{
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
					ID: "0",
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
					Custom: graphqlDataSource.ConfigJson(graphqlDataSource.Configuration{
						Fetch: graphqlDataSource.FetchConfiguration{
							URL:    "http://user.service",
							Method: "POST",
							Header: http.Header{},
						},
						Subscription: graphqlDataSource.SubscriptionConfiguration{
							URL: "http://user.service",
						},
						Federation: graphqlDataSource.FederationConfiguration{
							Enabled:    true,
							ServiceSDL: accountSchema,
						},
						UpstreamSchema:         accountUpstreamSchema,
						CustomScalarTypeFields: []graphqlDataSource.SingleTypeField{},
					}),
					Factory: &graphqlDataSource.Factory{
						HTTPClient:         httpClient,
						StreamingClient:    streamingClient,
						SubscriptionClient: mockSubscriptionClient,
					},
					FederationMetaData: plan.FederationMetaData{
						Keys: []plan.FederationFieldConfiguration{
							{
								TypeName:     "User",
								SelectionSet: "id",
							},
						},
					},
				},
				{
					ID: "1",
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
					Custom: graphqlDataSource.ConfigJson(graphqlDataSource.Configuration{
						Fetch: graphqlDataSource.FetchConfiguration{
							URL:    "http://product.service",
							Method: "POST",
							Header: http.Header{},
						},
						Subscription: graphqlDataSource.SubscriptionConfiguration{
							URL: "http://product.service",
						},
						Federation: graphqlDataSource.FederationConfiguration{
							Enabled:    true,
							ServiceSDL: productSchema,
						},
						UpstreamSchema:         productUpstreamSchema,
						CustomScalarTypeFields: []graphqlDataSource.SingleTypeField{},
					}),
					Factory: &graphqlDataSource.Factory{
						HTTPClient:         httpClient,
						StreamingClient:    streamingClient,
						SubscriptionClient: mockSubscriptionClient,
					},
					FederationMetaData: plan.FederationMetaData{
						Keys: []plan.FederationFieldConfiguration{
							{
								TypeName:     "Product",
								SelectionSet: "upc",
							},
						},
					},
				},
				{
					ID: "2",
					RootNodes: []plan.TypeField{
						{
							TypeName:   "User",
							FieldNames: []string{"reviews", "id"},
						},
						{
							TypeName:   "Product",
							FieldNames: []string{"reviews", "upc"},
						},
					},
					ChildNodes: []plan.TypeField{
						{
							TypeName:   "Review",
							FieldNames: []string{"body", "author", "product"},
						},
					},
					Factory: &graphqlDataSource.Factory{
						HTTPClient:         httpClient,
						StreamingClient:    streamingClient,
						SubscriptionClient: mockSubscriptionClient,
					},
					Custom: graphqlDataSource.ConfigJson(graphqlDataSource.Configuration{
						Fetch: graphqlDataSource.FetchConfiguration{
							URL:    "http://review.service",
							Method: "POST",
							Header: http.Header{},
						},
						Subscription: graphqlDataSource.SubscriptionConfiguration{
							URL: "http://review.service",
						},
						Federation: graphqlDataSource.FederationConfiguration{
							Enabled:    true,
							ServiceSDL: reviewSchema,
						},
						UpstreamSchema:         reviewUpstreamSchema,
						CustomScalarTypeFields: []graphqlDataSource.SingleTypeField{},
					}),
					FederationMetaData: plan.FederationMetaData{
						Keys: []plan.FederationFieldConfiguration{
							{
								TypeName:     "User",
								SelectionSet: "id",
							},
							{
								TypeName:     "Product",
								SelectionSet: "upc",
							},
						},
						Provides: []plan.FederationFieldConfiguration{
							{
								TypeName:     "Review",
								FieldName:    "author",
								SelectionSet: "username",
							},
						},
					},
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
	accountUpstreamSchema = `schema {
  query: Query
}

directive @eventsPublish(sourceID: String, topic: String!) on FIELD_DEFINITION

directive @eventsRequest(sourceID: String, topic: String!) on FIELD_DEFINITION

directive @eventsSubscribe(sourceID: String, topic: String!) on FIELD_DEFINITION

directive @extends on INTERFACE | OBJECT

directive @external on FIELD_DEFINITION | OBJECT

directive @key(fields: openfed__FieldSet!, resolvable: Boolean = true) repeatable on INTERFACE | OBJECT

directive @provides(fields: openfed__FieldSet!) on FIELD_DEFINITION

directive @requires(fields: openfed__FieldSet!) on FIELD_DEFINITION

directive @tag(name: String!) repeatable on ARGUMENT_DEFINITION | ENUM | ENUM_VALUE | FIELD_DEFINITION | INPUT_FIELD_DEFINITION | INPUT_OBJECT | INTERFACE | OBJECT | SCALAR | UNION

type Query {
  me: User
}

type User @key(fields: "id") {
  id: ID!
  username: String!
}

scalar openfed__FieldSet`

	productSchema = `
		extend type Query {
			topProducts(first: Int = 5): [Product]
		} 
		type Product @key(fields: "upc") {
			upc: String!
			name: String!
			price: Int!
		}`
	productUpstreamSchema = `schema {
  query: Query
}

directive @eventsPublish(sourceID: String, topic: String!) on FIELD_DEFINITION

directive @eventsRequest(sourceID: String, topic: String!) on FIELD_DEFINITION

directive @eventsSubscribe(sourceID: String, topic: String!) on FIELD_DEFINITION

directive @extends on INTERFACE | OBJECT

directive @external on FIELD_DEFINITION | OBJECT

directive @key(fields: openfed__FieldSet!, resolvable: Boolean = true) repeatable on INTERFACE | OBJECT

directive @provides(fields: openfed__FieldSet!) on FIELD_DEFINITION

directive @requires(fields: openfed__FieldSet!) on FIELD_DEFINITION

directive @tag(name: String!) repeatable on ARGUMENT_DEFINITION | ENUM | ENUM_VALUE | FIELD_DEFINITION | INPUT_FIELD_DEFINITION | INPUT_OBJECT | INTERFACE | OBJECT | SCALAR | UNION

type Product @key(fields: "upc") {
  name: String!
  price: Int!
  upc: String!
}

type Query {
  topProducts(first: Int = 5): [Product]
}

scalar openfed__FieldSet`

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

	reviewUpstreamSchema = `directive @eventsPublish(sourceID: String, topic: String!) on FIELD_DEFINITION

directive @eventsRequest(sourceID: String, topic: String!) on FIELD_DEFINITION

directive @eventsSubscribe(sourceID: String, topic: String!) on FIELD_DEFINITION

directive @extends on INTERFACE | OBJECT

directive @external on FIELD_DEFINITION | OBJECT

directive @key(fields: openfed__FieldSet!, resolvable: Boolean = true) repeatable on INTERFACE | OBJECT

directive @provides(fields: openfed__FieldSet!) on FIELD_DEFINITION

directive @requires(fields: openfed__FieldSet!) on FIELD_DEFINITION

directive @tag(name: String!) repeatable on ARGUMENT_DEFINITION | ENUM | ENUM_VALUE | FIELD_DEFINITION | INPUT_FIELD_DEFINITION | INPUT_OBJECT | INTERFACE | OBJECT | SCALAR | UNION

type Product @key(fields: "upc") {
  reviews: [Review]
  upc: String! @external
}

type Review {
  author: User! @provides(fields: "username")
  body: String!
  product: Product!
}

type User @key(fields: "id") {
  id: ID! @external
  reviews: [Review]
  username: String! @external
}

scalar openfed__FieldSet`

	baseFederationSchema = `
directive @tag(name: String!) repeatable on ARGUMENT_DEFINITION | ENUM | ENUM_VALUE | FIELD_DEFINITION | INPUT_FIELD_DEFINITION | INPUT_OBJECT | INTERFACE | OBJECT | SCALAR | UNION

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

type Query {
  me: User
  topProducts(first: Int = 5): [Product]
}`
)
