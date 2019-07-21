package ast

import (
	"github.com/jensneuse/graphql-go-tools/pkg/input"
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/position"
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
	NodeKindInputObjectTypeExtension
	NodeKindOperationDefinition
	NodeKindSelectionSet
	NodeKindField
	NodeKindFragmentSpread
	NodeKindInlineFragment
	NodeKindFragmentDefinition
)

type Document struct {
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
	Fields                       []Field
	InlineFragments              []InlineFragment
	FragmentSpreads              []FragmentSpread
	OperationDefinitions         []OperationDefinition
	VariableDefinitions          []VariableDefinition
	FragmentDefinitions          []FragmentDefinition
	BooleanValue                 [2]BooleanValue
	Refs                         [][8]int
	RefIndex                     int
}

func NewDocument() *Document {

	return &Document{
		RootNodes:                    make([]Node, 48)[:0],
		RootOperationTypeDefinitions: make([]RootOperationTypeDefinition, 3)[:0],
		SchemaDefinitions:            make([]SchemaDefinition, 2)[:0],
		SchemaExtensions:             make([]SchemaExtension, 2)[:0],
		Directives:                   make([]Directive, 16)[:0],
		Arguments:                    make([]Argument, 48)[:0],
		ObjectTypeDefinitions:        make([]ObjectTypeDefinition, 48)[:0],
		ObjectTypeExtensions:         make([]ObjectTypeExtension, 4)[:0],
		Types:                        make([]Type, 48)[:0],
		FieldDefinitions:             make([]FieldDefinition, 128)[:0],
		InputValueDefinitions:        make([]InputValueDefinition, 128)[:0],
		InputObjectTypeDefinitions:   make([]InputObjectTypeDefinition, 16)[:0],
		InputObjectTypeExtensions:    make([]InputObjectTypeExtension, 4)[:0],
		ScalarTypeDefinitions:        make([]ScalarTypeDefinition, 16)[:0],
		ScalarTypeExtensions:         make([]ScalarTypeExtension, 4)[:0],
		InterfaceTypeDefinitions:     make([]InterfaceTypeDefinition, 16)[:0],
		InterfaceTypeExtensions:      make([]InterfaceTypeExtension, 4)[:0],
		UnionTypeDefinitions:         make([]UnionTypeDefinition, 8)[:0],
		UnionTypeExtensions:          make([]UnionTypeExtension, 4)[:0],
		EnumTypeDefinitions:          make([]EnumTypeDefinition, 8)[:0],
		EnumTypeExtensions:           make([]EnumTypeExtension, 4)[:0],
		EnumValueDefinitions:         make([]EnumValueDefinition, 48)[:0],
		DirectiveDefinitions:         make([]DirectiveDefinition, 8)[:0],
		VariableValues:               make([]VariableValue, 8)[:0],
		StringValues:                 make([]StringValue, 24)[:0],
		EnumValues:                   make([]EnumValue, 24)[:0],
		IntValues:                    make([]IntValue, 128)[:0],
		FloatValues:                  make([]FloatValue, 128)[:0],
		Values:                       make([]Value, 64)[:0],
		ListValues:                   make([]ListValue, 4)[:0],
		ObjectFields:                 make([]ObjectField, 64)[:0],
		ObjectValues:                 make([]ObjectValue, 16)[:0],
		Selections:                   make([]Selection, 128)[:0],
		Fields:                       make([]Field, 128)[:0],
		InlineFragments:              make([]InlineFragment, 16)[:0],
		FragmentSpreads:              make([]FragmentSpread, 16)[:0],
		OperationDefinitions:         make([]OperationDefinition, 8)[:0],
		VariableDefinitions:          make([]VariableDefinition, 8)[:0],
		FragmentDefinitions:          make([]FragmentDefinition, 8)[:0],
		BooleanValue:                 [2]BooleanValue{false, true},
		Refs:                         make([][8]int, 48),
		RefIndex:                     -1,
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
	d.Fields = d.Fields[:0]
	d.InlineFragments = d.InlineFragments[:0]
	d.FragmentSpreads = d.FragmentSpreads[:0]
	d.OperationDefinitions = d.OperationDefinitions[:0]
	d.VariableDefinitions = d.VariableDefinitions[:0]
	d.FragmentDefinitions = d.FragmentDefinitions[:0]

	d.RefIndex = -1
}

func (d *Document) NextRefIndex() int {
	d.RefIndex++
	if d.RefIndex == len(d.Refs) {
		d.Refs = append(d.Refs, [8]int{})
	}
	return d.RefIndex
}

type Node struct {
	Kind NodeKind
	Ref  int
}

type SchemaDefinition struct {
	SchemaLiteral                position.Position
	Directives                   DirectiveList
	RootOperationTypeDefinitions RootOperationTypeDefinitionList
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
	At        position.Position        // @
	Name      input.ByteSliceReference // e.g. include
	Arguments ArgumentList             // e.g. (if: true)
}

type ArgumentList struct {
	LPAREN position.Position
	Refs   []int // Argument
	RPAREN position.Position
}

type FieldDefinition struct {
	Description         Description              // optional e.g. "FieldDefinition is ..."
	Name                input.ByteSliceReference // e.g. foo
	ArgumentsDefinition InputValueDefinitionList // optional
	Colon               position.Position        // :
	Type                int                      // e.g. String
	Directives          DirectiveList            // e.g. @foo
}

type InputValueDefinitionList struct {
	LPAREN position.Position // (
	Refs   []int             // InputValueDefinition
	RPAREN position.Position // )
}

type Argument struct {
	Name  input.ByteSliceReference // e.g. foo
	Colon position.Position        // :
	Value Value                    // e.g. 100 or "Bar"
}

type Value struct {
	Kind ValueKind // e.g. 100 or "Bar"
	Ref  int
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
	Name  input.ByteSliceReference // e.g. lon
	Colon position.Position        // :
	Value Value                    // e.g. 12.43
}

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
	Description  Description              // optional, e.g. "input Foo is..."
	Name         input.ByteSliceReference // e.g. Foo
	Colon        position.Position        // :
	Type         int                      // e.g. String
	DefaultValue DefaultValue             // e.g. = "Bar"
	Directives   DirectiveList            // e.g. @baz
}

