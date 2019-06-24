package ast

import (
	"github.com/jensneuse/graphql-go-tools/pkg/input"
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/position"
)

type DefinitionKind int
type OperationType int
type ValueKind int
type TypeKind int

const (
	DefinitionKindUnknown DefinitionKind = iota
	SchemaDefinitionKind

	OperationTypeUndefined OperationType = iota
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
)

type Document struct {
	Definitions                  []Definition
	SchemaDefinitions            []SchemaDefinition
	RootOperationTypeDefinitions []RootOperationTypeDefinition
	Directives                   []Directive
	Arguments                    []Argument
	ObjectTypeDefinitions        []ObjectTypeDefinition
	FieldDefinitions             []FieldDefinition
	Types                        []Type
	InputValueDefinitions        []InputValueDefinition
	InputObjectTypeDefinitions   []InputObjectTypeDefinition
	ScalarTypeDefinitions        []ScalarTypeDefinition
	InterfaceTypeDefinitions     []InterfaceTypeDefinition
	UnionTypeDefinitions         []UnionTypeDefinition
	EnumTypeDefinitions          []EnumTypeDefinition
	EnumValueDefinitions         []EnumValueDefinition
	DirectiveDefinitions         []DirectiveDefinition
	ListValues                   []Value
	VariableValues               []VariableValue
	StringValues                 []StringValue
	IntValues                    []IntValue
	FloatValues                  []FloatValue
	BooleanValue                 [2]BooleanValue
	EnumValues                   []EnumValue
	ValueLists                   []ValueList
}

func NewDocument() *Document {
	return &Document{
		Definitions:                  make([]Definition, 48),
		RootOperationTypeDefinitions: make([]RootOperationTypeDefinition, 3),
		SchemaDefinitions:            make([]SchemaDefinition, 2),
		Directives:                   make([]Directive, 16),
		Arguments:                    make([]Argument, 48),
		ObjectTypeDefinitions:        make([]ObjectTypeDefinition, 48),
		Types:                        make([]Type, 48),
		FieldDefinitions:             make([]FieldDefinition, 128),
		InputValueDefinitions:        make([]InputValueDefinition, 128),
		InputObjectTypeDefinitions:   make([]InputObjectTypeDefinition, 16),
		ScalarTypeDefinitions:        make([]ScalarTypeDefinition, 16),
		InterfaceTypeDefinitions:     make([]InterfaceTypeDefinition, 16),
		UnionTypeDefinitions:         make([]UnionTypeDefinition, 8),
		EnumTypeDefinitions:          make([]EnumTypeDefinition, 8),
		EnumValueDefinitions:         make([]EnumValueDefinition, 48),
		DirectiveDefinitions:         make([]DirectiveDefinition, 8),
		VariableValues:               make([]VariableValue, 8),
		StringValues:                 make([]StringValue, 24),
		EnumValues:                   make([]EnumValue, 24),
		IntValues:                    make([]IntValue, 128),
		FloatValues:                  make([]FloatValue, 128),
		ValueLists:                   make([]ValueList, 16),
		ListValues:                   make([]Value, 64),
		BooleanValue:                 [2]BooleanValue{false, true},
	}
}

func (d *Document) Reset() {
	d.Definitions = d.Definitions[:0]
	d.SchemaDefinitions = d.SchemaDefinitions[:0]
	d.RootOperationTypeDefinitions = d.RootOperationTypeDefinitions[:0]
	d.Directives = d.Directives[:0]
	d.Arguments = d.Arguments[:0]
	d.ObjectTypeDefinitions = d.ObjectTypeDefinitions[:0]
	d.Types = d.Types[:0]
	d.FieldDefinitions = d.FieldDefinitions[:0]
	d.InputValueDefinitions = d.InputValueDefinitions[:0]
	d.InputObjectTypeDefinitions = d.InputObjectTypeDefinitions[:0]
	d.ScalarTypeDefinitions = d.ScalarTypeDefinitions[:0]
	d.InterfaceTypeDefinitions = d.InterfaceTypeDefinitions[:0]
	d.UnionTypeDefinitions = d.UnionTypeDefinitions[:0]
	d.EnumTypeDefinitions = d.EnumTypeDefinitions[:0]
	d.EnumValueDefinitions = d.EnumValueDefinitions[:0]
	d.DirectiveDefinitions = d.DirectiveDefinitions[:0]
	d.VariableValues = d.VariableValues[:0]
	d.StringValues = d.StringValues[:0]
	d.EnumValues = d.EnumValues[:0]
	d.IntValues = d.IntValues[:0]
	d.FloatValues = d.FloatValues[:0]
	d.ValueLists = d.ValueLists[:0]
}

