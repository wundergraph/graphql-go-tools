package parser

import (
	. "github.com/franela/goblin"
	"github.com/jensneuse/graphql-go-tools/pkg/document"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/types"
	"testing"
)

func TestParseEnumTypeDefinition(t *testing.T) {

	g := Goblin(t)
	RegisterFailHandler(func(m string, _ ...int) { g.Fail(m) })

	g.Describe("parseEnumTypeDefinition", func() {
		tests := []struct {
			it                      string
			input                   string
			expectErr               types.GomegaMatcher
			expectIndex             types.GomegaMatcher
			expectParsedDefinitions types.GomegaMatcher
		}{

			{
				it: "should parse simple EnumTypeDefinition",
				input: ` Direction {
  NORTH
  EAST
  SOUTH
  WEST
}`,
				expectErr:   BeNil(),
				expectIndex: Equal([]int{0}),
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
					EnumTypeDefinitions: document.EnumTypeDefinitions{
						{
							Name:                 "Direction",
							EnumValuesDefinition: []int{0, 1, 2, 3},
							Directives:           []int{},
						},
					},
				}.initEmptySlices()),
			},
			{
				it: "should parse EnumTypeDefinition with descriptions",
				input: ` Direction {
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
				expectIndex: Equal([]int{0}),
				expectParsedDefinitions: Equal(ParsedDefinitions{
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
					EnumTypeDefinitions: document.EnumTypeDefinitions{
						{
							Name:                 "Direction",
							EnumValuesDefinition: []int{0, 1, 2, 3},
							Directives:           []int{},
						},
					},
				}.initEmptySlices()),
			},
			{
				it: "should parse EnumTypeDefinition with descriptions, spaces and empty lines",
				input: ` Direction {
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
				expectIndex: Equal([]int{0}),
				expectParsedDefinitions: Equal(ParsedDefinitions{
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
					EnumTypeDefinitions: document.EnumTypeDefinitions{
						{
							Name:                 "Direction",
							EnumValuesDefinition: []int{0, 1, 2, 3},
							Directives:           []int{},
						},
					},
				}.initEmptySlices()),
			},
			{
				it: "should parse EnumTypeDefinition with Directives",
				input: ` Direction @fromTop(to: "bottom") @fromBottom(to: "top"){
  NORTH
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
					Directives: document.Directives{
						document.Directive{
							Name:      "fromTop",
							Arguments: []int{0},
						},
						document.Directive{
							Name:      "fromBottom",
							Arguments: []int{1},
						},
					},
					EnumValuesDefinitions: document.EnumValueDefinitions{
						{
							EnumValue:  "NORTH",
							Directives: []int{},
						},
					},
					EnumTypeDefinitions: document.EnumTypeDefinitions{
						{
							Name:                 "Direction",
							Directives:           []int{0, 1},
							EnumValuesDefinition: []int{0},
						},
					},
				}.initEmptySlices()),
			},
			{
				it:          "should parse a EnumTypeDefinition with optional EnumValueDefinitions",
				input:       ` Direction`,
				expectErr:   BeNil(),
				expectIndex: Equal([]int{0}),
				expectParsedDefinitions: Equal(ParsedDefinitions{
					EnumTypeDefinitions: document.EnumTypeDefinitions{
						{
							Name:                 "Direction",
							Directives:           []int{},
							EnumValuesDefinition: []int{},
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
				err := parser.parseEnumTypeDefinition(&index)
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
