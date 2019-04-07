//go:generate mockgen -source=$GOFILE -destination=./parser_mock_test.go -package=parser Lexer
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

func (i *indexPool) grow(minimumSliceSize int) {

	grow := 10
	if (len(*i) / 2) > grow {
		grow = len(*i) / 2
	}

	for k := 0; k < grow; k++ {
		*i = append(*i, make([]int, 0, minimumSliceSize))
	}
}

// Parser holds the lexer and a buffer for writing literals
type Parser struct {
	l                 Lexer
	ParsedDefinitions ParsedDefinitions
	indexPool         indexPool
	indexPoolPosition int
	options           Options
	cacheStats        cacheStats
	sliceIndex        map[string]int
}

func (p *Parser) FieldDefinition(ref int) document.FieldDefinition {
	return p.ParsedDefinitions.FieldDefinitions[ref]
}

func (p *Parser) EnumValueDefinition(ref int) document.EnumValueDefinition {
	return p.ParsedDefinitions.EnumValuesDefinitions[ref]
}

func (p *Parser) InputValueDefinition(ref int) document.InputValueDefinition {
	return p.ParsedDefinitions.InputValueDefinitions[ref]
}

// ParsedDefinitions contains all parsed definitions to avoid deeply nested data structures while parsing
type ParsedDefinitions struct {
	TypeSystemDefinition document.TypeSystemDefinition
	ExecutableDefinition document.ExecutableDefinition

	OperationDefinitions       document.OperationDefinitions
	FragmentDefinitions        document.FragmentDefinitions
	VariableDefinitions        document.VariableDefinitions
	Fields                     document.Fields
	InlineFragments            document.InlineFragments
	FragmentSpreads            document.FragmentSpreads
	Arguments                  document.Arguments
	ArgumentSets               []document.ArgumentSet
	Directives                 document.Directives
	DirectiveSets              []document.DirectiveSet
	EnumTypeDefinitions        []document.EnumTypeDefinition
	ArgumentsDefinitions       document.ArgumentsDefinitions
	EnumValuesDefinitions      []document.EnumValueDefinition
	FieldDefinitions           []document.FieldDefinition
	InputValueDefinitions      []document.InputValueDefinition
	InputObjectTypeDefinitions document.InputObjectTypeDefinitions
	DirectiveDefinitions       document.DirectiveDefinitions
	InterfaceTypeDefinitions   document.InterfaceTypeDefinitions
	ObjectTypeDefinitions      document.ObjectTypeDefinitions
	ScalarTypeDefinitions      document.ScalarTypeDefinitions
	UnionTypeDefinitions       document.UnionTypeDefinitions
	InputFieldsDefinitions     []document.InputFieldsDefinition
	Values                     []document.Value
	ListValues                 []document.ListValue
	ObjectValues               []document.ObjectValue
	ObjectFields               document.ObjectFields
	Types                      document.Types
	SelectionSets              []document.SelectionSet

	ByteSliceReferences []document.ByteSliceReference
	Integers            []int32
	Floats              []float32
	Booleans            [2]bool
}

type cacheStats struct {
	IndexPoolPosition          int
	TypeSystemDefinition       int
	ExecutableDefinition       int
	OperationDefinitions       int
	FragmentDefinitions        int
	VariableDefinitions        int
	Fields                     int
	InlineFragments            int
	FragmentSpreads            int
	Arguments                  int
	ArgumentSets               int
	Directives                 int
	DirectiveSets              int
	EnumTypeDefinitions        int
	ArgumentsDefinitions       int
	EnumValuesDefinitions      int
	FieldDefinitions           int
	InputValueDefinitions      int
	InputObjectTypeDefinitions int
	DirectiveDefinitions       int
	InterfaceTypeDefinitions   int
	ObjectTypeDefinitions      int
	ScalarTypeDefinitions      int
	UnionTypeDefinitions       int
	InputFieldsDefinitions     int
	Values                     int
	ListValues                 int
	ObjectValues               int
	ObjectFields               int
	Types                      int
	SelectionSets              int
	ByteSliceReferences        int
	Integers                   int
	Floats                     int
}

// Lexer is the interface used by the Parser to lex tokens
type Lexer interface {
	SetTypeSystemInput(input []byte) error
	ExtendTypeSystemInput(input []byte) error
	ResetTypeSystemInput()
	SetExecutableInput(input []byte) error
	AppendBytes(input []byte) (err error)
	Read() (tok token.Token)
	Peek(ignoreWhitespace bool) keyword.Keyword
	ByteSlice(reference document.ByteSliceReference) document.ByteSlice
	TextPosition() position.Position
}

