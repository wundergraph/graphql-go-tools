package parser

import (
	"fmt"
	"github.com/jensneuse/graphql-go-tools/pkg/document"
	"github.com/jensneuse/graphql-go-tools/pkg/lexer"
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/keyword"
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/position"
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/token"
)

type errInvalidType struct {
	enclosingFunctionName string
	expected              string
	actual                string
	position              position.Position
}

func newErrInvalidType(position position.Position, enclosingFunctionName, expected, actual string) errInvalidType {
	return errInvalidType{
		enclosingFunctionName: enclosingFunctionName,
		expected:              expected,
		actual:                actual,
		position:              position,
	}
}

func (e errInvalidType) Error() string {
	return fmt.Sprintf("parser:%s:invalidType - expected '%s', got '%s' @ %s", e.enclosingFunctionName, e.expected, e.actual, e.position)
}

type indexPool [][]int

func (i *indexPool) grow() {

	grow := 10
	if (len(*i) / 2) > grow {
		grow = len(*i) / 2
	}

	for k := 0; k < grow; k++ {
		*i = append(*i, make([]int, 0, 8))
	}
}

// Parser holds the lexer and a buffer for writing literals
type Parser struct {
	l                 Lexer
	ParsedDefinitions ParsedDefinitions
	indexPool         indexPool
	indexPoolPosition int
}

// ParsedDefinitions contains all parsed definitions to avoid deeply nested data structures while parsing
type ParsedDefinitions struct {
	OperationDefinitions       document.OperationDefinitions
	FragmentDefinitions        document.FragmentDefinitions
	VariableDefinitions        document.VariableDefinitions
	Fields                     document.Fields
	InlineFragments            document.InlineFragments
	FragmentSpreads            document.FragmentSpreads
	Arguments                  document.Arguments
	Directives                 document.Directives
	EnumTypeDefinitions        document.EnumTypeDefinitions
	EnumValuesDefinitions      document.EnumValueDefinitions
	FieldDefinitions           document.FieldDefinitions
	InputValueDefinitions      document.InputValueDefinitions
	InputObjectTypeDefinitions document.InputObjectTypeDefinitions
	DirectiveDefinitions       document.DirectiveDefinitions
	InterfaceTypeDefinitions   document.InterfaceTypeDefinitions
	ObjectTypeDefinitions      document.ObjectTypeDefinitions
	ScalarTypeDefinitions      document.ScalarTypeDefinitions
	UnionTypeDefinitions       document.UnionTypeDefinitions
	Values                     []document.Value
	ListValues                 []document.ListValue
	ObjectValues               []document.ObjectValue
	ObjectFields               document.ObjectFields
	Types                      document.Types

	ByteSliceReferences []document.ByteSliceReference
	Integers            []int32
	Floats              []float32
	Booleans            [2]bool
}

// Lexer is the interface used by the Parser to lex tokens
type Lexer interface {
	SetInput(input []byte) error
	Read() (tok token.Token)
	Peek(ignoreWhitespace bool) keyword.Keyword
	ByteSlice(reference document.ByteSliceReference) document.ByteSlice
}

// NewParser returns a new parser using a buffered runestringer
func NewParser() *Parser {

	poolSize := 512
	pool := make([][]int, poolSize)
	for i := 0; i < poolSize; i++ {
		pool[i] = make([]int, 0, 8)
	}

	definitions := ParsedDefinitions{
		OperationDefinitions:       make(document.OperationDefinitions, 0, 8),
		FragmentDefinitions:        make(document.FragmentDefinitions, 0, 8),
		VariableDefinitions:        make(document.VariableDefinitions, 0, 8),
		Fields:                     make(document.Fields, 0, 48),
		InlineFragments:            make(document.InlineFragments, 0, 8),
		FragmentSpreads:            make(document.FragmentSpreads, 0, 8),
		Arguments:                  make(document.Arguments, 0, 8),
		Directives:                 make(document.Directives, 0, 8),
		EnumTypeDefinitions:        make(document.EnumTypeDefinitions, 0, 8),
		EnumValuesDefinitions:      make(document.EnumValueDefinitions, 0, 8),
		FieldDefinitions:           make(document.FieldDefinitions, 0, 8),
		InputValueDefinitions:      make(document.InputValueDefinitions, 0, 8),
		InputObjectTypeDefinitions: make(document.InputObjectTypeDefinitions, 0, 8),
		DirectiveDefinitions:       make(document.DirectiveDefinitions, 0, 8),
		InterfaceTypeDefinitions:   make(document.InterfaceTypeDefinitions, 0, 8),
		ObjectTypeDefinitions:      make(document.ObjectTypeDefinitions, 0, 8),
		ScalarTypeDefinitions:      make(document.ScalarTypeDefinitions, 0, 8),
		UnionTypeDefinitions:       make(document.UnionTypeDefinitions, 0, 8),
		Values:                     make([]document.Value, 0, 8),
		ListValues:                 make([]document.ListValue, 0, 8),
		ObjectValues:               make([]document.ObjectValue, 0, 8),
		ObjectFields:               make(document.ObjectFields, 0, 8),
		Types:                      make(document.Types, 0, 8),

		Integers:            make([]int32, 0, 8),
		Floats:              make([]float32, 0, 8),
		ByteSliceReferences: make([]document.ByteSliceReference, 0, 8),
	}

	definitions.Booleans[0] = false
	definitions.Booleans[1] = true

	return &Parser{
		l:                 lexer.NewLexer(),
		indexPool:         pool,
		ParsedDefinitions: definitions,
	}
}

