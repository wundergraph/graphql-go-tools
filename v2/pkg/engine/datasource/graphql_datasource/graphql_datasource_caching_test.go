package graphql_datasource

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wundergraph/astjson"
	"github.com/wundergraph/go-arena"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/astnormalization"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/asttransform"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astvalidation"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/internal/unsafeparser"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/operationreport"
)

func TestGraphQLDataSourceCachingRootFetchCacheKeyTemplate(t *testing.T) {
	cacheTTL := 3 * time.Minute
	definition := `
		scalar String
		schema { query: Query }
		type Query { product(upc: String): Product }
		type Product { upc: String! name: String! }
	`
	responsePlan := planCachingOperation(t,
		definition,
		`query Product($upc: String) { product(upc: $upc) { upc name } }`,
		"Product",
		plan.Configuration{
			DataSources: []plan.DataSource{
				mustDataSourceConfiguration(t, "products", &plan.DataSourceMetadata{
					RootNodes: []plan.TypeField{
						{TypeName: "Query", FieldNames: []string{"product"}},
						{TypeName: "Product", FieldNames: []string{"upc", "name"}},
					},
					FederationMetaData: plan.FederationMetaData{
						RootFieldCacheConfig: plan.RootFieldCacheConfigurations{
							{
								TypeName:  "Query",
								FieldName: "product",
								CacheName: "root-products",
								TTL:       cacheTTL,
								EntityKeyMappings: []plan.EntityKeyMapping{
									{
										EntityTypeName: "Product",
										FieldMappings: []plan.FieldMapping{
											{EntityKeyField: "upc", ArgumentPath: []string{"upc"}},
										},
									},
								},
							},
						},
					},
				}, mustCustomConfiguration(t, ConfigurationInput{
					Fetch:               &FetchConfiguration{URL: "http://product.service"},
					SchemaConfiguration: mustSchema(t, nil, definition),
				})),
			},
			DisableResolveFieldPositions:    true,
			DisableIncludeInfo:              true,
			DisableIncludeFieldDependencies: true,
		},
	)

	fetch := requireSingleFetch(t, responsePlan.Response.RawFetches[0].Fetch)
	require.NotNil(t, fetch.Cache)
	assert.Equal(t, &resolve.FetchCacheConfiguration{
		CacheName:     "root-products",
		EnableL2Cache: true,
		TTL:           cacheTTL,
		KeyTemplate: resolve.NewRootQueryCacheKeyTemplate([]resolve.RootField{
			{
				TypeName:    "Query",
				FieldName:   "product",
				ResponseKey: "product",
				Arguments: []resolve.RootFieldArgument{
					{Name: "upc", VariablePath: []string{"upc"}},
				},
			},
		}, []resolve.EntityKeyMappingConfig{
			{
				EntityTypeName: "Product",
				FieldMappings: []resolve.EntityFieldMappingConfig{
					{EntityKeyField: "upc", ArgumentPath: []string{"upc"}},
				},
			},
		}),
		ProvidesData: fetch.Cache.ProvidesData,
	}, fetch.Cache)
	require.NotNil(t, fetch.Cache.ProvidesData)

	keys := renderCacheKeys(t, fetch.Cache.KeyTemplate, nil, astjson.MustParseBytes([]byte(`{"upc":"top-1"}`)))
	assert.Equal(t, []string{`{"__typename":"Product","key":{"upc":"top-1"}}`}, keys[0].Keys)
}

