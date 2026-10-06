package postprocess

import (
	"bytes"
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

// Regression for ROUTER-789 / cosmo#2346: a field selected outside a fragment
// merges with selections on two concrete types. Nested objects must retain both
// parent type conditions, regardless of which fragment appears first.
func TestMergeFields_ParentTypeConditions(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name          string
		categoryArray bool
		ownerArray    bool
		deep          bool
		distinct      bool
		order         [2]string
		typeName      string
	}{
		{name: "objects/distinct_fields=false/ProductB_first/ProductA", order: [2]string{"ProductB", "ProductA"}, typeName: "ProductA"},
		{name: "objects/distinct_fields=false/ProductB_first/ProductB", order: [2]string{"ProductB", "ProductA"}, typeName: "ProductB"},
		{name: "objects/distinct_fields=false/ProductB_first/ProductC", order: [2]string{"ProductB", "ProductA"}, typeName: "ProductC"},
		{name: "objects/distinct_fields=false/ProductA_first/ProductA", order: [2]string{"ProductA", "ProductB"}, typeName: "ProductA"},
		{name: "objects/distinct_fields=false/ProductA_first/ProductB", order: [2]string{"ProductA", "ProductB"}, typeName: "ProductB"},
		{name: "objects/distinct_fields=false/ProductA_first/ProductC", order: [2]string{"ProductA", "ProductB"}, typeName: "ProductC"},

		{name: "objects/distinct_fields=true/ProductB_first/ProductA", distinct: true, order: [2]string{"ProductB", "ProductA"}, typeName: "ProductA"},
		{name: "objects/distinct_fields=true/ProductB_first/ProductB", distinct: true, order: [2]string{"ProductB", "ProductA"}, typeName: "ProductB"},
		{name: "objects/distinct_fields=true/ProductB_first/ProductC", distinct: true, order: [2]string{"ProductB", "ProductA"}, typeName: "ProductC"},
		{name: "objects/distinct_fields=true/ProductA_first/ProductA", distinct: true, order: [2]string{"ProductA", "ProductB"}, typeName: "ProductA"},
		{name: "objects/distinct_fields=true/ProductA_first/ProductB", distinct: true, order: [2]string{"ProductA", "ProductB"}, typeName: "ProductB"},
		{name: "objects/distinct_fields=true/ProductA_first/ProductC", distinct: true, order: [2]string{"ProductA", "ProductB"}, typeName: "ProductC"},

		{name: "category_list/distinct_fields=false/ProductB_first/ProductA", categoryArray: true, order: [2]string{"ProductB", "ProductA"}, typeName: "ProductA"},
		{name: "category_list/distinct_fields=false/ProductB_first/ProductB", categoryArray: true, order: [2]string{"ProductB", "ProductA"}, typeName: "ProductB"},
		{name: "category_list/distinct_fields=false/ProductB_first/ProductC", categoryArray: true, order: [2]string{"ProductB", "ProductA"}, typeName: "ProductC"},
		{name: "category_list/distinct_fields=false/ProductA_first/ProductA", categoryArray: true, order: [2]string{"ProductA", "ProductB"}, typeName: "ProductA"},
		{name: "category_list/distinct_fields=false/ProductA_first/ProductB", categoryArray: true, order: [2]string{"ProductA", "ProductB"}, typeName: "ProductB"},
		{name: "category_list/distinct_fields=false/ProductA_first/ProductC", categoryArray: true, order: [2]string{"ProductA", "ProductB"}, typeName: "ProductC"},

		{name: "category_list/distinct_fields=true/ProductB_first/ProductA", categoryArray: true, distinct: true, order: [2]string{"ProductB", "ProductA"}, typeName: "ProductA"},
		{name: "category_list/distinct_fields=true/ProductB_first/ProductB", categoryArray: true, distinct: true, order: [2]string{"ProductB", "ProductA"}, typeName: "ProductB"},
		{name: "category_list/distinct_fields=true/ProductB_first/ProductC", categoryArray: true, distinct: true, order: [2]string{"ProductB", "ProductA"}, typeName: "ProductC"},
		{name: "category_list/distinct_fields=true/ProductA_first/ProductA", categoryArray: true, distinct: true, order: [2]string{"ProductA", "ProductB"}, typeName: "ProductA"},
		{name: "category_list/distinct_fields=true/ProductA_first/ProductB", categoryArray: true, distinct: true, order: [2]string{"ProductA", "ProductB"}, typeName: "ProductB"},
		{name: "category_list/distinct_fields=true/ProductA_first/ProductC", categoryArray: true, distinct: true, order: [2]string{"ProductA", "ProductB"}, typeName: "ProductC"},

		{name: "owner_list/distinct_fields=false/ProductB_first/ProductA", ownerArray: true, order: [2]string{"ProductB", "ProductA"}, typeName: "ProductA"},
		{name: "owner_list/distinct_fields=false/ProductB_first/ProductB", ownerArray: true, order: [2]string{"ProductB", "ProductA"}, typeName: "ProductB"},
		{name: "owner_list/distinct_fields=false/ProductB_first/ProductC", ownerArray: true, order: [2]string{"ProductB", "ProductA"}, typeName: "ProductC"},
		{name: "owner_list/distinct_fields=false/ProductA_first/ProductA", ownerArray: true, order: [2]string{"ProductA", "ProductB"}, typeName: "ProductA"},
		{name: "owner_list/distinct_fields=false/ProductA_first/ProductB", ownerArray: true, order: [2]string{"ProductA", "ProductB"}, typeName: "ProductB"},
		{name: "owner_list/distinct_fields=false/ProductA_first/ProductC", ownerArray: true, order: [2]string{"ProductA", "ProductB"}, typeName: "ProductC"},

		{name: "owner_list/distinct_fields=true/ProductB_first/ProductA", ownerArray: true, distinct: true, order: [2]string{"ProductB", "ProductA"}, typeName: "ProductA"},
		{name: "owner_list/distinct_fields=true/ProductB_first/ProductB", ownerArray: true, distinct: true, order: [2]string{"ProductB", "ProductA"}, typeName: "ProductB"},
		{name: "owner_list/distinct_fields=true/ProductB_first/ProductC", ownerArray: true, distinct: true, order: [2]string{"ProductB", "ProductA"}, typeName: "ProductC"},
		{name: "owner_list/distinct_fields=true/ProductA_first/ProductA", ownerArray: true, distinct: true, order: [2]string{"ProductA", "ProductB"}, typeName: "ProductA"},
		{name: "owner_list/distinct_fields=true/ProductA_first/ProductB", ownerArray: true, distinct: true, order: [2]string{"ProductA", "ProductB"}, typeName: "ProductB"},
		{name: "owner_list/distinct_fields=true/ProductA_first/ProductC", ownerArray: true, distinct: true, order: [2]string{"ProductA", "ProductB"}, typeName: "ProductC"},

		{name: "deep_objects/distinct_fields=false/ProductB_first/ProductA", deep: true, order: [2]string{"ProductB", "ProductA"}, typeName: "ProductA"},
		{name: "deep_objects/distinct_fields=false/ProductB_first/ProductB", deep: true, order: [2]string{"ProductB", "ProductA"}, typeName: "ProductB"},
		{name: "deep_objects/distinct_fields=false/ProductB_first/ProductC", deep: true, order: [2]string{"ProductB", "ProductA"}, typeName: "ProductC"},
		{name: "deep_objects/distinct_fields=false/ProductA_first/ProductA", deep: true, order: [2]string{"ProductA", "ProductB"}, typeName: "ProductA"},
		{name: "deep_objects/distinct_fields=false/ProductA_first/ProductB", deep: true, order: [2]string{"ProductA", "ProductB"}, typeName: "ProductB"},
		{name: "deep_objects/distinct_fields=false/ProductA_first/ProductC", deep: true, order: [2]string{"ProductA", "ProductB"}, typeName: "ProductC"},

		{name: "deep_objects/distinct_fields=true/ProductB_first/ProductA", deep: true, distinct: true, order: [2]string{"ProductB", "ProductA"}, typeName: "ProductA"},
		{name: "deep_objects/distinct_fields=true/ProductB_first/ProductB", deep: true, distinct: true, order: [2]string{"ProductB", "ProductA"}, typeName: "ProductB"},
		{name: "deep_objects/distinct_fields=true/ProductB_first/ProductC", deep: true, distinct: true, order: [2]string{"ProductB", "ProductA"}, typeName: "ProductC"},
		{name: "deep_objects/distinct_fields=true/ProductA_first/ProductA", deep: true, distinct: true, order: [2]string{"ProductA", "ProductB"}, typeName: "ProductA"},
		{name: "deep_objects/distinct_fields=true/ProductA_first/ProductB", deep: true, distinct: true, order: [2]string{"ProductA", "ProductB"}, typeName: "ProductB"},
		{name: "deep_objects/distinct_fields=true/ProductA_first/ProductC", deep: true, distinct: true, order: [2]string{"ProductA", "ProductB"}, typeName: "ProductC"},
	}

	// ProductC must not receive owner at all. For A and B,
	// type-specific leaves must not leak from the other fragment.
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			stringField := func(name string) *resolve.Field {
				return &resolve.Field{Name: []byte(name), Value: &resolve.String{Path: []string{name}}}
			}
			objectField := func(name string, array bool, fields ...*resolve.Field) *resolve.Field {
				object := &resolve.Object{Fields: fields}
				if array {
					return &resolve.Field{Name: []byte(name), Value: &resolve.Array{Path: []string{name}, Item: object}}
				}
				object.Path = []string{name}
				return &resolve.Field{Name: []byte(name), Value: object}
			}
			categoryFields := []*resolve.Field{objectField("category", tc.categoryArray, stringField("id"))}
			for _, fragmentType := range tc.order {
				ownerFields := []*resolve.Field{stringField("name")}
				if tc.distinct {
					if fragmentType == "ProductA" {
						ownerFields = append(ownerFields, stringField("aOnly"))
					} else {
						ownerFields = append(ownerFields, stringField("bOnly"))
					}
				}
				if tc.deep {
					ownerFields = []*resolve.Field{objectField("profile", false, ownerFields...)}
				}
				category := objectField("category", tc.categoryArray, objectField("owner", tc.ownerArray, ownerFields...))
				category.OnTypeNames = [][]byte{[]byte(fragmentType)}
				categoryFields = append(categoryFields, category)
			}
			response := &resolve.Object{Fields: []*resolve.Field{objectField("product", false, categoryFields...)}}

			wrapOwner := func(owner string) string {
				if tc.deep {
					owner = `{"profile":` + owner + `}`
				}
				if tc.ownerArray {
					owner = `[` + owner + `]`
				}
				return owner
			}
			wrapCategory := func(category string) string {
				if tc.categoryArray {
					return `[` + category + `]`
				}
				return category
			}
			// Include every leaf upstream, so assertions detect both
			// missing requested fields and incorrectly included fields.
			inputOwner := wrapOwner(`{"name":"owner-name","aOnly":"a","bOnly":"b"}`)
			inputCategory := wrapCategory(`{"id":"category-id","owner":` + inputOwner + `}`)
			input := fmt.Sprintf(`{"product":{"__typename":%q,"category":%s}}`, tc.typeName, inputCategory)
			wantCategory := `{"id":"category-id"}`
			if tc.typeName != "ProductC" {
				wantOwner := `{"name":"owner-name"}`
				if tc.distinct && tc.typeName == "ProductA" {
					wantOwner = `{"name":"owner-name","aOnly":"a"}`
				} else if tc.distinct {
					wantOwner = `{"name":"owner-name","bOnly":"b"}`
				}
				wantCategory = `{"id":"category-id","owner":` + wrapOwner(wantOwner) + `}`
			}
			want := `{"data":{"product":{"category":` + wrapCategory(wantCategory) + `}}}`

			(&mergeFields{}).Process(response)
			resolvable := resolve.NewResolvable(nil, resolve.ResolvableOptions{})
			require.NoError(t, resolvable.Init(&resolve.Context{}, []byte(input), ast.OperationTypeQuery))
			var out bytes.Buffer
			require.NoError(t, resolvable.Resolve(context.Background(), response, nil, &out))
			require.JSONEq(t, want, out.String())
		})
	}
}
