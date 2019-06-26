package astparser

import (
	"fmt"
	"github.com/jensneuse/graphql-go-tools/pkg/ast"
	"github.com/jensneuse/graphql-go-tools/pkg/input"
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/keyword"
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/position"
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/token"
	"runtime"
)

type Lexer interface {
	SetInput(input *input.Input)
	Peek(ignoreWhitespace bool) keyword.Keyword
	Read() token.Token
}

type origin struct {
	file     string
	line     int
	funcName string
}

type ErrUnexpectedToken struct {
	keyword  keyword.Keyword
	expected []keyword.Keyword
	position position.Position
	literal  string
	origins  []origin
}

func (e ErrUnexpectedToken) Error() string {

	origins := ""
	for _, origin := range e.origins {
		origins = origins + fmt.Sprintf("\n\t\t%s:%d\n\t\t%s", origin.file, origin.line, origin.funcName)
	}

	return fmt.Sprintf("unexpected token - keyword: '%s' literal: '%s' - expected: '%s' position: '%s'%s", e.keyword, e.literal, e.expected, e.position, origins)
}

type Parser struct {
	lexer    Lexer
	input    *input.Input
	document *ast.Document
	err      error
}

func NewParser(lexer Lexer) *Parser {
	return &Parser{
		lexer: lexer,
	}
}

func (p *Parser) Parse(input *input.Input, document *ast.Document) error {
	p.input = input
	p.document = document
	p.lexer.SetInput(input)
	p.parse()
	return p.err
}

func (p *Parser) parse() {

	for {
		next := p.peek(true)
		switch next {
		case keyword.SCHEMA:
			p.parseSchema()
		case keyword.STRING, keyword.BLOCKSTRING:
			p.parseRootDescription()
		case keyword.SCALAR:
			p.parseScalarTypeDefinition(nil)
		case keyword.TYPE:
			p.parseObjectTypeDefinition(nil)
		case keyword.INPUT:
			p.parseInputObjectTypeDefinition(nil)
		case keyword.INTERFACE:
			p.parseInterfaceTypeDefinition(nil)
		case keyword.UNION:
			p.parseUnionTypeDefinition(nil)
		case keyword.ENUM:
			p.parseEnumTypeDefinition(nil)
		case keyword.DIRECTIVE:
			p.parseDirectiveDefinition(nil)
		case keyword.EOF:
			p.read()
			return
		default:
			p.errUnexpectedToken(p.read())
		}

		if p.err != nil {
			return
		}
	}
}

func (p *Parser) read() token.Token {
	return p.lexer.Read()
}

func (p *Parser) peek(ignoreWhitespace bool) keyword.Keyword {
	return p.lexer.Peek(ignoreWhitespace)
}

func (p *Parser) errUnexpectedToken(unexpected token.Token, expectedKeywords ...keyword.Keyword) {

	origins := make([]origin, 3)
	for i := range origins {
		fpcs := make([]uintptr, 1)
		callers := runtime.Callers(2+i, fpcs)

		if callers == 0 {
			origins = origins[:i]
			break
		}

		fn := runtime.FuncForPC(fpcs[0])
		file, line := fn.FileLine(fpcs[0])

		origins[i].file = file
		origins[i].line = line
		origins[i].funcName = fn.Name()
	}

	p.err = ErrUnexpectedToken{
		keyword:  unexpected.Keyword,
		position: unexpected.TextPosition,
		literal:  p.input.ByteSliceString(unexpected.Literal),
		origins:  origins,
	}
}

func (p *Parser) mustRead(oneOf ...keyword.Keyword) (next token.Token) {
	next = p.read()
	if len(oneOf) == 0 {
		return
	}

	for i := range oneOf {
		if next.Keyword == oneOf[i] {
			return
		}
	}

	p.errUnexpectedToken(next, oneOf...)
	return
}

