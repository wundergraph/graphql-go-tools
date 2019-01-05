package parser

import (
	. "github.com/franela/goblin"
	"github.com/jensneuse/graphql-go-tools/pkg/document"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/types"
	"testing"
)

func TestFragmentDefinitionParser(t *testing.T) {

	g := Goblin(t)
	RegisterFailHandler(func(m string, _ ...int) { g.Fail(m) })

	g.Describe("parser.parseFragmentDefinition", func() {

		tests := []struct {
			it                      string
			input                   string
			expectErr               types.GomegaMatcher
			expectIndex             types.GomegaMatcher
			expectParsedDefinitions types.GomegaMatcher
		}{
			{
				it: "should parse a simple FragmentDefinition",
				input: `
				MyFragment on SomeType @rename(index: 3){
					name
				}`,
				expectErr:   BeNil(),
				expectIndex: Equal([]int{0}),
				expectParsedDefinitions: Equal(ParsedDefinitions{
					Arguments: document.Arguments{
						{
							Name: "index",
							Value: document.Value{
								ValueType: document.ValueTypeInt,
								IntValue:  3,
							},
						},
					},
					Directives: document.Directives{
						{
							Name:      "rename",
							Arguments: []int{0},
						},
					},
					FragmentDefinitions: document.FragmentDefinitions{
						{
							FragmentName: "MyFragment",
							TypeCondition: document.NamedType{
								Name: "SomeType",
							},
							Directives: []int{0},
							SelectionSet: document.SelectionSet{
								Fields:          []int{0},
								FragmentSpreads: []int{},
								InlineFragments: []int{},
							},
						},
					},
					Fields: document.Fields{
						{
							Name:       "name",
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
			{
				it: "should parse a FragmentDefinition with optional Directives",
				input: `
				MyFragment on SomeType{
					name
				}`,
				expectErr:   BeNil(),
				expectIndex: Equal([]int{0}),
				expectParsedDefinitions: Equal(ParsedDefinitions{
					FragmentDefinitions: document.FragmentDefinitions{
						{
							FragmentName: "MyFragment",
							TypeCondition: document.NamedType{
								Name: "SomeType",
							},
							Directives: []int{},
							SelectionSet: document.SelectionSet{
								Fields:          []int{0},
								InlineFragments: []int{},
								FragmentSpreads: []int{},
							},
						},
					},
					Fields: document.Fields{
						{
							Name:       "name",
							Arguments:  []int{},
							Directives: []int{},
							SelectionSet: document.SelectionSet{
								Fields:          []int{},
								FragmentSpreads: []int{},
								InlineFragments: []int{},
							},
						},
					},
				}.initEmptySlices()),
			},
			{
				it: "should not parse a FragmentDefinition with 'on' missing",
				input: `
				MyFragment SomeType{
					name
				}`,
				expectErr: HaveOccurred(),
			},
			{
				it: "should not parse a FragmentDefinition with 'on' missing",
				input: `
				MyFragment un SomeType{
					name
				}`,
				expectErr: HaveOccurred(),
			},
		}

		for _, test := range tests {
			test := test

			g.It(test.it, func() {

				parser := NewParser()
				parser.l.SetInput(test.input)

				var index []int
				err := parser.parseFragmentDefinition(&index)
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
