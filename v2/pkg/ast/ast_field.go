package ast

import (
	"bytes"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/internal/unsafebytes"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/lexer/position"
)

type Field struct {
	Alias         Alias              // optional, e.g. renamed:
	Name          ByteSliceReference // field name, e.g. id
	HasArguments  bool
	Arguments     ArgumentList // optional
	HasDirectives bool
	Directives    DirectiveList // optional
	SelectionSet  int           // optional
	HasSelections bool
	Position      position.Position
}

func (d *Document) CopyField(ref int) int {
	var arguments ArgumentList
	var directives DirectiveList
	var selectionSet int
	if d.Fields[ref].HasArguments {
		arguments = d.CopyArgumentList(d.Fields[ref].Arguments)
	}
	if d.Fields[ref].HasDirectives {
		directives = d.CopyDirectiveList(d.Fields[ref].Directives)
	}
	if d.Fields[ref].HasSelections {
		selectionSet = d.CopySelectionSet(d.Fields[ref].SelectionSet)
	}
	return d.AddField(Field{
		Name:          d.copyByteSliceReference(d.Fields[ref].Name),
		Alias:         d.CopyAlias(d.Fields[ref].Alias),
		HasArguments:  d.Fields[ref].HasArguments,
		Arguments:     arguments,
		HasDirectives: d.Fields[ref].HasDirectives,
		Directives:    directives,
		HasSelections: d.Fields[ref].HasSelections,
		SelectionSet:  selectionSet,
	}).Ref
}

func (d *Document) FieldNameBytes(ref int) ByteSlice {
	return d.Input.ByteSlice(d.Fields[ref].Name)
}

// FieldNameUnsafeString - returns field name as a string which is unsafe pointer to document input content
func (d *Document) FieldNameUnsafeString(ref int) string {
	return unsafebytes.BytesToString(d.Input.ByteSlice(d.Fields[ref].Name))
}

// FieldNameString - returns fied name as a string value
func (d *Document) FieldNameString(ref int) string {
	return string(d.Input.ByteSlice(d.Fields[ref].Name))
}

func (d *Document) AddField(field Field) Node {
	d.Fields = append(d.Fields, field)
	return Node{
		Kind: NodeKindField,
		Ref:  len(d.Fields) - 1,
	}
}

func (d *Document) AddArgumentToField(fieldRef, argRef int) {
	if !d.Fields[fieldRef].HasArguments {
		d.Fields[fieldRef].HasArguments = true
		d.Fields[fieldRef].Arguments.Refs = d.Refs[d.NextRefIndex()][:0]
	}
	d.Fields[fieldRef].Arguments.Refs = append(d.Fields[fieldRef].Arguments.Refs, argRef)
}

func (d *Document) FieldArguments(ref int) []int {
	return d.Fields[ref].Arguments.Refs
}

func (d *Document) FieldArgument(field int, name ByteSlice) (ref int, exists bool) {
	for _, i := range d.Fields[field].Arguments.Refs {
		if bytes.Equal(d.ArgumentNameBytes(i), name) {
			return i, true
		}
	}
	return -1, false
}

func (d *Document) FieldDirectives(ref int) []int {
	return d.Fields[ref].Directives.Refs
}

func (d *Document) FieldsHaveSameShape(left, right int) bool {
	leftAliasDefined := d.FieldAliasIsDefined(left)
	rightAliasDefined := d.FieldAliasIsDefined(right)

	switch {
	case !leftAliasDefined && !rightAliasDefined:
		return d.Input.ByteSliceReferenceContentEquals(d.Fields[left].Name, d.Fields[right].Name)
	case leftAliasDefined && rightAliasDefined:
		return d.Input.ByteSliceReferenceContentEquals(d.Fields[left].Alias.Name, d.Fields[right].Alias.Name)
	case leftAliasDefined && !rightAliasDefined:
		return d.Input.ByteSliceReferenceContentEquals(d.Fields[left].Alias.Name, d.Fields[right].Name)
	case !leftAliasDefined && rightAliasDefined:
		return d.Input.ByteSliceReferenceContentEquals(d.Fields[left].Name, d.Fields[right].Alias.Name)
	default:
		return false
	}
}

func (d *Document) FieldHasArguments(ref int) bool {
	return d.Fields[ref].HasArguments
}

func (d *Document) FieldHasSelections(ref int) bool {
	return d.Fields[ref].HasSelections
}

func (d *Document) FieldHasDirectives(ref int) bool {
	return d.Fields[ref].HasDirectives
}

func (d *Document) FieldsAreEqualFlat(left, right int, checkDirectivesEquality bool) bool {
	equal := bytes.Equal(d.FieldNameBytes(left), d.FieldNameBytes(right)) && // name
		bytes.Equal(d.FieldAliasBytes(left), d.FieldAliasBytes(right)) && // alias
		!d.FieldHasSelections(left) && !d.FieldHasSelections(right) && // selections
		d.ArgumentSetsAreEquals(d.FieldArguments(left), d.FieldArguments(right)) // arguments

	if !equal {
		return false
	}

	if checkDirectivesEquality {
		// directives set equality
		return d.DirectiveSetsAreEqual(d.FieldDirectives(left), d.FieldDirectives(right))
	}

	// directives set has compatible stream directive
	// we don't care about other execution directives
	return d.DirectiveSetsHasCompatibleStreamDirective(d.FieldDirectives(left), d.FieldDirectives(right))
}

func (d *Document) FieldSelectionSet(ref int) (selectionSetRef int, ok bool) {
	if !d.Fields[ref].HasSelections {
		return InvalidRef, false
	}
	return d.Fields[ref].SelectionSet, true
}

// FieldTypeNode - returns the type node of a field. it is applicable for fields on object and interface types
func (d *Document) FieldTypeNode(fieldName []byte, enclosingNode Node) (node Node, ok bool) {
	var (
		fieldDefRef int
		hasField    bool
	)

	switch enclosingNode.Kind {
	case NodeKindObjectTypeDefinition:
		fieldDefRef, hasField = d.ObjectTypeDefinitionFieldWithName(enclosingNode.Ref, fieldName)
		if !hasField {
			return
		}
	case NodeKindInterfaceTypeDefinition:
		fieldDefRef, hasField = d.InterfaceTypeDefinitionFieldWithName(enclosingNode.Ref, fieldName)
		if !hasField {
			return
		}
	default:
		return
	}

	fieldDefTypeName := d.FieldDefinitionTypeNameBytes(fieldDefRef)
	node, hasNode := d.NodeByName(fieldDefTypeName)
	if !hasNode {
		return
	}

	return node, true
}
