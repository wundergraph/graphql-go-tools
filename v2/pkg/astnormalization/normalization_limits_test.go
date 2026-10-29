package astnormalization

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astparser"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astprinter"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/asttransform"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/operationreport"
)

func repeatedFragmentDocument(levels int, leaf string) string {
	var out strings.Builder
	fmt.Fprintf(&out, "query Q { ...F%d } fragment F0 on Query { %s } ", levels, leaf)
	for i := 1; i <= levels; i++ {
		fmt.Fprintf(&out, "fragment F%d on Query { ...F%d ...F%d } ", i, i-1, i-1)
	}
	return out.String()
}

func limitsTestDefinition(t testing.TB) ast.Document {
	t.Helper()
	definition, report := astparser.ParseGraphqlDocumentString(`type Query { hello: String child: Child other: Child } type Child { a: String b: String c: String }`)
	require.False(t, report.HasErrors())
	require.NoError(t, asttransform.MergeDefinitionWithBaseSchema(&definition))
	return definition
}

func TestNormalizationLimits(t *testing.T) {
	definition := limitsTestDefinition(t)
	for _, tt := range []struct {
		name, query    string
		copies, fields int
		wantLimit      string
	}{
		{"exact copying budget", repeatedFragmentDocument(2, "hello"), 17, 4, ""},
		{"copying budget exhausted", repeatedFragmentDocument(2, "hello"), 16, 4, "max_fragment_expansion_selections"},
		{"duplicate fields count", repeatedFragmentDocument(2, "__typename"), 17, 3, "max_expanded_selection_set_fields"},
		{"copied inline wrapper counts", `{ ...F } fragment F on Query { ... on Query { hello } }`, 2, 1, "max_fragment_expansion_selections"},
		{"existing inline wrappers are not copies", `{ ... on Query { ... on Query { hello } } }`, 1, 1, ""},
		{"inline wrappers share width", `{ hello ... on Query { hello ... on Query { hello } } }`, 0, 2, "max_expanded_selection_set_fields"},
		{"child widths are separate", `{ child { a b } other { a b } }`, 0, 2, ""},
		{"nested field width exceeded", `{ child { a b c } }`, 0, 2, "max_expanded_selection_set_fields"},
		{"aliases count", `{ a: hello b: hello }`, 0, 1, "max_expanded_selection_set_fields"},
		{"pruned fields do not count", `{ hello hello @skip(if: true) hello @include(if: false) }`, 0, 1, ""},
		{"pruned spreads do not spend budget", `{ hello ...F @skip(if: true) } fragment F on Query { hello hello }`, 1, 1, ""},
		{"unused fragments do not count", `{ hello } fragment Unused on Query { hello hello hello }`, 1, 1, ""},
		{"revisits do not double count fields", `{ hello ...F ...F } fragment F on Query { hello }`, 4, 3, ""},
		{"multiple operations have separate budgets", `query A { ...F } query B { ...F } fragment F on Query { hello }`, 2, 1, ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			op, report := astparser.ParseGraphqlDocumentString(tt.query)
			require.False(t, report.HasErrors())
			original := len(op.Selections)
			NewWithOpts(WithInlineFragmentSpreads(), WithRemoveFragmentDefinitions(), WithLimits(Limits{tt.copies, tt.fields})).NormalizeOperation(&op, &definition, &report)
			if tt.wantLimit == "" {
				require.False(t, report.HasErrors(), report.Error())
				return
			}
			require.Len(t, report.ExternalErrors, 1)
			require.Contains(t, report.ExternalErrors[0].Message, tt.wantLimit)
			require.Equal(t, "NORMALIZATION_LIMIT_EXCEEDED", report.ExternalErrors[0].ExtensionCode)
			require.Equal(t, 400, report.ExternalErrors[0].StatusCode)
			if tt.copies > 0 {
				require.LessOrEqual(t, len(op.Selections)-original, tt.copies)
			}
		})
	}
}