type Options struct {
	poolSize         int
	minimumSliceSize int
}

type Option func(options *Options)

func WithPoolSize(poolSize int) Option {
	return func(options *Options) {
		options.poolSize = poolSize
	}
}

func WithMinimumSliceSize(size int) Option {
	return func(options *Options) {
		options.minimumSliceSize = size
	}
}

// NewParser returns a new parser using a buffered runestringer
func NewParser(withOptions ...Option) *Parser {

	options := Options{
		poolSize:         256,
		minimumSliceSize: 8,
	}

	for _, option := range withOptions {
		option(&options)
	}

	pool := make([][]int, options.poolSize)
	for i := 0; i < options.poolSize; i++ {
		pool[i] = make([]int, 0, options.minimumSliceSize)
	}

	definitions := ParsedDefinitions{
		OperationDefinitions:       make(document.OperationDefinitions, 0, options.minimumSliceSize),
		FragmentDefinitions:        make(document.FragmentDefinitions, 0, options.minimumSliceSize),
		VariableDefinitions:        make(document.VariableDefinitions, 0, options.minimumSliceSize),
		Fields:                     make(document.Fields, 0, options.minimumSliceSize*4),
		InlineFragments:            make(document.InlineFragments, 0, options.minimumSliceSize),
		FragmentSpreads:            make(document.FragmentSpreads, 0, options.minimumSliceSize),
		Arguments:                  make(document.Arguments, 0, options.minimumSliceSize),
		ArgumentSets:               make([]document.ArgumentSet, 0, options.minimumSliceSize),
		Directives:                 make(document.Directives, 0, options.minimumSliceSize),
		DirectiveSets:              make([]document.DirectiveSet, 0, options.minimumSliceSize*2),
		EnumTypeDefinitions:        make([]document.EnumTypeDefinition, 0, options.minimumSliceSize),
		EnumValuesDefinitions:      make([]document.EnumValueDefinition, 0, options.minimumSliceSize*2),
		ArgumentsDefinitions:       make(document.ArgumentsDefinitions, 0, options.minimumSliceSize),
		FieldDefinitions:           make([]document.FieldDefinition, 0, options.minimumSliceSize*2),
		InputValueDefinitions:      make([]document.InputValueDefinition, 0, options.minimumSliceSize),
		InputObjectTypeDefinitions: make(document.InputObjectTypeDefinitions, 0, options.minimumSliceSize),
		DirectiveDefinitions:       make(document.DirectiveDefinitions, 0, options.minimumSliceSize),
		InterfaceTypeDefinitions:   make(document.InterfaceTypeDefinitions, 0, options.minimumSliceSize),
		ObjectTypeDefinitions:      make(document.ObjectTypeDefinitions, 0, options.minimumSliceSize),
		ScalarTypeDefinitions:      make(document.ScalarTypeDefinitions, 0, options.minimumSliceSize),
		UnionTypeDefinitions:       make(document.UnionTypeDefinitions, 0, options.minimumSliceSize),
		InputFieldsDefinitions:     make([]document.InputFieldsDefinition, 0, options.minimumSliceSize),
		Values:                     make([]document.Value, 0, options.minimumSliceSize),
		ListValues:                 make([]document.ListValue, 0, options.minimumSliceSize),
		ObjectValues:               make([]document.ObjectValue, 0, options.minimumSliceSize),
		ObjectFields:               make(document.ObjectFields, 0, options.minimumSliceSize),
		Types:                      make(document.Types, 0, options.minimumSliceSize*2),
		SelectionSets:              make([]document.SelectionSet, 0, options.minimumSliceSize*2),
		Integers:                   make([]int32, 0, options.minimumSliceSize),
		Floats:                     make([]float32, 0, options.minimumSliceSize),
		ByteSliceReferences:        make([]document.ByteSliceReference, 0, options.minimumSliceSize),
	}

	definitions.Booleans[0] = false
	definitions.Booleans[1] = true

	return &Parser{
		l:                 lexer.NewLexer(),
		indexPool:         pool,
		ParsedDefinitions: definitions,
		options:           options,
		sliceIndex:        make(map[string]int, 1024),
	}
}

