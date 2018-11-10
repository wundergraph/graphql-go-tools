package parser

import (
	"bytes"
	. "github.com/franela/goblin"
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/token"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/types"
	"testing"
)

func TestParserRead(t *testing.T) {

	g := Goblin(t)
	RegisterFailHandler(func(m string, _ ...int) { g.Fail(m) })

	g.Describe("parser.read", func() {

		tests := []struct {
			it                string
			input             string
			options           []ReadOption
			expectErr         types.GomegaMatcher
			expectToken       types.GomegaMatcher
			expectLiteral     types.GomegaMatcher
			expectDescription types.GomegaMatcher
		}{
			{
				it: "should parse WithIgnore",
				input: ` 	 	x`,
				options: []ReadOption{
					WithIgnore(token.TAB, token.SPACE),
				},
				expectErr:     BeNil(),
				expectToken:   Equal(token.IDENT),
				expectLiteral: Equal(token.Literal("x")),
			},
			{
				it:    "should parse WithWhitelist",
				input: `x`,
				options: []ReadOption{
					WithWhitelist(token.IDENT),
				},
				expectErr:     BeNil(),
				expectToken:   Equal(token.IDENT),
				expectLiteral: Equal(token.Literal("x")),
			},
			{
				it: "should parse WithIgnore & WithWhitelist",
				input: `	 	 x`,
				options: []ReadOption{
					WithIgnore(token.TAB, token.SPACE),
					WithWhitelist(token.IDENT),
				},
				expectErr:     BeNil(),
				expectToken:   Equal(token.IDENT),
				expectLiteral: Equal(token.Literal("x")),
			},
			{
				it:    "should not parse WithBlacklist",
				input: `x`,
				options: []ReadOption{
					WithBlacklist(token.IDENT),
				},
				expectErr:     Not(BeNil()),
				expectToken:   Equal(token.IDENT),
				expectLiteral: Equal(token.Literal("x")),
			},
			{
				it: "should parse ident with single line description",
				input: `"this is x"
x`,
				options: []ReadOption{
					WithDescription(),
					WithWhitelist(token.IDENT),
					WithIgnore(token.LINETERMINATOR),
				},
				expectErr:         BeNil(),
				expectToken:       Equal(token.IDENT),
				expectLiteral:     Equal(token.Literal("x")),
				expectDescription: Equal("this is x"),
			},
			{
				it: "should parse ident with multi line description",
				input: ` """
this is x
x is cool!
"""
	x`,
				options: []ReadOption{
					WithDescription(),
					WithWhitelist(token.IDENT),
					WithIgnore(token.LINETERMINATOR, token.TAB, token.SPACE),
				},
				expectErr:     BeNil(),
				expectToken:   Equal(token.IDENT),
				expectLiteral: Equal(token.Literal("x")),
				expectDescription: Equal(`this is x
x is cool!`),
			},
		}

		for _, test := range tests {
			test := test

			g.It(test.it, func() {

				reader := bytes.NewReader(token.Literal(test.input))
				parser := NewParser()
				parser.l.SetInput(reader)

				tok, err := parser.read(test.options...)
				Expect(err).To(test.expectErr)
				Expect(tok.Keyword).To(test.expectToken)
				Expect(tok.Literal).To(test.expectLiteral)

				if test.expectDescription != nil {
					Expect(tok.Description).To(test.expectDescription)
				}
			})
		}
	})
}