func (p *Parser) ByteSlice(reference document.ByteSliceReference) document.ByteSlice {
	return p.l.ByteSlice(reference)
}

// ParseTypeSystemDefinition parses a TypeSystemDefinition from an io.Reader
func (p *Parser) ParseTypeSystemDefinition(input []byte) (definition document.TypeSystemDefinition, err error) {
	p.resetObjects()
	err = p.l.SetInput(input)
	if err != nil {
		return
	}
	return p.parseTypeSystemDefinition()
}

// ParseExecutableDefinition parses an ExecutableDefinition from an io.Reader
func (p *Parser) ParseExecutableDefinition(input []byte) (definition document.ExecutableDefinition, err error) {
	p.resetObjects()
	err = p.l.SetInput(input)
	if err != nil {
		return
	}
	return p.parseExecutableDefinition()
}

func (p *Parser) readExpect(expected keyword.Keyword, enclosingFunctionName string) (t token.Token, err error) {
	t = p.l.Read()
	if t.Keyword != expected {
		return t, newErrInvalidType(t.TextPosition, enclosingFunctionName, expected.String(), t.Keyword.String()+" lit: "+string(p.ByteSlice(t.Literal)))
	}

	return
}

func (p *Parser) peekExpect(expected keyword.Keyword, swallow bool) (matched bool, err error) {
	next := p.l.Peek(true)
	if err != nil {
		return false, err
	}

	matched = next == expected

	if matched && swallow {
		p.l.Read()
	}

	return
}

func (p *Parser) indexPoolGet() []int {
	p.indexPoolPosition++

	if len(p.indexPool)-1 <= p.indexPoolPosition {
		p.indexPool.grow()
	}

	return p.indexPool[p.indexPoolPosition][:0]
}

func (p *Parser) initSelectionSet(set *document.SelectionSet) {
	set.InlineFragments = p.indexPoolGet()
	set.FragmentSpreads = p.indexPoolGet()
	set.Fields = p.indexPoolGet()
}

func (p *Parser) initField(field *document.Field) {
	field.Directives = p.indexPoolGet()
	field.Arguments = p.indexPoolGet()
	p.initSelectionSet(&field.SelectionSet)
}

func (p *Parser) makeFieldDefinition() document.FieldDefinition {
	return document.FieldDefinition{
		Directives:          p.indexPoolGet(),
		ArgumentsDefinition: p.indexPoolGet(),
	}
}

func (p *Parser) makeEnumTypeDefinition() document.EnumTypeDefinition {
	return document.EnumTypeDefinition{
		Directives:           p.indexPoolGet(),
		EnumValuesDefinition: p.indexPoolGet(),
	}
}

func (p *Parser) makeInputValueDefinition() document.InputValueDefinition {
	return document.InputValueDefinition{
		Directives: p.indexPoolGet(),
	}
}

func (p *Parser) makeInputObjectTypeDefinition() document.InputObjectTypeDefinition {
	return document.InputObjectTypeDefinition{
		Directives:            p.indexPoolGet(),
		InputFieldsDefinition: p.indexPoolGet(),
	}
}

func (p *Parser) makeTypeSystemDefinition() document.TypeSystemDefinition {
	return document.TypeSystemDefinition{
		InputObjectTypeDefinitions: p.indexPoolGet(),
		EnumTypeDefinitions:        p.indexPoolGet(),
		DirectiveDefinitions:       p.indexPoolGet(),
		InterfaceTypeDefinitions:   p.indexPoolGet(),
		ObjectTypeDefinitions:      p.indexPoolGet(),
		ScalarTypeDefinitions:      p.indexPoolGet(),
		UnionTypeDefinitions:       p.indexPoolGet(),
	}
}

