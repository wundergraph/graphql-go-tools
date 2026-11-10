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
	const operation = `
query Q {
  ...F%d
}

fragment F0 on Query {
  %s
}
`
	const fragment = `
fragment F%d on Query {
  ...F%d
  ...F%d
}
`
	var out strings.Builder
	fmt.Fprintf(&out, operation, levels, leaf)
	for i := 1; i <= levels; i++ {
		fmt.Fprintf(&out, fragment, i, i-1, i-1)
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
	const nextQuery = `
		{
		  hello
		}
	`
	next, r := astparser.ParseGraphqlDocumentString(nextQuery)
	n.NormalizeOperation(&next, &def, &r)
	assert.False(t, r.HasErrors(), r.Error())
}

func TestNormalizationLimitsDefaultAndOverride(t *testing.T) {
	t.Parallel()
	assert.Equal(t, Limits{}, NewNormalizer(true, false).options.limits)
	assert.Equal(t, Limits{}, NewWithOpts().options.limits)
	n := NewWithOpts(
		WithLimits(Limits{
			MaxSelections: 1,
		}),
		WithLimits(Limits{}),
	)
	assert.Equal(t, Limits{}, n.options.limits)
}

func TestNormalizationLimitsPreserveDisabledFragmentInlining(t *testing.T) {
	def := limitsTestDefinition(t)
	op, report := astparser.ParseGraphqlDocumentString(limitFragmentQuery)
	require.False(t, report.HasErrors(), report.Error())
	n := NewWithOpts(WithLimits(Limits{MaxSelections: 10}))
	n.NormalizeOperation(&op, &def, &report)
	require.False(t, report.HasErrors(), report.Error())
	assert.Empty(t, op.Index.ReplacedFragmentSpreads)
	assert.Empty(t, op.InlineFragments)
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
	n := NewWithOpts(WithInlineFragmentSpreads(), WithRemoveNotMatchingOperationDefinitions(), WithRemoveFragmentDefinitions(), WithLimits(Limits{MaxSelections: 64}))
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
			n := NewWithOpts(WithInlineFragmentSpreads(), WithRemoveFragmentDefinitions(), WithLimits(Limits{MaxSelections: 10000}))
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

func TestNormalizationSelectionLimitIncludesGeneratedPlaceholders(t *testing.T) {
	const query = `
		query Q {
		  child {
		    ... @defer {
		      a
		      b
		    }
		  }
		}
	`
	cases := []struct {
		name  string
		named bool
	}{
		{
			name: "operation",
		},
		{
			name:  "named operation",
			named: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			def := limitsTestDefinition(t)
			op, report := astparser.ParseGraphqlDocumentString(query)
			require.False(t, report.HasErrors(), report.Error())
			original := len(op.Selections)
			n := NewWithOpts(WithEnableDefer(), WithLimits(Limits{MaxSelections: original}))
			if tc.named {
				n.NormalizeNamedOperation(&op, &def, []byte("Q"), &report)
			} else {
				n.NormalizeOperation(&op, &def, &report)
			}
			require.Len(t, op.Selections, original+1)
			assertNormalizationLimit(t, report, "max_selections")
		})
	}
}
