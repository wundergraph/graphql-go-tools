package ast_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astparser"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astprinter"
)

func TestFragmentSpreadCopyBudget(t *testing.T) {
	t.Parallel()

	const query = `{ ...F } fragment F on Query { child(id: 1) { alias: hello @skip(if: false) ... on Child { hello } ...G } } fragment G on Child { hello }`
	// Five copied selections (including a nested spread) and one wrapper.
	check := func(t *testing.T, budget int, wantSuccess bool) {
		t.Helper()
		op, report := astparser.ParseGraphqlDocumentString(query)
		require.False(t, report.HasErrors(), report.Error())
		original := len(op.Selections)
		set := op.OperationDefinitions[0].SelectionSet
		selection := op.SelectionSets[set].SelectionRefs[0]
		spread := op.Selections[selection].Ref
		fragment := op.FragmentDefinitions[0]
		remaining := budget
		ok := op.ReplaceFragmentSpreadWithInlineFragmentWithBudget(set, spread, fragment.SelectionSet, fragment.TypeCondition, ast.DirectiveList{}, &remaining)
		assert.Equal(t, wantSuccess, ok)
		assert.GreaterOrEqual(t, remaining, 0)
		assert.LessOrEqual(t, len(op.Selections)-original, budget)
		if !ok {
			assert.Zero(t, remaining)
			assert.Equal(t, selection, op.SelectionSets[set].SelectionRefs[0], "do not link a partial copy")
			return
		}
		assert.Equal(t, budget-6, remaining)
		assert.Equal(t, original+6, len(op.Selections))
		unlimited, report := astparser.ParseGraphqlDocumentString(query)
		require.False(t, report.HasErrors(), report.Error())
		unlimited.ReplaceFragmentSpreadWithInlineFragment(set, spread, fragment.SelectionSet, fragment.TypeCondition, ast.DirectiveList{})
		want, err := astprinter.PrintString(&unlimited)
		require.NoError(t, err)
		got, err := astprinter.PrintString(&op)
		require.NoError(t, err)
		assert.Equal(t, want, got)
	}

	t.Run("zero budget", func(t *testing.T) {
		t.Parallel()
		check(t, 0, false)
	})
	t.Run("wrapper only", func(t *testing.T) {
		t.Parallel()
		check(t, 1, false)
	})
	t.Run("two selections", func(t *testing.T) {
		t.Parallel()
		check(t, 2, false)
	})
	t.Run("three selections", func(t *testing.T) {
		t.Parallel()
		check(t, 3, false)
	})
	t.Run("four selections", func(t *testing.T) {
		t.Parallel()
		check(t, 4, false)
	})
	t.Run("one selection short", func(t *testing.T) {
		t.Parallel()
		check(t, 5, false)
	})
	t.Run("exact budget", func(t *testing.T) {
		t.Parallel()
		check(t, 6, true)
	})
	t.Run("spare budget", func(t *testing.T) {
		t.Parallel()
		check(t, 100, true)
	})
}
