package parser

import (
	"bytes"
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
					Name: []byte("Person"),
					InputFieldsDefinition: document.InputFieldsDefinition{
						document.InputValueDefinition{
							Name: []byte("name"),
							Type: document.NamedType{
								Name: []byte("String"),
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
					Name: []byte("Person"),
					InputFieldsDefinition: document.InputFieldsDefinition{
						document.InputValueDefinition{
							Name: []byte("name"),
							Type: document.ListType{
								Type: document.NamedType{
									Name: []byte("String"),
								},
								NonNull: true,
							},
						},
						document.InputValueDefinition{
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
				it: "should parse a simple InputObjectTypeDefinition containing a DefaultValue",
				input: `Person {
					name: String = "Gophina"
				}`,
				expectErr: BeNil(),
				expectValues: Equal(document.InputObjectTypeDefinition{
					Name: []byte("Person"),
					InputFieldsDefinition: document.InputFieldsDefinition{
						document.InputValueDefinition{
							Name: []byte("name"),
							DefaultValue: document.StringValue{
								Val: []byte("Gophina"),
							},
							Type: document.NamedType{
								Name: []byte("String"),
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
					Name: []byte("Person"),
				}),
			},
			{
				it: "should parse an InputObjectTypeDefinition with Directives",
				input: `Person @fromTop(to: "bottom") @fromBottom(to: "top"){
					name: String
				}`,
				expectErr: BeNil(),
				expectValues: Equal(document.InputObjectTypeDefinition{
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
					InputFieldsDefinition: document.InputFieldsDefinition{
						document.InputValueDefinition{
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

				val, err := parser.parseInputObjectTypeDefinition()
				Expect(err).To(test.expectErr)
				Expect(val).To(test.expectValues)
			})
		}
	})
}
