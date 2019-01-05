package parser

import (
	. "github.com/franela/goblin"
	"github.com/jensneuse/graphql-go-tools/pkg/document"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/types"
	"testing"
)

func TestOperationDefinitionParser(t *testing.T) {

	g := Goblin(t)
	RegisterFailHandler(func(m string, _ ...int) { g.Fail(m) })

	g.Describe("parser.parseOperationDefinition", func() {

		tests := []struct {
			it                      string
			input                   string
			expectErr               types.GomegaMatcher
			expectIndex             types.GomegaMatcher
			expectParsedDefinitions types.GomegaMatcher
		}{
			{
				it: "should parse a simple OperationDefinition",
				input: `
				query allGophers($color: String)@rename(index: 3) {
					name
				}
				`,
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
					OperationDefinitions: document.OperationDefinitions{
						{
							OperationType:       document.OperationTypeQuery,
							Name:                "allGophers",
							VariableDefinitions: []int{0},
							Directives:          []int{0},
							SelectionSet: document.SelectionSet{
								Fields:          []int{0},
								FragmentSpreads: []int{},
								InlineFragments: []int{},
							},
						},
					},
					VariableDefinitions: document.VariableDefinitions{
						{
							Variable: "color",
							Type: document.NamedType{
								Name: "String",
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
				it: "should parse a OperationDefinition with optional Directives",
				input: `
				query allGophers($color: String) {
					name
				}
				`,
				expectErr:   BeNil(),
				expectIndex: Equal([]int{0}),
				expectParsedDefinitions: Equal(ParsedDefinitions{
					FragmentDefinitions: document.FragmentDefinitions{},
					InlineFragments:     document.InlineFragments{},
					FragmentSpreads:     document.FragmentSpreads{},
					OperationDefinitions: document.OperationDefinitions{
						{
							OperationType:       document.OperationTypeQuery,
							Name:                "allGophers",
							VariableDefinitions: []int{0},
							SelectionSet: document.SelectionSet{
								Fields:          []int{0},
								InlineFragments: []int{},
								FragmentSpreads: []int{},
							},
							Directives: []int{},
						},
					},
					VariableDefinitions: document.VariableDefinitions{
						{
							Variable: "color",
							Type: document.NamedType{
								Name: "String",
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
				it: "should parse a OperationDefinition with optional VariableDefinitions",
				input: `
				query allGophers@rename(index: 3) {
					name
				}
				`,
				expectErr: BeNil(),
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
					OperationDefinitions: document.OperationDefinitions{
						{
							OperationType: document.OperationTypeQuery,
							Name:          "allGophers",
							Directives:    []int{0},
							SelectionSet: document.SelectionSet{
								Fields:          []int{0},
								FragmentSpreads: []int{},
								InlineFragments: []int{},
							},
							VariableDefinitions: []int{},
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
				it: "should parse an OperationDefinition with optional Name",
				input: `
				query ($color: String)@rename(index: 3) {
					name
				}
				`,
				expectErr:   BeNil(),
				expectIndex: Equal([]int{0}),
				expectParsedDefinitions: Equal(ParsedDefinitions{
					Directives: document.Directives{
						{
							Name:      "rename",
							Arguments: []int{0},
						},
					},
					Arguments: document.Arguments{
						{
							Name: "index",
							Value: document.Value{
								ValueType: document.ValueTypeInt,
								IntValue:  3,
							},
						},
					},
					OperationDefinitions: document.OperationDefinitions{
						{
							OperationType:       document.OperationTypeQuery,
							VariableDefinitions: []int{0},
							Directives:          []int{0},
							SelectionSet: document.SelectionSet{
								Fields:          []int{0},
								InlineFragments: []int{},
								FragmentSpreads: []int{},
							},
						},
					},
					VariableDefinitions: document.VariableDefinitions{
						{
							Variable: "color",
							Type: document.NamedType{
								Name: "String",
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
				it: "should parse a OperationDefinition omitting all optional types",
				input: `
				{
					name
				}
				`,
				expectErr:   BeNil(),
				expectIndex: Equal([]int{0}),
				expectParsedDefinitions: Equal(ParsedDefinitions{
					OperationDefinitions: document.OperationDefinitions{
						{
							OperationType: document.OperationTypeQuery,
							SelectionSet: document.SelectionSet{
								Fields:          []int{0},
								FragmentSpreads: []int{},
								InlineFragments: []int{},
							},
							VariableDefinitions: []int{},
							Directives:          []int{},
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
				it: "should not parse a OperationDefinition without SelectionSet",
				input: `
				query allGophers($color: String)@rename(index: 3) `,
				expectErr:   Not(BeNil()),
				expectIndex: Equal([]int{0}),
				expectParsedDefinitions: Equal(ParsedDefinitions{
					Directives: document.Directives{
						{
							Name:      "rename",
							Arguments: []int{0},
						},
					},
					Arguments: document.Arguments{
						{
							Name: "index",
							Value: document.Value{
								ValueType: document.ValueTypeInt,
								IntValue:  3,
							},
						},
					},
					OperationDefinitions: document.OperationDefinitions{
						{
							OperationType:       document.OperationTypeQuery,
							Name:                "allGophers",
							VariableDefinitions: []int{0},
							Directives:          []int{0},
							SelectionSet: document.SelectionSet{
								Fields:          []int{},
								FragmentSpreads: []int{},
								InlineFragments: []int{},
							},
						},
					},
					VariableDefinitions: document.VariableDefinitions{
						{
							Variable: "color",
							Type: document.NamedType{
								Name: "String",
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
				err := parser.parseOperationDefinition(&index)
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
