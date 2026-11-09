package astnormalization

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astparser"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/asttransform"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astvisitor"
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

func TestNormalizationSelections(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, query string
		limit       int
		reject      bool
	}{
		{"source exact", "{hello hello}", 2, false},
		{"source exceeded", "{hello hello}", 1, true},
		{"document scope", "query A{hello} query B{hello}", 1, true},
		{"expansion exact", "{...F} fragment F on Query{hello}", 4, false},
		{"expansion exceeded", "{...F} fragment F on Query{hello}", 3, true},
		{"original unused nodes count", "{hello} fragment F on Query{hello hello}", 2, true},
		{"disabled", "{hello hello}", 0, false},
		{"doubling bomb", repeatedFragmentDocument(16, "__typename"), 10000, true},
		{"deep bomb", repeatedFragmentDocument(198, "hello"), 10000, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			def := limitsTestDefinition(t)
			op, report := astparser.ParseGraphqlDocumentString(tc.query)
			require.False(t, report.HasErrors(), report.Error())
			n := NewWithOpts(WithInlineFragmentSpreads(), WithRemoveFragmentDefinitions(), WithLimits(Limits{MaxSelections: tc.limit}))
			n.NormalizeOperation(&op, &def, &report)
			if tc.reject {
				assertNormalizationLimit(t, report, "max_selections")
			} else {
				assert.False(t, report.HasErrors(), report.Error())
			}
		})
	}
}
func TestNormalizationSelectionLimitStopsAndReuses(t *testing.T) {
	def := limitsTestDefinition(t)
	n := NewWithOpts(WithInlineFragmentSpreads(), WithRemoveFragmentDefinitions(), WithLimits(Limits{MaxSelections: 10}))
	op, report := astparser.ParseGraphqlDocumentString(`{...F} fragment F on Query{...G ...G} fragment G on Query{hello hello hello hello hello}`)
	require.False(t, report.HasErrors(), report.Error())
	n.NormalizeOperation(&op, &def, &report)
	assertNormalizationLimit(t, report, "max_selections")
	require.Len(t, op.Index.ReplacedFragmentSpreads, 1)
	refs := op.SelectionSets[op.InlineFragments[0].SelectionSet].SelectionRefs
	for _, ref := range refs {
		assert.Equal(t, ast.SelectionKindFragmentSpread, op.Selections[ref].Kind)
	}
	next, r := astparser.ParseGraphqlDocumentString(`{hello}`)
	n.NormalizeOperation(&next, &def, &r)
	assert.False(t, r.HasErrors(), r.Error())
}
func TestNormalizationSelectionSetSize(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, query string
		limit       int
		reject      bool
	}{
		{"source exact", "{hello hello}", 2, false},
		{"source exceeded", "{hello hello}", 1, true},
		{"wrappers are entries", "{... on Query{hello} ... on Query{hello}}", 1, true},
		{"flatten exact", "{hello ... on Query{hello hello}}", 3, false},
		{"flatten exceeded", "{hello ... on Query{hello hello}}", 2, true},
		{"parent merge exact", "{child{a b} child{b c}}", 4, false},
		{"parent merge exceeded", "{child{a b} child{b c}}", 3, true},
		{"inline merge exceeded", `query Q($v: Boolean!) { ... on Query @include(if:$v){hello hello} ... on Query @include(if:$v){hello hello} }`, 3, true},
		{"source spreads count", "{...F ...F ...F} fragment F on Query{hello}", 2, true},
		{
			"inline wrapper after child field shares parent width",
			`{
                a: hello
                b: hello
                child { a }
                ... on Query { hello }
            }`,
			3, true,
		},
		{
			"inline wrapper after child field does not use child width",
			`{
                child { a b c }
                ... on Query { hello }
            }`,
			3, false,
		},
		{"separate children", "{child{a b} other{b c}}", 2, false},
		{"disabled", "{hello hello}", 0, false},
		{"default exact", "{" + strings.Repeat("hello ", 900) + "}", 900, false},
		{"default exceeded", "{" + strings.Repeat("hello ", 901) + "}", 900, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			def := limitsTestDefinition(t)
			op, report := astparser.ParseGraphqlDocumentString(tc.query)
			require.False(t, report.HasErrors(), report.Error())
			n := NewWithOpts(WithInlineFragmentSpreads(), WithRemoveFragmentDefinitions(), WithLimits(Limits{MaxSelectionSetSize: tc.limit}))
			n.NormalizeOperation(&op, &def, &report)
			if tc.reject {
				assertNormalizationLimit(t, report, "max_selection_set_size")
			} else {
				assert.False(t, report.HasErrors(), report.Error())
			}
		})
	}
}
func TestSelectionSetLimitStopsBeforeMergeAndReuses(t *testing.T) {
	def := limitsTestDefinition(t)
	n := NewWithOpts(WithLimits(Limits{MaxSelectionSetSize: 3}))
	op, r := astparser.ParseGraphqlDocumentString(`{child{a b} child{b c}}`)
	require.False(t, r.HasErrors(), r.Error())
	merges := 0
	op.OnMergeFields = func(left, right int) { merges++ }
	n.NormalizeOperation(&op, &def, &r)
	assertNormalizationLimit(t, r, "max_selection_set_size")
	assert.Zero(t, merges)
	refs := op.SelectionSets[op.OperationDefinitions[0].SelectionSet].SelectionRefs
	assert.Len(t, refs, 2)
	next, report := astparser.ParseGraphqlDocumentString(`{child{a}}`)
	n.NormalizeOperation(&next, &def, &report)
	assert.False(t, report.HasErrors(), report.Error())
}
func TestNormalizationLimitsDefaultAndOverride(t *testing.T) {
	for _, n := range []*OperationNormalizer{NewNormalizer(true, false), NewWithOpts(WithInlineFragmentSpreads()), NewWithOpts(WithLimits(Limits{MaxSelections: 1, MaxSelectionSetSize: 1}), WithLimits(Limits{}))} {
		def := limitsTestDefinition(t)
		op, r := astparser.ParseGraphqlDocumentString(`{hello hello}`)
		n.NormalizeOperation(&op, &def, &r)
		assert.False(t, r.HasErrors(), r.Error())
	}
}

