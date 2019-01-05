package parser

import (
	. "github.com/franela/goblin"
	"github.com/jensneuse/graphql-go-tools/pkg/document"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/types"
	"testing"
)

func TestDirectiveDefinitionParser(t *testing.T) {

	g := Goblin(t)
	RegisterFailHandler(func(m string, _ ...int) { g.Fail(m) })

	g.Describe("parser.parseDirectiveDefinition", func() {

		tests := []struct {
			it                      string
			input                   string
			expectErr               types.GomegaMatcher
			expectIndex             types.GomegaMatcher
			expectParsedDefinitions types.GomegaMatcher
		}{
			{
				it:          "should parse a simple DirectiveDefinition",
				input:       "@ somewhere on QUERY",
				expectErr:   BeNil(),
				expectIndex: Equal([]int{0}),
				expectParsedDefinitions: Equal(ParsedDefinitions{
					DirectiveDefinitions: document.DirectiveDefinitions{
						{
							Name: "somewhere",
							DirectiveLocations: document.DirectiveLocations{
								document.DirectiveLocationQUERY,
							},
							ArgumentsDefinition: []int{},
						},
					},
				}.initEmptySlices()),
			},
			{
				it:          "should parse a simple DirectiveDefinition with trailing PIPE",
				input:       "@ somewhere on | QUERY",
				expectErr:   BeNil(),
				expectIndex: Equal([]int{0}),
				expectParsedDefinitions: Equal(ParsedDefinitions{
					DirectiveDefinitions: document.DirectiveDefinitions{
						{
							Name: "somewhere",
							DirectiveLocations: document.DirectiveLocations{
								document.DirectiveLocationQUERY,
							},
							ArgumentsDefinition: []int{},
						},
					},
				}.initEmptySlices()),
			},
			{
				it:          "should parse a DirectiveDefinition with ArgumentsDefinition",
				input:       "@ somewhere(inputValue: Int) on QUERY",
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
					DirectiveDefinitions: document.DirectiveDefinitions{
						{
							Name:                "somewhere",
							ArgumentsDefinition: []int{0},
							DirectiveLocations: document.DirectiveLocations{
								document.DirectiveLocationQUERY,
							},
						},
					},
				}.initEmptySlices()),
			},
			{
				it:                      "should not parse a DirectiveDefinition where the 'on' is missing",
				input:                   "@ somewhere QUERY",
				expectErr:               HaveOccurred(),
				expectIndex:             Equal([]int{}),
				expectParsedDefinitions: Equal(ParsedDefinitions{}.initEmptySlices()),
			},
			{
				it:                      "should not parse a DirectiveDefinition where 'on' is not exactly spelled",
				input:                   "@ somewhere off QUERY",
				expectErr:               HaveOccurred(),
				expectIndex:             Equal([]int{}),
				expectParsedDefinitions: Equal(ParsedDefinitions{}.initEmptySlices()),
			},
			{
				it:                      "should not parse a DirectiveDefinition when an invalid DirectiveLocation is given",
				input:                   "@ somewhere on QUERY | thisshouldntwork",
				expectErr:               HaveOccurred(),
				expectIndex:             Equal([]int{}),
				expectParsedDefinitions: Equal(ParsedDefinitions{}.initEmptySlices()),
			},
		}

		for _, test := range tests {
			test := test

			g.It(test.it, func() {

				parser := NewParser()
				parser.l.SetInput(test.input)

				index := []int{}
				err := parser.parseDirectiveDefinition(&index)
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
