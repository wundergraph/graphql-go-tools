package parser

import (
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
						Name: "name",
						Type: document.NamedType{
							Name: "String",
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
						Name: "name",
						Type: document.NamedType{
							Name: "String",
						},
					},
					{
						Name: "age",
						Type: document.NamedType{
							Name: "Int",
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
						Description: "describes the name",
						Name:        "name",
						Type: document.NamedType{
							Name: "String",
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
						Name: "name",
						Type: document.ListType{
							Type: document.NamedType{
								Name: "String",
							},
							NonNull: true,
						},
					},
					{
						Name: "age",
						Type: document.NamedType{
							Name:    "Int",
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
						Name: "name",
						Type: document.NamedType{
							Name: "String",
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
						Name: "name",
						Type: document.NamedType{
							Name: "String",
						},
						ArgumentsDefinition: document.ArgumentsDefinition{
							document.InputValueDefinition{
								Name: "isSet",
								Type: document.NamedType{
									Name:    "boolean",
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
						Name: "name",
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
						Type: document.NamedType{
							Name: "String",
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

				val, err := parser.parseFieldsDefinition()
				Expect(err).To(test.expectErr)
				Expect(val).To(test.expectValues)
			})
		}
	})
}
