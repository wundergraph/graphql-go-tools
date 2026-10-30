package graphql_datasource

import (
	"testing"

	. "github.com/wundergraph/graphql-go-tools/v2/pkg/engine/datasourcetesting"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

// TestGraphQLDataSourceFederationEntityInterfacesEventDriven covers entity interface subscription
// roots on a datasource with every key having DisableEntityResolver, as composed for an event-driven
// subgraph: the concrete __typename has to come from a datasource which can resolve the entity.
func TestGraphQLDataSourceFederationEntityInterfacesEventDriven(t *testing.T) {
	federationFactory := &Factory[Configuration]{}

	definition := EntityInterfacesDefinition
	planConfiguration := *EntityInterfacesPlanConfiguration(t, federationFactory)

	t.Run("subscription 0 - entity interface __typename with a local and a key-only fragment", RunTest(
		definition,
		`
			subscription _0_EntityInterfaceTypenameMixed {
				accountEvents {
					__typename
					... on Moderator {
						id
						title
					}
					... on User {
						id
					}
				}
			}`,
		"_0_EntityInterfaceTypenameMixed",
		&plan.SubscriptionResponsePlan{
			Response: &resolve.GraphQLSubscription{
				Trigger: resolve.GraphQLSubscriptionTrigger{
					Input:          []byte(`{"url":"ws://localhost:4005/graphql","body":{"query":"subscription{accountEvents {__typename ... on Moderator {id __typename} ... on User {id} id}}"}}`),
					Source:         &SubscriptionSource{},
					PostProcessing: DefaultPostProcessingConfiguration,
					SourceName:     "events",
					SourceID:       "events",
				},
				Response: &resolve.GraphQLResponse{
					Fetches: resolve.Sequence(
						resolve.SingleWithPath(&resolve.SingleFetch{
							FetchDependencies: resolve.FetchDependencies{
								FetchID:           1,
								DependsOnFetchIDs: []int{0},
							},
							FetchConfiguration: resolve.FetchConfiguration{
								Input: `{"method":"POST","url":"http://localhost:4001/graphql","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Account {__typename} ... on Moderator {__typename title}}}","variables":{"representations":[$$0$$]}}}`,
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
													OnTypeNames: [][]byte{[]byte("Moderator"), []byte("Account")},
												},
												{
													Name: []byte("id"),
													Value: &resolve.Scalar{
														Path: []string{"id"},
													},
													OnTypeNames: [][]byte{[]byte("Moderator"), []byte("Account")},
												},
												{
													Name: []byte("__typename"),
													Value: &resolve.String{
														Path: []string{"__typename"},
													},
													OnTypeNames: [][]byte{[]byte("Account")},
												},
												{
													Name: []byte("id"),
													Value: &resolve.Scalar{
														Path: []string{"id"},
													},
													OnTypeNames: [][]byte{[]byte("Account")},
												},
											},
										}),
									},
								},
								RequiresEntityFetch:                   true,
								PostProcessing:                        SingleEntityPostProcessingConfiguration,
								DataSource:                            &Source{},
								SetTemplateOutputToNullOnVariableNull: true,
							},
							DataSourceIdentifier: []byte("graphql_datasource.Source"),
						}, "accountEvents", resolve.ObjectPath("accountEvents")),
					),
					Data: &resolve.Object{
						Fields: []*resolve.Field{
							{
								Name: []byte("accountEvents"),
								Value: &resolve.Object{
									Path: []string{"accountEvents"},
									PossibleTypes: map[string]struct{}{
										"Account":   {},
										"Admin":     {},
										"Moderator": {},
										"User":      {},
									},
									TypeName: "Account",
									Fields: []*resolve.Field{
										{
											Name: []byte("__typename"),
											Value: &resolve.String{
												Path:       []string{"__typename"},
												IsTypeName: true,
											},
										},
										{
											Name: []byte("id"),
											Value: &resolve.Scalar{
												Path: []string{"id"},
											},
											OnTypeNames: [][]byte{[]byte("Moderator")},
										},
										{
											Name: []byte("title"),
											Value: &resolve.String{
												Path: []string{"title"},
											},
											OnTypeNames: [][]byte{[]byte("Moderator")},
										},
										{
											Name: []byte("id"),
											Value: &resolve.Scalar{
												Path: []string{"id"},
											},
											OnTypeNames: [][]byte{[]byte("User")},
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

	t.Run("subscription 1 - entity interface __typename with a fragment resolved on a third subgraph", RunTest(
		definition,
		`
			subscription _1_EntityInterfaceTypenameRemoteFragment {
				accountEvents {
					__typename
					... on Admin {
						id
						title
					}
					... on User {
						id
					}
				}
			}`,
		"_1_EntityInterfaceTypenameRemoteFragment",
		&plan.SubscriptionResponsePlan{
			Response: &resolve.GraphQLSubscription{
				Trigger: resolve.GraphQLSubscriptionTrigger{
					Input:          []byte(`{"url":"ws://localhost:4005/graphql","body":{"query":"subscription{accountEvents {__typename ... on Admin {id __typename} ... on User {id} id}}"}}`),
					Source:         &SubscriptionSource{},
					PostProcessing: DefaultPostProcessingConfiguration,
					SourceName:     "events",
					SourceID:       "events",
				},
				Response: &resolve.GraphQLResponse{
					Fetches: resolve.Sequence(
						// Admin.title is external on the first subgraph, so it is fetched from the
						// third one, while the entity interface __typename comes from the first.
						resolve.SingleWithPath(&resolve.SingleFetch{
							FetchDependencies: resolve.FetchDependencies{
								FetchID:           1,
								DependsOnFetchIDs: []int{0},
							},
							FetchConfiguration: resolve.FetchConfiguration{
								Input: `{"method":"POST","url":"http://localhost:4003/graphql","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Admin {__typename title}}}","variables":{"representations":[$$0$$]}}}`,
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
													OnTypeNames: [][]byte{[]byte("Admin")},
												},
												{
													Name: []byte("id"),
													Value: &resolve.Scalar{
														Path: []string{"id"},
													},
													OnTypeNames: [][]byte{[]byte("Admin")},
												},
											},
										}),
									},
								},
								RequiresEntityFetch:                   true,
								PostProcessing:                        SingleEntityPostProcessingConfiguration,
								DataSource:                            &Source{},
								SetTemplateOutputToNullOnVariableNull: true,
							},
							DataSourceIdentifier: []byte("graphql_datasource.Source"),
						}, "accountEvents", resolve.ObjectPath("accountEvents")),
						resolve.SingleWithPath(&resolve.SingleFetch{
							FetchDependencies: resolve.FetchDependencies{
								FetchID:           2,
								DependsOnFetchIDs: []int{0},
							},
							FetchConfiguration: resolve.FetchConfiguration{
								Input: `{"method":"POST","url":"http://localhost:4001/graphql","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Account {__typename}}}","variables":{"representations":[$$0$$]}}}`,
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
													OnTypeNames: [][]byte{[]byte("Account")},
												},
												{
													Name: []byte("id"),
													Value: &resolve.Scalar{
														Path: []string{"id"},
													},
													OnTypeNames: [][]byte{[]byte("Account")},
												},
											},
										}),
									},
								},
								RequiresEntityFetch:                   true,
								PostProcessing:                        SingleEntityPostProcessingConfiguration,
								DataSource:                            &Source{},
								SetTemplateOutputToNullOnVariableNull: true,
							},
							DataSourceIdentifier: []byte("graphql_datasource.Source"),
						}, "accountEvents", resolve.ObjectPath("accountEvents")),
					),
					Data: &resolve.Object{
						Fields: []*resolve.Field{
							{
								Name: []byte("accountEvents"),
								Value: &resolve.Object{
									Path: []string{"accountEvents"},
									PossibleTypes: map[string]struct{}{
										"Account":   {},
										"Admin":     {},
										"Moderator": {},
										"User":      {},
									},
									TypeName: "Account",
									Fields: []*resolve.Field{
										{
											Name: []byte("__typename"),
											Value: &resolve.String{
												Path:       []string{"__typename"},
												IsTypeName: true,
											},
										},
										{
											Name: []byte("id"),
											Value: &resolve.Scalar{
												Path: []string{"id"},
											},
											OnTypeNames: [][]byte{[]byte("Admin")},
										},
										{
											Name: []byte("title"),
											Value: &resolve.String{
												Path: []string{"title"},
											},
											OnTypeNames: [][]byte{[]byte("Admin")},
										},
										{
											Name: []byte("id"),
											Value: &resolve.Scalar{
												Path: []string{"id"},
											},
											OnTypeNames: [][]byte{[]byte("User")},
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

	t.Run("subscription 2 - entity interface __typename with key-only fragments", RunTest(
		definition,
		`
			subscription _2_EntityInterfaceTypenameKeyOnly {
				accountEvents {
					__typename
					... on User {
						id
					}
				}
			}`,
		"_2_EntityInterfaceTypenameKeyOnly",
		&plan.SubscriptionResponsePlan{
			Response: &resolve.GraphQLSubscription{
				Trigger: resolve.GraphQLSubscriptionTrigger{
					Input:          []byte(`{"url":"ws://localhost:4005/graphql","body":{"query":"subscription{accountEvents {__typename ... on User {id} id}}"}}`),
					Source:         &SubscriptionSource{},
					PostProcessing: DefaultPostProcessingConfiguration,
					SourceName:     "events",
					SourceID:       "events",
				},
				Response: &resolve.GraphQLResponse{
					Fetches: resolve.Sequence(
						// Nothing but the key is selected from the concrete type, yet the entity
						// still has to be fetched to learn its concrete __typename.
						resolve.SingleWithPath(&resolve.SingleFetch{
							FetchDependencies: resolve.FetchDependencies{
								FetchID:           1,
								DependsOnFetchIDs: []int{0},
							},
							FetchConfiguration: resolve.FetchConfiguration{
								Input: `{"method":"POST","url":"http://localhost:4001/graphql","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Account {__typename}}}","variables":{"representations":[$$0$$]}}}`,
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
													OnTypeNames: [][]byte{[]byte("Account")},
												},
												{
													Name: []byte("id"),
													Value: &resolve.Scalar{
														Path: []string{"id"},
													},
													OnTypeNames: [][]byte{[]byte("Account")},
												},
											},
										}),
									},
								},
								RequiresEntityFetch:                   true,
								PostProcessing:                        SingleEntityPostProcessingConfiguration,
								DataSource:                            &Source{},
								SetTemplateOutputToNullOnVariableNull: true,
							},
							DataSourceIdentifier: []byte("graphql_datasource.Source"),
						}, "accountEvents", resolve.ObjectPath("accountEvents")),
					),
					Data: &resolve.Object{
						Fields: []*resolve.Field{
							{
								Name: []byte("accountEvents"),
								Value: &resolve.Object{
									Path: []string{"accountEvents"},
									PossibleTypes: map[string]struct{}{
										"Account":   {},
										"Admin":     {},
										"Moderator": {},
										"User":      {},
									},
									TypeName: "Account",
									Fields: []*resolve.Field{
										{
											Name: []byte("__typename"),
											Value: &resolve.String{
												Path:       []string{"__typename"},
												IsTypeName: true,
											},
										},
										{
											Name: []byte("id"),
											Value: &resolve.Scalar{
												Path: []string{"id"},
											},
											OnTypeNames: [][]byte{[]byte("User")},
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

	t.Run("subscription 3 - entity interface which no datasource can resolve keeps __typename local", RunTest(
		definition,
		`
			subscription _3_SoleSourceEntityInterfaceTypename {
				orphanEvents {
					__typename
				}
			}`,
		"_3_SoleSourceEntityInterfaceTypename",
		&plan.SubscriptionResponsePlan{
			Response: &resolve.GraphQLSubscription{
				Trigger: resolve.GraphQLSubscriptionTrigger{
					Input:          []byte(`{"url":"ws://localhost:4006/graphql","body":{"query":"subscription{orphanEvents {__typename}}"}}`),
					Source:         &SubscriptionSource{},
					PostProcessing: DefaultPostProcessingConfiguration,
					SourceName:     "orphan-events",
					SourceID:       "orphan-events",
				},
				Response: &resolve.GraphQLResponse{
					Fetches: resolve.Sequence(),
					Data: &resolve.Object{
						Fields: []*resolve.Field{
							{
								Name: []byte("orphanEvents"),
								Value: &resolve.Object{
									Path: []string{"orphanEvents"},
									PossibleTypes: map[string]struct{}{
										"OrphanA":     {},
										"OrphanB":     {},
										"OrphanEvent": {},
									},
									TypeName: "OrphanEvent",
									Fields: []*resolve.Field{
										{
											Name: []byte("__typename"),
											Value: &resolve.String{
												Path:       []string{"__typename"},
												IsTypeName: true,
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
		WithDefaultPostProcessor(),
	))

	t.Run("subscription 4 - __typename inside a concrete member fragment stays local", RunTest(
		definition,
		`
			subscription _4_ConcreteMemberTypename {
				accountEvents {
					... on User {
						__typename
						id
					}
				}
			}`,
		"_4_ConcreteMemberTypename",
		&plan.SubscriptionResponsePlan{
			Response: &resolve.GraphQLSubscription{
				Trigger: resolve.GraphQLSubscriptionTrigger{
					Input:          []byte(`{"url":"ws://localhost:4005/graphql","body":{"query":"subscription{accountEvents {__typename ... on User {__typename id}}}"}}`),
					Source:         &SubscriptionSource{},
					PostProcessing: DefaultPostProcessingConfiguration,
					SourceName:     "events",
					SourceID:       "events",
				},
				Response: &resolve.GraphQLResponse{
					Fetches: resolve.Sequence(),
					Data: &resolve.Object{
						Fields: []*resolve.Field{
							{
								Name: []byte("accountEvents"),
								Value: &resolve.Object{
									Path: []string{"accountEvents"},
									PossibleTypes: map[string]struct{}{
										"Account":   {},
										"Admin":     {},
										"Moderator": {},
										"User":      {},
									},
									TypeName: "Account",
									Fields: []*resolve.Field{
										{
											Name: []byte("__typename"),
											Value: &resolve.String{
												Path:       []string{"__typename"},
												IsTypeName: true,
											},
											OnTypeNames: [][]byte{[]byte("User")},
										},
										{
											Name: []byte("id"),
											Value: &resolve.Scalar{
												Path: []string{"id"},
											},
											OnTypeNames: [][]byte{[]byte("User")},
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

	t.Run("subscription 5 - __typename on a concrete member root field stays local", RunTest(
		definition,
		`
			subscription _5_ConcreteMemberRootTypename {
				userEvents {
					__typename
					id
				}
			}`,
		"_5_ConcreteMemberRootTypename",
		&plan.SubscriptionResponsePlan{
			Response: &resolve.GraphQLSubscription{
				Trigger: resolve.GraphQLSubscriptionTrigger{
					Input:          []byte(`{"url":"ws://localhost:4005/graphql","body":{"query":"subscription{userEvents {__typename id}}"}}`),
					Source:         &SubscriptionSource{},
					PostProcessing: DefaultPostProcessingConfiguration,
					SourceName:     "events",
					SourceID:       "events",
				},
				Response: &resolve.GraphQLResponse{
					Fetches: resolve.Sequence(),
					Data: &resolve.Object{
						Fields: []*resolve.Field{
							{
								Name: []byte("userEvents"),
								Value: &resolve.Object{
									Path: []string{"userEvents"},
									PossibleTypes: map[string]struct{}{
										"User": {},
									},
									TypeName: "User",
									Fields: []*resolve.Field{
										{
											Name: []byte("__typename"),
											Value: &resolve.String{
												Path:       []string{"__typename"},
												IsTypeName: true,
											},
										},
										{
											Name: []byte("id"),
											Value: &resolve.Scalar{
												Path: []string{"id"},
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
		WithDefaultPostProcessor(),
	))
}
