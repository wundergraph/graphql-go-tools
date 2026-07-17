package graphql_datasource

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/astnormalization"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/asttransform"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astvalidation"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/internal/unsafeparser"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/operationreport"
)

func TestGraphQLDataSourceMultiHopCompoundKeyEntityRoute(t *testing.T) {
	definition := unsafeparser.ParseGraphqlDocumentString(multiHopCompoundKeyDefinition)
	require.NoError(t, asttransform.MergeDefinitionWithBaseSchema(&definition))

	operation := unsafeparser.ParseGraphqlDocumentString(`query {
		topProducts {
			products {
				id
				pid
				price {
					price
				}
				category {
					mainProduct {
						id
					}
					id
					tag
				}
			}
			first { id }
			selected { id }
		}
	}`)

	report := &operationreport.Report{}
	astnormalization.NewNormalizer(true, true).NormalizeOperation(&operation, &definition, report)
	require.False(t, report.HasErrors(), report.Error())

	astvalidation.DefaultOperationValidator().Validate(&operation, &definition, report)
	require.False(t, report.HasErrors(), report.Error())

	planner, err := plan.NewPlanner(plan.Configuration{
		DataSources:                     multiHopCompoundKeyGraphQLDataSources(t),
		DisableIncludeInfo:              true,
		DisableIncludeFieldDependencies: true,
	})
	require.NoError(t, err)

	preparedPlan := planner.Plan(&operation, &definition, "", report)
	require.False(t, report.HasErrors(), report.Error())

	responsePlan, ok := preparedPlan.(*plan.SynchronousResponsePlan)
	require.True(t, ok)
	require.NotNil(t, responsePlan.Response)
	require.Len(t, responsePlan.Response.RawFetches, 4)

	require.Equal(t, "", responsePlan.Response.RawFetches[0].ResponsePath)
	require.Empty(t, responsePlan.Response.RawFetches[0].Fetch.Dependencies().DependsOnFetchIDs)

	require.Equal(t, "topProducts.products", responsePlan.Response.RawFetches[1].ResponsePath)
	require.Equal(t, []int{0}, responsePlan.Response.RawFetches[1].Fetch.Dependencies().DependsOnFetchIDs)

	require.Equal(t, "topProducts", responsePlan.Response.RawFetches[2].ResponsePath)
	require.Equal(t, []int{0, 1}, responsePlan.Response.RawFetches[2].Fetch.Dependencies().DependsOnFetchIDs)

	require.Equal(t, "topProducts.products", responsePlan.Response.RawFetches[3].ResponsePath)
	require.Equal(t, []int{0, 1}, responsePlan.Response.RawFetches[3].Fetch.Dependencies().DependsOnFetchIDs)
}

