package plan

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/astparser"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

func TestBuildRepresentationVariableNode(t *testing.T) {
	runTest := func(t *testing.T, definitionStr string, cfg FederationFieldConfiguration, federationMeta FederationMetaData, expectedNode *resolve.Object) {
		t.Helper()
		definition, report := astparser.ParseGraphqlDocumentString(definitionStr)
		require.False(t, report.HasErrors(), report.Error())

		node, err := BuildRepresentationVariableNode(&definition, cfg, federationMeta)
		require.NoError(t, err)
		assert.Equal(t, expectedNode, node)
	}

	t.Run("simple scalar fields", func(t *testing.T) {
		runTest(t, `
			scalar String

			type User {
				id: String!
				name: String!
			}
		`,
			FederationFieldConfiguration{
				TypeName:     "User",
				SelectionSet: "id name",
			},
			FederationMetaData{},
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

	t.Run("with RemappedPaths", func(t *testing.T) {
		runTest(t, `
			scalar String

			type User {
				id: String!
				name: String!
			}
		`,
			FederationFieldConfiguration{
				TypeName:     "User",
				SelectionSet: "id name",
				RemappedPaths: map[string]string{
					"User.id":   "userId",
					"User.name": "displayName",
				},
			},
			FederationMetaData{},
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
							Path: []string{"userId"},
						},
						OnTypeNames: [][]byte{[]byte("User")},
					},
					{
						Name: []byte("name"),
						Value: &resolve.String{
							Path: []string{"displayName"},
						},
						OnTypeNames: [][]byte{[]byte("User")},
					},
				},
			})
	})

	t.Run("with interface object type name", func(t *testing.T) {
		runTest(t, `
			scalar String

			type User {
				id: String!
				name: String!
			}
		`,
			FederationFieldConfiguration{
				TypeName:     "User",
				SelectionSet: "id name",
			},
			FederationMetaData{
				InterfaceObjects: []EntityInterfaceConfiguration{
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

	t.Run("with entity interface type name", func(t *testing.T) {
		runTest(t, `
			scalar String

			type User {
				id: String!
				name: String!
			}
		`,
			FederationFieldConfiguration{
				TypeName:     "User",
				SelectionSet: "id name",
			},
			FederationMetaData{
				EntityInterfaces: []EntityInterfaceConfiguration{
					{
						InterfaceTypeName: "Node",
						ConcreteTypeNames: []string{"User", "Product"},
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
						OnTypeNames: [][]byte{[]byte("User"), []byte("Node")},
					},
					{
						Name: []byte("id"),
						Value: &resolve.String{
							Path: []string{"id"},
						},
						OnTypeNames: [][]byte{[]byte("User"), []byte("Node")},
					},
					{
						Name: []byte("name"),
						Value: &resolve.String{
							Path: []string{"name"},
						},
						OnTypeNames: [][]byte{[]byte("User"), []byte("Node")},
					},
				},
			})
	})

	t.Run("deeply nested fields", func(t *testing.T) {
		runTest(t, `
			scalar String
			scalar Int
			scalar Float

			type User {
				id: String!
				account: Account!
			}

			type Account {
				accountID: Int!
				address: Address!
			}

			type Address {
				zip: Float!
			}
		`,
			FederationFieldConfiguration{
				TypeName:     "User",
				SelectionSet: "id account { accountID address { zip } }",
			},
			FederationMetaData{},
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

	t.Run("with inline fragment on interface", func(t *testing.T) {
		runTest(t, `
			scalar String

			type User {
				id: String!
				info: Info!
			}

			interface Info {
				title: String!
			}

			type PersonalInfo implements Info {
				title: String!
				nickname: String!
			}

			type WorkInfo implements Info {
				title: String!
				role: String!
			}
		`,
			FederationFieldConfiguration{
				TypeName:     "User",
				SelectionSet: "id info { ... on PersonalInfo { nickname } }",
			},
			FederationMetaData{},
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
						Name: []byte("info"),
						Value: &resolve.Object{
							Path: []string{"info"},
							Fields: []*resolve.Field{
								{
									Name: []byte("nickname"),
									Value: &resolve.String{
										Path: []string{"nickname"},
									},
									OnTypeNames: [][]byte{[]byte("PersonalInfo")},
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
	t.Run("different entities by OnTypeNames", func(t *testing.T) {
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
		assert.Equal(t, expected, merged)
	})

	t.Run("same entity disjoint fields", func(t *testing.T) {
		keyRepresentation := &resolve.Object{
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

		requiresRepresentation := &resolve.Object{
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

		merged := MergeRepresentationVariableNodes([]*resolve.Object{keyRepresentation, requiresRepresentation})
		assert.Equal(t, expected, merged)
	})

	t.Run("overlapping nested fields are merged", func(t *testing.T) {
		first := &resolve.Object{
			Fields: []*resolve.Field{
				{
					Name: []byte("info"),
					Value: &resolve.Object{
						Path: []string{"info"},
						Fields: []*resolve.Field{
							{
								Name: []byte("kind"),
								Value: &resolve.String{
									Path: []string{"kind"},
								},
							},
						},
					},
					OnTypeNames: [][]byte{[]byte("User")},
				},
			},
		}

		second := &resolve.Object{
			Fields: []*resolve.Field{
				{
					Name: []byte("info"),
					Value: &resolve.Object{
						Path: []string{"info"},
						Fields: []*resolve.Field{
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
			},
		}

		expected := &resolve.Object{
			Nullable: true,
			Fields: []*resolve.Field{
				{
					Name: []byte("info"),
					Value: &resolve.Object{
						Path: []string{"info"},
						Fields: []*resolve.Field{
							{
								Name: []byte("kind"),
								Value: &resolve.String{
									Path: []string{"kind"},
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
			},
		}

		merged := MergeRepresentationVariableNodes([]*resolve.Object{first, second})
		assert.Equal(t, expected, merged)
	})

	t.Run("overlapping array fields are merged", func(t *testing.T) {
		first := &resolve.Object{
			Fields: []*resolve.Field{
				{
					Name: []byte("items"),
					Value: &resolve.Array{
						Path: []string{"items"},
						Item: &resolve.Object{
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
					OnTypeNames: [][]byte{[]byte("User")},
				},
			},
		}

		second := &resolve.Object{
			Fields: []*resolve.Field{
				{
					Name: []byte("items"),
					Value: &resolve.Array{
						Path: []string{"items"},
						Item: &resolve.Object{
							Fields: []*resolve.Field{
								{
									Name: []byte("name"),
									Value: &resolve.String{
										Path: []string{"name"},
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
					Name: []byte("items"),
					Value: &resolve.Array{
						Path: []string{"items"},
						Item: &resolve.Object{
							Fields: []*resolve.Field{
								{
									Name: []byte("id"),
									Value: &resolve.String{
										Path: []string{"id"},
									},
								},
								{
									Name: []byte("name"),
									Value: &resolve.String{
										Path: []string{"name"},
									},
								},
							},
						},
					},
					OnTypeNames: [][]byte{[]byte("User")},
				},
			},
		}

		merged := MergeRepresentationVariableNodes([]*resolve.Object{first, second})
		assert.Equal(t, expected, merged)
	})
}
