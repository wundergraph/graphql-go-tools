package parser

import (
	"bytes"
	. "github.com/franela/goblin"
	"github.com/jensneuse/graphql-go-tools/pkg/document"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/types"
	"testing"
)

func TestFragmentSpreadParser(t *testing.T) {

	g := Goblin(t)
	RegisterFailHandler(func(m string, _ ...int) { g.Fail(m) })

	g.Describe("parser.parseFragmentSpread", func() {

		tests := []struct {
			it           string
			input        string
			expectErr    types.GomegaMatcher
			expectValues types.GomegaMatcher
		}{
			{
				it:        "should parse a simple FragmentSpread",
				input:     "firstFragment @rename(index: 3)",
				expectErr: BeNil(),
				expectValues: Equal(document.FragmentSpread{
					FragmentName: "firstFragment",
					Directives: document.Directives{
						document.Directive{
							Name: "rename",
							Arguments: document.Arguments{
								document.Argument{
									Name: "index",
									Value: document.IntValue{
										Val: 3,
									},
								},
							},
						},
					},
				}),
			},
			{
				it:        "should parse FragmentSpread with optional Directives",
				input:     "firstFragment ",
				expectErr: BeNil(),
				expectValues: Equal(document.FragmentSpread{
					FragmentName: "firstFragment",
				}),
			},
			{
				it:           "should not parse FragmentSpread when FragmentName is 'on'",
				input:        "on",
				expectErr:    Not(BeNil()),
				expectValues: Equal(document.FragmentSpread{}),
			},
		}

		for _, test := range tests {
			test := test

			g.It(test.it, func() {

				reader := bytes.NewReader([]byte(test.input))
				parser := NewParser()
				parser.l.SetInput(reader)

				val, err := parser.parseFragmentSpread()
				Expect(err).To(test.expectErr)
				Expect(val).To(test.expectValues)
			})
		}
	})
}