func TestNormalizationLimitsBoundExpansion(t *testing.T) {
	definition := limitsTestDefinition(t)
	for _, levels := range []int{18, 198} {
		for _, leaf := range []string{"__typename", "hello"} {
			for _, limits := range []Limits{{MaxFragmentExpansionSelections: 10000}, {MaxFragmentExpansionSelections: 10000, MaxExpandedSelectionSetFields: 1000}} {
				t.Run(fmt.Sprintf("%s/%d/%v", leaf, levels, limits), func(t *testing.T) {
					op := ast.NewSmallDocument()
					op.Input.ResetInputString(repeatedFragmentDocument(levels, leaf))
					var report operationreport.Report
					_, err := astparser.NewParser().ParseWithLimits(astparser.TokenizerLimits{MaxDepth: 200, MaxFields: 3500}, op, &report)
					require.NoError(t, err)
					require.False(t, report.HasErrors())
					original := len(op.Selections)
					NewWithOpts(WithInlineFragmentSpreads(), WithLimits(limits)).NormalizeOperation(op, &definition, &report)
					require.Len(t, report.ExternalErrors, 1)
					require.Equal(t, "NORMALIZATION_LIMIT_EXCEEDED", report.ExternalErrors[0].ExtensionCode)
					require.Positive(t, len(op.Selections)-original, "work is allowed up to the budget")
					require.LessOrEqual(t, len(op.Selections)-original, limits.MaxFragmentExpansionSelections)
				})
			}
		}
	}
}

func TestNormalizationLimitsNamedOperationAndReuse(t *testing.T) {
	definition := limitsTestDefinition(t)
	normalizer := NewWithOpts(WithInlineFragmentSpreads(), WithRemoveFragmentDefinitions(), WithRemoveNotMatchingOperationDefinitions(), WithLimits(Limits{10000, 1000}))
	for _, tt := range []struct{ query, name, want string }{
		{repeatedFragmentDocument(18, "hello"), "Q", "NORMALIZATION_LIMIT_EXCEEDED"},
		{`query Small { hello hello } ` + repeatedFragmentDocument(18, "hello"), "Small", ""},
		{`query Q { ...Missing }`, "Q", "undefined"},
		{`query Q { ...A } fragment A on Query { ...B } fragment B on Query { ...A }`, "Q", "cycle"},
		{`query Q { hello hello }`, "Q", ""},
	} {
		op, report := astparser.ParseGraphqlDocumentString(tt.query)
		require.False(t, report.HasErrors())
		normalizer.NormalizeNamedOperation(&op, &definition, []byte(tt.name), &report)
		if tt.want == "" {
			require.False(t, report.HasErrors(), report.Error())
			printed, err := astprinter.PrintString(&op)
			require.NoError(t, err)
			require.Equal(t, "query "+tt.name+" {hello}", printed)
		} else {
			require.True(t, report.HasErrors())
			if tt.want == "NORMALIZATION_LIMIT_EXCEEDED" {
				require.Equal(t, tt.want, report.ExternalErrors[0].ExtensionCode)
			} else {
				require.Contains(t, report.Error(), tt.want)
			}
		}
	}
}

func TestNormalizationLimitsDisabled(t *testing.T) {
	definition := limitsTestDefinition(t)
	query := "{ " + strings.Repeat("hello ", 1001) + "}"
	for _, tt := range []struct {
		name       string
		normalizer *OperationNormalizer
		reject     bool
	}{
		{"legacy constructor", NewNormalizer(true, false), false},
		{"options constructor", NewWithOpts(WithInlineFragmentSpreads()), false},
		{"explicit zero value", NewWithOpts(WithLimits(Limits{})), false},
		{"source fields do not spend copy budget", NewWithOpts(WithInlineFragmentSpreads(), WithLimits(Limits{MaxFragmentExpansionSelections: 1})), false},
		{"width without fragment inlining", NewWithOpts(WithLimits(Limits{MaxExpandedSelectionSetFields: 1000})), true},
		{"zero overrides limits", NewWithOpts(WithLimits(Limits{1000, 1000}), WithLimits(Limits{})), false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			op, report := astparser.ParseGraphqlDocumentString(query)
			tt.normalizer.NormalizeOperation(&op, &definition, &report)
			require.Equal(t, tt.reject, report.HasErrors(), report.Error())
		})
	}
}

