package postprocess

import (
	"testing"

	"github.com/stretchr/testify/require"

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
