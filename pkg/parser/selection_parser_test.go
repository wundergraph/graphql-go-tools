package parser

import (
	"bytes"
	. "github.com/franela/goblin"
	"github.com/jensneuse/graphql-go-tools/pkg/document"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/types"
	"testing"
)

func TestSelectionParser(t *testing.T) {

	g := Goblin(t)
	RegisterFailHandler(func(m string, _ ...int) { g.Fail(m) })

	g.Describe("parser.parseSelection", func() {

		tests := []struct {
			it           string
			input        string
			expectErr    types.GomegaMatcher
			expectValues types.GomegaMatcher
		}{
			{
				it:        "should parse a InlineFragment",
				input:     "...on Land",
				expectErr: BeNil(),
				expectValues: Equal(document.InlineFragment{
					TypeCondition: document.NamedType{
						Name: "Land",
					},
				}),
			},
			{
				it:        "should parse a simple Field",
				input:     "originalName(isSet: true)",
				expectErr: BeNil(),
				expectValues: Equal(document.Field{
					Name: "originalName",
					Arguments: document.Arguments{
						document.Argument{
							Name: "isSet",
							Value: document.BooleanValue{
								Val: true,
							},
						},
					},
				}),
			},
			{
				it:        "should parse a FragmentSpread",
				input:     "...Land",
				expectErr: BeNil(),
				expectValues: Equal(document.FragmentSpread{
					FragmentName: "Land",
				}),
			},
		}

		for _, test := range tests {
			test := test

			g.It(test.it, func() {

				reader := bytes.NewReader([]byte(test.input))
				parser := NewParser()
				parser.l.SetInput(reader)

				val, err := parser.parseSelection()
				Expect(err).To(test.expectErr)
				Expect(val).To(test.expectValues)
			})
		}
	})
}