func (p *Parser) makeDirectiveDefinition() document.DirectiveDefinition {
	return document.DirectiveDefinition{
		ArgumentsDefinition: p.indexPoolGet(),
	}
}

func (p *Parser) makeInterfaceTypeDefinition() document.InterfaceTypeDefinition {
	return document.InterfaceTypeDefinition{
		Directives:       p.indexPoolGet(),
		FieldsDefinition: p.indexPoolGet(),
	}
}

func (p *Parser) makeObjectTypeDefinition() document.ObjectTypeDefinition {
	return document.ObjectTypeDefinition{
		Directives:       p.indexPoolGet(),
		FieldsDefinition: p.indexPoolGet(),
	}
}

func (p *Parser) makeScalarTypeDefinition() document.ScalarTypeDefinition {
	return document.ScalarTypeDefinition{
		Directives: p.indexPoolGet(),
	}
}

func (p *Parser) makeUnionTypeDefinition() document.UnionTypeDefinition {
	return document.UnionTypeDefinition{
		Directives: p.indexPoolGet(),
	}
}

func (p *Parser) makeEnumValueDefinition() document.EnumValueDefinition {
	return document.EnumValueDefinition{
		Directives: p.indexPoolGet(),
	}
}

func (p *Parser) initFragmentDefinition(definition *document.FragmentDefinition) {
	definition.Directives = p.indexPoolGet()
	p.initSelectionSet(&definition.SelectionSet)
}

func (p *Parser) initOperationDefinition(definition *document.OperationDefinition) {
	p.initSelectionSet(&definition.SelectionSet)
	definition.Directives = p.indexPoolGet()
	definition.VariableDefinitions = p.indexPoolGet()
}

func (p *Parser) initInlineFragment(fragment *document.InlineFragment) {
	fragment.Directives = p.indexPoolGet()
	p.initSelectionSet(&fragment.SelectionSet)
}

func (p *Parser) makeFragmentSpread() document.FragmentSpread {
	return document.FragmentSpread{
		Directives: p.indexPoolGet(),
	}
}

func (p *Parser) makeExecutableDefinition() document.ExecutableDefinition {
	return document.ExecutableDefinition{
		FragmentDefinitions:  p.indexPoolGet(),
		OperationDefinitions: p.indexPoolGet(),
	}
}

func (p *Parser) makeListValue(index *int) document.ListValue {
	value := p.indexPoolGet()
	p.ParsedDefinitions.ListValues = append(p.ParsedDefinitions.ListValues, value)
	*index = len(p.ParsedDefinitions.ListValues) - 1
	return value
}

func (p *Parser) makeObjectValue(index *int) document.ObjectValue {
	value := p.indexPoolGet()
	p.ParsedDefinitions.ObjectValues = append(p.ParsedDefinitions.ObjectValues, value)
	*index = len(p.ParsedDefinitions.ObjectValues) - 1
	return value
}

func (p *Parser) makeValue(index *int) document.Value {
	value := document.Value{}
	p.ParsedDefinitions.Values = append(p.ParsedDefinitions.Values, value)
	*index = len(p.ParsedDefinitions.Values) - 1
	return value
}

func (p *Parser) makeType(index *int) document.Type {
	documentType := document.Type{
		OfType: -1,
	}
	p.ParsedDefinitions.Types = append(p.ParsedDefinitions.Types, documentType)
	*index = len(p.ParsedDefinitions.Types) - 1
	return documentType
}

