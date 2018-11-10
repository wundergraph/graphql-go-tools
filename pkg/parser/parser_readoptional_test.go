package parser

import (
	"bytes"
	. "github.com/franela/goblin"
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/literal"
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/token"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/types"
	"testing"
)

func TestReadOptional(t *testing.T) {

	g := Goblin(t)
	RegisterFailHandler(func(m string, _ ...int) { g.Fail(m) })

	g.Describe("parser.readOptionalLiteral", func() {

		tests := []struct {
			it                     string
			input                  string
			literalExpect          token.Literal
			expectErr              types.GomegaMatcher
			expectKeyword          types.GomegaMatcher
			expectMatched          types.GomegaMatcher
			expectNextTokenLiteral types.GomegaMatcher
		}{
			{
				it:                     "should read optional curly bracket open",
				input:                  "{ foo",
				literalExpect:          literal.CURLYBRACKETOPEN,
				expectErr:              BeNil(),
				expectKeyword:          Equal(token.CURLYBRACKETOPEN),
				expectMatched:          Equal(true),
				expectNextTokenLiteral: Equal(token.Literal("foo")),
			},
			{
				it:                     "should re-read wrong optional",
				input:                  "& foo",
				literalExpect:          literal.CURLYBRACKETOPEN,
				expectErr:              BeNil(),
				expectKeyword:          Equal(token.AND),
				expectMatched:          Equal(false),
				expectNextTokenLiteral: Equal(token.Literal("&")),
			},
		}

		for _, test := range tests {
			test := test

			g.It(test.it, func() {

				reader := bytes.NewReader(token.Literal(test.input))
				parser := NewParser()
				parser.l.SetInput(reader)
				tok, matched, err := parser.readOptionalLiteral(test.literalExpect)

				if test.expectErr != nil {
					Expect(err).To(test.expectErr)
				}

				if test.expectKeyword != nil {
					Expect(tok.Keyword).To(test.expectKeyword)
				}

				if test.expectMatched != nil {
					Expect(matched).To(test.expectMatched)
				}

				if test.expectNextTokenLiteral != nil {
					nextToken, _ := parser.read()
					Expect(nextToken.Literal).To(test.expectNextTokenLiteral)
				}
			})
		}
	})

	g.Describe("parser.readOptionalToken", func() {

		tests := []struct {
			it              string
			input           string
			tokenExpect     token.Keyword
			expectErr       types.GomegaMatcher
			expectKeyword   types.GomegaMatcher
			expectMatched   types.GomegaMatcher
			expectNextToken types.GomegaMatcher
		}{
			{
				it:              "should read optional curly bracket open",
				input:           "{ foo",
				tokenExpect:     token.CURLYBRACKETOPEN,
				expectErr:       BeNil(),
				expectKeyword:   Equal(token.CURLYBRACKETOPEN),
				expectMatched:   Equal(true),
				expectNextToken: Equal(token.Literal("foo")),
			},
			{
				it:              "should re-read wrong optional",
				input:           "& foo",
				tokenExpect:     token.CURLYBRACKETOPEN,
				expectErr:       BeNil(),
				expectKeyword:   Equal(token.AND),
				expectMatched:   Equal(false),
				expectNextToken: Equal(token.Literal("&")),
			},
		}

		for _, test := range tests {
			test := test

			g.It(test.it, func() {

				reader := bytes.NewReader(token.Literal(test.input))
				parser := NewParser()
				parser.l.SetInput(reader)
				tok, matched, err := parser.readOptionalToken(test.tokenExpect)

				if test.expectErr != nil {
					Expect(err).To(test.expectErr)
				}

				if test.expectKeyword != nil {
					Expect(tok.Keyword).To(test.expectKeyword)
				}

				if test.expectMatched != nil {
					Expect(matched).To(test.expectMatched)
				}

				if test.expectNextToken != nil {
					nextToken, _ := parser.read()
					Expect(nextToken.Literal).To(test.expectNextToken)
				}
			})
		}
	})
}
