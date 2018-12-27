package parser

import (
	"bytes"
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
					Name: []byte("NameEntity"),
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
				it: "should parse an InterfaceTypeDefinition with multiple FieldDefinition",
				input: `Person {
					name: [String]!
					age: [ Int ]
				}`,
				expectErr: BeNil(),
				expectValues: Equal(document.InterfaceTypeDefinition{
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
				it:        "should parse an InterfaceTypeDefinition with optional FieldsDefinition",
				input:     `Person `,
				expectErr: BeNil(),
				expectValues: Equal(document.InterfaceTypeDefinition{
					Name: []byte("Person"),
				}),
			},
			{
				it: "should parse an InterfaceTypeDefinition with Directives",
				input: `NameEntity @fromTop(to: "bottom") @fromBottom(to: "top") {
					name: String
				}`,
				expectErr: BeNil(),
				expectValues: Equal(document.InterfaceTypeDefinition{
					Name: []byte("NameEntity"),
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

				val, err := parser.parseInterfaceTypeDefinition()
				Expect(err).To(test.expectErr)
				Expect(val).To(test.expectValues)
			})
		}
	})
}
