package parser

import (
	. "github.com/franela/goblin"
	"github.com/jensneuse/graphql-go-tools/pkg/document"
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
				expectValues: Equal(document.Value{
					ValueType: document.ValueTypeList,
					ListValue: []document.Value{
						{
							ValueType: document.ValueTypeInt,
							IntValue:  1,
						},
						{
							ValueType: document.ValueTypeInt,
							IntValue:  2,
						},
						{
							ValueType: document.ValueTypeInt,
							IntValue:  3,
						},
					},
				}),
			},
			{
				it: "should parse complex list",
				input: `[ 1	,"2" 3,,[	1	]]`,
				expectErr: BeNil(),
				expectValues: Equal(document.Value{
					ValueType: document.ValueTypeList,
					ListValue: []document.Value{
						{
							ValueType: document.ValueTypeInt,
							IntValue:  1,
						},
						{
							ValueType:   document.ValueTypeString,
							StringValue: "2",
						},
						{
							ValueType: document.ValueTypeInt,
							IntValue:  3,
						},
						{
							ValueType: document.ValueTypeList,
							ListValue: []document.Value{
								{
									ValueType: document.ValueTypeInt,
									IntValue:  1,
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

				val, err := parser.parsePeekedListValue()
				Expect(err).To(test.expectErr)
				Expect(val).To(test.expectValues)
			})
		}
	})
}