func (p *Parser) ByteSlice(reference document.ByteSliceReference) document.ByteSlice {
	return p.l.ByteSlice(reference)
}

func (p *Parser) CachedByteSlice(i int) document.ByteSlice {
	if i == -1 {
		return nil
	}
	return p.l.ByteSlice(p.ParsedDefinitions.ByteSliceReferences[i])
}

func (p *Parser) TextPosition() position.Position {
	return p.l.TextPosition()
}

// ParseTypeSystemDefinition parses a TypeSystemDefinition from an io.Reader
func (p *Parser) ParseTypeSystemDefinition(input []byte) (err error) {
	p.resetCaches()
	err = p.l.SetTypeSystemInput(input)
	if err != nil {
		return
	}

	p.initTypeSystemDefinition()
	err = p.parseTypeSystemDefinition()
	p.setCacheStats()

	return err
}

func (p *Parser) ExtendTypeSystemDefinition(input []byte) (err error) {
	err = p.l.ExtendTypeSystemInput(input)
	if err != nil {
		return
	}
	err = p.parseTypeSystemDefinition()
	if err != nil {
		return
	}
	p.setCacheStats()
	return
}

// ParseExecutableDefinition parses an ExecutableDefinition from an io.Reader
func (p *Parser) ParseExecutableDefinition(input []byte) (err error) {
	p.resetExecutableCaches()
	err = p.l.SetExecutableInput(input)
	if err != nil {
		return
	}

	p.initExecutableDefinition()
	err = p.parseExecutableDefinition()
	return err
}

func (p *Parser) readExpect(expected keyword.Keyword, enclosingFunctionName string) (t token.Token, err error) {
	t = p.l.Read()
	if t.Keyword != expected {
		return t, newErrInvalidType(t.TextPosition, enclosingFunctionName, expected.String(), t.Keyword.String()+" lit: "+string(p.ByteSlice(t.Literal)))
	}

	return
}

func (p *Parser) peekExpect(expected keyword.Keyword, swallow bool) bool {
	matches := expected == p.l.Peek(true)
	if swallow && matches {
		p.l.Read()
	}

	return matches
}

func (p *Parser) peekExpectSwallow(expected keyword.Keyword) (tok token.Token, matches bool) {
	matches = expected == p.l.Peek(true)
	if matches {
		tok = p.l.Read()
	}

	return
}

func (p *Parser) IndexPoolGet() []int {
	p.indexPoolPosition++

	if len(p.indexPool)-1 <= p.indexPoolPosition {
		p.indexPool.grow(p.options.minimumSliceSize)
	}

	return p.indexPool[p.indexPoolPosition][:0]
}

func (p *Parser) initSelectionSet(set *document.SelectionSet) {
	set.InlineFragments = p.IndexPoolGet()
	set.FragmentSpreads = p.IndexPoolGet()
	set.Fields = p.IndexPoolGet()
}

func (p *Parser) initField(field *document.Field) {
	field.DirectiveSet = -1
	field.ArgumentSet = -1
	field.SelectionSet = -1
}

func (p *Parser) makeFieldDefinition() document.FieldDefinition {
	return document.FieldDefinition{
		DirectiveSet:        -1,
		ArgumentsDefinition: -1,
	}
}

func (p *Parser) makeEnumTypeDefinition() document.EnumTypeDefinition {
	return document.EnumTypeDefinition{
		DirectiveSet: -1,
	}
}

func (p *Parser) makeInputValueDefinition() document.InputValueDefinition {
	return document.InputValueDefinition{
		DefaultValue: -1,
		DirectiveSet: -1,
	}
}

func (p *Parser) makeInputObjectTypeDefinition() document.InputObjectTypeDefinition {
	return document.InputObjectTypeDefinition{
		DirectiveSet: -1,
	}
}

func (p *Parser) initTypeSystemDefinition() {
	p.ParsedDefinitions.TypeSystemDefinition.SchemaDefinition = document.SchemaDefinition{
		DirectiveSet: -1,
	}
}

func (p *Parser) makeInterfaceTypeDefinition() document.InterfaceTypeDefinition {
	return document.InterfaceTypeDefinition{
		DirectiveSet: -1,
	}
}

func (p *Parser) makeObjectTypeDefinition() document.ObjectTypeDefinition {
	return document.ObjectTypeDefinition{
		DirectiveSet: -1,
	}
}

