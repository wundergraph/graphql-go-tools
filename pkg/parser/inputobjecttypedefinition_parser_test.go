package parser

import (
	. "github.com/franela/goblin"
	"github.com/jensneuse/graphql-go-tools/pkg/document"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/types"
	"testing"
)

func TestInputObjectTypeDefinitionParser(t *testing.T) {

	g := Goblin(t)
	RegisterFailHandler(func(m string, _ ...int) { g.Fail(m) })

	g.Describe("parser.parseInputObjectTypeDefinition", func() {

		tests := []struct {
			it                      string
			input                   string
			expectErr               types.GomegaMatcher
			expectParsedDefinitions types.GomegaMatcher
			expectIndex             types.GomegaMatcher
		}{
			{
				it: "should parse a simple InputObjectTypeDefinition",
				input: `Person {
					name: String
				}`,
				expectErr:   BeNil(),
				expectIndex: Equal([]int{0}),
				expectParsedDefinitions: Equal(ParsedDefinitions{
					InputObjectTypeDefinitions: document.InputObjectTypeDefinitions{
						{
							Name: "Person",
							InputFieldsDefinition: []int{0},
							Directives:            []int{},
						},
					},
					InputValueDefinitions: document.InputValueDefinitions{
						{
							Name:       "name",
							Directives: []int{},
							Type: document.NamedType{
								Name: "String",
							},
						},
					},
				}.initEmptySlices()),
			},
			{
				it: "should parse an InputObjectTypeDefinition with multiple InputValueDefinition",
				input: `Person {
					name: [String]!
					age: [ Int ]
				}`,
				expectErr:   BeNil(),
				expectIndex: Equal([]int{0}),
				expectParsedDefinitions: Equal(ParsedDefinitions{
					InputObjectTypeDefinitions: document.InputObjectTypeDefinitions{
						{
							Name: "Person",
							InputFieldsDefinition: []int{0, 1},
							Directives:            []int{},
						},
					},
					InputValueDefinitions: document.InputValueDefinitions{
						{
							Name:       "name",
							Directives: []int{},
							Type: document.ListType{
								Type: document.NamedType{
									Name: "String",
								},
								NonNull: true,
							},
						},
						{
							Name:       "age",
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
				it: "should parse a simple InputObjectTypeDefinition containing a DefaultValue",
				input: `Person {
					name: String = "Gophina"
				}`,
				expectErr:   BeNil(),
				expectIndex: Equal([]int{0}),
				expectParsedDefinitions: Equal(ParsedDefinitions{
					InputObjectTypeDefinitions: document.InputObjectTypeDefinitions{
						{
							Name: "Person",
							InputFieldsDefinition: []int{0},
							Directives:            []int{},
						},
					},
					InputValueDefinitions: document.InputValueDefinitions{
						{
							Name:       "name",
							Directives: []int{},
							DefaultValue: document.Value{
								ValueType:   document.ValueTypeString,
								StringValue: "Gophina",
							},
							Type: document.NamedType{
								Name: "String",
							},
						},
					},
				}.initEmptySlices()),
			},
			{
				it:          "should parse an InputObjectTypeDefinition with optional InputValueDefinitions",
				input:       `Person `,
				expectErr:   BeNil(),
				expectIndex: Equal([]int{0}),
				expectParsedDefinitions: Equal(ParsedDefinitions{
					InputObjectTypeDefinitions: document.InputObjectTypeDefinitions{
						{
							Name:                  "Person",
							Directives:            []int{},
							InputFieldsDefinition: []int{},
						},
					},
					InputValueDefinitions: document.InputValueDefinitions{},
				}.initEmptySlices()),
			},
			{
				it: "should parse an InputObjectTypeDefinition with Directives",
				input: `Person @fromTop(to: "bottom") @fromBottom(to: "top"){
					name: String
				}`,
				expectErr:   BeNil(),
				expectIndex: Equal([]int{0}),
				expectParsedDefinitions: Equal(ParsedDefinitions{
					InputObjectTypeDefinitions: document.InputObjectTypeDefinitions{
						{
							Name:                  "Person",
							Directives:            []int{0, 1},
							InputFieldsDefinition: []int{0},
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
					EnumValuesDefinitions: document.EnumValueDefinitions{},
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
					InputValueDefinitions: document.InputValueDefinitions{
						{
							Name:       "name",
							Directives: []int{},
							Type: document.NamedType{
								Name: "String",
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

				var index []int
				err := parser.parseInputObjectTypeDefinition(&index)
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
