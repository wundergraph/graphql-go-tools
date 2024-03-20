package ast

import (
	"github.com/wundergraph/graphql-go-tools/pkg/internal/unsafebytes"
	"github.com/wundergraph/graphql-go-tools/pkg/lexer/position"
)

// InlineFragment
// example:
//
//	... on User {
//	     friends {
//	       count
//	     }
//	   }
type InlineFragment struct {
	Spread        position.Position // ...
	TypeCondition TypeCondition     // on NamedType, e.g. on User
	HasDirectives bool
	Directives    DirectiveList // optional, e.g. @foo
	SelectionSet  int           // optional, e.g. { nextField }
	HasSelections bool
}

func (d *Document) CopyInlineFragment(ref int) int {
	var directives DirectiveList
	var selectionSet int
	if d.InlineFragments[ref].HasDirectives {
		directives = d.CopyDirectiveList(d.InlineFragments[ref].Directives)
	}
	if d.InlineFragments[ref].HasSelections {
		selectionSet = d.CopySelectionSet(d.InlineFragments[ref].SelectionSet)
	}
	return d.AddInlineFragment(InlineFragment{
		TypeCondition: d.InlineFragments[ref].TypeCondition, // Value type; doesn't need to be copied.
		HasDirectives: d.InlineFragments[ref].HasDirectives,
		Directives:    directives,
		SelectionSet:  selectionSet,
		HasSelections: d.InlineFragments[ref].HasSelections,
	})
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

func (d *Document) AddInlineFragment(fragment InlineFragment) int {
	d.InlineFragments = append(d.InlineFragments, fragment)
	return len(d.InlineFragments) - 1
}
