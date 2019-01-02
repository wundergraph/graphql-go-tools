package parser

import (
	. "github.com/franela/goblin"
	"github.com/jensneuse/graphql-go-tools/pkg/document"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/types"
	"testing"
)

func TestVariableDefinitionsParser(t *testing.T) {

	g := Goblin(t)
	RegisterFailHandler(func(m string, _ ...int) { g.Fail(m) })

	g.Describe("parser.parseVariableDefinitions", func() {

		tests := []struct {
			it           string
			input        string
			expectErr    types.GomegaMatcher
			expectValues types.GomegaMatcher
		}{
			{
				it:        "should parse a simple, single VariableDefinition",
				input:     "($foo : bar!)",
				expectErr: BeNil(),
				expectValues: Equal(document.VariableDefinitions{
					document.VariableDefinition{
						Variable: "foo",
						Type: document.NamedType{
							Name:    "bar",
							NonNull: true,
						},
					},
				}),
			},
			{
				it:        "should parse a simple, single nullable VariableDefinition",
				input:     "($color: String)",
				expectErr: BeNil(),
				expectValues: Equal(document.VariableDefinitions{
					document.VariableDefinition{
						Variable: "color",
						Type: document.NamedType{
							Name:    "String",
							NonNull: false,
						},
					},
				}),
			},
			{
				it:        "should parse simple VariableDefinitions",
				input:     "($foo : bar $baz : bax)",
				expectErr: BeNil(),
				expectValues: Equal(document.VariableDefinitions{
					document.VariableDefinition{
						Variable: "foo",
						Type: document.NamedType{
							Name: "bar",
						},
					},
					document.VariableDefinition{
						Variable: "baz",
						Type: document.NamedType{
							Name: "bax",
						},
					},
				}),
			},
			{
				it:        "should parse simple VariableDefinitions with ListType between",
				input:     "($foo : [bar] $baz : bax)",
				expectErr: BeNil(),
				expectValues: Equal(document.VariableDefinitions{
					document.VariableDefinition{
						Variable: "foo",
						Type: document.ListType{Type: document.NamedType{
							Name: "bar",
						}},
					},
					document.VariableDefinition{
						Variable: "baz",
						Type: document.NamedType{
							Name: "bax",
						},
					},
				}),
			},
			{
				it:        "should parse simple VariableDefinitions with NonNullType between",
				input:     "($foo : bar! $baz : bax)",
				expectErr: BeNil(),
				expectValues: Equal(document.VariableDefinitions{
					document.VariableDefinition{
						Variable: "foo",
						Type: document.NamedType{
							Name:    "bar",
							NonNull: true,
						},
					},
					document.VariableDefinition{
						Variable: "baz",
						Type: document.NamedType{
							Name: "bax",
						},
					},
				}),
			},
			{
				it:        "should parse simple VariableDefinitions with DefaultValue between",
				input:     `($foo : bar! = "me" $baz : bax)`,
				expectErr: BeNil(),
				expectValues: Equal(document.VariableDefinitions{
					document.VariableDefinition{
						Variable: "foo",
						Type: document.NamedType{
							Name:    "bar",
							NonNull: true,
						},
						DefaultValue: document.StringValue{
							Val: "me",
						},
					},
					document.VariableDefinition{
						Variable: "baz",
						Type: document.NamedType{
							Name: "bax",
						},
					},
				}),
			},
			{
				it:        "should not parse VariableDefinitions when no closing bracket",
				input:     "($foo : bar!",
				expectErr: Not(BeNil()),
				expectValues: Equal(document.VariableDefinitions{
					document.VariableDefinition{
						Variable: "foo",
						Type: document.NamedType{
							Name:    "bar",
							NonNull: true,
						},
					},
				}),
			},
			{
				it:           "should not parse optional VariableDefinitions",
				input:        " ",
				expectErr:    BeNil(),
				expectValues: Equal(document.VariableDefinitions(nil)),
			},
		}

		for _, test := range tests {
			test := test

			g.It(test.it, func() {

				parser := NewParser()
				parser.l.SetInput(test.input)

				val, err := parser.parseVariableDefinitions()
				Expect(err).To(test.expectErr)
				Expect(val).To(test.expectValues)
			})
		}
	})
}
