package parser

import (
	. "github.com/franela/goblin"
	"github.com/jensneuse/graphql-go-tools/pkg/document"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/types"
	"testing"
)

func TestFieldParser(t *testing.T) {

	g := Goblin(t)
	RegisterFailHandler(func(m string, _ ...int) { g.Fail(m) })

	g.Describe("parser.parseField", func() {

		tests := []struct {
			it                      string
			input                   string
			expectErr               types.GomegaMatcher
			expectIndex             types.GomegaMatcher
			expectParsedDefinitions types.GomegaMatcher
		}{
			{
				it:          "should parse a simple Field",
				input:       "preferredName: originalName(isSet: true) @rename(index: 3)",
				expectErr:   BeNil(),
				expectIndex: Equal([]int{0}),
				expectParsedDefinitions: Equal(ParsedDefinitions{
					Directives: document.Directives{
						{
							Name:      "rename",
							Arguments: []int{1},
						},
					},
					Arguments: document.Arguments{
						{
							Name: "isSet",
							Value: document.Value{
								ValueType:    document.ValueTypeBoolean,
								BooleanValue: true,
							},
						},
						{
							Name: "index",
							Value: document.Value{
								ValueType: document.ValueTypeInt,
								IntValue:  3,
							},
						},
					},
					Fields: document.Fields{
						{
							Alias:      "preferredName",
							Name:       "originalName",
							Arguments:  []int{0},
							Directives: []int{0},
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
				it:          "should parse Field with optional Alias",
				input:       "originalName(isSet: true) @rename(index: 3)",
				expectErr:   BeNil(),
				expectIndex: Equal([]int{0}),
				expectParsedDefinitions: Equal(ParsedDefinitions{
					Arguments: document.Arguments{
						{
							Name: "isSet",
							Value: document.Value{
								ValueType:    document.ValueTypeBoolean,
								BooleanValue: true,
							},
						},
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
							Arguments: []int{1},
						},
					},
					EnumValuesDefinitions: document.EnumValueDefinitions{},
					EnumTypeDefinitions:   document.EnumTypeDefinitions{},
					Fields: document.Fields{
						{
							Name:       "originalName",
							Arguments:  []int{0},
							Directives: []int{0},
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
				it:          "should parse Field with optional Arguments",
				input:       "preferredName: originalName @rename(index: 3)",
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
					Fields: document.Fields{
						{
							Alias:      "preferredName",
							Name:       "originalName",
							Directives: []int{0},
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
				it:          "should parse Field with optional Directives",
				input:       "preferredName: originalName(isSet: true)",
				expectErr:   BeNil(),
				expectIndex: Equal([]int{0}),
				expectParsedDefinitions: Equal(ParsedDefinitions{
					Arguments: document.Arguments{
						{
							Name: "isSet",
							Value: document.Value{
								ValueType:    document.ValueTypeBoolean,
								BooleanValue: true,
							},
						},
					},
					Fields: document.Fields{
						{
							Alias:      "preferredName",
							Name:       "originalName",
							Arguments:  []int{0},
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
				it: "should parse Field with nested SelectionSets",
				input: `
				originalName {
					unoriginalName {
						worstNamePossible
					}
				}
				`,
				expectErr:   BeNil(),
				expectIndex: Equal([]int{2}),
				expectParsedDefinitions: Equal(ParsedDefinitions{
					Fields: document.Fields{
						{
							Name:       "worstNamePossible",
							Arguments:  []int{},
							Directives: []int{},
							SelectionSet: document.SelectionSet{
								Fields:          []int{},
								FragmentSpreads: []int{},
								InlineFragments: []int{},
							},
						},
						{
							Name:       "unoriginalName",
							Arguments:  []int{},
							Directives: []int{},
							SelectionSet: document.SelectionSet{
								Fields:          []int{0},
								FragmentSpreads: []int{},
								InlineFragments: []int{},
							},
						},
						{
							Name:       "originalName",
							Arguments:  []int{},
							Directives: []int{},
							SelectionSet: document.SelectionSet{
								Fields:          []int{1},
								InlineFragments: []int{},
								FragmentSpreads: []int{},
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
				err := parser.parseField(&index)

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

var parseFieldBenchmarkInput = `t { kind name ofType { kind name ofType { kind name } } }`

func BenchmarkParseField(b *testing.B) {

	var err error

	parser := NewParser()

	b.ReportAllocs()

	for i := 0; i < b.N; i++ {

		parser.l.SetInput(parseFieldBenchmarkInput)
		var index []int
		err = parser.parseField(&index)
		if err != nil {
			b.Fatal(err)
		}
	}
}
