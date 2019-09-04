//go:generate stringer -type=OperationType,ValueKind,TypeKind,SelectionKind,NodeKind -output ast_string.go
package ast

import (
	"bytes"
	"fmt"
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/literal"
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/position"
	"io"
	"log"
	"strconv"
)

type OperationType int
type ValueKind int
type TypeKind int
type SelectionKind int
type NodeKind int

const (
	OperationTypeUnknown OperationType = iota
	OperationTypeQuery
	OperationTypeMutation
	OperationTypeSubscription

	ValueKindUnknown ValueKind = iota
	ValueKindString
	ValueKindBoolean
	ValueKindInteger
	ValueKindFloat
	ValueKindVariable
	ValueKindNull
	ValueKindList
	ValueKindObject
	ValueKindEnum

	TypeKindUnknown TypeKind = iota
	TypeKindNamed
	TypeKindList
	TypeKindNonNull

	SelectionKindUnknown SelectionKind = iota
	SelectionKindField
	SelectionKindFragmentSpread
	SelectionKindInlineFragment

	NodeKindUnknown NodeKind = iota
	NodeKindSchemaDefinition
	NodeKindSchemaExtension
	NodeKindObjectTypeDefinition
	NodeKindObjectTypeExtension
	NodeKindInterfaceTypeDefinition
	NodeKindInterfaceTypeExtension
	NodeKindUnionTypeDefinition
	NodeKindUnionTypeExtension
	NodeKindEnumTypeDefinition
	NodeKindEnumTypeExtension
	NodeKindInputObjectTypeDefinition
	NodeKindInputValueDefinition
	NodeKindInputObjectTypeExtension
	NodeKindScalarTypeDefinition
	NodeKindDirectiveDefinition
	NodeKindOperationDefinition
	NodeKindSelectionSet
	NodeKindField
	NodeKindFieldDefinition
	NodeKindFragmentSpread
	NodeKindInlineFragment
	NodeKindFragmentDefinition
	NodeKindArgument
	NodeKindDirective
	NodeKindVariableDefinition
)

type Document struct {
	Input                        Input
	RootNodes                    []Node
	SchemaDefinitions            []SchemaDefinition
	SchemaExtensions             []SchemaExtension
	RootOperationTypeDefinitions []RootOperationTypeDefinition
	Directives                   []Directive
	Arguments                    []Argument
	ObjectTypeDefinitions        []ObjectTypeDefinition
	ObjectTypeExtensions         []ObjectTypeExtension
	FieldDefinitions             []FieldDefinition
	Types                        []Type
	InputValueDefinitions        []InputValueDefinition
	InputObjectTypeDefinitions   []InputObjectTypeDefinition
	InputObjectTypeExtensions    []InputObjectTypeExtension
	ScalarTypeDefinitions        []ScalarTypeDefinition
	ScalarTypeExtensions         []ScalarTypeExtension
	InterfaceTypeDefinitions     []InterfaceTypeDefinition
	InterfaceTypeExtensions      []InterfaceTypeExtension
	UnionTypeDefinitions         []UnionTypeDefinition
	UnionTypeExtensions          []UnionTypeExtension
	EnumTypeDefinitions          []EnumTypeDefinition
	EnumTypeExtensions           []EnumTypeExtension
	EnumValueDefinitions         []EnumValueDefinition
	DirectiveDefinitions         []DirectiveDefinition
	Values                       []Value
	ListValues                   []ListValue
	VariableValues               []VariableValue
	StringValues                 []StringValue
	IntValues                    []IntValue
	FloatValues                  []FloatValue
	EnumValues                   []EnumValue
	ObjectFields                 []ObjectField
	ObjectValues                 []ObjectValue
	Selections                   []Selection
	SelectionSets                []SelectionSet
	Fields                       []Field
	InlineFragments              []InlineFragment
	FragmentSpreads              []FragmentSpread
	OperationDefinitions         []OperationDefinition
	VariableDefinitions          []VariableDefinition
	FragmentDefinitions          []FragmentDefinition
	BooleanValues                [2]BooleanValue
	Refs                         [][8]int
	RefIndex                     int
	Index                        Index
}

func (d *Document) IndexOf(slice []int, ref int) (int, bool) {
	for i, j := range slice {
		if ref == j {
			return i, true
		}
	}
	return -1, false
}

func (d *Document) FragmentDefinitionIsUsed(name ByteSlice) bool {
	for _, i := range d.Index.ReplacedFragmentSpreads {
		if bytes.Equal(name, d.FragmentSpreadName(i)) {
			return true
		}
	}
	return false
}

func (d *Document) ReplaceFragmentSpread(selectionSet int, spreadRef int, replaceWithSelectionSet int) {
	for i, j := range d.SelectionSets[selectionSet].SelectionRefs {
		if d.Selections[j].Kind == SelectionKindFragmentSpread && d.Selections[j].Ref == spreadRef {
			d.SelectionSets[selectionSet].SelectionRefs = append(d.SelectionSets[selectionSet].SelectionRefs[:i], append(d.SelectionSets[replaceWithSelectionSet].SelectionRefs, d.SelectionSets[selectionSet].SelectionRefs[i+1:]...)...)
			d.Index.ReplacedFragmentSpreads = append(d.Index.ReplacedFragmentSpreads, spreadRef)
			return
		}
	}
}

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

func NewDocument() *Document {

	return &Document{
		RootNodes:                    make([]Node, 0, 48),
		RootOperationTypeDefinitions: make([]RootOperationTypeDefinition, 0, 3),
		SchemaDefinitions:            make([]SchemaDefinition, 0, 2),
		SchemaExtensions:             make([]SchemaExtension, 0, 2),
		Directives:                   make([]Directive, 0, 16),
		Arguments:                    make([]Argument, 0, 48),
		ObjectTypeDefinitions:        make([]ObjectTypeDefinition, 0, 48),
		ObjectTypeExtensions:         make([]ObjectTypeExtension, 0, 4),
		Types:                        make([]Type, 0, 48),
		FieldDefinitions:             make([]FieldDefinition, 0, 128),
		InputValueDefinitions:        make([]InputValueDefinition, 0, 128),
		InputObjectTypeDefinitions:   make([]InputObjectTypeDefinition, 0, 16),
		InputObjectTypeExtensions:    make([]InputObjectTypeExtension, 0, 4),
		ScalarTypeDefinitions:        make([]ScalarTypeDefinition, 0, 16),
		ScalarTypeExtensions:         make([]ScalarTypeExtension, 0, 4),
		InterfaceTypeDefinitions:     make([]InterfaceTypeDefinition, 0, 16),
		InterfaceTypeExtensions:      make([]InterfaceTypeExtension, 0, 4),
		UnionTypeDefinitions:         make([]UnionTypeDefinition, 0, 8),
		UnionTypeExtensions:          make([]UnionTypeExtension, 0, 4),
		EnumTypeDefinitions:          make([]EnumTypeDefinition, 0, 8),
		EnumTypeExtensions:           make([]EnumTypeExtension, 0, 4),
		EnumValueDefinitions:         make([]EnumValueDefinition, 0, 48),
		DirectiveDefinitions:         make([]DirectiveDefinition, 0, 8),
		VariableValues:               make([]VariableValue, 0, 8),
		StringValues:                 make([]StringValue, 0, 24),
		EnumValues:                   make([]EnumValue, 0, 24),
		IntValues:                    make([]IntValue, 0, 128),
		FloatValues:                  make([]FloatValue, 0, 128),
		Values:                       make([]Value, 0, 64),
		ListValues:                   make([]ListValue, 0, 4),
		ObjectFields:                 make([]ObjectField, 0, 64),
		ObjectValues:                 make([]ObjectValue, 0, 16),
		Selections:                   make([]Selection, 0, 128),
		SelectionSets:                make([]SelectionSet, 0, 48),
		Fields:                       make([]Field, 0, 128),
		InlineFragments:              make([]InlineFragment, 0, 16),
		FragmentSpreads:              make([]FragmentSpread, 0, 16),
		OperationDefinitions:         make([]OperationDefinition, 0, 8),
		VariableDefinitions:          make([]VariableDefinition, 0, 8),
		FragmentDefinitions:          make([]FragmentDefinition, 0, 8),
		BooleanValues:                [2]BooleanValue{false, true},
		Refs:                         make([][8]int, 48),
		RefIndex:                     -1,
		Index: Index{
			Nodes: make(map[string]Node, 48),
		},
	}
}

