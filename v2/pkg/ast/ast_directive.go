package ast

import (
	"bytes"
	"io"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/lexer/literal"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/lexer/position"
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

func (l *DirectiveList) HasDirectiveByNameBytes(document *Document, directiveName ByteSlice) (directiveRef int, exists bool) {
	for i := range l.Refs {
		if bytes.Equal(directiveName, document.DirectiveNameBytes(l.Refs[i])) {
			return l.Refs[i], true
		}
	}
	return InvalidRef, false
}

func (l *DirectiveList) RemoveDirectiveByName(document *Document, name string) {
	for i := range l.Refs {
		if document.DirectiveNameString(l.Refs[i]) == name {
			l.Refs = append(l.Refs[:i], l.Refs[i+1:]...)
			return
		}
	}
}

func (l *DirectiveList) RemoveDirectiveByRef(directiveRef int) {
	for i := range l.Refs {
		if l.Refs[i] == directiveRef {
			l.Refs = append(l.Refs[:i], l.Refs[i+1:]...)
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

// DirectiveSetsAreEqual reports whether two directive sets are equal as
// multisets, ignoring @__defer_internal directives (an internal planning
// concern). Directives may be repeatable, so each directive in one set must
// have a distinct matching directive in the other set: [@cache, @cache] and
// [@cache] are NOT equal.
func (d *Document) DirectiveSetsAreEqual(left, right []int) bool {
	leftDirectives := d.directivesWithoutDeferInternal(left)
	rightDirectives := d.directivesWithoutDeferInternal(right)

	if len(leftDirectives) != len(rightDirectives) {
		return false
	}

	matched := make([]bool, len(rightDirectives))
	for _, leftDirective := range leftDirectives {
		found := false
		for j, rightDirective := range rightDirectives {
			if matched[j] {
				continue
			}
			if d.DirectivesAreEqual(leftDirective, rightDirective) {
				matched[j] = true
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	return true
}

// directivesWithoutDeferInternal returns the directive refs with any
// @__defer_internal directives filtered out.
func (d *Document) directivesWithoutDeferInternal(refs []int) []int {
	filtered := make([]int, 0, len(refs))
	for _, ref := range refs {
		if bytes.Equal(d.DirectiveNameBytes(ref), literal.DEFER_INTERNAL) {
			continue
		}
		filtered = append(filtered, ref)
	}
	return filtered
}

// DirectiveSetsHasCompatibleStreamDirective checks if directives sets contains stream directive with same arguments
func (d *Document) DirectiveSetsHasCompatibleStreamDirective(left, right []int) bool {
	leftRef, leftExists := d.DirectiveWithNameBytes(left, literal.STREAM)
	rightRef, rightExists := d.DirectiveWithNameBytes(right, literal.STREAM)

	if leftExists && rightExists {
		return d.DirectivesAreEqual(leftRef, rightRef)
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

func (d *Document) ResolveSkipDirectiveVariable(directiveRefs []int) (variableName string, exists bool) {
	if ref, ok := d.DirectiveWithNameBytes(directiveRefs, literal.SKIP); ok {
		if value, ok := d.DirectiveArgumentValueByName(ref, literal.IF); ok {
			if value.Kind == ValueKindVariable {
				return d.VariableValueNameString(value.Ref), true
			}
		}
	}
	return "", false
}

func (d *Document) ResolveIncludeDirectiveVariable(directiveRefs []int) (variableName string, exists bool) {
	if ref, ok := d.DirectiveWithNameBytes(directiveRefs, literal.INCLUDE); ok {
		if value, ok := d.DirectiveArgumentValueByName(ref, literal.IF); ok {
			if value.Kind == ValueKindVariable {
				return d.VariableValueNameString(value.Ref), true
			}
		}
	}

	return "", false
}

func (d *Document) DirectiveWithNameBytes(directiveRefs []int, name []byte) (directiveRef int, exists bool) {
	for _, i := range directiveRefs {
		if bytes.Equal(d.DirectiveNameBytes(i), name) {
			return i, true
		}
	}
	return InvalidRef, false
}
