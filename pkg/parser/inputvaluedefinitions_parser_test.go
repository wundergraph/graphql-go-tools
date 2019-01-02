package parser

import (
	. "github.com/franela/goblin"
	"github.com/jensneuse/graphql-go-tools/pkg/document"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/types"
	"testing"
)

func TestInputValueDefinitionsParser(t *testing.T) {

	g := Goblin(t)
	RegisterFailHandler(func(m string, _ ...int) { g.Fail(m) })

	g.Describe("parser.parseInputValueDefinitions", func() {
		tests := []struct {
			it           string
			input        string
			expectErr    types.GomegaMatcher
			expectValues types.GomegaMatcher
		}{
			{
				it:        "should parse a single InputValueDefinition",
				input:     "inputValue: Int",
				expectErr: BeNil(),
				expectValues: Equal([]document.InputValueDefinition{
					{
						Name: "inputValue",
						Type: document.NamedType{
							Name: "Int",
						},
					},
				}),
			},
			{
				it:        "should parse a InputValueDefinition with DefaultValue",
				input:     `inputValue: Int = 2`,
				expectErr: BeNil(),
				expectValues: Equal([]document.InputValueDefinition{
					{
						Name: "inputValue",
						Type: document.NamedType{
							Name: "Int",
						},
						DefaultValue: document.IntValue{
							Val: 2,
						},
					},
				}),
			},
			{
				it:        "should parse a InputValueDefinition with Description",
				input:     `"useful description"inputValue: Int = 2`,
				expectErr: BeNil(),
				expectValues: Equal([]document.InputValueDefinition{
					{
						Description: "useful description",
						Name:        "inputValue",
						Type: document.NamedType{
							Name: "Int",
						},
						DefaultValue: document.IntValue{
							Val: 2,
						},
					},
				}),
			},
			{
				it:        "should parse multiple InputValueDefinition",
				input:     `inputValue: Int, outputValue: String`,
				expectErr: BeNil(),
				expectValues: Equal([]document.InputValueDefinition{
					{
						Name: "inputValue",
						Type: document.NamedType{
							Name: "Int",
						},
					},
					{
						Name: "outputValue",
						Type: document.NamedType{
							Name: "String",
						},
					},
				}),
			},
			{
				it:        "should parse a  multiple InputFieldDefinitions with Descriptions",
				input:     `"this is a inputValue"inputValue: Int, "this is a outputValue"outputValue: String`,
				expectErr: BeNil(),
				expectValues: Equal([]document.InputValueDefinition{
					{
						Description: "this is a inputValue",
						Name:        "inputValue",
						Type: document.NamedType{
							Name: "Int",
						},
					},
					{
						Description: "this is a outputValue",
						Name:        "outputValue",
						Type: document.NamedType{
							Name: "String",
						},
					},
				}),
			},
			{
				it:        "should parse multiple InputFieldDefinitions with Descriptions and DefaultValues",
				input:     `"this is a inputValue"inputValue: Int = 2, "this is a outputValue"outputValue: String = "test"`,
				expectErr: BeNil(),
				expectValues: Equal([]document.InputValueDefinition{
					{
						Description: "this is a inputValue",
						Name:        "inputValue",
						Type: document.NamedType{
							Name: "Int",
						},
						DefaultValue: document.IntValue{
							Val: 2,
						},
					},
					{
						Description: "this is a outputValue",
						Name:        "outputValue",
						Type: document.NamedType{
							Name: "String",
						},
						DefaultValue: document.StringValue{
							Val: "test",
						},
					},
				}),
			},
			{
				it:        "should parse nonNull Types",
				input:     "inputValue: Int!",
				expectErr: BeNil(),
				expectValues: Equal([]document.InputValueDefinition{
					{
						Name: "inputValue",
						Type: document.NamedType{
							Name:    "Int",
							NonNull: true,
						},
					},
				}),
			},
			{
				it:        "should parse list Types",
				input:     "inputValue: [Int]",
				expectErr: BeNil(),
				expectValues: Equal([]document.InputValueDefinition{
					{
						Name: "inputValue",
						Type: document.ListType{
							Type: document.NamedType{
								Name: "Int",
							},
						},
					},
				}),
			},
			{
				it:        "should parse an InputValueDefinition with Directives",
				input:     `inputValue: Int @fromTop(to: "bottom") @fromBottom(to: "top")`,
				expectErr: BeNil(),
				expectValues: Equal([]document.InputValueDefinition{
					{
						Name: "inputValue",
						Type: document.NamedType{
							Name: "Int",
						},
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
				}),
			},
		}

		for _, test := range tests {
			test := test

			g.It(test.it, func() {

				parser := NewParser()
				parser.l.SetInput(test.input)

				val, err := parser.parseInputValueDefinitions()
				Expect(err).To(test.expectErr)
				Expect(val).To(test.expectValues)
			})
		}
	})
}
