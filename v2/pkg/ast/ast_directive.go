package ast

import (
	"bytes"
	"io"

	"github.com/wundergraph/graphql-go-tools/pkg/lexer/literal"
	"github.com/wundergraph/graphql-go-tools/pkg/lexer/position"
)

type DirectiveList struct {
	Refs []int
}

type Directive struct {
	At           position.Position  // @
	Name         ByteSliceReference // e.g. include
	HasArguments bool
	Arguments    ArgumentList // e.g. (if: true)
}

func (l *DirectiveList) HasDirectiveByName(document *Document, name string) bool {
	for i := range l.Refs {
		if document.DirectiveNameString(l.Refs[i]) == name {
			return true
		}
	}
	return false
}

func (l *DirectiveList) RemoveDirectiveByName(document *Document, name string) {
	for i := range l.Refs {
		if document.DirectiveNameString(l.Refs[i]) == name {
			if i < len(l.Refs)-1 {
				l.Refs = append(l.Refs[:i], l.Refs[i+1:]...)
			} else {
				l.Refs = l.Refs[:i]
			}
			return
		}
	}
}

func (d *Document) CopyDirective(ref int) int {
	var arguments ArgumentList
	if d.Directives[ref].HasArguments {
		arguments = d.CopyArgumentList(d.Directives[ref].Arguments)
	}
	return d.AddDirective(Directive{
		Name:         d.copyByteSliceReference(d.Directives[ref].Name),
		HasArguments: d.Directives[ref].HasArguments,
		Arguments:    arguments,
	})
}

func (d *Document) CopyDirectiveList(list DirectiveList) DirectiveList {
	refs := d.NewEmptyRefs()
	for _, r := range list.Refs {
		refs = append(refs, d.CopyDirective(r))
	}
	return DirectiveList{Refs: refs}
}

func (d *Document) PrintDirective(ref int, w io.Writer) error {
	_, err := w.Write(literal.AT)
	if err != nil {
		return err
	}
	_, err = w.Write(d.Input.ByteSlice(d.Directives[ref].Name))
	if err != nil {
		return err
	}
	if d.Directives[ref].HasArguments {
		err = d.PrintArguments(d.Directives[ref].Arguments.Refs, w)
	}
	return err
}

func (d *Document) DirectiveName(ref int) ByteSliceReference {
	return d.Directives[ref].Name
}

func (d *Document) DirectiveNameBytes(ref int) ByteSlice {
	return d.Input.ByteSlice(d.Directives[ref].Name)
}

func (d *Document) DirectiveNameString(ref int) string {
	return d.Input.ByteSliceString(d.Directives[ref].Name)
}

func (d *Document) DirectiveIsFirst(directive int, ancestor Node) bool {
	directives := d.NodeDirectives(ancestor)
	return len(directives) != 0 && directives[0] == directive
}

func (d *Document) DirectiveIsLast(directive int, ancestor Node) bool {
	directives := d.NodeDirectives(ancestor)
	return len(directives) != 0 && directives[len(directives)-1] == directive
}

func (d *Document) DirectiveArgumentSet(ref int) []int {
	return d.Directives[ref].Arguments.Refs
}

func (d *Document) DirectiveArgumentValueByName(ref int, name ByteSlice) (Value, bool) {
	for i := 0; i < len(d.Directives[ref].Arguments.Refs); i++ {
		arg := d.Directives[ref].Arguments.Refs[i]
		if bytes.Equal(d.ArgumentNameBytes(arg), name) {
			return d.ArgumentValue(arg), true
		}
	}
	return Value{}, false
}

func (d *Document) DirectivesAreEqual(left, right int) bool {
	return d.Input.ByteSliceReferenceContentEquals(d.DirectiveName(left), d.DirectiveName(right)) &&
		d.ArgumentSetsAreEquals(d.DirectiveArgumentSet(left), d.DirectiveArgumentSet(right))
}

func (d *Document) DirectiveSetsAreEqual(left, right []int) bool {
	if len(left) != len(right) {
		return false
	}
	for i := 0; i < len(left); i++ {
		leftDirective, rightDirective := left[i], right[i]
		if !d.DirectivesAreEqual(leftDirective, rightDirective) {
			return false
		}
	}
	return true
}

