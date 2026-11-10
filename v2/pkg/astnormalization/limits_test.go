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

const limitSourceQuery = `
	{
	  hello
	  hello
	}
`

const limitFragmentQuery = `
	{
	  ...F
	}

	fragment F on Query {
	  hello
	}
`

const limitParentMergeQuery = `
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

func TestNormalizationLimits(t *testing.T) {
	t.Parallel()

	const multipleOperations = `
		query A {
		  hello
		}

		query B {
		  hello
		}
	`

	const flattenQuery = `
		{
		  hello
		  ... on Query {
		    hello
		    hello
		  }
		}
	`

	const inlineMergeQuery = `
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

	const parentWidthQuery = `
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

	const childWidthQuery = `
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

	cases := []struct {
		name      string
		query     string
		limits    Limits
		wantLimit string
	}{
		{
			name:   "source selections exact",
			query:  limitSourceQuery,
			limits: Limits{MaxSelections: 2},
		},
		{
			name:      "source selections exceeded",
			query:     limitSourceQuery,
			limits:    Limits{MaxSelections: 1},
			wantLimit: "max_selections",
		},
		{
			name:      "document scope",
			query:     multipleOperations,
			limits:    Limits{MaxSelections: 1},
			wantLimit: "max_selections",
		},
		{
			name:   "expansion exact",
			query:  limitFragmentQuery,
			limits: Limits{MaxSelections: 4},
		},
		{
			name:      "expansion exceeded",
			query:     limitFragmentQuery,
			limits:    Limits{MaxSelections: 3},
			wantLimit: "max_selections",
		},
		{
			name:      "doubling bomb",
			query:     repeatedFragmentDocument(16, "__typename"),
			limits:    Limits{MaxSelections: 10000},
			wantLimit: "max_selections",
		},
		{
			name:      "deep bomb",
			query:     repeatedFragmentDocument(198, "hello"),
			limits:    Limits{MaxSelections: 10000},
			wantLimit: "max_selections",
		},
		{
			name:   "source list exact",
			query:  limitSourceQuery,
			limits: Limits{MaxSelectionSetSize: 2},
		},
		{
			name:   "flatten exact",
			query:  flattenQuery,
			limits: Limits{MaxSelectionSetSize: 3},
		},
		{
			name:      "flatten exceeded",
			query:     flattenQuery,
			limits:    Limits{MaxSelectionSetSize: 2},
			wantLimit: "max_selection_set_size",
		},
		{
			name:   "parent merge exact",
			query:  limitParentMergeQuery,
			limits: Limits{MaxSelectionSetSize: 4},
		},
		{
			name:      "inline merge exceeded",
			query:     inlineMergeQuery,
			limits:    Limits{MaxSelectionSetSize: 3},
			wantLimit: "max_selection_set_size",
		},
		{
			name:      "source spreads count",
			query:     repeatedFragmentDocument(1, "hello"),
			limits:    Limits{MaxSelectionSetSize: 1},
			wantLimit: "max_selection_set_size",
		},
		{
			name:      "inline wrapper after child field shares parent width",
			query:     parentWidthQuery,
			limits:    Limits{MaxSelectionSetSize: 3},
			wantLimit: "max_selection_set_size",
		},
		{
			name:   "inline wrapper after child field does not use child width",
			query:  childWidthQuery,
			limits: Limits{MaxSelectionSetSize: 3},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			def := limitsTestDefinition(t)
			op, report := astparser.ParseGraphqlDocumentString(tc.query)
			require.False(t, report.HasErrors(), report.Error())
			n := NewWithOpts(
				WithInlineFragmentSpreads(),
				WithRemoveFragmentDefinitions(),
				WithLimits(tc.limits),
			)
			n.NormalizeOperation(&op, &def, &report)
			if tc.wantLimit != "" {
				assertNormalizationLimit(t, report, tc.wantLimit)
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

func TestSelectionSetLimitStopsBeforeMergeAndReuses(t *testing.T) {
	def := limitsTestDefinition(t)
	n := NewWithOpts(WithLimits(Limits{MaxSelectionSetSize: 3}))
	op, r := astparser.ParseGraphqlDocumentString(limitParentMergeQuery)
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
	t.Parallel()
	assert.Equal(t, Limits{}, NewNormalizer(true, false).options.limits)
	assert.Equal(t, Limits{}, NewWithOpts().options.limits)
	n := NewWithOpts(
		WithLimits(Limits{
			MaxSelections:       1,
			MaxSelectionSetSize: 1,
		}),
		WithLimits(Limits{}),
	)
	assert.Equal(t, Limits{}, n.options.limits)
}

func TestNormalizationLimitsPreserveDisabledFragmentInlining(t *testing.T) {
	def := limitsTestDefinition(t)
	op, report := astparser.ParseGraphqlDocumentString(limitFragmentQuery)
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

func TestCleanupSelectionSetLimitWithoutDefer(t *testing.T) {
	t.Parallel()
	const query = `
        {
            a: hello
            b: hello
            c: hello
        }
    `
	limits := []int{0, 2, 3}
	for _, limit := range limits {
		t.Run(fmt.Sprint(limit), func(t *testing.T) {
			t.Parallel()
			n := NewWithOpts(WithLimits(Limits{MaxSelectionSetSize: limit}))
			def := limitsTestDefinition(t)
			op, report := astparser.ParseGraphqlDocumentString(query)
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