func (p *Parser) parseSchema() {

	schemaLiteral := p.read()

	schemaDefinition := ast.SchemaDefinition{
		SchemaLiteral: schemaLiteral.TextPosition,
	}

	if p.peek(true) == keyword.AT {
		schemaDefinition.Directives = p.parseDirectiveList()
	}

	schemaDefinition.RootOperationTypeDefinitions = p.parseRootOperationTypeDefinitionList()

	ref := p.document.PutSchemaDefinition(schemaDefinition)

	definition := ast.Definition{
		Kind: ast.SchemaDefinitionKind,
		Ref:  ref,
	}

	p.document.PutDefinition(definition)
}

func (p *Parser) parseRootOperationTypeDefinitionList() (list ast.RootOperationTypeDefinitionList) {

	curlyBracketOpen := p.mustRead(keyword.CURLYBRACKETOPEN)

	previous := -1

	for {
		next := p.peek(true)
		switch next {
		case keyword.CURLYBRACKETCLOSE:

			curlyBracketClose := p.read()
			list.Open = curlyBracketOpen.TextPosition
			list.Close = curlyBracketClose.TextPosition

			return list
		case keyword.QUERY, keyword.MUTATION, keyword.SUBSCRIPTION:

			operationType := p.read()
			colon := p.mustRead(keyword.COLON)
			namedType := p.mustRead(keyword.IDENT)

			rootOperationTypeDefinition := ast.RootOperationTypeDefinition{
				OperationType: p.operationTypeFromKeyword(operationType.Keyword),
				Colon:         colon.TextPosition,
				NamedType: ast.Type{
					TypeKind: ast.TypeKindNamed,
					Name:     namedType.Literal,
				},
			}

			ref := p.document.PutRootOperationTypeDefinition(rootOperationTypeDefinition)

			if !list.HasNext() {
				list.SetFirst(ref)
			}

			if previous != -1 {
				p.document.RootOperationTypeDefinitions[previous].SetNext(ref)
			}

			previous = ref

		default:
			p.errUnexpectedToken(p.read())
			return
		}
	}
}

func (p *Parser) operationTypeFromKeyword(key keyword.Keyword) ast.OperationType {
	switch key {
	case keyword.QUERY:
		return ast.OperationTypeQuery
	case keyword.MUTATION:
		return ast.OperationTypeMutation
	case keyword.SUBSCRIPTION:
		return ast.OperationTypeSubscription
	default:
		return ast.OperationTypeUndefined
	}
}

func (p *Parser) parseDirectiveList() (directives ast.DirectiveList) {

	previous := -1

	for {

		if p.peek(true) != keyword.AT {
			break
		}

		at := p.read()
		name := p.mustRead(keyword.IDENT)

		directive := ast.Directive{
			At:   at.TextPosition,
			Name: name.Literal,
		}

		if p.peek(true) == keyword.BRACKETOPEN {
			directive.ArgumentList = p.parseArgumentList()
		}

		ref := p.document.PutDirective(directive)

		if !directives.HasNext() {
			directives.SetFirst(ref)
		}

		if previous != -1 {
			p.document.Directives[previous].SetNext(ref)
		}

		previous = ref
	}

	return
}

func (p *Parser) parseArgumentList() (arguments ast.ArgumentList) {

	bracketOpen := p.read()

	previous := -1

	for {
		if p.peek(true) != keyword.IDENT {
			break
		}

		name := p.read()
		colon := p.mustRead(keyword.COLON)
		value := p.parseValue()

		argument := ast.Argument{
			Name:  name.Literal,
			Colon: colon.TextPosition,
			Value: value,
		}

		ref := p.document.PutArgument(argument)

		if !arguments.HasNext() {
			arguments.SetFirst(ref)
		}

		if previous != -1 {
			p.document.Arguments[previous].SetNext(ref)
		}

		previous = ref
	}

	bracketClose := p.mustRead(keyword.BRACKETCLOSE)

	arguments.Open = bracketOpen.TextPosition
	arguments.Close = bracketClose.TextPosition

	return
}