func TestNormalizationLimitsPreserveDisabledFragmentInlining(t *testing.T) {
	def := limitsTestDefinition(t)
	op, report := astparser.ParseGraphqlDocumentString(`{hello ...F} fragment F on Query{hello hello}`)
	require.False(t, report.HasErrors(), report.Error())
	n := NewWithOpts(WithLimits(Limits{MaxSelections: 10, MaxSelectionSetSize: 3}))
	n.NormalizeOperation(&op, &def, &report)
	require.False(t, report.HasErrors(), report.Error())
	assert.Empty(t, op.Index.ReplacedFragmentSpreads)
	assert.Empty(t, op.InlineFragments)
}

func TestSelectionSetLimitIncludesDeferPlaceholder(t *testing.T) {
	def := limitsTestDefinition(t)
	op, report := astparser.ParseGraphqlDocumentString(`{child{a @__defer_internal(id:1) b @__defer_internal(id:1)}}`)
	require.False(t, report.HasErrors(), report.Error())
	walker := astvisitor.NewWalker(8)
	deferEnsureTypenameWithLimits(&walker, Limits{MaxSelectionSetSize: 2})
	walker.Walk(&op, &def, &report)
	assertNormalizationLimit(t, report, "max_selection_set_size")
	assert.Len(t, op.Selections, 3)
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
			{"copies", Limits{MaxSelections: 10000}},
			{"fields", Limits{MaxSelectionSetSize: 1000}},
			{"both", Limits{MaxSelections: 10000, MaxSelectionSetSize: 1000}},
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