func (d *Document) Reset() {
	d.RootNodes = d.RootNodes[:0]
	d.SchemaDefinitions = d.SchemaDefinitions[:0]
	d.SchemaExtensions = d.SchemaExtensions[:0]
	d.RootOperationTypeDefinitions = d.RootOperationTypeDefinitions[:0]
	d.Directives = d.Directives[:0]
	d.Arguments = d.Arguments[:0]
	d.ObjectTypeDefinitions = d.ObjectTypeDefinitions[:0]
	d.ObjectTypeExtensions = d.ObjectTypeExtensions[:0]
	d.Types = d.Types[:0]
	d.FieldDefinitions = d.FieldDefinitions[:0]
	d.InputValueDefinitions = d.InputValueDefinitions[:0]
	d.InputObjectTypeDefinitions = d.InputObjectTypeDefinitions[:0]
	d.InputObjectTypeExtensions = d.InputObjectTypeExtensions[:0]
	d.ScalarTypeDefinitions = d.ScalarTypeDefinitions[:0]
	d.ScalarTypeExtensions = d.ScalarTypeExtensions[:0]
	d.InterfaceTypeDefinitions = d.InterfaceTypeDefinitions[:0]
	d.InterfaceTypeExtensions = d.InterfaceTypeExtensions[:0]
	d.UnionTypeDefinitions = d.UnionTypeDefinitions[:0]
	d.UnionTypeExtensions = d.UnionTypeExtensions[:0]
	d.EnumTypeDefinitions = d.EnumTypeDefinitions[:0]
	d.EnumTypeExtensions = d.EnumTypeExtensions[:0]
	d.EnumValueDefinitions = d.EnumValueDefinitions[:0]
	d.DirectiveDefinitions = d.DirectiveDefinitions[:0]
	d.VariableValues = d.VariableValues[:0]
	d.StringValues = d.StringValues[:0]
	d.EnumValues = d.EnumValues[:0]
	d.IntValues = d.IntValues[:0]
	d.FloatValues = d.FloatValues[:0]
	d.Values = d.Values[:0]
	d.ListValues = d.ListValues[:0]
	d.ObjectFields = d.ObjectFields[:0]
	d.ObjectValues = d.ObjectValues[:0]
	d.Selections = d.Selections[:0]
	d.SelectionSets = d.SelectionSets[:0]
	d.Fields = d.Fields[:0]
	d.InlineFragments = d.InlineFragments[:0]
	d.FragmentSpreads = d.FragmentSpreads[:0]
	d.OperationDefinitions = d.OperationDefinitions[:0]
	d.VariableDefinitions = d.VariableDefinitions[:0]
	d.FragmentDefinitions = d.FragmentDefinitions[:0]

	d.RefIndex = -1
	d.Index.Reset()
}

func (d *Document) NextRefIndex() int {
	d.RefIndex++
	if d.RefIndex == len(d.Refs) {
		d.Refs = append(d.Refs, [8]int{})
	}
	return d.RefIndex
}

func (d *Document) FragmentDefinitionRef(byName ByteSlice) (ref int, exists bool) {
	for i := range d.FragmentDefinitions {
		if bytes.Equal(byName, d.Input.ByteSlice(d.FragmentDefinitions[i].Name)) {
			return i, true
		}
	}
	return -1, false
}

func (d *Document) DeleteRootNodes(nodes []Node) {
	for i := range nodes {
		d.DeleteRootNode(nodes[i])
	}
}

func (d *Document) DeleteRootNode(node Node) {
	for i := range d.RootNodes {
		if d.RootNodes[i].Kind == node.Kind && d.RootNodes[i].Ref == node.Ref {
			d.RootNodes = append(d.RootNodes[:i], d.RootNodes[i+1:]...)
			return
		}
	}
}

func (d *Document) NodeTypeName(node Node) ByteSlice {

	var ref ByteSliceReference

	switch node.Kind {
	case NodeKindObjectTypeDefinition:
		ref = d.ObjectTypeDefinitions[node.Ref].Name
	case NodeKindInterfaceTypeDefinition:
		ref = d.InterfaceTypeDefinitions[node.Ref].Name
	case NodeKindInputObjectTypeDefinition:
		ref = d.InputObjectTypeDefinitions[node.Ref].Name
	case NodeKindUnionTypeDefinition:
		ref = d.UnionTypeDefinitions[node.Ref].Name
	case NodeKindScalarTypeDefinition:
		ref = d.ScalarTypeDefinitions[node.Ref].Name
	case NodeKindDirectiveDefinition:
		ref = d.DirectiveDefinitions[node.Ref].Name
	}

	return d.Input.ByteSlice(ref)
}

func (d *Document) FieldDefinitionArgumentsDefinitions(ref int) []int {
	return d.FieldDefinitions[ref].ArgumentsDefinition.Refs
}

func (d *Document) NodeFieldDefinitionArgumentDefinitionByName(node Node, fieldName, argumentName ByteSlice) int {
	fieldDefinition, err := d.NodeFieldDefinitionByName(node, fieldName)
	if err != nil {
		return -1
	}
	argumentDefinitions := d.FieldDefinitionArgumentsDefinitions(fieldDefinition)
	for _, i := range argumentDefinitions {
		if bytes.Equal(argumentName, d.Input.ByteSlice(d.InputValueDefinitions[i].Name)) {
			return i
		}
	}
	return -1
}

func (d *Document) NodeFieldDefinitionArgumentsDefinitions(node Node, fieldName ByteSlice) []int {
	fieldDefinition, err := d.NodeFieldDefinitionByName(node, fieldName)
	if err != nil {
		return nil
	}
	return d.FieldDefinitionArgumentsDefinitions(fieldDefinition)
}

func (d *Document) FieldDefinitionType(ref int) int {
	return d.FieldDefinitions[ref].Type
}

func (d *Document) FieldDefinitionTypeNodeKind(ref int) NodeKind {
	typeName := d.ResolveTypeName(d.FieldDefinitions[ref].Type)
	return d.Index.Nodes[string(typeName)].Kind
}

func (d *Document) NodeFieldDefinitions(node Node) []int {
	switch node.Kind {
	case NodeKindObjectTypeDefinition:
		return d.ObjectTypeDefinitions[node.Ref].FieldsDefinition.Refs
	case NodeKindInterfaceTypeDefinition:
		return d.InterfaceTypeDefinitions[node.Ref].FieldsDefinition.Refs
	default:
		return nil
	}
}

