package ast_test

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astparser"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astprinter"
)

func TestFragmentSpreadCopyBudget(t *testing.T) {
	const query = `{ ...F } fragment F on Query { child(id: 1) { alias: hello @skip(if: false) ... on Child { hello } ...G } } fragment G on Child { hello }`
	// Five copied selections (including a nested spread) and one wrapper.
	for _, budget := range []int{0, 1, 2, 3, 4, 5, 6, 100} {
		t.Run(fmt.Sprint(budget), func(t *testing.T) {
			op, report := astparser.ParseGraphqlDocumentString(query)
			require.False(t, report.HasErrors())
			original := len(op.Selections)
			set := op.OperationDefinitions[0].SelectionSet
			selection := op.SelectionSets[set].SelectionRefs[0]
			spread := op.Selections[selection].Ref
			fragment := op.FragmentDefinitions[0]
			remaining := budget
			ok := op.ReplaceFragmentSpreadWithInlineFragmentWithBudget(set, spread, fragment.SelectionSet, fragment.TypeCondition, ast.DirectiveList{}, &remaining)
			require.Equal(t, budget >= 6, ok)
			require.GreaterOrEqual(t, remaining, 0)
			require.LessOrEqual(t, len(op.Selections)-original, budget)
			if !ok {
				require.Zero(t, remaining)
				require.Equal(t, selection, op.SelectionSets[set].SelectionRefs[0], "do not link a partial copy")
				return
			}
			require.Equal(t, budget-6, remaining)
			require.Equal(t, original+6, len(op.Selections))
			unlimited, report := astparser.ParseGraphqlDocumentString(query)
			require.False(t, report.HasErrors())
			unlimited.ReplaceFragmentSpreadWithInlineFragment(set, spread, fragment.SelectionSet, fragment.TypeCondition, ast.DirectiveList{})
			want, err := astprinter.PrintString(&unlimited)
			require.NoError(t, err)
			got, err := astprinter.PrintString(&op)
			require.NoError(t, err)
			require.Equal(t, want, got)
		})
	}
}
