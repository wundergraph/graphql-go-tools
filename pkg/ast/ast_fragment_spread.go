package ast

import (
	"github.com/jensneuse/graphql-go-tools/internal/pkg/unsafebytes"
	"github.com/jensneuse/graphql-go-tools/pkg/lexer/position"
)

// InlineFragment
// example:
// ... on User {
//      friends {
//        count
//      }
//    }
type InlineFragment struct {
	Spread        position.Position // ...
	TypeCondition TypeCondition     // on NamedType, e.g. on User
	HasDirectives bool
	Directives    DirectiveList // optional, e.g. @foo
	SelectionSet  int           // optional, e.g. { nextField }
	HasSelections bool
}

func (d *Document) InlineFragmentTypeConditionName(ref int) ByteSlice {
	if d.InlineFragments[ref].TypeCondition.Type == -1 {
		return nil
	}
	return d.Input.ByteSlice(d.Types[d.InlineFragments[ref].TypeCondition.Type].Name)
}

func (d *Document) InlineFragmentTypeConditionNameString(ref int) string {
	return unsafebytes.BytesToString(d.InlineFragmentTypeConditionName(ref))
}

func (d *Document) InlineFragmentHasTypeCondition(ref int) bool {
	return d.InlineFragments[ref].TypeCondition.Type != -1
}

func (d *Document) InlineFragmentHasDirectives(ref int) bool {
	return len(d.InlineFragments[ref].Directives.Refs) != 0
}

func (d *Document) InlineFragmentSelections(ref int) []int {
	if !d.InlineFragments[ref].HasSelections {
		return nil
	}
	return d.SelectionSets[d.InlineFragments[ref].SelectionSet].SelectionRefs
}

// FragmentSpread
// example:
// ...MyFragment
type FragmentSpread struct {
	Spread        position.Position  // ...
	FragmentName  ByteSliceReference // Name but not on, e.g. MyFragment
	HasDirectives bool
	Directives    DirectiveList // optional, e.g. @foo
}

func (d *Document) FragmentSpreadNameBytes(ref int) ByteSlice {
	return d.Input.ByteSlice(d.FragmentSpreads[ref].FragmentName)
}

func (d *Document) FragmentSpreadNameString(ref int) string {
	return unsafebytes.BytesToString(d.FragmentSpreadNameBytes(ref))
}

// ReplaceFragmentSpread replaces a fragment spread with a given selection set
// attention! this might lead to duplicate field problems because the same field with its unique field reference might be copied into the same selection set
// possible problems: changing directives or sub selections will affect both fields with the same id
// simple solution: run normalization deduplicate fields
// as part of the normalization flow this problem will be handled automatically
// just be careful in case you use this function outside of the normalization package
func (d *Document) ReplaceFragmentSpread(selectionSet int, spreadRef int, replaceWithSelectionSet int) {
	for i, j := range d.SelectionSets[selectionSet].SelectionRefs {
		if d.Selections[j].Kind == SelectionKindFragmentSpread && d.Selections[j].Ref == spreadRef {
			d.SelectionSets[selectionSet].SelectionRefs = append(d.SelectionSets[selectionSet].SelectionRefs[:i], append(d.SelectionSets[replaceWithSelectionSet].SelectionRefs, d.SelectionSets[selectionSet].SelectionRefs[i+1:]...)...)
			d.Index.ReplacedFragmentSpreads = append(d.Index.ReplacedFragmentSpreads, spreadRef)
			return
		}
	}
}

// ReplaceFragmentSpreadWithInlineFragment replaces a given fragment spread with a inline fragment
// attention! the same rules apply as for 'ReplaceFragmentSpread', look above!
func (d *Document) ReplaceFragmentSpreadWithInlineFragment(selectionSet int, spreadRef int, replaceWithSelectionSet int, typeCondition TypeCondition) {
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
		if d.Selections[j].Kind == SelectionKindFragmentSpread && d.Selections[j].Ref == spreadRef {
			d.SelectionSets[selectionSet].SelectionRefs = append(d.SelectionSets[selectionSet].SelectionRefs[:i], append([]int{selectionRef}, d.SelectionSets[selectionSet].SelectionRefs[i+1:]...)...)
			d.Index.ReplacedFragmentSpreads = append(d.Index.ReplacedFragmentSpreads, spreadRef)
			return
		}
	}
}