func (d *Document) NodeFieldDefinitionByName(node Node, fieldName ByteSlice) (int, error) {
	for _, i := range d.NodeFieldDefinitions(node) {
		if bytes.Equal(d.Input.ByteSlice(d.FieldDefinitions[i].Name), fieldName) {
			return i, nil
		}
	}
	return -1, fmt.Errorf("node field definition not found for node: %+v name: %s", node, string(fieldName))
}

func (d *Document) NodeTypeNameString(node Node) string {
	return string(d.NodeTypeName(node))
}

func (d *Document) FieldName(ref int) ByteSlice {
	return d.Input.ByteSlice(d.Fields[ref].Name)
}

func (d *Document) FieldAlias(ref int) ByteSlice {
	return d.Input.ByteSlice(d.Fields[ref].Alias.Name)
}

func (d *Document) FieldAliasIsDefined(ref int) bool {
	return d.Fields[ref].Alias.IsDefined
}

func (d *Document) FieldNameString(ref int) string {
	return string(d.FieldName(ref))
}

func (d *Document) FragmentSpreadName(ref int) ByteSlice {
	return d.Input.ByteSlice(d.FragmentSpreads[ref].FragmentName)
}

func (d *Document) FragmentSpreadNameString(ref int) string {
	return string(d.FragmentSpreadName(ref))
}

func (d *Document) InlineFragmentTypeConditionName(ref int) ByteSlice {
	if d.InlineFragments[ref].TypeCondition.Type == -1 {
		return nil
	}
	return d.Input.ByteSlice(d.Types[d.InlineFragments[ref].TypeCondition.Type].Name)
}

func (d *Document) InlineFragmentTypeConditionNameString(ref int) string {
	return string(d.InlineFragmentTypeConditionName(ref))
}

func (d *Document) FragmentDefinitionName(ref int) ByteSlice {
	return d.Input.ByteSlice(d.FragmentDefinitions[ref].Name)
}

func (d *Document) FragmentDefinitionTypeName(ref int) ByteSlice {
	return d.ResolveTypeName(d.FragmentDefinitions[ref].TypeCondition.Type)
}

func (d *Document) FragmentDefinitionNameString(ref int) string {
	return string(d.FragmentDefinitionName(ref))
}

