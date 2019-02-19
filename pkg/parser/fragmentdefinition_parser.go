package parser

import (
	"github.com/jensneuse/graphql-go-tools/pkg/document"
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/keyword"
)

func (p *Parser) parseFragmentDefinition(index *[]int) error {

	start := p.l.Read()

	var fragmentDefinition document.FragmentDefinition
	p.initFragmentDefinition(&fragmentDefinition)
	fragmentDefinition.Position.MergeStartIntoStart(start.TextPosition)

	fragmentIdent, err := p.readExpect(keyword.IDENT, "parseFragmentDefinition")
	if err != nil {
		return err
	}

	fragmentDefinition.FragmentName = p.putByteSliceReference(fragmentIdent.Literal)

	_, err = p.readExpect(keyword.ON, "parseFragmentDefinition")
	if err != nil {
		return err
	}

	typeIdent, err := p.readExpect(keyword.IDENT, "parseFragmentDefinition")
	if err != nil {
		return err
	}
	fragmentType := p.makeType(&fragmentDefinition.TypeCondition)
	fragmentType.Name = p.putByteSliceReference(typeIdent.Literal)
	fragmentType.Kind = document.TypeKindNAMED
	p.putType(fragmentType, fragmentDefinition.TypeCondition)

	err = p.parseDirectives(&fragmentDefinition.DirectiveSet)
	if err != nil {
		return err
	}

	err = p.parseSelectionSet(&fragmentDefinition.SelectionSet)
	if err != nil {
		return err
	}

	fragmentDefinition.Position.MergeStartIntoEnd(p.TextPosition())
	*index = append(*index, p.putFragmentDefinition(fragmentDefinition))

	return nil
}
