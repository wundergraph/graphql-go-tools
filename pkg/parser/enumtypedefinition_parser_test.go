package parser

import (
	"bytes"
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
			it           string
			input        string
			expectErr    types.GomegaMatcher
			expectValues types.GomegaMatcher
		}{

			{
				it: "should parse simple EnumTypeDefinition",
				input: ` Direction {
  NORTH
  EAST
  SOUTH
  WEST
}`,
				expectErr: BeNil(),
				expectValues: Equal(document.EnumTypeDefinition{
					Name: "Direction",
					EnumValuesDefinition: document.EnumValuesDefinition{
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
				}),
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
				expectErr: BeNil(),
				expectValues: Equal(document.EnumTypeDefinition{
					Name: "Direction",
					EnumValuesDefinition: document.EnumValuesDefinition{
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
				}),
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
				expectErr: BeNil(),
				expectValues: Equal(document.EnumTypeDefinition{
					Name: "Direction",
					EnumValuesDefinition: document.EnumValuesDefinition{
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
				}),
			},
			{
				it: "should parse EnumTypeDefinition with Directives",
				input: ` Direction @fromTop(to: "bottom") @fromBottom(to: "top"){
  NORTH
}`,
				expectErr: BeNil(),
				expectValues: Equal(document.EnumTypeDefinition{
					Name: "Direction",
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
					EnumValuesDefinition: document.EnumValuesDefinition{
						{
							EnumValue: "NORTH",
						},
					},
				}),
			},
			{
				it:        "should parse a EnumTypeDefinition with optional EnumValuesDefinition",
				input:     ` Direction`,
				expectErr: BeNil(),
				expectValues: Equal(document.EnumTypeDefinition{
					Name: "Direction",
				}),
			},
		}

		for _, test := range tests {
			test := test

			g.It(test.it, func() {

				reader := bytes.NewReader([]byte(test.input))
				parser := NewParser()
				parser.l.SetInput(reader)

				val, err := parser.parseEnumTypeDefinition()
				Expect(err).To(test.expectErr)
				Expect(val).To(test.expectValues)
			})
		}
	})
}
