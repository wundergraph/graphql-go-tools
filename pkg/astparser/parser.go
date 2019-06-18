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
		case keyword.STRING:
			p.parseDescription()
		case keyword.TYPE:
			p.parseObjectTypeDefinition()
		case keyword.EOF:
			p.read()
			return
		default:
			p.err = p.errPeekUnexpected()
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

func (p *Parser) errPeekUnexpected() error {

	unexpected := p.read()

	fpcs := make([]uintptr, 1)
	// Skip 2 levels to get the caller
	runtime.Callers(2, fpcs)

	//_, file, line, _ := runtime.Caller(1)
	fn := runtime.FuncForPC(fpcs[0])
	file, line := fn.FileLine(fpcs[0])

	return ErrUnexpectedToken{
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
				NamedType: ast.NamedType{
					Name: namedType.Literal,
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
			p.err = p.errPeekUnexpected()
			return ast.RootOperationTypeDefinitionList{}
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
	}

	value.Raw = tok.Literal

	return
}

func (p *Parser) parseObjectTypeDefinition(description ...ast.Description) {

	var objectTypeDefinition ast.ObjectTypeDefinition

	objectTypeDefinition.TypeLiteral = p.read().TextPosition
	objectTypeDefinition.Name = p.mustRead(keyword.IDENT).Literal
	if p.peek(true) == keyword.IMPLEMENTS {
		objectTypeDefinition.ImplementsInterfaces = p.parseImplementsInterfaces()
	}
	if p.peek(true) == keyword.AT {
		objectTypeDefinition.Directives = p.parseDirectiveList()
	}

	objectTypeDefinition.FieldsDefinition = p.parseFieldDefinitionList()
}

func (p *Parser) parseDescription() {
	descriptionLiteral := p.read()
	description := ast.Description{
		Position:  descriptionLiteral.TextPosition,
		Body:      descriptionLiteral.Literal,
		IsDefined: true,
	}

	next := p.peek(true)
	switch next {
	case keyword.TYPE:
		p.parseObjectTypeDefinition(description)
		return
	default:
		p.err = p.errPeekUnexpected()
	}
}

func (p *Parser) parseImplementsInterfaces() (list ast.NamedTypeList) {
	return
}

func (p *Parser) parseFieldDefinitionList() (list ast.FieldDefinitionList) {

	p.mustRead(keyword.CURLYBRACKETOPEN)

	for {
		next := p.read()
		if next.Keyword == keyword.CURLYBRACKETCLOSE {
			break
		}

		colon := p.mustRead(keyword.COLON)
		

		field := ast.FieldDefinition{
			Name: next.Literal,
			Colon: colon.TextPosition,
			Type:
		}
	}

	return
}