func (d *Document) GetValue(ref int) (node Value, nextRef int) {
	node = d.ListValues[ref]
	nextRef = node.Next()
	return
}

func (d *Document) GetEnumValueDefinition(ref int) (node EnumValueDefinition, nextRef int) {
	node = d.EnumValueDefinitions[ref]
	nextRef = node.Next()
	return
}

func (d *Document) GetInputValueDefinition(ref int) (node InputValueDefinition, nextRef int) {
	node = d.InputValueDefinitions[ref]
	nextRef = node.Next()
	return
}

func (d *Document) GetType(ref int) (node Type, nextRef int) {
	node = d.Types[ref]
	nextRef = node.Next()
	return
}

func (d *Document) GetFieldDefinition(ref int) (node FieldDefinition, nextRef int) {
	node = d.FieldDefinitions[ref]
	nextRef = node.Next()
	return
}

func (d *Document) GetArgument(ref int) (node Argument, nextRef int) {
	node = d.Arguments[ref]
	nextRef = node.Next()
	return
}

func (d *Document) GetDirective(ref int) (node Directive, nextRef int) {
	node = d.Directives[ref]
	nextRef = node.Next()
	return
}

func (d *Document) GetRootOperationTypeDefinition(ref int) (node RootOperationTypeDefinition, nextRef int) {
	node = d.RootOperationTypeDefinitions[ref]
	nextRef = node.Next()
	return
}

func (d *Document) PutRootOperationTypeDefinition(def RootOperationTypeDefinition) int {
	d.RootOperationTypeDefinitions = append(d.RootOperationTypeDefinitions, def)
	return len(d.RootOperationTypeDefinitions) - 1
}

func (d *Document) PutSchemaDefinition(def SchemaDefinition) int {
	d.SchemaDefinitions = append(d.SchemaDefinitions, def)
	return len(d.SchemaDefinitions) - 1
}

func (d *Document) PutDefinition(def Definition) int {
	d.Definitions = append(d.Definitions, def)
	return len(d.Definitions) - 1
}

func (d *Document) PutDirective(directive Directive) int {
	d.Directives = append(d.Directives, directive)
	return len(d.Directives) - 1
}

func (d *Document) PutArgument(argument Argument) int {
	d.Arguments = append(d.Arguments, argument)
	return len(d.Arguments) - 1
}

func (d *Document) PutType(docType Type) int {
	d.Types = append(d.Types, docType)
	return len(d.Types) - 1
}

func (d *Document) PutFieldDefinition(definition FieldDefinition) int {
	d.FieldDefinitions = append(d.FieldDefinitions, definition)
	return len(d.FieldDefinitions) - 1
}

func (d *Document) PutObjectTypeDefinition(definition ObjectTypeDefinition) int {
	d.ObjectTypeDefinitions = append(d.ObjectTypeDefinitions, definition)
	return len(d.ObjectTypeDefinitions) - 1
}

func (d *Document) PutInputValueDefinition(definition InputValueDefinition) int {
	d.InputValueDefinitions = append(d.InputValueDefinitions, definition)
	return len(d.InputValueDefinitions) - 1
}

func (d *Document) PutInputObjectTypeDefinition(definition InputObjectTypeDefinition) int {
	d.InputObjectTypeDefinitions = append(d.InputObjectTypeDefinitions, definition)
	return len(d.InputObjectTypeDefinitions) - 1
}

func (d *Document) PutScalarTypeDefinition(definition ScalarTypeDefinition) int {
	d.ScalarTypeDefinitions = append(d.ScalarTypeDefinitions, definition)
	return len(d.ScalarTypeDefinitions) - 1
}

func (d *Document) PutInterfaceTypeDefinition(definition InterfaceTypeDefinition) int {
	d.InterfaceTypeDefinitions = append(d.InterfaceTypeDefinitions, definition)
	return len(d.InterfaceTypeDefinitions) - 1
}