func (p *Parser) makeScalarTypeDefinition() document.ScalarTypeDefinition {
	return document.ScalarTypeDefinition{
		DirectiveSet: -1,
	}
}

func (p *Parser) makeUnionTypeDefinition() document.UnionTypeDefinition {
	return document.UnionTypeDefinition{
		DirectiveSet:     -1,
		UnionMemberTypes: p.IndexPoolGet(),
	}
}

func (p *Parser) makeEnumValueDefinition() document.EnumValueDefinition {
	return document.EnumValueDefinition{
		DirectiveSet: -1,
	}
}

func (p *Parser) initFragmentDefinition(definition *document.FragmentDefinition) {
	definition.DirectiveSet = -1
	definition.SelectionSet = -1
}

func (p *Parser) initOperationDefinition(definition *document.OperationDefinition) {
	definition.DirectiveSet = -1
	definition.VariableDefinitions = p.IndexPoolGet()
	definition.SelectionSet = -1
}

func (p *Parser) initInlineFragment(fragment *document.InlineFragment) {
	fragment.DirectiveSet = -1
	fragment.TypeCondition = -1
	fragment.SelectionSet = -1
}

func (p *Parser) InitDirectiveSet(set *document.DirectiveSet) {
	*set = p.IndexPoolGet()
}

func (p *Parser) makeFragmentSpread() document.FragmentSpread {
	return document.FragmentSpread{
		DirectiveSet: -1,
	}
}

func (p *Parser) initExecutableDefinition() {
	p.ParsedDefinitions.ExecutableDefinition.OperationDefinitions = p.IndexPoolGet()
	p.ParsedDefinitions.ExecutableDefinition.FragmentDefinitions = p.IndexPoolGet()
}

func (p *Parser) makeListValue(index *int) document.ListValue {
	value := p.IndexPoolGet()
	p.ParsedDefinitions.ListValues = append(p.ParsedDefinitions.ListValues, value)
	*index = len(p.ParsedDefinitions.ListValues) - 1
	return value
}

func (p *Parser) makeObjectValue(index *int) document.ObjectValue {
	value := p.IndexPoolGet()
	p.ParsedDefinitions.ObjectValues = append(p.ParsedDefinitions.ObjectValues, value)
	*index = len(p.ParsedDefinitions.ObjectValues) - 1
	return value
}

func (p *Parser) makeValue() (value document.Value, ref int) {
	p.ParsedDefinitions.Values = append(p.ParsedDefinitions.Values, value)
	ref = len(p.ParsedDefinitions.Values) - 1
	return
}

func (p *Parser) initArgumentSet(set *document.ArgumentSet) {
	*set = p.IndexPoolGet()
}

func (p *Parser) makeType(index *int) document.Type {
	documentType := document.Type{
		OfType: -1,
	}
	p.ParsedDefinitions.Types = append(p.ParsedDefinitions.Types, documentType)
	*index = len(p.ParsedDefinitions.Types) - 1
	return documentType
}

