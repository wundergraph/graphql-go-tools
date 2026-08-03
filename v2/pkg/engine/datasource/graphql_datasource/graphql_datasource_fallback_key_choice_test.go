package graphql_datasource

import (
	"testing"

	. "github.com/wundergraph/graphql-go-tools/v2/pkg/engine/datasourcetesting"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

// TestFallbackKeyChoice covers the choice between several fallback
// (subset -> compound key) jumps into the same datasource.
//
// The pricing subgraph declares two compound keys, the more expensive one first:
//
//	type Product @key(fields: "id upc sku") @key(fields: "id upc")
//
// The entry subgraph owns only the key "id", so both compound keys are reachable
// only via a fallback jump gathering the missing members from the info subgraph.
// The planner must choose the cheapest fallback jump - the key "id upc" with a
// single missing member (upc) - and not the first recorded one ("id upc sku",
// which would gather two members).
func TestFallbackKeyChoice(t *testing.T) {
	definition := `
		type Product {
			id: ID!
			upc: ID!
			sku: ID!
			price: Float
		}

		type Query {
			product: Product
		}
	`

	entrySDL := `
		type Query {
			product: Product
		}

		type Product @key(fields: "id") {
			id: ID!
		}
	`

	entrySubgraph := mustDataSourceConfiguration(
		t,
		"entry",
		&plan.DataSourceMetadata{
			RootNodes: []plan.TypeField{
				{TypeName: "Query", FieldNames: []string{"product"}},
				{TypeName: "Product", FieldNames: []string{"id"}},
			},
			FederationMetaData: plan.FederationMetaData{
				Keys: plan.FederationFieldConfigurations{
					{TypeName: "Product", SelectionSet: "id"},
				},
			},
		},
		mustCustomConfiguration(t,
			ConfigurationInput{
				Fetch: &FetchConfiguration{URL: "http://entry"},
				SchemaConfiguration: mustSchema(t,
					&FederationConfiguration{Enabled: true, ServiceSDL: entrySDL},
					entrySDL,
				),
			},
		),
	)

	infoSDL := `
		type Product @key(fields: "id") {
			id: ID!
			upc: ID!
			sku: ID!
		}
	`

	infoSubgraph := mustDataSourceConfiguration(
		t,
		"info",
		&plan.DataSourceMetadata{
			RootNodes: []plan.TypeField{
				{TypeName: "Product", FieldNames: []string{"id", "upc", "sku"}},
			},
			FederationMetaData: plan.FederationMetaData{
				Keys: plan.FederationFieldConfigurations{
					{TypeName: "Product", SelectionSet: "id"},
				},
			},
		},
		mustCustomConfiguration(t,
			ConfigurationInput{
				Fetch: &FetchConfiguration{URL: "http://info"},
				SchemaConfiguration: mustSchema(t,
					&FederationConfiguration{Enabled: true, ServiceSDL: infoSDL},
					infoSDL,
				),
			},
		),
	)

	pricingSDL := `
		type Product @key(fields: "id upc sku") @key(fields: "id upc") {
			id: ID!
			upc: ID!
			sku: ID!
			price: Float
		}
	`

	pricingSubgraph := mustDataSourceConfiguration(
		t,
		"pricing",
		&plan.DataSourceMetadata{
			RootNodes: []plan.TypeField{
				{TypeName: "Product", FieldNames: []string{"id", "upc", "sku", "price"}},
			},
			FederationMetaData: plan.FederationMetaData{
				Keys: plan.FederationFieldConfigurations{
					{TypeName: "Product", SelectionSet: "id upc sku"},
					{TypeName: "Product", SelectionSet: "id upc"},
				},
			},
		},
		mustCustomConfiguration(t,
			ConfigurationInput{
				Fetch: &FetchConfiguration{URL: "http://pricing"},
				SchemaConfiguration: mustSchema(t,
					&FederationConfiguration{Enabled: true, ServiceSDL: pricingSDL},
					pricingSDL,
				),
			},
		),
	)

	planConfiguration := plan.Configuration{
		DataSources: []plan.DataSource{
			entrySubgraph,
			infoSubgraph,
			pricingSubgraph,
		},
		DisableResolveFieldPositions: true,
	}

	t.Run("gathers only the missing member of the cheapest compound key", RunTest(
		definition,
		`query { product { price } }`,
		"",
		&plan.SynchronousResponsePlan{
			Response: &resolve.GraphQLResponse{
				Fetches: resolve.Sequence(
					resolve.Single(
						&resolve.SingleFetch{
							FetchDependencies: resolve.FetchDependencies{
								FetchID: 0,
							},
							FetchConfiguration: resolve.FetchConfiguration{
								Input:          `{"method":"POST","url":"http://entry","body":{"query":"{product {__typename id}}"}}`,
								DataSource:     &Source{},
								PostProcessing: DefaultPostProcessingConfiguration,
							},
							DataSourceIdentifier: []byte("graphql_datasource.Source"),
						}),
					resolve.SingleWithPath(
						&resolve.SingleFetch{
							FetchDependencies: resolve.FetchDependencies{
								FetchID:           1,
								DependsOnFetchIDs: []int{0},
							},
							FetchConfiguration: resolve.FetchConfiguration{
								Input:                                 `{"method":"POST","url":"http://info","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Product {__typename upc}}}","variables":{"representations":[$$0$$]}}}`,
								DataSource:                            &Source{},
								SetTemplateOutputToNullOnVariableNull: true,
								RequiresEntityFetch:                   true,
								PostProcessing:                        SingleEntityPostProcessingConfiguration,
								Variables: []resolve.Variable{
									&resolve.ResolvableObjectVariable{
										Renderer: resolve.NewGraphQLVariableResolveRenderer(&resolve.Object{
											Nullable: true,
											Fields: []*resolve.Field{
												{
													Name: []byte("__typename"),
													Value: &resolve.String{
														Path: []string{"__typename"},
													},
													OnTypeNames: [][]byte{[]byte("Product")},
												},
												{
													Name: []byte("id"),
													Value: &resolve.Scalar{
														Path: []string{"id"},
													},
													OnTypeNames: [][]byte{[]byte("Product")},
												},
											},
										}),
									},
								},
							},
							DataSourceIdentifier: []byte("graphql_datasource.Source"),
						}, "product", resolve.ObjectPath("product")),
					resolve.SingleWithPath(
						&resolve.SingleFetch{
							FetchDependencies: resolve.FetchDependencies{
								FetchID:           2,
								DependsOnFetchIDs: []int{0, 1},
							},
							FetchConfiguration: resolve.FetchConfiguration{
								Input:                                 `{"method":"POST","url":"http://pricing","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Product {__typename price}}}","variables":{"representations":[$$0$$]}}}`,
								DataSource:                            &Source{},
								SetTemplateOutputToNullOnVariableNull: true,
								RequiresEntityFetch:                   true,
								PostProcessing:                        SingleEntityPostProcessingConfiguration,
								Variables: []resolve.Variable{
									&resolve.ResolvableObjectVariable{
										Renderer: resolve.NewGraphQLVariableResolveRenderer(&resolve.Object{
											Nullable: true,
											Fields: []*resolve.Field{
												{
													Name: []byte("__typename"),
													Value: &resolve.String{
														Path: []string{"__typename"},
													},
													OnTypeNames: [][]byte{[]byte("Product")},
												},
												{
													Name: []byte("id"),
													Value: &resolve.Scalar{
														Path: []string{"id"},
													},
													OnTypeNames: [][]byte{[]byte("Product")},
												},
												{
													Name: []byte("upc"),
													Value: &resolve.Scalar{
														Path: []string{"upc"},
													},
													OnTypeNames: [][]byte{[]byte("Product")},
												},
											},
										}),
									},
								},
							},
							DataSourceIdentifier: []byte("graphql_datasource.Source"),
						}, "product", resolve.ObjectPath("product")),
				),
				Data: &resolve.Object{
					Fields: []*resolve.Field{
						{
							Name: []byte("product"),
							Value: &resolve.Object{
								Path:          []string{"product"},
								Nullable:      true,
								PossibleTypes: map[string]struct{}{"Product": {}},
								TypeName:      "Product",
								Fields: []*resolve.Field{
									{
										Name: []byte("price"),
										Value: &resolve.Float{
											Path:     []string{"price"},
											Nullable: true,
										},
									},
								},
							},
						},
					},
				},
			},
		},
		planConfiguration,
		WithDefaultPostProcessor(),
	))
}
