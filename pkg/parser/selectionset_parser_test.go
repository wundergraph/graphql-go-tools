package parser

import (
	. "github.com/franela/goblin"
	"github.com/jensneuse/graphql-go-tools/pkg/document"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/types"
	"testing"
)

func TestSelectionSetParser(t *testing.T) {

	g := Goblin(t)
	RegisterFailHandler(func(m string, _ ...int) { g.Fail(m) })

	g.Describe("parser.parseSelectionSet", func() {

		tests := []struct {
			it                      string
			input                   string
			expectErr               types.GomegaMatcher
			expectSelectionSet      types.GomegaMatcher
			expectParsedDefinitions types.GomegaMatcher
		}{
			{
				it: "should parse a simple SelectionSet",
				input: `{
					foo
				}`,
				expectErr: BeNil(),
				expectSelectionSet: Equal(document.SelectionSet{
					Fields:          []int{0},
					InlineFragments: []int{},
					FragmentSpreads: []int{},
				}),
				expectParsedDefinitions: Equal(ParsedDefinitions{
					Fields: document.Fields{
						{
							Name:       "foo",
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
				it: "should parse SelectionSet with multiple elements in it",
				input: `{
					... on Goland
					...Air
					... on Water
				}`,
				expectErr: BeNil(),
				expectSelectionSet: Equal(document.SelectionSet{
					InlineFragments: []int{0, 1},
					FragmentSpreads: []int{0},
					Fields:          []int{},
				}),
				expectParsedDefinitions: Equal(ParsedDefinitions{
					InlineFragments: document.InlineFragments{
						{
							TypeCondition: document.NamedType{
								Name: "Goland",
							},
							Directives: []int{},
							SelectionSet: document.SelectionSet{
								InlineFragments: []int{},
								FragmentSpreads: []int{},
								Fields:          []int{},
							},
						},
						{
							TypeCondition: document.NamedType{
								Name: "Water",
							},
							Directives: []int{},
							SelectionSet: document.SelectionSet{
								InlineFragments: []int{},
								FragmentSpreads: []int{},
								Fields:          []int{},
							},
						},
					},
					FragmentSpreads: document.FragmentSpreads{
						{
							FragmentName: "Air",
							Directives:   []int{},
						},
					},
				}.initEmptySlices()),
			},
			{
				it: "should parse SelectionSet with multiple different elements in it",
				input: `{
					... on Goland
					preferredName: originalName(isSet: true)
					... on Water
				}`,
				expectErr: BeNil(),
				expectSelectionSet: Equal(document.SelectionSet{
					Fields:          []int{0},
					InlineFragments: []int{0, 1},
					FragmentSpreads: []int{},
				}),
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
							Alias:     "preferredName",
							Name:      "originalName",
							Arguments: []int{0},
							SelectionSet: document.SelectionSet{
								Fields:          []int{},
								FragmentSpreads: []int{},
								InlineFragments: []int{},
							},
							Directives: []int{},
						},
					},
					InlineFragments: document.InlineFragments{
						{
							TypeCondition: document.NamedType{
								Name: "Goland",
							},
							Directives: []int{},
							SelectionSet: document.SelectionSet{
								InlineFragments: []int{},
								FragmentSpreads: []int{},
								Fields:          []int{},
							},
						},
						{
							TypeCondition: document.NamedType{
								Name: "Water",
							},
							Directives: []int{},
							SelectionSet: document.SelectionSet{
								InlineFragments: []int{},
								FragmentSpreads: []int{},
								Fields:          []int{},
							},
						},
					},
				}.initEmptySlices()),
			},
			{
				it: "should parse SelectionSet with Field containing directives",
				input: `{
					... on Goland
					preferredName: originalName(isSet: true) @rename(index: 3)
					... on Water
				}`,
				expectErr: BeNil(),
				expectSelectionSet: Equal(document.SelectionSet{
					Fields:          []int{0},
					InlineFragments: []int{0, 1},
					FragmentSpreads: []int{},
				}),
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
					InlineFragments: document.InlineFragments{
						{
							TypeCondition: document.NamedType{
								Name: "Goland",
							},
							SelectionSet: document.SelectionSet{
								InlineFragments: []int{},
								FragmentSpreads: []int{},
								Fields:          []int{},
							},
							Directives: []int{},
						},
						{
							TypeCondition: document.NamedType{
								Name: "Water",
							},
							SelectionSet: document.SelectionSet{
								InlineFragments: []int{},
								FragmentSpreads: []int{},
								Fields:          []int{},
							},
							Directives: []int{},
						},
					},
				}.initEmptySlices()),
			},
			{
				it: "should parse SelectionSet with FragmentSpread containing Directive",
				input: `{
					... on Goland
					...firstFragment @rename(index: 3)
					... on Water
				}`,
				expectErr: BeNil(),
				expectSelectionSet: Equal(document.SelectionSet{
					InlineFragments: []int{0, 1},
					FragmentSpreads: []int{0},
					Fields:          []int{},
				}),
				expectParsedDefinitions: Equal(ParsedDefinitions{
					InlineFragments: document.InlineFragments{
						{
							TypeCondition: document.NamedType{
								Name: "Goland",
							},
							SelectionSet: document.SelectionSet{
								InlineFragments: []int{},
								FragmentSpreads: []int{},
								Fields:          []int{},
							},
							Directives: []int{},
						},
						{
							TypeCondition: document.NamedType{
								Name: "Water",
							},
							SelectionSet: document.SelectionSet{
								InlineFragments: []int{},
								FragmentSpreads: []int{},
								Fields:          []int{},
							},
							Directives: []int{},
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
					Directives: document.Directives{
						document.Directive{
							Name:      "rename",
							Arguments: []int{0},
						},
					},
					FragmentSpreads: document.FragmentSpreads{
						{
							FragmentName: "firstFragment",
							Directives:   []int{0},
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

				set := parser.makeSelectionSet()
				err := parser.parseSelectionSet(&set)
				Expect(err).To(test.expectErr)
				if test.expectSelectionSet != nil {
					Expect(set).To(test.expectSelectionSet)
				}
				if test.expectParsedDefinitions != nil {
					Expect(parser.ParsedDefinitions).To(test.expectParsedDefinitions)
				}
			})
		}
	})
}

var selectionSetBenchmarkInput = `{
  kind
  name
  ofType {
    kind
    name
    ofType {
      kind
      name
      ofType {
        kind
        name
        ofType {
          kind
          name
          ofType {
            kind
            name
            ofType {
              kind
              name
              ofType {
                kind
                name
              }
            }
          }
        }
      }
    }
  }
}`

func BenchmarkParseSelectionSet(b *testing.B) {

	parser := NewParser()
	var err error

	parse := func() {

		parser.l.SetInput(selectionSetBenchmarkInput)

		selectionSet := parser.makeSelectionSet()
		err = parser.parseSelectionSet(&selectionSet)
		if err != nil {
			b.Fatal(err)
		}
	}

	for i := 0; i < 10; i++ {
		parse()
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		parse()
	}
}
