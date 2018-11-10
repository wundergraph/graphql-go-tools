package parser

import (
	"bytes"
	. "github.com/franela/goblin"
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/token"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/types"
	"testing"
)

func TestParseUnionMemberTypes(t *testing.T) {

	g := Goblin(t)
	RegisterFailHandler(func(m string, _ ...int) { g.Fail(m) })

	g.Describe("parseKeywordDividedIdentifiers", func() {
		tests := []struct {
			it           string
			input        string
			expectErr    types.GomegaMatcher
			expectValues types.GomegaMatcher
		}{
			{
				it:        "should parse multiple UnionMemberTypes",
				input:     `Photo | Person | Frame`,
				expectErr: BeNil(),
				expectValues: Equal([]string{
					"Photo", "Person", "Frame",
				}),
			},
			{
				it: "should parse multiple UnionMemberTypes spread over multiple lines",
				input: ` Photo
| Person 
| Frame`,
				expectErr: BeNil(),
				expectValues: Equal([]string{
					"Photo", "Person", "Frame",
				}),
			},
			{
				it:        "should parse multiple UnionMemberTypes even when not separated by spaces",
				input:     `Photo|Person|Frame`,
				expectErr: BeNil(),
				expectValues: Equal([]string{
					"Photo", "Person", "Frame",
				}),
			},
			{
				it:        "should not parse trailing IDENT when no PIPE is found in front",
				input:     `Photo|Person|Frame trailingIdent`,
				expectErr: BeNil(),
				expectValues: Equal([]string{
					"Photo", "Person", "Frame",
				}),
			},
		}

		for _, test := range tests {
			test := test

			g.It(test.it, func() {

				reader := bytes.NewReader([]byte(test.input))
				parser := NewParser()
				parser.l.SetInput(reader)

				val, err := parser.parseKeywordDividedIdentifiers(token.PIPE)
				Expect(err).To(test.expectErr)
				Expect(val).To(test.expectValues)
			})
		}
	})
}
