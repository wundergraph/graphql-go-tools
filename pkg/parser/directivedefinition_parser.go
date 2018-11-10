package parser

import (
	"github.com/jensneuse/graphql-go-tools/pkg/document"
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/literal"
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/token"
)

func (p *Parser) parseDirectiveDefinition() (directiveDefinition document.DirectiveDefinition, err error) {

	if _, err = p.read(WithWhitelist(token.AT)); err != nil {
		return directiveDefinition, err
	}

	definingTok, err := p.read(WithWhitelist(token.IDENT))
	if err != nil {
		return
	}

	directiveDefinition.Name = string(definingTok.Literal)

	directiveDefinition.ArgumentsDefinition, err = p.parseArgumentsDefinition()
	if err != nil {
		return
	}

	if tok, matched, err := p.readOptionalLiteral(literal.ON); err != nil || !matched {
		if err != nil {
			return directiveDefinition, err
		}
		return directiveDefinition, newErrInvalidType(tok.Position, "parseDirectiveDefinition", string(literal.ON), string(tok.Literal))
	}

	locations, err := p.parseKeywordDividedIdentifiers(token.PIPE)
	if err != nil {
		return
	}

	directiveDefinition.DirectiveLocations, err = document.NewDirectiveLocations(locations, definingTok.Position)
	return
}
