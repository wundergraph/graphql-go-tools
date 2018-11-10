package parser

import (
	"github.com/jensneuse/graphql-go-tools/pkg/document"
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/literal"
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/token"
)

func (p *Parser) parseFragmentDefinition() (fragmentDefinition document.FragmentDefinition, err error) {

	tok, err := p.read(WithWhitelist(token.IDENT), WithExcludeLiteral(literal.ON))
	if err != nil {
		return
	}

	fragmentDefinition.FragmentName = string(tok.Literal)

	tok, err = p.read(WithWhitelist(token.IDENT))
	if err != nil {
		return
	}
	if !tok.Literal.Equals(literal.ON) {
		return fragmentDefinition, newErrInvalidType(tok.Position, "parseFragmentDefinition", string(literal.ON), string(tok.Literal))
	}

	fragmentDefinition.TypeCondition, err = p.parseNamedType()
	if err != nil {
		return
	}

	fragmentDefinition.Directives, err = p.parseDirectives()
	if err != nil {
		return
	}

	fragmentDefinition.SelectionSet, err = p.parseSelectionSet()

	return
}