func (d *Document) PutUnionTypeDefinition(definition UnionTypeDefinition) int {
	d.UnionTypeDefinitions = append(d.UnionTypeDefinitions, definition)
	return len(d.UnionTypeDefinitions) - 1
}

func (d *Document) PutEnumTypeDefinition(definition EnumTypeDefinition) int {
	d.EnumTypeDefinitions = append(d.EnumTypeDefinitions, definition)
	return len(d.EnumTypeDefinitions) - 1
}

func (d *Document) PutEnumValueDefinition(definition EnumValueDefinition) int {
	d.EnumValueDefinitions = append(d.EnumValueDefinitions, definition)
	return len(d.EnumValueDefinitions) - 1
}

func (d *Document) PutDirectiveDefinition(definition DirectiveDefinition) int {
	d.DirectiveDefinitions = append(d.DirectiveDefinitions, definition)
	return len(d.DirectiveDefinitions) - 1
}

func (d *Document) PutStringValue(value StringValue) int {
	d.StringValues = append(d.StringValues, value)
	return len(d.StringValues) - 1
}

func (d *Document) PutEnumValue(value EnumValue) int {
	d.EnumValues = append(d.EnumValues, value)
	return len(d.EnumValues) - 1
}

func (d *Document) PutVariableValue(value VariableValue) int {
	d.VariableValues = append(d.VariableValues, value)
	return len(d.VariableValues) - 1
}

func (d *Document) PutIntValue(value IntValue) int {
	d.IntValues = append(d.IntValues, value)
	return len(d.IntValues) - 1
}

func (d *Document) PutFloatValue(value FloatValue) int {
	d.FloatValues = append(d.FloatValues, value)
	return len(d.FloatValues) - 1
}

func (d *Document) PutValueList(list ValueList) int {
	d.ValueLists = append(d.ValueLists, list)
	return len(d.ValueLists) - 1
}

func (d *Document) PutListValue(value Value) int {
	d.ListValues = append(d.ListValues, value)
	return len(d.ListValues) - 1
}

type Definition struct {
	Kind DefinitionKind
	Ref  int
}

type SchemaDefinition struct {
	SchemaLiteral                position.Position
	Directives                   DirectiveList
	RootOperationTypeDefinitions RootOperationTypeDefinitionList
}

type iterable struct {
	next    int
	hasNext bool
}

func (i *iterable) SetNext(next int) {
	if next == -1 {
		return
	}
	i.hasNext = true
	i.next = next
}

func (i iterable) Next() int {
	if i.hasNext {
		return i.next
	}
	return -1
}

type RootOperationTypeDefinition struct {
	iterable
	OperationType OperationType     // one of query, mutation, subscription
	Colon         position.Position // :
	NamedType     Type              // e.g. Query
}

type Directive struct {
	iterable
	At           position.Position        // @
	Name         input.ByteSliceReference // e.g. include
	ArgumentList ArgumentList             // e.g. (if: true)
}

type FieldDefinition struct {
	iterable
	Description         Description              // optional e.g. "FieldDefinition is ..."
	Name                input.ByteSliceReference // e.g. foo
	ArgumentsDefinition InputValueDefinitionList // optional
	Colon               position.Position        // :
	Type                int                      // e.g. String
	Directives          DirectiveList            // e.g. @foo
}

type Argument struct {
	iterable
	Name  input.ByteSliceReference // e.g. foo
	Colon position.Position        // :
	Value Value                    // e.g. 100 or "Bar"
}

type Value struct {
	iterable
	Kind ValueKind // e.g. 100 or "Bar"
	Ref  int
}

// VariableValue
// example:
// $devicePicSize
type VariableValue struct {
	Dollar position.Position        // $
	Name   input.ByteSliceReference // e.g. devicePicSize
}

// StringValue
// example:
// "foo"
type StringValue struct {
	BlockString bool                     // """foo""" = blockString, "foo" string
	Content     input.ByteSliceReference // e.g. foo
}

// IntValue
// example:
// 123 / -123
type IntValue struct {
	Negative     bool                     // indicates if the value is negative
	NegativeSign position.Position        // optional -
	Raw          input.ByteSliceReference // e.g. 123
}

// FloatValue
// example:
// 13.37 / -13.37
type FloatValue struct {
	Negative     bool                     // indicates if the value is negative
	NegativeSign position.Position        // optional -
	Raw          input.ByteSliceReference // e.g. 13.37
}

