package astnormalization

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
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

func assertNormalizationLimit(t *testing.T, report operationreport.Report, name string) {
	t.Helper()
	require.Len(t, report.ExternalErrors, 1)
	assert.Contains(t, report.ExternalErrors[0].Message, name)
	assert.Equal(t, "NORMALIZATION_LIMIT_EXCEEDED", report.ExternalErrors[0].ExtensionCode)
	assert.Equal(t, 400, report.ExternalErrors[0].StatusCode)
}

func TestNormalizationLimits(t *testing.T) {
	t.Parallel()

	normalize := func(t *testing.T, query string, limits Limits) operationreport.Report {
		t.Helper()
		definition := limitsTestDefinition(t)
		op, report := astparser.ParseGraphqlDocumentString(query)
		require.False(t, report.HasErrors(), report.Error())
		original := len(op.Selections)
		normalizer := NewWithOpts(
			WithInlineFragmentSpreads(),
			WithRemoveFragmentDefinitions(),
			WithLimits(limits),
		)
		normalizer.NormalizeOperation(&op, &definition, &report)
		if report.HasErrors() && limits.MaxFragmentExpansionSelections > 0 {
			assert.LessOrEqual(t, len(op.Selections)-original, limits.MaxFragmentExpansionSelections)
		}
		return report
	}

	t.Run("exact copying budget", func(t *testing.T) {
		t.Parallel()
		report := normalize(t, repeatedFragmentDocument(2, "hello"), Limits{
			MaxFragmentExpansionSelections: 17,
			MaxExpandedSelectionSetFields:  4,
		})
		assert.False(t, report.HasErrors(), report.Error())
	})
	t.Run("copying budget exhausted", func(t *testing.T) {
		t.Parallel()
		report := normalize(t, repeatedFragmentDocument(2, "hello"), Limits{
			MaxFragmentExpansionSelections: 16,
			MaxExpandedSelectionSetFields:  4,
		})
		assertNormalizationLimit(t, report, "max_fragment_expansion_selections")
	})
	t.Run("duplicate fields count", func(t *testing.T) {
		t.Parallel()
		report := normalize(t, repeatedFragmentDocument(2, "__typename"), Limits{
			MaxFragmentExpansionSelections: 17,
			MaxExpandedSelectionSetFields:  3,
		})
		assertNormalizationLimit(t, report, "max_expanded_selection_set_fields")
	})
	t.Run("copied inline wrapper counts", func(t *testing.T) {
		t.Parallel()
		report := normalize(t, `{ ...F } fragment F on Query { ... on Query { hello } }`, Limits{
			MaxFragmentExpansionSelections: 2,
			MaxExpandedSelectionSetFields:  1,
		})
		assertNormalizationLimit(t, report, "max_fragment_expansion_selections")
	})
	t.Run("existing inline wrappers are not copies", func(t *testing.T) {
		t.Parallel()
		report := normalize(t, `{ ... on Query { ... on Query { hello } } }`, Limits{
			MaxFragmentExpansionSelections: 1,
			MaxExpandedSelectionSetFields:  1,
		})
		assert.False(t, report.HasErrors(), report.Error())
	})
	t.Run("inline wrappers share width", func(t *testing.T) {
		t.Parallel()
		report := normalize(t, `{ hello ... on Query { hello ... on Query { hello } } }`, Limits{
			MaxExpandedSelectionSetFields: 2,
		})
		assertNormalizationLimit(t, report, "max_expanded_selection_set_fields")
	})
	t.Run("child widths are separate", func(t *testing.T) {
		t.Parallel()
		report := normalize(t, `{ child { a b } other { a b } }`, Limits{
			MaxExpandedSelectionSetFields: 2,
		})
		assert.False(t, report.HasErrors(), report.Error())
	})
	t.Run("duplicate parent fields have separate widths before merging", func(t *testing.T) {
		t.Parallel()
		report := normalize(t, `{ child { a b } child { b c } }`, Limits{
			MaxExpandedSelectionSetFields: 2,
		})
		assert.False(t, report.HasErrors(), report.Error())
	})
	t.Run("nested field width exceeded", func(t *testing.T) {
		t.Parallel()
		report := normalize(t, `{ child { a b c } }`, Limits{
			MaxExpandedSelectionSetFields: 2,
		})
		assertNormalizationLimit(t, report, "max_expanded_selection_set_fields")
	})
	t.Run("aliases count", func(t *testing.T) {
		t.Parallel()
		report := normalize(t, `{ a: hello b: hello }`, Limits{
			MaxExpandedSelectionSetFields: 1,
		})
		assertNormalizationLimit(t, report, "max_expanded_selection_set_fields")
	})
	t.Run("pruned fields do not count", func(t *testing.T) {
		t.Parallel()
		report := normalize(t, `{ hello hello @skip(if: true) hello @include(if: false) }`, Limits{
			MaxExpandedSelectionSetFields: 1,
		})
		assert.False(t, report.HasErrors(), report.Error())
	})
	t.Run("pruned spreads do not spend budget", func(t *testing.T) {
		t.Parallel()
		report := normalize(t, `{ hello ...F @skip(if: true) } fragment F on Query { hello hello }`, Limits{
			MaxFragmentExpansionSelections: 1,
			MaxExpandedSelectionSetFields:  1,
		})
		assert.False(t, report.HasErrors(), report.Error())
	})
	t.Run("unused fragments do not count", func(t *testing.T) {
		t.Parallel()
		report := normalize(t, `{ hello } fragment Unused on Query { hello hello hello }`, Limits{
			MaxFragmentExpansionSelections: 1,
			MaxExpandedSelectionSetFields:  1,
		})
		assert.False(t, report.HasErrors(), report.Error())
	})
	t.Run("revisits do not double count fields", func(t *testing.T) {
		t.Parallel()
		report := normalize(t, `{ hello ...F ...F } fragment F on Query { hello }`, Limits{
			MaxFragmentExpansionSelections: 4,
			MaxExpandedSelectionSetFields:  3,
		})
		assert.False(t, report.HasErrors(), report.Error())
	})
	t.Run("multiple operations have separate budgets", func(t *testing.T) {
		t.Parallel()
		report := normalize(t, `query A { ...F } query B { ...F } fragment F on Query { hello }`, Limits{
			MaxFragmentExpansionSelections: 2,
			MaxExpandedSelectionSetFields:  1,
		})
		assert.False(t, report.HasErrors(), report.Error())
	})
}

