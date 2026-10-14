package astvalidation

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/astparser"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/asttransform"
)

func TestFieldSelectionMergingTypename(t *testing.T) {
	t.Parallel()
	const schema = `schema { query: Query } scalar String scalar Int
  type Query { item: Item result: Result }
  union Result = Item | Other
  enum Kind { VALUE }
  type Item { name: String! child: Item kind: Kind }
  type Other { name: String! nullableName: String number: Int! names: [String!]! child: Item kind: Kind }`
	cases := []struct {
		name, query     string
		strict, relaxed ValidationState
	}{
		{"AliasCollision", `{ item { a: __typename a: name } }`, Invalid, Invalid},
		{"ReverseAliasCollision", `{ item { a: name a: __typename } }`, Invalid, Invalid},
		{"CompositeAliasCollision", `{ item { a: __typename a: child { name } } }`, Invalid, Invalid},
		{"ReverseCompositeAliasCollision", `{ item { a: child { name } a: __typename } }`, Invalid, Invalid},
		{"EnumAliasCollision", `{ item { a: __typename a: kind } }`, Invalid, Invalid},
		{"UnionAliasCollision", `{ result { a: __typename ... on Other { a: name } } }`, Invalid, Invalid},
		{"InheritedExclusivity", `{ result { ... on Item { child { a: __typename } } ... on Other { child { a: name } } } }`, Valid, Valid},
		{"MatchingTypenames", `{ item { a: __typename a: __typename } }`, Valid, Valid},
		{"DisjointMatchingShape", `{ result { ... on Item { a: __typename } ... on Other { a: name } } }`, Valid, Valid},
		{"DisjointNullability", `{ result { ... on Item { a: __typename } ... on Other { a: nullableName } } }`, Invalid, Valid},
		{"DisjointList", `{ result { ... on Item { a: __typename } ... on Other { a: names } } }`, Invalid, Invalid},
		{"DisjointComposite", `{ result { ... on Item { a: __typename } ... on Other { a: child { name } } } }`, Invalid, Invalid},
		{"DisjointEnum", `{ result { ... on Item { a: __typename } ... on Other { a: kind } } }`, Invalid, Invalid},
		{"DisjointDifferentLeaf", `{ result { ... on Item { a: __typename } ... on Other { a: number } } }`, Invalid, Invalid},
	}
	modes := []struct {
		name string
		base bool
	}{{"RawSchema", false}, {"BaseSchema", true}}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			for _, mode := range modes {
				t.Run(mode.name, func(t *testing.T) {
					t.Parallel()
					definition, report := astparser.ParseGraphqlDocumentString(schema)
					require.False(t, report.HasErrors(), report.Error())
					if mode.base {
						require.NoError(t, asttransform.MergeDefinitionWithBaseSchema(&definition))
					}
					operation, report := astparser.ParseGraphqlDocumentString(tc.query)
					require.False(t, report.HasErrors(), report.Error())
					assert.Equal(t, tc.strict, DefaultOperationValidator().Validate(&operation, &definition, &report), report.Error())
					report.Reset()
					assert.Equal(t, tc.relaxed, DefaultOperationValidator(WithRelaxFieldSelectionMergingNullability()).Validate(&operation, &definition, &report), report.Error())
				})
			}
		})
	}
}