func (p *Parser) parseValue() (value ast.Value) {

	next := p.peek(true)

	switch next {
	case keyword.STRING, keyword.BLOCKSTRING:
		value.Kind = ast.ValueKindString
		value.Ref = p.parseStringValue()
	case keyword.IDENT:
		value.Kind = ast.ValueKindEnum
		value.Ref = p.parseEnumValue()
	case keyword.TRUE, keyword.FALSE:
		value.Kind = ast.ValueKindBoolean
		value.Ref = p.parseBooleanValue()
	case keyword.DOLLAR:
		value.Kind = ast.ValueKindVariable
		value.Ref = p.parseVariableValue()
	case keyword.INTEGER:
		value.Kind = ast.ValueKindInteger
		value.Ref = p.parseIntegerValue(nil)
	case keyword.FLOAT:
		value.Kind = ast.ValueKindFloat
		value.Ref = p.parseFloatValue(nil)
	case keyword.NEGATIVESIGN:
		return p.parseNegativeNumberValue()
	case keyword.NULL:
		value.Kind = ast.ValueKindNull
		p.read()
	case keyword.SQUAREBRACKETOPEN:
		value.Kind = ast.ValueKindList
		value.Ref = p.parseValueList()
	case keyword.CURLYBRACKETOPEN:
		value.Kind = ast.ValueKindObject
		value.Ref = p.parseObjectValue()
	default:
		p.errUnexpectedToken(p.read())
	}

	return
}

func (p *Parser) parseObjectValue() int {
	var objectValue ast.ObjectValue
	objectValue.Open = p.mustRead(keyword.CURLYBRACKETOPEN).TextPosition

	previous := -1
	for {
		next := p.peek(true)
		switch next {
		case keyword.CURLYBRACKETCLOSE:
			objectValue.Close = p.read().TextPosition
			return p.document.PutObjectValue(objectValue)
		case keyword.IDENT:
			ref := p.parseObjectField()
			if !objectValue.HasNext() {
				objectValue.SetFirst(ref)
			}
			if previous != -1 {
				p.document.ObjectFields[previous].SetNext(ref)
			}
			previous = ref
		default:
			p.errUnexpectedToken(p.read(), keyword.IDENT, keyword.CURLYBRACKETCLOSE)
			return -1
		}
	}
}

func (p *Parser) parseObjectField() int {
	name := p.mustRead(keyword.IDENT)
	colon := p.mustRead(keyword.COLON)
	value := p.parseValue()
	return p.document.PutObjectField(ast.ObjectField{
		Name:  name.Literal,
		Colon: colon.TextPosition,
		Value: value,
	})
}

func (p *Parser) parseValueList() int {
	var list ast.ValueList
	list.Open = p.mustRead(keyword.SQUAREBRACKETOPEN).TextPosition

	previous := -1

	for {
		next := p.peek(true)
		switch next {
		case keyword.SQUAREBRACKETCLOSE:
			list.Close = p.read().TextPosition
			return p.document.PutValueList(list)
		default:
			value := p.parseValue()
			ref := p.document.PutValue(value)
			if !list.HasNext() {
				list.SetFirst(ref)
			}
			if previous != -1 {
				p.document.Values[previous].SetNext(ref)
			}
			previous = ref
		}
	}
}

func (p *Parser) parseNegativeNumberValue() (value ast.Value) {
	negativeSign := p.mustRead(keyword.NEGATIVESIGN).TextPosition
	switch p.peek(false) {
	case keyword.INTEGER:
		value.Kind = ast.ValueKindInteger
		value.Ref = p.parseIntegerValue(&negativeSign)
	case keyword.FLOAT:
		value.Kind = ast.ValueKindFloat
		value.Ref = p.parseFloatValue(&negativeSign)
	default:
		p.errUnexpectedToken(p.read(), keyword.INTEGER, keyword.FLOAT)
	}
	return
}

func (p *Parser) parseFloatValue(negativeSign *position.Position) int {
	floatValue := ast.FloatValue{
		Raw: p.mustRead(keyword.FLOAT).Literal,
	}
	if negativeSign != nil {
		floatValue.Negative = true
		floatValue.NegativeSign = *negativeSign
	}
	return p.document.PutFloatValue(floatValue)
}

