package parser

import (
	"github.com/jensneuse/graphql-go-tools/pkg/document"
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/keyword"
)

func (p *Parser) parseDirectives() (directives document.Directives, err error) {

	for {
		next, err := p.l.Peek(true)
		if err != nil {
			return directives, err
		}

		if next == keyword.AT {

			_, err = p.l.Read()
			if err != nil {
				return directives, err
			}

			ident, err := p.readExpect(keyword.IDENT, "parseDirectives")
			if err != nil {
				return directives, err
			}

			directive := document.Directive{
				Name: string(ident.Literal),
			}

			directive.Arguments, err = p.parseArguments()
			if err != nil {
				return directives, err
			}

			directives = append(directives, directive)

		} else {
			return directives, err
		}
	}
}
