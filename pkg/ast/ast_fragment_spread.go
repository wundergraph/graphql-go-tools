package ast

import (
	"github.com/wundergraph/graphql-go-tools/internal/pkg/unsafebytes"
	"github.com/wundergraph/graphql-go-tools/pkg/lexer/position"
)

type FragmentSpreadRef int

// FragmentSpread
// example:
// ...MyFragment
type FragmentSpread struct {
	Spread        position.Position  // ...
	FragmentName  ByteSliceReference // Name but not on, e.g. MyFragment
	HasDirectives bool
	Directives    DirectiveList // optional, e.g. @foo
}

func (d *Document) CopyFragmentSpread(ref FragmentSpreadRef) FragmentSpreadRef {
	var directives DirectiveList
	if d.FragmentSpreads[ref].HasDirectives {
		directives = d.CopyDirectiveList(d.FragmentSpreads[ref].Directives)
	}
	return d.AddFragmentSpread(FragmentSpread{
		FragmentName:  d.copyByteSliceReference(d.FragmentSpreads[ref].FragmentName),
		HasDirectives: d.FragmentSpreads[ref].HasDirectives,
		Directives:    directives,
	})
}

func (d *Document) AddFragmentSpread(spread FragmentSpread) FragmentSpreadRef {
	d.FragmentSpreads = append(d.FragmentSpreads, spread)
	return FragmentSpreadRef(len(d.FragmentSpreads) - 1)
}

func (d *Document) FragmentSpreadNameBytes(ref FragmentSpreadRef) ByteSlice {
	return d.Input.ByteSlice(d.FragmentSpreads[ref].FragmentName)
}

func (d *Document) FragmentSpreadNameString(ref FragmentSpreadRef) string {
	return unsafebytes.BytesToString(d.FragmentSpreadNameBytes(ref))
}

// ReplaceFragmentSpread replaces a fragment spread with a given selection set
// attention! this might lead to duplicate field problems because the same field with its unique field reference might be copied into the same selection set
// possible problems: changing directives or sub selections will affect both fields with the same id
// simple solution: run normalization deduplicate fields
// as part of the normalization flow this problem will be handled automatically
// just be careful in case you use this function outside the normalization package
func (d *Document) ReplaceFragmentSpread(selectionSet int, spreadRef FragmentSpreadRef, replaceWithSelectionSet int) {
	for i, j := range d.SelectionSets[selectionSet].SelectionRefs {
		if d.Selections[j].Kind == SelectionKindFragmentSpread && d.Selections[j].Ref == int(spreadRef) {
			d.SelectionSets[selectionSet].SelectionRefs = append(d.SelectionSets[selectionSet].SelectionRefs[:i], append(d.SelectionSets[replaceWithSelectionSet].SelectionRefs, d.SelectionSets[selectionSet].SelectionRefs[i+1:]...)...)
			d.Index.ReplacedFragmentSpreads = append(d.Index.ReplacedFragmentSpreads, int(spreadRef))
			return
		}
	}
}

// ReplaceFragmentSpreadWithInlineFragment replaces a given fragment spread with an inline fragment
// attention! the same rules apply as for 'ReplaceFragmentSpread', look above!
func (d *Document) ReplaceFragmentSpreadWithInlineFragment(selectionSet int, spreadRef FragmentSpreadRef, replaceWithSelectionSet int, typeCondition TypeCondition) {
	d.InlineFragments = append(d.InlineFragments, InlineFragment{
		TypeCondition: typeCondition,
		SelectionSet:  replaceWithSelectionSet,
		HasSelections: len(d.SelectionSets[replaceWithSelectionSet].SelectionRefs) != 0,
	})
	ref := len(d.InlineFragments) - 1
	d.Selections = append(d.Selections, Selection{
		Kind: SelectionKindInlineFragment,
		Ref:  ref,
	})
	selectionRef := len(d.Selections) - 1
	for i, j := range d.SelectionSets[selectionSet].SelectionRefs {
		if d.Selections[j].Kind == SelectionKindFragmentSpread && d.Selections[j].Ref == int(spreadRef) {
			d.SelectionSets[selectionSet].SelectionRefs = append(d.SelectionSets[selectionSet].SelectionRefs[:i], append([]int{selectionRef}, d.SelectionSets[selectionSet].SelectionRefs[i+1:]...)...)
			d.Index.ReplacedFragmentSpreads = append(d.Index.ReplacedFragmentSpreads, int(spreadRef))
			return
		}
	}
}
