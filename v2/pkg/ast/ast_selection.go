package ast

import (
	"bytes"
	"fmt"

	"github.com/wundergraph/graphql-go-tools/v2/internal/pkg/unsafebytes"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/lexer/position"
)

type SelectionKind int

const (
	SelectionKindUnknown SelectionKind = 18 + iota
	SelectionKindField
	SelectionKindFragmentSpread
	SelectionKindInlineFragment
)

type SelectionSet struct {
	LBrace        position.Position
	RBrace        position.Position
	SelectionRefs []int
}

type Selection struct {
	Kind SelectionKind // one of Field, FragmentSpread, InlineFragment
	Ref  int           // reference to the actual selection
}

func (d *Document) CopySelection(ref int) int {
	innerRef := -1

	switch d.Selections[ref].Kind {
	case SelectionKindField:
		innerRef = d.CopyField(d.Selections[ref].Ref)
	case SelectionKindFragmentSpread:
		innerRef = d.CopyFragmentSpread(d.Selections[ref].Ref)
	case SelectionKindInlineFragment:
		innerRef = d.CopyInlineFragment(d.Selections[ref].Ref)
	}

	return d.AddSelectionToDocument(Selection{
		Kind: d.Selections[ref].Kind,
		Ref:  innerRef,
	})
}

func (d *Document) CopySelectionSet(ref int) int {
	refs := d.NewEmptyRefs()
	for _, r := range d.SelectionSets[ref].SelectionRefs {
		refs = append(refs, d.CopySelection(r))
	}
	return d.AddSelectionSetToDocument(SelectionSet{
		SelectionRefs: refs,
	})
}

func (d *Document) PrintSelections(selections []int) (out string) {
	out += "["
	for i, ref := range selections {
		out += fmt.Sprintf("%+v", d.Selections[ref])
		if i != len(selections)-1 {
			out += ","
		}
	}
	out += "]"
	return
}

func (d *Document) SelectionsBeforeField(field int, selectionSet Node) bool {
	if selectionSet.Kind != NodeKindSelectionSet {
		return false
	}

	if len(d.SelectionSets[selectionSet.Ref].SelectionRefs) == 1 {
		return false
	}

	for i, j := range d.SelectionSets[selectionSet.Ref].SelectionRefs {
		if d.Selections[j].Kind == SelectionKindField && d.Selections[j].Ref == field {
			return i != 0
		}
	}

	return false
}

func (d *Document) SelectionsAfter(selectionKind SelectionKind, selectionRef int, selectionSet Node) bool {
	if selectionSet.Kind != NodeKindSelectionSet {
		return false
	}

	if len(d.SelectionSets[selectionSet.Ref].SelectionRefs) == 1 {
		return false
	}

	for i, j := range d.SelectionSets[selectionSet.Ref].SelectionRefs {
		if d.Selections[j].Kind == selectionKind && d.Selections[j].Ref == selectionRef {
			return i != len(d.SelectionSets[selectionSet.Ref].SelectionRefs)-1
		}
	}

	return false
}

func (d *Document) SelectionsAfterField(field int, selectionSet Node) bool {
	return d.SelectionsAfter(SelectionKindField, field, selectionSet)
}

func (d *Document) SelectionsAfterInlineFragment(inlineFragment int, selectionSet Node) bool {
	return d.SelectionsAfter(SelectionKindInlineFragment, inlineFragment, selectionSet)
}

func (d *Document) SelectionsAfterFragmentSpread(fragmentSpread int, selectionSet Node) bool {
	return d.SelectionsAfter(SelectionKindFragmentSpread, fragmentSpread, selectionSet)
}

func (d *Document) AddSelectionSetToDocument(set SelectionSet) int {
	d.SelectionSets = append(d.SelectionSets, set)
	return len(d.SelectionSets) - 1
}

func (d *Document) AddSelectionSet() Node {
	return Node{
		Kind: NodeKindSelectionSet,
		Ref: d.AddSelectionSetToDocument(SelectionSet{
			SelectionRefs: d.Refs[d.NextRefIndex()][:0],
		}),
	}
}

func (d *Document) AddSelectionToDocument(selection Selection) int {
	d.Selections = append(d.Selections, selection)
	return len(d.Selections) - 1
}

func (d *Document) AddSelection(set int, selection Selection) {
	d.SelectionSets[set].SelectionRefs = append(d.SelectionSets[set].SelectionRefs, d.AddSelectionToDocument(selection))
}

func (d *Document) EmptySelectionSet(ref int) {
	d.SelectionSets[ref].SelectionRefs = d.SelectionSets[ref].SelectionRefs[:0]
}

func (d *Document) AppendSelectionSet(ref int, appendRef int) {
	d.SelectionSets[ref].SelectionRefs = append(d.SelectionSets[ref].SelectionRefs, d.SelectionSets[appendRef].SelectionRefs...)
}

func (d *Document) ReplaceSelectionOnSelectionSet(ref, replace, with int) {
	d.SelectionSets[ref].SelectionRefs = append(d.SelectionSets[ref].SelectionRefs[:replace], append(d.SelectionSets[with].SelectionRefs, d.SelectionSets[ref].SelectionRefs[replace+1:]...)...)
}

func (d *Document) RemoveFromSelectionSet(ref int, index int) {
	d.SelectionSets[ref].SelectionRefs = append(d.SelectionSets[ref].SelectionRefs[:index], d.SelectionSets[ref].SelectionRefs[index+1:]...)
}

func (d *Document) SelectionSetHasFieldSelectionWithNameOrAliasBytes(set int, nameOrAlias []byte) (exist bool, fieldRef int) {
	for _, i := range d.SelectionSets[set].SelectionRefs {
		if d.Selections[i].Kind != SelectionKindField {
			continue
		}
		fieldRef := d.Selections[i].Ref
		fieldName := d.FieldNameBytes(fieldRef)
		if bytes.Equal(fieldName, nameOrAlias) {
			return true, fieldRef
		}
		if !d.FieldAliasIsDefined(fieldRef) {
			continue
		}
		fieldAlias := d.FieldAliasBytes(fieldRef)
		if bytes.Equal(fieldAlias, nameOrAlias) {
			return true, fieldRef
		}
	}
	return false, InvalidRef
}

func (d *Document) SelectionSetHasFieldSelectionWithNameOrAliasString(set int, nameOrAlias string) (exist bool, fieldRef int) {
	return d.SelectionSetHasFieldSelectionWithNameOrAliasBytes(set, unsafebytes.StringToBytes(nameOrAlias))
}

func (d *Document) SelectionSetHasFieldSelectionWithExactName(set int, name []byte) (exist bool, fieldRef int) {
	for _, i := range d.SelectionSets[set].SelectionRefs {
		if d.Selections[i].Kind != SelectionKindField {
			continue
		}
		fieldRef := d.Selections[i].Ref
		fieldName := d.FieldNameBytes(fieldRef)
		if !bytes.Equal(fieldName, name) {
			continue
		}

		if !d.FieldAliasIsDefined(fieldRef) {
			return true, fieldRef
		}

		fieldAlias := d.FieldAliasBytes(fieldRef)
		if bytes.Equal(fieldAlias, name) {
			return true, fieldRef
		}
	}
	return false, InvalidRef
}
