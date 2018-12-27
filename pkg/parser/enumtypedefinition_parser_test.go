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
					Name: []byte("Direction"),
					EnumValuesDefinition: document.EnumValuesDefinition{
						{
							EnumValue: []byte("NORTH"),
						},
						{
							EnumValue: []byte("EAST"),
						},
						{
							EnumValue: []byte("SOUTH"),
						},
						{
							EnumValue: []byte("WEST"),
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
					Name: []byte("Direction"),
					EnumValuesDefinition: document.EnumValuesDefinition{
						{
							Description: []byte("describes north"),
							EnumValue:   []byte("NORTH"),
						},
						{
							Description: []byte("describes east"),
							EnumValue:   []byte("EAST"),
						},
						{
							Description: []byte("describes south"),
							EnumValue:   []byte("SOUTH"),
						},
						{
							Description: []byte("describes west"),
							EnumValue:   []byte("WEST"),
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
					Name: []byte("Direction"),
					EnumValuesDefinition: document.EnumValuesDefinition{
						{
							Description: []byte("describes north"),
							EnumValue:   []byte("NORTH"),
						},
						{
							Description: []byte("describes east"),
							EnumValue:   []byte("EAST"),
						},
						{
							Description: []byte("describes south"),
							EnumValue:   []byte("SOUTH"),
						},
						{
							Description: []byte("describes west"),
							EnumValue:   []byte("WEST"),
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
					Name: []byte("Direction"),
					Directives: document.Directives{
						document.Directive{
							Name: []byte("fromTop"),
							Arguments: document.Arguments{
								document.Argument{
									Name: []byte("to"),
									Value: document.StringValue{
										Val: []byte("bottom"),
									},
								},
							},
						},
						document.Directive{
							Name: []byte("fromBottom"),
							Arguments: document.Arguments{
								document.Argument{
									Name: []byte("to"),
									Value: document.StringValue{
										Val: []byte("top"),
									},
								},
							},
						},
					},
					EnumValuesDefinition: document.EnumValuesDefinition{
						{
							EnumValue: []byte("NORTH"),
						},
					},
				}),
			},
			{
				it:        "should parse a EnumTypeDefinition with optional EnumValuesDefinition",
				input:     ` Direction`,
				expectErr: BeNil(),
				expectValues: Equal(document.EnumTypeDefinition{
					Name: []byte("Direction"),
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
