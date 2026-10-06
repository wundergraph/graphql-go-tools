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
	}{
		{name: "objects"},
		{name: "category_list", categoryArray: true},
		{name: "owner_list", ownerArray: true},
		{name: "deep_objects", deep: true},
	}
	selections := []struct {
		name     string
		distinct bool
	}{
		{name: "shared_fields"},
		{name: "distinct_fields", distinct: true},
	}
	orders := []struct {
		name  string
		types [2]string
	}{
		{name: "ProductB_first", types: [2]string{"ProductB", "ProductA"}},
		{name: "ProductA_first", types: [2]string{"ProductA", "ProductB"}},
	}
	runtimeTypes := []struct {
		name string
	}{
		{name: "ProductA"},
		{name: "ProductB"},
		{name: "ProductC"},
	}

	// ProductC must not receive owner at all. For A and B,
	// type-specific leaves must not leak from the other fragment.
	for _, shape := range cases {
		for _, selection := range selections {
			for _, order := range orders {
				for _, runtimeType := range runtimeTypes {
					name := fmt.Sprintf("%s/%s/%s/%s", shape.name, selection.name, order.name, runtimeType.name)
					t.Run(name, func(t *testing.T) {
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
						categoryFields := []*resolve.Field{objectField("category", shape.categoryArray, stringField("id"))}
						for _, fragmentType := range order.types {
							ownerFields := []*resolve.Field{stringField("name")}
							if selection.distinct {
								if fragmentType == "ProductA" {
									ownerFields = append(ownerFields, stringField("aOnly"))
								} else {
									ownerFields = append(ownerFields, stringField("bOnly"))
								}
							}
							if shape.deep {
								ownerFields = []*resolve.Field{objectField("profile", false, ownerFields...)}
							}
							category := objectField("category", shape.categoryArray, objectField("owner", shape.ownerArray, ownerFields...))
							category.OnTypeNames = [][]byte{[]byte(fragmentType)}
							categoryFields = append(categoryFields, category)
						}
						response := &resolve.Object{Fields: []*resolve.Field{objectField("product", false, categoryFields...)}}

						wrapOwner := func(owner string) string {
							if shape.deep {
								owner = `{"profile":` + owner + `}`
							}
							if shape.ownerArray {
								owner = `[` + owner + `]`
							}
							return owner
						}
						wrapCategory := func(category string) string {
							if shape.categoryArray {
								return `[` + category + `]`
							}
							return category
						}
						// Include every leaf upstream, so assertions detect both
						// missing requested fields and incorrectly included fields.
						inputOwner := wrapOwner(`{"name":"owner-name","aOnly":"a","bOnly":"b"}`)
						inputCategory := wrapCategory(`{"id":"category-id","owner":` + inputOwner + `}`)
						input := fmt.Sprintf(`{"product":{"__typename":%q,"category":%s}}`, runtimeType.name, inputCategory)
						wantCategory := `{"id":"category-id"}`
						if runtimeType.name != "ProductC" {
							wantOwner := `{"name":"owner-name"}`
							if selection.distinct && runtimeType.name == "ProductA" {
								wantOwner = `{"name":"owner-name","aOnly":"a"}`
							} else if selection.distinct {
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
		}
	}
}
