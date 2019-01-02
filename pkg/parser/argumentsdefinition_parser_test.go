package parser

import (
	. "github.com/franela/goblin"
	"github.com/jensneuse/graphql-go-tools/pkg/document"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/types"
	"testing"
)

func TestArgumentsDefinitionParser(t *testing.T) {

	g := Goblin(t)
	RegisterFailHandler(func(m string, _ ...int) { g.Fail(m) })

	g.Describe("parser.parseArgumentsDefinition", func() {

		tests := []struct {
			it           string
			input        string
			expectErr    types.GomegaMatcher
			expectValues types.GomegaMatcher
		}{
			{
				it:        "should parse a simple ArgumentsDefinition",
				input:     `(inputValue: Int)`,
				expectErr: BeNil(),
				expectValues: Equal(document.ArgumentsDefinition{
					document.InputValueDefinition{
						Name: "inputValue",
						Type: document.NamedType{
							Name: "Int",
						},
					},
				}),
			},
			{
				it:           "should not parse an optional ArgumentsDefinition",
				input:        ` `,
				expectErr:    BeNil(),
				expectValues: Equal(document.ArgumentsDefinition(nil)),
			},
			{
				it:        "should be able to parse multiple InputValueDefinitions within an ArgumentsDefinition",
				input:     `(inputValue: Int, outputValue: String)`,
				expectErr: BeNil(),
				expectValues: Equal(document.ArgumentsDefinition{
					document.InputValueDefinition{
						Name: "inputValue",
						Type: document.NamedType{
							Name: "Int",
						},
					},
					document.InputValueDefinition{
						Name: "outputValue",
						Type: document.NamedType{
							Name: "String",
						},
					},
				}),
			},
			{
				it:           "should return empty when no BRACKETOPEN at beginning (since it can be optional)",
				input:        `inputValue: Int)`,
				expectErr:    BeNil(),
				expectValues: Equal(document.ArgumentsDefinition(nil)),
			},
			{
				it:           "should fail when double BRACKETOPEN at beginning",
				input:        `((inputValue: Int)`,
				expectErr:    Not(BeNil()),
				expectValues: Equal(document.ArgumentsDefinition(nil)),
			},
			{
				it:        "should fail when no BRACKETCLOSE at the end",
				input:     `(inputValue: Int`,
				expectErr: Not(BeNil()),
				expectValues: Equal(document.ArgumentsDefinition(document.ArgumentsDefinition{
					document.InputValueDefinition{
						Name: "inputValue",
						Type: document.NamedType{
							Name: "Int",
						},
					},
				})),
			},
		}

		for _, test := range tests {
			test := test

			g.It(test.it, func() {

				parser := NewParser()
				parser.l.SetInput(test.input)

				val, err := parser.parseArgumentsDefinition()
				Expect(err).To(test.expectErr)
				Expect(val).To(test.expectValues)
			})
		}
	})
}
