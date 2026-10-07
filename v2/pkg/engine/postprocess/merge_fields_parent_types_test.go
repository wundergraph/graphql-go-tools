package postprocess

import (
	"bytes"
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

func TestMergeFields_ParentTypeConditions(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		array bool
		types [2]string
	}{
		{name: "object/A_first", types: [2]string{"ProductA", "ProductB"}},
		{name: "object/B_first", types: [2]string{"ProductB", "ProductA"}},
		{name: "array/A_first", array: true, types: [2]string{"ProductA", "ProductB"}},
		{name: "array/B_first", array: true, types: [2]string{"ProductB", "ProductA"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// ROUTER-789: after category selections merge, owner appears twice
			// with different parent type restrictions. Neither may be lost.
			category := &resolve.Object{}
			for _, typeName := range tc.types {
				owner := &resolve.Object{Fields: []*resolve.Field{{
					Name:              []byte(typeName),
					Value:             &resolve.String{},
					ParentOnTypeNames: []resolve.ParentOnTypeNames{{Depth: 2, Names: [][]byte{[]byte(typeName)}}},
				}}}
				field := &resolve.Field{
					Name:              []byte("owner"),
					Value:             owner,
					ParentOnTypeNames: []resolve.ParentOnTypeNames{{Depth: 1, Names: [][]byte{[]byte(typeName)}}},
				}
				if tc.array {
					field.Value = &resolve.Array{Item: owner}
				}
				category.Fields = append(category.Fields, field)
			}

			(&mergeFields{}).Process(category)

			require.Len(t, category.Fields, 1)
			merged := category.Fields[0]
			require.Len(t, merged.ParentOnTypeNames, 1)
			require.Equal(t, 1, merged.ParentOnTypeNames[0].Depth)
			require.ElementsMatch(t, [][]byte{[]byte("ProductA"), []byte("ProductB")}, merged.ParentOnTypeNames[0].Names)

			value := merged.Value
			if tc.array {
				value = value.(*resolve.Array).Item
			}
			children := value.(*resolve.Object).Fields
			require.Len(t, children, 2)
			for i, typeName := range tc.types {
				// Merging the parent must not expose one type's leaves to the other.
				require.Equal(t, []byte(typeName), children[i].Name)
				require.Equal(t, []resolve.ParentOnTypeNames{{Depth: 2, Names: [][]byte{[]byte(typeName)}}}, children[i].ParentOnTypeNames)
			}
		})
	}
}