func TestGraphQLDataSourceCachingEntityFetchCacheKeyTemplate(t *testing.T) {
	cacheTTL := 5 * time.Minute
	responsePlan := planCachingOperation(t,
		federationTestSchema,
		`query ProductReviews { topProducts(first: 1) { upc reviews { body } } }`,
		"ProductReviews",
		plan.Configuration{
			DataSources: []plan.DataSource{
				mustDataSourceConfiguration(t, "products", &plan.DataSourceMetadata{
					RootNodes: []plan.TypeField{
						{TypeName: "Query", FieldNames: []string{"topProducts"}},
						{TypeName: "Product", FieldNames: []string{"upc", "name", "price"}},
					},
					FederationMetaData: plan.FederationMetaData{
						Keys: plan.FederationFieldConfigurations{{TypeName: "Product", SelectionSet: "upc"}},
					},
				}, mustCustomConfiguration(t, ConfigurationInput{
					Fetch: &FetchConfiguration{URL: "http://product.service"},
					SchemaConfiguration: mustSchema(t, &FederationConfiguration{
						Enabled:    true,
						ServiceSDL: `extend type Query { topProducts(first: Int = 5): [Product] } type Product @key(fields: "upc") { upc: String! name: String! price: Int! }`,
					}, `type Query { topProducts(first: Int = 5): [Product] } type Product @key(fields: "upc") { upc: String! name: String! price: Int! }`),
				})),
				mustDataSourceConfiguration(t, "reviews", &plan.DataSourceMetadata{
					RootNodes: []plan.TypeField{
						{TypeName: "Product", FieldNames: []string{"upc", "reviews"}},
					},
					ChildNodes: []plan.TypeField{
						{TypeName: "Review", FieldNames: []string{"body"}},
					},
					FederationMetaData: plan.FederationMetaData{
						Keys:              plan.FederationFieldConfigurations{{TypeName: "Product", SelectionSet: "upc"}},
						EntityCacheConfig: plan.EntityCacheConfigurations{{TypeName: "Product", CacheName: "entities", TTL: cacheTTL, IncludeSubgraphHeaderPrefix: true}},
					},
				}, mustCustomConfiguration(t, ConfigurationInput{
					Fetch: &FetchConfiguration{URL: "http://review.service"},
					SchemaConfiguration: mustSchema(t, &FederationConfiguration{
						Enabled:    true,
						ServiceSDL: `type Review { body: String! } extend type Product @key(fields: "upc") { upc: String! reviews: [Review] }`,
					}, `type Review { body: String! } type Product @key(fields: "upc") { upc: String! reviews: [Review] }`),
				})),
			},
			DisableResolveFieldPositions:    true,
			DisableIncludeInfo:              true,
			DisableIncludeFieldDependencies: true,
		},
	)

	require.Len(t, responsePlan.Response.RawFetches, 2)
	fetch := requireSingleFetch(t, responsePlan.Response.RawFetches[1].Fetch)
	require.NotNil(t, fetch.Cache)
	assert.True(t, fetch.RequiresEntityBatchFetch)
	assert.Equal(t, "entities", fetch.Cache.CacheName)
	assert.True(t, fetch.Cache.EnableL2Cache)
	assert.True(t, fetch.Cache.IncludeSubgraphHeaderPrefix)
	assert.Equal(t, cacheTTL, fetch.Cache.TTL)
	assert.False(t, fetch.Cache.UseL1Cache)
	require.IsType(t, &resolve.EntityQueryCacheKeyTemplate{}, fetch.Cache.KeyTemplate)
	assert.Equal(t, []resolve.KeyField{{Name: "upc"}}, fetch.Cache.KeyTemplate.(*resolve.EntityQueryCacheKeyTemplate).KeyFields())
	require.NotNil(t, fetch.Cache.ProvidesData)

	item := astjson.MustParseBytes([]byte(`{"__typename":"Product","upc":"top-1","reviews":[{"body":"great"}]}`))
	keys := renderCacheKeys(t, fetch.Cache.KeyTemplate, []*astjson.Value{item}, nil)
	assert.Equal(t, []string{`{"__typename":"Product","key":{"upc":"top-1"}}`}, keys[0].Keys)
}

func planCachingOperation(t *testing.T, definition, operation, operationName string, config plan.Configuration) *plan.SynchronousResponsePlan {
	t.Helper()

	definitionDoc := unsafeparser.ParseGraphqlDocumentString(definition)
	operationDoc := unsafeparser.ParseGraphqlDocumentString(operation)
	require.NoError(t, asttransform.MergeDefinitionWithBaseSchema(&definitionDoc))

	norm := astnormalization.NewWithOpts(astnormalization.WithExtractVariables(), astnormalization.WithInlineFragmentSpreads(), astnormalization.WithRemoveFragmentDefinitions(), astnormalization.WithRemoveUnusedVariables())
	var report operationreport.Report
	norm.NormalizeOperation(&operationDoc, &definitionDoc, &report)
	require.False(t, report.HasErrors(), report.Error())

	validator := astvalidation.DefaultOperationValidator()
	validator.Validate(&operationDoc, &definitionDoc, &report)
	require.False(t, report.HasErrors(), report.Error())

	planner, err := plan.NewPlanner(config)
	require.NoError(t, err)
	actualPlan := planner.Plan(&operationDoc, &definitionDoc, operationName, &report)
	require.False(t, report.HasErrors(), report.Error())

	responsePlan, ok := actualPlan.(*plan.SynchronousResponsePlan)
	require.True(t, ok)
	return responsePlan
}

func requireSingleFetch(t *testing.T, fetch resolve.Fetch) *resolve.SingleFetch {
	t.Helper()

	singleFetch, ok := fetch.(*resolve.SingleFetch)
	require.True(t, ok)
	return singleFetch
}

func renderCacheKeys(t *testing.T, template resolve.CacheKeyTemplate, items []*astjson.Value, variables *astjson.Value) []*resolve.CacheKey {
	t.Helper()

	pool := arena.NewArenaPool()
	item := pool.Acquire(0)
	t.Cleanup(func() {
		pool.Release(item)
	})
	ctx := resolve.NewContext(context.Background())
	ctx.Variables = variables
	keys, err := template.RenderCacheKeys(item.Arena, ctx, items, "")
	require.NoError(t, err)
	return keys
}
