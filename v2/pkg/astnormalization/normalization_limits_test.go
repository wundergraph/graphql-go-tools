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
	const definitionSDL = `
		type Query {
		  hello: String
		  child: Child
		  other: Child
		}

		type Child {
		  a: String
		  b: String
		  c: String
		}
	`

	t.Helper()
	definition, report := astparser.ParseGraphqlDocumentString(definitionSDL)
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
	const (
		sourceQuery = `
			{
			  hello
			  hello
			}
		`
		multipleOperations = `
			query A {
			  hello
			}

			query B {
			  hello
			}
		`
		fragmentQuery = `
			{
			  ...F
			}

			fragment F on Query {
			  hello
			}
		`
		unusedFragmentQuery = `
			{
			  hello
			}

			fragment F on Query {
			  hello
			  hello
			}
		`
	)

	cases := []struct {
		name   string
		query  string
		limit  int
		reject bool
	}{
		{
			name:   "source exact",
			query:  sourceQuery,
			limit:  2,
			reject: false,
		},
		{
			name:   "source exceeded",
			query:  sourceQuery,
			limit:  1,
			reject: true,
		},
		{
			name:   "document scope",
			query:  multipleOperations,
			limit:  1,
			reject: true,
		},
		{
			name:   "expansion exact",
			query:  fragmentQuery,
			limit:  4,
			reject: false,
		},
		{
			name:   "expansion exceeded",
			query:  fragmentQuery,
			limit:  3,
			reject: true,
		},
		{
			name:   "original unused nodes count",
			query:  unusedFragmentQuery,
			limit:  2,
			reject: true,
		},
		{
			name:   "disabled",
			query:  sourceQuery,
			limit:  0,
			reject: false,
		},
		{
			name:   "doubling bomb",
			query:  repeatedFragmentDocument(16, "__typename"),
			limit:  10000,
			reject: true,
		},
		{
			name:   "deep bomb",
			query:  repeatedFragmentDocument(198, "hello"),
			limit:  10000,
			reject: true,
		},
	}

	for _, tc := range cases {
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
	const (
		query = `
			{
			  ...F
			}

			fragment F on Query {
			  ...G
			  ...G
			}

			fragment G on Query {
			  hello
			  hello
			  hello
			  hello
			  hello
			}
		`
	)

	def := limitsTestDefinition(t)
	n := NewWithOpts(WithInlineFragmentSpreads(), WithRemoveFragmentDefinitions(), WithLimits(Limits{MaxSelections: 10}))
	op, report := astparser.ParseGraphqlDocumentString(query)
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
	const (
		sourceQuery = `
			{
			  hello
			  hello
			}
		`
		wrapperQuery = `
			{
			  ... on Query {
			    hello
			  }
			  ... on Query {
			    hello
			  }
			}
		`
		flattenQuery = `
			{
			  hello
			  ... on Query {
			    hello
			    hello
			  }
			}
		`
		parentMergeQuery = `
			{
			  child {
			    a
			    b
			  }
			  child {
			    b
			    c
			  }
			}
		`
		inlineMergeQuery = `
			query Q($v: Boolean!) {
			  ... on Query @include(if: $v) {
			    hello
			    hello
			  }
			  ... on Query @include(if: $v) {
			    hello
			    hello
			  }
			}
		`
		spreadQuery = `
			{
			  ...F
			  ...F
			  ...F
			}

			fragment F on Query {
			  hello
			}
		`
		parentWidthQuery = `
			{
			  a: hello
			  b: hello
			  child {
			    a
			  }
			  ... on Query {
			    hello
			  }
			}
		`
		childWidthQuery = `
			{
			  child {
			    a
			    b
			    c
			  }
			  ... on Query {
			    hello
			  }
			}
		`
		separateChildrenQuery = `
			{
			  child {
			    a
			    b
			  }
			  other {
			    b
			    c
			  }
			}
		`
	)

	cases := []struct {
		name   string
		query  string
		limit  int
		reject bool
	}{
		{
			name:   "source exact",
			query:  sourceQuery,
			limit:  2,
			reject: false,
		},
		{
			name:   "source exceeded",
			query:  sourceQuery,
			limit:  1,
			reject: true,
		},
		{
			name:   "wrappers are entries",
			query:  wrapperQuery,
			limit:  1,
			reject: true,
		},
		{
			name:   "flatten exact",
			query:  flattenQuery,
			limit:  3,
			reject: false,
		},
		{
			name:   "flatten exceeded",
			query:  flattenQuery,
			limit:  2,
			reject: true,
		},
		{
			name:   "parent merge exact",
			query:  parentMergeQuery,
			limit:  4,
			reject: false,
		},
		{
			name:   "parent merge exceeded",
			query:  parentMergeQuery,
			limit:  3,
			reject: true,
		},
		{
			name:   "inline merge exceeded",
			query:  inlineMergeQuery,
			limit:  3,
			reject: true,
		},
		{
			name:   "source spreads count",
			query:  spreadQuery,
			limit:  2,
			reject: true,
		},
		{
			name:   "inline wrapper after child field shares parent width",
			query:  parentWidthQuery,
			limit:  3,
			reject: true,
		},
		{
			name:   "inline wrapper after child field does not use child width",
			query:  childWidthQuery,
			limit:  3,
			reject: false,
		},
		{
			name:   "separate children",
			query:  separateChildrenQuery,
			limit:  2,
			reject: false,
		},
		{
			name:   "disabled",
			query:  sourceQuery,
			limit:  0,
			reject: false,
		},
		{
			name:   "default exact",
			query:  "{" + strings.Repeat("hello ", 900) + "}",
			limit:  900,
			reject: false,
		},
		{
			name:   "default exceeded",
			query:  "{" + strings.Repeat("hello ", 901) + "}",
			limit:  900,
			reject: true,
		},
	}

	for _, tc := range cases {
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
	const query = `
		{
		  child {
		    a
		    b
		  }
		  child {
		    b
		    c
		  }
		}
	`

	def := limitsTestDefinition(t)
	n := NewWithOpts(WithLimits(Limits{MaxSelectionSetSize: 3}))
	op, r := astparser.ParseGraphqlDocumentString(query)
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
	const query = `
		{
		  hello
		  ...F
		}

		fragment F on Query {
		  hello
		  hello
		}
	`

	def := limitsTestDefinition(t)
	op, report := astparser.ParseGraphqlDocumentString(query)
	require.False(t, report.HasErrors(), report.Error())
	n := NewWithOpts(WithLimits(Limits{MaxSelections: 10, MaxSelectionSetSize: 3}))
	n.NormalizeOperation(&op, &def, &report)
	require.False(t, report.HasErrors(), report.Error())
	assert.Empty(t, op.Index.ReplacedFragmentSpreads)
	assert.Empty(t, op.InlineFragments)
}

type selectionLimitFieldCounter struct{ fields int }

func (v *selectionLimitFieldCounter) EnterField(ref int) { v.fields++ }

func TestSelectionSetLimitIncludesDeferPlaceholder(t *testing.T) {
	const query = `
		{
		  child {
		    ... @defer {
		      a
		      b
		    }
		  }
		}
	`

	t.Parallel()
	for _, limit := range []int{0, 2, 3} {
		t.Run(fmt.Sprint(limit), func(t *testing.T) {
			t.Parallel()
			def := limitsTestDefinition(t)
			op, report := astparser.ParseGraphqlDocumentString(query)
			require.False(t, report.HasErrors(), report.Error())
			original := len(op.Selections)
			n := NewWithOpts(WithEnableDefer(), WithLimits(Limits{MaxSelectionSetSize: limit}))
			counter := &selectionLimitFieldCounter{}
			for _, stage := range n.operationWalkers {
				if stage.name == "deduplicateFields, deleteUnusedVariables" {
					stage.walker.RegisterEnterFieldVisitor(counter)
				}
			}
			n.NormalizeOperation(&op, &def, &report)
			assert.Len(t, op.Selections, original+1)
			if limit == 2 {
				assertNormalizationLimit(t, report, "max_selection_set_size")
				assert.Equal(t, 1, counter.fields)
			} else {
				require.False(t, report.HasErrors(), report.Error())
				assert.Equal(t, 4, counter.fields)
			}
		})
	}
}

func TestCleanupSelectionSetLimit(t *testing.T) {
	t.Parallel()
	for _, enableDefer := range []bool{false, true} {
		for _, limit := range []int{0, 2, 3} {
			t.Run(fmt.Sprintf("defer_%t/limit_%d", enableDefer, limit), func(t *testing.T) {
				t.Parallel()
				opts := []Option{WithLimits(Limits{MaxSelectionSetSize: limit})}
				if enableDefer {
					opts = append(opts, WithEnableDefer())
				}
				n := NewWithOpts(opts...)
				def := limitsTestDefinition(t)
				op, report := astparser.ParseGraphqlDocumentString(`{a:hello b:hello c:hello}`)
				require.False(t, report.HasErrors(), report.Error())
				found := false
				for _, stage := range n.operationWalkers {
					if stage.name == "deduplicateFields, deleteUnusedVariables" {
						found = true
						stage.walker.Walk(&op, &def, &report)
						break
					}
				}
				require.True(t, found)
				if limit == 2 {
					assertNormalizationLimit(t, report, "max_selection_set_size")
				} else {
					assert.False(t, report.HasErrors(), report.Error())
				}
			})
		}
	}
}

func TestNormalizationLimitsOnlyExpandSelectedOperation(t *testing.T) {
	const selectedQuery = `
		query Small {
		  hello
		  hello
		}
	`

	def := limitsTestDefinition(t)
	op, report := astparser.ParseGraphqlDocumentString(selectedQuery + repeatedFragmentDocument(18, "hello"))
	require.False(t, report.HasErrors(), report.Error())
	original := len(op.Selections)
	n := NewWithOpts(WithInlineFragmentSpreads(), WithRemoveNotMatchingOperationDefinitions(), WithRemoveFragmentDefinitions(), WithLimits(Limits{MaxSelections: 64, MaxSelectionSetSize: 4}))
	n.NormalizeNamedOperation(&op, &def, []byte("Small"), &report)
	require.False(t, report.HasErrors(), report.Error())
	assert.Empty(t, op.Index.ReplacedFragmentSpreads)
	activeRoots := 0
	for _, root := range op.RootNodes {
		if root.Kind != ast.NodeKindUnknown {
			activeRoots++
		}
	}
	assert.Equal(t, 1, activeRoots)
	assert.Equal(t, original, len(op.Selections))
	assert.Len(t, op.SelectionSets[op.OperationDefinitions[0].SelectionSet].SelectionRefs, 1)
}

func BenchmarkNormalizationLimits(b *testing.B) {
	definition := limitsTestDefinition(b)
	for _, levels := range []int{8, 18, 198} {
		b.Run(fmt.Sprintf("levels_%d", levels), func(b *testing.B) {
			query := repeatedFragmentDocument(levels, "hello")
			n := NewWithOpts(WithInlineFragmentSpreads(), WithRemoveFragmentDefinitions(), WithLimits(Limits{MaxSelections: 10000, MaxSelectionSetSize: 900}))
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
	for i := range 900 {
		fmt.Fprintf(&flat, "a%d:hello ", i)
	}
	flat.WriteString("}")
	wrappers.WriteString("query Q {" + strings.Repeat("...F4 ", 800) + "} fragment F0 on Query {hello} ")
	for i := 1; i <= 4; i++ {
		fmt.Fprintf(&wrappers, "fragment F%d on Query {...F%d} ", i, i-1)
	}
	queries := []struct {
		name     string
		document string
	}{
		{
			name:     "small",
			document: "query Q { hello child { a b } }",
		},
		{
			name:     "fields_900",
			document: flat.String(),
		},
		{
			name:     "spreads_900",
			document: "query Q {" + strings.Repeat("...F ", 900) + "} fragment F on Query {hello}",
		},
		{
			name:     "wrapped_spreads_800",
			document: wrappers.String(),
		},
	}

	budgets := []struct {
		name   string
		limits Limits
	}{
		{
			name:   "disabled",
			limits: Limits{},
		},
		{
			name:   "selections",
			limits: Limits{MaxSelections: 10000},
		},
		{
			name:   "selection_set_size",
			limits: Limits{MaxSelectionSetSize: 900},
		},
		{
			name: "both",
			limits: Limits{
				MaxSelections:       10000,
				MaxSelectionSetSize: 900,
			},
		},
	}

	for _, query := range queries {
		for _, budget := range budgets {
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
