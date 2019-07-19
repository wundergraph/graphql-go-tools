package astparser

import (
	"fmt"
	"github.com/jensneuse/graphql-go-tools/pkg/ast"
	"github.com/jensneuse/graphql-go-tools/pkg/input"
	"github.com/jensneuse/graphql-go-tools/pkg/lexer"
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/keyword"
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/position"
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/token"
	"runtime"
)

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
	input        *input.Input
	document     *ast.Document
	err          error
	lexer        *lexer.Lexer
	tokens       []token.Token
	maxTokens    int
	currentToken int
}

func NewParser() *Parser {
	return &Parser{
		tokens: make([]token.Token, 256),
		lexer:  &lexer.Lexer{},
	}
}

func (p *Parser) Parse(input *input.Input, document *ast.Document) error {
	p.input = input
	p.document = document
	p.lexer.SetInput(input)
	p.tokenize()
	p.parse()
	return p.err
}

func (p *Parser) tokenize() {

	p.tokens = p.tokens[:0]

	for {
		next := p.lexer.Read()
		if next.Keyword == keyword.EOF {
			p.maxTokens = len(p.tokens)
			p.currentToken = -1
			return
		}
		p.tokens = append(p.tokens, next)
	}
}

func (p *Parser) parse() {

	for {
		next := p.peek()

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
		case keyword.QUERY, keyword.MUTATION, keyword.SUBSCRIPTION, keyword.CURLYBRACKETOPEN:
			p.parseOperationDefinition()
		case keyword.FRAGMENT:
			p.parseFragmentDefinition()
		case keyword.EXTEND:
			p.parseExtension()
		case keyword.COMMENT:
			p.read()
			continue
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
	p.currentToken++
	if p.currentToken < p.maxTokens {
		return p.tokens[p.currentToken]
	}

	return token.Token{
		Keyword: keyword.EOF,
	}
}

func (p *Parser) peek() keyword.Keyword {
	nextIndex := p.currentToken + 1
	if nextIndex < p.maxTokens {
		return p.tokens[nextIndex].Keyword
	}
	return keyword.EOF
}

func (p *Parser) peekEquals(key keyword.Keyword) bool {
	return p.peek() == key
}

func (p *Parser) errUnexpectedToken(unexpected token.Token, expectedKeywords ...keyword.Keyword) {

	if p.err != nil {
		return
	}

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
		expected: expectedKeywords,
	}
}

func (p *Parser) mustRead(key keyword.Keyword) (next token.Token) {
	next = p.read()
	if next.Keyword != key {
		p.errUnexpectedToken(next, key)
	}
	return
}

func (p *Parser) parseSchema() {

	schemaLiteral := p.read()

	schemaDefinition := ast.SchemaDefinition{
		SchemaLiteral: schemaLiteral.TextPosition,
	}

	if p.peekEquals(keyword.AT) {
		schemaDefinition.Directives = p.parseDirectiveList()
	}

	p.parseRootOperationTypeDefinitionList(&schemaDefinition.RootOperationTypeDefinitions)

	p.document.SchemaDefinitions = append(p.document.SchemaDefinitions, schemaDefinition)
	ref := len(p.document.SchemaDefinitions) - 1
	rootNode := ast.RootNode{
		Kind: ast.NodeKindSchemaDefinition,
		Ref:  ref,
	}
	p.document.RootNodes = append(p.document.RootNodes, rootNode)
}

