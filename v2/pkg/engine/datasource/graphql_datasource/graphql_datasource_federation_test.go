package graphql_datasource

import (
	"testing"

	. "github.com/wundergraph/graphql-go-tools/v2/pkg/engine/datasourcetesting"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

func TestGraphQLDataSourceFederation(t *testing.T) {
	batchFactory := NewBatchFactory()
	federationFactory := &Factory{BatchFactory: batchFactory}

	t.Run("composed keys, provides, requires", func(t *testing.T) {
		definition := `
			type Account {
				id: ID!
				name: String!
				info: Info
				address: Address
				shippingInfo: ShippingInfo
			}
			type Address {
				id: ID!
				line1: String!
				line2: String!
				fullAddress: String!
			}
			type Info {
				a: ID!
				b: ID!
			}
			type ShippingInfo {
				zip: String!
			}
			type User {
				id: ID!
				account: Account
				oldAccount: Account
			}
			type Query {
				user: User
				account: Account
			}
		`
		usersSubgraphSDL := `
			extend type Query {
				user: User
			}

			type User @key(fields: "id") {
				id: ID!
				account: Account
				oldAccount: Account @provides(fields: "name shippingInfo {zip}")
			}

			extend type Account @key(fields: "id info {a b}") {
				id: ID!
				info: Info
				name: String! @external
				shippingInfo: ShippingInfo @external
				address: Address
			}

			type ShippingInfo {
				zip: String! @external
			}

			extend type Info {
				a: ID! @external
				b: ID! @external
			}

			type Address @key(fields: "id") {
				id: ID!
				line1: String!
				line2: String!
			}
		`

		accountsSubgraphSDL := `
			extend type Query {
				account: Account
			}

			type Account @key(fields: "id info {a b}") {
				id: ID!
				name: String!
				info: Info
				shippingInfo: ShippingInfo
			}

			type Info {
				a: ID!
				b: ID!
			}

			type ShippingInfo {
				zip: String!
			}

			extend type Address @key(fields: "id") {
				id: ID!
				line1: String! @external
				line2: String! @external
				fullAddress: String! @requires(fields: "line1 line2")
			}
		`

		usersDatasourceConfiguration := plan.DataSourceConfiguration{
			RootNodes: []plan.TypeField{
				{
					TypeName:   "Query",
					FieldNames: []string{"user"},
				},
			},
			ChildNodes: []plan.TypeField{
				{
					TypeName:   "User",
					FieldNames: []string{"id", "account"},
				},
				{
					TypeName:   "Account",
					FieldNames: []string{"id", "info"},
				},
				{
					TypeName:   "Info",
					FieldNames: []string{"a", "b"},
				},
			},
			Custom: ConfigJson(Configuration{
				Fetch: FetchConfiguration{
					URL: "http://user.service",
				},
				Federation: FederationConfiguration{
					Enabled:    true,
					ServiceSDL: usersSubgraphSDL,
				},
			}),
			Factory: federationFactory,
			FieldConfigurations: plan.FieldConfigurations{
				{
					TypeName:                   "Account",
					RequiresFieldsSelectionSet: "id info {a b}",
				},
			},
		}

		accountsDatasourceConfiguration := plan.DataSourceConfiguration{
			RootNodes: []plan.TypeField{
				{
					TypeName:   "Account",
					FieldNames: []string{"id", "name", "info", "shippingInfo"},
				},
			},
			ChildNodes: []plan.TypeField{
				{
					TypeName:   "Info",
					FieldNames: []string{"a", "b"},
				},
				{
					TypeName:   "ShippingInfo",
					FieldNames: []string{"zip"},
				},
				{
					TypeName:   "Address",
					FieldNames: []string{"id", "fullAddress"},
				},
			},
			Custom: ConfigJson(Configuration{
				Fetch: FetchConfiguration{
					URL: "http://account.service",
				},
				Federation: FederationConfiguration{
					Enabled:    true,
					ServiceSDL: accountsSubgraphSDL,
				},
			}),
			Factory: federationFactory,
			FieldConfigurations: plan.FieldConfigurations{
				{
					TypeName:                   "Account",
					FieldName:                  "",
					RequiresFieldsSelectionSet: "id info {a b}",
				},
				{
					TypeName:                   "Address",
					FieldName:                  "fullAddress",
					RequiresFieldsSelectionSet: "line1 line2",
				},
			},
		}

		planConfiguration := plan.Configuration{
			DataSources: []plan.DataSourceConfiguration{
				usersDatasourceConfiguration,
				accountsDatasourceConfiguration,
			},
			DisableResolveFieldPositions: true,
			Debug: plan.DebugConfiguration{
				PrintOperationWithRequiredFields: true,
				PrintPlanningPaths:               true,
				PrintQueryPlans:                  true,
				ConfigurationVisitor:             true,
				PlanningVisitor:                  true,
				DatasourceVisitor:                true,
			},
		}

		t.Run("composed keys", RunTest(
			definition,
			`
				query ComposedKeys {
					user {
						account {
							name
							shippingInfo {
								zip
							}
						}
					}
				}
			`,
			"ComposedKeys",
			&plan.SynchronousResponsePlan{
				Response: &resolve.GraphQLResponse{
					Data: &resolve.Object{
						Fetch: &resolve.SingleFetch{
							BufferId:              0,
							Input:                 `{"method":"POST","url":"http://user.service","body":{"query":"query{user{account{id info{a b}}}}"}}`,
							DataSource:            &Source{},
							DataSourceIdentifier:  []byte("graphql_datasource.Source"),
							ProcessResponseConfig: resolve.ProcessResponseConfig{ExtractGraphqlResponse: true},
						},
						Fields: []*resolve.Field{
							{
								HasBuffer: true,
								BufferID:  0,
								Name:      []byte("user"),
								Value: &resolve.Object{
									Path:     []string{"user"},
									Nullable: true,
									Fields: []*resolve.Field{
										{
											HasBuffer: true,
											BufferID:  1,
											Name:      []byte("account"),

											Value: &resolve.Object{
												Path:     []string{"account"},
												Nullable: true,
												Fields: []*resolve.Field{
													{
														Name: []byte("name"),
														Value: &resolve.String{
															Path: []string{"name"},
														},
													},
													{
														Name: []byte("shippingInfo"),
														Value: &resolve.Object{
															Path:     []string{"shippingInfo"},
															Nullable: true,
															Fields: []*resolve.Field{
																{
																	Name: []byte("zip"),
																	Value: &resolve.String{
																		Path: []string{"zip"},
																	},
																},
															},
														},
													},
												},
												Fetch: &resolve.SingleFetch{
													BufferId:   1,
													Input:      `{"method":"POST","url":"http://account.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){__typename ... on Account {name shippingInfo {zip}}}}","variables":{"representations":$$0$$}}}`,
													DataSource: &Source{},
													Variables: resolve.NewVariables(
														&resolve.ContextVariable{
															Path:     []string{"b"},
															Renderer: resolve.NewJSONVariableRendererWithValidation(`{"type":["string"]}`),
														},
													),
													DataSourceIdentifier:  []byte("graphql_datasource.Source"),
													ProcessResponseConfig: resolve.ProcessResponseConfig{ExtractGraphqlResponse: true},
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
			planConfiguration,
		))

		t.Run("provides", RunTest(
			`definition`,
			`
				query ComposedKeys {
					user {
						oldAccount {
							name
							shippingInfo {
								zip
							}
						}
					}
				}
			`,
			"ComposedKeys",
			&plan.SynchronousResponsePlan{
				Response: &resolve.GraphQLResponse{
					Data: &resolve.Object{
						Fetch: &resolve.ParallelFetch{
							Fetches: []resolve.Fetch{
								&resolve.SingleFetch{
									BufferId:              0,
									Input:                 `{"method":"POST","url":"http://user.service","body":{"query":"query{user{oldAccount{name ShippingInfo{zip}}}}"}}`,
									DataSource:            &Source{},
									DataSourceIdentifier:  []byte("graphql_datasource.Source"),
									ProcessResponseConfig: resolve.ProcessResponseConfig{ExtractGraphqlResponse: true},
								},
							},
						},
						Fields: []*resolve.Field{
							{
								HasBuffer: true,
								BufferID:  0,
								Name:      []byte("user"),
								Value: &resolve.Object{
									Path:     []string{"user"},
									Nullable: true,
									Fields: []*resolve.Field{
										{
											HasBuffer: true,
											BufferID:  1,
											Name:      []byte("oldAccount"),
											Value: &resolve.Object{
												Path:     []string{"oldAccount"},
												Nullable: true,
												Fields: []*resolve.Field{
													{
														Name: []byte("name"),
														Value: &resolve.String{
															Path: []string{"name"},
														},
													},
													{
														Name: []byte("shippingInfo"),
														Value: &resolve.Object{
															Path:     []string{"shippingInfo"},
															Nullable: true,
															Fields: []*resolve.Field{
																{
																	Name: []byte("zip"),
																	Value: &resolve.String{
																		Path: []string{"zip"},
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
					},
				},
			},
			planConfiguration,
		))

		// TODO: more complex example
		// `query ComposedKeys {
		// 	user {
		// 		account {
		// 			name
		// 			shippingInfo {
		// 				zip
		// 			}
		// 			address {
		// 				line1
		// 			}
		// 		}
		// 		oldAccount {
		// 			name
		// 			shippingInfo {
		// 				zip
		// 			}
		// 			address {
		// 				line1
		// 			}
		// 		}
		// 	}
		// }`
	})
}
