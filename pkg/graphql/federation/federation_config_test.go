package federation

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jensneuse/graphql-go-tools/pkg/astparser"
	"github.com/jensneuse/graphql-go-tools/pkg/astprinter"
	graphqlDataSource "github.com/jensneuse/graphql-go-tools/pkg/engine/datasource/graphql_datasource"
	"github.com/jensneuse/graphql-go-tools/pkg/engine/datasource/httpclient"
	"github.com/jensneuse/graphql-go-tools/pkg/engine/plan"
	"github.com/jensneuse/graphql-go-tools/pkg/engine/resolve"
	"github.com/jensneuse/graphql-go-tools/pkg/graphql"
)

func TestEngineConfigV2Factory_EngineV2Configuration(t *testing.T) {
	_ = func(t *testing.T, httpClient *http.Client,  batchFactory resolve.DataSourceBatchFactory, dataSourceConfigs []graphqlDataSource.Configuration, expectedErr error) {
		engineConfigV2Factory := NewEngineConfigV2Factory(httpClient, batchFactory, dataSourceConfigs...)
		_, err := engineConfigV2Factory.EngineV2Configuration()
		assert.Error(t, err)
		assert.Equal(t, expectedErr, err)
	}

	runWithoutError := func(
		t *testing.T,
		httpClient *http.Client,
		batchFactory resolve.DataSourceBatchFactory,
		dataSourceConfigs []graphqlDataSource.Configuration,
		baseSchema string,
		expectedConfigFactory func(t *testing.T, baseSchema string) graphql.EngineV2Configuration,
	) {
		doc, report := astparser.ParseGraphqlDocumentString(baseSchema)
		if report.HasErrors() {
			require.Fail(t, report.Error())
		}
		printedBaseSchema, err := astprinter.PrintString(&doc, nil)
		require.NoError(t, err)

		engineConfigV2Factory := NewEngineConfigV2Factory(httpClient, batchFactory, dataSourceConfigs...)
		config, err := engineConfigV2Factory.EngineV2Configuration()
		assert.NoError(t, err)
		assert.Equal(t, expectedConfigFactory(t, printedBaseSchema), config)
	}

	httpClient := &http.Client{}
	batchFactory := graphqlDataSource.NewBatchFactory()

	t.Run("should create engine V2 configuration", func(t *testing.T) {
		runWithoutError(t, httpClient, batchFactory, []graphqlDataSource.Configuration{
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
			},
		}, baseFederationSchema, func(t *testing.T, baseSchema string) graphql.EngineV2Configuration {
			schema, err := graphql.NewSchemaFromString(baseSchema)
			require.NoError(t, err)

			conf := graphql.NewEngineV2Configuration(schema)
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
						BatchFactory: batchFactory,
						Client: httpclient.NewNetHttpClient(httpClient),
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
						BatchFactory: batchFactory,
						Client: httpclient.NewNetHttpClient(httpClient),
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
							TypeName:   "User",
							FieldNames: []string{"id", "username", "reviews"},
						},
						{
							TypeName:   "Product",
							FieldNames: []string{"upc", "reviews"},
						},
					},
					Factory: &graphqlDataSource.Factory{
						BatchFactory: batchFactory,
						Client: httpclient.NewNetHttpClient(httpClient),
					},
					Custom: graphqlDataSource.ConfigJson(graphqlDataSource.Configuration{
						Fetch: graphqlDataSource.FetchConfiguration{
							URL: "http://review.service",
						},
						Federation: graphqlDataSource.FederationConfiguration{
							Enabled:    true,
							ServiceSDL: reviewSchema,
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