func (p *Parser) parseIntegerValue(negativeSign *position.Position) int {
	intValue := ast.IntValue{
		Raw: p.mustRead(keyword.INTEGER).Literal,
	}
	if negativeSign != nil {
		intValue.Negative = true
		intValue.NegativeSign = *negativeSign
	}
	return p.document.PutIntValue(intValue)
}

func (p *Parser) parseVariableValue() int {
	dollar := p.mustRead(keyword.DOLLAR)
	var value token.Token
	if p.peek(false) == keyword.IDENT {
		value = p.read()
	} else {
		p.errUnexpectedToken(p.read(), keyword.IDENT, keyword.INTEGER)
		return -1
	}
	switch p.peek(false) {
	case keyword.TAB, keyword.SPACE, keyword.LINETERMINATOR, keyword.EOF:
		break
	default:
		p.errUnexpectedToken(p.read(), keyword.TAB, keyword.SPACE, keyword.LINETERMINATOR)
		return -1
	}
	return p.document.PutVariableValue(ast.VariableValue{
		Dollar: dollar.TextPosition,
		Name:   value.Literal,
	})
}

func (p *Parser) parseBooleanValue() int {
	value := p.mustRead(keyword.TRUE, keyword.FALSE)
	switch value.Keyword {
	case keyword.FALSE:
		return 0
	case keyword.TRUE:
		return 1
	default:
		return -1
	}
}

func (p *Parser) parseEnumValue() int {
	value := p.mustRead(keyword.IDENT)
	return p.document.PutEnumValue(ast.EnumValue{
		Name: value.Literal,
	})
}

func (p *Parser) parseStringValue() int {
	value := p.mustRead(keyword.STRING, keyword.BLOCKSTRING)
	return p.document.PutStringValue(ast.StringValue{
		Content:     value.Literal,
		BlockString: value.Keyword == keyword.BLOCKSTRING,
	})
}

func (p *Parser) parseObjectTypeDefinition(description *ast.Description) {

	var objectTypeDefinition ast.ObjectTypeDefinition
	if description != nil {
		objectTypeDefinition.Description = *description
	}
	objectTypeDefinition.TypeLiteral = p.mustRead(keyword.TYPE).TextPosition
	objectTypeDefinition.Name = p.mustRead(keyword.IDENT).Literal
	if p.peek(true) == keyword.IMPLEMENTS {
		objectTypeDefinition.ImplementsInterfaces = p.parseImplementsInterfaces()
	}
	if p.peek(true) == keyword.AT {
		objectTypeDefinition.Directives = p.parseDirectiveList()
	}
	if p.peek(true) == keyword.CURLYBRACKETOPEN {
		objectTypeDefinition.FieldsDefinition = p.parseFieldDefinitionList()
	}
	p.document.PutObjectTypeDefinition(objectTypeDefinition)
}

func (p *Parser) parseRootDescription() {

	description := p.parseDescription()

	next := p.peek(true)
	switch next {
	case keyword.TYPE:
		p.parseObjectTypeDefinition(&description)
	case keyword.INPUT:
		p.parseInputObjectTypeDefinition(&description)
	case keyword.SCALAR:
		p.parseScalarTypeDefinition(&description)
	case keyword.INTERFACE:
		p.parseInterfaceTypeDefinition(&description)
	case keyword.UNION:
		p.parseUnionTypeDefinition(&description)
	case keyword.ENUM:
		p.parseEnumTypeDefinition(&description)
	case keyword.DIRECTIVE:
		p.parseDirectiveDefinition(&description)
	default:
		p.errUnexpectedToken(p.read())
	}
}

