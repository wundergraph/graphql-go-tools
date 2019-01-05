package parser

import (
	. "github.com/franela/goblin"
	"github.com/jensneuse/graphql-go-tools/pkg/document"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/types"
	"testing"
)

func TestBoolValueParser(t *testing.T) {

	g := Goblin(t)
	RegisterFailHandler(func(m string, _ ...int) { g.Fail(m) })

	g.Describe("parser.parsePeekedBoolValue", func() {

		tests := []struct {
			it           string
			input        string
			expectErr    types.GomegaMatcher
			expectValues types.GomegaMatcher
		}{
			{
				it:        "should parse a true boolean",
				input:     "true",
				expectErr: BeNil(),
				expectValues: Equal(document.Value{
					ValueType:    document.ValueTypeBoolean,
					BooleanValue: true,
				}),
			},
			{
				it:        "should parse a false boolean",
				input:     "false",
				expectErr: BeNil(),
				expectValues: Equal(document.Value{
					ValueType:    document.ValueTypeBoolean,
					BooleanValue: false,
				}),
			},
		}

		for _, test := range tests {
			test := test

			g.It(test.it, func() {

				parser := NewParser()
				parser.l.SetInput(test.input)

				val, err := parser.parsePeekedBoolValue()
				Expect(err).To(test.expectErr)
				Expect(val).To(test.expectValues)
			})
		}
	})
}
