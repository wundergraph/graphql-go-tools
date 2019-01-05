package parser

import (
	. "github.com/franela/goblin"
	"github.com/jensneuse/graphql-go-tools/pkg/document"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/types"
	"testing"
)

func TestParseScalar(t *testing.T) {

	g := Goblin(t)
	RegisterFailHandler(func(m string, _ ...int) { g.Fail(m) })

	g.Describe("parseScalar", func() {
		tests := []struct {
			it                      string
			input                   string
			expectErr               types.GomegaMatcher
			expectIndex             types.GomegaMatcher
			expectParsedDefinitions types.GomegaMatcher
		}{
			{
				it:          "should parse simple scalar",
				input:       ` JSON`,
				expectErr:   BeNil(),
				expectIndex: Equal([]int{0}),
				expectParsedDefinitions: Equal(ParsedDefinitions{
					ScalarTypeDefinitions: document.ScalarTypeDefinitions{
						{
							Name:       "JSON",
							Directives: []int{},
						},
					},
				}.initEmptySlices()),
			},
			{
				it:          "should parse a scalar with Directives",
				input:       ` JSON @fromTop(to: "bottom") @fromBottom(to: "top") `,
				expectErr:   BeNil(),
				expectIndex: Equal([]int{0}),
				expectParsedDefinitions: Equal(ParsedDefinitions{
					Arguments: document.Arguments{
						{
							Name: "to",
							Value: document.Value{
								ValueType:   document.ValueTypeString,
								StringValue: "bottom",
							},
						},
						{
							Name: "to",
							Value: document.Value{
								ValueType:   document.ValueTypeString,
								StringValue: "top",
							},
						},
					},
					Directives: document.Directives{
						{
							Name:      "fromTop",
							Arguments: []int{0},
						},
						{
							Name:      "fromBottom",
							Arguments: []int{1},
						},
					},
					ScalarTypeDefinitions: document.ScalarTypeDefinitions{
						{
							Name:       "JSON",
							Directives: []int{0, 1},
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

				var index []int
				err := parser.parseScalarTypeDefinition(&index)
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
