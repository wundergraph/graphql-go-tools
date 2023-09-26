package graphql_datasource

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/astparser"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

func TestBuildRepresentationVariableNode(t *testing.T) {
	runTest := func(t *testing.T, definitionStr, keyStr string, addTypename, addOnType bool, expectedNode resolve.Node) {
		definition, _ := astparser.ParseGraphqlDocumentString(definitionStr)
		cfg := plan.FederationFieldConfiguration{
			TypeName:     "User",
			SelectionSet: keyStr,
		}

		node, err := buildRepresentationVariableNode(cfg, &definition, addTypename, addOnType)
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
		`, `id name`,
			false,
			false,
			&resolve.Object{
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
				
		`, `id name account { accoundID address(home: true) { zip } }`,
			true,
			true,
			&resolve.Object{
				Fields: []*resolve.Field{
					{
						Name: []byte("__typename"),
						Value: &resolve.String{
							Path: []string{"__typename"},
						},
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
		Fields: []*resolve.Field{
			{
				Name: []byte("__typename"),
				Value: &resolve.String{
					Path: []string{"__typename"},
				},
			},
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
}
