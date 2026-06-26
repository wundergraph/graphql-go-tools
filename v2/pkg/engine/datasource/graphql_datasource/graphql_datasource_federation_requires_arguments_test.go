package graphql_datasource

import (
	"testing"

	. "github.com/wundergraph/graphql-go-tools/v2/pkg/engine/datasourcetesting"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

func TestGraphQLDataSourceFederation_RequiresSameFieldWithDifferentArguments(t *testing.T) {
	definition := `
		type Query {
			products: [Product]
		}

		type Product {
			upc: String!
			weight: Int
			price(currency: String!): Int
			estimateA: Int
			estimateB: Int
		}
	`

	catalogSDL := `
		type Query {
			products: [Product]
		}

		type Product @key(fields: "upc") {
			upc: String!
			weight: Int
			price(currency: String!): Int
		}
	`

	inventorySDL := `
		type Product @key(fields: "upc") {
			upc: String!
			weight: Int @external
			price(currency: String!): Int @external
			estimateA: Int @requires(fields: "price(currency: \"USD\") weight")
			estimateB: Int @requires(fields: "price(currency: \"EUR\") weight")
		}
	`

	catalog := mustDataSourceConfiguration(
		t,
		"catalog",
		&plan.DataSourceMetadata{
			RootNodes: []plan.TypeField{
				{
					TypeName:   "Query",
					FieldNames: []string{"products"},
				},
				{
					TypeName:   "Product",
					FieldNames: []string{"upc", "weight", "price"},
				},
			},
			FederationMetaData: plan.FederationMetaData{
				Keys: plan.FederationFieldConfigurations{
					{
						TypeName:     "Product",
						SelectionSet: "upc",
					},
				},
			},
		},
		mustCustomConfiguration(t, ConfigurationInput{
			Fetch: &FetchConfiguration{
				URL: "http://catalog.service",
			},
			SchemaConfiguration: mustSchema(t,
				&FederationConfiguration{
					Enabled:    true,
					ServiceSDL: catalogSDL,
				},
				catalogSDL,
			),
		}),
	)

	inventory := mustDataSourceConfiguration(
		t,
		"inventory",
		&plan.DataSourceMetadata{
			RootNodes: []plan.TypeField{
				{
					TypeName:   "Product",
					FieldNames: []string{"upc", "estimateA", "estimateB"},
				},
			},
			FederationMetaData: plan.FederationMetaData{
				Keys: plan.FederationFieldConfigurations{
					{
						TypeName:     "Product",
						SelectionSet: "upc",
					},
				},
				Requires: plan.FederationFieldConfigurations{
					{
						TypeName:     "Product",
						FieldName:    "estimateA",
						SelectionSet: `price(currency: "USD") weight`,
					},
					{
						TypeName:     "Product",
						FieldName:    "estimateB",
						SelectionSet: `price(currency: "EUR") weight`,
					},
				},
			},
		},
		mustCustomConfiguration(t, ConfigurationInput{
			Fetch: &FetchConfiguration{
				URL: "http://inventory.service",
			},
			SchemaConfiguration: mustSchema(t,
				&FederationConfiguration{
					Enabled:    true,
					ServiceSDL: inventorySDL,
				},
				inventorySDL,
			),
		}),
	)

	RunWithPermutations(
		t,
		definition,
		`
		query Products {
			products {
				upc
				estimateA
				estimateB
			}
		}`,
		"Products",
		&plan.SynchronousResponsePlan{
			Response: &resolve.GraphQLResponse{
				Fetches: resolve.Sequence(
					resolve.Single(&resolve.SingleFetch{
						FetchConfiguration: resolve.FetchConfiguration{
							Input:          `{"method":"POST","url":"http://catalog.service","body":{"query":"query($a: String!, $b: String!){products {upc price(currency: $a) weight __internal_price: price(currency: $b) __typename}}","variables":{"a":"USD","b":"EUR"}}}`,
							DataSource:     &Source{},
							PostProcessing: DefaultPostProcessingConfiguration,
						},
						DataSourceIdentifier: []byte("graphql_datasource.Source"),
					}),
					resolve.SingleWithPath(&resolve.SingleFetch{
						FetchDependencies: resolve.FetchDependencies{
							FetchID:           1,
							DependsOnFetchIDs: []int{0},
						},
						FetchConfiguration: resolve.FetchConfiguration{
							RequiresEntityBatchFetch:              true,
							Input:                                 `{"method":"POST","url":"http://inventory.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Product {__typename estimateA}}}","variables":{"representations":[$$0$$]}}}`,
							DataSource:                            &Source{},
							PostProcessing:                        EntitiesPostProcessingConfiguration,
							SetTemplateOutputToNullOnVariableNull: true,
							Variables: resolve.NewVariables(
								resolve.NewResolvableObjectVariable(&resolve.Object{
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
											Name: []byte("price"),
											Value: &resolve.Integer{
												Path:     []string{"price"},
												Nullable: true,
											},
											OnTypeNames: [][]byte{[]byte("Product")},
										},
										{
											Name: []byte("weight"),
											Value: &resolve.Integer{
												Path:     []string{"weight"},
												Nullable: true,
											},
											OnTypeNames: [][]byte{[]byte("Product")},
										},
										{
											Name: []byte("upc"),
											Value: &resolve.String{
												Path: []string{"upc"},
											},
											OnTypeNames: [][]byte{[]byte("Product")},
										},
									},
								}),
							),
						},
						DataSourceIdentifier: []byte("graphql_datasource.Source"),
					}, "products", resolve.ArrayPath("products")),
					resolve.SingleWithPath(&resolve.SingleFetch{
						FetchDependencies: resolve.FetchDependencies{
							FetchID:           2,
							DependsOnFetchIDs: []int{0},
						},
						FetchConfiguration: resolve.FetchConfiguration{
							RequiresEntityBatchFetch:              true,
							Input:                                 `{"method":"POST","url":"http://inventory.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Product {__typename estimateB}}}","variables":{"representations":[$$0$$]}}}`,
							DataSource:                            &Source{},
							PostProcessing:                        EntitiesPostProcessingConfiguration,
							SetTemplateOutputToNullOnVariableNull: true,
							Variables: resolve.NewVariables(
								resolve.NewResolvableObjectVariable(&resolve.Object{
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
											Name: []byte("price"),
											Value: &resolve.Integer{
												Path:     []string{"__internal_price"},
												Nullable: true,
											},
											OnTypeNames: [][]byte{[]byte("Product")},
										},
										{
											Name: []byte("weight"),
											Value: &resolve.Integer{
												Path:     []string{"weight"},
												Nullable: true,
											},
											OnTypeNames: [][]byte{[]byte("Product")},
										},
										{
											Name: []byte("upc"),
											Value: &resolve.String{
												Path: []string{"upc"},
											},
											OnTypeNames: [][]byte{[]byte("Product")},
										},
									},
								}),
							),
						},
						DataSourceIdentifier: []byte("graphql_datasource.Source"),
					}, "products", resolve.ArrayPath("products")),
				),
				Data: &resolve.Object{
					Fields: []*resolve.Field{
						{
							Name: []byte("products"),
							Value: &resolve.Array{
								Path:     []string{"products"},
								Nullable: true,
								Item: &resolve.Object{
									Nullable: true,
									Fields: []*resolve.Field{
										{
											Name: []byte("upc"),
											Value: &resolve.String{
												Path: []string{"upc"},
											},
										},
										{
											Name: []byte("estimateA"),
											Value: &resolve.Integer{
												Path:     []string{"estimateA"},
												Nullable: true,
											},
										},
										{
											Name: []byte("estimateB"),
											Value: &resolve.Integer{
												Path:     []string{"estimateB"},
												Nullable: true,
											},
										},
									},
									TypeName: "Product",
									PossibleTypes: map[string]struct{}{
										"Product": {},
									},
								},
							},
						},
					},
				},
			},
		},
		plan.Configuration{
			DataSources: []plan.DataSource{
				catalog,
				inventory,
			},
			DisableResolveFieldPositions: true,
		},
		WithDefaultPostProcessor(),
	)
}
