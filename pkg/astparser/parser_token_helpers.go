package astparser

import (
	"github.com/jensneuse/graphql-go-tools/pkg/ast"
	"github.com/jensneuse/graphql-go-tools/pkg/lexer/identkeyword"
	"github.com/jensneuse/graphql-go-tools/pkg/lexer/keyword"
	"github.com/jensneuse/graphql-go-tools/pkg/lexer/token"
)

func (p *Parser) next() int {
	if p.currentToken != p.maxTokens-1 {
		p.currentToken++
	}
	return p.currentToken
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

func (p *Parser) readExpectLiteral(expect ...identkeyword.IdentKeyword) (token.Token, identkeyword.IdentKeyword) {
	p.currentToken++
	if p.currentToken < p.maxTokens {
		out := p.tokens[p.currentToken]
		identKey := p.identKeywordToken(out)
		for _, expectation := range expect {
			if identKey == expectation {
				return out, identKey
			}
		}
		p.errUnexpectedToken(out)
		return out, identKey
	}

	return token.Token{
		Keyword: keyword.EOF,
	}, identkeyword.UNDEFINED
}

func (p *Parser) peek() keyword.Keyword {
	nextIndex := p.currentToken + 1
	if nextIndex < p.maxTokens {
		return p.tokens[nextIndex].Keyword
	}
	return keyword.EOF
}

func (p *Parser) peekLiteral() (keyword.Keyword, ast.ByteSliceReference) {
	nextIndex := p.currentToken + 1
	if nextIndex < p.maxTokens {
		return p.tokens[nextIndex].Keyword, p.tokens[nextIndex].Literal
	}
	return keyword.EOF, ast.ByteSliceReference{}
}

func (p *Parser) peekEquals(key keyword.Keyword) bool {
	return p.peek() == key
}

func (p *Parser) peekEqualsIdentKey(identKey identkeyword.IdentKeyword) bool {
	key, literal := p.peekLiteral()
	if key != keyword.IDENT {
		return false
	}
	actualKey := p.identKeywordSliceRef(literal)
	return actualKey == identKey
}

func (p *Parser) mustNext(key keyword.Keyword) int {
	current := p.currentToken
	if p.next() == current {
		p.errUnexpectedToken(p.tokens[p.currentToken], key)
		return p.currentToken
	}
	if p.tokens[p.currentToken].Keyword != key {
		p.errUnexpectedToken(p.tokens[p.currentToken], key)
		return p.currentToken
	}
	return p.currentToken
}

func (p *Parser) mustRead(key keyword.Keyword) (next token.Token) {
	next = p.read()
	if next.Keyword != key {
		p.errUnexpectedToken(next, key)
	}
	return
}

func (p *Parser) mustReadIdentKey(key identkeyword.IdentKeyword) (next token.Token) {
	next = p.read()
	if next.Keyword != keyword.IDENT {
		p.errUnexpectedToken(next, keyword.IDENT)
	}
	identKey := p.identKeywordToken(next)
	if identKey != key {
		p.errUnexpectedIdentKey(next, identKey, key)
	}
	return
}

func (p *Parser) mustReadExceptIdentKey(key identkeyword.IdentKeyword) (next token.Token) {
	next = p.read()
	if next.Keyword != keyword.IDENT {
		p.errUnexpectedToken(next, keyword.IDENT)
	}
	identKey := p.identKeywordToken(next)
	if identKey == key {
		p.errUnexpectedIdentKey(next, identKey, key)
	}
	return
}