type Type struct {
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

type InputObjectTypeExtension struct {
	ExtendLiteral position.Position
	InputObjectTypeDefinition
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
	Description      Description              // optional, describes the interface
	InterfaceLiteral position.Position        // interface
	Name             input.ByteSliceReference // e.g. NamedEntity
	Directives       DirectiveList            // optional, e.g. @foo
	FieldsDefinition FieldDefinitionList      // optional, e.g. { name: String }
}

type InterfaceTypeExtension struct {
	ExtendLiteral position.Position
	InterfaceTypeDefinition
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
	Description          Description              // optional, describes enum
	EnumLiteral          position.Position        // enum
	Name                 input.ByteSliceReference // e.g. Direction
	Directives           DirectiveList            // optional, e.g. @foo
	EnumValuesDefinition EnumValueDefinitionList  // optional, e.g. { NORTH EAST }
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

type OperationDefinition struct {
	OperationType        OperationType            // one of query, mutation, subscription
	OperationTypeLiteral position.Position        // position of the operation type literal, if present
	Name                 input.ByteSliceReference // optional, user defined name of the operation
	VariableDefinitions  VariableDefinitionList   // optional, e.g. ($devicePicSize: Int)
	Directives           DirectiveList            // optional, e.g. @foo
	SelectionSet         SelectionSet             // e.g. {field}
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
	Variable     int               // $ Name
	Colon        position.Position // :
	Type         int               // e.g. String
	DefaultValue DefaultValue      // optional, e.g. = "Default"
	Directives   DirectiveList     // optional, e.g. @foo
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

type Field struct {
	Alias        Alias                    // optional, e.g. renamed:
	Name         input.ByteSliceReference // field name, e.g. id
	Arguments    ArgumentList             // optional
	Directives   DirectiveList            // optional
	SelectionSet SelectionSet             // optional
}

type Alias struct {
	IsDefined bool
	Name      input.ByteSliceReference // optional, e.g. renamedField
	Colon     position.Position        // :
}

// FragmentSpread
// example:
// ...MyFragment
type FragmentSpread struct {
	Spread       position.Position        // ...
	FragmentName input.ByteSliceReference // Name but not on, e.g. MyFragment
	Directives   DirectiveList            // optional, e.g. @foo
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
	Directives    DirectiveList     // optional, e.g. @foo
	SelectionSet  SelectionSet      // optional, e.g. { nextField }
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
	FragmentLiteral position.Position        // fragment
	Name            input.ByteSliceReference // Name but not on, e.g. friendFields
	TypeCondition   TypeCondition            // e.g. on User
	Directives      DirectiveList            // optional, e.g. @foo
	SelectionSet    SelectionSet             // e.g. { id }
}