func multiHopCompoundKeyGraphQLDataSources(t *testing.T) []plan.DataSource {
	t.Helper()

	return []plan.DataSource{
		mustMultiHopDataSourceConfiguration(t, "catalog", []plan.TypeField{
			{TypeName: "Query", FieldNames: []string{"topProducts"}},
			{TypeName: "ProductList", FieldNames: []string{"products"}},
			{TypeName: "Product", FieldNames: []string{"id", "category"}},
			{TypeName: "Category", FieldNames: []string{"mainProduct", "id", "tag"}},
		}, []plan.FederationFieldConfiguration{
			{TypeName: "ProductList", SelectionSet: "products { id }"},
			{TypeName: "Product", SelectionSet: "id"},
			{TypeName: "Category", SelectionSet: "id"},
		}, multiHopCatalogSubgraphSchema),
		mustMultiHopDataSourceConfiguration(t, "link", []plan.TypeField{
			{TypeName: "Product", FieldNames: []string{"id", "pid"}},
		}, []plan.FederationFieldConfiguration{
			{TypeName: "Product", SelectionSet: "id"},
			{TypeName: "Product", SelectionSet: "id pid"},
		}, multiHopLinkSubgraphSchema),
		mustMultiHopDataSourceConfiguration(t, "collection", []plan.TypeField{
			{TypeName: "ProductList", FieldNames: []string{"products", "first", "selected"}},
			{TypeName: "Product", FieldNames: []string{"id", "pid"}},
		}, []plan.FederationFieldConfiguration{
			{TypeName: "ProductList", SelectionSet: "products { id pid }"},
			{TypeName: "ProductList", SelectionSet: "products { id }", DisableEntityResolver: true},
			{TypeName: "Product", SelectionSet: "id pid"},
			{TypeName: "Product", SelectionSet: "id", DisableEntityResolver: true},
		}, multiHopCollectionSubgraphSchema),
		mustMultiHopDataSourceConfiguration(t, "pricing", []plan.TypeField{
			{TypeName: "ProductList", FieldNames: []string{"products", "first", "selected"}},
			{TypeName: "Product", FieldNames: []string{"id", "price", "pid", "category"}},
			{TypeName: "Category", FieldNames: []string{"id", "tag"}},
			{TypeName: "Price", FieldNames: []string{"price"}},
		}, []plan.FederationFieldConfiguration{
			{TypeName: "ProductList", SelectionSet: "products { category { id tag } id pid } selected { id }"},
			{TypeName: "ProductList", SelectionSet: "products { id }", DisableEntityResolver: true},
			{TypeName: "ProductList", SelectionSet: "products { id pid }", DisableEntityResolver: true},
			{TypeName: "Product", SelectionSet: "category { id tag } id pid"},
			{TypeName: "Product", SelectionSet: "id", DisableEntityResolver: true},
			{TypeName: "Product", SelectionSet: "id pid", DisableEntityResolver: true},
			{TypeName: "Category", SelectionSet: "id tag"},
			{TypeName: "Category", SelectionSet: "id", DisableEntityResolver: true},
		}, multiHopPricingSubgraphSchema),
	}
}

func mustMultiHopDataSourceConfiguration(t *testing.T, id string, rootNodes []plan.TypeField, keys []plan.FederationFieldConfiguration, schema string) plan.DataSource {
	t.Helper()

	ds, err := plan.NewDataSourceConfiguration[Configuration](id, &Factory[Configuration]{}, &plan.DataSourceMetadata{
		RootNodes: rootNodes,
		FederationMetaData: plan.FederationMetaData{
			Keys: keys,
		},
	}, mustCustomConfiguration(t, ConfigurationInput{
		Fetch: &FetchConfiguration{
			URL: "https://example.com/" + id,
		},
		SchemaConfiguration: mustSchema(t, &FederationConfiguration{
			Enabled:    true,
			ServiceSDL: schema,
		}, schema),
	}))
	require.NoError(t, err)
	return ds
}

const multiHopCompoundKeyDefinition = `
type Query {
	topProducts: ProductList!
}

type ProductList {
	products: [Product!]!
	first: Product
	selected: Product
}

type Product {
	id: String!
	pid: String
	category: Category
	price: Price
}

type Category {
	mainProduct: Product!
	id: String!
	tag: String!
}

type Price {
	price: Float!
}
`

const multiHopCatalogSubgraphSchema = `
type Query {
	topProducts: ProductList!
}

type ProductList @key(fields: "products { id }") {
	products: [Product!]!
}

type Product @key(fields: "id") {
	id: String!
	category: Category
}

type Category @key(fields: "id") {
	mainProduct: Product!
	id: String!
	tag: String
}
`

const multiHopCollectionSubgraphSchema = `
type ProductList @key(fields: "products { id pid }") @key(fields: "products { id }", resolvable: false) {
	products: [Product!]!
	first: Product
	selected: Product
}

type Product @key(fields: "id pid") @key(fields: "id", resolvable: false) {
	id: String!
	pid: String
}
`

const multiHopLinkSubgraphSchema = `
type Product @key(fields: "id") @key(fields: "id pid") {
	id: String!
	pid: String!
}
`

const multiHopPricingSubgraphSchema = `
type ProductList @key(fields: "products { id pid category { id tag } } selected { id }") {
	products: [Product!]!
	first: Product
	selected: Product
}

type Product @key(fields: "id pid category { id tag }") {
	id: String!
	price: Price
	pid: String
	category: Category
}

type Category @key(fields: "id tag") {
	id: String!
	tag: String
}

type Price {
	price: Float!
}
`
