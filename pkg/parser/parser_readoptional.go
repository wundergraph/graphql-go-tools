package parser

import (
	"bytes"
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/token"
)

func (p *Parser) readOptionalLiteral(expect []byte) (tok token.Token, matched bool, err error) {

	tok, err = p.read(WithReadRepeat())
	if err != nil {
		return
	}

	if !bytes.Equal(tok.Literal, expect) {
		return
	}

	tok, err = p.read()
	if err != nil {
		return
	}

	return tok, true, nil
}

func (p *Parser) readOptionalToken(expect token.Keyword) (tok token.Token, matched bool, err error) {

	tok, err = p.read(WithReadRepeat())
	if err != nil {
		return
	}

	if tok.Keyword != expect {
		return
	}

	tok, err = p.read()
	if err != nil {
		return
	}

	return tok, true, nil
}
