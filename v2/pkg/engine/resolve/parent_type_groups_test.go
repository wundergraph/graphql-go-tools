package resolve

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
)

// The leaf c sits in b, which sits in a. Depth 0 from c is b, depth 1 is a.
// Each row sets the groups on c and the runtime __typename of a and b, and states whether c renders.
func TestResolvable_ParentTypeGroups(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		groups  [][]ParentOnTypeNames
		a, b    string // runtime __typename of a and b, empty for none
		renders bool
	}{
		{
			name:    "no groups renders",
			a:       "A1",
			b:       "B1",
			renders: true,
		},
		{
			name: "one group, one depth matches",
			groups: [][]ParentOnTypeNames{
				{{Depth: 1, Names: [][]byte{[]byte("A1")}}},
			},
			a:       "A1",
			b:       "B1",
			renders: true,
		},
		{
			name: "one group, one depth mismatches",
			groups: [][]ParentOnTypeNames{
				{{Depth: 1, Names: [][]byte{[]byte("A1")}}},
			},
			a:       "A2",
			b:       "B1",
			renders: false,
		},
		{
			name: "one group, both depths must match",
			groups: [][]ParentOnTypeNames{
				{
					{Depth: 1, Names: [][]byte{[]byte("A1")}},
					{Depth: 0, Names: [][]byte{[]byte("B1")}},
				},
			},
			a:       "A1",
			b:       "B2",
			renders: false,
		},
		{
			name: "second group matches",
			groups: [][]ParentOnTypeNames{
				{{Depth: 1, Names: [][]byte{[]byte("A1")}}},
				{{Depth: 0, Names: [][]byte{[]byte("B2")}}},
			},
			a:       "A2",
			b:       "B2",
			renders: true,
		},
		{
			name: "no group matches",
			groups: [][]ParentOnTypeNames{
				{{Depth: 1, Names: [][]byte{[]byte("A1")}}},
				{{Depth: 0, Names: [][]byte{[]byte("B1")}}},
			},
			a:       "A2",
			b:       "B2",
			renders: false,
		},
		{
			name: "one of several names matches",
			groups: [][]ParentOnTypeNames{
				{{Depth: 0, Names: [][]byte{[]byte("B1"), []byte("B2")}}},
			},
			a:       "A2",
			b:       "B2",
			renders: true,
		},
		{
			name: "missing __typename at a depth skips",
			groups: [][]ParentOnTypeNames{
				{{Depth: 1, Names: [][]byte{[]byte("A1")}}},
			},
			a:       "",
			b:       "B1",
			renders: false,
		},
		{
			name: "missing __typename skips even with a second group",
			groups: [][]ParentOnTypeNames{
				{{Depth: 1, Names: [][]byte{[]byte("A1")}}},
				{{Depth: 1, Names: [][]byte{[]byte("A2")}}},
			},
			a:       "",
			b:       "B1",
			renders: false,
		},
		{
			name: "depth 0 is the containing object",
			groups: [][]ParentOnTypeNames{
				{{Depth: 0, Names: [][]byte{[]byte("B1")}}},
			},
			a:       "A1",
			b:       "B1",
			renders: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			root := &Object{
				Fields: []*Field{
					{
						Name: []byte("a"),
						Value: &Object{
							Path: []string{"a"},
							Fields: []*Field{
								{
									Name: []byte("b"),
									Value: &Object{
										Path: []string{"b"},
										Fields: []*Field{
											{
												Name:              []byte("c"),
												Value:             &String{Path: []string{"c"}},
												ParentOnTypeNames: tc.groups,
											},
										},
									},
								},
							},
						},
					},
				},
			}
			typename := func(name string) string {
				if name == "" {
					return ""
				}
				return `"__typename": "` + name + `",`
			}
			input := `{
				"a": {` + typename(tc.a) + `
					"b": {` + typename(tc.b) + `
						"c": "v"
					}
				}
			}`
			want := `{
				"data": {
					"a": {
						"b": {}
					}
				}
			}`
			if tc.renders {
				want = `{
					"data": {
						"a": {
							"b": {"c": "v"}
						}
					}
				}`
			}
			var compactWant bytes.Buffer
			require.NoError(t, json.Compact(&compactWant, []byte(want)))

			r := NewResolvable(nil, ResolvableOptions{})
			require.NoError(t, r.Init(&Context{}, []byte(input), ast.OperationTypeQuery))
			var out bytes.Buffer
			require.NoError(t, r.Resolve(t.Context(), root, nil, &out))
			assert.Equal(t, compactWant.String(), out.String())
		})
	}
}
