package parser

import (
	"github.com/jensneuse/graphql-go-tools/pkg/document"
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/literal"
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/token"
)

func (p *Parser) parseSelection() (selection document.Selection, err error) {

	_, matchField, err := p.readOptionalToken(token.SPREAD)
	if err != nil {
		return
	}

	if !matchField {
		return p.parseField()
	}

	_, matchInline, err := p.readOptionalLiteral(literal.ON)
	if err != nil {
		return
	}

	if matchInline {
		return p.parseInlineFragment()
	}

	return p.parseFragmentSpread()
}
