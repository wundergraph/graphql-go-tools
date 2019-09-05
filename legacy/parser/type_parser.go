package parser

import (
	"github.com/jensneuse/graphql-go-tools/legacy/document"
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/keyword"
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/token"
)

func (p *Parser) parseType(index *int) error {

	isListType := p.peekExpect(keyword.LBRACK, false)

	var start token.Token
	var err error

	firstType := p.makeType(index)
	var ofType int
	var name document.ByteSliceReference

	if isListType {

		start = p.l.Read()

		err := p.parseType(&ofType)
		if err != nil {
			return err
		}

		_, err = p.readExpect(keyword.RBRACK, "parseListType")
		if err != nil {
			return err
		}
	} else {

		start, err = p.readExpect(keyword.IDENT, "parseNamedType")
		if err != nil {
			return err
		}

		name = start.Literal
	}

	firstType.Position.MergeStartIntoStart(start.TextPosition)

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

	firstType.Position.MergeStartIntoEnd(p.TextPosition())
	p.putType(firstType, *index)
	return nil
}
