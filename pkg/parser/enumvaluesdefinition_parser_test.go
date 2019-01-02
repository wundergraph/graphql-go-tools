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
			it           string
			input        string
			expectErr    types.GomegaMatcher
			expectValues types.GomegaMatcher
		}{

			{
				it: "should parse simple EnumValuesDefinition",
				input: `{
	NORTH
	EAST
	SOUTH
	WEST
}`,
				expectErr: BeNil(),
				expectValues: Equal(document.EnumValuesDefinition{
					{
						EnumValue: "NORTH",
					},
					{
						EnumValue: "EAST",
					},
					{
						EnumValue: "SOUTH",
					},
					{
						EnumValue: "WEST",
					},
				},
				),
			},
			{
				it: "should parse EnumValuesDefinition with descriptions",
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
				expectErr: BeNil(),
				expectValues: Equal(document.EnumValuesDefinition{
					{
						Description: "describes north",
						EnumValue:   "NORTH",
					},
					{
						Description: "describes east",
						EnumValue:   "EAST",
					},
					{
						Description: "describes south",
						EnumValue:   "SOUTH",
					},
					{
						Description: "describes west",
						EnumValue:   "WEST",
					},
				},
				),
			},
			{
				it: "should parse EnumValuesDefinition with descriptions and empty lines",
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
				expectErr: BeNil(),
				expectValues: Equal(document.EnumValuesDefinition{
					{
						Description: "describes north",
						EnumValue:   "NORTH",
					},
					{
						Description: "describes east",
						EnumValue:   "EAST",
					},
					{
						Description: "describes south",
						EnumValue:   "SOUTH",
					},
					{
						Description: "describes west",
						EnumValue:   "WEST",
					},
				},
				),
			},
			{
				it: "should parse a EnumValuesDefinition with multiple Directives",
				input: `{
	NORTH @fromTop(to: "bottom") @fromBottom(to: "top")
}`,
				expectErr: BeNil(),
				expectValues: Equal(document.EnumValuesDefinition{
					{
						EnumValue: "NORTH",
						Directives: document.Directives{
							document.Directive{
								Name: "fromTop",
								Arguments: document.Arguments{
									document.Argument{
										Name: "to",
										Value: document.StringValue{
											Val: "bottom",
										},
									},
								},
							},
							document.Directive{
								Name: "fromBottom",
								Arguments: document.Arguments{
									document.Argument{
										Name: "to",
										Value: document.StringValue{
											Val: "top",
										},
									},
								},
							},
						},
					},
				},
				),
			},
			{
				it: "should parse multiple EnumValuesDefinition with multiple Directives",
				input: `{
	NORTH @fromTop(to: "bottom") @fromBottom(to: "top")
	EAST @fromTop(to: "bottom") @fromBottom(to: "top")
}`,
				expectErr: BeNil(),
				expectValues: Equal(document.EnumValuesDefinition{
					{
						EnumValue: "NORTH",
						Directives: document.Directives{
							document.Directive{
								Name: "fromTop",
								Arguments: document.Arguments{
									document.Argument{
										Name: "to",
										Value: document.StringValue{
											Val: "bottom",
										},
									},
								},
							},
							document.Directive{
								Name: "fromBottom",
								Arguments: document.Arguments{
									document.Argument{
										Name: "to",
										Value: document.StringValue{
											Val: "top",
										},
									},
								},
							},
						},
					},
					{
						EnumValue: "EAST",
						Directives: document.Directives{
							document.Directive{
								Name: "fromTop",
								Arguments: document.Arguments{
									document.Argument{
										Name: "to",
										Value: document.StringValue{
											Val: "bottom",
										},
									},
								},
							},
							document.Directive{
								Name: "fromBottom",
								Arguments: document.Arguments{
									document.Argument{
										Name: "to",
										Value: document.StringValue{
											Val: "top",
										},
									},
								},
							},
						},
					},
				},
				),
			},
		}

		for _, test := range tests {
			test := test

			g.It(test.it, func() {

				parser := NewParser()
				parser.l.SetInput(test.input)

				val, err := parser.parseEnumValuesDefinition()
				Expect(err).To(test.expectErr)
				Expect(val).To(test.expectValues)
			})
		}
	})
}
