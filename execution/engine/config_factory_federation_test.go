package engine

import (
	"context"
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
	engineCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	runWithoutError := func(
		t *testing.T,
		httpClient *http.Client,
		streamingClient *http.Client,
		dataSourceConfigs []DataSourceConfiguration,
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
			engineCtx,
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
		runWithoutError(t, httpClient, streamingClient, []DataSourceConfiguration{
			{
				"users",
				mustConfiguration(t, graphqlDataSource.ConfigurationInput{
					Fetch: &graphqlDataSource.FetchConfiguration{
						URL: "http://user.service",
					},
					SchemaConfiguration: mustSchemaConfig(
						t,
						&graphqlDataSource.FederationConfiguration{
							Enabled:    true,
							ServiceSDL: accountSchema,
						},
						accountSchema,
					),
				}),
			},
			{
				"products",
				mustConfiguration(t, graphqlDataSource.ConfigurationInput{
					Fetch: &graphqlDataSource.FetchConfiguration{
						URL: "http://product.service",
					},
					SchemaConfiguration: mustSchemaConfig(
						t,
						&graphqlDataSource.FederationConfiguration{
							Enabled:    true,
							ServiceSDL: productSchema,
						},
						productSchema,
					),
				}),
			},
			{
				"reviews",
				mustConfiguration(t, graphqlDataSource.ConfigurationInput{
					Fetch: &graphqlDataSource.FetchConfiguration{
						URL: "http://review.service",
					},
					SchemaConfiguration: mustSchemaConfig(
						t,
						&graphqlDataSource.FederationConfiguration{
							Enabled:    true,
							ServiceSDL: reviewSchema,
						},
						reviewSchema,
					),
					Subscription: &graphqlDataSource.SubscriptionConfiguration{
						UseSSE: true,
					},
				}),
			},
		}, baseFederationSchema, func(t *testing.T, baseSchema string) EngineV2Configuration {
			schema, err := graphql.NewSchemaFromString(baseSchema)
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

			gqlFactory, err := graphqlDataSource.NewFactory(engineCtx, httpClient, mockSubscriptionClient)
			require.NoError(t, err)

			conf.SetDataSources([]plan.DataSource{
				mustGraphqlDataSourceConfiguration(t,
					"users",
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
						ChildNodes: []plan.TypeField{
							{
								TypeName:   "User",
								FieldNames: []string{"id", "username"},
							},
						},
					},
					mustConfiguration(t, graphqlDataSource.ConfigurationInput{
						Fetch: &graphqlDataSource.FetchConfiguration{
							URL: "http://user.service",
						},
						SchemaConfiguration: mustSchemaConfig(
							t,
							&graphqlDataSource.FederationConfiguration{
								Enabled:    true,
								ServiceSDL: accountSchema,
							},
							accountSchema,
						),
					}),
				),
				mustGraphqlDataSourceConfiguration(t,
					"products",
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
						ChildNodes: []plan.TypeField{
							{
								TypeName:   "Product",
								FieldNames: []string{"upc", "name", "price"},
							},
						},
					},
					mustConfiguration(t, graphqlDataSource.ConfigurationInput{
						Fetch: &graphqlDataSource.FetchConfiguration{
							URL: "http://product.service",
						},
						SchemaConfiguration: mustSchemaConfig(
							t,
							&graphqlDataSource.FederationConfiguration{
								Enabled:    true,
								ServiceSDL: productSchema,
							},
							productSchema,
						),
					}),
				),
				mustGraphqlDataSourceConfiguration(t,
					"reviews",
					gqlFactory,
					&plan.DataSourceMetadata{
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
					},
					mustConfiguration(t, graphqlDataSource.ConfigurationInput{
						Fetch: &graphqlDataSource.FetchConfiguration{
							URL: "http://review.service",
						},
						Subscription: &graphqlDataSource.SubscriptionConfiguration{
							UseSSE: true,
						},
						SchemaConfiguration: mustSchemaConfig(
							t,
							&graphqlDataSource.FederationConfiguration{
								Enabled:    true,
								ServiceSDL: reviewSchema,
							},
							reviewSchema,
						),
					}),
				),
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
