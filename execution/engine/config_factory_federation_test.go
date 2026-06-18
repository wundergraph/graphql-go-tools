package engine

import (
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protojson"

	nodev1 "github.com/wundergraph/cosmo/router/gen/proto/wg/cosmo/node/v1"

	"github.com/wundergraph/graphql-go-tools/execution/graphql"
	graphqlDataSource "github.com/wundergraph/graphql-go-tools/v2/pkg/engine/datasource/graphql_datasource"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan"
)

func TestEngineConfigFactory_EngineConfiguration(t *testing.T) {
	engineCtx := t.Context()

	runWithoutErrorUsingRouteConfig := func(
		t *testing.T,
		httpClient *http.Client,
		streamingClient *http.Client,
		baseSchema string,
		expectedConfigFactory func(t *testing.T, baseSchema string) Configuration,
	) {
		engineConfigFactory := NewFederationEngineConfigFactory(
			engineCtx,
			WithFederationHttpClient(httpClient),
			WithFederationStreamingClient(streamingClient),
			WithFederationSubscriptionClientFactory(&MockSubscriptionClientFactory{}),
		)

		data, err := os.ReadFile("testdata/config_factory_federation/config.json")
		require.NoError(t, err)

		// Build the engine configuration using the router config
		var rc1 nodev1.RouterConfig
		assert.NoError(t, protojson.Unmarshal(data, &rc1))
		config, err := engineConfigFactory.BuildEngineConfiguration(&rc1)
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

func TestEngineConfigFactory_SubgraphCachingConfigs(t *testing.T) {
	engineCtx := t.Context()
	data, err := os.ReadFile("testdata/config_factory_federation/config.json")
	require.NoError(t, err)

	var routerConfig nodev1.RouterConfig
	require.NoError(t, protojson.Unmarshal(data, &routerConfig))

	config, err := NewFederationEngineConfigFactory(
		engineCtx,
		WithFederationHttpClient(&http.Client{}),
		WithFederationStreamingClient(&http.Client{}),
		WithFederationSubscriptionClientFactory(&MockSubscriptionClientFactory{}),
		WithSubgraphEntityCachingConfigs(SubgraphCachingConfigs{
			{
				SubgraphName: "products",
				EntityCaching: plan.EntityCacheConfigurations{
					{
						TypeName:                    "Product",
						CacheName:                   "default",
						TTL:                         30 * time.Second,
						IncludeSubgraphHeaderPrefix: true,
						EnablePartialCacheLoad:      true,
						HashAnalyticsKeys:           true,
						ShadowMode:                  true,
						NegativeCacheTTL:            5 * time.Second,
					},
				},
				RootFieldCaching: plan.RootFieldCacheConfigurations{
					{
						TypeName:                    "Query",
						FieldName:                   "topProducts",
						CacheName:                   "default",
						TTL:                         45 * time.Second,
						IncludeSubgraphHeaderPrefix: true,
						EntityKeyMappings: []plan.EntityKeyMapping{
							{
								EntityTypeName: "Product",
								FieldMappings: []plan.FieldMapping{
									{
										EntityKeyField:      "upc",
										ArgumentPath:        []string{"upc"},
										ArgumentIsEntityKey: true,
									},
								},
							},
						},
						ShadowMode:       true,
						PartialBatchLoad: true,
					},
				},
				MutationFieldCaching: plan.MutationFieldCacheConfigurations{
					{
						FieldName:                     "setPrice",
						EnableEntityL2CachePopulation: true,
						TTL:                           15 * time.Second,
					},
				},
				MutationCacheInvalidation: plan.MutationCacheInvalidationConfigurations{
					{
						FieldName:      "setPrice",
						EntityTypeName: "Product",
					},
				},
				SubscriptionEntityPopulation: plan.SubscriptionEntityPopulationConfigurations{
					{
						TypeName:                    "Product",
						FieldName:                   "updatedPrice",
						CacheName:                   "default",
						TTL:                         20 * time.Second,
						IncludeSubgraphHeaderPrefix: true,
						EnableInvalidationOnKeyOnly: true,
					},
				},
				RequestScopedFields: []plan.RequestScopedField{
					{
						TypeName:  "Product",
						FieldName: "price",
						L1Key:     "products.price",
					},
				},
			},
		}),
	).BuildEngineConfiguration(&routerConfig)
	require.NoError(t, err)

	require.Len(t, config.plannerConfig.DataSources, 3)
	federationConfig := config.plannerConfig.DataSources[1].FederationConfiguration()
	assert.Equal(t, plan.EntityCacheConfigurations{
		{
			TypeName:                    "Product",
			CacheName:                   "default",
			TTL:                         30 * time.Second,
			IncludeSubgraphHeaderPrefix: true,
			EnablePartialCacheLoad:      true,
			HashAnalyticsKeys:           true,
			ShadowMode:                  true,
			NegativeCacheTTL:            5 * time.Second,
		},
	}, federationConfig.EntityCacheConfig)
	assert.Equal(t, plan.RootFieldCacheConfigurations{
		{
			TypeName:                    "Query",
			FieldName:                   "topProducts",
			CacheName:                   "default",
			TTL:                         45 * time.Second,
			IncludeSubgraphHeaderPrefix: true,
			EntityKeyMappings: []plan.EntityKeyMapping{
				{
					EntityTypeName: "Product",
					FieldMappings: []plan.FieldMapping{
						{
							EntityKeyField:      "upc",
							ArgumentPath:        []string{"upc"},
							ArgumentIsEntityKey: true,
						},
					},
				},
			},
			ShadowMode:       true,
			PartialBatchLoad: true,
		},
	}, federationConfig.RootFieldCacheConfig)
	assert.Equal(t, plan.MutationFieldCacheConfigurations{
		{
			FieldName:                     "setPrice",
			EnableEntityL2CachePopulation: true,
			TTL:                           15 * time.Second,
		},
	}, federationConfig.MutationFieldCacheConfig)
	assert.Equal(t, plan.MutationCacheInvalidationConfigurations{
		{
			FieldName:      "setPrice",
			EntityTypeName: "Product",
		},
	}, federationConfig.MutationCacheInvalidationConfig)
	assert.Equal(t, plan.SubscriptionEntityPopulationConfigurations{
		{
			TypeName:                    "Product",
			FieldName:                   "updatedPrice",
			CacheName:                   "default",
			TTL:                         20 * time.Second,
			IncludeSubgraphHeaderPrefix: true,
			EnableInvalidationOnKeyOnly: true,
		},
	}, federationConfig.SubscriptionEntityPopulationConfig)
	assert.Equal(t, []plan.RequestScopedField{
		{
			TypeName:  "Product",
			FieldName: "price",
			L1Key:     "products.price",
		},
	}, federationConfig.RequestScopedFields)
	assert.Equal(t, plan.EntityCacheConfigurations(nil), config.plannerConfig.DataSources[0].FederationConfiguration().EntityCacheConfig)
	assert.Equal(t, plan.RootFieldCacheConfigurations(nil), config.plannerConfig.DataSources[2].FederationConfiguration().RootFieldCacheConfig)
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