func (d *Document) AddDirective(directive Directive) (ref int) {
	d.Directives = append(d.Directives, directive)
	return len(d.Directives) - 1
}

func (d *Document) ImportDirective(name string, argRefs []int) (ref int) {
	directive := Directive{
		Name:         d.Input.AppendInputString(name),
		HasArguments: len(argRefs) > 0,
		Arguments: ArgumentList{
			Refs: argRefs,
		},
	}

	return d.AddDirective(directive)
}

func (d *Document) AddDirectiveToNode(directiveRef int, node Node) bool {
	switch node.Kind {
	case NodeKindField:
		d.Fields[node.Ref].Directives.Refs = append(d.Fields[node.Ref].Directives.Refs, directiveRef)
		d.Fields[node.Ref].HasDirectives = true
		return true
	case NodeKindVariableDefinition:
		d.VariableDefinitions[node.Ref].Directives.Refs = append(d.VariableDefinitions[node.Ref].Directives.Refs, directiveRef)
		d.VariableDefinitions[node.Ref].HasDirectives = true
		return true
	case NodeKindOperationDefinition:
		d.OperationDefinitions[node.Ref].Directives.Refs = append(d.OperationDefinitions[node.Ref].Directives.Refs, directiveRef)
		d.OperationDefinitions[node.Ref].HasDirectives = true
		return true
	case NodeKindInputValueDefinition:
		d.InputValueDefinitions[node.Ref].Directives.Refs = append(d.InputValueDefinitions[node.Ref].Directives.Refs, directiveRef)
		d.InputValueDefinitions[node.Ref].HasDirectives = true
		return true
	case NodeKindInlineFragment:
		d.InlineFragments[node.Ref].Directives.Refs = append(d.InlineFragments[node.Ref].Directives.Refs, directiveRef)
		d.InlineFragments[node.Ref].HasDirectives = true
		return true
	case NodeKindFragmentSpread:
		d.FragmentSpreads[node.Ref].Directives.Refs = append(d.FragmentSpreads[node.Ref].Directives.Refs, directiveRef)
		d.FragmentSpreads[node.Ref].HasDirectives = true
		return true
	case NodeKindFragmentDefinition:
		d.FragmentDefinitions[node.Ref].Directives.Refs = append(d.FragmentDefinitions[node.Ref].Directives.Refs, directiveRef)
		d.FragmentDefinitions[node.Ref].HasDirectives = true
		return true
	default:
		return false
	}
}

func (d *Document) DirectiveIsAllowedOnNodeKind(directiveName string, kind NodeKind, operationType OperationType) bool {
	definition, ok := d.DirectiveDefinitionByName(directiveName)
	if !ok {
		return false
	}

	switch kind {
	case NodeKindOperationDefinition:
		switch operationType {
		case OperationTypeQuery:
			return d.DirectiveDefinitions[definition].DirectiveLocations.Get(ExecutableDirectiveLocationQuery)
		case OperationTypeMutation:
			return d.DirectiveDefinitions[definition].DirectiveLocations.Get(ExecutableDirectiveLocationMutation)
		case OperationTypeSubscription:
			return d.DirectiveDefinitions[definition].DirectiveLocations.Get(ExecutableDirectiveLocationSubscription)
		}
	case NodeKindField:
		return d.DirectiveDefinitions[definition].DirectiveLocations.Get(ExecutableDirectiveLocationField)
	case NodeKindFragmentDefinition:
		return d.DirectiveDefinitions[definition].DirectiveLocations.Get(ExecutableDirectiveLocationFragmentDefinition)
	case NodeKindFragmentSpread:
		return d.DirectiveDefinitions[definition].DirectiveLocations.Get(ExecutableDirectiveLocationFragmentSpread)
	case NodeKindInlineFragment:
		return d.DirectiveDefinitions[definition].DirectiveLocations.Get(ExecutableDirectiveLocationInlineFragment)
	case NodeKindVariableDefinition:
		return d.DirectiveDefinitions[definition].DirectiveLocations.Get(ExecutableDirectiveLocationVariableDefinition)
	}

	return false
}
