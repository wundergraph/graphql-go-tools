package parser

import (
	. "github.com/franela/goblin"
	"github.com/jensneuse/graphql-go-tools/pkg/document"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/types"
	"testing"
)

func TestParseScalar(t *testing.T) {

	g := Goblin(t)
	RegisterFailHandler(func(m string, _ ...int) { g.Fail(m) })

	g.Describe("parseScalar", func() {
		tests := []struct {
			it           string
			input        string
			expectErr    types.GomegaMatcher
			expectValues types.GomegaMatcher
		}{
			{
				it:        "should parse simple scalar",
				input:     ` JSON`,
				expectErr: BeNil(),
				expectValues: Equal(document.ScalarTypeDefinition{
					Name: "JSON",
				}),
			},
			{
				it:        "should parse a scalar with Directives",
				input:     ` JSON @fromTop(to: "bottom") @fromBottom(to: "top") `,
				expectErr: BeNil(),
				expectValues: Equal(document.ScalarTypeDefinition{
					Name: "JSON",
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
				}),
			},
		}

		for _, test := range tests {
			test := test

			g.It(test.it, func() {

				parser := NewParser()
				parser.l.SetInput(test.input)

				val, err := parser.parseScalarTypeDefinition()
				Expect(err).To(test.expectErr)
				Expect(val).To(test.expectValues)
			})
		}
	})
}
