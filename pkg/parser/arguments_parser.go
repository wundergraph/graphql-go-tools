package parser

import (
	"fmt"
	"github.com/jensneuse/graphql-go-tools/pkg/document"
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/keyword"
)

func (p *Parser) parseArguments(index *[]int) error {

	key := p.l.Peek(true)

	if key != keyword.BRACKETOPEN {
		return nil
	}

	p.l.Read()

	for {

		var argument document.Argument

		key = p.l.Peek(true)
		if key == keyword.IDENT {
			identToken := p.l.Read()
			argument.Name = identToken.Literal
			argument.Position.MergeStartIntoStart(identToken.TextPosition)
		} else if key == keyword.BRACKETCLOSE {
			_ = p.l.Read()
			return nil
		} else {
			return fmt.Errorf("parseArguments: ident/bracketclose expected, got %s", key)
		}

		key = p.l.Peek(true)

		if key == keyword.COLON {
			_ = p.l.Read()
		} else {
			return fmt.Errorf("parseArguments: colon expected, got %s", key)
		}

		err := p.parseValue(&argument.Value)
		if err != nil {
			return err
		}

		argument.Position.MergeStartIntoEnd(p.TextPosition())

		*index = append(*index, p.putArgument(argument))
	}
}
