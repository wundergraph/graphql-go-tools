package parser

import (
	"github.com/jensneuse/graphql-go-tools/pkg/document"
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/keyword"
)

func (p *Parser) parseType(index *int) error {

	isListType := p.peekExpect(keyword.SQUAREBRACKETOPEN, true)

	firstType := p.makeType(index)
	var ofType int
	var name document.ByteSliceReference

	if isListType {

		err := p.parseType(&ofType)
		if err != nil {
			return err
		}

		_, err = p.readExpect(keyword.SQUAREBRACKETCLOSE, "parseListType")
		if err != nil {
			return err
		}
	} else {

		ident, err := p.readExpect(keyword.IDENT, "parseNamedType")
		if err != nil {
			return err
		}

		name = ident.Literal
	}

	isNonNull := p.peekExpect(keyword.BANG, true)

	if !isNonNull && isListType {
		firstType.Kind = document.TypeKindLIST
		firstType.OfType = ofType
	} else if !isNonNull && !isListType {
		firstType.Kind = document.TypeKindNAMED
		firstType.Name = name
	} else if isNonNull && isListType {
		var secondIndex int
		secondType := p.makeType(&secondIndex)
		secondType.Kind = document.TypeKindLIST
		secondType.OfType = ofType
		p.putType(secondType, secondIndex)

		firstType.Kind = document.TypeKindNON_NULL
		firstType.OfType = secondIndex

	} else if isNonNull && !isListType {
		var secondIndex int
		secondType := p.makeType(&secondIndex)
		secondType.Kind = document.TypeKindNAMED
		secondType.Name = name
		p.putType(secondType, secondIndex)

		firstType.Kind = document.TypeKindNON_NULL
		firstType.OfType = secondIndex
	}

	p.putType(firstType, *index)
	return nil
}
