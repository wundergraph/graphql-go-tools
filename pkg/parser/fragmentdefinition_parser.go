package parser

import (
	"github.com/jensneuse/graphql-go-tools/pkg/document"
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/keyword"
)

func (p *Parser) parseFragmentDefinition() (fragmentDefinition document.FragmentDefinition, err error) {

	fragmentIdent, err := p.readExpect(keyword.IDENT, "parseFragmentDefinition")
	if err != nil {
		return fragmentDefinition, err
	}

	fragmentDefinition.FragmentName = string(fragmentIdent.Literal)

	_, err = p.readExpect(keyword.ON, "parseFragmentDefinition")
	if err != nil {
		return fragmentDefinition, err
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
