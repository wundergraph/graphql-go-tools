package representationvariable_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/astparser"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan/representationvariable"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

func TestBuildRepresentationVariableNode(t *testing.T) {
	tests := []struct {
		name       string
		definition string
		key        string
		federation plan.FederationMetaData
		expected   *resolve.Object
	}{
		{
			name: "single key",
			definition: `
				scalar String

				type User {
					id: String!
				}
			`,
			key:        `id`,
			federation: plan.FederationMetaData{},
			expected: &resolve.Object{
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
				},
			},
		},
		{
			name: "composite key",
			definition: `
				scalar String

				type User {
					a: String!
					b: String!
				}
			`,
			key:        `a b`,
			federation: plan.FederationMetaData{},
			expected: &resolve.Object{
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
						Name: []byte("a"),
						Value: &resolve.String{
							Path: []string{"a"},
						},
						OnTypeNames: [][]byte{[]byte("User")},
					},
					{
						Name: []byte("b"),
						Value: &resolve.String{
							Path: []string{"b"},
						},
						OnTypeNames: [][]byte{[]byte("User")},
					},
				},
			},
		},
		{
			name: "nested object key",
			definition: `
				scalar String

				type User {
					info: UserInfo!
				}

				type UserInfo {
					id: String!
				}
			`,
			key:        `info { id }`,
			federation: plan.FederationMetaData{},
			expected: &resolve.Object{
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
						Name: []byte("info"),
						Value: &resolve.Object{
							Path: []string{"info"},
							Fields: []*resolve.Field{
								{
									Name: []byte("id"),
									Value: &resolve.String{
										Path: []string{"id"},
									},
								},
							},
						},
						OnTypeNames: [][]byte{[]byte("User")},
					},
				},
			},
		},
		{
			name: "interface object key",
			definition: `
				scalar String

				type User {
					id: String!
				}
			`,
			key: `id`,
			federation: plan.FederationMetaData{
				InterfaceObjects: []plan.EntityInterfaceConfiguration{
					{
						InterfaceTypeName: "Account",
						ConcreteTypeNames: []string{"User", "Admin"},
					},
				},
			},
			expected: &resolve.Object{
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
				},
			},
		},
		{
			name: "entity interface key",
			definition: `
				scalar String

				type User {
					id: String!
				}
			`,
			key: `id`,
			federation: plan.FederationMetaData{
				EntityInterfaces: []plan.EntityInterfaceConfiguration{
					{
						InterfaceTypeName: "Account",
						ConcreteTypeNames: []string{"User", "Admin"},
					},
				},
			},
			expected: &resolve.Object{
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
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			definition, report := astparser.ParseGraphqlDocumentString(tt.definition)
			require.False(t, report.HasErrors())

			cfg := plan.FederationFieldConfiguration{
				TypeName:     "User",
				SelectionSet: tt.key,
			}

			node, err := representationvariable.BuildRepresentationVariableNode(&definition, cfg, tt.federation)
			require.NoError(t, err)

			assert.Equal(t, tt.expected, node)
		})
	}
}
