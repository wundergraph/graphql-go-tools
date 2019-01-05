package parser

import (
	. "github.com/franela/goblin"
	"github.com/jensneuse/graphql-go-tools/pkg/document"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/types"
	"testing"
)

func TestParseEnumValuesDefinition(t *testing.T) {

	g := Goblin(t)
	RegisterFailHandler(func(m string, _ ...int) { g.Fail(m) })

	g.Describe("parseEnumValuesDefinition", func() {

		tests := []struct {
			it                      string
			input                   string
			expectErr               types.GomegaMatcher
			expectIndex             types.GomegaMatcher
			expectParsedDefinitions types.GomegaMatcher
		}{

			{
				it: "should parse simple EnumValueDefinitions",
				input: `{
	NORTH
	EAST
	SOUTH
	WEST
}`,
				expectErr:   BeNil(),
				expectIndex: Equal([]int{0, 1, 2, 3}),
				expectParsedDefinitions: Equal(ParsedDefinitions{
					EnumValuesDefinitions: document.EnumValueDefinitions{
						{
							EnumValue:  "NORTH",
							Directives: []int{},
						},
						{
							EnumValue:  "EAST",
							Directives: []int{},
						},
						{
							EnumValue:  "SOUTH",
							Directives: []int{},
						},
						{
							EnumValue:  "WEST",
							Directives: []int{},
						},
					},
				}.initEmptySlices()),
			},
			{
				it: "should parse EnumValueDefinitions with descriptions",
				input: `{
	"describes north"
	NORTH
	"describes east"
	EAST
	"describes south"
	SOUTH
	"describes west"
	WEST
}`,
				expectErr:   BeNil(),
				expectIndex: Equal([]int{0, 1, 2, 3}),
				expectParsedDefinitions: Equal(ParsedDefinitions{
					Arguments: document.Arguments{},
					EnumValuesDefinitions: document.EnumValueDefinitions{
						{
							Description: "describes north",
							EnumValue:   "NORTH",
							Directives:  []int{},
						},
						{
							Description: "describes east",
							EnumValue:   "EAST",
							Directives:  []int{},
						},
						{
							Description: "describes south",
							EnumValue:   "SOUTH",
							Directives:  []int{},
						},
						{
							Description: "describes west",
							EnumValue:   "WEST",
							Directives:  []int{},
						},
					},
				}.initEmptySlices()),
			},
			{
				it: "should parse EnumValueDefinitions with descriptions and empty lines",
				input: `{

	"describes north"

	NORTH

	"describes east"
	EAST

	"describes south"

	SOUTH

"""
describes west
"""

	WEST

}`,
				expectErr:   BeNil(),
				expectIndex: Equal([]int{0, 1, 2, 3}),
				expectParsedDefinitions: Equal(ParsedDefinitions{
					Arguments: document.Arguments{},
					EnumValuesDefinitions: document.EnumValueDefinitions{
						{
							Description: "describes north",
							EnumValue:   "NORTH",
							Directives:  []int{},
						},
						{
							Description: "describes east",
							EnumValue:   "EAST",
							Directives:  []int{},
						},
						{
							Description: "describes south",
							EnumValue:   "SOUTH",
							Directives:  []int{},
						},
						{
							Description: "describes west",
							EnumValue:   "WEST",
							Directives:  []int{},
						},
					},
				}.initEmptySlices()),
			},
			{
				it: "should parse a EnumValueDefinitions with multiple Directives",
				input: `{
	NORTH @fromTop(to: "bottom") @fromBottom(to: "top")
}`,
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
					EnumValuesDefinitions: document.EnumValueDefinitions{
						{
							EnumValue:  "NORTH",
							Directives: []int{0, 1},
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
				}.initEmptySlices()),
			},
			{
				it: "should parse multiple EnumValueDefinitions with multiple Directives",
				input: `{
	NORTH @fromTop(to: "bottom") @fromBottom(to: "top")
	EAST @fromTop(to: "bottom") @fromBottom(to: "top")
}`,
				expectErr:   BeNil(),
				expectIndex: Equal([]int{0, 1}),
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
					EnumValuesDefinitions: document.EnumValueDefinitions{
						{
							EnumValue:  "NORTH",
							Directives: []int{0, 1},
						},
						{
							EnumValue:  "EAST",
							Directives: []int{2, 3},
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
						document.Directive{
							Name:      "fromTop",
							Arguments: []int{2},
						},
						document.Directive{
							Name:      "fromBottom",
							Arguments: []int{3},
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
				err := parser.parseEnumValuesDefinition(&index)
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