func TestNormalizationLimitsBoundExpansion(t *testing.T) {
	t.Parallel()

	check := func(t *testing.T, levels int, leaf string, limits Limits) {
		t.Helper()
		definition := limitsTestDefinition(t)
		op := ast.NewSmallDocument()
		op.Input.ResetInputString(repeatedFragmentDocument(levels, leaf))
		var report operationreport.Report
		parser := astparser.NewParser()
		_, err := parser.ParseWithLimits(astparser.TokenizerLimits{MaxDepth: 200, MaxFields: 3500}, op, &report)
		require.NoError(t, err)
		require.False(t, report.HasErrors(), report.Error())
		original := len(op.Selections)
		normalizer := NewWithOpts(WithInlineFragmentSpreads(), WithLimits(limits))
		normalizer.NormalizeOperation(op, &definition, &report)
		require.Len(t, report.ExternalErrors, 1)
		assert.Equal(t, "NORMALIZATION_LIMIT_EXCEEDED", report.ExternalErrors[0].ExtensionCode)
		assert.Positive(t, len(op.Selections)-original, "work is allowed up to the budget")
		assert.LessOrEqual(t, len(op.Selections)-original, limits.MaxFragmentExpansionSelections)
	}

	t.Run("typename level 18 copy budget", func(t *testing.T) {
		t.Parallel()
		check(t, 18, "__typename", Limits{MaxFragmentExpansionSelections: 10000})
	})
	t.Run("typename level 18 both budgets", func(t *testing.T) {
		t.Parallel()
		check(t, 18, "__typename", Limits{
			MaxFragmentExpansionSelections: 10000,
			MaxExpandedSelectionSetFields:  1000,
		})
	})
	t.Run("field level 18 copy budget", func(t *testing.T) {
		t.Parallel()
		check(t, 18, "hello", Limits{MaxFragmentExpansionSelections: 10000})
	})
	t.Run("field level 18 both budgets", func(t *testing.T) {
		t.Parallel()
		check(t, 18, "hello", Limits{
			MaxFragmentExpansionSelections: 10000,
			MaxExpandedSelectionSetFields:  1000,
		})
	})
	t.Run("typename level 198 copy budget", func(t *testing.T) {
		t.Parallel()
		check(t, 198, "__typename", Limits{MaxFragmentExpansionSelections: 10000})
	})
	t.Run("typename level 198 both budgets", func(t *testing.T) {
		t.Parallel()
		check(t, 198, "__typename", Limits{
			MaxFragmentExpansionSelections: 10000,
			MaxExpandedSelectionSetFields:  1000,
		})
	})
	t.Run("field level 198 copy budget", func(t *testing.T) {
		t.Parallel()
		check(t, 198, "hello", Limits{MaxFragmentExpansionSelections: 10000})
	})
	t.Run("field level 198 both budgets", func(t *testing.T) {
		t.Parallel()
		check(t, 198, "hello", Limits{
			MaxFragmentExpansionSelections: 10000,
			MaxExpandedSelectionSetFields:  1000,
		})
	})
}

