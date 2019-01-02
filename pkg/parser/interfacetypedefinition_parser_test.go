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
			it           string
			input        string
			expectErr    types.GomegaMatcher
			expectValues types.GomegaMatcher
		}{
			{
				it: "should parse a simple InterfaceTypeDefinition",
				input: `NameEntity {
					name: String
				}`,
				expectErr: BeNil(),
				expectValues: Equal(document.InterfaceTypeDefinition{
					Name: "NameEntity",
					FieldsDefinition: document.FieldsDefinition{
						document.FieldDefinition{
							Name: "name",
							Type: document.NamedType{
								Name: "String",
							},
						},
					},
				}),
			},
			{
				it: "should parse an InterfaceTypeDefinition with multiple FieldDefinition",
				input: `Person {
					name: [String]!
					age: [ Int ]
				}`,
				expectErr: BeNil(),
				expectValues: Equal(document.InterfaceTypeDefinition{
					Name: "Person",
					FieldsDefinition: document.FieldsDefinition{
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
				}),
			},
			{
				it:        "should parse an InterfaceTypeDefinition with optional FieldsDefinition",
				input:     `Person `,
				expectErr: BeNil(),
				expectValues: Equal(document.InterfaceTypeDefinition{
					Name: "Person",
				}),
			},
			{
				it: "should parse an InterfaceTypeDefinition with Directives",
				input: `NameEntity @fromTop(to: "bottom") @fromBottom(to: "top") {
					name: String
				}`,
				expectErr: BeNil(),
				expectValues: Equal(document.InterfaceTypeDefinition{
					Name: "NameEntity",
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
					FieldsDefinition: document.FieldsDefinition{
						document.FieldDefinition{
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

				val, err := parser.parseInterfaceTypeDefinition()
				Expect(err).To(test.expectErr)
				Expect(val).To(test.expectValues)
			})
		}
	})
}
