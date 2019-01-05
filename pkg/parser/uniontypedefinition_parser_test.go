package parser

import (
	. "github.com/franela/goblin"
	"github.com/jensneuse/graphql-go-tools/pkg/document"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/types"
	"testing"
)

func TestParseUnionTypeDefinition(t *testing.T) {

	g := Goblin(t)
	RegisterFailHandler(func(m string, _ ...int) { g.Fail(m) })

	g.Describe("parseUnionTypeDefinition", func() {
		tests := []struct {
			it                      string
			input                   string
			expectErr               types.GomegaMatcher
			expectIndex             types.GomegaMatcher
			expectParsedDefinitions types.GomegaMatcher
		}{

			{
				it:          "should parse simple UnionTypeDefinition",
				input:       ` SearchResult = Photo | Person`,
				expectErr:   BeNil(),
				expectIndex: Equal([]int{0}),
				expectParsedDefinitions: Equal(ParsedDefinitions{
					UnionTypeDefinitions: document.UnionTypeDefinitions{
						{
							Name: "SearchResult",
							UnionMemberTypes: document.UnionMemberTypes{
								"Photo",
								"Person",
							},
						},
					},
				}.initEmptySlices()),
			},
			{
				it:          "should parse multiple UnionMemberTypes in UnionTypeDefinition",
				input:       ` SearchResult = Photo | Person | Car | Planet`,
				expectErr:   BeNil(),
				expectIndex: Equal([]int{0}),
				expectParsedDefinitions: Equal(ParsedDefinitions{
					UnionTypeDefinitions: document.UnionTypeDefinitions{
						{
							Name: "SearchResult",
							UnionMemberTypes: document.UnionMemberTypes{
								"Photo",
								"Person",
								"Car",
								"Planet",
							},
						},
					},
				}.initEmptySlices()),
			},
			{
				it: "should parse multiple UnionMemberTypes spread over multiple lines in UnionTypeDefinition",
				input: ` SearchResult = Photo 
| Person 
| Car 
| Planet`,
				expectErr:   BeNil(),
				expectIndex: Equal([]int{0}),
				expectParsedDefinitions: Equal(ParsedDefinitions{
					UnionTypeDefinitions: document.UnionTypeDefinitions{
						{
							Name: "SearchResult",
							UnionMemberTypes: document.UnionMemberTypes{
								"Photo",
								"Person",
								"Car",
								"Planet",
							},
						},
					},
				}.initEmptySlices()),
			},
			{
				it:          "should parse a UnionTypeDefinition with Directives",
				input:       ` SearchResult @fromTop(to: "bottom") @fromBottom(to: "top") = Photo | Person`,
				expectErr:   BeNil(),
				expectIndex: Equal([]int{0}),
				expectParsedDefinitions: Equal(ParsedDefinitions{
					Arguments: document.Arguments{
						{
							Name: "to",
							Value: document.Value{
								ValueType:   document.ValueTypeString,
								StringValue: "bottom",
							},
						},
						{
							Name: "to",
							Value: document.Value{
								ValueType:   document.ValueTypeString,
								StringValue: "top",
							},
						},
					},
					Directives: document.Directives{
						document.Directive{
							Name:      "fromTop",
							Arguments: []int{0},
						},
						document.Directive{
							Name:      "fromBottom",
							Arguments: []int{1},
						},
					},
					UnionTypeDefinitions: document.UnionTypeDefinitions{
						{
							Name:       "SearchResult",
							Directives: []int{0, 1},
							UnionMemberTypes: document.UnionMemberTypes{
								"Photo",
								"Person",
							},
						},
					},
				}.initEmptySlices()),
			},
			{
				it:          "should parse a UnionTypeDefinition with optional UnionMemberTypes",
				input:       ` SearchResult`,
				expectErr:   BeNil(),
				expectIndex: Equal([]int{0}),
				expectParsedDefinitions: Equal(ParsedDefinitions{
					UnionTypeDefinitions: document.UnionTypeDefinitions{
						{
							Name: "SearchResult",
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
				err := parser.parseUnionTypeDefinition(&index)
				Expect(err).To(test.expectErr)

			})
		}

	})
}