// EnumValue
// example:
// Name but not true or false or null
type EnumValue struct {
	Name input.ByteSliceReference // e.g. ORIGIN
}

// BooleanValue
// one of: true, false
type BooleanValue bool

type Description struct {
	IsDefined     bool
	IsBlockString bool                     // true if -> """content""" ; else "content"
	Content       input.ByteSliceReference // literal
	Position      position.Position
}

type ObjectTypeDefinition struct {
	Description          Description              // optional, e.g. "type Foo is ..."
	TypeLiteral          position.Position        // type
	Name                 input.ByteSliceReference // e.g. Foo
	ImplementsInterfaces TypeList                 // e.g implements Bar & Baz
	Directives           DirectiveList            // e.g. @foo
	FieldsDefinition     FieldDefinitionList      // { foo:Bar bar(baz:String) }
}

type InputValueDefinition struct {
	iterable
	Description  Description              // optional, e.g. "input Foo is..."
	Name         input.ByteSliceReference // e.g. Foo
	Colon        position.Position        // :
	Type         int                      // e.g. String
	DefaultValue DefaultValue             // e.g. = "Bar"
	Directives   DirectiveList            // e.g. @baz
}

type Type struct {
	iterable
	TypeKind TypeKind                 // one of Named,List,NonNull
	Name     input.ByteSliceReference // e.g. String (only on NamedType)
	Open     position.Position        // [ (only on ListType)
	Close    position.Position        // ] (only on ListType)
	Bang     position.Position        // ! (only on NonNullType)
	OfType   int
}

type DefaultValue struct {
	IsDefined bool
	Equals    position.Position // =
	Value     Value             // e.g. "Foo"
}

type InputObjectTypeDefinition struct {
	Description           Description              // optional, describes the input type
	InputLiteral          position.Position        // input
	Name                  input.ByteSliceReference // name of the input type
	Directives            DirectiveList            // optional, e.g. @foo
	InputFieldsDefinition InputValueDefinitionList // e.g. x:Float
}

// ScalarTypeDefinition
// example:
// scalar JSON
type ScalarTypeDefinition struct {
	Description   Description              // optional, describes the scalar
	ScalarLiteral position.Position        // scalar
	Name          input.ByteSliceReference // e.g. JSON
	Directives    DirectiveList            // optional, e.g. @foo
}

// InterfaceTypeDefinition
// example:
// interface NamedEntity {
// 	name: String
// }
type InterfaceTypeDefinition struct {
	Description      Description              // optional, describes the interface
	InterfaceLiteral position.Position        // interface
	Name             input.ByteSliceReference // e.g. NamedEntity
	Directives       DirectiveList            // optional, e.g. @foo
	FieldsDefinition FieldDefinitionList      // optional, e.g. { name: String }
}

// UnionTypeDefinition
// example:
// union SearchResult = Photo | Person
type UnionTypeDefinition struct {
	Description      Description              // optional, describes union
	UnionLiteral     position.Position        // union
	Name             input.ByteSliceReference // e.g. SearchResult
	Directives       DirectiveList            // optional, e.g. @foo
	Equals           position.Position        // =
	UnionMemberTypes TypeList                 // optional, e.g. Photo | Person
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
	Description          Description              // optional, describes enum
	EnumLiteral          position.Position        // enum
	Name                 input.ByteSliceReference // e.g. Direction
	Directives           DirectiveList            // optional, e.g. @foo
	EnumValuesDefinition EnumValueDefinitionList  // optional, e.g. { NORTH EAST }
}

// EnumValueDefinition
// example:
// "NORTH enum value" NORTH @foo
type EnumValueDefinition struct {
	iterable
	Description Description              // optional, describes enum value
	EnumValue   input.ByteSliceReference // e.g. NORTH (Name but not true, false or null
	Directives  DirectiveList            // optional, e.g. @foo
}

// DirectiveDefinition
// example:
// directive @example on FIELD
type DirectiveDefinition struct {
	Description         Description              // optional, describes the directive
	DirectiveLiteral    position.Position        // directive
	At                  position.Position        // @
	Name                input.ByteSliceReference // e.g. example
	ArgumentsDefinition InputValueDefinitionList // optional, e.g. (if: Boolean)
	On                  position.Position        // on
	DirectiveLocations  DirectiveLocations       // e.g. FIELD
}