func (p *Parser) parseImplementsInterfaces() (list ast.TypeList) {

	list.Open = p.read().TextPosition

	acceptIdent := true
	acceptAnd := true

	previous := -1

	for {
		next := p.peek(true)
		switch next {
		case keyword.AND:
			if acceptAnd {
				acceptAnd = false
				acceptIdent = true
				p.read()
			} else {
				p.errUnexpectedToken(p.read())
				return
			}
		case keyword.IDENT:
			if acceptIdent {
				acceptIdent = false
				acceptAnd = true
				name := p.read()
				ref := p.document.PutType(ast.Type{
					TypeKind: ast.TypeKindNamed,
					Name:     name.Literal,
				})
				if !list.HasNext() {
					list.SetFirst(ref)
				}
				if previous != -1 {
					p.document.Types[previous].SetNext(ref)
				}
				previous = ref
			} else {
				p.errUnexpectedToken(p.read())
				return
			}
		default:
			if acceptIdent {
				p.errUnexpectedToken(p.read())
			}
			return
		}
	}
}

func (p *Parser) parseFieldDefinitionList() (list ast.FieldDefinitionList) {

	p.mustRead(keyword.CURLYBRACKETOPEN)

	previous := -1

	for {

		next := p.peek(true)

		switch next {
		case keyword.CURLYBRACKETCLOSE:
			p.read()
			return
		case keyword.STRING, keyword.BLOCKSTRING, keyword.IDENT, keyword.TYPE:
			ref := p.parseFieldDefinition()
			if !list.HasNext() {
				list.SetFirst(ref)
			}
			if previous != -1 {
				p.document.FieldDefinitions[previous].SetNext(ref)
			}
			previous = ref
		default:
			p.errUnexpectedToken(p.read())
			return
		}
	}
}

func (p *Parser) parseFieldDefinition() int {

	var fieldDefinition ast.FieldDefinition

	name := p.peek(true)
	switch name {
	case keyword.STRING, keyword.BLOCKSTRING:
		fieldDefinition.Description = p.parseDescription()
	case keyword.IDENT, keyword.TYPE:
		break
	default:
		p.errUnexpectedToken(p.read())
		return -1
	}

	fieldDefinition.Name = p.mustRead(keyword.IDENT, keyword.TYPE).Literal
	if p.peek(true) == keyword.BRACKETOPEN {
		fieldDefinition.ArgumentsDefinition = p.parseInputValueDefinitionList(keyword.BRACKETCLOSE)
	}
	fieldDefinition.Colon = p.mustRead(keyword.COLON).TextPosition
	fieldDefinition.Type = p.parseType()
	if p.peek(true) == keyword.DIRECTIVE {
		fieldDefinition.Directives = p.parseDirectiveList()
	}

	return p.document.PutFieldDefinition(fieldDefinition)
}

func (p *Parser) parseNamedType() (ref int) {
	ident := p.mustRead(keyword.IDENT)
	return p.document.PutType(ast.Type{
		TypeKind: ast.TypeKindNamed,
		Name:     ident.Literal,
	})
}

func (p *Parser) parseType() (ref int) {

	first := p.peek(true)

	if first == keyword.IDENT {

		named := p.read()
		ref = p.document.PutType(ast.Type{
			TypeKind: ast.TypeKindNamed,
			Name:     named.Literal,
		})

	} else if first == keyword.SQUAREBRACKETOPEN {

		openList := p.read()
		ofType := p.parseType()
		closeList := p.mustRead(keyword.SQUAREBRACKETCLOSE)

		ref = p.document.PutType(ast.Type{
			TypeKind: ast.TypeKindList,
			Open:     openList.TextPosition,
			Close:    closeList.TextPosition,
			OfType:   ofType,
		})

	} else {
		p.errUnexpectedToken(p.read())
		return
	}

	next := p.peek(true)
	if next == keyword.BANG {
		nonNull := ast.Type{
			TypeKind: ast.TypeKindNonNull,
			Bang:     p.read().TextPosition,
			OfType:   ref,
		}

		if p.peek(true) == keyword.BANG {
			p.errUnexpectedToken(p.read())
			return
		}

		return p.document.PutType(nonNull)
	}

	return
}

func (p *Parser) parseDescription() ast.Description {
	tok := p.read()
	return ast.Description{
		IsDefined:     true,
		Content:       tok.Literal,
		Position:      tok.TextPosition,
		IsBlockString: tok.Keyword == keyword.BLOCKSTRING,
	}
}

