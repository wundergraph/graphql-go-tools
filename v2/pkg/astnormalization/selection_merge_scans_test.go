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
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astvisitor"
)

const mergeScansDefinition = `
	type Query {
		hello: String
		child: Child
	}

	type Child {
		a: String
		b: String
		c: String
	}

	directive @tag(value: Int) on INLINE_FRAGMENT
`

func TestSelectionMergeScansPreserveOrder(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name     string
		query    string
		want     string
		register func(*astvisitor.Walker)
	}{
		{
			name: "leaves",
			query: `
				{
					a: hello
					b: hello
					c: hello
					a: hello
					a: hello
					b: hello
					c: hello
				}
			`,
			want: `
				{
					a: hello
					b: hello
					c: hello
				}
			`,
			register: deduplicateFields,
		},
		{
			name: "parents",
			query: `
				{
					a: child {
						a
					}
					b: child {
						b
					}
					c: child {
						c
					}
					a: child {
						b
					}
					a: child {
						c
					}
					b: child {
						a
					}
					c: child {
						a
					}
				}
			`,
			want: `
				{
					a: child {
						a
						b
						c
					}
					b: child {
						b
						a
					}
					c: child {
						c
						a
					}
				}
			`,
			register: mergeInlineFragmentSelections,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			def, report := astparser.ParseGraphqlDocumentString(mergeScansDefinition)
			require.False(t, report.HasErrors(), report.Error())
			require.NoError(t, asttransform.MergeDefinitionWithBaseSchema(&def))
			op, report := astparser.ParseGraphqlDocumentString(tc.query)
			require.False(t, report.HasErrors(), report.Error())
			refs := op.SelectionSets[op.OperationDefinitions[0].SelectionSet].SelectionRefs
			fields := make([]int, len(refs))
			for i, ref := range refs {
				fields[i] = op.Selections[ref].Ref
			}
			var merged [][2]int
			op.OnMergeFields = func(left, right int) { merged = append(merged, [2]int{left, right}) }
			walker := astvisitor.NewWalker(8)
			tc.register(&walker)
			walker.Walk(&op, &def, &report)
			require.False(t, report.HasErrors(), report.Error())
			printed, err := astprinter.PrintString(&op)
			require.NoError(t, err)
			expected, report := astparser.ParseGraphqlDocumentString(tc.want)
			require.False(t, report.HasErrors(), report.Error())
			want, err := astprinter.PrintString(&expected)
			require.NoError(t, err)
			assert.Equal(t, want, printed)
			assert.Equal(t, [][2]int{{fields[0], fields[3]}, {fields[0], fields[4]}, {fields[1], fields[5]}, {fields[2], fields[6]}}, merged)
		})
	}
	t.Run("inline fragments", func(t *testing.T) {
		t.Parallel()
		def, report := astparser.ParseGraphqlDocumentString(mergeScansDefinition)
		require.False(t, report.HasErrors(), report.Error())
		require.NoError(t, asttransform.MergeDefinitionWithBaseSchema(&def))
		op, report := astparser.ParseGraphqlDocumentString(`
			{
				... on Query @tag(value: 1) {
					hello
				}
				... on Query @tag(value: 2) {
					hello
				}
				child {
					a
				}
				... on Query @tag(value: 1) {
					a: hello
				}
				... on Query @tag(value: 1) {
					b: hello
				}
				... on Query @tag(value: 2) {
					c: hello
				}
			}
		`)
		require.False(t, report.HasErrors(), report.Error())
		walker := astvisitor.NewWalker(8)
		mergeInlineFragmentSelections(&walker)
		walker.Walk(&op, &def, &report)
		require.False(t, report.HasErrors(), report.Error())
		printed, err := astprinter.PrintString(&op)
		require.NoError(t, err)
		expected, report := astparser.ParseGraphqlDocumentString(`
			{
				... on Query @tag(value: 1) {
					hello
					a: hello
					b: hello
				}
				... on Query @tag(value: 2) {
					hello
					c: hello
				}
				child {
					a
				}
			}
		`)
		require.False(t, report.HasErrors(), report.Error())
		want, err := astprinter.PrintString(&expected)
		require.NoError(t, err)
		assert.Equal(t, want, printed)
	})
}

func BenchmarkSelectionMergeScans(b *testing.B) {
	cases := []struct {
		name            string
		selectionFormat string
		register        func(*astvisitor.Walker)
	}{
		{
			name:            "leaves",
			selectionFormat: "a%d: hello\n",
			register:        deduplicateFields,
		},
		{
			name: "parents",
			selectionFormat: `
				a%d: child {
					a
				}
			`,
			register: mergeInlineFragmentSelections,
		},
		{
			name: "inline_fragments",
			selectionFormat: `
				... on Query @tag(value: %d) {
					hello
				}
			`,
			register: mergeInlineFragmentSelections,
		},
	}
	for _, tc := range cases {
		b.Run(tc.name, func(b *testing.B) {
			var selections strings.Builder
			for i := range 128 {
				fmt.Fprintf(&selections, tc.selectionFormat, i)
			}
			// Each selection appears twice, with other selections between the copies.
			query := "{\n" + strings.Repeat(selections.String(), 2) + "}"
			def, report := astparser.ParseGraphqlDocumentString(mergeScansDefinition)
			if report.HasErrors() {
				b.Fatal(report.Error())
			}
			if err := asttransform.MergeDefinitionWithBaseSchema(&def); err != nil {
				b.Fatal(err)
			}
			walker := astvisitor.NewWalker(8)
			tc.register(&walker)
			parser := astparser.NewParser()
			op := ast.NewSmallDocument()
			run := func() {
				op.Reset()
				report.Reset()
				op.Input.ResetInputString(query)
				parser.Parse(op, &report)
				if report.HasErrors() {
					b.Fatal(report.Error())
				}
				walker.Walk(op, &def, &report)
				if report.HasErrors() {
					b.Fatal(report.Error())
				}
			}
			run()
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				run()
			}
		})
	}
}
