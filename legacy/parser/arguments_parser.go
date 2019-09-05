package parser

import (
	"fmt"
	"github.com/jensneuse/graphql-go-tools/legacy/document"
	"github.com/jensneuse/graphql-go-tools/pkg/lexer/keyword"
)

func (p *Parser) parseArgumentSet(index *int) error {

	key := p.l.Peek(true)

	if key != keyword.LPAREN {
		*index = -1
		return nil
	}

	p.l.Read()

	var set document.ArgumentSet
	p.initArgumentSet(&set)

	for {

		var argument document.Argument

		key = p.l.Peek(true)
		if key == keyword.IDENT {
			identToken := p.l.Read()
			argument.Name = identToken.Literal
			argument.Position.MergeStartIntoStart(identToken.TextPosition)
		} else if key == keyword.RPAREN {
			_ = p.l.Read()
			*index = p.putArgumentSet(set)
			return nil
		} else {
			return fmt.Errorf("parseArgumentSet: ident/bracketclose expected, got %s", key)
		}

		_, err := p.readExpect(keyword.COLON, "parseArgumentSet")
		if err != nil {
			return err
		}

		argument.Value, err = p.parseValue()
		if err != nil {
			return err
		}

		argument.Position.MergeStartIntoEnd(p.TextPosition())

		set = append(set, p.putArgument(argument))
	}
}