func (p *Parser) parseInputValueDefinitionList(closingKeyword keyword.Keyword) (list ast.InputValueDefinitionList) {

	list.Open = p.read().TextPosition

	previous := -1

	for {
		next := p.peek(true)
		switch next {
		case keyword.STRING, keyword.BLOCKSTRING, keyword.IDENT:
			ref := p.parseInputValueDefinition()
			if !list.HasNext() {
				list.SetFirst(ref)
			}
			if previous != -1 {
				p.document.InputValueDefinitions[previous].SetNext(ref)
			}
			previous = ref
		case closingKeyword:
			list.Close = p.read().TextPosition
			return
		default:
			p.errUnexpectedToken(p.read())
			return
		}
	}
}

func (p *Parser) parseInputValueDefinition() int {

	var inputValueDefinition ast.InputValueDefinition

	name := p.peek(true)
	switch name {
	case keyword.STRING, keyword.BLOCKSTRING:
		inputValueDefinition.Description = p.parseDescription()
	case keyword.IDENT:
		break
	default:
		p.errUnexpectedToken(p.read())
		return -1
	}

	inputValueDefinition.Name = p.mustRead(keyword.IDENT).Literal
	inputValueDefinition.Colon = p.mustRead(keyword.COLON).TextPosition
	inputValueDefinition.Type = p.parseType()
	if p.peek(true) == keyword.EQUALS {
		equals := p.read()
		inputValueDefinition.DefaultValue.IsDefined = true
		inputValueDefinition.DefaultValue.Equals = equals.TextPosition
		inputValueDefinition.DefaultValue.Value = p.parseValue()
	}
	if p.peek(true) == keyword.AT {
		inputValueDefinition.Directives = p.parseDirectiveList()
	}

	return p.document.PutInputValueDefinition(inputValueDefinition)
}

func (p *Parser) parseInputObjectTypeDefinition(description *ast.Description) int {
	var inputObjectTypeDefinition ast.InputObjectTypeDefinition
	if description != nil {
		inputObjectTypeDefinition.Description = *description
	}
	inputObjectTypeDefinition.InputLiteral = p.mustRead(keyword.INPUT).TextPosition
	inputObjectTypeDefinition.Name = p.mustRead(keyword.IDENT).Literal
	if p.peek(true) == keyword.AT {
		inputObjectTypeDefinition.Directives = p.parseDirectiveList()
	}
	if p.peek(true) == keyword.CURLYBRACKETOPEN {
		inputObjectTypeDefinition.InputFieldsDefinition = p.parseInputValueDefinitionList(keyword.CURLYBRACKETCLOSE)
	}
	return p.document.PutInputObjectTypeDefinition(inputObjectTypeDefinition)
}

func (p *Parser) parseScalarTypeDefinition(description *ast.Description) int {
	var scalarTypeDefinition ast.ScalarTypeDefinition
	if description != nil {
		scalarTypeDefinition.Description = *description
	}
	scalarTypeDefinition.ScalarLiteral = p.mustRead(keyword.SCALAR).TextPosition
	scalarTypeDefinition.Name = p.mustRead(keyword.IDENT).Literal
	if p.peek(true) == keyword.AT {
		scalarTypeDefinition.Directives = p.parseDirectiveList()
	}
	return p.document.PutScalarTypeDefinition(scalarTypeDefinition)
}

func (p *Parser) parseInterfaceTypeDefinition(description *ast.Description) int {
	var interfaceTypeDefinition ast.InterfaceTypeDefinition
	if description != nil {
		interfaceTypeDefinition.Description = *description
	}
	interfaceTypeDefinition.InterfaceLiteral = p.mustRead(keyword.INTERFACE).TextPosition
	interfaceTypeDefinition.Name = p.mustRead(keyword.IDENT).Literal
	if p.peek(true) == keyword.AT {
		interfaceTypeDefinition.Directives = p.parseDirectiveList()
	}
	if p.peek(true) == keyword.CURLYBRACKETOPEN {
		interfaceTypeDefinition.FieldsDefinition = p.parseFieldDefinitionList()
	}
	return p.document.PutInterfaceTypeDefinition(interfaceTypeDefinition)
}

