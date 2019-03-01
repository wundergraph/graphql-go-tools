package parser

import (
	"fmt"
	"github.com/jensneuse/graphql-go-tools/pkg/document"
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/keyword"
)

func (p *Parser) parseArgumentSet(index *int) error {

	key := p.l.Peek(true)

	if key != keyword.BRACKETOPEN {
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
			argument.Name = p.putByteSliceReference(identToken.Literal)
			argument.Position.MergeStartIntoStart(identToken.TextPosition)
		} else if key == keyword.BRACKETCLOSE {
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

		err = p.parseValue(&argument.Value)
		if err != nil {
			return err
		}

		argument.Position.MergeStartIntoEnd(p.TextPosition())

		set = append(set, p.putArgument(argument))
	}
}
