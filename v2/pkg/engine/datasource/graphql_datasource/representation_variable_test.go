package graphql_datasource

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/astparser"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

func TestBuildRepresentationVariableNode(t *testing.T) {
	runTest := func(t *testing.T, definitionStr, keyStr string, federationMeta plan.FederationMetaData, expectedNode resolve.Node) {
		definition, _ := astparser.ParseGraphqlDocumentString(definitionStr)
		cfg := plan.FederationFieldConfiguration{
			TypeName:     "User",
			SelectionSet: keyStr,
		}

		node, err := buildRepresentationVariableNode(&definition, cfg, federationMeta)
		require.NoError(t, err)

		require.Equal(t, expectedNode, node)
	}

	t.Run("simple", func(t *testing.T) {
		runTest(t, `
			scalar String
	
			type User {
				id: String!
				name: String!
			}
		`,
			`id name`,
			plan.FederationMetaData{},
			&resolve.Object{
				Nullable: true,
				Fields: []*resolve.Field{
					{
						Name: []byte("__typename"),
						Value: &resolve.String{
							Path: []string{"__typename"},
						},
						OnTypeNames: [][]byte{[]byte("User")},
					},
					{
						Name: []byte("id"),
						Value: &resolve.String{
							Path: []string{"id"},
						},
						OnTypeNames: [][]byte{[]byte("User")},
					},
					{
						Name: []byte("name"),
						Value: &resolve.String{
							Path: []string{"name"},
						},
						OnTypeNames: [][]byte{[]byte("User")},
					},
				},
			})
	})

	t.Run("with interface object", func(t *testing.T) {
		runTest(t, `
			scalar String
	
			type User {
				id: String!
				name: String!
			}
		`,
			`id name`,
			plan.FederationMetaData{
				InterfaceObjects: []plan.EntityInterfaceConfiguration{
					{
						InterfaceTypeName: "Account",
						ConcreteTypeNames: []string{"User", "Admin"},
					},
				},
			},
			&resolve.Object{
				Nullable: true,
				Fields: []*resolve.Field{
					{
						Name: []byte("__typename"),
						Value: &resolve.StaticString{
							Path:  []string{"__typename"},
							Value: "Account",
						},
						OnTypeNames: [][]byte{[]byte("User"), []byte("Account")},
					},
					{
						Name: []byte("id"),
						Value: &resolve.String{
							Path: []string{"id"},
						},
						OnTypeNames: [][]byte{[]byte("User"), []byte("Account")},
					},
					{
						Name: []byte("name"),
						Value: &resolve.String{
							Path: []string{"name"},
						},
						OnTypeNames: [][]byte{[]byte("User"), []byte("Account")},
					},
				},
			})
	})

	t.Run("deeply nested", func(t *testing.T) {
		runTest(t, `
			scalar String
			scalar Int
			scalar Float
	
			type User {
				id: String!
				name: String!
				account: Account!
			}

			type Account {
				accoundID: Int!
				address(home: Boolean): Address!
			}

			type Address {
				zip: Float!
			}
				
		`,
			`id name account { accoundID address(home: true) { zip } }`,
			plan.FederationMetaData{},
			&resolve.Object{
				Nullable: true,
				Fields: []*resolve.Field{
					{
						Name: []byte("__typename"),
						Value: &resolve.String{
							Path: []string{"__typename"},
						},
						OnTypeNames: [][]byte{[]byte("User")},
					},
					{
						Name: []byte("id"),
						Value: &resolve.String{
							Path: []string{"id"},
						},
						OnTypeNames: [][]byte{[]byte("User")},
					},
					{
						Name: []byte("name"),
						Value: &resolve.String{
							Path: []string{"name"},
						},
						OnTypeNames: [][]byte{[]byte("User")},
					},
					{
						Name: []byte("account"),
						Value: &resolve.Object{
							Path: []string{"account"},
							Fields: []*resolve.Field{
								{
									Name: []byte("accoundID"),
									Value: &resolve.Integer{
										Path: []string{"accoundID"},
									},
								},
								{
									Name: []byte("address"),
									Value: &resolve.Object{
										Path: []string{"address"},
										Fields: []*resolve.Field{
											{
												Name: []byte("zip"),
												Value: &resolve.Float{
													Path: []string{"zip"},
												},
											},
										},
									},
								},
							},
						},
						OnTypeNames: [][]byte{[]byte("User")},
					},
				},
			})
	})
}

