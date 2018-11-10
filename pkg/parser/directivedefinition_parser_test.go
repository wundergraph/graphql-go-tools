package parser

import (
	"bytes"
	. "github.com/franela/goblin"
	"github.com/jensneuse/graphql-go-tools/pkg/document"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/types"
	"testing"
)

func TestDirectiveDefinitionParser(t *testing.T) {

	g := Goblin(t)
	RegisterFailHandler(func(m string, _ ...int) { g.Fail(m) })

	g.Describe("parser.parseDirectiveDefinition", func() {

		tests := []struct {
			it           string
			input        string
			expectErr    types.GomegaMatcher
			expectValues types.GomegaMatcher
		}{
			{
				it:        "should parse a simple DirectiveDefinition",
				input:     "@ somewhere on QUERY",
				expectErr: BeNil(),
				expectValues: Equal(document.DirectiveDefinition{
					Name: "somewhere",
					DirectiveLocations: document.DirectiveLocations{
						document.DirectiveLocationQUERY,
					},
				}),
			},
			{
				it:        "should parse a simple DirectiveDefinition with trailing PIPE",
				input:     "@ somewhere on | QUERY",
				expectErr: BeNil(),
				expectValues: Equal(document.DirectiveDefinition{
					Name: "somewhere",
					DirectiveLocations: document.DirectiveLocations{
						document.DirectiveLocationQUERY,
					},
				}),
			},
			{
				it:        "should parse a DirectiveDefinition with ArgumentsDefinition",
				input:     "@ somewhere(inputValue: Int) on QUERY",
				expectErr: BeNil(),
				expectValues: Equal(document.DirectiveDefinition{
					Name: "somewhere",
					ArgumentsDefinition: document.ArgumentsDefinition{
						document.InputValueDefinition{
							Name: "inputValue",
							Type: document.NamedType{
								Name: "Int",
							},
						},
					},
					DirectiveLocations: document.DirectiveLocations{
						document.DirectiveLocationQUERY,
					},
				}),
			},
			{
				it:        "should not parse a DirectiveDefinition where the 'on' is missing",
				input:     "@ somewhere QUERY",
				expectErr: Not(BeNil()),
				expectValues: Equal(document.DirectiveDefinition{
					Name: "somewhere",
				}),
			},
			{
				it:        "should not parse a DirectiveDefinition where 'on' is not exactly spelled",
				input:     "@ somewhere off QUERY",
				expectErr: Not(BeNil()),
				expectValues: Equal(document.DirectiveDefinition{
					Name: "somewhere",
				}),
			},
			{
				it:        "should not parse a DirectiveDefinition when an invalid DirectiveLocation is given",
				input:     "@ somewhere on QUERY | thisshouldntwork",
				expectErr: Not(BeNil()),
				expectValues: Equal(document.DirectiveDefinition{
					Name: "somewhere",
					DirectiveLocations: document.DirectiveLocations{
						document.DirectiveLocationQUERY,
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

				val, err := parser.parseDirectiveDefinition()
				Expect(err).To(test.expectErr)
				Expect(val).To(test.expectValues)
			})
		}
	})
}
