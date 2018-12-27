package parser

import (
	"bytes"
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
						EnumValue: []byte("NORTH"),
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
						EnumValue: []byte("NORTH"),
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
					},
					{
						EnumValue: []byte("EAST"),
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
					},
				},
				),
			},
		}

		for _, test := range tests {
			test := test

			g.It(test.it, func() {

				reader := bytes.NewReader([]byte(test.input))
				parser := NewParser()
				parser.l.SetInput(reader)

				val, err := parser.parseEnumValuesDefinition()
				Expect(err).To(test.expectErr)
				Expect(val).To(test.expectValues)
			})
		}
	})
}
