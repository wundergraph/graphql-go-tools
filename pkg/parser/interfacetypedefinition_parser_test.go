package parser

import (
	. "github.com/franela/goblin"
	"github.com/jensneuse/graphql-go-tools/pkg/document"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/types"
	"testing"
)

func TestInterfaceTypeDefinitionParser(t *testing.T) {

	g := Goblin(t)
	RegisterFailHandler(func(m string, _ ...int) { g.Fail(m) })

	g.Describe("parser.parseInterfaceTypeDefinition", func() {

		tests := []struct {
			it                      string
			input                   string
			expectErr               types.GomegaMatcher
			expectIndex             types.GomegaMatcher
			expectParsedDefinitions types.GomegaMatcher
		}{
			{
				it: "should parse a simple InterfaceTypeDefinition",
				input: `NameEntity {
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
					InterfaceTypeDefinitions: document.InterfaceTypeDefinitions{
						{
							Name:             "NameEntity",
							FieldsDefinition: []int{0},
							Directives:       []int{},
						},
					},
				}.initEmptySlices()),
			},
			{
				it: "should parse an InterfaceTypeDefinition with multiple FieldDefinition",
				input: `Person {
					name: [String]!
					age: [ Int ]
				}`,
				expectErr:   BeNil(),
				expectIndex: Equal([]int{0}),
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
							Type: document.ListType{
								Type: document.NamedType{
									Name: "Int",
								},
							},
						},
					},
					InterfaceTypeDefinitions: document.InterfaceTypeDefinitions{
						{
							Name:             "Person",
							FieldsDefinition: []int{0, 1},
							Directives:       []int{},
						},
					},
				}.initEmptySlices()),
			},
			{
				it:          "should parse an InterfaceTypeDefinition with optional FieldDefinitions",
				input:       `Person `,
				expectErr:   BeNil(),
				expectIndex: Equal([]int{0}),
				expectParsedDefinitions: Equal(ParsedDefinitions{
					InterfaceTypeDefinitions: document.InterfaceTypeDefinitions{
						{
							Name:             "Person",
							Directives:       []int{},
							FieldsDefinition: []int{},
						},
					},
				}.initEmptySlices()),
			},
			{
				it: "should parse an InterfaceTypeDefinition with Directives",
				input: `NameEntity @fromTop(to: "bottom") @fromBottom(to: "top") {
					name: String
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
						{
							Name:      "fromTop",
							Arguments: []int{0},
						},
						{
							Name:      "fromBottom",
							Arguments: []int{1},
						},
					},
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
					InterfaceTypeDefinitions: document.InterfaceTypeDefinitions{
						{
							Name:             "NameEntity",
							Directives:       []int{0, 1},
							FieldsDefinition: []int{0},
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
				err := parser.parseInterfaceTypeDefinition(&index)
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
