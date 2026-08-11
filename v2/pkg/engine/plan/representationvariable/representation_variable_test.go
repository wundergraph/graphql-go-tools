package representationvariable

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

		// Without caching configured the planner asks for no @key marking, so
		// these rows also pin that the node is what it was before the marking
		// existed.
		node, err := BuildRepresentationVariableNode(&definition, cfg, federationMeta, false)
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

	t.Run("with entity interface", func(t *testing.T) {
		runTest(t, `
			scalar String

			type User {
				id: String!
				name: String!
			}
		`,
			`id name`,
			plan.FederationMetaData{
				EntityInterfaces: []plan.EntityInterfaceConfiguration{
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
						Value: &resolve.String{
							Path: []string{"__typename"},
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

	t.Run("with inline fragment", func(t *testing.T) {
		runTest(t, `
			scalar String

			type User {
				id: String!
				name: String!
				u: ab!
				i: Title!
			}

			interface Title {
				title: String!
			}

			type A implements Title {
			  	a: String!
				title: String!
			}

			type B implements Title {
				b: String!
				title: String!
			}

		    union ab = A | B
		`,
			`u { ... on A { a } } i { ... on B { title } }`,
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
						Name: []byte("u"),
						Value: &resolve.Object{
							Path: []string{"u"},
							Fields: []*resolve.Field{
								{
									Name: []byte("a"),
									Value: &resolve.String{
										Path: []string{"a"},
									},
									OnTypeNames: [][]byte{[]byte("A")},
								},
							},
						},
						OnTypeNames: [][]byte{[]byte("User")},
					},
					{
						Name: []byte("i"),
						Value: &resolve.Object{
							Path: []string{"i"},
							Fields: []*resolve.Field{
								{
									Name: []byte("title"),
									Value: &resolve.String{
										Path: []string{"title"},
									},
									OnTypeNames: [][]byte{[]byte("B")},
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

		merged := MergeRepresentationVariableNodes([]*resolve.Object{userRepresentation, adminRepresentation})
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

		merged := MergeRepresentationVariableNodes([]*resolve.Object{userKeyRepresentation, userRequiresRepresentation})
		require.Equal(t, expected, merged)
	})

	t.Run("array of scalars", func(t *testing.T) {
		userKeyRepresentation := &resolve.Object{
			Fields: []*resolve.Field{
				{
					Name: []byte("items"),
					Value: &resolve.Array{
						Path: []string{"items"},
						Item: &resolve.String{
							Path: []string{"id"},
						},
					},
					OnTypeNames: [][]byte{[]byte("User")},
				},
			},
		}

		userRequiresRepresentation := &resolve.Object{
			Fields: []*resolve.Field{
				{
					Name: []byte("items"),
					Value: &resolve.Array{
						Path: []string{"items"},
						Item: &resolve.String{
							Path: []string{"id"},
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
					Name: []byte("items"),
					Value: &resolve.Array{
						Path: []string{"items"},
						Item: &resolve.String{
							Path: []string{"id"},
						},
					},
					OnTypeNames: [][]byte{[]byte("User")},
				},
			},
		}

		merged := MergeRepresentationVariableNodes([]*resolve.Object{userKeyRepresentation, userRequiresRepresentation})
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

		merged := MergeRepresentationVariableNodes([]*resolve.Object{userKeyRepresentation, userRequiresRepresentation})
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

		merged := MergeRepresentationVariableNodes([]*resolve.Object{userKeyRepresentation, userRequiresRepresentation})
		require.Equal(t, expected, merged)
	})
}

// TestBuildRepresentationVariableNodeKeyFieldMarking pins the @key provenance
// the cache reads off the merged representation node: which fields carry it,
// which do not, that a field selected by BOTH a @key and a @requires set stays
// a key field, and that the marking is opt-in.
func TestBuildRepresentationVariableNodeKeyFieldMarking(t *testing.T) {
	definitionStr := `
		scalar String
		scalar Int

		type User {
			id: String!
			name: String!
			account: Account!
		}

		type Account {
			accountID: Int!
			balance: Int!
		}
	`

	build := func(t *testing.T, cfg plan.FederationFieldConfiguration, markKeyFields bool) *resolve.Object {
		t.Helper()
		definition, _ := astparser.ParseGraphqlDocumentString(definitionStr)
		node, err := BuildRepresentationVariableNode(&definition, cfg, plan.FederationMetaData{}, markKeyFields)
		require.NoError(t, err)
		return node
	}

	t.Run("a @key set marks every field it emits, at every depth", func(t *testing.T) {
		node := build(t, plan.FederationFieldConfiguration{
			TypeName:     "User",
			SelectionSet: `id account { accountID }`,
		}, true)
		require.Equal(t, &resolve.Object{
			Nullable: true,
			Fields: []*resolve.Field{
				{
					// __typename is never part of the hashed key subset, so the
					// builder leaves it unmarked.
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
					OnTypeNames:         [][]byte{[]byte("User")},
					CacheEntityKeyField: true,
				},
				{
					Name: []byte("account"),
					Value: &resolve.Object{
						Path: []string{"account"},
						Fields: []*resolve.Field{
							{
								Name: []byte("accountID"),
								Value: &resolve.Integer{
									Path: []string{"accountID"},
								},
								CacheEntityKeyField: true,
							},
						},
					},
					OnTypeNames:         [][]byte{[]byte("User")},
					CacheEntityKeyField: true,
				},
			},
		}, node)
	})

	t.Run("a @requires set marks nothing: it names the field carrying the directive", func(t *testing.T) {
		node := build(t, plan.FederationFieldConfiguration{
			TypeName:     "User",
			FieldName:    "shippingEstimate",
			SelectionSet: `name`,
		}, true)
		require.Equal(t, &resolve.Object{
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
					Name: []byte("name"),
					Value: &resolve.String{
						Path: []string{"name"},
					},
					OnTypeNames: [][]byte{[]byte("User")},
				},
			},
		}, node)
	})

	t.Run("merging keeps a field the @key set claims, whichever side declared it first", func(t *testing.T) {
		key := build(t, plan.FederationFieldConfiguration{
			TypeName:     "User",
			SelectionSet: `id account { accountID }`,
		}, true)
		requires := build(t, plan.FederationFieldConfiguration{
			TypeName:     "User",
			FieldName:    "shippingEstimate",
			SelectionSet: `id name account { balance }`,
		}, true)

		// @requires FIRST, so the merge has to OR the flag onto a field it
		// already collected unmarked.
		require.Equal(t, &resolve.Object{
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
					OnTypeNames:         [][]byte{[]byte("User")},
					CacheEntityKeyField: true,
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
								Name: []byte("balance"),
								Value: &resolve.Integer{
									Path: []string{"balance"},
								},
							},
							{
								Name: []byte("accountID"),
								Value: &resolve.Integer{
									Path: []string{"accountID"},
								},
								CacheEntityKeyField: true,
							},
						},
					},
					OnTypeNames:         [][]byte{[]byte("User")},
					CacheEntityKeyField: true,
				},
			},
		}, MergeRepresentationVariableNodes([]*resolve.Object{requires, key}))
	})

	t.Run("without the marking a @key set emits exactly the unmarked node", func(t *testing.T) {
		node := build(t, plan.FederationFieldConfiguration{
			TypeName:     "User",
			SelectionSet: `id account { accountID }`,
		}, false)
		require.Equal(t, &resolve.Object{
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
					Name: []byte("account"),
					Value: &resolve.Object{
						Path: []string{"account"},
						Fields: []*resolve.Field{
							{
								Name: []byte("accountID"),
								Value: &resolve.Integer{
									Path: []string{"accountID"},
								},
							},
						},
					},
					OnTypeNames: [][]byte{[]byte("User")},
				},
			},
		}, node)
	})
}