func (p *Parser) resetObjects() {

	p.indexPoolPosition = -1

	p.ParsedDefinitions.OperationDefinitions = p.ParsedDefinitions.OperationDefinitions[:0]
	p.ParsedDefinitions.FragmentDefinitions = p.ParsedDefinitions.FragmentDefinitions[:0]
	p.ParsedDefinitions.VariableDefinitions = p.ParsedDefinitions.VariableDefinitions[:0]
	p.ParsedDefinitions.Fields = p.ParsedDefinitions.Fields[:0]
	p.ParsedDefinitions.InlineFragments = p.ParsedDefinitions.InlineFragments[:0]
	p.ParsedDefinitions.FragmentSpreads = p.ParsedDefinitions.FragmentSpreads[:0]
	p.ParsedDefinitions.Arguments = p.ParsedDefinitions.Arguments[:0]
	p.ParsedDefinitions.Directives = p.ParsedDefinitions.Directives[:0]
	p.ParsedDefinitions.EnumTypeDefinitions = p.ParsedDefinitions.EnumTypeDefinitions[:0]
	p.ParsedDefinitions.EnumValuesDefinitions = p.ParsedDefinitions.EnumValuesDefinitions[:0]
	p.ParsedDefinitions.FieldDefinitions = p.ParsedDefinitions.FieldDefinitions[:0]
	p.ParsedDefinitions.InputValueDefinitions = p.ParsedDefinitions.InputValueDefinitions[:0]
	p.ParsedDefinitions.InputObjectTypeDefinitions = p.ParsedDefinitions.InputObjectTypeDefinitions[:0]
	p.ParsedDefinitions.DirectiveDefinitions = p.ParsedDefinitions.DirectiveDefinitions[:0]
	p.ParsedDefinitions.InterfaceTypeDefinitions = p.ParsedDefinitions.InterfaceTypeDefinitions[:0]
	p.ParsedDefinitions.ObjectTypeDefinitions = p.ParsedDefinitions.ObjectTypeDefinitions[:0]
	p.ParsedDefinitions.ScalarTypeDefinitions = p.ParsedDefinitions.ScalarTypeDefinitions[:0]
	p.ParsedDefinitions.UnionTypeDefinitions = p.ParsedDefinitions.UnionTypeDefinitions[:0]
	p.ParsedDefinitions.ByteSliceReferences = p.ParsedDefinitions.ByteSliceReferences[:0]
	p.ParsedDefinitions.Values = p.ParsedDefinitions.Values[:0]
	p.ParsedDefinitions.Integers = p.ParsedDefinitions.Integers[:0]
	p.ParsedDefinitions.Floats = p.ParsedDefinitions.Floats[:0]
	p.ParsedDefinitions.ListValues = p.ParsedDefinitions.ListValues[:0]
	p.ParsedDefinitions.ObjectValues = p.ParsedDefinitions.ObjectValues[:0]
	p.ParsedDefinitions.ObjectFields = p.ParsedDefinitions.ObjectFields[:0]
	p.ParsedDefinitions.Types = p.ParsedDefinitions.Types[:0]
}

func (p *Parser) putOperationDefinition(definition document.OperationDefinition) int {
	p.ParsedDefinitions.OperationDefinitions = append(p.ParsedDefinitions.OperationDefinitions, definition)
	return len(p.ParsedDefinitions.OperationDefinitions) - 1
}

func (p *Parser) putFragmentDefinition(definition document.FragmentDefinition) int {
	p.ParsedDefinitions.FragmentDefinitions = append(p.ParsedDefinitions.FragmentDefinitions, definition)
	return len(p.ParsedDefinitions.FragmentDefinitions) - 1
}

func (p *Parser) putVariableDefinition(definition document.VariableDefinition) int {
	p.ParsedDefinitions.VariableDefinitions = append(p.ParsedDefinitions.VariableDefinitions, definition)
	return len(p.ParsedDefinitions.VariableDefinitions) - 1
}

func (p *Parser) putField(field document.Field) int {
	p.ParsedDefinitions.Fields = append(p.ParsedDefinitions.Fields, field)
	return len(p.ParsedDefinitions.Fields) - 1
}

func (p *Parser) putInlineFragment(fragment document.InlineFragment) int {
	p.ParsedDefinitions.InlineFragments = append(p.ParsedDefinitions.InlineFragments, fragment)
	return len(p.ParsedDefinitions.InlineFragments) - 1
}

func (p *Parser) putFragmentSpread(spread document.FragmentSpread) int {
	p.ParsedDefinitions.FragmentSpreads = append(p.ParsedDefinitions.FragmentSpreads, spread)
	return len(p.ParsedDefinitions.FragmentSpreads) - 1
}

func (p *Parser) putArgument(argument document.Argument) int {
	p.ParsedDefinitions.Arguments = append(p.ParsedDefinitions.Arguments, argument)
	return len(p.ParsedDefinitions.Arguments) - 1
}

func (p *Parser) putDirective(directive document.Directive) int {
	p.ParsedDefinitions.Directives = append(p.ParsedDefinitions.Directives, directive)
	return len(p.ParsedDefinitions.Directives) - 1
}

func (p *Parser) putEnumTypeDefinition(definition document.EnumTypeDefinition) int {
	p.ParsedDefinitions.EnumTypeDefinitions = append(p.ParsedDefinitions.EnumTypeDefinitions, definition)
	return len(p.ParsedDefinitions.EnumTypeDefinitions) - 1
}

