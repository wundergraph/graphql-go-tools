package parser

import (
	"github.com/jensneuse/graphql-go-tools/pkg/document"
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/keyword"
)

func (p *Parser) parseImplementsInterfaces() (implementsInterfaces document.ImplementsInterfaces, err error) {

	doesImplement, err := p.peekExpect(keyword.IMPLEMENTS, true)
	if err != nil {
		return implementsInterfaces, err
	}

	if !doesImplement {
		return
	}

	for {
		next, err := p.readExpect(keyword.IDENT, "parseImplementsInterfaces")
		if err != nil {
			return implementsInterfaces, err
		}

		implementsInterfaces = append(implementsInterfaces, string(next.Literal))

		willImplementAnother, err := p.peekExpect(keyword.AND, true)
		if err != nil {
			return implementsInterfaces, err
		}

		if !willImplementAnother {
			return implementsInterfaces, err
		}
	}
}
