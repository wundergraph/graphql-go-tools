package engine

import (
	"context"
	"net/http"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protojson"

	nodev1 "github.com/wundergraph/cosmo/router/gen/proto/wg/cosmo/node/v1"

	"github.com/wundergraph/graphql-go-tools/execution/graphql"
	graphqlDataSource "github.com/wundergraph/graphql-go-tools/v2/pkg/engine/datasource/graphql_datasource"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan"
)

func TestEngineConfigFactory_EngineConfiguration(t *testing.T) {
	engineCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	runWithoutErrorUsingRouteConfig := func(
		t *testing.T,
		httpClient *http.Client,
		streamingClient *http.Client,
		baseSchema string,
		expectedConfigFactory func(t *testing.T, baseSchema string) Configuration,
	) {
		engineConfigFactory := NewFederationEngineConfigFactory(
			engineCtx,
			nil, // no subgraphsConfigs — RouterConfig comes through the explicit Build call below
			WithFederationHttpClient(httpClient),
			WithFederationStreamingClient(streamingClient),
			WithFederationSubscriptionClientFactory(&MockSubscriptionClientFactory{}),
		)

		data, err := os.ReadFile("testdata/config_factory_federation/config.json")
		require.NoError(t, err)

		// Build the engine configuration using the router config
		var rc1 nodev1.RouterConfig
		assert.NoError(t, protojson.Unmarshal(data, &rc1))
		config, err := engineConfigFactory.BuildEngineConfigurationWithRouterConfig(&rc1)
		assert.NoError(t, err)

		expectedConfig := expectedConfigFactory(t, baseSchema)
		assert.Equal(t, expectedConfig, config)
	}

	httpClient := &http.Client{}
	streamingClient := &http.Client{}

	t.Run("should create engine configuration using router config", func(t *testing.T) {
		runWithoutErrorUsingRouteConfig(t, httpClient, streamingClient, baseFederationSchema, func(t *testing.T, baseSchema string) Configuration {
			schema, err := graphql.NewSchemaFromString(baseSchema)
			require.NoError(t, err)

			conf := NewConfiguration(schema)
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

			gqlFactory, err := graphqlDataSource.NewFactory(engineCtx, httpClient, mockSubscriptionClient)
			require.NoError(t, err)

			conf.SetDataSources([]plan.DataSource{
				mustGraphqlDataSourceConfiguration(t,
					"0",
					gqlFactory,
					&plan.DataSourceMetadata{
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
						ChildNodes: []plan.TypeField{},
						FederationMetaData: plan.FederationMetaData{
							Keys: []plan.FederationFieldConfiguration{
								{
									TypeName:     "User",
									SelectionSet: "id",
								},
							},
							Requires: []plan.FederationFieldConfiguration{},
							Provides: []plan.FederationFieldConfiguration{},
						},
						Directives: plan.NewDirectiveConfigurations([]plan.DirectiveConfiguration{}),
					},
					mustConfiguration(t, graphqlDataSource.ConfigurationInput{
						Fetch: &graphqlDataSource.FetchConfiguration{
							URL:    "http://user.service",
							Header: make(http.Header),
						},
						Subscription: &graphqlDataSource.SubscriptionConfiguration{
							URL: "http://user.service",
						},
						SchemaConfiguration: mustSchemaConfig(
							t,
							&graphqlDataSource.FederationConfiguration{
								Enabled:    true,
								ServiceSDL: accountSchema,
							},
							accountUpstreamSchema,
						),
						CustomScalarTypeFields: []graphqlDataSource.SingleTypeField{},
					}),
				),
				mustGraphqlDataSourceConfiguration(t,
					"1",
					gqlFactory,
					&plan.DataSourceMetadata{
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
						ChildNodes: []plan.TypeField{},
						FederationMetaData: plan.FederationMetaData{
							Keys: []plan.FederationFieldConfiguration{
								{
									TypeName:     "Product",
									SelectionSet: "upc",
								},
							},
							Requires: []plan.FederationFieldConfiguration{},
							Provides: []plan.FederationFieldConfiguration{},
						},
						Directives: plan.NewDirectiveConfigurations([]plan.DirectiveConfiguration{}),
					},
					mustConfiguration(t, graphqlDataSource.ConfigurationInput{
						Fetch: &graphqlDataSource.FetchConfiguration{
							URL:    "http://product.service",
							Header: make(http.Header),
						},
						Subscription: &graphqlDataSource.SubscriptionConfiguration{
							URL: "http://product.service",
						},
						SchemaConfiguration: mustSchemaConfig(
							t,
							&graphqlDataSource.FederationConfiguration{
								Enabled:    true,
								ServiceSDL: productSchema,
							},
							productUpstreamSchema,
						),
						CustomScalarTypeFields: []graphqlDataSource.SingleTypeField{},
					}),
				),
				mustGraphqlDataSourceConfiguration(t,
					"2",
					gqlFactory,
					&plan.DataSourceMetadata{
						RootNodes: []plan.TypeField{
							{
								TypeName:   "User",
								FieldNames: []string{"id", "reviews"},
							},
							{
								TypeName:   "Product",
								FieldNames: []string{"upc", "reviews"},
							},
						},
						ChildNodes: []plan.TypeField{
							{
								TypeName:   "Review",
								FieldNames: []string{"body", "author", "product"},
							},
						},
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
							Requires: []plan.FederationFieldConfiguration{},
							Provides: []plan.FederationFieldConfiguration{
								{
									TypeName:     "Review",
									FieldName:    "author",
									SelectionSet: "username",
								},
							},
						},
						Directives: plan.NewDirectiveConfigurations([]plan.DirectiveConfiguration{}),
					},
					mustConfiguration(t, graphqlDataSource.ConfigurationInput{
						Fetch: &graphqlDataSource.FetchConfiguration{
							URL:    "http://review.service",
							Header: make(http.Header),
						},
						Subscription: &graphqlDataSource.SubscriptionConfiguration{
							URL: "http://review.service",
						},
						SchemaConfiguration: mustSchemaConfig(
							t,
							&graphqlDataSource.FederationConfiguration{
								Enabled:    true,
								ServiceSDL: reviewSchema,
							},
							reviewUpstreamSchema,
						),
						CustomScalarTypeFields: []graphqlDataSource.SingleTypeField{},
					}),
				),
			})

			return conf
		})
	})
}

const (
	accountSchema = `extend type Query {
    me: User
}
type User @key(fields: "id"){
    id: ID!
    username: String!
}`
	accountUpstreamSchema = `schema {
  query: Query
}

directive @key(fields: openfed__FieldSet!, resolvable: Boolean = true) repeatable on INTERFACE | OBJECT

type Query {
  me: User
}

type User @key(fields: "id") {
  id: ID!
  username: String!
}

scalar openfed__FieldSet`

	productSchema = `extend type Query {
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

directive @key(fields: openfed__FieldSet!, resolvable: Boolean = true) repeatable on INTERFACE | OBJECT

type Product @key(fields: "upc") {
  name: String!
  price: Int!
  upc: String!
}

type Query {
  topProducts(first: Int = 5): [Product]
}

scalar openfed__FieldSet`

	reviewSchema = `type Review {
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

	reviewUpstreamSchema = `directive @external on FIELD_DEFINITION | OBJECT

directive @key(fields: openfed__FieldSet!, resolvable: Boolean = true) repeatable on INTERFACE | OBJECT

directive @provides(fields: openfed__FieldSet!) on FIELD_DEFINITION

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

	baseFederationSchema = `schema {
  query: Query
}

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
}`
)