func (p *Parser) setCacheStats() {
	p.cacheStats.IndexPoolPosition = p.indexPoolPosition
	p.cacheStats.OperationDefinitions = len(p.ParsedDefinitions.OperationDefinitions)
	p.cacheStats.FragmentDefinitions = len(p.ParsedDefinitions.FragmentDefinitions)
	p.cacheStats.VariableDefinitions = len(p.ParsedDefinitions.VariableDefinitions)
	p.cacheStats.Fields = len(p.ParsedDefinitions.Fields)
	p.cacheStats.InlineFragments = len(p.ParsedDefinitions.InlineFragments)
	p.cacheStats.FragmentSpreads = len(p.ParsedDefinitions.FragmentSpreads)
	p.cacheStats.Arguments = len(p.ParsedDefinitions.Arguments)
	p.cacheStats.ArgumentSets = len(p.ParsedDefinitions.ArgumentSets)
	p.cacheStats.Directives = len(p.ParsedDefinitions.Directives)
	p.cacheStats.DirectiveSets = len(p.ParsedDefinitions.DirectiveSets)
	p.cacheStats.EnumTypeDefinitions = len(p.ParsedDefinitions.EnumTypeDefinitions)
	p.cacheStats.EnumValuesDefinitions = len(p.ParsedDefinitions.EnumValuesDefinitions)
	p.cacheStats.FieldDefinitions = len(p.ParsedDefinitions.FieldDefinitions)
	p.cacheStats.InputValueDefinitions = len(p.ParsedDefinitions.InputValueDefinitions)
	p.cacheStats.InputObjectTypeDefinitions = len(p.ParsedDefinitions.InputObjectTypeDefinitions)
	p.cacheStats.DirectiveDefinitions = len(p.ParsedDefinitions.DirectiveDefinitions)
	p.cacheStats.InterfaceTypeDefinitions = len(p.ParsedDefinitions.InterfaceTypeDefinitions)
	p.cacheStats.ObjectTypeDefinitions = len(p.ParsedDefinitions.ObjectTypeDefinitions)
	p.cacheStats.ScalarTypeDefinitions = len(p.ParsedDefinitions.ScalarTypeDefinitions)
	p.cacheStats.UnionTypeDefinitions = len(p.ParsedDefinitions.UnionTypeDefinitions)
	p.cacheStats.ByteSliceReferences = len(p.ParsedDefinitions.ByteSliceReferences)
	p.cacheStats.Values = len(p.ParsedDefinitions.Values)
	p.cacheStats.Integers = len(p.ParsedDefinitions.Integers)
	p.cacheStats.Floats = len(p.ParsedDefinitions.Floats)
	p.cacheStats.ListValues = len(p.ParsedDefinitions.ListValues)
	p.cacheStats.ObjectValues = len(p.ParsedDefinitions.ObjectValues)
	p.cacheStats.ObjectFields = len(p.ParsedDefinitions.ObjectFields)
	p.cacheStats.Types = len(p.ParsedDefinitions.Types)
	p.cacheStats.SelectionSets = len(p.ParsedDefinitions.SelectionSets)
	p.cacheStats.ArgumentsDefinitions = len(p.ParsedDefinitions.ArgumentsDefinitions)
	p.cacheStats.InputFieldsDefinitions = len(p.ParsedDefinitions.InputFieldsDefinitions)
}

func (p *Parser) resetCaches() {

	p.indexPoolPosition = -1

	p.ParsedDefinitions.OperationDefinitions = p.ParsedDefinitions.OperationDefinitions[:0]
	p.ParsedDefinitions.FragmentDefinitions = p.ParsedDefinitions.FragmentDefinitions[:0]
	p.ParsedDefinitions.VariableDefinitions = p.ParsedDefinitions.VariableDefinitions[:0]
	p.ParsedDefinitions.Fields = p.ParsedDefinitions.Fields[:0]
	p.ParsedDefinitions.InlineFragments = p.ParsedDefinitions.InlineFragments[:0]
	p.ParsedDefinitions.FragmentSpreads = p.ParsedDefinitions.FragmentSpreads[:0]
	p.ParsedDefinitions.Arguments = p.ParsedDefinitions.Arguments[:0]
	p.ParsedDefinitions.ArgumentSets = p.ParsedDefinitions.ArgumentSets[:0]
	p.ParsedDefinitions.Directives = p.ParsedDefinitions.Directives[:0]
	p.ParsedDefinitions.DirectiveSets = p.ParsedDefinitions.DirectiveSets[:0]
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
	p.ParsedDefinitions.SelectionSets = p.ParsedDefinitions.SelectionSets[:0]
	p.ParsedDefinitions.ArgumentsDefinitions = p.ParsedDefinitions.ArgumentsDefinitions[:0]
	p.ParsedDefinitions.InputFieldsDefinitions = p.ParsedDefinitions.InputFieldsDefinitions[:0]
}

