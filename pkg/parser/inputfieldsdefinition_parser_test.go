package parser

import (
	. "github.com/franela/goblin"
	"github.com/jensneuse/graphql-go-tools/pkg/document"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/types"
	"testing"
)

func TestInputFieldsDefinitionParser(t *testing.T) {

	g := Goblin(t)
	RegisterFailHandler(func(m string, _ ...int) { g.Fail(m) })

	g.Describe("parser.parseInputFieldsDefinition", func() {

		tests := []struct {
			it                      string
			input                   string
			expectErr               types.GomegaMatcher
			expectIndex             types.GomegaMatcher
			expectParsedDefinitions types.GomegaMatcher
		}{
			{
				it:          "should parse a simple InputValueDefinitions",
				input:       `{inputValue: Int}`,
				expectErr:   BeNil(),
				expectIndex: Equal([]int{0}),
				expectParsedDefinitions: Equal(ParsedDefinitions{
					InputValueDefinitions: document.InputValueDefinitions{
						{
							Name:       "inputValue",
							Directives: []int{},
							Type: document.NamedType{
								Name: "Int",
							},
						},
					},
				}.initEmptySlices()),
			},
			{
				it:                      "should not parse an optional InputValueDefinitions",
				input:                   ` `,
				expectErr:               BeNil(),
				expectIndex:             Equal([]int{}),
				expectParsedDefinitions: Equal(ParsedDefinitions{}.initEmptySlices()),
			},
			{
				it:          "should be able to parse multiple InputValueDefinitions within an InputValueDefinitions",
				input:       `{inputValue: Int, outputValue: String}`,
				expectErr:   BeNil(),
				expectIndex: Equal([]int{0, 1}),
				expectParsedDefinitions: Equal(ParsedDefinitions{
					InputValueDefinitions: document.InputValueDefinitions{
						{
							Name:       "inputValue",
							Directives: []int{},
							Type: document.NamedType{
								Name: "Int",
							},
						},
						{
							Name:       "outputValue",
							Directives: []int{},
							Type: document.NamedType{
								Name: "String",
							},
						},
					},
				}.initEmptySlices()),
			},
			{
				it:                      "should return empty when no CURLYBRACKETOPEN at beginning (since it can be optional)",
				input:                   `inputValue: Int}`,
				expectErr:               BeNil(),
				expectIndex:             Equal([]int{}),
				expectParsedDefinitions: Equal(ParsedDefinitions{}.initEmptySlices()),
			},
			{
				it:                      "should fail when double CURLYBRACKETOPEN at beginning",
				input:                   `{{inputValue: Int}`,
				expectErr:               HaveOccurred(),
				expectIndex:             Equal([]int{}),
				expectParsedDefinitions: Equal(ParsedDefinitions{}.initEmptySlices()),
			},
			{
				it:          "should fail when no CURLYBRACKETCLOSE at the end",
				input:       `{inputValue: Int`,
				expectErr:   HaveOccurred(),
				expectIndex: Equal([]int{0}),
				expectParsedDefinitions: Equal(ParsedDefinitions{
					InputValueDefinitions: document.InputValueDefinitions{
						{
							Name:       "inputValue",
							Directives: []int{},
							Type: document.NamedType{
								Name: "Int",
							},
						},
					},
				}.initEmptySlices()),
			},
		}

		for _, test := range tests {
			test := test

			g.It(test.it, func() {

				parser := NewParser()
				parser.l.SetInput(test.input)

				index := []int{}
				err := parser.parseInputFieldsDefinition(&index)
				Expect(err).To(test.expectErr)
				if test.expectIndex != nil {
					Expect(index).To(test.expectIndex)
				}
				if test.expectParsedDefinitions != nil {
					Expect(parser.ParsedDefinitions).To(test.expectParsedDefinitions)
				}
			})
		}
	})
}
