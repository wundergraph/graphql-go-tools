package astvalidation

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/astnormalization"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astparser"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/asttransform"
)

func TestFieldSelectionMergingLeafTypes(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name, left, right, leftSelection, rightSelection string
		want                                             ValidationState
	}{
		{"SameScalar", "String", "String", "value", "value", Valid},
		{"DifferentScalars", "String", "Int", "value", "value", Invalid},
		{"SameEnum", "E", "E", "value", "value", Valid},
		{"DifferentEnums", "E", "F", "value", "value", Invalid},
		{"ScalarAndEnum", "String", "E", "value", "value", Invalid},
		{"ScalarAndObject", "String", "Object", "value", "value { text }", Invalid},
		{"EnumAndObject", "E", "Object", "value", "value { text }", Invalid},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Check both selection orders: mixed leaf/composite pairs must not be skipped.
			orders := []bool{false, true}
			for _, reverse := range orders {
				left, right := tc.left, tc.right
				leftSelection, rightSelection := tc.leftSelection, tc.rightSelection
				if reverse {
					left, right = right, left
					leftSelection, rightSelection = rightSelection, leftSelection
				}
				schema := fmt.Sprintf(`schema { query: Query } scalar String scalar Int
     enum E { VALUE } enum F { VALUE } type Object { text: String }
     interface X { value: %s } interface Y { value: %s }
     type A { field: X } type B { field: Y }
     union Result = A | B type Query { result: Result }`, left, right)
				query := fmt.Sprintf(`{ result { ... on A { field { %s } } ... on B { field { %s } } } }`, leftSelection, rightSelection)
				assertFieldMerging(t, schema, query, tc.want, false)
			}
		})
	}
}

func TestFieldSelectionMergingIdentity(t *testing.T) {
	t.Parallel()

	const schema = `schema { query: Query } scalar String scalar ID
  interface X { text: String otherText: String }
  interface Y { text: String otherText: String }
  type A { field: X } type B { field: Y }
  union Result = A | B
  type Query { result: Result first(id: ID): X second: Y }`
	cases := []struct {
		name, query string
		want        ValidationState
	}{
		{"DifferentNames", `{ value: first { text } value: second { text } }`, Invalid},
		{"DifferentArguments", `{ first(id: "1") { text } first(id: "2") { text } }`, Invalid},
		{"MissingArgument", `{ first(id: "1") { text } first { text } }`, Invalid},
		{"EqualArguments", `{ first(id: "1") { text } first(id: "1") { otherText } }`, Valid},
		{"InheritedExclusivity", `{ result {
   ... on A { field { value: text } }
   ... on B { field { value: otherText } }
  } }`, Valid},
		{"ConflictWithinExclusiveBranch", `{ result {
   ... on A { field { value: text } }
   ... on B { field { value: text value: otherText } }
  } }`, Invalid},
	}
	modes := []struct {
		name      string
		normalize bool
	}{{"Parsed", false}, {"Normalized", true}}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			for _, mode := range modes {
				t.Run(mode.name, func(t *testing.T) {
					t.Parallel()
					assertFieldMerging(t, schema, tc.query, tc.want, mode.normalize)
				})
			}
		})
	}
}

func TestFieldSelectionMergingCompositeTypes(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name, left, right, leftSelection, rightSelection string
		strict, relaxed                                  ValidationState
	}{
		{"DifferentObjects", "X", "Y", "value", "value", Valid, Valid},
		{"DifferentInterfaces", "I", "J", "value", "value", Valid, Valid},
		{"ObjectAndInterface", "X", "I", "value", "value", Valid, Valid},
		{"DifferentUnions", "U", "V", "... on X { value }", "... on Y { value }", Valid, Valid},
		{"ObjectAndUnion", "X", "V", "value", "... on Y { value }", Valid, Valid},
		{"ListAndObject", "[X]", "Y", "value", "value", Invalid, Invalid},
		{"ListDepth", "[X]", "[[Y]]", "value", "value", Invalid, Invalid},
		{"NestedNullability", "[[X!]!]!", "[[Y]]", "value", "value", Invalid, Valid},
	}
	modes := []struct {
		name  string
		relax bool
	}{{"Strict", false}, {"Relaxed", true}}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			for _, mode := range modes {
				t.Run(mode.name, func(t *testing.T) {
					t.Parallel()
					schema := fmt.Sprintf(`schema { query: Query } scalar String
      type X { value: String } type Y { value: String }
      interface I { value: String } interface J { value: String }
      union U = X union V = Y
      type A { field: %s } type B { field: %s }
      union Result = A | B type Query { result: Result }`, tc.left, tc.right)
					query := fmt.Sprintf(`{ result {
      ... on A { field { %s } } ... on B { field { %s } }
     } }`, tc.leftSelection, tc.rightSelection)
					want := tc.strict
					if mode.relax {
						want = tc.relaxed
					}
					assertFieldMerging(t, schema, query, want, false, mode.relax)
				})
			}
		})
	}
}

func TestFieldSelectionMergingValidatorReuse(t *testing.T) {
	t.Parallel()

	definition, report := astparser.ParseGraphqlDocumentString(`schema { query: Query } scalar String scalar ID
  type Query { item(id: ID): Item } type Item { name: String }`)
	require.False(t, report.HasErrors(), report.Error())
	validator := NewOperationValidator([]Rule{FieldSelectionMerging()})
	cases := []struct {
		name, query string
		want        ValidationState
	}{
		{"ConflictingArguments", `{ item(id: "1") { name } item(id: "2") { name } }`, Invalid},
		{"ValidAfterError", `{ item(id: "3") { name } item(id: "3") { name } }`, Valid},
	}
	// Reuse the validator sequentially after an aborted walk with ancestor state.
	for _, tc := range cases {
		operation, report := astparser.ParseGraphqlDocumentString(tc.query)
		require.False(t, report.HasErrors(), report.Error())
		assert.Equal(t, tc.want, validator.Validate(&operation, &definition, &report), "%s: %s", tc.name, report.Error())
	}
}

