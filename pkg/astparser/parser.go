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

type ErrUnexpectedToken struct {
	keyword  keyword.Keyword
	position position.Position
	literal  string
	file     string
	line     int
	funcName string
}

func (e ErrUnexpectedToken) Error() string {
	return fmt.Sprintf("unexpected token - keyword: '%s' literal: '%s' position: '%s'\n\t\t%s:%d\n\t\t%s", e.keyword, e.literal, e.position, e.file, e.line, e.funcName)
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
		case keyword.EOF:
			p.read()
			return
		default:
			p.errPeekUnexpected()
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

func (p *Parser) errPeekUnexpected() {

	unexpected := p.read()

	fpcs := make([]uintptr, 1)
	// Skip 2 levels to get the caller
	runtime.Callers(2, fpcs)

	//_, file, line, _ := runtime.Caller(1)
	fn := runtime.FuncForPC(fpcs[0])
	file, line := fn.FileLine(fpcs[0])

	p.err = ErrUnexpectedToken{
		keyword:  unexpected.Keyword,
		position: unexpected.TextPosition,
		literal:  p.input.ByteSliceString(unexpected.Literal),
		file:     file,
		line:     line,
		funcName: fn.Name(),
	}
}

func (p *Parser) mustRead(keyword keyword.Keyword) token.Token {
	next := p.read()
	if next.Keyword != keyword {
		p.err = fmt.Errorf("want keyword '%s', got: '%s'", keyword.String(), next.Keyword.String())
	}
	return next
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
			p.errPeekUnexpected()
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

	tok := p.read()

	switch tok.Keyword {
	case keyword.STRING:
		value.Kind = ast.ValueKindString
	default:
		p.err = fmt.Errorf("must implement parseValue for keyword %s", tok.Keyword)
	}

	value.Raw = tok.Literal

	return
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
	default:
		p.errPeekUnexpected()
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
				p.errPeekUnexpected()
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
				p.errPeekUnexpected()
				return
			}
		default:
			if acceptIdent {
				p.errPeekUnexpected()
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
		case keyword.STRING, keyword.BLOCKSTRING, keyword.IDENT:
			ref := p.parseFieldDefinition()
			if !list.HasNext() {
				list.SetFirst(ref)
			}
			if previous != -1 {
				p.document.FieldDefinitions[previous].SetNext(ref)
			}
			previous = ref
		default:
			p.errPeekUnexpected()
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
	case keyword.IDENT:
		break
	default:
		p.errPeekUnexpected()
		return -1
	}

	fieldDefinition.Name = p.mustRead(keyword.IDENT).Literal
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
		p.errPeekUnexpected()
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
			p.errPeekUnexpected()
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
		Body:          tok.Literal,
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
			p.errPeekUnexpected()
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
		p.errPeekUnexpected()
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
				p.errPeekUnexpected()
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
				p.errPeekUnexpected()
				return
			}
		default:
			if expectNext {
				p.errPeekUnexpected()
			}
			return
		}
	}
}
