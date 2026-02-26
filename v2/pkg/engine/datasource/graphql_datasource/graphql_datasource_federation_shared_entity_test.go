package graphql_datasource

import (
	"testing"

	. "github.com/wundergraph/graphql-go-tools/v2/pkg/engine/datasourcetesting"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

// TestGraphQLDataSourceFederation_SharedEntityWithExternalKeyField reproduces a bug where
// query planning fails with a 500 error when a query references a shared federation entity
// where the @key field is @external in one subgraph but non-external in another.
//
// Error: "could not select the datasource to resolve CartBundleProduct.code on path
// query.cartById.$0CartResultSuccess.cart.items.bundleProducts.code"
//
// Root cause: When newer composition puts an @external key field only in ExternalFieldNames
// (not in both FieldNames and ExternalFieldNames), the key_fields_visitor marks the key as
// external, preventing the cart datasource from being a key source. This breaks the jump
// graph so no datasource can resolve the field.
func TestGraphQLDataSourceFederation_SharedEntityWithExternalKeyField(t *testing.T) {

	// Merged supergraph schema (what the router sees after composition)
	definition := `
		type Query {
			cartById(cartId: String!): CartResult
		}

		union CartResult = CartExceptions | CartResultSuccess

		type CartExceptions {
			exceptions: [String]!
		}

		type CartResultSuccess {
			cart: Cart!
		}

		type Cart {
			id: String!
			items: [CartItem]
		}

		type CartItem {
			id: String!
			product: CommerceProduct
			bundleProducts: [CartBundleProduct]
		}

		type CartBundleProduct {
			code: String
			name: String
			category: String
			entryGroupNumber: Int
		}

		type CommerceProduct {
			id: String!
			title: String
		}
	`

	// Cart subgraph SDL — code is @external (not the authority)
	cartSubgraphSDL := `
		type Query {
			cartById(cartId: String!): CartResult
		}

		union CartResult = CartExceptions | CartResultSuccess

		type CartExceptions {
			exceptions: [String]!
		}

		type CartResultSuccess {
			cart: Cart!
		}

		type Cart {
			id: String!
			items: [CartItem]
		}

		type CartItem {
			id: String!
			product: CommerceProduct
			bundleProducts: [CartBundleProduct]
		}

		type CartBundleProduct @key(fields: "code", resolvable: true) @shareable {
			code: String @external
			name: String
			category: String
			entryGroupNumber: Int
		}

		type CommerceProduct @key(fields: "id") {
			id: String!
		}
	`

	// Products subgraph SDL — code is non-external (the authority)
	productsSubgraphSDL := `
		type Query {
			_dummy: String
		}

		type CartBundleProduct @key(fields: "code", resolvable: true) @shareable {
			code: String
			name: String
			entryGroupNumber: Int
		}

		type CommerceProduct @key(fields: "id") {
			id: String!
			title: String
		}
	`

	cartDatasourceConfiguration := mustDataSourceConfiguration(t,
		"cart.service",
		&plan.DataSourceMetadata{
			RootNodes: []plan.TypeField{
				{
					TypeName:   "Query",
					FieldNames: []string{"cartById"},
				},
				{
					TypeName: "CartBundleProduct",
					// Key critical detail: code is ONLY in ExternalFieldNames, NOT in FieldNames.
					// This models newer composition output where @external key fields
					// are only in ExternalFieldNames.
					FieldNames:         []string{"name", "category", "entryGroupNumber"},
					ExternalFieldNames: []string{"code"},
				},
				{
					TypeName:   "CommerceProduct",
					FieldNames: []string{"id"},
				},
			},
			ChildNodes: []plan.TypeField{
				{
					TypeName:   "CartResultSuccess",
					FieldNames: []string{"cart"},
				},
				{
					TypeName:   "CartExceptions",
					FieldNames: []string{"exceptions"},
				},
				{
					TypeName:   "Cart",
					FieldNames: []string{"id", "items"},
				},
				{
					TypeName:   "CartItem",
					FieldNames: []string{"id", "product", "bundleProducts"},
				},
			},
			FederationMetaData: plan.FederationMetaData{
				Keys: plan.FederationFieldConfigurations{
					{
						TypeName:     "CartBundleProduct",
						SelectionSet: "code",
					},
					{
						TypeName:     "CommerceProduct",
						SelectionSet: "id",
					},
				},
			},
		},
		mustCustomConfiguration(t,
			ConfigurationInput{
				Fetch: &FetchConfiguration{
					URL: "http://cart.service",
				},
				SchemaConfiguration: mustSchema(t,
					&FederationConfiguration{
						Enabled:    true,
						ServiceSDL: cartSubgraphSDL,
					},
					cartSubgraphSDL,
				),
			},
		),
	)

	productsDatasourceConfiguration := mustDataSourceConfiguration(t,
		"products.service",
		&plan.DataSourceMetadata{
			RootNodes: []plan.TypeField{
				{
					TypeName: "CartBundleProduct",
					// In the products subgraph, code is a regular field (the authority)
					FieldNames: []string{"code", "name", "entryGroupNumber"},
				},
				{
					TypeName:   "CommerceProduct",
					FieldNames: []string{"id", "title"},
				},
			},
			FederationMetaData: plan.FederationMetaData{
				Keys: plan.FederationFieldConfigurations{
					{
						TypeName:     "CartBundleProduct",
						SelectionSet: "code",
					},
					{
						TypeName:     "CommerceProduct",
						SelectionSet: "id",
					},
				},
			},
		},
		mustCustomConfiguration(t,
			ConfigurationInput{
				Fetch: &FetchConfiguration{
					URL: "http://products.service",
				},
				SchemaConfiguration: mustSchema(t,
					&FederationConfiguration{
						Enabled:    true,
						ServiceSDL: productsSubgraphSDL,
					},
					productsSubgraphSDL,
				),
			},
		),
	)

	dataSources := []plan.DataSource{
		cartDatasourceConfiguration,
		productsDatasourceConfiguration,
	}

	planConfiguration := plan.Configuration{
		DataSources:                  dataSources,
		DisableResolveFieldPositions: true,
		Fields: plan.FieldConfigurations{
			{
				TypeName:  "Query",
				FieldName: "cartById",
				Arguments: plan.ArgumentsConfigurations{
					{
						Name:       "cartId",
						SourceType: plan.FieldArgumentSource,
					},
				},
			},
		},
	}

	// Test 1: bundleProducts with code field — was FAIL before fix.
	// The cart subgraph can resolve bundleProducts (it has CartItem.bundleProducts child node)
	// and should be able to return the code field despite it being @external, because the
	// planner always includes key fields in subgraph queries and the datasource actively
	// resolves this entity type (it has non-external fields like name, category).
	t.Run("bundleProducts with code field", func(t *testing.T) {
		t.Run("run", RunTest(
			definition,
			`
			query CartQuery($cartId: String!) {
				cartById(cartId: $cartId) {
					... on CartResultSuccess {
						cart {
							id
							items {
								id
								bundleProducts {
									code
									name
								}
							}
						}
					}
				}
			}`,
			"CartQuery",
			&plan.SynchronousResponsePlan{
				Response: &resolve.GraphQLResponse{
					Fetches: resolve.Sequence(
						resolve.Single(&resolve.SingleFetch{
							FetchDependencies: resolve.FetchDependencies{
								FetchID: 0,
							},
							DataSourceIdentifier: []byte("graphql_datasource.Source"),
							FetchConfiguration: resolve.FetchConfiguration{
								Input:          `{"method":"POST","url":"http://cart.service","body":{"query":"query($cartId: String!){cartById(cartId: $cartId){__typename ... on CartResultSuccess {cart {id items {id bundleProducts {code name}}}}}}","variables":{"cartId":$$0$$}}}`,
								DataSource:     &Source{},
								PostProcessing: DefaultPostProcessingConfiguration,
								Variables: resolve.NewVariables(
									&resolve.ContextVariable{
										Path:     []string{"cartId"},
										Renderer: resolve.NewJSONVariableRenderer(),
									},
								),
							},
						}),
					),
					Data: &resolve.Object{
						Fields: []*resolve.Field{
							{
								Name: []byte("cartById"),
								Value: &resolve.Object{
									Path:     []string{"cartById"},
									Nullable: true,
									PossibleTypes: map[string]struct{}{
										"CartExceptions":    {},
										"CartResultSuccess": {},
									},
									TypeName: "CartResult",
									Fields: []*resolve.Field{
										{
											Name: []byte("cart"),
											Value: &resolve.Object{
												Path: []string{"cart"},
												PossibleTypes: map[string]struct{}{
													"Cart": {},
												},
												TypeName: "Cart",
												Fields: []*resolve.Field{
													{
														Name: []byte("id"),
														Value: &resolve.String{
															Path: []string{"id"},
														},
													},
													{
														Name: []byte("items"),
														Value: &resolve.Array{
															Path:     []string{"items"},
															Nullable: true,
															Item: &resolve.Object{
																Nullable: true,
																PossibleTypes: map[string]struct{}{
																	"CartItem": {},
																},
																TypeName: "CartItem",
																Fields: []*resolve.Field{
																	{
																		Name: []byte("id"),
																		Value: &resolve.String{
																			Path: []string{"id"},
																		},
																	},
																	{
																		Name: []byte("bundleProducts"),
																		Value: &resolve.Array{
																			Path:     []string{"bundleProducts"},
																			Nullable: true,
																			Item: &resolve.Object{
																				Nullable: true,
																				PossibleTypes: map[string]struct{}{
																					"CartBundleProduct": {},
																				},
																				TypeName: "CartBundleProduct",
																				Fields: []*resolve.Field{
																					{
																						Name: []byte("code"),
																						Value: &resolve.String{
																							Path:     []string{"code"},
																							Nullable: true,
																						},
																					},
																					{
																						Name: []byte("name"),
																						Value: &resolve.String{
																							Path:     []string{"name"},
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
													},
												},
											},
											OnTypeNames: [][]byte{[]byte("CartResultSuccess")},
										},
									},
								},
							},
						},
					},
				},
			},
			planConfiguration, WithDefaultPostProcessor(),
		))
	})

	// Test 2: Query without bundleProducts — control, should always pass
	t.Run("cart only fields without bundleProducts", func(t *testing.T) {
		t.Run("run", RunTest(
			definition,
			`
			query CartQuery($cartId: String!) {
				cartById(cartId: $cartId) {
					... on CartResultSuccess {
						cart {
							id
							items {
								id
							}
						}
					}
				}
			}`,
			"CartQuery",
			&plan.SynchronousResponsePlan{
				Response: &resolve.GraphQLResponse{
					Fetches: resolve.Sequence(
						resolve.Single(&resolve.SingleFetch{
							FetchDependencies: resolve.FetchDependencies{
								FetchID: 0,
							},
							DataSourceIdentifier: []byte("graphql_datasource.Source"),
							FetchConfiguration: resolve.FetchConfiguration{
								Input:          `{"method":"POST","url":"http://cart.service","body":{"query":"query($cartId: String!){cartById(cartId: $cartId){__typename ... on CartResultSuccess {cart {id items {id}}}}}","variables":{"cartId":$$0$$}}}`,
								DataSource:     &Source{},
								PostProcessing: DefaultPostProcessingConfiguration,
								Variables: resolve.NewVariables(
									&resolve.ContextVariable{
										Path:     []string{"cartId"},
										Renderer: resolve.NewJSONVariableRenderer(),
									},
								),
							},
						}),
					),
					Data: &resolve.Object{
						Fields: []*resolve.Field{
							{
								Name: []byte("cartById"),
								Value: &resolve.Object{
									Path:     []string{"cartById"},
									Nullable: true,
									PossibleTypes: map[string]struct{}{
										"CartExceptions":    {},
										"CartResultSuccess": {},
									},
									TypeName: "CartResult",
									Fields: []*resolve.Field{
										{
											Name: []byte("cart"),
											Value: &resolve.Object{
												Path: []string{"cart"},
												PossibleTypes: map[string]struct{}{
													"Cart": {},
												},
												TypeName: "Cart",
												Fields: []*resolve.Field{
													{
														Name: []byte("id"),
														Value: &resolve.String{
															Path: []string{"id"},
														},
													},
													{
														Name: []byte("items"),
														Value: &resolve.Array{
															Path:     []string{"items"},
															Nullable: true,
															Item: &resolve.Object{
																Nullable: true,
																PossibleTypes: map[string]struct{}{
																	"CartItem": {},
																},
																TypeName: "CartItem",
																Fields: []*resolve.Field{
																	{
																		Name: []byte("id"),
																		Value: &resolve.String{
																			Path: []string{"id"},
																		},
																	},
																},
															},
														},
													},
												},
											},
											OnTypeNames: [][]byte{[]byte("CartResultSuccess")},
										},
									},
								},
							},
						},
					},
				},
			},
			planConfiguration, WithDefaultPostProcessor(),
		))
	})

	// Test 3: bundleProducts WITHOUT code field — control, should always pass
	t.Run("bundleProducts without code field", func(t *testing.T) {
		t.Run("run", RunTest(
			definition,
			`
			query CartQuery($cartId: String!) {
				cartById(cartId: $cartId) {
					... on CartResultSuccess {
						cart {
							id
							items {
								id
								bundleProducts {
									name
									category
								}
							}
						}
					}
				}
			}`,
			"CartQuery",
			&plan.SynchronousResponsePlan{
				Response: &resolve.GraphQLResponse{
					Fetches: resolve.Sequence(
						resolve.Single(&resolve.SingleFetch{
							FetchDependencies: resolve.FetchDependencies{
								FetchID: 0,
							},
							DataSourceIdentifier: []byte("graphql_datasource.Source"),
							FetchConfiguration: resolve.FetchConfiguration{
								Input:          `{"method":"POST","url":"http://cart.service","body":{"query":"query($cartId: String!){cartById(cartId: $cartId){__typename ... on CartResultSuccess {cart {id items {id bundleProducts {name category}}}}}}","variables":{"cartId":$$0$$}}}`,
								DataSource:     &Source{},
								PostProcessing: DefaultPostProcessingConfiguration,
								Variables: resolve.NewVariables(
									&resolve.ContextVariable{
										Path:     []string{"cartId"},
										Renderer: resolve.NewJSONVariableRenderer(),
									},
								),
							},
						}),
					),
					Data: &resolve.Object{
						Fields: []*resolve.Field{
							{
								Name: []byte("cartById"),
								Value: &resolve.Object{
									Path:     []string{"cartById"},
									Nullable: true,
									PossibleTypes: map[string]struct{}{
										"CartExceptions":    {},
										"CartResultSuccess": {},
									},
									TypeName: "CartResult",
									Fields: []*resolve.Field{
										{
											Name: []byte("cart"),
											Value: &resolve.Object{
												Path: []string{"cart"},
												PossibleTypes: map[string]struct{}{
													"Cart": {},
												},
												TypeName: "Cart",
												Fields: []*resolve.Field{
													{
														Name: []byte("id"),
														Value: &resolve.String{
															Path: []string{"id"},
														},
													},
													{
														Name: []byte("items"),
														Value: &resolve.Array{
															Path:     []string{"items"},
															Nullable: true,
															Item: &resolve.Object{
																Nullable: true,
																PossibleTypes: map[string]struct{}{
																	"CartItem": {},
																},
																TypeName: "CartItem",
																Fields: []*resolve.Field{
																	{
																		Name: []byte("id"),
																		Value: &resolve.String{
																			Path: []string{"id"},
																		},
																	},
																	{
																		Name: []byte("bundleProducts"),
																		Value: &resolve.Array{
																			Path:     []string{"bundleProducts"},
																			Nullable: true,
																			Item: &resolve.Object{
																				Nullable: true,
																				PossibleTypes: map[string]struct{}{
																					"CartBundleProduct": {},
																				},
																				TypeName: "CartBundleProduct",
																				Fields: []*resolve.Field{
																					{
																						Name: []byte("name"),
																						Value: &resolve.String{
																							Path:     []string{"name"},
																							Nullable: true,
																						},
																					},
																					{
																						Name: []byte("category"),
																						Value: &resolve.String{
																							Path:     []string{"category"},
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
													},
												},
											},
											OnTypeNames: [][]byte{[]byte("CartResultSuccess")},
										},
									},
								},
							},
						},
					},
				},
			},
			planConfiguration, WithDefaultPostProcessor(),
		))
	})

	// Test 4: bundleProducts with ONLY code field — was FAIL before fix
	t.Run("bundleProducts with only code field", func(t *testing.T) {
		t.Run("run", RunTest(
			definition,
			`
			query CartQuery($cartId: String!) {
				cartById(cartId: $cartId) {
					... on CartResultSuccess {
						cart {
							id
							items {
								id
								bundleProducts {
									code
								}
							}
						}
					}
				}
			}`,
			"CartQuery",
			&plan.SynchronousResponsePlan{
				Response: &resolve.GraphQLResponse{
					Fetches: resolve.Sequence(
						resolve.Single(&resolve.SingleFetch{
							FetchDependencies: resolve.FetchDependencies{
								FetchID: 0,
							},
							DataSourceIdentifier: []byte("graphql_datasource.Source"),
							FetchConfiguration: resolve.FetchConfiguration{
								Input:          `{"method":"POST","url":"http://cart.service","body":{"query":"query($cartId: String!){cartById(cartId: $cartId){__typename ... on CartResultSuccess {cart {id items {id bundleProducts {code}}}}}}","variables":{"cartId":$$0$$}}}`,
								DataSource:     &Source{},
								PostProcessing: DefaultPostProcessingConfiguration,
								Variables: resolve.NewVariables(
									&resolve.ContextVariable{
										Path:     []string{"cartId"},
										Renderer: resolve.NewJSONVariableRenderer(),
									},
								),
							},
						}),
					),
					Data: &resolve.Object{
						Fields: []*resolve.Field{
							{
								Name: []byte("cartById"),
								Value: &resolve.Object{
									Path:     []string{"cartById"},
									Nullable: true,
									PossibleTypes: map[string]struct{}{
										"CartExceptions":    {},
										"CartResultSuccess": {},
									},
									TypeName: "CartResult",
									Fields: []*resolve.Field{
										{
											Name: []byte("cart"),
											Value: &resolve.Object{
												Path: []string{"cart"},
												PossibleTypes: map[string]struct{}{
													"Cart": {},
												},
												TypeName: "Cart",
												Fields: []*resolve.Field{
													{
														Name: []byte("id"),
														Value: &resolve.String{
															Path: []string{"id"},
														},
													},
													{
														Name: []byte("items"),
														Value: &resolve.Array{
															Path:     []string{"items"},
															Nullable: true,
															Item: &resolve.Object{
																Nullable: true,
																PossibleTypes: map[string]struct{}{
																	"CartItem": {},
																},
																TypeName: "CartItem",
																Fields: []*resolve.Field{
																	{
																		Name: []byte("id"),
																		Value: &resolve.String{
																			Path: []string{"id"},
																		},
																	},
																	{
																		Name: []byte("bundleProducts"),
																		Value: &resolve.Array{
																			Path:     []string{"bundleProducts"},
																			Nullable: true,
																			Item: &resolve.Object{
																				Nullable: true,
																				PossibleTypes: map[string]struct{}{
																					"CartBundleProduct": {},
																				},
																				TypeName: "CartBundleProduct",
																				Fields: []*resolve.Field{
																					{
																						Name: []byte("code"),
																						Value: &resolve.String{
																							Path:     []string{"code"},
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
													},
												},
											},
											OnTypeNames: [][]byte{[]byte("CartResultSuccess")},
										},
									},
								},
							},
						},
					},
				},
			},
			planConfiguration, WithDefaultPostProcessor(),
		))
	})
}
