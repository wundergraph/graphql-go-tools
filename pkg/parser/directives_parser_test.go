package parser

import (
	. "github.com/franela/goblin"
	"github.com/jensneuse/graphql-go-tools/pkg/document"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/types"
	"testing"
)

func TestDirectivesParser(t *testing.T) {

	g := Goblin(t)
	RegisterFailHandler(func(m string, _ ...int) { g.Fail(m) })

	g.Describe("parser.parseDirectives", func() {

		tests := []struct {
			it                      string
			input                   string
			expectErr               types.GomegaMatcher
			expectIndex             types.GomegaMatcher
			expectParsedDefinitions types.GomegaMatcher
		}{
			{
				it:          "should parse a single simple directive",
				input:       `@rename(index: 3)`,
				expectErr:   BeNil(),
				expectIndex: Equal([]int{0}),
				expectParsedDefinitions: Equal(ParsedDefinitions{
					Arguments: document.Arguments{
						{
							Name: "index",
							Value: document.Value{
								ValueType: document.ValueTypeInt,
								IntValue:  3,
							},
						},
					},
					Directives: document.Directives{
						{
							Name:      "rename",
							Arguments: []int{0},
						},
					},
				}.initEmptySlices()),
			},
			{
				it:          "should parse multiple simple directives",
				input:       `@rename(index: 3)@moveto(index: 4)`,
				expectErr:   BeNil(),
				expectIndex: Equal([]int{0, 1}),
				expectParsedDefinitions: Equal(ParsedDefinitions{
					Arguments: document.Arguments{
						{
							Name: "index",
							Value: document.Value{
								ValueType: document.ValueTypeInt,
								IntValue:  3,
							},
						},
						{
							Name: "index",
							Value: document.Value{
								ValueType: document.ValueTypeInt,
								IntValue:  4,
							},
						},
					},
					Directives: document.Directives{
						document.Directive{
							Name:      "rename",
							Arguments: []int{0},
						},
						document.Directive{
							Name:      "moveto",
							Arguments: []int{1},
						},
					},
				}.initEmptySlices()),
			},
			{
				it:          "should parse a single simple directive with multiple Arguments",
				input:       `@rename(index: 3, count: 10)`,
				expectErr:   BeNil(),
				expectIndex: Equal([]int{0}),
				expectParsedDefinitions: Equal(ParsedDefinitions{
					Arguments: document.Arguments{
						document.Argument{
							Name: "index",
							Value: document.Value{
								ValueType: document.ValueTypeInt,
								IntValue:  3,
							},
						},
						document.Argument{
							Name: "count",
							Value: document.Value{
								ValueType: document.ValueTypeInt,
								IntValue:  10,
							},
						},
					},
					Directives: document.Directives{
						{
							Name:      "rename",
							Arguments: []int{0, 1},
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
				err := parser.parseDirectives(&index)
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
