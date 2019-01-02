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
			it           string
			input        string
			expectErr    types.GomegaMatcher
			expectValues types.GomegaMatcher
		}{
			{
				it: "should parse a simple InputObjectTypeDefinition",
				input: `Person {
					name: String
				}`,
				expectErr: BeNil(),
				expectValues: Equal(document.InputObjectTypeDefinition{
					Name: "Person",
					InputFieldsDefinition: document.InputFieldsDefinition{
						document.InputValueDefinition{
							Name: "name",
							Type: document.NamedType{
								Name: "String",
							},
						},
					},
				}),
			},
			{
				it: "should parse an InputObjectTypeDefinition with multiple InputValueDefinition",
				input: `Person {
					name: [String]!
					age: [ Int ]
				}`,
				expectErr: BeNil(),
				expectValues: Equal(document.InputObjectTypeDefinition{
					Name: "Person",
					InputFieldsDefinition: document.InputFieldsDefinition{
						document.InputValueDefinition{
							Name: "name",
							Type: document.ListType{
								Type: document.NamedType{
									Name: "String",
								},
								NonNull: true,
							},
						},
						document.InputValueDefinition{
							Name: "age",
							Type: document.ListType{
								Type: document.NamedType{
									Name: "Int",
								},
							},
						},
					},
				}),
			},
			{
				it: "should parse a simple InputObjectTypeDefinition containing a DefaultValue",
				input: `Person {
					name: String = "Gophina"
				}`,
				expectErr: BeNil(),
				expectValues: Equal(document.InputObjectTypeDefinition{
					Name: "Person",
					InputFieldsDefinition: document.InputFieldsDefinition{
						document.InputValueDefinition{
							Name: "name",
							DefaultValue: document.StringValue{
								Val: "Gophina",
							},
							Type: document.NamedType{
								Name: "String",
							},
						},
					},
				}),
			},
			{
				it:        "should parse an InputObjectTypeDefinition with optional InputFieldsDefinition",
				input:     `Person `,
				expectErr: BeNil(),
				expectValues: Equal(document.InputObjectTypeDefinition{
					Name: "Person",
				}),
			},
			{
				it: "should parse an InputObjectTypeDefinition with Directives",
				input: `Person @fromTop(to: "bottom") @fromBottom(to: "top"){
					name: String
				}`,
				expectErr: BeNil(),
				expectValues: Equal(document.InputObjectTypeDefinition{
					Name: "Person",
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
					InputFieldsDefinition: document.InputFieldsDefinition{
						document.InputValueDefinition{
							Name: "name",
							Type: document.NamedType{
								Name: "String",
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

				val, err := parser.parseInputObjectTypeDefinition()
				Expect(err).To(test.expectErr)
				Expect(val).To(test.expectValues)
			})
		}
	})
}