func (p *Parser) parseUnionTypeDefinition(description *ast.Description) int {
	var unionTypeDefinition ast.UnionTypeDefinition
	if description != nil {
		unionTypeDefinition.Description = *description
	}
	unionTypeDefinition.UnionLiteral = p.mustRead(keyword.UNION).TextPosition
	unionTypeDefinition.Name = p.mustRead(keyword.IDENT).Literal
	if p.peek(true) == keyword.AT {
		unionTypeDefinition.Directives = p.parseDirectiveList()
	}
	if p.peek(true) == keyword.EQUALS {
		unionTypeDefinition.Equals, unionTypeDefinition.UnionMemberTypes = p.parseUnionMemberTypes()
	}
	return p.document.PutUnionTypeDefinition(unionTypeDefinition)
}

func (p *Parser) parseUnionMemberTypes() (equals position.Position, members ast.TypeList) {

	equals = p.mustRead(keyword.EQUALS).TextPosition

	previous := -1

	acceptPipe := true
	acceptIdent := true
	expectNext := true

	for {
		next := p.peek(true)
		switch next {
		case keyword.PIPE:
			if acceptPipe {
				acceptPipe = false
				acceptIdent = true
				expectNext = true
				p.read()
			} else {
				p.errUnexpectedToken(p.read())
				return
			}
		case keyword.IDENT:
			if acceptIdent {
				acceptPipe = true
				acceptIdent = false
				expectNext = false

				ident := p.read()

				ref := p.document.PutType(ast.Type{
					TypeKind: ast.TypeKindNamed,
					Name:     ident.Literal,
				})

				if !members.HasNext() {
					members.SetFirst(ref)
				}

				if previous != -1 {
					p.document.Types[previous].SetNext(ref)
				}

				previous = ref

			} else {
				p.errUnexpectedToken(p.read())
				return
			}
		default:
			if expectNext {
				p.errUnexpectedToken(p.read())
			}
			return
		}
	}
}

func (p *Parser) parseEnumTypeDefinition(description *ast.Description) int {
	var enumTypeDefinition ast.EnumTypeDefinition
	if description != nil {
		enumTypeDefinition.Description = *description
	}
	enumTypeDefinition.EnumLiteral = p.mustRead(keyword.ENUM).TextPosition
	enumTypeDefinition.Name = p.mustRead(keyword.IDENT).Literal
	if p.peek(true) == keyword.AT {
		enumTypeDefinition.Directives = p.parseDirectiveList()
	}
	if p.peek(true) == keyword.CURLYBRACKETOPEN {
		enumTypeDefinition.EnumValuesDefinition = p.parseEnumValueDefinitionList()
	}
	return p.document.PutEnumTypeDefinition(enumTypeDefinition)
}

func (p *Parser) parseEnumValueDefinitionList() (list ast.EnumValueDefinitionList) {

	list.Open = p.mustRead(keyword.CURLYBRACKETOPEN).TextPosition

	previous := -1

	for {
		next := p.peek(true)
		switch next {
		case keyword.STRING, keyword.BLOCKSTRING, keyword.IDENT:
			ref := p.parseEnumValueDefinition()
			if !list.HasNext() {
				list.SetFirst(ref)
			}
			if previous != -1 {
				p.document.EnumValueDefinitions[previous].SetNext(ref)
			}
			previous = ref
		case keyword.CURLYBRACKETCLOSE:
			list.Close = p.read().TextPosition
			return
		default:
			p.errUnexpectedToken(p.read())
			return
		}
	}
}

