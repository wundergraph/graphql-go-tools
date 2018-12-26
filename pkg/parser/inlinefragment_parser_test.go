package parser

import (
	"bytes"
	. "github.com/franela/goblin"
	"github.com/jensneuse/graphql-go-tools/pkg/document"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/types"
	"testing"
)

func TestInlineFragmentParser(t *testing.T) {

	g := Goblin(t)
	RegisterFailHandler(func(m string, _ ...int) { g.Fail(m) })

	g.Describe("parser.parseInlineFragment", func() {

		tests := []struct {
			it           string
			input        string
			expectErr    types.GomegaMatcher
			expectValues types.GomegaMatcher
		}{
			{
				it: "should parse InlineFragment with nested SelectionSets",
				input: `Goland {
					... on GoWater {
						... on GoAir {
							go
						}
					}
				}
				`,
				expectErr: BeNil(),
				expectValues: Equal(document.InlineFragment{
					TypeCondition: document.NamedType{
						Name: "Goland",
					},
					SelectionSet: document.SelectionSet{
						document.InlineFragment{
							TypeCondition: document.NamedType{
								Name: "GoWater",
							},
							SelectionSet: document.SelectionSet{
								document.InlineFragment{
									TypeCondition: document.NamedType{
										Name: "GoAir",
									},
									SelectionSet: []document.Selection{
										document.Field{
											Name: "go",
										},
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

				reader := bytes.NewReader([]byte(test.input))
				parser := NewParser()
				parser.l.SetInput(reader)

				val, err := parser.parseInlineFragment()
				Expect(err).To(test.expectErr)
				Expect(val).To(test.expectValues)
			})
		}
	})
}
