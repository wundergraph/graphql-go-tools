package parser

import (
	"github.com/jensneuse/graphql-go-tools/pkg/document"
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/keyword"
)

func (p *Parser) parseDirectives(index *int) error {

	var set document.DirectiveSet

	for {
		next := p.l.Peek(true)

		if next == keyword.AT {

			if cap(set) == 0 {
				p.InitDirectiveSet(&set)
			}

			start := p.l.Read()

			ident, err := p.readExpect(keyword.IDENT, "parseDirectives")
			if err != nil {
				return err
			}

			directive := document.Directive{
				Name: p.putByteSliceReference(ident.Literal),
			}

			directive.Position.MergeStartIntoStart(start.TextPosition)

			err = p.parseArgumentSet(&directive.ArgumentSet)
			if err != nil {
				return err
			}

			directive.Position.MergeStartIntoEnd(p.TextPosition())

			set = append(set, p.putDirective(directive))

		} else {
			*index = p.putDirectiveSet(set)
			return nil
		}
	}
}