func BenchmarkNormalizationLimits(b *testing.B) {
	definition := limitsTestDefinition(b)
	for _, levels := range []int{8, 18, 198} {
		b.Run(fmt.Sprintf("levels_%d", levels), func(b *testing.B) {
			query := repeatedFragmentDocument(levels, "hello")
			n := NewWithOpts(WithInlineFragmentSpreads(), WithRemoveFragmentDefinitions(), WithLimits(Limits{10000, 1000}))
			op := ast.NewSmallDocument()
			parser := astparser.NewParser()
			var report operationreport.Report
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				op.Reset()
				report.Reset()
				op.Input.ResetInputString(query)
				parser.Parse(op, &report)
				if report.HasErrors() {
					b.Fatal(report.Error())
				}
				n.NormalizeOperation(op, &definition, &report)
				if report.HasErrors() != (levels > 8) {
					b.Fatal(report.Error())
				}
			}
		})
	}
}

// BenchmarkNormalizationLimitOverhead compares the individual budgets and their
// combined cost on accepted inputs, reusing parser and normalizer state like a router kit.
func BenchmarkNormalizationLimitOverhead(b *testing.B) {
	definition := limitsTestDefinition(b)
	var flat, wrappers strings.Builder
	flat.WriteString("query Q {")
	for i := range 1000 {
		fmt.Fprintf(&flat, "a%d:hello ", i)
	}
	flat.WriteString("}")
	wrappers.WriteString("query Q {" + strings.Repeat("...F4 ", 1000) + "} fragment F0 on Query {hello} ")
	for i := 1; i <= 4; i++ {
		fmt.Fprintf(&wrappers, "fragment F%d on Query {...F%d} ", i, i-1)
	}
	for _, query := range []struct{ name, document string }{
		{"small", "query Q { hello child { a b } }"},
		{"fields_1000", flat.String()},
		{"spreads_1000", "query Q {" + strings.Repeat("...F ", 1000) + "} fragment F on Query {hello}"},
		{"copies_10000", wrappers.String()},
	} {
		for _, budget := range []struct {
			name   string
			limits Limits
		}{
			{"disabled", Limits{}},
			{"copies", Limits{MaxFragmentExpansionSelections: 10000}},
			{"fields", Limits{MaxExpandedSelectionSetFields: 1000}},
			{"both", Limits{MaxFragmentExpansionSelections: 10000, MaxExpandedSelectionSetFields: 1000}},
		} {
			b.Run(query.name+"/"+budget.name, func(b *testing.B) {
				n := NewWithOpts(WithInlineFragmentSpreads(), WithRemoveFragmentDefinitions(), WithRemoveUnusedVariables(), WithRemoveNotMatchingOperationDefinitions(), WithLimits(budget.limits))
				parser := astparser.NewParser()
				op := ast.NewSmallDocument()
				var report operationreport.Report
				name := []byte("Q")
				run := func() {
					op.Reset()
					report.Reset()
					op.Input.ResetInputString(query.document)
					_, err := parser.ParseWithLimits(astparser.TokenizerLimits{MaxDepth: 200, MaxFields: 3500}, op, &report)
					if err != nil {
						b.Fatal(err)
					}
					if report.HasErrors() {
						b.Fatal(report.Error())
					}
					n.NormalizeNamedOperation(op, &definition, name, &report)
					if report.HasErrors() {
						b.Fatal(report.Error())
					}
				}
				run() // Grow reusable AST and walker buffers outside the measurement.
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					run()
				}
			})
		}
	}
}
