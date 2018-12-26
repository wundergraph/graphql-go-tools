package parser

import (
	"github.com/jensneuse/graphql-go-tools/pkg/document"
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/keyword"
)

func (p *Parser) parseDirectiveDefinition() (directiveDefinition document.DirectiveDefinition, err error) {

	_, err = p.readExpect(keyword.AT, "parseDirectiveDefinition")
	if err != nil {
		return directiveDefinition, err
	}

	directiveIdent, err := p.readExpect(keyword.IDENT, "parseDirectiveDefinition")
	if err != nil {
		return directiveDefinition, err
	}

	directiveDefinition.Name = string(directiveIdent.Literal)

	directiveDefinition.ArgumentsDefinition, err = p.parseArgumentsDefinition()
	if err != nil {
		return
	}

	_, err = p.readExpect(keyword.ON, "parseDirectiveDefinition")
	if err != nil {
		return directiveDefinition, err
	}

	var possibleLocations []string

	for {
		next, err := p.l.Peek(true)
		if err != nil {
			return directiveDefinition, err
		}

		if next == keyword.PIPE {
			_, err = p.l.Read()
			if err != nil {
				return directiveDefinition, err
			}
		} else if next == keyword.IDENT {
			location, err := p.l.Read()
			if err != nil {
				return directiveDefinition, err
			}

			possibleLocations = append(possibleLocations, string(location.Literal))
		} else {
			break
		}
	}

	directiveDefinition.DirectiveLocations, err = document.NewDirectiveLocations(possibleLocations, directiveIdent.Position)

	return
}
