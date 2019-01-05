package parser

import (
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
			it                      string
			input                   string
			expectErr               types.GomegaMatcher
			expectIndex             types.GomegaMatcher
			expectParsedDefinitions types.GomegaMatcher
		}{
			{
				it:          "should parse a simple FragmentSpread",
				input:       "firstFragment @rename(index: 3)",
				expectErr:   BeNil(),
				expectIndex: Equal([]int{0}),
				expectParsedDefinitions: Equal(ParsedDefinitions{
					Directives: document.Directives{
						{
							Name:      "rename",
							Arguments: []int{0},
						},
					},
					FieldDefinitions: document.FieldDefinitions{},
					Arguments: document.Arguments{
						{
							Name: "index",
							Value: document.Value{
								ValueType: document.ValueTypeInt,
								IntValue:  3,
							},
						},
					},
					EnumValuesDefinitions: document.EnumValueDefinitions{},
					EnumTypeDefinitions:   document.EnumTypeDefinitions{},
					FragmentSpreads: document.FragmentSpreads{
						{
							FragmentName: "firstFragment",
							Directives:   []int{0},
						},
					},
				}.initEmptySlices()),
			},
			{
				it:          "should parse FragmentSpread with optional Directives",
				input:       "firstFragment ",
				expectErr:   BeNil(),
				expectIndex: Equal([]int{0}),
				expectParsedDefinitions: Equal(ParsedDefinitions{
					FragmentSpreads: document.FragmentSpreads{
						{
							FragmentName: "firstFragment",
							Directives:   []int{},
						},
					},
				}.initEmptySlices()),
			},
			{
				it:        "should not parse FragmentSpread when FragmentName is 'on'",
				input:     "on",
				expectErr: HaveOccurred(),
			},
		}

		for _, test := range tests {
			test := test

			g.It(test.it, func() {

				parser := NewParser()
				parser.l.SetInput(test.input)

				var index []int
				err := parser.parseFragmentSpread(&index)
				Expect(err).To(test.expectErr)
				if test.expectIndex != nil {
					Expect(index).To(test.expectIndex)
				}
				if test.expectParsedDefinitions != nil {
					Expect(parser.ParsedDefinitions).To(test.expectParsedDefinitions)
				}
			})
		}
	})
}
