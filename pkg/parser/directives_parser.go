package parser

import (
	"github.com/jensneuse/graphql-go-tools/pkg/document"
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/keyword"
)

func (p *Parser) parseDirectives(index *[]int) error {

	for {
		next, err := p.l.Peek(true)
		if err != nil {
			return err
		}

		if next == keyword.AT {

			_, err = p.l.Read()
			if err != nil {
				return err
			}

			ident, err := p.readExpect(keyword.IDENT, "parseDirectives")
			if err != nil {
				return err
			}

			directive := document.Directive{
				Name: ident.Literal,
			}

			err = p.parseArguments(&directive.Arguments)
			if err != nil {
				return err
			}

			*index = append(*index, p.putDirective(directive))

		} else {
			return err
		}
	}
}