func (p *Parser) parseEnumValueDefinition() int {
	var enumValueDefinition ast.EnumValueDefinition
	next := p.peek(true)
	switch next {
	case keyword.STRING, keyword.BLOCKSTRING:
		enumValueDefinition.Description = p.parseDescription()
	case keyword.IDENT:
		break
	default:
		p.errUnexpectedToken(p.read())
		return -1
	}

	enumValueDefinition.EnumValue = p.mustRead(keyword.IDENT).Literal
	if p.peek(true) == keyword.AT {
		enumValueDefinition.Directives = p.parseDirectiveList()
	}

	return p.document.PutEnumValueDefinition(enumValueDefinition)
}

func (p *Parser) parseDirectiveDefinition(description *ast.Description) int {
	var directiveDefinition ast.DirectiveDefinition
	if description != nil {
		directiveDefinition.Description = *description
	}
	directiveDefinition.DirectiveLiteral = p.mustRead(keyword.DIRECTIVE).TextPosition
	directiveDefinition.At = p.mustRead(keyword.AT).TextPosition
	directiveDefinition.Name = p.mustRead(keyword.IDENT).Literal
	if p.peek(true) == keyword.BRACKETOPEN {
		directiveDefinition.ArgumentsDefinition = p.parseInputValueDefinitionList(keyword.BRACKETCLOSE)
	}
	directiveDefinition.On = p.mustRead(keyword.ON).TextPosition
	p.parseDirectiveLocations(&directiveDefinition.DirectiveLocations)
	return p.document.PutDirectiveDefinition(directiveDefinition)
}

func (p *Parser) parseDirectiveLocations(locations *ast.DirectiveLocations) {
	acceptPipe := true
	acceptIdent := true
	expectNext := true
	for {
		next := p.peek(true)
		switch next {
		case keyword.IDENT:
			if acceptIdent {
				acceptIdent = false
				acceptPipe = true
				expectNext = false

				raw := p.input.ByteSlice(p.read().Literal)
				p.err = locations.SetFromRaw(raw)
				if p.err != nil {
					return
				}

			} else {
				p.errUnexpectedToken(p.read())
				return
			}
		case keyword.PIPE:
			if acceptPipe {
				acceptPipe = false
				acceptIdent = true
				expectNext = true
				p.read()
			} else {
				p.errUnexpectedToken(p.read())
				return
			}
		default:
			if expectNext {
				p.errUnexpectedToken(p.read())
			}
			return
		}
	}
}

func (p *Parser) parseSelectionSet() (set ast.SelectionSet) {

	set.Open = p.mustRead(keyword.CURLYBRACKETOPEN).TextPosition

	previous := -1
	for {
		next := p.peek(true)
		switch next {
		case keyword.CURLYBRACKETCLOSE:
			set.Close = p.read().TextPosition
			return
		default:
			ref := p.parseSelection()
			if !set.HasNext() {
				set.SetFirst(ref)
			}
			if previous != -1 {
				p.document.Selections[previous].SetNext(ref)
			}
			previous = ref
		}
	}
}

func (p *Parser) parseSelection() int {
	next := p.peek(true)
	switch next {
	case keyword.IDENT:
		field := p.parseField()
		return p.document.PutSelection(ast.Selection{
			Kind: ast.SelectionKindField,
			Ref:  field,
		})
	default:
		p.errUnexpectedToken(p.read(), keyword.IDENT)
		return -1
	}
}

func (p *Parser) parseField() int {

	var field ast.Field

	firstIdent := p.mustRead(keyword.IDENT)
	if p.peek(true) == keyword.COLON {
		field.Alias.IsDefined = true
		field.Alias.Name = firstIdent.Literal
		field.Alias.Colon = p.read().TextPosition
		field.Name = p.mustRead(keyword.IDENT).Literal
	} else {
		field.Name = firstIdent.Literal
	}

	if p.peek(true) == keyword.BRACKETOPEN {
		field.Arguments = p.parseArgumentList()
	}
	if p.peek(true) == keyword.AT {
		field.Directives = p.parseDirectiveList()
	}
	if p.peek(true) == keyword.CURLYBRACKETOPEN {
		field.SelectionSet = p.parseSelectionSet()
	}

	return p.document.PutField(field)
}
