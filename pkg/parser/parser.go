package parser

import (
	"bytes"
	"fmt"
	"github.com/jensneuse/graphql-go-tools/pkg/document"
	"github.com/jensneuse/graphql-go-tools/pkg/lexer"
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/keyword"
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/position"
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/token"
	"io"
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

// Parser holds the lexer and a buffer for writing literals
type Parser struct {
	l    Lexer
	buff bytes.Buffer
}

// Lexer is the interface used by the Parser to lex tokens
type Lexer interface {
	SetInput(reader io.Reader)
	Read() (tok token.Token, err error)
	Peek(ignoreWhitespace bool) (key keyword.Keyword, err error)
}

// NewParser returns a new parser using a buffered runestringer
func NewParser() *Parser {
	return &Parser{
		l:    lexer.NewLexer(),
		buff: bytes.Buffer{},
	}
}

// ParseTypeSystemDefinition parses a TypeSystemDefinition from an io.Reader
func (p *Parser) ParseTypeSystemDefinition(reader io.Reader) (def document.TypeSystemDefinition, err error) {
	p.l.SetInput(reader)
	return p.parseTypeSystemDefinition()
}

// ParseExecutableDefinition parses an ExecutableDefinition from an io.Reader
func (p *Parser) ParseExecutableDefinition(reader io.Reader) (def document.ExecutableDefinition, err error) {
	p.l.SetInput(reader)
	return p.parseExecutableDefinition()
}

func (p *Parser) readExpect(expected keyword.Keyword, enclosingFunctionName string) (t token.Token, err error) {
	t, err = p.l.Read()
	if err != nil {
		return t, err
	}

	if t.Keyword != expected {
		return t, newErrInvalidType(t.Position, enclosingFunctionName, expected.String(), t.Keyword.String()+" lit: "+string(t.Literal))
	}

	return
}

func (p *Parser) peekExpect(expected keyword.Keyword, swallow bool) (matched bool, err error) {
	next, err := p.l.Peek(true)
	if err != nil {
		return false, err
	}

	matched = next == expected

	if matched && swallow {
		_, err = p.l.Read()
	}

	return
}
