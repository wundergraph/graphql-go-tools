package astvalidation

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/astnormalization"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astparser"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/operationreport"
)

func TestFieldSelectionMergingCompositeResponseShapes(t *testing.T) {
	for _, tc := range []struct {
		name, left, right, leftSelection, rightSelection string
		want                                             ValidationState
	}{
		{"same scalar", "String", "String", "value", "value", Valid},
		{"different scalars", "String", "Int", "value", "value", Invalid},
		{"same enum", "E", "E", "value", "value", Valid},
		{"different enums", "E", "F", "value", "value", Invalid},
		{"scalar and enum", "String", "E", "value", "value", Invalid},
		{"scalar and object", "String", "Object", "value", "value { text }", Invalid},
		{"enum and object", "E", "Object", "value", "value { text }", Invalid},
		{"nested lists", "[[String!]!]!", "[[String!]!]!", "value", "value", Valid},
		{"list depth mismatch", "[String]", "[[String]]", "value", "value", Invalid},
		{"nested nullability mismatch", "[String!]", "[String]", "value", "value", Invalid},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, reverse := range []bool{false, true} {
				t.Run(fmt.Sprintf("reverse=%t", reverse), func(t *testing.T) {
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
					validateFieldMerging(t, schema, query, false, tc.want)
				})
			}
		})
	}
}

func TestFieldSelectionMergingCompositeIdentity(t *testing.T) {
	const schema = `schema { query: Query } scalar String scalar ID
  interface X { text: String otherText: String }
  interface Y { text: String otherText: String }
  type A { field: X } type B { field: Y }
  union Result = A | B
  type Query { result: Result first(id: ID): X second: Y }`
	for _, tc := range []struct {
		name, query string
		want        ValidationState
	}{
		{"different names", `{ value: first { text } value: second { text } }`, Invalid},
		{"different arguments", `{ first(id: "1") { text } first(id: "2") { text } }`, Invalid},
		{"missing argument", `{ first(id: "1") { text } first { text } }`, Invalid},
		{"equal arguments", `{ first(id: "1") { text } first(id: "1") { otherText } }`, Valid},
		{"inherited exclusivity", `{ result {
   ... on A { field { value: text } }
   ... on B { field { value: otherText } }
  } }`, Valid},
		{"conflict within exclusive branch", `{ result {
   ... on A { field { value: text } }
   ... on B { field { value: text value: otherText } }
  } }`, Invalid},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, normalize := range []bool{false, true} {
				t.Run(fmt.Sprintf("normalized=%t", normalize), func(t *testing.T) {
					validateFieldMerging(t, schema, tc.query, normalize, tc.want)
				})
			}
		})
	}
}

func validateFieldMerging(t *testing.T, schema, query string, normalize bool, want ValidationState) {
	t.Helper()
	definition, report := astparser.ParseGraphqlDocumentString(schema)
	require.False(t, report.HasErrors(), report.Error())
	operation, report := astparser.ParseGraphqlDocumentString(query)
	require.False(t, report.HasErrors(), report.Error())
	if normalize {
		astnormalization.NewNormalizer(false, false).NormalizeOperation(&operation, &definition, &report)
		require.False(t, report.HasErrors(), report.Error())
	}
	validator := NewOperationValidator([]Rule{FieldSelectionMerging()})
	require.Equal(t, want, validator.Validate(&operation, &definition, &report), report.Error())
	// A failed walk may leave ancestor state behind. Reusing the validator must
	// reset that state before validating the next operation.
	valid, parseReport := astparser.ParseGraphqlDocumentString(`{ __typename }`)
	require.False(t, parseReport.HasErrors(), parseReport.Error())
	report = operationreport.Report{}
	require.Equal(t, Valid, validator.Validate(&valid, &definition, &report), report.Error())
}

func TestFieldSelectionMergingCompositeWrappers(t *testing.T) {
	for _, tc := range []struct {
		name, left, right, leftSelection, rightSelection string
		strict, relaxed                                  ValidationState
	}{
		{"different objects", "X", "Y", "value", "value", Valid, Valid},
		{"different unions", "U", "V", "... on X { value }", "... on Y { value }", Valid, Valid},
		{"object and union", "X", "V", "value", "... on Y { value }", Valid, Valid},
		{"list versus object", "[X]", "Y", "value", "value", Invalid, Invalid},
		{"list depth mismatch", "[X]", "[[Y]]", "value", "value", Invalid, Invalid},
		{"object nullability", "X!", "Y", "value", "value", Invalid, Valid},
		{"nested list nullability", "[[X!]!]!", "[[Y]]", "value", "value", Invalid, Valid},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, relax := range []bool{false, true} {
				t.Run(fmt.Sprintf("relax=%t", relax), func(t *testing.T) {
					definition, report := astparser.ParseGraphqlDocumentString(fmt.Sprintf(`
      schema { query: Query } scalar String
      type X { value: String } type Y { value: String }
      union U = X union V = Y
      type A { field: %s } type B { field: %s }
      union Result = A | B type Query { result: Result }`, tc.left, tc.right))
					require.False(t, report.HasErrors(), report.Error())
					operation, report := astparser.ParseGraphqlDocumentString(fmt.Sprintf(`{ result {
      ... on A { field { %s } } ... on B { field { %s } }
     } }`, tc.leftSelection, tc.rightSelection))
					require.False(t, report.HasErrors(), report.Error())
					want := tc.strict
					if relax {
						want = tc.relaxed
					}
					validator := NewOperationValidator([]Rule{FieldSelectionMerging(relax)})
					require.Equal(t, want, validator.Validate(&operation, &definition, &report), report.Error())
				})
			}
		})
	}
}