func TestMergeFields_ParentTypeConditionAlternatives(t *testing.T) {
	t.Parallel()
	condition := func(depth int, name string) resolve.ParentOnTypeNames {
		return resolve.ParentOnTypeNames{Depth: depth, Names: [][]byte{[]byte(name)}}
	}
	cases := []struct {
		name        string
		left, right []resolve.ParentOnTypeNames
		want        [4]string // OuterA/ProductA, OuterA/ProductB, OuterB/ProductA, OuterB/ProductB
	}{
		{name: "unrestricted", right: []resolve.ParentOnTypeNames{condition(1, "ProductA")},
			want: [4]string{`{"left":"L","right":"R"}`, `{"left":"L"}`, `{"left":"L","right":"R"}`, `{"left":"L"}`}},
		{name: "different_depths", left: []resolve.ParentOnTypeNames{condition(1, "ProductA")}, right: []resolve.ParentOnTypeNames{condition(2, "OuterA")},
			want: [4]string{`{"left":"L","right":"R"}`, `{"right":"R"}`, `{"left":"L"}`, ``}},
		{name: "correlated_depths", left: []resolve.ParentOnTypeNames{condition(1, "ProductA"), condition(2, "OuterA")}, right: []resolve.ParentOnTypeNames{condition(1, "ProductB"), condition(2, "OuterB")},
			want: [4]string{`{"left":"L"}`, ``, ``, `{"right":"R"}`}},
	}
	shapes := []struct {
		name          string
		array, scalar bool
	}{{name: "object"}, {name: "array", array: true}, {name: "scalar", scalar: true}}
	orders := []struct {
		name    string
		reverse bool
	}{{name: "left_first"}, {name: "right_first", reverse: true}}
	runtimeTypes := []struct{ outer, product string }{{"OuterA", "ProductA"}, {"OuterA", "ProductB"}, {"OuterB", "ProductA"}, {"OuterB", "ProductB"}}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			for _, shape := range shapes {
				t.Run(shape.name, func(t *testing.T) {
					t.Parallel()
					for _, order := range orders {
						t.Run(order.name, func(t *testing.T) {
							t.Parallel()
							for i, runtime := range runtimeTypes {
								t.Run(runtime.outer+"/"+runtime.product, func(t *testing.T) {
									t.Parallel()
									category := &resolve.Object{Path: []string{"category"}}
									for j, parents := range [][]resolve.ParentOnTypeNames{tc.left, tc.right} {
										leafName := []string{"left", "right"}[j]
										leaf := &resolve.Field{Name: []byte(leafName), Value: &resolve.String{Path: []string{leafName}}}
										for _, parent := range parents {
											leaf.ParentOnTypeNames = append(leaf.ParentOnTypeNames, condition(parent.Depth+1, string(parent.Names[0])))
										}
										owner := &resolve.Object{Path: []string{"owner"}, Fields: []*resolve.Field{leaf}}
										field := &resolve.Field{Name: []byte("owner"), Value: owner, OnTypeNames: [][]byte{[]byte("Category")}, ParentOnTypeNames: slices.Clone(parents)}
										if shape.array {
											owner.Path = nil
											field.Value = &resolve.Array{Path: []string{"owner"}, Item: owner}
										}
										if shape.scalar {
											field.Value = &resolve.String{Path: []string{"owner"}}
										}
										category.Fields = append(category.Fields, field)
									}
									if order.reverse {
										category.Fields[0], category.Fields[1] = category.Fields[1], category.Fields[0]
									}
									wrap := func(name string, value resolve.Node) *resolve.Object {
										return &resolve.Object{Fields: []*resolve.Field{{Name: []byte(name), Value: value}}}
									}
									product := wrap("category", category)
									product.Path = []string{"product"}
									outer := wrap("product", product)
									outer.Path = []string{"outer"}
									response := wrap("outer", outer)
									(&mergeFields{}).Process(response)
									require.Len(t, category.Fields, 1)
									// Copies must preserve the merged alternatives and child restrictions.
									response = response.Copy().(*resolve.Object)

									inputOwner, wantOwner := `{"left":"L","right":"R"}`, tc.want[i]
									if shape.scalar {
										inputOwner = `"name"`
										if wantOwner != "" {
											wantOwner = inputOwner
										}
									}
									if shape.array {
										inputOwner = "[" + inputOwner + "]"
										if wantOwner != "" {
											wantOwner = "[" + wantOwner + "]"
										}
									}
									input := `{"outer":{"__typename":"` + runtime.outer + `","product":{"__typename":"` + runtime.product + `","category":{"__typename":"Category","owner":` + inputOwner + `}}}}`
									wantCategory := `{}`
									if wantOwner != "" {
										wantCategory = `{"owner":` + wantOwner + `}`
									}
									r := resolve.NewResolvable(nil, resolve.ResolvableOptions{})
									require.NoError(t, r.Init(&resolve.Context{}, []byte(input), ast.OperationTypeQuery))
									var out bytes.Buffer
									require.NoError(t, r.Resolve(context.Background(), response, nil, &out))
									require.JSONEq(t, `{"data":{"outer":{"product":{"category":`+wantCategory+`}}}}`, out.String())
									require.LessOrEqual(t, strings.Count(out.String(), `"owner":`), 1)
								})
							}
						})
					}
				})
			}
		})
	}
}
