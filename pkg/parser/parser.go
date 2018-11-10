package parser

import (
	"bytes"
	"fmt"
	"github.com/jensneuse/graphql-go-tools/pkg/document"
	"github.com/jensneuse/graphql-go-tools/pkg/lexer"
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/token"
	"github.com/jensneuse/graphql-go-tools/pkg/runestringer"
	"io"
)

type errInvalidType struct {
	enclosingFunctionName string
	expected              string
	actual                string
	position              token.Position
}

func newErrInvalidType(position token.Position, enclosingFunctionName, expected, actual string) errInvalidType {
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
	l    *lexer.Lexer
	buff bytes.Buffer
}

// NewParser returns a new parser using a buffered runestringer
func NewParser() *Parser {
	return &Parser{
		l:    lexer.NewLexer(runestringer.NewBuffered()),
		buff: bytes.Buffer{},
	}
}

// ParseTypeSystemDefinition reads from an io.Reader and emits a document.Definition
func (p *Parser) ParseTypeSystemDefinition(reader io.Reader) (def document.TypeSystemDefinition, err error) {
	p.l.SetInput(reader)
	return p.parseTypeSystemDefinition()
}