func TestNormalizationLimitsNamedOperationAndReuse(t *testing.T) {
	t.Parallel()

	definition := limitsTestDefinition(t)
	normalizer := NewWithOpts(
		WithInlineFragmentSpreads(),
		WithRemoveFragmentDefinitions(),
		WithRemoveNotMatchingOperationDefinitions(),
		WithLimits(Limits{
			MaxFragmentExpansionSelections: 10000,
			MaxExpandedSelectionSetFields:  1000,
		}),
	)
	normalize := func(query, name string) (*ast.Document, operationreport.Report) {
		t.Helper()
		op, report := astparser.ParseGraphqlDocumentString(query)
		require.False(t, report.HasErrors(), report.Error())
		normalizer.NormalizeNamedOperation(&op, &definition, []byte(name), &report)
		return &op, report
	}

	// These operations intentionally share a normalizer and run in sequence.
	_, report := normalize(repeatedFragmentDocument(18, "hello"), "Q")
	assertNormalizationLimit(t, report, "max_expanded_selection_set_fields")

	op, report := normalize(`query Small { hello hello } `+repeatedFragmentDocument(18, "hello"), "Small")
	require.False(t, report.HasErrors(), report.Error())
	printed, err := astprinter.PrintString(op)
	require.NoError(t, err)
	assert.Equal(t, "query Small {hello}", printed)

	_, report = normalize(`query Q { ...Missing }`, "Q")
	assert.True(t, report.HasErrors())
	assert.Contains(t, report.Error(), "undefined")

	_, report = normalize(`query Q { ...A } fragment A on Query { ...B } fragment B on Query { ...A }`, "Q")
	assert.True(t, report.HasErrors())
	assert.Contains(t, report.Error(), "cycle")

	op, report = normalize(`query Q { hello hello }`, "Q")
	require.False(t, report.HasErrors(), report.Error())
	printed, err = astprinter.PrintString(op)
	require.NoError(t, err)
	assert.Equal(t, "query Q {hello}", printed)
}

func TestNormalizationLimitsDisabled(t *testing.T) {
	t.Parallel()

	query := "{ " + strings.Repeat("hello ", 1001) + "}"
	normalize := func(t *testing.T, normalizer *OperationNormalizer) operationreport.Report {
		t.Helper()
		definition := limitsTestDefinition(t)
		op, report := astparser.ParseGraphqlDocumentString(query)
		require.False(t, report.HasErrors(), report.Error())
		normalizer.NormalizeOperation(&op, &definition, &report)
		return report
	}

	t.Run("legacy constructor", func(t *testing.T) {
		t.Parallel()
		normalizer := NewNormalizer(true, false)
		report := normalize(t, normalizer)
		assert.False(t, report.HasErrors(), report.Error())
	})
	t.Run("options constructor", func(t *testing.T) {
		t.Parallel()
		normalizer := NewWithOpts(WithInlineFragmentSpreads())
		report := normalize(t, normalizer)
		assert.False(t, report.HasErrors(), report.Error())
	})
	t.Run("explicit zero value", func(t *testing.T) {
		t.Parallel()
		normalizer := NewWithOpts(WithLimits(Limits{}))
		report := normalize(t, normalizer)
		assert.False(t, report.HasErrors(), report.Error())
	})
	t.Run("source fields do not spend copy budget", func(t *testing.T) {
		t.Parallel()
		normalizer := NewWithOpts(
			WithInlineFragmentSpreads(),
			WithLimits(Limits{MaxFragmentExpansionSelections: 1}),
		)
		report := normalize(t, normalizer)
		assert.False(t, report.HasErrors(), report.Error())
	})
	t.Run("width without fragment inlining", func(t *testing.T) {
		t.Parallel()
		normalizer := NewWithOpts(WithLimits(Limits{MaxExpandedSelectionSetFields: 1000}))
		report := normalize(t, normalizer)
		assertNormalizationLimit(t, report, "max_expanded_selection_set_fields")
	})
	t.Run("unexpanded named fragments are not counted", func(t *testing.T) {
		t.Parallel()
		definition := limitsTestDefinition(t)
		op, report := astparser.ParseGraphqlDocumentString(`{ hello ...F } fragment F on Query { hello hello }`)
		require.False(t, report.HasErrors(), report.Error())
		normalizer := NewWithOpts(WithLimits(Limits{MaxExpandedSelectionSetFields: 1}))
		normalizer.NormalizeOperation(&op, &definition, &report)
		assert.False(t, report.HasErrors(), report.Error())
	})
	t.Run("zero overrides limits", func(t *testing.T) {
		t.Parallel()
		normalizer := NewWithOpts(
			WithLimits(Limits{
				MaxFragmentExpansionSelections: 1000,
				MaxExpandedSelectionSetFields:  1000,
			}),
			WithLimits(Limits{}),
		)
		report := normalize(t, normalizer)
		assert.False(t, report.HasErrors(), report.Error())
	})
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
