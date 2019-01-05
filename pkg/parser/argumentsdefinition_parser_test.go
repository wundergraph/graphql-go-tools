package parser

import (
	. "github.com/franela/goblin"
	"github.com/jensneuse/graphql-go-tools/pkg/document"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/types"
	"testing"
)

func TestArgumentsDefinitionParser(t *testing.T) {

	g := Goblin(t)
	RegisterFailHandler(func(m string, _ ...int) { g.Fail(m) })

	g.Describe("parser.parseArgumentsDefinition", func() {

		tests := []struct {
			it                      string
			input                   string
			expectErr               types.GomegaMatcher
			expectIndex             types.GomegaMatcher
			expectParsedDefinitions types.GomegaMatcher
		}{
			{
				it:          "should parse a simple ArgumentsDefinition",
				input:       `(inputValue: Int)`,
				expectErr:   BeNil(),
				expectIndex: Equal([]int{0}),
				expectParsedDefinitions: Equal(ParsedDefinitions{
					InputValueDefinitions: document.InputValueDefinitions{
						{
							Name: "inputValue",
							Type: document.NamedType{
								Name: "Int",
							},
							Directives: []int{},
						},
					},
				}.initEmptySlices()),
			},
			{
				it:                      "should not parse an optional ArgumentsDefinition",
				input:                   ` `,
				expectErr:               BeNil(),
				expectIndex:             Equal([]int{}),
				expectParsedDefinitions: Equal(ParsedDefinitions{}.initEmptySlices()),
			},
			{
				it:          "should be able to parse multiple InputValueDefinitions within an ArgumentsDefinition",
				input:       `(inputValue: Int, outputValue: String)`,
				expectErr:   BeNil(),
				expectIndex: Equal([]int{0, 1}),
				expectParsedDefinitions: Equal(ParsedDefinitions{
					InputValueDefinitions: document.InputValueDefinitions{
						{
							Name: "inputValue",
							Type: document.NamedType{
								Name: "Int",
							},
							Directives: []int{},
						},
						{
							Name: "outputValue",
							Type: document.NamedType{
								Name: "String",
							},
							Directives: []int{},
						},
					},
				}.initEmptySlices()),
			},
			{
				it:                      "should return empty when no BRACKETOPEN at beginning (since it can be optional)",
				input:                   `inputValue: Int)`,
				expectErr:               BeNil(),
				expectIndex:             Equal([]int{}),
				expectParsedDefinitions: Equal(ParsedDefinitions{}.initEmptySlices()),
			},
			{
				it:                      "should fail when double BRACKETOPEN at beginning",
				input:                   `((inputValue: Int)`,
				expectErr:               HaveOccurred(),
				expectIndex:             Equal([]int{}),
				expectParsedDefinitions: Equal(ParsedDefinitions{}.initEmptySlices()),
			},
			{
				it:          "should fail when no BRACKETCLOSE at the end",
				input:       `(inputValue: Int`,
				expectErr:   HaveOccurred(),
				expectIndex: Equal([]int{0}),
				expectParsedDefinitions: Equal(ParsedDefinitions{
					InputValueDefinitions: document.InputValueDefinitions{
						{
							Name: "inputValue",
							Type: document.NamedType{
								Name: "Int",
							},
							Directives: []int{},
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
				err := parser.parseArgumentsDefinition(&index)
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
