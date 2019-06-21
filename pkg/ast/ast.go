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
}

func NewDocument() *Document {
	return &Document{
		Definitions:                  make([]Definition, 48),
		RootOperationTypeDefinitions: make([]RootOperationTypeDefinition, 3),
		SchemaDefinitions:            make([]SchemaDefinition, 2),
		Directives:                   make([]Directive, 16),
		Arguments:                    make([]Argument, 48),
		ObjectTypeDefinitions:        make([]ObjectTypeDefinition, 24),
		Types:                        make([]Type, 48),
		FieldDefinitions:             make([]FieldDefinition, 128),
		InputValueDefinitions:        make([]InputValueDefinition, 128),
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
	Kind ValueKind                // e.g. 100 or "Bar"
	Raw  input.ByteSliceReference // raw byte reference to content
}

type Description struct {
	IsDefined     bool
	IsBlockString bool                     // true if -> """content""" ; else "content"
	Body          input.ByteSliceReference // literal
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
