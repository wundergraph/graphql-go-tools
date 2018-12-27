package parser

import (
	"bytes"
	. "github.com/franela/goblin"
	document "github.com/jensneuse/graphql-go-tools/pkg/document"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/types"
	"testing"
)

func TestListValueParser(t *testing.T) {

	g := Goblin(t)
	RegisterFailHandler(func(m string, _ ...int) { g.Fail(m) })

	g.Describe("parser.parsePeekedListValue", func() {

		tests := []struct {
			it           string
			input        string
			expectErr    types.GomegaMatcher
			expectValues types.GomegaMatcher
		}{
			{
				it:        "should parse simple list",
				input:     "[1,2,3]",
				expectErr: BeNil(),
				expectValues: Equal([]document.Value{
					document.IntValue{
						Val: 1,
					},
					document.IntValue{
						Val: 2,
					},
					document.IntValue{
						Val: 3,
					},
				}),
			},
			{
				it: "should parse complex list",
				input: `[ 1	,"2" 3,,[	1	]]`,
				expectErr: BeNil(),
				expectValues: Equal([]document.Value{
					document.IntValue{
						Val: 1,
					},
					document.StringValue{
						Val: []byte("2"),
					},
					document.IntValue{
						Val: 3,
					},
					document.ListValue{
						Values: []document.Value{
							document.IntValue{
								Val: 1,
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

				val, err := parser.parsePeekedListValue()
				Expect(err).To(test.expectErr)
				Expect(val.Values).To(test.expectValues)
			})
		}
	})
}