func (p *Parser) putEnumValueDefinition(definition document.EnumValueDefinition) int {
	p.ParsedDefinitions.EnumValuesDefinitions = append(p.ParsedDefinitions.EnumValuesDefinitions, definition)
	return len(p.ParsedDefinitions.EnumValuesDefinitions) - 1
}

func (p *Parser) putFieldDefinition(definition document.FieldDefinition) int {
	p.ParsedDefinitions.FieldDefinitions = append(p.ParsedDefinitions.FieldDefinitions, definition)
	return len(p.ParsedDefinitions.FieldDefinitions) - 1
}

func (p *Parser) putInputValueDefinition(definition document.InputValueDefinition) int {
	p.ParsedDefinitions.InputValueDefinitions = append(p.ParsedDefinitions.InputValueDefinitions, definition)
	return len(p.ParsedDefinitions.InputValueDefinitions) - 1
}

func (p *Parser) putInputObjectTypeDefinition(definition document.InputObjectTypeDefinition) int {
	p.ParsedDefinitions.InputObjectTypeDefinitions = append(p.ParsedDefinitions.InputObjectTypeDefinitions, definition)
	return len(p.ParsedDefinitions.InputObjectTypeDefinitions) - 1
}

func (p *Parser) putDirectiveDefinition(definition document.DirectiveDefinition) int {
	p.ParsedDefinitions.DirectiveDefinitions = append(p.ParsedDefinitions.DirectiveDefinitions, definition)
	return len(p.ParsedDefinitions.DirectiveDefinitions) - 1
}

func (p *Parser) putInterfaceTypeDefinition(definition document.InterfaceTypeDefinition) int {
	p.ParsedDefinitions.InterfaceTypeDefinitions = append(p.ParsedDefinitions.InterfaceTypeDefinitions, definition)
	return len(p.ParsedDefinitions.InterfaceTypeDefinitions) - 1
}

func (p *Parser) putObjectTypeDefinition(definition document.ObjectTypeDefinition) int {
	p.ParsedDefinitions.ObjectTypeDefinitions = append(p.ParsedDefinitions.ObjectTypeDefinitions, definition)
	return len(p.ParsedDefinitions.ObjectTypeDefinitions) - 1
}

func (p *Parser) putScalarTypeDefinition(definition document.ScalarTypeDefinition) int {
	p.ParsedDefinitions.ScalarTypeDefinitions = append(p.ParsedDefinitions.ScalarTypeDefinitions, definition)
	return len(p.ParsedDefinitions.ScalarTypeDefinitions) - 1
}

func (p *Parser) putUnionTypeDefinition(definition document.UnionTypeDefinition) int {
	p.ParsedDefinitions.UnionTypeDefinitions = append(p.ParsedDefinitions.UnionTypeDefinitions, definition)
	return len(p.ParsedDefinitions.UnionTypeDefinitions) - 1
}

func (p *Parser) putByteSliceReference(slice document.ByteSliceReference) int {
	p.ParsedDefinitions.ByteSliceReferences = append(p.ParsedDefinitions.ByteSliceReferences, slice)
	return len(p.ParsedDefinitions.ByteSliceReferences) - 1
}

func (p *Parser) putValue(value document.Value, index int) {
	p.ParsedDefinitions.Values[index] = value
}

func (p *Parser) putInteger(integer int32) int {
	p.ParsedDefinitions.Integers = append(p.ParsedDefinitions.Integers, integer)
	return len(p.ParsedDefinitions.Integers) - 1
}

func (p *Parser) putFloat(float float32) int {
	p.ParsedDefinitions.Floats = append(p.ParsedDefinitions.Floats, float)
	return len(p.ParsedDefinitions.Floats) - 1
}

func (p *Parser) putListValue(value document.ListValue, index int) {
	p.ParsedDefinitions.ListValues[index] = value
}

func (p *Parser) putObjectValue(value document.ObjectValue, index int) {
	p.ParsedDefinitions.ObjectValues[index] = value
}

func (p *Parser) putObjectField(field document.ObjectField) int {
	p.ParsedDefinitions.ObjectFields = append(p.ParsedDefinitions.ObjectFields, field)
	return len(p.ParsedDefinitions.ObjectFields) - 1
}

func (p *Parser) putType(documentType document.Type, index int) {
	p.ParsedDefinitions.Types[index] = documentType
}
