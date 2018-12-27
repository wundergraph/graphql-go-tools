package parser

import (
	"bytes"
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
			it           string
			input        string
			expectErr    types.GomegaMatcher
			expectValues types.GomegaMatcher
		}{
			{
				it: "should parse a simple FieldsDefinition",
				input: `{
					name: String
				}`,
				expectErr: BeNil(),
				expectValues: Equal(document.FieldsDefinition{
					{
						Name: []byte("name"),
						Type: document.NamedType{
							Name: []byte("String"),
						},
					},
				}),
			},
			{
				it: "should parse FieldsDefinition with multiple FieldDefinitions",
				input: `{
					name: String
					age: Int
				}`,
				expectErr: BeNil(),
				expectValues: Equal(document.FieldsDefinition{
					{
						Name: []byte("name"),
						Type: document.NamedType{
							Name: []byte("String"),
						},
					},
					{
						Name: []byte("age"),
						Type: document.NamedType{
							Name: []byte("Int"),
						},
					},
				}),
			},
			{
				it: "should parse FieldsDefinition with a Description on a FieldDefinition",
				input: `{
					"describes the name"
					name: String
				}`,
				expectErr: BeNil(),
				expectValues: Equal(document.FieldsDefinition{
					{
						Description: []byte("describes the name"),
						Name:        []byte("name"),
						Type: document.NamedType{
							Name: []byte("String"),
						},
					},
				}),
			},
			{
				it: "should parse FieldsDefinition with multiple FieldDefinitions and special Types",
				input: `{
					name: [ String ]!
					age: Int!
				}`,
				expectErr: BeNil(),
				expectValues: Equal(document.FieldsDefinition{
					{
						Name: []byte("name"),
						Type: document.ListType{
							Type: document.NamedType{
								Name: []byte("String"),
							},
							NonNull: true,
						},
					},
					{
						Name: []byte("age"),
						Type: document.NamedType{
							Name:    []byte("Int"),
							NonNull: true,
						},
					},
				}),
			},
			{
				it: "should return empty when no bracket open (FieldsDefinition can be optional)",
				input: `
					name: String
				}`,
				expectErr:    BeNil(),
				expectValues: Equal(document.FieldsDefinition(nil)),
			},
			{
				it: "should not parse FieldsDefinition when multiple brackets open",
				input: `{{
					name: String
				}`,
				expectErr:    Not(BeNil()),
				expectValues: Equal(document.FieldsDefinition(nil)),
			},
			{
				it: "should not parse FieldsDefinition when no bracket close",
				input: `{
					name: String
				`,
				expectErr: Not(BeNil()),
				expectValues: Equal(document.FieldsDefinition{
					{
						Name: []byte("name"),
						Type: document.NamedType{
							Name: []byte("String"),
						},
					},
				}),
			},
			{
				it: "should parse FieldsDefinition with FieldDefinition containing an ArgumentsDefinition",
				input: `{
					name(isSet: boolean!): String
				}`,
				expectErr: BeNil(),
				expectValues: Equal(document.FieldsDefinition{
					{
						Name: []byte("name"),
						Type: document.NamedType{
							Name: []byte("String"),
						},
						ArgumentsDefinition: document.ArgumentsDefinition{
							document.InputValueDefinition{
								Name: []byte("isSet"),
								Type: document.NamedType{
									Name:    []byte("boolean"),
									NonNull: true,
								},
							},
						},
					},
				}),
			},
			{
				it: "should parse a FieldsDefinition with Directives",
				input: `{
					name: String @fromTop(to: "bottom") @fromBottom(to: "top") 
				}`,
				expectErr: BeNil(),
				expectValues: Equal(document.FieldsDefinition{
					{
						Name: []byte("name"),
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
						Type: document.NamedType{
							Name: []byte("String"),
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

				val, err := parser.parseFieldsDefinition()
				Expect(err).To(test.expectErr)
				Expect(val).To(test.expectValues)
			})
		}
	})
}
