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
					Name: []byte("Person"),
					FieldsDefinition: document.FieldsDefinition{
						document.FieldDefinition{
							Name: []byte("name"),
							Type: document.NamedType{
								Name: []byte("String"),
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
					Name: []byte("Person"),
					FieldsDefinition: document.FieldsDefinition{
						document.FieldDefinition{
							Name: []byte("name"),
							Type: document.ListType{
								Type: document.NamedType{
									Name: []byte("String"),
								},
								NonNull: true,
							},
						},
						document.FieldDefinition{
							Name: []byte("age"),
							Type: document.ListType{
								Type: document.NamedType{
									Name: []byte("Int"),
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
					Name: []byte("Person"),
				}),
			},
			{
				it: "should parse a ObjectTypeDefinition implementing a single interface",
				input: `Person implements Human {
					name: String
				}`,
				expectErr: BeNil(),
				expectValues: Equal(document.ObjectTypeDefinition{
					Name:                 []byte("Person"),
					ImplementsInterfaces: document.ImplementsInterfaces{[]byte("Human")},
					FieldsDefinition: document.FieldsDefinition{
						document.FieldDefinition{
							Name: []byte("name"),
							Type: document.NamedType{
								Name: []byte("String"),
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
					Name:                 []byte("Person"),
					ImplementsInterfaces: document.ImplementsInterfaces{[]byte("Human"), []byte("Mammal")},
					FieldsDefinition: document.FieldsDefinition{
						document.FieldDefinition{
							Name: []byte("name"),
							Type: document.NamedType{
								Name: []byte("String"),
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
					Name: []byte("Person"),
					Directives: document.Directives{
						document.Directive{
							Name: []byte("fromTop"),
							Arguments: document.Arguments{
								document.Argument{
									Name: []byte("to"),
									Value: document.StringValue{
										Val: []byte("bottom"),
									},
								},
							},
						},
						document.Directive{
							Name: []byte("fromBottom"),
							Arguments: document.Arguments{
								document.Argument{
									Name: []byte("to"),
									Value: document.StringValue{
										Val: []byte("top"),
									},
								},
							},
						},
					},
					FieldsDefinition: document.FieldsDefinition{
						document.FieldDefinition{
							Name: []byte("name"),
							Type: document.NamedType{
								Name: []byte("String"),
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
