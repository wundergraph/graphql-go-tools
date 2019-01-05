package parser

import (
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
			it                      string
			input                   string
			expectErr               types.GomegaMatcher
			expectIndex             types.GomegaMatcher
			expectParsedDefinitions types.GomegaMatcher
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
				expectErr:   BeNil(),
				expectIndex: Equal([]int{2}),
				expectParsedDefinitions: Equal(ParsedDefinitions{
					InlineFragments: document.InlineFragments{
						{
							TypeCondition: document.NamedType{
								Name: "GoAir",
							},
							Directives: []int{},
							SelectionSet: document.SelectionSet{
								Fields:          []int{0},
								FragmentSpreads: []int{},
								InlineFragments: []int{},
							},
						},
						{
							TypeCondition: document.NamedType{
								Name: "GoWater",
							},
							Directives: []int{},
							SelectionSet: document.SelectionSet{
								InlineFragments: []int{0},
								FragmentSpreads: []int{},
								Fields:          []int{},
							},
						},
						{
							TypeCondition: document.NamedType{
								Name: "Goland",
							},
							Directives: []int{},
							SelectionSet: document.SelectionSet{
								InlineFragments: []int{1},
								Fields:          []int{},
								FragmentSpreads: []int{},
							},
						},
					},
					Fields: document.Fields{
						{
							Name:       "go",
							Directives: []int{},
							Arguments:  []int{},
							SelectionSet: document.SelectionSet{
								Fields:          []int{},
								FragmentSpreads: []int{},
								InlineFragments: []int{},
							},
						},
					},
				}.initEmptySlices()),
			},
		}

		for _, test := range tests {
			test := test

			g.It(test.it, func() {

				parser := NewParser()
				parser.l.SetInput(test.input)

				var index []int
				err := parser.parseInlineFragment(&index)
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
