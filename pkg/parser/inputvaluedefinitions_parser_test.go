package parser

import (
	. "github.com/franela/goblin"
	"github.com/jensneuse/graphql-go-tools/pkg/document"
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/keyword"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/types"
	"testing"
)

func TestInputValueDefinitionsParser(t *testing.T) {

	g := Goblin(t)
	RegisterFailHandler(func(m string, _ ...int) { g.Fail(m) })

	g.Describe("parser.parseInputValueDefinitions", func() {
		tests := []struct {
			it                      string
			input                   string
			expectErr               types.GomegaMatcher
			expectIndex             types.GomegaMatcher
			expectParsedDefinitions types.GomegaMatcher
		}{
			{
				it:          "should parse a single InputValueDefinition",
				input:       "inputValue: Int",
				expectErr:   BeNil(),
				expectIndex: Equal([]int{0}),
				expectParsedDefinitions: Equal(ParsedDefinitions{
					InputValueDefinitions: document.InputValueDefinitions{
						{
							Name:       "inputValue",
							Directives: []int{},
							Type: document.NamedType{
								Name: "Int",
							},
						},
					},
				}.initEmptySlices()),
			},
			{
				it:          "should parse a InputValueDefinition with DefaultValue",
				input:       `inputValue: Int = 2`,
				expectErr:   BeNil(),
				expectIndex: Equal([]int{0}),
				expectParsedDefinitions: Equal(ParsedDefinitions{
					InputValueDefinitions: document.InputValueDefinitions{
						{
							Name:       "inputValue",
							Directives: []int{},
							Type: document.NamedType{
								Name: "Int",
							},
							DefaultValue: document.Value{
								ValueType: document.ValueTypeInt,
								IntValue:  2,
							},
						},
					},
				}.initEmptySlices()),
			},
			{
				it:          "should parse a InputValueDefinition with Description",
				input:       `"useful description"inputValue: Int = 2`,
				expectErr:   BeNil(),
				expectIndex: Equal([]int{0}),
				expectParsedDefinitions: Equal(ParsedDefinitions{
					InputValueDefinitions: document.InputValueDefinitions{
						{
							Description: "useful description",
							Name:        "inputValue",
							Directives:  []int{},
							Type: document.NamedType{
								Name: "Int",
							},
							DefaultValue: document.Value{
								ValueType: document.ValueTypeInt,
								IntValue:  2,
							},
						},
					},
				}.initEmptySlices()),
			},
			{
				it:          "should parse multiple InputValueDefinition",
				input:       `inputValue: Int, outputValue: String`,
				expectErr:   BeNil(),
				expectIndex: Equal([]int{0, 1}),
				expectParsedDefinitions: Equal(ParsedDefinitions{
					InputValueDefinitions: document.InputValueDefinitions{
						{
							Name:       "inputValue",
							Directives: []int{},
							Type: document.NamedType{
								Name: "Int",
							},
						},
						{
							Name:       "outputValue",
							Directives: []int{},
							Type: document.NamedType{
								Name: "String",
							},
						},
					},
				}.initEmptySlices()),
			},
			{
				it:          "should parse a  multiple InputFieldDefinitions with Descriptions",
				input:       `"this is a inputValue"inputValue: Int, "this is a outputValue"outputValue: String`,
				expectErr:   BeNil(),
				expectIndex: Equal([]int{0, 1}),
				expectParsedDefinitions: Equal(ParsedDefinitions{
					InputValueDefinitions: document.InputValueDefinitions{
						{
							Description: "this is a inputValue",
							Name:        "inputValue",
							Directives:  []int{},
							Type: document.NamedType{
								Name: "Int",
							},
						},
						{
							Description: "this is a outputValue",
							Name:        "outputValue",
							Directives:  []int{},
							Type: document.NamedType{
								Name: "String",
							},
						},
					},
				}.initEmptySlices()),
			},
			{
				it:          "should parse multiple InputFieldDefinitions with Descriptions and DefaultValues",
				input:       `"this is a inputValue"inputValue: Int = 2, "this is a outputValue"outputValue: String = "test"`,
				expectErr:   BeNil(),
				expectIndex: Equal([]int{0, 1}),
				expectParsedDefinitions: Equal(ParsedDefinitions{
					InputValueDefinitions: document.InputValueDefinitions{
						{
							Description: "this is a inputValue",
							Name:        "inputValue",
							Directives:  []int{},
							Type: document.NamedType{
								Name: "Int",
							},
							DefaultValue: document.Value{
								ValueType: document.ValueTypeInt,
								IntValue:  2,
							},
						},
						{
							Description: "this is a outputValue",
							Name:        "outputValue",
							Directives:  []int{},
							Type: document.NamedType{
								Name: "String",
							},
							DefaultValue: document.Value{
								ValueType:   document.ValueTypeString,
								StringValue: "test",
							},
						},
					},
				}.initEmptySlices()),
			},
			{
				it:          "should parse nonNull Types",
				input:       "inputValue: Int!",
				expectErr:   BeNil(),
				expectIndex: Equal([]int{0}),
				expectParsedDefinitions: Equal(ParsedDefinitions{
					InputValueDefinitions: document.InputValueDefinitions{
						{
							Name:       "inputValue",
							Directives: []int{},
							Type: document.NamedType{
								Name:    "Int",
								NonNull: true,
							},
						},
					},
				}.initEmptySlices()),
			},
			{
				it:          "should parse list Types",
				input:       "inputValue: [Int]",
				expectErr:   BeNil(),
				expectIndex: Equal([]int{0}),
				expectParsedDefinitions: Equal(ParsedDefinitions{
					InputValueDefinitions: document.InputValueDefinitions{
						{
							Name:       "inputValue",
							Directives: []int{},
							Type: document.ListType{
								Type: document.NamedType{
									Name: "Int",
								},
							},
						},
					},
				}.initEmptySlices()),
			},
			{
				it:          "should parse an InputValueDefinition with Directives",
				input:       `inputValue: Int @fromTop(to: "bottom") @fromBottom(to: "top")`,
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
					InputValueDefinitions: document.InputValueDefinitions{
						{
							Name: "inputValue",
							Type: document.NamedType{
								Name: "Int",
							},
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
				err := parser.parseInputValueDefinitions(&index, keyword.UNDEFINED)
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