func TestMergeRepresentationVariableNodes(t *testing.T) {
	t.Run("different entities", func(t *testing.T) {
		userRepresentation := &resolve.Object{
			Fields: []*resolve.Field{
				{
					Name: []byte("id"),
					Value: &resolve.String{
						Path: []string{"id"},
					},
					OnTypeNames: [][]byte{[]byte("User")},
				},
			},
		}

		adminRepresentation := &resolve.Object{
			Fields: []*resolve.Field{
				{
					Name: []byte("id"),
					Value: &resolve.String{
						Path: []string{"id"},
					},
					OnTypeNames: [][]byte{[]byte("Admin")},
				},
			},
		}

		expected := &resolve.Object{
			Nullable: true,
			Fields: []*resolve.Field{
				{
					Name: []byte("id"),
					Value: &resolve.String{
						Path: []string{"id"},
					},
					OnTypeNames: [][]byte{[]byte("User")},
				},
				{
					Name: []byte("id"),
					Value: &resolve.String{
						Path: []string{"id"},
					},
					OnTypeNames: [][]byte{[]byte("Admin")},
				},
			},
		}

		merged := mergeRepresentationVariableNodes([]*resolve.Object{userRepresentation, adminRepresentation})
		require.Equal(t, expected, merged)
	})

	t.Run("same entity plain fields", func(t *testing.T) {
		userKeyRepresentation := &resolve.Object{
			Fields: []*resolve.Field{
				{
					Name: []byte("id"),
					Value: &resolve.String{
						Path: []string{"id"},
					},
					OnTypeNames: [][]byte{[]byte("User")},
				},
			},
		}

		userRequiresRepresentation := &resolve.Object{
			Fields: []*resolve.Field{
				{
					Name: []byte("name"),
					Value: &resolve.String{
						Path: []string{"name"},
					},
					OnTypeNames: [][]byte{[]byte("User")},
				},
			},
		}

		expected := &resolve.Object{
			Nullable: true,
			Fields: []*resolve.Field{
				{
					Name: []byte("id"),
					Value: &resolve.String{
						Path: []string{"id"},
					},
					OnTypeNames: [][]byte{[]byte("User")},
				},
				{
					Name: []byte("name"),
					Value: &resolve.String{
						Path: []string{"name"},
					},
					OnTypeNames: [][]byte{[]byte("User")},
				},
			},
		}

		merged := mergeRepresentationVariableNodes([]*resolve.Object{userKeyRepresentation, userRequiresRepresentation})
		require.Equal(t, expected, merged)
	})

	t.Run("same entity nested fields - merge on depth 1 and 2", func(t *testing.T) {
		userKeyRepresentation := &resolve.Object{
			Fields: []*resolve.Field{
				{
					Name: []byte("id"),
					Value: &resolve.String{
						Path: []string{"id"},
					},
					OnTypeNames: [][]byte{[]byte("User")},
				},
				{
					Name: []byte("userInfo"),
					Value: &resolve.Object{
						Path: []string{"userInfo"},
						Fields: []*resolve.Field{
							{
								Name: []byte("kind"),
								Value: &resolve.String{
									Path: []string{"kind"},
								},
							},
							{
								Name: []byte("addresses"),
								Value: &resolve.Array{
									Path: []string{"addresses"},
									Item: &resolve.Object{
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
					OnTypeNames: [][]byte{[]byte("User")},
				},
			},
		}

		userRequiresRepresentation := &resolve.Object{
			Fields: []*resolve.Field{
				{
					Name: []byte("name"),
					Value: &resolve.String{
						Path: []string{"name"},
					},
					OnTypeNames: [][]byte{[]byte("User")},
				},
				{
					Name: []byte("userInfo"),
					Value: &resolve.Object{
						Path: []string{"userInfo"},
						Fields: []*resolve.Field{
							{
								Name: []byte("type"),
								Value: &resolve.String{
									Path: []string{"type"},
								},
							},
							{
								Name: []byte("addresses"),
								Value: &resolve.Array{
									Path: []string{"addresses"},
									Item: &resolve.Object{
										Fields: []*resolve.Field{
											{
												Name: []byte("street"),
												Value: &resolve.String{
													Path: []string{"street"},
												},
											},
										},
									},
								},
							},
						},
					},
					OnTypeNames: [][]byte{[]byte("User")},
				},
			},
		}

		expected := &resolve.Object{
			Nullable: true,
			Fields: []*resolve.Field{
				{
					Name: []byte("id"),
					Value: &resolve.String{
						Path: []string{"id"},
					},
					OnTypeNames: [][]byte{[]byte("User")},
				},
				{
					Name: []byte("userInfo"),
					Value: &resolve.Object{
						Path: []string{"userInfo"},
						Fields: []*resolve.Field{
							{
								Name: []byte("kind"),
								Value: &resolve.String{
									Path: []string{"kind"},
								},
							},
							{
								Name: []byte("addresses"),
								Value: &resolve.Array{
									Path: []string{"addresses"},
									Item: &resolve.Object{
										Fields: []*resolve.Field{
											{
												Name: []byte("zip"),
												Value: &resolve.String{
													Path: []string{"zip"},
												},
											},
											{
												Name: []byte("street"),
												Value: &resolve.String{
													Path: []string{"street"},
												},
											},
										},
									},
								},
							},
							{
								Name: []byte("type"),
								Value: &resolve.String{
									Path: []string{"type"},
								},
							},
						},
					},
					OnTypeNames: [][]byte{[]byte("User")},
				},
				{
					Name: []byte("name"),
					Value: &resolve.String{
						Path: []string{"name"},
					},
					OnTypeNames: [][]byte{[]byte("User")},
				},
			},
		}

		merged := mergeRepresentationVariableNodes([]*resolve.Object{userKeyRepresentation, userRequiresRepresentation})
		require.Equal(t, expected, merged)
	})

	t.Run("same entity nested fields - merge on depth 1,2,3", func(t *testing.T) {
		userKeyRepresentation := &resolve.Object{
			Fields: []*resolve.Field{
				{
					Name: []byte("id"),
					Value: &resolve.String{
						Path: []string{"id"},
					},
					OnTypeNames: [][]byte{[]byte("User")},
				},
				{
					Name: []byte("userInfo"),
					Value: &resolve.Object{
						Path: []string{"userInfo"},
						Fields: []*resolve.Field{
							{
								Name: []byte("kind"),
								Value: &resolve.String{
									Path: []string{"kind"},
								},
							},
							{
								Name: []byte("addresses"),
								Value: &resolve.Array{
									Path: []string{"addresses"},
									Item: &resolve.Object{
										Fields: []*resolve.Field{
											{
												Name: []byte("zipInfo"),
												Value: &resolve.Object{
													Path: []string{"zipInfo"},
													Fields: []*resolve.Field{
														{
															Name: []byte("zip1"),
															Value: &resolve.String{
																Path: []string{"zip1"},
															},
														},
													},
												},
											},
											{
												Name: []byte("streetInfo"),
												Value: &resolve.Object{
													Path: []string{"streetInfo"},
													Fields: []*resolve.Field{
														{
															Name: []byte("street1"),
															Value: &resolve.String{
																Path: []string{"street1"},
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
					OnTypeNames: [][]byte{[]byte("User")},
				},
			},
		}

		userRequiresRepresentation := &resolve.Object{
			Fields: []*resolve.Field{
				{
					Name: []byte("name"),
					Value: &resolve.String{
						Path: []string{"name"},
					},
					OnTypeNames: [][]byte{[]byte("User")},
				},
				{
					Name: []byte("userInfo"),
					Value: &resolve.Object{
						Path: []string{"userInfo"},
						Fields: []*resolve.Field{
							{
								Name: []byte("type"),
								Value: &resolve.String{
									Path: []string{"type"},
								},
							},
							{
								Name: []byte("addresses"),
								Value: &resolve.Array{
									Path: []string{"addresses"},
									Item: &resolve.Object{
										Fields: []*resolve.Field{
											{
												Name: []byte("zipInfo"),
												Value: &resolve.Object{
													Path: []string{"zipInfo"},
													Fields: []*resolve.Field{
														{
															Name: []byte("zip2"),
															Value: &resolve.String{
																Path: []string{"zip2"},
															},
														},
													},
												},
											},
											{
												Name: []byte("streetInfo"),
												Value: &resolve.Object{
													Path: []string{"streetInfo"},
													Fields: []*resolve.Field{
														{
															Name: []byte("street2"),
															Value: &resolve.String{
																Path: []string{"street2"},
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
					OnTypeNames: [][]byte{[]byte("User")},
				},
			},
		}

		expected := &resolve.Object{
			Nullable: true,
			Fields: []*resolve.Field{
				{
					Name: []byte("id"),
					Value: &resolve.String{
						Path: []string{"id"},
					},
					OnTypeNames: [][]byte{[]byte("User")},
				},
				{
					Name: []byte("userInfo"),
					Value: &resolve.Object{
						Path: []string{"userInfo"},
						Fields: []*resolve.Field{
							{
								Name: []byte("kind"),
								Value: &resolve.String{
									Path: []string{"kind"},
								},
							},
							{
								Name: []byte("addresses"),
								Value: &resolve.Array{
									Path: []string{"addresses"},
									Item: &resolve.Object{
										Fields: []*resolve.Field{
											{
												Name: []byte("zipInfo"),
												Value: &resolve.Object{
													Path: []string{"zipInfo"},
													Fields: []*resolve.Field{
														{
															Name: []byte("zip1"),
															Value: &resolve.String{
																Path: []string{"zip1"},
															},
														},
														{
															Name: []byte("zip2"),
															Value: &resolve.String{
																Path: []string{"zip2"},
															},
														},
													},
												},
											},
											{
												Name: []byte("streetInfo"),
												Value: &resolve.Object{
													Path: []string{"streetInfo"},
													Fields: []*resolve.Field{
														{
															Name: []byte("street1"),
															Value: &resolve.String{
																Path: []string{"street1"},
															},
														},
														{
															Name: []byte("street2"),
															Value: &resolve.String{
																Path: []string{"street2"},
															},
														},
													},
												},
											},
										},
									},
								},
							},
							{
								Name: []byte("type"),
								Value: &resolve.String{
									Path: []string{"type"},
								},
							},
						},
					},
					OnTypeNames: [][]byte{[]byte("User")},
				},
				{
					Name: []byte("name"),
					Value: &resolve.String{
						Path: []string{"name"},
					},
					OnTypeNames: [][]byte{[]byte("User")},
				},
			},
		}

		merged := mergeRepresentationVariableNodes([]*resolve.Object{userKeyRepresentation, userRequiresRepresentation})
		require.Equal(t, expected, merged)
	})
}