func (p *Parser) parseRootOperationTypeDefinitionList(list *ast.RootOperationTypeDefinitionList) {

	curlyBracketOpen := p.mustRead(keyword.CURLYBRACKETOPEN)

	for {
		next := p.peek()
		switch next {
		case keyword.CURLYBRACKETCLOSE:

			curlyBracketClose := p.read()
			list.LBrace = curlyBracketOpen.TextPosition
			list.RBrace = curlyBracketClose.TextPosition
			return
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

			p.document.RootOperationTypeDefinitions = append(p.document.RootOperationTypeDefinitions, rootOperationTypeDefinition)
			ref := len(p.document.RootOperationTypeDefinitions) - 1

			if cap(list.Refs) == 0 {
				list.Refs = p.document.Refs[p.document.NextRefIndex()][:0]
				list.Refs = p.document.Refs[p.document.NextRefIndex()][:0]
			}

			list.Refs = append(list.Refs, ref)

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
		return ast.OperationTypeUnknown
	}
}

func (p *Parser) parseDirectiveList() (list ast.DirectiveList) {

	for {

		if p.peek() != keyword.AT {
			break
		}

		at := p.read()
		name := p.mustRead(keyword.IDENT)

		directive := ast.Directive{
			At:   at.TextPosition,
			Name: name.Literal,
		}

		if p.peekEquals(keyword.BRACKETOPEN) {
			directive.Arguments = p.parseArgumentList()
		}

		p.document.Directives = append(p.document.Directives, directive)
		ref := len(p.document.Directives) - 1

		if cap(list.Refs) == 0 {
			list.Refs = p.document.Refs[p.document.NextRefIndex()][:0]
		}

		list.Refs = append(list.Refs, ref)
	}

	return
}

func (p *Parser) parseArgumentList() (list ast.ArgumentList) {

	bracketOpen := p.read()

Loop:
	for {

		next := p.peek()
		switch next {
		case keyword.IDENT, keyword.INPUT:
		default:
			break Loop
		}

		name := p.read()
		colon := p.mustRead(keyword.COLON)
		value := p.parseValue()

		argument := ast.Argument{
			Name:  name.Literal,
			Colon: colon.TextPosition,
			Value: value,
		}

		p.document.Arguments = append(p.document.Arguments, argument)
		ref := len(p.document.Arguments) - 1

		if cap(list.Refs) == 0 {
			list.Refs = p.document.Refs[p.document.NextRefIndex()][:0]
		}

		list.Refs = append(list.Refs, ref)
	}

	bracketClose := p.mustRead(keyword.BRACKETCLOSE)

	list.LPAREN = bracketOpen.TextPosition
	list.RPAREN = bracketClose.TextPosition

	return
}

func (p *Parser) parseValue() (value ast.Value) {

	next := p.peek()

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
		value = p.parseNegativeNumberValue()
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
	objectValue.LBRACE = p.mustRead(keyword.CURLYBRACKETOPEN).TextPosition

	for {
		next := p.peek()
		switch next {
		case keyword.CURLYBRACKETCLOSE:
			objectValue.RBRACE = p.read().TextPosition
			p.document.ObjectValues = append(p.document.ObjectValues, objectValue)
			return len(p.document.ObjectValues) - 1
		case keyword.IDENT:
			ref := p.parseObjectField()
			if cap(objectValue.Refs) == 0 {
				objectValue.Refs = p.document.Refs[p.document.NextRefIndex()][:0]
			}
			objectValue.Refs = append(objectValue.Refs, ref)
		default:
			p.errUnexpectedToken(p.read(), keyword.IDENT, keyword.CURLYBRACKETCLOSE)
			return -1
		}
	}
}

func (p *Parser) parseObjectField() int {
	objectField := ast.ObjectField{
		Name:  p.mustRead(keyword.IDENT).Literal,
		Colon: p.mustRead(keyword.COLON).TextPosition,
		Value: p.parseValue(),
	}
	p.document.ObjectFields = append(p.document.ObjectFields, objectField)
	return len(p.document.ObjectFields) - 1
}

func (p *Parser) parseValueList() int {
	var list ast.ListValue
	list.LBRACK = p.mustRead(keyword.SQUAREBRACKETOPEN).TextPosition

	for {
		next := p.peek()
		switch next {
		case keyword.SQUAREBRACKETCLOSE:
			list.RBRACK = p.read().TextPosition
			p.document.ListValues = append(p.document.ListValues, list)
			return len(p.document.ListValues) - 1
		default:
			value := p.parseValue()
			p.document.Values = append(p.document.Values, value)
			ref := len(p.document.Values) - 1
			if cap(list.Refs) == 0 {
				list.Refs = p.document.Refs[p.document.NextRefIndex()][:0]
			}
			list.Refs = append(list.Refs, ref)
		}
	}
}

func (p *Parser) parseNegativeNumberValue() (value ast.Value) {
	negativeSign := p.mustRead(keyword.NEGATIVESIGN).TextPosition
	switch p.peek() {
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

	value := p.mustRead(keyword.FLOAT)

	if negativeSign != nil && negativeSign.CharEnd != value.TextPosition.CharStart {
		p.errUnexpectedToken(value)
		return -1
	}

	floatValue := ast.FloatValue{
		Raw: value.Literal,
	}
	if negativeSign != nil {
		floatValue.Negative = true
		floatValue.NegativeSign = *negativeSign
	}

	p.document.FloatValues = append(p.document.FloatValues, floatValue)
	return len(p.document.FloatValues) - 1
}

func (p *Parser) parseIntegerValue(negativeSign *position.Position) int {

	value := p.mustRead(keyword.INTEGER)

	if negativeSign != nil && negativeSign.CharEnd != value.TextPosition.CharStart {
		p.errUnexpectedToken(value)
		return -1
	}

	intValue := ast.IntValue{
		Raw: value.Literal,
	}
	if negativeSign != nil {
		intValue.Negative = true
		intValue.NegativeSign = *negativeSign
	}

	p.document.IntValues = append(p.document.IntValues, intValue)
	return len(p.document.IntValues) - 1
}

func (p *Parser) parseVariableValue() int {
	dollar := p.mustRead(keyword.DOLLAR)
	var value token.Token

	next := p.peek()
	switch next {
	case keyword.IDENT, keyword.INPUT:
		value = p.read()
	default:
		p.errUnexpectedToken(p.read(), keyword.IDENT, keyword.INPUT)
		return -1
	}

	if dollar.TextPosition.CharEnd != value.TextPosition.CharStart {
		p.errUnexpectedToken(p.read(), keyword.IDENT, keyword.INPUT)
		return -1
	}

	variable := ast.VariableValue{
		Dollar: dollar.TextPosition,
		Name:   value.Literal,
	}

	p.document.VariableValues = append(p.document.VariableValues, variable)
	return len(p.document.VariableValues) - 1
}

func (p *Parser) parseBooleanValue() int {
	value := p.read()
	switch value.Keyword {
	case keyword.FALSE:
		return 0
	case keyword.TRUE:
		return 1
	default:
		p.errUnexpectedToken(value, keyword.FALSE, keyword.TRUE)
		return -1
	}
}

func (p *Parser) parseEnumValue() int {
	enum := ast.EnumValue{
		Name: p.mustRead(keyword.IDENT).Literal,
	}
	p.document.EnumValues = append(p.document.EnumValues, enum)
	return len(p.document.EnumValues) - 1
}

func (p *Parser) parseStringValue() int {
	value := p.read()
	if value.Keyword != keyword.STRING && value.Keyword != keyword.BLOCKSTRING {
		p.errUnexpectedToken(value, keyword.STRING, keyword.BLOCKSTRING)
		return -1
	}
	stringValue := ast.StringValue{
		Content:     value.Literal,
		BlockString: value.Keyword == keyword.BLOCKSTRING,
	}
	p.document.StringValues = append(p.document.StringValues, stringValue)
	return len(p.document.StringValues) - 1
}

func (p *Parser) parseObjectTypeDefinition(description *ast.Description) {

	var objectTypeDefinition ast.ObjectTypeDefinition
	if description != nil {
		objectTypeDefinition.Description = *description
	}
	objectTypeDefinition.TypeLiteral = p.mustRead(keyword.TYPE).TextPosition
	objectTypeDefinition.Name = p.mustRead(keyword.IDENT).Literal
	if p.peekEquals(keyword.IMPLEMENTS) {
		objectTypeDefinition.ImplementsInterfaces = p.parseImplementsInterfaces()
	}
	if p.peekEquals(keyword.AT) {
		objectTypeDefinition.Directives = p.parseDirectiveList()
	}
	if p.peekEquals(keyword.CURLYBRACKETOPEN) {
		objectTypeDefinition.FieldsDefinition = p.parseFieldDefinitionList()
	}

	p.document.ObjectTypeDefinitions = append(p.document.ObjectTypeDefinitions, objectTypeDefinition)
}

func (p *Parser) parseRootDescription() {

	description := p.parseDescription()

	next := p.peek()
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

	p.read() // implements

	acceptIdent := true
	acceptAnd := true

	for {
		next := p.peek()
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
				astType := ast.Type{
					TypeKind: ast.TypeKindNamed,
					Name:     name.Literal,
				}
				p.document.Types = append(p.document.Types, astType)
				ref := len(p.document.Types) - 1
				if cap(list.Refs) == 0 {
					list.Refs = p.document.Refs[p.document.NextRefIndex()][:0]
				}
				list.Refs = append(list.Refs, ref)
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

	list.LBRACE = p.mustRead(keyword.CURLYBRACKETOPEN).TextPosition

	for {

		next := p.peek()

		switch next {
		case keyword.CURLYBRACKETCLOSE:
			list.RBRACE = p.read().TextPosition
			return
		case keyword.STRING, keyword.BLOCKSTRING, keyword.IDENT, keyword.TYPE:
			ref := p.parseFieldDefinition()
			if cap(list.Refs) == 0 {
				list.Refs = p.document.Refs[p.document.NextRefIndex()][:0]
			}
			list.Refs = append(list.Refs, ref)
		default:
			p.errUnexpectedToken(p.read())
			return
		}
	}
}

func (p *Parser) parseFieldDefinition() int {

	var fieldDefinition ast.FieldDefinition

	name := p.peek()
	switch name {
	case keyword.STRING, keyword.BLOCKSTRING:
		fieldDefinition.Description = p.parseDescription()
	case keyword.IDENT, keyword.TYPE:
		break
	default:
		p.errUnexpectedToken(p.read())
		return -1
	}

	nameToken := p.read()
	if nameToken.Keyword != keyword.IDENT && nameToken.Keyword != keyword.TYPE {
		p.errUnexpectedToken(nameToken, keyword.IDENT, keyword.TYPE)
		return -1
	}

	fieldDefinition.Name = nameToken.Literal
	if p.peekEquals(keyword.BRACKETOPEN) {
		fieldDefinition.ArgumentsDefinition = p.parseInputValueDefinitionList(keyword.BRACKETCLOSE)
	}
	fieldDefinition.Colon = p.mustRead(keyword.COLON).TextPosition
	fieldDefinition.Type = p.parseType()
	if p.peek() == keyword.DIRECTIVE {
		fieldDefinition.Directives = p.parseDirectiveList()
	}

	p.document.FieldDefinitions = append(p.document.FieldDefinitions, fieldDefinition)
	return len(p.document.FieldDefinitions) - 1
}

func (p *Parser) parseNamedType() (ref int) {
	ident := p.mustRead(keyword.IDENT)
	namedType := ast.Type{
		TypeKind: ast.TypeKindNamed,
		Name:     ident.Literal,
	}
	p.document.Types = append(p.document.Types, namedType)
	return len(p.document.Types) - 1
}

func (p *Parser) parseType() (ref int) {

	first := p.peek()

	if first == keyword.IDENT {

		namedType := ast.Type{
			TypeKind: ast.TypeKindNamed,
			Name:     p.read().Literal,
		}

		p.document.Types = append(p.document.Types, namedType)
		ref = len(p.document.Types) - 1

	} else if first == keyword.SQUAREBRACKETOPEN {

		openList := p.read()
		ofType := p.parseType()
		closeList := p.mustRead(keyword.SQUAREBRACKETCLOSE)

		listType := ast.Type{
			TypeKind: ast.TypeKindList,
			Open:     openList.TextPosition,
			Close:    closeList.TextPosition,
			OfType:   ofType,
		}

		p.document.Types = append(p.document.Types, listType)
		ref = len(p.document.Types) - 1

	} else {
		p.errUnexpectedToken(p.read(), keyword.IDENT, keyword.SQUAREBRACKETOPEN)
		return
	}

	next := p.peek()
	if next == keyword.BANG {
		nonNull := ast.Type{
			TypeKind: ast.TypeKindNonNull,
			Bang:     p.read().TextPosition,
			OfType:   ref,
		}

		if p.peek() == keyword.BANG {
			p.errUnexpectedToken(p.read())
			return
		}

		p.document.Types = append(p.document.Types, nonNull)
		ref = len(p.document.Types) - 1
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

	list.LPAREN = p.read().TextPosition

	for {
		next := p.peek()
		switch next {
		case closingKeyword:
			list.RPAREN = p.read().TextPosition
			return
		case keyword.STRING, keyword.BLOCKSTRING, keyword.IDENT:
			ref := p.parseInputValueDefinition()
			if cap(list.Refs) == 0 {
				list.Refs = p.document.Refs[p.document.NextRefIndex()][:0]
			}
			list.Refs = append(list.Refs, ref)
		default:
			p.errUnexpectedToken(p.read())
			return
		}
	}
}

func (p *Parser) parseInputValueDefinition() int {

	var inputValueDefinition ast.InputValueDefinition

	name := p.peek()
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
	if p.peekEquals(keyword.EQUALS) {
		equals := p.read()
		inputValueDefinition.DefaultValue.IsDefined = true
		inputValueDefinition.DefaultValue.Equals = equals.TextPosition
		inputValueDefinition.DefaultValue.Value = p.parseValue()
	}
	if p.peekEquals(keyword.AT) {
		inputValueDefinition.Directives = p.parseDirectiveList()
	}

	p.document.InputValueDefinitions = append(p.document.InputValueDefinitions, inputValueDefinition)
	return len(p.document.InputValueDefinitions) - 1
}

func (p *Parser) parseInputObjectTypeDefinition(description *ast.Description) {
	var inputObjectTypeDefinition ast.InputObjectTypeDefinition
	if description != nil {
		inputObjectTypeDefinition.Description = *description
	}
	inputObjectTypeDefinition.InputLiteral = p.mustRead(keyword.INPUT).TextPosition
	inputObjectTypeDefinition.Name = p.mustRead(keyword.IDENT).Literal
	if p.peekEquals(keyword.AT) {
		inputObjectTypeDefinition.Directives = p.parseDirectiveList()
	}
	if p.peekEquals(keyword.CURLYBRACKETOPEN) {
		inputObjectTypeDefinition.InputFieldsDefinition = p.parseInputValueDefinitionList(keyword.CURLYBRACKETCLOSE)
	}
	p.document.InputObjectTypeDefinitions = append(p.document.InputObjectTypeDefinitions, inputObjectTypeDefinition)
}

func (p *Parser) parseScalarTypeDefinition(description *ast.Description) {
	var scalarTypeDefinition ast.ScalarTypeDefinition
	if description != nil {
		scalarTypeDefinition.Description = *description
	}
	scalarTypeDefinition.ScalarLiteral = p.mustRead(keyword.SCALAR).TextPosition
	scalarTypeDefinition.Name = p.mustRead(keyword.IDENT).Literal
	if p.peekEquals(keyword.AT) {
		scalarTypeDefinition.Directives = p.parseDirectiveList()
	}
	p.document.ScalarTypeDefinitions = append(p.document.ScalarTypeDefinitions, scalarTypeDefinition)
}

func (p *Parser) parseInterfaceTypeDefinition(description *ast.Description) {
	var interfaceTypeDefinition ast.InterfaceTypeDefinition
	if description != nil {
		interfaceTypeDefinition.Description = *description
	}
	interfaceTypeDefinition.InterfaceLiteral = p.mustRead(keyword.INTERFACE).TextPosition
	interfaceTypeDefinition.Name = p.mustRead(keyword.IDENT).Literal
	if p.peekEquals(keyword.AT) {
		interfaceTypeDefinition.Directives = p.parseDirectiveList()
	}
	if p.peekEquals(keyword.CURLYBRACKETOPEN) {
		interfaceTypeDefinition.FieldsDefinition = p.parseFieldDefinitionList()
	}
	p.document.InterfaceTypeDefinitions = append(p.document.InterfaceTypeDefinitions, interfaceTypeDefinition)
}

func (p *Parser) parseUnionTypeDefinition(description *ast.Description) {
	var unionTypeDefinition ast.UnionTypeDefinition
	if description != nil {
		unionTypeDefinition.Description = *description
	}
	unionTypeDefinition.UnionLiteral = p.mustRead(keyword.UNION).TextPosition
	unionTypeDefinition.Name = p.mustRead(keyword.IDENT).Literal
	if p.peekEquals(keyword.AT) {
		unionTypeDefinition.Directives = p.parseDirectiveList()
	}
	if p.peekEquals(keyword.EQUALS) {
		unionTypeDefinition.Equals = p.mustRead(keyword.EQUALS).TextPosition
		unionTypeDefinition.UnionMemberTypes = p.parseUnionMemberTypes()
	}
	p.document.UnionTypeDefinitions = append(p.document.UnionTypeDefinitions, unionTypeDefinition)
}

func (p *Parser) parseUnionMemberTypes() (list ast.TypeList) {

	acceptPipe := true
	acceptIdent := true
	expectNext := true

	for {
		next := p.peek()
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

				namedType := ast.Type{
					TypeKind: ast.TypeKindNamed,
					Name:     ident.Literal,
				}

				p.document.Types = append(p.document.Types, namedType)
				ref := len(p.document.Types) - 1
				if cap(list.Refs) == 0 {
					list.Refs = p.document.Refs[p.document.NextRefIndex()][:0]
				}
				list.Refs = append(list.Refs, ref)
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

func (p *Parser) parseEnumTypeDefinition(description *ast.Description) {
	var enumTypeDefinition ast.EnumTypeDefinition
	if description != nil {
		enumTypeDefinition.Description = *description
	}
	enumTypeDefinition.EnumLiteral = p.mustRead(keyword.ENUM).TextPosition
	enumTypeDefinition.Name = p.mustRead(keyword.IDENT).Literal
	if p.peekEquals(keyword.AT) {
		enumTypeDefinition.Directives = p.parseDirectiveList()
	}
	if p.peekEquals(keyword.CURLYBRACKETOPEN) {
		enumTypeDefinition.EnumValuesDefinition = p.parseEnumValueDefinitionList()
	}
	p.document.EnumTypeDefinitions = append(p.document.EnumTypeDefinitions, enumTypeDefinition)
}

func (p *Parser) parseEnumValueDefinitionList() (list ast.EnumValueDefinitionList) {

	list.LBRACE = p.mustRead(keyword.CURLYBRACKETOPEN).TextPosition

	for {
		next := p.peek()
		switch next {
		case keyword.STRING, keyword.BLOCKSTRING, keyword.IDENT:
			ref := p.parseEnumValueDefinition()
			if cap(list.Refs) == 0 {
				list.Refs = p.document.Refs[p.document.NextRefIndex()][:0]
			}
			list.Refs = append(list.Refs, ref)
		case keyword.CURLYBRACKETCLOSE:
			list.RBRACE = p.read().TextPosition
			return
		default:
			p.errUnexpectedToken(p.read())
			return
		}
	}
}

func (p *Parser) parseEnumValueDefinition() int {
	var enumValueDefinition ast.EnumValueDefinition
	next := p.peek()
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
	if p.peekEquals(keyword.AT) {
		enumValueDefinition.Directives = p.parseDirectiveList()
	}

	p.document.EnumValueDefinitions = append(p.document.EnumValueDefinitions, enumValueDefinition)
	return len(p.document.EnumValueDefinitions) - 1
}

func (p *Parser) parseDirectiveDefinition(description *ast.Description) {
	var directiveDefinition ast.DirectiveDefinition
	if description != nil {
		directiveDefinition.Description = *description
	}
	directiveDefinition.DirectiveLiteral = p.mustRead(keyword.DIRECTIVE).TextPosition
	directiveDefinition.At = p.mustRead(keyword.AT).TextPosition
	directiveDefinition.Name = p.mustRead(keyword.IDENT).Literal
	if p.peekEquals(keyword.BRACKETOPEN) {
		directiveDefinition.ArgumentsDefinition = p.parseInputValueDefinitionList(keyword.BRACKETCLOSE)
	}
	directiveDefinition.On = p.mustRead(keyword.ON).TextPosition
	p.parseDirectiveLocations(&directiveDefinition.DirectiveLocations)
	p.document.DirectiveDefinitions = append(p.document.DirectiveDefinitions, directiveDefinition)
}

func (p *Parser) parseDirectiveLocations(locations *ast.DirectiveLocations) {
	acceptPipe := true
	acceptIdent := true
	expectNext := true
	for {
		next := p.peek()
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

	set.SelectionRefs = p.document.Refs[p.document.NextRefIndex()][:0]
	set.LBrace = p.mustRead(keyword.CURLYBRACKETOPEN).TextPosition

	for {
		switch p.peek() {
		case keyword.CURLYBRACKETCLOSE:
			set.RBrace = p.read().TextPosition
			return
		default:
			if cap(set.SelectionRefs) == 0 {
				set.SelectionRefs = p.document.Refs[p.document.NextRefIndex()][:0]
			}
			set.SelectionRefs = append(set.SelectionRefs, p.parseSelection())
		}
	}
}

func (p *Parser) parseSelection() int {
	next := p.peek()
	switch next {
	case keyword.IDENT, keyword.QUERY, keyword.TYPE:
		p.document.Selections = append(p.document.Selections, ast.Selection{
			Kind: ast.SelectionKindField,
			Ref:  p.parseField(),
		})
		return len(p.document.Selections) - 1
	case keyword.SPREAD:
		return p.parseFragmentSelection(p.read().TextPosition)
	default:
		p.errUnexpectedToken(p.read(), keyword.IDENT, keyword.SPREAD)
		return -1
	}
}

func (p *Parser) parseFragmentSelection(spread position.Position) int {

	var selection ast.Selection

	next := p.peek()
	switch next {
	case keyword.ON, keyword.CURLYBRACKETOPEN, keyword.AT:
		selection.Kind = ast.SelectionKindInlineFragment
		selection.Ref = p.parseInlineFragment(spread)
	case keyword.IDENT:
		selection.Kind = ast.SelectionKindFragmentSpread
		selection.Ref = p.parseFragmentSpread(spread)
	default:
		p.errUnexpectedToken(p.read(), keyword.ON, keyword.IDENT)
	}

	p.document.Selections = append(p.document.Selections, selection)
	return len(p.document.Selections) - 1
}

func (p *Parser) parseField() int {

	var field ast.Field

	firstIdent := p.read()
	if firstIdent.Keyword != keyword.IDENT && firstIdent.Keyword != keyword.QUERY && firstIdent.Keyword != keyword.TYPE {
		p.errUnexpectedToken(firstIdent, keyword.IDENT, keyword.QUERY)
	}

	if p.peek() == keyword.COLON {
		field.Alias.IsDefined = true
		field.Alias.Name = firstIdent.Literal
		field.Alias.Colon = p.read().TextPosition
		field.Name = p.mustRead(keyword.IDENT).Literal
	} else {
		field.Name = firstIdent.Literal
	}

	if p.peekEquals(keyword.BRACKETOPEN) {
		field.Arguments = p.parseArgumentList()
	}
	if p.peekEquals(keyword.AT) {
		field.Directives = p.parseDirectiveList()
	}
	if p.peekEquals(keyword.CURLYBRACKETOPEN) {
		field.SelectionSet = p.parseSelectionSet()
	}

	p.document.Fields = append(p.document.Fields, field)
	return len(p.document.Fields) - 1
}

func (p *Parser) parseFragmentSpread(spread position.Position) int {
	var fragmentSpread ast.FragmentSpread
	fragmentSpread.Spread = spread
	fragmentSpread.FragmentName = p.mustRead(keyword.IDENT).Literal
	if p.peekEquals(keyword.AT) {
		fragmentSpread.Directives = p.parseDirectiveList()
	}
	p.document.FragmentSpreads = append(p.document.FragmentSpreads, fragmentSpread)
	return len(p.document.FragmentSpreads) - 1
}

func (p *Parser) parseInlineFragment(spread position.Position) int {
	var fragment ast.InlineFragment
	fragment.Spread = spread
	if p.peekEquals(keyword.ON) {
		fragment.TypeCondition = p.parseTypeCondition()
	}
	if p.peekEquals(keyword.AT) {
		fragment.Directives = p.parseDirectiveList()
	}
	if p.peekEquals(keyword.CURLYBRACKETOPEN) {
		fragment.SelectionSet = p.parseSelectionSet()
	}
	p.document.InlineFragments = append(p.document.InlineFragments, fragment)
	return len(p.document.InlineFragments) - 1
}

func (p *Parser) parseTypeCondition() (typeCondition ast.TypeCondition) {
	typeCondition.On = p.mustRead(keyword.ON).TextPosition
	typeCondition.Type = p.parseNamedType()
	return
}

func (p *Parser) parseOperationDefinition() {

	var operationDefinition ast.OperationDefinition

	next := p.peek()
	switch next {
	case keyword.QUERY:
		operationDefinition.OperationTypeLiteral = p.read().TextPosition
		operationDefinition.OperationType = ast.OperationTypeQuery
	case keyword.MUTATION:
		operationDefinition.OperationTypeLiteral = p.read().TextPosition
		operationDefinition.OperationType = ast.OperationTypeMutation
	case keyword.SUBSCRIPTION:
		operationDefinition.OperationTypeLiteral = p.read().TextPosition
		operationDefinition.OperationType = ast.OperationTypeSubscription
	case keyword.CURLYBRACKETOPEN:
		operationDefinition.OperationType = ast.OperationTypeQuery
		operationDefinition.SelectionSet = p.parseSelectionSet()
		p.document.OperationDefinitions = append(p.document.OperationDefinitions, operationDefinition)
		return
	default:
		p.errUnexpectedToken(p.read(), keyword.QUERY, keyword.MUTATION, keyword.SUBSCRIPTION, keyword.CURLYBRACKETOPEN)
		return
	}

	if p.peekEquals(keyword.IDENT) {
		operationDefinition.Name = p.read().Literal
	}
	if p.peekEquals(keyword.BRACKETOPEN) {
		operationDefinition.VariableDefinitions = p.parseVariableDefinitionList()
	}
	if p.peekEquals(keyword.AT) {
		operationDefinition.Directives = p.parseDirectiveList()
	}

	operationDefinition.SelectionSet = p.parseSelectionSet()

	p.document.OperationDefinitions = append(p.document.OperationDefinitions, operationDefinition)
	ref := len(p.document.OperationDefinitions) - 1
	rootNode := ast.RootNode{
		Kind: ast.NodeKindOperationDefinition,
		Ref:  ref,
	}
	p.document.RootNodes = append(p.document.RootNodes, rootNode)
}

func (p *Parser) parseVariableDefinitionList() (list ast.VariableDefinitionList) {

	list.LPAREN = p.mustRead(keyword.BRACKETOPEN).TextPosition

	for {
		next := p.peek()
		switch next {
		case keyword.BRACKETCLOSE:
			list.RPAREN = p.read().TextPosition
			return
		case keyword.DOLLAR:
			if cap(list.Refs) == 0 {
				list.Refs = p.document.Refs[p.document.NextRefIndex()][:0]
			}
			ref := p.parseVariableDefinition()
			if cap(list.Refs) == 0 {
				list.Refs = p.document.Refs[p.document.NextRefIndex()][:0]
			}
			list.Refs = append(list.Refs, ref)
		default:
			p.errUnexpectedToken(p.read(), keyword.BRACKETCLOSE, keyword.DOLLAR)
			return
		}
	}
}

func (p *Parser) parseVariableDefinition() int {

	var variableDefinition ast.VariableDefinition

	variableDefinition.Variable = p.parseVariableValue()
	variableDefinition.Colon = p.mustRead(keyword.COLON).TextPosition
	variableDefinition.Type = p.parseType()
	if p.peekEquals(keyword.EQUALS) {
		variableDefinition.DefaultValue = p.parseDefaultValue()
	}
	if p.peekEquals(keyword.AT) {
		variableDefinition.Directives = p.parseDirectiveList()
	}
	p.document.VariableDefinitions = append(p.document.VariableDefinitions, variableDefinition)
	return len(p.document.VariableDefinitions) - 1
}

func (p *Parser) parseDefaultValue() ast.DefaultValue {
	equals := p.mustRead(keyword.EQUALS).TextPosition
	value := p.parseValue()
	return ast.DefaultValue{
		IsDefined: true,
		Equals:    equals,
		Value:     value,
	}
}

func (p *Parser) parseFragmentDefinition() {
	var fragmentDefinition ast.FragmentDefinition
	fragmentDefinition.FragmentLiteral = p.mustRead(keyword.FRAGMENT).TextPosition
	fragmentDefinition.Name = p.mustRead(keyword.IDENT).Literal
	fragmentDefinition.TypeCondition = p.parseTypeCondition()
	if p.peekEquals(keyword.AT) {
		fragmentDefinition.Directives = p.parseDirectiveList()
	}
	fragmentDefinition.SelectionSet = p.parseSelectionSet()
	p.document.FragmentDefinitions = append(p.document.FragmentDefinitions, fragmentDefinition)
}

func (p *Parser) parseExtension() {
	extend := p.mustRead(keyword.EXTEND).TextPosition
	next := p.peek()
	switch next {
	case keyword.SCHEMA:
		p.parseSchemaExtension(extend)
	case keyword.TYPE:
		p.parseObjectTypeExtension(extend)
	case keyword.INTERFACE:
		p.parseInterfaceTypeExtension(extend)
	case keyword.SCALAR:
		p.parseScalarTypeExtension(extend)
	case keyword.UNION:
		p.parseUnionTypeExtension(extend)
	case keyword.ENUM:
		p.parseEnumTypeExtension(extend)
	case keyword.INPUT:
		p.parseInputObjectTypeExtension(extend)
	default:
		p.errUnexpectedToken(p.read(), keyword.SCHEMA)
	}
}

func (p *Parser) parseSchemaExtension(extend position.Position) {

	schemaLiteral := p.read()

	schemaDefinition := ast.SchemaDefinition{
		SchemaLiteral: schemaLiteral.TextPosition,
	}

	if p.peekEquals(keyword.AT) {
		schemaDefinition.Directives = p.parseDirectiveList()
	}

	p.parseRootOperationTypeDefinitionList(&schemaDefinition.RootOperationTypeDefinitions)

	schemaExtension := ast.SchemaExtension{
		ExtendLiteral:    extend,
		SchemaDefinition: schemaDefinition,
	}

	p.document.SchemaExtensions = append(p.document.SchemaExtensions, schemaExtension)
}

func (p *Parser) parseObjectTypeExtension(extend position.Position) {

	var objectTypeDefinition ast.ObjectTypeDefinition
	objectTypeDefinition.TypeLiteral = p.mustRead(keyword.TYPE).TextPosition
	objectTypeDefinition.Name = p.mustRead(keyword.IDENT).Literal
	if p.peekEquals(keyword.IMPLEMENTS) {
		objectTypeDefinition.ImplementsInterfaces = p.parseImplementsInterfaces()
	}
	if p.peekEquals(keyword.AT) {
		objectTypeDefinition.Directives = p.parseDirectiveList()
	}
	if p.peekEquals(keyword.CURLYBRACKETOPEN) {
		objectTypeDefinition.FieldsDefinition = p.parseFieldDefinitionList()
	}

	objectTypeExtension := ast.ObjectTypeExtension{
		ExtendLiteral:        extend,
		ObjectTypeDefinition: objectTypeDefinition,
	}

	p.document.ObjectTypeExtensions = append(p.document.ObjectTypeExtensions, objectTypeExtension)
}

func (p *Parser) parseInterfaceTypeExtension(extend position.Position) {

	var interfaceTypeDefinition ast.InterfaceTypeDefinition
	interfaceTypeDefinition.InterfaceLiteral = p.mustRead(keyword.INTERFACE).TextPosition
	interfaceTypeDefinition.Name = p.mustRead(keyword.IDENT).Literal
	if p.peekEquals(keyword.AT) {
		interfaceTypeDefinition.Directives = p.parseDirectiveList()
	}
	if p.peekEquals(keyword.CURLYBRACKETOPEN) {
		interfaceTypeDefinition.FieldsDefinition = p.parseFieldDefinitionList()
	}

	interfaceTypeExtension := ast.InterfaceTypeExtension{
		ExtendLiteral:           extend,
		InterfaceTypeDefinition: interfaceTypeDefinition,
	}

	p.document.InterfaceTypeExtensions = append(p.document.InterfaceTypeExtensions, interfaceTypeExtension)
}

func (p *Parser) parseScalarTypeExtension(extend position.Position) {
	var scalarTypeDefinition ast.ScalarTypeDefinition
	scalarTypeDefinition.ScalarLiteral = p.mustRead(keyword.SCALAR).TextPosition
	scalarTypeDefinition.Name = p.mustRead(keyword.IDENT).Literal
	if p.peekEquals(keyword.AT) {
		scalarTypeDefinition.Directives = p.parseDirectiveList()
	}
	scalarTypeExtension := ast.ScalarTypeExtension{
		ExtendLiteral:        extend,
		ScalarTypeDefinition: scalarTypeDefinition,
	}
	p.document.ScalarTypeExtensions = append(p.document.ScalarTypeExtensions, scalarTypeExtension)
}

func (p *Parser) parseUnionTypeExtension(extend position.Position) {
	var unionTypeDefinition ast.UnionTypeDefinition
	unionTypeDefinition.UnionLiteral = p.mustRead(keyword.UNION).TextPosition
	unionTypeDefinition.Name = p.mustRead(keyword.IDENT).Literal
	if p.peekEquals(keyword.AT) {
		unionTypeDefinition.Directives = p.parseDirectiveList()
	}
	if p.peekEquals(keyword.EQUALS) {
		unionTypeDefinition.Equals = p.mustRead(keyword.EQUALS).TextPosition
		unionTypeDefinition.UnionMemberTypes = p.parseUnionMemberTypes()
	}
	unionTypeExtension := ast.UnionTypeExtension{
		ExtendLiteral:       extend,
		UnionTypeDefinition: unionTypeDefinition,
	}
	p.document.UnionTypeExtensions = append(p.document.UnionTypeExtensions, unionTypeExtension)
}

func (p *Parser) parseEnumTypeExtension(extend position.Position) {
	var enumTypeDefinition ast.EnumTypeDefinition
	enumTypeDefinition.EnumLiteral = p.mustRead(keyword.ENUM).TextPosition
	enumTypeDefinition.Name = p.mustRead(keyword.IDENT).Literal
	if p.peekEquals(keyword.AT) {
		enumTypeDefinition.Directives = p.parseDirectiveList()
	}
	if p.peekEquals(keyword.CURLYBRACKETOPEN) {
		enumTypeDefinition.EnumValuesDefinition = p.parseEnumValueDefinitionList()
	}
	enumTypeExtension := ast.EnumTypeExtension{
		ExtendLiteral:      extend,
		EnumTypeDefinition: enumTypeDefinition,
	}
	p.document.EnumTypeExtensions = append(p.document.EnumTypeExtensions, enumTypeExtension)
}

func (p *Parser) parseInputObjectTypeExtension(extend position.Position) {
	var inputObjectTypeDefinition ast.InputObjectTypeDefinition
	inputObjectTypeDefinition.InputLiteral = p.mustRead(keyword.INPUT).TextPosition
	inputObjectTypeDefinition.Name = p.mustRead(keyword.IDENT).Literal
	if p.peekEquals(keyword.AT) {
		inputObjectTypeDefinition.Directives = p.parseDirectiveList()
	}
	if p.peekEquals(keyword.CURLYBRACKETOPEN) {
		inputObjectTypeDefinition.InputFieldsDefinition = p.parseInputValueDefinitionList(keyword.CURLYBRACKETCLOSE)
	}
	inputObjectTypeExtension := ast.InputObjectTypeExtension{
		ExtendLiteral:             extend,
		InputObjectTypeDefinition: inputObjectTypeDefinition,
	}
	p.document.InputObjectTypeExtensions = append(p.document.InputObjectTypeExtensions, inputObjectTypeExtension)
}