func assertFieldMerging(t *testing.T, schema, query string, want ValidationState, normalize bool, relax ...bool) {
	t.Helper()
	definition, report := astparser.ParseGraphqlDocumentString(schema)
	require.False(t, report.HasErrors(), report.Error())
	operation, report := astparser.ParseGraphqlDocumentString(query)
	require.False(t, report.HasErrors(), report.Error())
	if normalize {
		astnormalization.NewNormalizer(false, false).NormalizeOperation(&operation, &definition, &report)
		require.False(t, report.HasErrors(), report.Error())
	}
	validator := NewOperationValidator([]Rule{FieldSelectionMerging(relax...)})
	assert.Equal(t, want, validator.Validate(&operation, &definition, &report), report.Error())
}

func TestFieldSelectionMergingExtractedInputObjects(t *testing.T) {
	t.Parallel()

	const schema = `directive @tag on FIELD
  schema { query: Query } scalar String scalar Int
  input Filter { a: Int b: Int children: [Filter] }
  type Query { item(f: Filter): Item } type Item { name: String id: Int }`
	cases := []struct {
		name, left, right, directive string
		want                         ValidationState
		variables                    int
	}{
		{"ReorderedFields", `{a: 1, b: 2}`, `{b: 2, a: 1}`, "", Valid, 1},
		{"UnmergedFields", `{a: 1, b: 2}`, `{b: 2, a: 1}`, "@tag", Valid, 1},
		{"NestedObjects", `{children: [{a: 1, b: 2}]}`, `{children: [{b: 2, a: 1}]}`, "", Valid, 1},
		{"DuplicateFields", `{a: 1, a: 1}`, `{a: 1, b: 2}`, "", Invalid, 2},
		{"DifferentValues", `{a: 1, b: 2}`, `{b: 3, a: 1}`, "", Invalid, 2},
		{"DifferentListOrder", `{children: [{a: 1}, {a: 2}]}`, `{children: [{a: 2}, {a: 1}]}`, "", Invalid, 2},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			definition, report := astparser.ParseGraphqlDocumentString(schema)
			require.False(t, report.HasErrors(), report.Error())
			operation, report := astparser.ParseGraphqlDocumentString(fmt.Sprintf(`{ item(f: %s) %s { name } item(f: %s) { id } }`, tc.left, tc.directive, tc.right))
			require.False(t, report.HasErrors(), report.Error())
			astnormalization.NewNormalizer(false, true).NormalizeOperation(&operation, &definition, &report)
			require.False(t, report.HasErrors(), report.Error())

			assert.Len(t, operation.OperationDefinitions[0].VariableDefinitions.Refs, tc.variables)
			assert.Equal(t, tc.want, DefaultOperationValidator().Validate(&operation, &definition, &report), report.Error())
		})
	}
}

func TestFieldSelectionMergingSkippedFields(t *testing.T) {
	t.Parallel()

	const schema = `schema { query: Query } scalar String
  interface Node { id: String other: String }
  type A implements Node { id: String other: String f: B }
  type B { id: String }
  type Query { node: Node }`
	cases := []struct {
		name, query string
		want        ValidationState
	}{
		{"ConflictingSibling", `{ node { x: id ... on A { f { __typename } } x: other } }`, Invalid},
		{"RepeatedSkippedFields", `{ node { x: id ... on A { f { __typename __typename } } x: other } }`, Invalid},
		{"MatchingSibling", `{ node { x: id ... on A { f { __typename } } x: id } }`, Valid},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Without the base schema, KnownArguments skips __typename. Other
			// visitors receive EnterField but not LeaveField for that selection.
			definition, report := astparser.ParseGraphqlDocumentString(schema)
			require.False(t, report.HasErrors(), report.Error())
			operation, report := astparser.ParseGraphqlDocumentString(tc.query)
			require.False(t, report.HasErrors(), report.Error())
			assert.Equal(t, tc.want, DefaultOperationValidator().Validate(&operation, &definition, &report), report.Error())
			if tc.want == Invalid {
				assert.Contains(t, report.Error(), "differing fields for objectName 'x'")
			}
		})
	}
}

func TestFieldSelectionMergingTypename(t *testing.T) {
	t.Parallel()
	const schema = `schema { query: Query } scalar String scalar Int
  type Query { item: Item result: Result }
  union Result = Item | Other
  type Item { name: String! } type Other { name: String! nullableName: String number: Int! }`
	cases := []struct {
		name, query     string
		strict, relaxed ValidationState
	}{
		{"AliasCollision", `{ item { a: __typename a: name } }`, Invalid, Invalid},
		{"ReverseAliasCollision", `{ item { a: name a: __typename } }`, Invalid, Invalid},
		{"MatchingTypenames", `{ item { a: __typename a: __typename } }`, Valid, Valid},
		{"DisjointMatchingShape", `{ result { ... on Item { a: __typename } ... on Other { a: name } } }`, Valid, Valid},
		{"DisjointNullability", `{ result { ... on Item { a: __typename } ... on Other { a: nullableName } } }`, Invalid, Valid},
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
