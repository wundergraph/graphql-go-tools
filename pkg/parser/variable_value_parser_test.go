package parser

import (
	. "github.com/franela/goblin"
	"github.com/jensneuse/graphql-go-tools/pkg/document"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/types"
	"testing"
)

func TestVariableValueParser(t *testing.T) {

	g := Goblin(t)
	RegisterFailHandler(func(m string, _ ...int) { g.Fail(m) })

	g.Describe("parser.parsePeekedVariableValue", func() {

		tests := []struct {
			it           string
			input        string
			expectErr    types.GomegaMatcher
			expectValues types.GomegaMatcher
		}{
			{
				it:        "should parse a simple variable",
				input:     `$anyIdent`,
				expectErr: BeNil(),
				expectValues: Equal(document.Value{
					ValueType:     document.ValueTypeVariable,
					VariableValue: "anyIdent",
				}),
			},
			{
				it:           "should parse a simple variable",
				input:        `$ anyIdent`,
				expectErr:    HaveOccurred(),
				expectValues: Equal(document.Value{}),
			},
		}

		for _, test := range tests {
			test := test

			g.It(test.it, func() {

				parser := NewParser()
				parser.l.SetInput(test.input)

				val, err := parser.parsePeekedVariableValue()
				Expect(err).To(test.expectErr)
				Expect(val).To(test.expectValues)
			})
		}
	})
}
