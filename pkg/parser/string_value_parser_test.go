package parser

import (
	. "github.com/franela/goblin"
	"github.com/jensneuse/graphql-go-tools/pkg/document"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/types"
	"testing"
)

func TestStringValueParser(t *testing.T) {

	g := Goblin(t)
	RegisterFailHandler(func(m string, _ ...int) { g.Fail(m) })

	g.Describe("parser.parsePeekedStringValue", func() {

		tests := []struct {
			it        string
			input     string
			expectErr types.GomegaMatcher
			expectVal types.GomegaMatcher
		}{
			{
				it:        "should parse single line string value",
				input:     `"lorem ipsum"`,
				expectErr: BeNil(),
				expectVal: Equal(document.Value{
					ValueType:   document.ValueTypeString,
					StringValue: "lorem ipsum",
				}),
			},
			{
				it: "should parse multi line string value",
				input: `"""
lorem ipsum
"""`,
				expectErr: BeNil(),
				expectVal: Equal(document.Value{
					ValueType:   document.ValueTypeString,
					StringValue: "lorem ipsum",
				}),
			},
			{
				it: "should parse multi line string value with escaped quote",
				input: `"""
foo \" bar 
"""`,
				expectErr: BeNil(),
				expectVal: Equal(document.Value{
					ValueType:   document.ValueTypeString,
					StringValue: `foo \" bar`,
				}),
			},
			{
				it:        "should parse single line string with escaped\"",
				input:     `"foo bar \" baz"`,
				expectErr: BeNil(),
				expectVal: Equal(document.Value{
					ValueType:   document.ValueTypeString,
					StringValue: "foo bar \\\" baz",
				}),
			},
		}

		for _, test := range tests {
			test := test

			g.It(test.it, func() {

				parser := NewParser()
				parser.l.SetInput(test.input)

				val, err := parser.parsePeekedStringValue()
				Expect(err).To(test.expectErr)
				Expect(val).To(test.expectVal)
			})
		}
	})
}
