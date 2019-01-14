package parser

import (
	"github.com/jensneuse/graphql-go-tools/pkg/document"
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/keyword"
)

func (p *Parser) parseFragmentDefinition(index *[]int) error {

	var fragmentDefinition document.FragmentDefinition
	p.initFragmentDefinition(&fragmentDefinition)

	fragmentIdent, err := p.readExpect(keyword.IDENT, "parseFragmentDefinition")
	if err != nil {
		return err
	}

	fragmentDefinition.FragmentName = fragmentIdent.Literal

	_, err = p.readExpect(keyword.ON, "parseFragmentDefinition")
	if err != nil {
		return err
	}

	err = p.parseType(&fragmentDefinition.TypeCondition)
	if err != nil {
		return err
	}

	err = p.parseDirectives(&fragmentDefinition.Directives)
	if err != nil {
		return err
	}

	err = p.parseSelectionSet(&fragmentDefinition.SelectionSet)
	if err != nil {
		return err
	}

	*index = append(*index, p.putFragmentDefinition(fragmentDefinition))

	return nil
}
