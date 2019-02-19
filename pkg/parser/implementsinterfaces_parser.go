package parser

import (
	"github.com/jensneuse/graphql-go-tools/pkg/document"
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/keyword"
)

func (p *Parser) parseImplementsInterfaces() (implementsInterfaces document.ImplementsInterfaces, err error) {

	if implements := p.peekExpect(keyword.IMPLEMENTS, true); !implements {
		return
	}

	for {
		next, err := p.readExpect(keyword.IDENT, "parseImplementsInterfaces")
		if err != nil {
			return implementsInterfaces, err
		}

		implementsInterfaces = append(implementsInterfaces, p.putByteSliceReference(next.Literal))

		if another := p.peekExpect(keyword.AND, true); !another {
			return implementsInterfaces, err
		}
	}
}
