package parser

import (
	"fmt"
	"github.com/jensneuse/graphql-go-tools/pkg/document"
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/keyword"
)

func (p *Parser) parseArgumentSet(index *int) error {

	key := p.l.Peek(true)

	if key != keyword.BRACKETOPEN {
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
			argument.Name = p.putByteSliceReference(identToken.Literal)
			argument.Position.MergeStartIntoStart(identToken.TextPosition)
		} else if key == keyword.BRACKETCLOSE {
			_ = p.l.Read()
			*index = p.putArgumentSet(set)
			return nil
		} else {
			return fmt.Errorf("parseArgumentSet: ident/bracketclose expected, got %s", key)
		}

		key = p.l.Peek(true)

		if key == keyword.COLON {
			_ = p.l.Read()
		} else {
			return fmt.Errorf("parseArgumentSet: colon expected, got %s", key)
		}

		err := p.parseValue(&argument.Value)
		if err != nil {
			return err
		}

		argument.Position.MergeStartIntoEnd(p.TextPosition())

		set = append(set, p.putArgument(argument))
	}
}
