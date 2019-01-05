package parser

import (
	. "github.com/franela/goblin"
	"github.com/jensneuse/graphql-go-tools/pkg/document"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/types"
	"testing"
)

func TestArgumentsParser(t *testing.T) {

	g := Goblin(t)
	RegisterFailHandler(func(m string, _ ...int) { g.Fail(m) })

	g.Describe("parser.parseArguments", func() {

		tests := []struct {
			it                      string
			input                   string
			expectErr               types.GomegaMatcher
			expectIndex             types.GomegaMatcher
			expectParsedDefinitions types.GomegaMatcher
		}{
			{
				it:          "should parse simple arguments",
				input:       `(name: "Gophus")`,
				expectErr:   BeNil(),
				expectIndex: Equal([]int{0}),
				expectParsedDefinitions: Equal(ParsedDefinitions{
					Arguments: document.Arguments{
						{
							Name: "name",
							Value: document.Value{
								ValueType:   document.ValueTypeString,
								StringValue: "Gophus",
							},
						},
					},
				}.initEmptySlices()),
			},
			{
				it:          "should parse a list of const strings",
				input:       `(fooBars: ["foo","bar"])`,
				expectErr:   BeNil(),
				expectIndex: Equal([]int{0}),
				expectParsedDefinitions: Equal(ParsedDefinitions{
					Arguments: document.Arguments{
						{
							Name: "fooBars",
							Value: document.Value{
								ValueType: document.ValueTypeList,
								ListValue: []document.Value{
									{
										ValueType:   document.ValueTypeString,
										StringValue: "foo",
									},
									{
										ValueType:   document.ValueTypeString,
										StringValue: "bar",
									},
								},
							},
						},
					},
				}.initEmptySlices()),
			},
			{
				it:          "should parse a list of const integers",
				input:       `(integers: [1,2,3])`,
				expectErr:   BeNil(),
				expectIndex: Equal([]int{0}),
				expectParsedDefinitions: Equal(ParsedDefinitions{
					Arguments: document.Arguments{
						{
							Name: "integers",
							Value: document.Value{
								ValueType: document.ValueTypeList,
								ListValue: []document.Value{
									{
										ValueType: document.ValueTypeInt,
										IntValue:  1,
									},
									{
										ValueType: document.ValueTypeInt,
										IntValue:  2,
									},
									{
										ValueType: document.ValueTypeInt,
										IntValue:  3,
									},
								},
							},
						},
					},
				}.initEmptySlices()),
			},
			{
				it:          "should parse multiple arguments",
				input:       `(name: "Gophus", surname: "Gophersson")`,
				expectErr:   BeNil(),
				expectIndex: Equal([]int{0, 1}),
				expectParsedDefinitions: Equal(ParsedDefinitions{
					Arguments: document.Arguments{
						{
							Name: "name",
							Value: document.Value{
								ValueType:   document.ValueTypeString,
								StringValue: "Gophus",
							},
						},
						{
							Name: "surname",
							Value: document.Value{
								ValueType:   document.ValueTypeString,
								StringValue: "Gophersson",
							},
						},
					},
				}.initEmptySlices()),
			},
			{
				it:          "should not parse arguments when no bracket close",
				input:       `(name: "Gophus", surname: "Gophersson"`,
				expectErr:   HaveOccurred(),
				expectIndex: Equal([]int{0, 1}),
			},
			{
				it:          "should parse Arguments optionally",
				input:       `name: "Gophus", surname: "Gophersson")`,
				expectErr:   BeNil(),
				expectIndex: BeNil(),
			},
			{
				it:          "should not parse arguments when multiple brackets open",
				input:       `((name: "Gophus", surname: "Gophersson")`,
				expectErr:   HaveOccurred(),
				expectIndex: BeNil(),
			},
		}

		for _, test := range tests {
			test := test

			g.It(test.it, func() {

				parser := NewParser()
				parser.l.SetInput(test.input)

				var index []int
				err := parser.parseArguments(&index)
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