func (d *Document) ResolveTypeName(ref int) ByteSlice {
	graphqlType := d.Types[ref]
	for graphqlType.TypeKind != TypeKindNamed {
		graphqlType = d.Types[graphqlType.OfType]
	}
	return d.Input.ByteSlice(graphqlType.Name)
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

func (d *Document) NodeImplementsInterface(node Node, interfaceNode Node) bool {

	nodeFields := d.NodeFieldDefinitions(node)
	interfaceFields := d.NodeFieldDefinitions(interfaceNode)

	for _, i := range interfaceFields {
		interfaceFieldName := d.FieldDefinitionNameBytes(i)
		if !d.FieldDefinitionsContainField(nodeFields, interfaceFieldName) {
			return false
		}
	}

	return true
}

func (d *Document) FieldDefinitionsContainField(definitions []int, field ByteSlice) bool {
	for _, i := range definitions {
		if bytes.Equal(field, d.FieldDefinitionNameBytes(i)) {
			return true
		}
	}
	return false
}

func (d *Document) NodeByName(name ByteSlice) (Node, bool) {
	node, exists := d.Index.Nodes[string(name)]
	return node, exists
}

func (d *Document) FieldHasSelections(ref int) bool {
	return d.Fields[ref].HasSelections
}

func (d *Document) FieldHasDirectives(ref int) bool {
	return d.Fields[ref].HasDirectives
}

func (d *Document) BooleanValue(ref int) BooleanValue {
	return d.BooleanValues[ref]
}

func (d *Document) BooleanValuesAreEqual(left, right int) bool {
	return d.BooleanValue(left) == d.BooleanValue(right)
}

func (d *Document) StringValue(ref int) StringValue {
	return d.StringValues[ref]
}

func (d *Document) StringValueContent(ref int) ByteSlice {
	return d.Input.ByteSlice(d.StringValues[ref].Content)
}

func (d *Document) StringValueIsBlockString(ref int) bool {
	return d.StringValues[ref].BlockString
}

func (d *Document) StringValuesAreEquals(left, right int) bool {
	return d.StringValueIsBlockString(left) == d.StringValueIsBlockString(right) &&
		bytes.Equal(d.StringValueContent(left), d.StringValueContent(right))
}

func (d *Document) IntValue(ref int) IntValue {
	return d.IntValues[ref]
}

func (d *Document) IntValueIsNegative(ref int) bool {
	return d.IntValues[ref].Negative
}

func (d *Document) IntValueRaw(ref int) ByteSlice {
	return d.Input.ByteSlice(d.IntValues[ref].Raw)
}

func (d *Document) IntValuesAreEquals(left, right int) bool {
	return d.IntValueIsNegative(left) == d.IntValueIsNegative(right) &&
		bytes.Equal(d.IntValueRaw(left), d.IntValueRaw(right))
}

func (d *Document) FloatValueIsNegative(ref int) bool {
	return d.FloatValues[ref].Negative
}

func (d *Document) FloatValueRaw(ref int) ByteSlice {
	return d.Input.ByteSlice(d.FloatValues[ref].Raw)
}

func (d *Document) FloatValuesAreEqual(left, right int) bool {
	return d.FloatValueIsNegative(left) == d.FloatValueIsNegative(right) &&
		bytes.Equal(d.FloatValueRaw(left), d.FloatValueRaw(right))
}

func (d *Document) VariableValueName(ref int) ByteSlice {
	return d.Input.ByteSlice(d.VariableValues[ref].Name)
}

func (d *Document) VariableValuesAreEqual(left, right int) bool {
	return bytes.Equal(d.VariableValueName(left), d.VariableValueName(right))
}

func (d *Document) Value(ref int) Value {
	return d.Values[ref]
}

func (d *Document) ListValuesAreEqual(left, right int) bool {
	leftValues, rightValues := d.ListValues[left].Refs, d.ListValues[right].Refs
	if len(leftValues) != len(rightValues) {
		return false
	}
	for i := 0; i < len(leftValues); i++ {
		left, right = leftValues[i], rightValues[i]
		leftValue, rightValue := d.Value(left), d.Value(right)
		if !d.ValuesAreEqual(leftValue, rightValue) {
			return false
		}
	}
	return true
}

func (d *Document) ObjectField(ref int) ObjectField {
	return d.ObjectFields[ref]
}

func (d *Document) ObjectFieldName(ref int) ByteSlice {
	return d.Input.ByteSlice(d.ObjectFields[ref].Name)
}

func (d *Document) ObjectFieldValue(ref int) Value {
	return d.ObjectFields[ref].Value
}

func (d *Document) ObjectFieldsAreEqual(left, right int) bool {
	return bytes.Equal(d.ObjectFieldName(left), d.ObjectFieldName(right)) &&
		d.ValuesAreEqual(d.ObjectFieldValue(left), d.ObjectFieldValue(right))
}

func (d *Document) ObjectValuesAreEqual(left, right int) bool {
	leftFields, rightFields := d.ObjectValues[left].Refs, d.ObjectValues[right].Refs
	if len(leftFields) != len(rightFields) {
		return false
	}
	for i := 0; i < len(leftFields); i++ {
		left, right = leftFields[i], rightFields[i]
		if !d.ObjectFieldsAreEqual(left, right) {
			return false
		}
	}
	return true
}

func (d *Document) EnumValueName(ref int) ByteSliceReference {
	return d.EnumValues[ref].Name
}

func (d *Document) EnumValueNameBytes(ref int) ByteSlice {
	return d.Input.ByteSlice(d.EnumValues[ref].Name)
}

func (d *Document) EnumValuesAreEqual(left, right int) bool {
	return d.Input.ByteSliceReferenceContentEquals(d.EnumValueName(left), d.EnumValueName(right))
}

func (d *Document) ValuesAreEqual(left, right Value) bool {
	if left.Kind != right.Kind {
		return false
	}
	switch left.Kind {
	case ValueKindString:
		return d.StringValuesAreEquals(left.Ref, right.Ref)
	case ValueKindBoolean:
		return d.BooleanValuesAreEqual(left.Ref, right.Ref)
	case ValueKindInteger:
		return d.IntValuesAreEquals(left.Ref, right.Ref)
	case ValueKindFloat:
		return d.FloatValuesAreEqual(left.Ref, right.Ref)
	case ValueKindVariable:
		return d.VariableValuesAreEqual(left.Ref, right.Ref)
	case ValueKindNull:
		return true
	case ValueKindList:
		return d.ListValuesAreEqual(left.Ref, right.Ref)
	case ValueKindObject:
		return d.ObjectValuesAreEqual(left.Ref, right.Ref)
	case ValueKindEnum:
		return d.EnumValuesAreEqual(left.Ref, right.Ref)
	default:
		return false
	}
}

func (d *Document) ArgumentName(ref int) ByteSlice {
	return d.Input.ByteSlice(d.Arguments[ref].Name)
}

func (d *Document) ArgumentNameString(ref int) string {
	return string(d.ArgumentName(ref))
}

func (d *Document) ArgumentValue(ref int) Value {
	return d.Arguments[ref].Value
}

func (d *Document) ArgumentsAreEqual(left, right int) bool {
	return bytes.Equal(d.ArgumentName(left), d.ArgumentName(right)) &&
		d.ValuesAreEqual(d.ArgumentValue(left), d.ArgumentValue(right))
}

func (d *Document) ArgumentSetsAreEquals(left, right []int) bool {
	if len(left) != len(right) {
		return false
	}
	for i := 0; i < len(left); i++ {
		leftArgument, rightArgument := left[i], right[i]
		if !d.ArgumentsAreEqual(leftArgument, rightArgument) {
			return false
		}
	}
	return true
}

func (d *Document) FieldArguments(ref int) []int {
	return d.Fields[ref].Arguments.Refs
}

func (d *Document) FieldArgument(field int, name ByteSlice) (ref int, exists bool) {
	for _, i := range d.Fields[field].Arguments.Refs {
		if bytes.Equal(d.ArgumentName(i), name) {
			return i, true
		}
	}
	return -1, false
}

func (d *Document) FieldDirectives(ref int) []int {
	return d.Fields[ref].Directives.Refs
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

func (d *Document) DirectiveArgumentSet(ref int) []int {
	return d.Directives[ref].Arguments.Refs
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

func (d *Document) FieldsAreEqualFlat(left, right int) bool {
	return bytes.Equal(d.FieldName(left), d.FieldName(right)) && // name
		bytes.Equal(d.FieldAlias(left), d.FieldAlias(right)) && // alias
		!d.FieldHasSelections(left) && !d.FieldHasSelections(right) && // selections
		d.ArgumentSetsAreEquals(d.FieldArguments(left), d.FieldArguments(right)) && // arguments
		d.DirectiveSetsAreEqual(d.FieldDirectives(left), d.FieldDirectives(right)) // directives
}

func (d *Document) InlineFragmentHasTypeCondition(ref int) bool {
	return d.InlineFragments[ref].TypeCondition.Type != -1
}

func (d *Document) InlineFragmentHasDirectives(ref int) bool {
	return len(d.InlineFragments[ref].Directives.Refs) != 0
}

func (d *Document) TypeDefinitionContainsImplementsInterface(typeName, interfaceName ByteSlice) bool {
	typeDefinition, exists := d.Index.Nodes[string(typeName)]
	if !exists {
		return false
	}
	if typeDefinition.Kind != NodeKindObjectTypeDefinition {
		return false
	}
	for _, i := range d.ObjectTypeDefinitions[typeDefinition.Ref].ImplementsInterfaces.Refs {
		implements := d.ResolveTypeName(i)
		if bytes.Equal(interfaceName, implements) {
			return true
		}
	}
	return false
}

func (d *Document) RemoveFieldAlias(ref int) {
	d.Fields[ref].Alias.IsDefined = false
	d.Fields[ref].Alias.Name.Start = 0
	d.Fields[ref].Alias.Name.End = 0
}

func (d *Document) InlineFragmentSelections(ref int) []int {
	if !d.InlineFragments[ref].HasSelections {
		return nil
	}
	return d.SelectionSets[d.InlineFragments[ref].SelectionSet].SelectionRefs
}

func (d *Document) TypesAreEqualDeep(left int, right int) bool {
	for {
		if left == -1 || right == -1 {
			return false
		}
		if d.Types[left].TypeKind != d.Types[right].TypeKind {
			return false
		}
		if d.Types[left].TypeKind == TypeKindNamed {
			return d.Input.ByteSliceReferenceContentEquals(d.Types[left].Name, d.Types[right].Name)
		}
		left = d.Types[left].OfType
		right = d.Types[right].OfType
	}
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

func (d *Document) NodeFragmentIsAllowedOnNode(fragmentNode, onNode Node) bool {
	switch onNode.Kind {
	case NodeKindObjectTypeDefinition:
		return d.NodeFragmentIsAllowedOnObjectTypeDefinition(fragmentNode, onNode)
	case NodeKindInterfaceTypeDefinition:
		return d.NodeFragmentIsAllowedOnInterfaceTypeDefinition(fragmentNode, onNode)
	case NodeKindUnionTypeDefinition:
		return d.NodeFragmentIsAllowedOnUnionTypeDefinition(fragmentNode, onNode)
	default:
		return false
	}
}

func (d *Document) NodeFragmentIsAllowedOnInterfaceTypeDefinition(fragmentNode, interfaceTypeNode Node) bool {

	switch fragmentNode.Kind {
	case NodeKindObjectTypeDefinition:
		return d.NodeImplementsInterface(fragmentNode, interfaceTypeNode)
	case NodeKindInterfaceTypeDefinition:
		return bytes.Equal(d.InterfaceTypeDefinitionName(fragmentNode.Ref), d.InterfaceTypeDefinitionName(interfaceTypeNode.Ref))
	case NodeKindUnionTypeDefinition:
		return d.UnionNodeIntersectsInterfaceNode(fragmentNode, interfaceTypeNode)
	}

	return false
}

func (d *Document) NodeFragmentIsAllowedOnUnionTypeDefinition(fragmentNode, unionTypeNode Node) bool {

	switch fragmentNode.Kind {
	case NodeKindObjectTypeDefinition:
		return d.NodeIsUnionMember(fragmentNode, unionTypeNode)
	case NodeKindInterfaceTypeDefinition:
		return false
	case NodeKindUnionTypeDefinition:
		return bytes.Equal(d.UnionTypeDefinitionName(fragmentNode.Ref), d.UnionTypeDefinitionName(unionTypeNode.Ref))
	}

	return false
}

func (d *Document) NodeFragmentIsAllowedOnObjectTypeDefinition(fragmentNode, objectTypeNode Node) bool {

	switch fragmentNode.Kind {
	case NodeKindObjectTypeDefinition:
		return bytes.Equal(d.ObjectTypeDefinitionName(fragmentNode.Ref), d.ObjectTypeDefinitionName(objectTypeNode.Ref))
	case NodeKindInterfaceTypeDefinition:
		return d.NodeImplementsInterface(objectTypeNode, fragmentNode)
	case NodeKindUnionTypeDefinition:
		return d.NodeIsUnionMember(objectTypeNode, fragmentNode)
	}

	return false
}

func (d *Document) UnionNodeIntersectsInterfaceNode(unionNode, interfaceNode Node) bool {
	for _, i := range d.UnionTypeDefinitions[unionNode.Ref].UnionMemberTypes.Refs {
		memberName := d.ResolveTypeName(i)
		node := d.Index.Nodes[string(memberName)]
		if node.Kind != NodeKindObjectTypeDefinition {
			continue
		}
		if d.NodeImplementsInterface(node, interfaceNode) {
			return true
		}
	}
	return false
}

func (d *Document) NodeIsUnionMember(node Node, union Node) bool {
	nodeTypeName := d.NodeTypeName(node)
	for _, i := range d.UnionTypeDefinitions[union.Ref].UnionMemberTypes.Refs {
		memberName := d.ResolveTypeName(i)
		if bytes.Equal(nodeTypeName, memberName) {
			return true
		}
	}
	return false
}

type Node struct {
	Kind NodeKind
	Ref  int
}

func (d *Document) RemoveNodeFromNode(remove, from Node) {
	switch from.Kind {
	case NodeKindSelectionSet:
		d.RemoveNodeFromSelectionSet(from.Ref, remove)
	default:
		log.Printf("RemoveNodeFromNode not implemented for from: %s", from.Kind)
	}
}

func (d *Document) RemoveNodeFromSelectionSet(set int, node Node) {

	var selectionKind SelectionKind

	switch node.Kind {
	case NodeKindFragmentSpread:
		selectionKind = SelectionKindFragmentSpread
	case NodeKindInlineFragment:
		selectionKind = SelectionKindInlineFragment
	case NodeKindField:
		selectionKind = SelectionKindField
	default:
		log.Printf("RemoveNodeFromSelectionSet not implemented for node: %s", node.Kind)
		return
	}

	for i, j := range d.SelectionSets[set].SelectionRefs {
		if d.Selections[j].Kind == selectionKind && d.Selections[j].Ref == node.Ref {
			d.SelectionSets[set].SelectionRefs = append(d.SelectionSets[set].SelectionRefs[:i], d.SelectionSets[set].SelectionRefs[i+1:]...)
			return
		}
	}
}

func (n Node) Name(definition *Document) string {
	return string(definition.NodeTypeName(n))
}

type SchemaDefinition struct {
	SchemaLiteral                position.Position
	Directives                   DirectiveList
	RootOperationTypeDefinitions RootOperationTypeDefinitionList
}

func (d *Document) NodeDirectives(node Node) []int {
	switch node.Kind {
	case NodeKindField:
		return d.Fields[node.Ref].Directives.Refs
	case NodeKindInlineFragment:
		return d.InlineFragments[node.Ref].Directives.Refs
	case NodeKindFragmentSpread:
		return d.FragmentSpreads[node.Ref].Directives.Refs
	}
	return nil
}

type DirectiveList struct {
	Refs []int
}

type RootOperationTypeDefinitionList struct {
	LBrace position.Position // {
	Refs   []int             // RootOperationTypeDefinition
	RBrace position.Position // }
}

type SchemaExtension struct {
	ExtendLiteral position.Position
	SchemaDefinition
}

type RootOperationTypeDefinition struct {
	OperationType OperationType     // one of query, mutation, subscription
	Colon         position.Position // :
	NamedType     Type              // e.g. Query
}

type Directive struct {
	At           position.Position  // @
	Name         ByteSliceReference // e.g. include
	HasArguments bool
	Arguments    ArgumentList // e.g. (if: true)
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

type ArgumentList struct {
	LPAREN position.Position
	Refs   []int // Argument
	RPAREN position.Position
}

type FieldDefinition struct {
	Description         Description              // optional e.g. "FieldDefinition is ..."
	Name                ByteSliceReference       // e.g. foo
	ArgumentsDefinition InputValueDefinitionList // optional
	Colon               position.Position        // :
	Type                int                      // e.g. String
	Directives          DirectiveList            // e.g. @foo
}

func (d *Document) FieldDefinitionNameBytes(ref int) ByteSlice {
	return d.Input.ByteSlice(d.FieldDefinitions[ref].Name)
}

func (d *Document) FieldDefinitionNameString(ref int) string {
	return string(d.FieldDefinitionNameBytes(ref))
}

func (d *Document) FieldDefinitionDirectiveByName(fieldDefinition int, directiveName ByteSlice) (ref int, exists bool) {
	for _, i := range d.FieldDefinitions[fieldDefinition].Directives.Refs {
		if bytes.Equal(directiveName, d.DirectiveNameBytes(i)) {
			return i, true
		}
	}
	return
}

type InputValueDefinitionList struct {
	LPAREN position.Position // (
	Refs   []int             // InputValueDefinition
	RPAREN position.Position // )
}

type Argument struct {
	Name  ByteSliceReference // e.g. foo
	Colon position.Position  // :
	Value Value              // e.g. 100 or "Bar"
}

func (d *Document) ArgumentsBefore(ancestor Node, argument int) []int {
	switch ancestor.Kind {
	case NodeKindField:
		for i, j := range d.Fields[ancestor.Ref].Arguments.Refs {
			if argument == j {
				return d.Fields[ancestor.Ref].Arguments.Refs[:i]
			}
		}
	case NodeKindDirective:
		for i, j := range d.Directives[ancestor.Ref].Arguments.Refs {
			if argument == j {
				return d.Directives[ancestor.Ref].Arguments.Refs[:i]
			}
		}
	}
	return nil
}

func (d *Document) ArgumentsAfter(ancestor Node, argument int) []int {
	switch ancestor.Kind {
	case NodeKindField:
		for i, j := range d.Fields[ancestor.Ref].Arguments.Refs {
			if argument == j {
				return d.Fields[ancestor.Ref].Arguments.Refs[i+1:]
			}
		}
	case NodeKindDirective:
		for i, j := range d.Directives[ancestor.Ref].Arguments.Refs {
			if argument == j {
				return d.Directives[ancestor.Ref].Arguments.Refs[i+1:]
			}
		}
	}
	return nil
}

func (d *Document) PrintArgument(ref int, w io.Writer) error {
	_, err := w.Write(d.Input.ByteSlice(d.Arguments[ref].Name))
	if err != nil {
		return err
	}
	_, err = w.Write(literal.COLON)
	if err != nil {
		return err
	}
	_, err = w.Write(literal.SPACE)
	if err != nil {
		return err
	}
	return d.PrintValue(d.Arguments[ref].Value, w)
}

func (d *Document) PrintArguments(refs []int, w io.Writer) (err error) {
	_, err = w.Write(literal.LPAREN)
	if err != nil {
		return
	}
	for i, j := range refs {
		err = d.PrintArgument(j, w)
		if err != nil {
			return
		}
		if i != len(refs)-1 {
			_, err = w.Write(literal.COMMA)
			if err != nil {
				return
			}
			_, err = w.Write(literal.SPACE)
			if err != nil {
				return
			}
		}
	}
	_, err = w.Write(literal.RPAREN)
	return
}

type Value struct {
	Kind ValueKind // e.g. 100 or "Bar"
	Ref  int
}

func (d *Document) PrintValue(value Value, w io.Writer) (err error) {
	switch value.Kind {
	case ValueKindBoolean:
		if d.BooleanValues[value.Ref] {
			_, err = w.Write(literal.TRUE)
		} else {
			_, err = w.Write(literal.FALSE)
		}
	case ValueKindString:
		_, err = w.Write(literal.QUOTE)
		_, err = w.Write(d.Input.ByteSlice(d.StringValues[value.Ref].Content))
		_, err = w.Write(literal.QUOTE)
	case ValueKindInteger:
		if d.IntValues[value.Ref].Negative {
			_, err = w.Write(literal.SUB)
		}
		_, err = w.Write(d.Input.ByteSlice(d.IntValues[value.Ref].Raw))
	case ValueKindFloat:
		if d.FloatValues[value.Ref].Negative {
			_, err = w.Write(literal.SUB)
		}
		_, err = w.Write(d.Input.ByteSlice(d.FloatValues[value.Ref].Raw))
	case ValueKindVariable:
		_, err = w.Write(literal.DOLLAR)
		_, err = w.Write(d.Input.ByteSlice(d.VariableValues[value.Ref].Name))
	case ValueKindNull:
		_, err = w.Write(literal.NULL)
	case ValueKindList:
		_, err = w.Write(literal.LBRACK)
		for i, j := range d.ListValues[value.Ref].Refs {
			err = d.PrintValue(d.Value(j), w)
			if err != nil {
				return
			}
			if i != len(d.ListValues[value.Ref].Refs)-1 {
				_, err = w.Write(literal.COMMA)
			}
		}
		_, err = w.Write(literal.RBRACK)
	case ValueKindObject:
		_, err = w.Write(literal.LBRACE)
		for i, j := range d.ObjectValues[value.Ref].Refs {
			_, err = w.Write(d.ObjectFieldName(j))
			if err != nil {
				return
			}
			_, err = w.Write(literal.COLON)
			if err != nil {
				return
			}
			_, err = w.Write(literal.SPACE)
			if err != nil {
				return
			}
			err = d.PrintValue(d.ObjectFieldValue(j), w)
			if err != nil {
				return
			}
			if i != len(d.ObjectValues[value.Ref].Refs)-1 {
				_, err = w.Write(literal.COMMA)
				if err != nil {
					return
				}
			}
		}
		_, err = w.Write(literal.RBRACE)
	case ValueKindEnum:
		_, err = w.Write(d.Input.ByteSlice(d.EnumValues[value.Ref].Name))
	}
	return
}

type ListValue struct {
	LBRACK position.Position // [
	Refs   []int             // Value
	RBRACK position.Position // ]
}

// VariableValue
// example:
// $devicePicSize
type VariableValue struct {
	Dollar position.Position  // $
	Name   ByteSliceReference // e.g. devicePicSize
}

// StringValue
// example:
// "foo"
type StringValue struct {
	BlockString bool               // """foo""" = blockString, "foo" string
	Content     ByteSliceReference // e.g. foo
}

// IntValue
// example:
// 123 / -123
type IntValue struct {
	Negative     bool               // indicates if the value is negative
	NegativeSign position.Position  // optional -
	Raw          ByteSliceReference // e.g. 123
}

func (d *Document) IntValueAsInt(ref int) (out int) {
	in := d.Input.ByteSliceString(d.IntValues[ref].Raw)
	raw, _ := strconv.ParseInt(in, 10, 64)
	if d.IntValues[ref].Negative {
		out = -int(raw)
	}
	out = int(raw)
	return
}

// FloatValue
// example:
// 13.37 / -13.37
type FloatValue struct {
	Negative     bool               // indicates if the value is negative
	NegativeSign position.Position  // optional -
	Raw          ByteSliceReference // e.g. 13.37
}

// EnumValue
// example:
// Name but not true or false or null
type EnumValue struct {
	Name ByteSliceReference // e.g. ORIGIN
}

// BooleanValues
// one of: true, false
type BooleanValue bool

// ObjectValue
// example:
// { lon: 12.43, lat: -53.211 }
type ObjectValue struct {
	LBRACE position.Position
	Refs   []int // ObjectField
	RBRACE position.Position
}

// ObjectField
// example:
// lon: 12.43
type ObjectField struct {
	Name  ByteSliceReference // e.g. lon
	Colon position.Position  // :
	Value Value              // e.g. 12.43
}

type Description struct {
	IsDefined     bool
	IsBlockString bool               // true if -> """content""" ; else "content"
	Content       ByteSliceReference // literal
	Position      position.Position
}

type ObjectTypeDefinition struct {
	Description          Description         // optional, e.g. "type Foo is ..."
	TypeLiteral          position.Position   // type
	Name                 ByteSliceReference  // e.g. Foo
	ImplementsInterfaces TypeList            // e.g implements Bar & Baz
	Directives           DirectiveList       // e.g. @foo
	FieldsDefinition     FieldDefinitionList // { foo:Bar bar(baz:String) }
}

func (d *Document) ObjectTypeDefinitionName(ref int) ByteSlice {
	return d.Input.ByteSlice(d.ObjectTypeDefinitions[ref].Name)
}

type TypeList struct {
	Refs []int // Type
}

type FieldDefinitionList struct {
	LBRACE position.Position // {
	Refs   []int             // FieldDefinition
	RBRACE position.Position // }
}

type ObjectTypeExtension struct {
	ExtendLiteral position.Position
	ObjectTypeDefinition
}

type InputValueDefinition struct {
	Description   Description        // optional, e.g. "input Foo is..."
	Name          ByteSliceReference // e.g. Foo
	Colon         position.Position  // :
	Type          int                // e.g. String
	DefaultValue  DefaultValue       // e.g. = "Bar"
	HasDirectives bool
	Directives    DirectiveList // e.g. @baz
}

func (d *Document) InputValueDefinitionName(ref int) ByteSlice {
	return d.Input.ByteSlice(d.InputValueDefinitions[ref].Name)
}

func (d *Document) InputValueDefinitionType(ref int) int {
	return d.InputValueDefinitions[ref].Type
}

func (d *Document) InputValueDefinitionArgumentIsOptional(ref int) bool {
	nonNull := d.Types[d.InputValueDefinitions[ref].Type].TypeKind == TypeKindNonNull
	hasDefault := d.InputValueDefinitions[ref].DefaultValue.IsDefined
	return !nonNull || hasDefault
}

func (d *Document) InputValueDefinitionHasDirective(ref int, directiveName ByteSlice) bool {
	if !d.InputValueDefinitions[ref].HasDirectives {
		return false
	}
	for _, i := range d.InputValueDefinitions[ref].Directives.Refs {
		if bytes.Equal(directiveName, d.DirectiveNameBytes(i)) {
			return true
		}
	}
	return false
}

type Type struct {
	TypeKind TypeKind           // one of Named,List,NonNull
	Name     ByteSliceReference // e.g. String (only on NamedType)
	Open     position.Position  // [ (only on ListType)
	Close    position.Position  // ] (only on ListType)
	Bang     position.Position  // ! (only on NonNullType)
	OfType   int
}

func (d *Document) PrintType(ref int, w io.Writer) error {
	switch d.Types[ref].TypeKind {
	case TypeKindNonNull:
		err := d.PrintType(d.Types[ref].OfType, w)
		if err != nil {
			return err
		}
		_, err = w.Write(literal.BANG)
		return err
	case TypeKindNamed:
		_, err := w.Write(d.Input.ByteSlice(d.Types[ref].Name))
		return err
	case TypeKindList:
		_, err := w.Write(literal.LBRACK)
		if err != nil {
			return err
		}
		err = d.PrintType(d.Types[ref].OfType, w)
		if err != nil {
			return err
		}
		_, err = w.Write(literal.RBRACK)
		return err
	}
	return nil
}

type DefaultValue struct {
	IsDefined bool
	Equals    position.Position // =
	Value     Value             // e.g. "Foo"
}

type InputObjectTypeDefinition struct {
	Description           Description              // optional, describes the input type
	InputLiteral          position.Position        // input
	Name                  ByteSliceReference       // name of the input type
	Directives            DirectiveList            // optional, e.g. @foo
	InputFieldsDefinition InputValueDefinitionList // e.g. x:Float
}

type InputObjectTypeExtension struct {
	ExtendLiteral position.Position
	InputObjectTypeDefinition
}

// ScalarTypeDefinition
// example:
// scalar JSON
type ScalarTypeDefinition struct {
	Description   Description        // optional, describes the scalar
	ScalarLiteral position.Position  // scalar
	Name          ByteSliceReference // e.g. JSON
	Directives    DirectiveList      // optional, e.g. @foo
}

func (d *Document) ScalarTypeDefinitionName(ref int) ByteSlice {
	return d.Input.ByteSlice(d.ScalarTypeDefinitions[ref].Name)
}

type ScalarTypeExtension struct {
	ExtendLiteral position.Position
	ScalarTypeDefinition
}

// InterfaceTypeDefinition
// example:
// interface NamedEntity {
// 	name: String
// }
type InterfaceTypeDefinition struct {
	Description      Description         // optional, describes the interface
	InterfaceLiteral position.Position   // interface
	Name             ByteSliceReference  // e.g. NamedEntity
	Directives       DirectiveList       // optional, e.g. @foo
	FieldsDefinition FieldDefinitionList // optional, e.g. { name: String }
}

func (d *Document) InterfaceTypeDefinitionName(ref int) ByteSlice {
	return d.Input.ByteSlice(d.InterfaceTypeDefinitions[ref].Name)
}

type InterfaceTypeExtension struct {
	ExtendLiteral position.Position
	InterfaceTypeDefinition
}

// UnionTypeDefinition
// example:
// union SearchResult = Photo | Person
type UnionTypeDefinition struct {
	Description      Description        // optional, describes union
	UnionLiteral     position.Position  // union
	Name             ByteSliceReference // e.g. SearchResult
	Directives       DirectiveList      // optional, e.g. @foo
	Equals           position.Position  // =
	UnionMemberTypes TypeList           // optional, e.g. Photo | Person
}

func (d *Document) UnionTypeDefinitionName(ref int) ByteSlice {
	return d.Input.ByteSlice(d.UnionTypeDefinitions[ref].Name)
}

type UnionTypeExtension struct {
	ExtendLiteral position.Position
	UnionTypeDefinition
}

// EnumTypeDefinition
// example:
// enum Direction {
//  NORTH
//  EAST
//  SOUTH
//  WEST
//}
type EnumTypeDefinition struct {
	Description          Description             // optional, describes enum
	EnumLiteral          position.Position       // enum
	Name                 ByteSliceReference      // e.g. Direction
	Directives           DirectiveList           // optional, e.g. @foo
	EnumValuesDefinition EnumValueDefinitionList // optional, e.g. { NORTH EAST }
}

func (d *Document) EnumTypeDefinitionContainsEnumValue(enumTypeDef int, valueName ByteSlice) bool {
	for _, i := range d.EnumTypeDefinitions[enumTypeDef].EnumValuesDefinition.Refs {
		if bytes.Equal(valueName, d.EnumValueDefinitionEnumValue(i)) {
			return true
		}
	}
	return false
}

type EnumValueDefinitionList struct {
	LBRACE position.Position // {
	Refs   []int             //
	RBRACE position.Position // }
}

type EnumTypeExtension struct {
	ExtendLiteral position.Position
	EnumTypeDefinition
}

// EnumValueDefinition
// example:
// "NORTH enum value" NORTH @foo
type EnumValueDefinition struct {
	Description Description        // optional, describes enum value
	EnumValue   ByteSliceReference // e.g. NORTH (Name but not true, false or null
	Directives  DirectiveList      // optional, e.g. @foo
}

func (d *Document) EnumValueDefinitionEnumValue(ref int) ByteSlice {
	return d.Input.ByteSlice(d.EnumValueDefinitions[ref].EnumValue)
}

// DirectiveDefinition
// example:
// directive @example on FIELD
type DirectiveDefinition struct {
	Description         Description              // optional, describes the directive
	DirectiveLiteral    position.Position        // directive
	At                  position.Position        // @
	Name                ByteSliceReference       // e.g. example
	ArgumentsDefinition InputValueDefinitionList // optional, e.g. (if: Boolean)
	On                  position.Position        // on
	DirectiveLocations  DirectiveLocations       // e.g. FIELD
}

func (d *Document) RemoveDirectiveFromNode(node Node, ref int) {
	switch node.Kind {
	case NodeKindFragmentSpread:
		if i, ok := d.IndexOf(d.FragmentSpreads[node.Ref].Directives.Refs, ref); ok {
			d.FragmentSpreads[node.Ref].Directives.Refs = append(d.FragmentSpreads[node.Ref].Directives.Refs[:i], d.FragmentSpreads[node.Ref].Directives.Refs[i+1:]...)
			d.FragmentSpreads[node.Ref].HasDirectives = len(d.FragmentSpreads[node.Ref].Directives.Refs) > 0
		}
	case NodeKindInlineFragment:
		if i, ok := d.IndexOf(d.InlineFragments[node.Ref].Directives.Refs, ref); ok {
			d.InlineFragments[node.Ref].Directives.Refs = append(d.InlineFragments[node.Ref].Directives.Refs[:i], d.InlineFragments[node.Ref].Directives.Refs[i+1:]...)
			d.InlineFragments[node.Ref].HasDirectives = len(d.InlineFragments[node.Ref].Directives.Refs) > 0
		}
	case NodeKindField:
		if i, ok := d.IndexOf(d.Fields[node.Ref].Directives.Refs, ref); ok {
			d.Fields[node.Ref].Directives.Refs = append(d.Fields[node.Ref].Directives.Refs[:i], d.Fields[node.Ref].Directives.Refs[i+1:]...)
			d.Fields[node.Ref].HasDirectives = len(d.Fields[node.Ref].Directives.Refs) > 0
		}
	default:
		log.Printf("RemoveDirectiveFromNode not implemented for node kind: %s", node.Kind)
	}
}

func (d *Document) NodeDirectiveLocation(node Node) (location DirectiveLocation, err error) {
	switch node.Kind {
	case NodeKindSchemaDefinition:
		location = TypeSystemDirectiveLocationSchema
	case NodeKindSchemaExtension:
		location = TypeSystemDirectiveLocationSchema
	case NodeKindObjectTypeDefinition:
		location = TypeSystemDirectiveLocationObject
	case NodeKindObjectTypeExtension:
		location = TypeSystemDirectiveLocationObject
	case NodeKindInterfaceTypeDefinition:
		location = TypeSystemDirectiveLocationInterface
	case NodeKindInterfaceTypeExtension:
		location = TypeSystemDirectiveLocationInterface
	case NodeKindUnionTypeDefinition:
		location = TypeSystemDirectiveLocationUnion
	case NodeKindUnionTypeExtension:
		location = TypeSystemDirectiveLocationUnion
	case NodeKindEnumTypeDefinition:
		location = TypeSystemDirectiveLocationEnum
	case NodeKindEnumTypeExtension:
		location = TypeSystemDirectiveLocationEnum
	case NodeKindInputObjectTypeDefinition:
		location = TypeSystemDirectiveLocationInputObject
	case NodeKindInputObjectTypeExtension:
		location = TypeSystemDirectiveLocationInputObject
	case NodeKindScalarTypeDefinition:
		location = TypeSystemDirectiveLocationScalar
	case NodeKindOperationDefinition:
		switch d.OperationDefinitions[node.Ref].OperationType {
		case OperationTypeQuery:
			location = ExecutableDirectiveLocationQuery
		case OperationTypeMutation:
			location = ExecutableDirectiveLocationMutation
		case OperationTypeSubscription:
			location = ExecutableDirectiveLocationSubscription
		}
	case NodeKindField:
		location = ExecutableDirectiveLocationField
	case NodeKindFragmentSpread:
		location = ExecutableDirectiveLocationFragmentSpread
	case NodeKindInlineFragment:
		location = ExecutableDirectiveLocationInlineFragment
	case NodeKindFragmentDefinition:
		location = ExecutableDirectiveLocationFragmentDefinition
	case NodeKindVariableDefinition:
		location = ExecutableDirectiveLocationVariableDefinition
	default:
		err = fmt.Errorf("node kind: %s is not allowed to have directives", node.Kind)
	}
	return
}

type OperationDefinition struct {
	OperationType          OperationType      // one of query, mutation, subscription
	OperationTypeLiteral   position.Position  // position of the operation type literal, if present
	Name                   ByteSliceReference // optional, user defined name of the operation
	HasVariableDefinitions bool
	VariableDefinitions    VariableDefinitionList // optional, e.g. ($devicePicSize: Int)
	HasDirectives          bool
	Directives             DirectiveList // optional, e.g. @foo
	SelectionSet           int           // e.g. {field}
	HasSelections          bool
}

func (d *Document) OperationDefinitionIsLastRootNode(ref int) bool {
	for i := range d.RootNodes {
		if d.RootNodes[i].Kind == NodeKindOperationDefinition && d.RootNodes[i].Ref == ref {
			return len(d.RootNodes)-1 == i
		}
	}
	return false
}

func (d *Document) FragmentDefinitionIsLastRootNode(ref int) bool {
	for i := range d.RootNodes {
		if d.RootNodes[i].Kind == NodeKindFragmentDefinition && d.RootNodes[i].Ref == ref {
			return len(d.RootNodes)-1 == i
		}
	}
	return false
}

type VariableDefinitionList struct {
	LPAREN position.Position // (
	Refs   []int             // VariableDefinition
	RPAREN position.Position // )
}

// VariableDefinition
// example:
// $devicePicSize: Int = 100 @small
type VariableDefinition struct {
	VariableValue Value             // $ Name
	Colon         position.Position // :
	Type          int               // e.g. String
	DefaultValue  DefaultValue      // optional, e.g. = "Default"
	HasDirectives bool
	Directives    DirectiveList // optional, e.g. @foo
}

func (d *Document) VariableDefinitionsBefore(variableDefinition int) bool {
	return variableDefinition != 0
}

func (d *Document) VariableDefinitionsAfter(variableDefinition int) bool {
	return len(d.VariableDefinitions) != 1 && variableDefinition != len(d.VariableDefinitions)-1
}

func (d *Document) VariableDefinitionName(ref int) ByteSlice {
	return d.VariableValueName(d.VariableDefinitions[ref].VariableValue.Ref)
}

func (d *Document) VariableDefinitionByName(name ByteSlice) (definition int, exists bool) {
	for i := range d.VariableDefinitions {
		definitionName := d.VariableValueName(d.VariableDefinitions[i].VariableValue.Ref)
		if bytes.Equal(name, definitionName) {
			return i, true
		}
	}
	return -1, false
}

func (d *Document) DirectiveArgumentInputValueDefinition(directiveName ByteSlice, argumentName ByteSlice) int {
	for i := range d.DirectiveDefinitions {
		if bytes.Equal(directiveName, d.Input.ByteSlice(d.DirectiveDefinitions[i].Name)) {
			for _, j := range d.DirectiveDefinitions[i].ArgumentsDefinition.Refs {
				if bytes.Equal(argumentName, d.Input.ByteSlice(d.InputValueDefinitions[j].Name)) {
					return j
				}
			}
		}
	}
	return -1
}

type SelectionSet struct {
	LBrace        position.Position
	RBrace        position.Position
	SelectionRefs []int
}

type Selection struct {
	Kind SelectionKind // one of Field, FragmentSpread, InlineFragment
	Ref  int           // reference to the actual selection
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

func (d *Document) SelectionsAfterField(field int, selectionSet Node) bool {
	if selectionSet.Kind != NodeKindSelectionSet {
		return false
	}

	if len(d.SelectionSets[selectionSet.Ref].SelectionRefs) == 1 {
		return false
	}

	for i, j := range d.SelectionSets[selectionSet.Ref].SelectionRefs {
		if d.Selections[j].Kind == SelectionKindField && d.Selections[j].Ref == field {
			return i != len(d.SelectionSets[selectionSet.Ref].SelectionRefs)-1
		}
	}

	return false
}

func (d *Document) SelectionsAfterInlineFragment(inlineFragment int, selectionSet Node) bool {
	if selectionSet.Kind != NodeKindSelectionSet {
		return false
	}

	if len(d.SelectionSets[selectionSet.Ref].SelectionRefs) == 1 {
		return false
	}

	for i, j := range d.SelectionSets[selectionSet.Ref].SelectionRefs {
		if d.Selections[j].Kind == SelectionKindInlineFragment && d.Selections[j].Ref == inlineFragment {
			return i != len(d.SelectionSets[selectionSet.Ref].SelectionRefs)-1
		}
	}

	return false
}

type Field struct {
	Alias         Alias              // optional, e.g. renamed:
	Name          ByteSliceReference // field name, e.g. id
	HasArguments  bool
	Arguments     ArgumentList // optional
	HasDirectives bool
	Directives    DirectiveList // optional
	SelectionSet  int           // optional
	HasSelections bool
}

type Alias struct {
	IsDefined bool
	Name      ByteSliceReference // optional, e.g. renamedField
	Colon     position.Position  // :
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

// TypeCondition
// example:
// on User
type TypeCondition struct {
	On   position.Position // on
	Type int               // NamedType
}

// FragmentDefinition
// example:
// fragment friendFields on User {
//  id
//  name
//  profilePic(size: 50)
//}
type FragmentDefinition struct {
	FragmentLiteral position.Position  // fragment
	Name            ByteSliceReference // Name but not on, e.g. friendFields
	TypeCondition   TypeCondition      // e.g. on User
	Directives      DirectiveList      // optional, e.g. @foo
	SelectionSet    int                // e.g. { id }
	HasSelections   bool
}
