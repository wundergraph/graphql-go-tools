package astparser

import (
	"github.com/jensneuse/graphql-go-tools/pkg/ast"
	"github.com/jensneuse/graphql-go-tools/pkg/lexer"
	"github.com/jensneuse/graphql-go-tools/pkg/lexer/keyword"
	"github.com/jensneuse/graphql-go-tools/pkg/lexer/token"
)

// Tokenizer takes a raw input and turns it into an AST
type Tokenizer struct {
	lexer        *lexer.Lexer
	tokens       []token.Token
	maxTokens    int
	currentToken int
	skipComments bool
}

// NewTokenizer returns a new tokenizer
func NewTokenizer() *Tokenizer {
	return &Tokenizer{
		tokens:       make([]token.Token, 256),
		lexer:        &lexer.Lexer{},
		skipComments: true,
	}
}

func (p *Tokenizer) Tokenize(input *ast.Input) {
	p.lexer.SetInput(input)
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

// hasNextToken - checks that we haven't reached eof
func (p *Tokenizer) hasNextToken() bool {
	return p.currentToken+1 < p.maxTokens
}

// next - increments current token index if hasNextToken
// otherwise returns current token
func (p *Tokenizer) next() int {
	if p.hasNextToken() {
		p.currentToken++
	}
	return p.currentToken
}

// Read - increments currentToken index and return token if hasNextToken
// otherwise returns keyword.EOF
func (p *Tokenizer) Read() token.Token {
	if p.hasNextToken() {
		return p.tokens[p.next()]
	}

	return token.Token{
		Keyword: keyword.EOF,
	}
}

// Peek - returns token next to currentToken if hasNextToken
// otherwise returns keyword.EOF
func (p *Tokenizer) Peek() keyword.Keyword {
	if p.hasNextToken() {
		nextIndex := p.currentToken + 1
		return p.tokens[nextIndex].Keyword
	}
	return keyword.EOF
}

// PeekLiteral - returns token next to currentToken and token name as a ast.ByteSliceReference if hasNextToken
// otherwise returns keyword.EOF
func (p *Tokenizer) PeekLiteral() (keyword.Keyword, ast.ByteSliceReference) {
	if p.hasNextToken() {
		nextIndex := p.currentToken + 1
		return p.tokens[nextIndex].Keyword, p.tokens[nextIndex].Literal
	}
	return keyword.EOF, ast.ByteSliceReference{}
}
