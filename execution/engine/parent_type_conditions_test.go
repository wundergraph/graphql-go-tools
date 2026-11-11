package engine_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/execution/federationtesting"
)

// Each query selects the same nested field through several fragment paths. The accounts subgraph
// serves someNestedInterfaces with SomeNestedType1 and SomeNestedType2, each holding otherInterfaces
// with SomeType1, SomeType2 and SomeType3, each holding someObject { a b c }. Values are prefixed
// with the nested type index: 1.A, 1.AA, 1.AAA for SomeNestedType1, 2.A, 2.AA, 2.AAA for SomeNestedType2.
func TestParentTypeConditions(t *testing.T) {
	t.Parallel()

	setup, err := federationtesting.NewFederationSetup(addGateway(false))
	require.NoError(t, err)
	t.Cleanup(setup.Close)
	gqlClient := NewGraphqlClient(http.DefaultClient)

	cases := []struct {
		name     string
		query    string
		expected string
	}{
		{
			name:  "[E1] non-matching fragment first",
			query: "e01_non_matching_fragment_first.graphql",
			expected: `{
				"data": {
					"someNestedInterfaces": [
						{
							"__typename": "SomeNestedType1",
							"otherInterfaces": [
								{
									"__typename": "SomeType1",
									"someObject": {"a": "1.A"}
								},
								{
									"__typename": "SomeType2",
									"someObject": {"a": "1.AA"}
								},
								{
									"__typename": "SomeType3",
									"someObject": {"a": "1.AAA"}
								}
							]
						},
						{
							"__typename": "SomeNestedType2",
							"otherInterfaces": [
								{
									"__typename": "SomeType1",
									"someObject": {"a": "2.A"}
								},
								{
									"__typename": "SomeType2",
									"someObject": {"a": "2.AA"}
								},
								{
									"__typename": "SomeType3",
									"someObject": {"a": "2.AAA"}
								}
							]
						}
					]
				}
			}`,
		},
		{
			name:  "[E2] unrestricted sibling object plus restricted sibling",
			query: "e02_unrestricted_sibling.graphql",
			expected: `{
				"data": {
					"someNestedInterfaces": [
						{
							"__typename": "SomeNestedType1",
							"otherInterfaces": [
								{
									"__typename": "SomeType1",
									"someObject": {"a": "1.A", "b": "1.B"}
								},
								{
									"__typename": "SomeType2",
									"someObject": {"a": "1.AA", "b": "1.BB"}
								},
								{
									"__typename": "SomeType3",
									"someObject": {"a": "1.AAA", "b": "1.BBB"}
								}
							]
						},
						{
							"__typename": "SomeNestedType2",
							"otherInterfaces": [
								{
									"__typename": "SomeType1",
									"someObject": {"a": "2.A"}
								},
								{
									"__typename": "SomeType2",
									"someObject": {"a": "2.AA"}
								},
								{
									"__typename": "SomeType3",
									"someObject": {"a": "2.AAA"}
								}
							]
						}
					]
				}
			}`,
		},
		{
			name:  "[E3] interface field under a fragment with an inline fragment inside",
			query: "e03_interface_under_fragment.graphql",
			expected: `{
				"data": {
					"someNestedInterfaces": [
						{
							"__typename": "SomeNestedType1",
							"otherInterfaces": [
								{
									"__typename": "SomeType1",
									"someObject": {"a": "1.A", "b": "1.B"}
								},
								{
									"__typename": "SomeType2",
									"someObject": {"a": "1.AA"}
								},
								{
									"__typename": "SomeType3",
									"someObject": {"a": "1.AAA"}
								}
							]
						},
						{"__typename": "SomeNestedType2"}
					]
				}
			}`,
		},
		{
			name:  "[E6] three paths select c",
			query: "e06_three_paths.graphql",
			expected: `{
				"data": {
					"someNestedInterfaces": [
						{
							"__typename": "SomeNestedType1",
							"otherInterfaces": [
								{
									"__typename": "SomeType1",
									"someObject": {"c": "1.C"}
								},
								{
									"__typename": "SomeType2",
									"someObject": {"c": "1.CC"}
								},
								{
									"__typename": "SomeType3",
									"someObject": {"c": "1.CCC"}
								}
							]
						},
						{
							"__typename": "SomeNestedType2",
							"otherInterfaces": [
								{"__typename": "SomeType1"},
								{
									"__typename": "SomeType2",
									"someObject": {"c": "2.CC"}
								},
								{
									"__typename": "SomeType3",
									"someObject": {"c": "2.CCC"}
								}
							]
						}
					]
				}
			}`,
		},
		{
			name:  "[E7] nested objects under union member fragments",
			query: "e07_union_members.graphql",
			expected: `{
				"data": {
					"histories": [
						{
							"__typename": "Purchase",
							"product": {"upc": "top-1"},
							"quantity": 1
						},
						{
							"__typename": "Sale",
							"product": {"upc": "top-1"},
							"rating": 1
						},
						{
							"__typename": "Purchase",
							"product": {"upc": "top-2"},
							"quantity": 2
						},
						{
							"__typename": "Sale",
							"product": {"upc": "top-2"},
							"rating": 2
						},
						{
							"__typename": "Purchase",
							"product": {"upc": "top-3"},
							"quantity": 3
						},
						{
							"__typename": "Sale",
							"product": {"upc": "top-3"},
							"rating": 3
						}
					]
				}
			}`,
		},
		{
			name:  "[E8] named fragment on the interface wraps the inline fragments",
			query: "e08_named_fragment.graphql",
			expected: `{
				"data": {
					"someNestedInterfaces": [
						{
							"__typename": "SomeNestedType1",
							"otherInterfaces": [
								{
									"__typename": "SomeType1",
									"someObject": {"a": "1.A"}
								},
								{
									"__typename": "SomeType2",
									"someObject": {"a": "1.AA"}
								},
								{
									"__typename": "SomeType3",
									"someObject": {"a": "1.AAA"}
								}
							]
						},
						{
							"__typename": "SomeNestedType2",
							"otherInterfaces": [
								{
									"__typename": "SomeType1",
									"someObject": {"a": "2.A"}
								},
								{
									"__typename": "SomeType2",
									"someObject": {"a": "2.AA"}
								},
								{
									"__typename": "SomeType3",
									"someObject": {"a": "2.AAA"}
								}
							]
						}
					]
				}
			}`,
		},
		{
			name:  "[E9] scalar selected at two depths",
			query: "e09_scalar_two_depths.graphql",
			expected: `{
				"data": {
					"someNestedInterfaces": [
						{
							"__typename": "SomeNestedType1",
							"otherInterfaces": [
								{
									"__typename": "SomeType1",
									"someObject": {"a": "1.A"}
								},
								{
									"__typename": "SomeType2",
									"someObject": {"a": "1.AA"}
								},
								{
									"__typename": "SomeType3",
									"someObject": {"a": "1.AAA"}
								}
							]
						},
						{
							"__typename": "SomeNestedType2",
							"otherInterfaces": [
								{"__typename": "SomeType1"},
								{
									"__typename": "SomeType2",
									"someObject": {"a": "2.AA"}
								},
								{"__typename": "SomeType3"}
							]
						}
					]
				}
			}`,
		},
		{
			name:  "[E10] unrestricted sibling object, restricted selection first",
			query: "e10_unrestricted_sibling_reversed.graphql",
			expected: `{
				"data": {
					"someNestedInterfaces": [
						{
							"__typename": "SomeNestedType1",
							"otherInterfaces": [
								{
									"__typename": "SomeType1",
									"someObject": {"a": "1.A", "b": "1.B"}
								},
								{
									"__typename": "SomeType2",
									"someObject": {"a": "1.AA", "b": "1.BB"}
								},
								{
									"__typename": "SomeType3",
									"someObject": {"a": "1.AAA", "b": "1.BBB"}
								}
							]
						},
						{
							"__typename": "SomeNestedType2",
							"otherInterfaces": [
								{
									"__typename": "SomeType1",
									"someObject": {"a": "2.A"}
								},
								{
									"__typename": "SomeType2",
									"someObject": {"a": "2.AA"}
								},
								{
									"__typename": "SomeType3",
									"someObject": {"a": "2.AAA"}
								}
							]
						}
					]
				}
			}`,
		},
		{
			name:  "[E11] alias on the merged field",
			query: "e11_alias.graphql",
			expected: `{
				"data": {
					"someNestedInterfaces": [
						{
							"__typename": "SomeNestedType1",
							"otherInterfaces": [
								{
									"__typename": "SomeType1",
									"obj": {"a": "1.A"}
								},
								{
									"__typename": "SomeType2",
									"obj": {"a": "1.AA"}
								},
								{
									"__typename": "SomeType3",
									"obj": {"a": "1.AAA"}
								}
							]
						},
						{
							"__typename": "SomeNestedType2",
							"otherInterfaces": [
								{
									"__typename": "SomeType1",
									"obj": {"a": "2.A"}
								},
								{
									"__typename": "SomeType2",
									"obj": {"a": "2.AA"}
								},
								{
									"__typename": "SomeType3",
									"obj": {"a": "2.AAA"}
								}
							]
						}
					]
				}
			}`,
		},
		{
			name:  "[E12] concrete parent with fragments on its own type and its interface",
			query: "e12_concrete_parent.graphql",
			expected: `{
				"data": {
					"me": {
						"username": "Me",
						"history": [
							{"__typename": "Purchase"},
							{"__typename": "Sale"},
							{"__typename": "Purchase"}
						],
						"id": "1234"
					}
				}
			}`,
		},
		{
			name:  "[E13] three fragments at one depth",
			query: "e13_three_fragments_one_depth.graphql",
			expected: `{
				"data": {
					"someNestedInterfaces": [
						{
							"__typename": "SomeNestedType1",
							"otherInterfaces": [
								{
									"__typename": "SomeType1",
									"someObject": {"a": "1.A"}
								},
								{
									"__typename": "SomeType2",
									"someObject": {"a": "1.AA"}
								},
								{
									"__typename": "SomeType3",
									"someObject": {"a": "1.AAA"}
								}
							]
						},
						{"__typename": "SomeNestedType2"}
					]
				}
			}`,
		},
		{
			name:  "[E14] the same type in two fragments",
			query: "e14_same_type_twice.graphql",
			expected: `{
				"data": {
					"otherInterfaces": [
						{
							"__typename": "SomeType1",
							"someObject": {"a": "A", "b": "B"}
						},
						{"__typename": "SomeType2"},
						{"__typename": "SomeType3"}
					]
				}
			}`,
		},
		{
			name:  "[E15] fragment on the interface itself",
			query: "e15_fragment_on_interface_itself.graphql",
			expected: `{
				"data": {
					"otherInterfaces": [
						{
							"__typename": "SomeType1",
							"someObject": {"a": "A"}
						},
						{
							"__typename": "SomeType2",
							"someObject": {"a": "AA"}
						},
						{
							"__typename": "SomeType3",
							"someObject": {"a": "AAA"}
						}
					]
				}
			}`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var expected bytes.Buffer
			require.NoError(t, json.Compact(&expected, []byte(tc.expected)))
			resp := gqlClient.Query(t.Context(), setup.GatewayServer.URL, testQueryPath("queries/parent_type_conditions/"+tc.query), nil, t)
			assert.Equal(t, expected.String(), string(resp))
		})
	}
}
