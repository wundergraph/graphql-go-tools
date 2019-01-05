package parser

import (
	. "github.com/franela/goblin"
	"github.com/jensneuse/graphql-go-tools/pkg/document"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/types"
	"testing"
)

func TestObjectTypeDefinitionParser(t *testing.T) {

	g := Goblin(t)
	RegisterFailHandler(func(m string, _ ...int) { g.Fail(m) })

	g.Describe("parser.parseObjectTypeDefinition", func() {

		tests := []struct {
			it                      string
			input                   string
			expectErr               types.GomegaMatcher
			expectIndex             types.GomegaMatcher
			expectParsedDefinitions types.GomegaMatcher
		}{
			{
				it: "should parse a simple ObjectTypeDefinition",
				input: `Person {
					name: String
				}`,
				expectErr:   BeNil(),
				expectIndex: Equal([]int{0}),
				expectParsedDefinitions: Equal(ParsedDefinitions{
					FieldDefinitions: document.FieldDefinitions{
						{
							Name: "name",
							Type: document.NamedType{
								Name: "String",
							},
						},
					},
					ObjectTypeDefinitions: document.ObjectTypeDefinitions{
						{
							Name:             "Person",
							FieldsDefinition: []int{0},
						},
					},
				}.initEmptySlices()),
			},
			{
				it: "should parse an ObjectTypeDefinition with multiple FieldDefinition",
				input: `Person {
					name: [String]!
					age: [ Int ]
				}`,
				expectErr:   BeNil(),
				expectIndex: Equal([]int{0}),
				expectParsedDefinitions: Equal(ParsedDefinitions{
					FieldDefinitions: document.FieldDefinitions{
						document.FieldDefinition{
							Name: "name",
							Type: document.ListType{
								Type: document.NamedType{
									Name: "String",
								},
								NonNull: true,
							},
						},
						document.FieldDefinition{
							Name: "age",
							Type: document.ListType{
								Type: document.NamedType{
									Name: "Int",
								},
							},
						},
					},
					ObjectTypeDefinitions: document.ObjectTypeDefinitions{
						{
							Name:             "Person",
							FieldsDefinition: []int{0, 1},
						},
					},
				}.initEmptySlices()),
			},
			{
				it:          "should parse an ObjectTypeDefinition with optional FieldDefinitions",
				input:       `Person `,
				expectErr:   BeNil(),
				expectIndex: Equal([]int{0}),
				expectParsedDefinitions: Equal(ParsedDefinitions{
					ObjectTypeDefinitions: document.ObjectTypeDefinitions{
						{
							Name: "Person",
						},
					},
				}.initEmptySlices()),
			},
			{
				it: "should parse a ObjectTypeDefinition implementing a single interface",
				input: `Person implements Human {
					name: String
				}`,
				expectErr:   BeNil(),
				expectIndex: Equal([]int{0}),
				expectParsedDefinitions: Equal(ParsedDefinitions{
					FieldDefinitions: document.FieldDefinitions{
						document.FieldDefinition{
							Name: "name",
							Type: document.NamedType{
								Name: "String",
							},
						},
					},
					ObjectTypeDefinitions: document.ObjectTypeDefinitions{
						{
							Name:                 "Person",
							ImplementsInterfaces: document.ImplementsInterfaces{"Human"},
							FieldsDefinition:     []int{0},
						},
					},
				}.initEmptySlices()),
			},
			{
				it: "should parse a ObjectTypeDefinition implementing a multiple interfaces",
				input: `Person implements Human & Mammal {
					name: String
				}`,
				expectErr:   BeNil(),
				expectIndex: Equal([]int{0}),
				expectParsedDefinitions: Equal(ParsedDefinitions{
					FieldDefinitions: document.FieldDefinitions{
						document.FieldDefinition{
							Name: "name",
							Type: document.NamedType{
								Name: "String",
							},
						},
					},
					ObjectTypeDefinitions: document.ObjectTypeDefinitions{
						{
							Name:                 "Person",
							ImplementsInterfaces: document.ImplementsInterfaces{"Human", "Mammal"},
							FieldsDefinition:     []int{0},
						},
					},
				}.initEmptySlices()),
			},
			{
				it: "should parse an ObjectTypeDefinition with Directives",
				input: `Person @fromTop(to: "bottom") @fromBottom(to: "top") {
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
					FieldDefinitions: document.FieldDefinitions{
						{
							Name: "name",
							Type: document.NamedType{
								Name: "String",
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
					ObjectTypeDefinitions: document.ObjectTypeDefinitions{
						{
							Name:             "Person",
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
				err := parser.parseObjectTypeDefinition(&index)
				Expect(err).To(test.expectErr)

			})
		}
	})
}
