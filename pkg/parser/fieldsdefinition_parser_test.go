package parser

import (
	. "github.com/franela/goblin"
	"github.com/jensneuse/graphql-go-tools/pkg/document"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/types"
	"testing"
)

func TestFieldsDefinitionParser(t *testing.T) {

	g := Goblin(t)
	RegisterFailHandler(func(m string, _ ...int) { g.Fail(m) })

	g.Describe("parser.parseFieldsDefinition", func() {

		tests := []struct {
			it                      string
			input                   string
			expectErr               types.GomegaMatcher
			expectIndex             types.GomegaMatcher
			expectParsedDefinitions types.GomegaMatcher
		}{
			{
				it: "should parse a simple FieldDefinitions",
				input: `{
					name: String
				}`,
				expectErr:   BeNil(),
				expectIndex: Equal([]int{0}),
				expectParsedDefinitions: Equal(ParsedDefinitions{
					FieldDefinitions: document.FieldDefinitions{
						{
							Name:                "name",
							Directives:          []int{},
							ArgumentsDefinition: []int{},
							Type: document.NamedType{
								Name: "String",
							},
						},
					},
				}.initEmptySlices()),
			},
			{
				it: "should parse FieldDefinitions with multiple FieldDefinitions",
				input: `{
					name: String
					age: Int
				}`,
				expectErr:   BeNil(),
				expectIndex: Equal([]int{0, 1}),
				expectParsedDefinitions: Equal(ParsedDefinitions{
					FieldDefinitions: document.FieldDefinitions{
						{
							Name:                "name",
							Directives:          []int{},
							ArgumentsDefinition: []int{},
							Type: document.NamedType{
								Name: "String",
							},
						},
						{
							Name:                "age",
							Directives:          []int{},
							ArgumentsDefinition: []int{},
							Type: document.NamedType{
								Name: "Int",
							},
						},
					},
				}.initEmptySlices()),
			},
			{
				it: "should parse FieldDefinitions with a Description on a FieldDefinition",
				input: `{
					"describes the name"
					name: String
				}`,
				expectErr:   BeNil(),
				expectIndex: Equal([]int{0}),
				expectParsedDefinitions: Equal(ParsedDefinitions{
					FieldDefinitions: document.FieldDefinitions{
						{
							Description:         "describes the name",
							Name:                "name",
							Directives:          []int{},
							ArgumentsDefinition: []int{},
							Type: document.NamedType{
								Name: "String",
							},
						},
					},
				}.initEmptySlices()),
			},
			{
				it: "should parse FieldDefinitions with multiple FieldDefinitions and special Types",
				input: `{
					name: [ String ]!
					age: Int!
				}`,
				expectErr:   BeNil(),
				expectIndex: Equal([]int{0, 1}),
				expectParsedDefinitions: Equal(ParsedDefinitions{
					FieldDefinitions: document.FieldDefinitions{
						{
							Name:                "name",
							Directives:          []int{},
							ArgumentsDefinition: []int{},
							Type: document.ListType{
								Type: document.NamedType{
									Name: "String",
								},
								NonNull: true,
							},
						},
						{
							Name:                "age",
							Directives:          []int{},
							ArgumentsDefinition: []int{},
							Type: document.NamedType{
								Name:    "Int",
								NonNull: true,
							},
						},
					},
				}.initEmptySlices()),
			},
			{
				it: "should return empty when no bracket open (FieldDefinitions can be optional)",
				input: `
					name: String
				}`,
				expectErr:               BeNil(),
				expectIndex:             Equal([]int{}),
				expectParsedDefinitions: Equal(ParsedDefinitions{}.initEmptySlices()),
			},
			{
				it: "should not parse FieldDefinitions when multiple brackets open",
				input: `{{
					name: String
				}`,
				expectErr:               HaveOccurred(),
				expectIndex:             Equal([]int{}),
				expectParsedDefinitions: Equal(ParsedDefinitions{}.initEmptySlices()),
			},
			{
				it: "should not parse FieldDefinitions when no bracket close",
				input: `{
					name: String
				`,
				expectErr:   HaveOccurred(),
				expectIndex: Equal([]int{0}),
				expectParsedDefinitions: Equal(ParsedDefinitions{
					FieldDefinitions: document.FieldDefinitions{
						{
							Name:                "name",
							Directives:          []int{},
							ArgumentsDefinition: []int{},
							Type: document.NamedType{
								Name: "String",
							},
						},
					},
				}.initEmptySlices()),
			},
			{
				it: "should parse FieldDefinitions with FieldDefinition containing an ArgumentsDefinition",
				input: `{
					name(isSet: boolean!): String
				}`,
				expectErr:   BeNil(),
				expectIndex: Equal([]int{0}),
				expectParsedDefinitions: Equal(ParsedDefinitions{
					InputValueDefinitions: document.InputValueDefinitions{
						{
							Name:       "isSet",
							Directives: []int{},
							Type: document.NamedType{
								Name:    "boolean",
								NonNull: true,
							},
						},
					},
					FieldDefinitions: document.FieldDefinitions{
						{
							Name: "name",
							Type: document.NamedType{
								Name: "String",
							},
							Directives:          []int{},
							ArgumentsDefinition: []int{0},
						},
					},
				}.initEmptySlices()),
			},
			{
				it: "should parse a FieldDefinitions with Directives",
				input: `{
					name: String @fromTop(to: "bottom") @fromBottom(to: "top") 
				}`,
				expectErr:   BeNil(),
				expectIndex: Equal([]int{0}),
				expectParsedDefinitions: Equal(ParsedDefinitions{
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
					FieldDefinitions: document.FieldDefinitions{
						{
							Name:                "name",
							Directives:          []int{0, 1},
							ArgumentsDefinition: []int{},
							Type: document.NamedType{
								Name: "String",
							},
						},
					},
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
				}.initEmptySlices()),
			},
		}

		for _, test := range tests {
			test := test

			g.It(test.it, func() {

				parser := NewParser()
				parser.l.SetInput(test.input)

				index := []int{}
				err := parser.parseFieldsDefinition(&index)
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
