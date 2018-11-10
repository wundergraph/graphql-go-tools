package parser

import (
	"bytes"
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
			it           string
			input        string
			expectErr    types.GomegaMatcher
			expectValues types.GomegaMatcher
		}{
			{
				it: "should parse a simple ObjectTypeDefinition",
				input: `Person {
					name: String
				}`,
				expectErr: BeNil(),
				expectValues: Equal(document.ObjectTypeDefinition{
					Name: "Person",
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
				it: "should parse an ObjectTypeDefinition with multiple FieldDefinition",
				input: `Person {
					name: [String]!
					age: [ Int ]
				}`,
				expectErr: BeNil(),
				expectValues: Equal(document.ObjectTypeDefinition{
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
				it:        "should parse an ObjectTypeDefinition with optional FieldsDefinition",
				input:     `Person `,
				expectErr: BeNil(),
				expectValues: Equal(document.ObjectTypeDefinition{
					Name: "Person",
				}),
			},
			{
				it: "should parse a ObjectTypeDefinition implementing a single interface",
				input: `Person implements Human {
					name: String
				}`,
				expectErr: BeNil(),
				expectValues: Equal(document.ObjectTypeDefinition{
					Name:                 "Person",
					ImplementsInterfaces: document.ImplementsInterfaces{"Human"},
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
				it: "should parse a ObjectTypeDefinition implementing a multiple interfaces",
				input: `Person implements Human & Mammal {
					name: String
				}`,
				expectErr: BeNil(),
				expectValues: Equal(document.ObjectTypeDefinition{
					Name:                 "Person",
					ImplementsInterfaces: document.ImplementsInterfaces{"Human", "Mammal"},
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
				it: "should parse an ObjectTypeDefinition with Directives",
				input: `Person @fromTop(to: "bottom") @fromBottom(to: "top") {
					name: String
				}`,
				expectErr: BeNil(),
				expectValues: Equal(document.ObjectTypeDefinition{
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

				reader := bytes.NewReader([]byte(test.input))
				parser := NewParser()
				parser.l.SetInput(reader)

				val, err := parser.parseObjectTypeDefinition()
				Expect(err).To(test.expectErr)
				Expect(val).To(test.expectValues)
			})
		}
	})
}