func (p *Parser) resetExecutableCaches() {

	s := p.cacheStats

	p.indexPoolPosition = s.IndexPoolPosition
	p.ParsedDefinitions.OperationDefinitions = p.ParsedDefinitions.OperationDefinitions[:s.OperationDefinitions]
	p.ParsedDefinitions.FragmentDefinitions = p.ParsedDefinitions.FragmentDefinitions[:s.FragmentDefinitions]
	p.ParsedDefinitions.VariableDefinitions = p.ParsedDefinitions.VariableDefinitions[:s.VariableDefinitions]
	p.ParsedDefinitions.Fields = p.ParsedDefinitions.Fields[:s.Fields]
	p.ParsedDefinitions.InlineFragments = p.ParsedDefinitions.InlineFragments[:s.InlineFragments]
	p.ParsedDefinitions.FragmentSpreads = p.ParsedDefinitions.FragmentSpreads[:s.FragmentSpreads]
	p.ParsedDefinitions.Arguments = p.ParsedDefinitions.Arguments[:s.Arguments]
	p.ParsedDefinitions.ArgumentSets = p.ParsedDefinitions.ArgumentSets[:s.ArgumentSets]
	p.ParsedDefinitions.Directives = p.ParsedDefinitions.Directives[:s.Directives]
	p.ParsedDefinitions.DirectiveSets = p.ParsedDefinitions.DirectiveSets[:s.DirectiveSets]
	p.ParsedDefinitions.EnumTypeDefinitions = p.ParsedDefinitions.EnumTypeDefinitions[:s.EnumTypeDefinitions]
	p.ParsedDefinitions.EnumValuesDefinitions = p.ParsedDefinitions.EnumValuesDefinitions[:s.EnumValuesDefinitions]
	p.ParsedDefinitions.FieldDefinitions = p.ParsedDefinitions.FieldDefinitions[:s.FieldDefinitions]
	p.ParsedDefinitions.InputValueDefinitions = p.ParsedDefinitions.InputValueDefinitions[:s.InputValueDefinitions]
	p.ParsedDefinitions.InputObjectTypeDefinitions = p.ParsedDefinitions.InputObjectTypeDefinitions[:s.InputObjectTypeDefinitions]
	p.ParsedDefinitions.DirectiveDefinitions = p.ParsedDefinitions.DirectiveDefinitions[:s.DirectiveDefinitions]
	p.ParsedDefinitions.InterfaceTypeDefinitions = p.ParsedDefinitions.InterfaceTypeDefinitions[:s.InterfaceTypeDefinitions]
	p.ParsedDefinitions.ObjectTypeDefinitions = p.ParsedDefinitions.ObjectTypeDefinitions[:s.ObjectTypeDefinitions]
	p.ParsedDefinitions.ScalarTypeDefinitions = p.ParsedDefinitions.ScalarTypeDefinitions[:s.ScalarTypeDefinitions]
	p.ParsedDefinitions.UnionTypeDefinitions = p.ParsedDefinitions.UnionTypeDefinitions[:s.UnionTypeDefinitions]
	p.ParsedDefinitions.ByteSliceReferences = p.ParsedDefinitions.ByteSliceReferences[:s.ByteSliceReferences]
	p.ParsedDefinitions.Values = p.ParsedDefinitions.Values[:s.Values]
	p.ParsedDefinitions.Integers = p.ParsedDefinitions.Integers[:s.Integers]
	p.ParsedDefinitions.Floats = p.ParsedDefinitions.Floats[:s.Floats]
	p.ParsedDefinitions.ListValues = p.ParsedDefinitions.ListValues[:s.ListValues]
	p.ParsedDefinitions.ObjectValues = p.ParsedDefinitions.ObjectValues[:s.ObjectValues]
	p.ParsedDefinitions.ObjectFields = p.ParsedDefinitions.ObjectFields[:s.ObjectFields]
	p.ParsedDefinitions.Types = p.ParsedDefinitions.Types[:s.Types]
	p.ParsedDefinitions.SelectionSets = p.ParsedDefinitions.SelectionSets[:s.SelectionSets]
	p.ParsedDefinitions.ArgumentsDefinitions = p.ParsedDefinitions.ArgumentsDefinitions[:s.ArgumentsDefinitions]
	p.ParsedDefinitions.InputFieldsDefinitions = p.ParsedDefinitions.InputFieldsDefinitions[:s.InputFieldsDefinitions]
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

	for i, current := range p.ParsedDefinitions.FragmentSpreads {

		if spread.FragmentName != current.FragmentName {
			continue
		}

		if spread.DirectiveSet == -1 && current.DirectiveSet == -1 {
			return i
		}

		if spread.DirectiveSet == -1 || current.DirectiveSet == -1 {
			continue
		}

		if p.integersContainSameValues(
			p.ParsedDefinitions.DirectiveSets[spread.DirectiveSet],
			p.ParsedDefinitions.DirectiveSets[current.DirectiveSet]) {
			return i
		}
	}

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

func (p *Parser) _putByteSliceReference(slice document.ByteSliceReference) int {
	p.ParsedDefinitions.ByteSliceReferences = append(p.ParsedDefinitions.ByteSliceReferences, slice)
	return len(p.ParsedDefinitions.ByteSliceReferences) - 1
}

func (p *Parser) putValue(value document.Value, index int) {
	p.ParsedDefinitions.Values[index] = value
}

func (p *Parser) putInteger(integer int32) int {

	for i, known := range p.ParsedDefinitions.Integers {
		if known == integer {
			return i
		}
	}

	p.ParsedDefinitions.Integers = append(p.ParsedDefinitions.Integers, integer)
	return len(p.ParsedDefinitions.Integers) - 1
}

func (p *Parser) putFloat(float float32) int {

	for i, known := range p.ParsedDefinitions.Floats {
		if known == float {
			return i
		}
	}

	p.ParsedDefinitions.Floats = append(p.ParsedDefinitions.Floats, float)
	return len(p.ParsedDefinitions.Floats) - 1
}

func (p *Parser) putListValue(value document.ListValue, index *int) {
	for i, known := range p.ParsedDefinitions.ListValues {
		if p.integersContainSameValues(value, known) {
			p.ParsedDefinitions.ListValues = // delete the dupe
				append(p.ParsedDefinitions.ListValues[:*index], p.ParsedDefinitions.ListValues[*index+1:]...)
			*index = i
			return
		}
	}
	p.ParsedDefinitions.ListValues[*index] = value
}

func (p *Parser) putObjectValue(value document.ObjectValue, index *int) {

	if len(value) == 0 {
		p.ParsedDefinitions.ObjectValues[*index] = value
		return
	}

	for i, known := range p.ParsedDefinitions.ObjectValues {
		if p.integersContainSameValues(value, known) {
			p.ParsedDefinitions.ObjectValues = // delete the dupe
				append(p.ParsedDefinitions.ObjectValues[:*index], p.ParsedDefinitions.ObjectValues[*index+1:]...)
			*index = i
		}
	}

	p.ParsedDefinitions.ObjectValues[*index] = value
}

func (p *Parser) putObjectField(field document.ObjectField) int {

	for i, known := range p.ParsedDefinitions.ObjectFields {
		if field.Name == known.Name && field.Value == known.Value {
			return i
		}
	}

	p.ParsedDefinitions.ObjectFields = append(p.ParsedDefinitions.ObjectFields, field)
	return len(p.ParsedDefinitions.ObjectFields) - 1
}

func (p *Parser) putType(documentType document.Type, index int) {
	p.ParsedDefinitions.Types[index] = documentType
}

func (p *Parser) putArgumentsDefinition(definition document.ArgumentsDefinition) int {
	p.ParsedDefinitions.ArgumentsDefinitions = append(p.ParsedDefinitions.ArgumentsDefinitions, definition)
	return len(p.ParsedDefinitions.ArgumentsDefinitions) - 1
}

func (p *Parser) putInputFieldsDefinitions(definition document.InputFieldsDefinition) int {
	p.ParsedDefinitions.InputFieldsDefinitions = append(p.ParsedDefinitions.InputFieldsDefinitions, definition)
	return len(p.ParsedDefinitions.InputFieldsDefinitions) - 1
}

func (p *Parser) putArgumentSet(set document.ArgumentSet) int {

	if len(set) == 0 {
		return -1
	}

	p.ParsedDefinitions.ArgumentSets = append(p.ParsedDefinitions.ArgumentSets, set)
	return len(p.ParsedDefinitions.ArgumentSets) - 1
}

func (p *Parser) putDirectiveSet(set document.DirectiveSet) int {

	if len(set) == 0 {
		return -1
	}

	p.ParsedDefinitions.DirectiveSets = append(p.ParsedDefinitions.DirectiveSets, set)
	return len(p.ParsedDefinitions.DirectiveSets) - 1
}

func (p *Parser) putSelectionSet(set document.SelectionSet) int {
	p.ParsedDefinitions.SelectionSets = append(p.ParsedDefinitions.SelectionSets, set)
	return len(p.ParsedDefinitions.SelectionSets) - 1
}

func (p *Parser) integersContainSameValues(left []int, right []int) bool {
	if len(left) != len(right) {
		return false
	}
	for _, i := range left {
		if !p.integersContainValue(right, i) {
			return false
		}
	}
	return true
}

func (p *Parser) integersContainValue(integer []int, want int) bool {
	for _, got := range integer {
		if want == got {
			return true
		}
	}
	return false
}
