package parser

import (
	. "github.com/franela/goblin"
	"github.com/jensneuse/graphql-go-tools/pkg/document"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/types"
	"testing"
)

func TestArgumentsParser(t *testing.T) {

	g := Goblin(t)
	RegisterFailHandler(func(m string, _ ...int) { g.Fail(m) })

	g.Describe("parser.parseArguments", func() {

		tests := []struct {
			it           string
			input        string
			expectErr    types.GomegaMatcher
			expectValues types.GomegaMatcher
		}{
			{
				it:        "should parse simple arguments",
				input:     `(name: "Gophus")`,
				expectErr: BeNil(),
				expectValues: Equal(document.Arguments{
					document.Argument{
						Name: "name",
						Value: document.StringValue{
							Val: "Gophus",
						},
					},
				}),
			},
			{
				it:        "should parse a list of const strings",
				input:     `(fooBars: ["foo","bar"])`,
				expectErr: BeNil(),
				expectValues: Equal(document.Arguments{
					document.Argument{
						Name: "fooBars",
						Value: document.ListValue{
							Values: []document.Value{
								document.StringValue{
									Val: "foo",
								},
								document.StringValue{
									Val: "bar",
								},
							},
						},
					},
				}),
			},
			{
				it:        "should parse a list of const integers",
				input:     `(integers: [1,2,3])`,
				expectErr: BeNil(),
				expectValues: Equal(document.Arguments{
					document.Argument{
						Name: "integers",
						Value: document.ListValue{
							Values: []document.Value{
								document.IntValue{
									Val: 1,
								},
								document.IntValue{
									Val: 2,
								},
								document.IntValue{
									Val: 3,
								},
							},
						},
					},
				}),
			},
			{
				it:        "should parse multiple arguments",
				input:     `(name: "Gophus", surname: "Gophersson")`,
				expectErr: BeNil(),
				expectValues: Equal(document.Arguments{
					document.Argument{
						Name: "name",
						Value: document.StringValue{
							Val: "Gophus",
						},
					},
					document.Argument{
						Name: "surname",
						Value: document.StringValue{
							Val: "Gophersson",
						},
					},
				}),
			},
			{
				it:           "should not parse arguments when no bracket close",
				input:        `(name: "Gophus", surname: "Gophersson"`,
				expectErr:    HaveOccurred(),
				expectValues: BeNil(),
			},
			{
				it:           "should parse Arguments optionally",
				input:        `name: "Gophus", surname: "Gophersson")`,
				expectErr:    BeNil(),
				expectValues: Equal(document.Arguments(nil)),
			},
			{
				it:           "should not parse arguments when multiple brackets open",
				input:        `((name: "Gophus", surname: "Gophersson")`,
				expectErr:    Not(BeNil()),
				expectValues: Equal(document.Arguments(nil)),
			},
		}

		for _, test := range tests {
			test := test

			g.It(test.it, func() {

				parser := NewParser()
				parser.l.SetInput(test.input)

				val, err := parser.parseArguments()
				Expect(err).To(test.expectErr)
				Expect(val).To(test.expectValues)
			})
		}
	})
}
