package parser

import (
	"github.com/jensneuse/graphql-go-tools/legacy/document"
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/keyword"
)

func (p *Parser) parseImplementsInterfaces() (implementsInterfaces document.ByteSliceReferences, err error) {

	if implements := p.peekExpect(keyword.IMPLEMENTS, true); !implements {
		return
	}

	nextRef := -1

	for {
		next, err := p.readExpect(keyword.IDENT, "parseImplementsInterfaces")
		if err != nil {
			return implementsInterfaces, err
		}

		next.Literal.NextRef = nextRef
		nextRef = p._putByteSliceReference(next.Literal)

		if another := p.peekExpect(keyword.AND, true); !another {
			return document.NewByteSliceReferences(nextRef), err
		}
	}
}
