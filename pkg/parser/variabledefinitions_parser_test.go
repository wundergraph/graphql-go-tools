package parser

import (
	. "github.com/franela/goblin"
	"github.com/jensneuse/graphql-go-tools/pkg/document"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/types"
	"testing"
)

func TestVariableDefinitionsParser(t *testing.T) {

	g := Goblin(t)
	RegisterFailHandler(func(m string, _ ...int) { g.Fail(m) })

	g.Describe("parser.parseVariableDefinitions", func() {

		tests := []struct {
			it                      string
			input                   string
			expectErr               types.GomegaMatcher
			expectIndex             types.GomegaMatcher
			expectParsedDefinitions types.GomegaMatcher
		}{
			{
				it:          "should parse a simple, single VariableDefinition",
				input:       "($foo : bar!)",
				expectErr:   BeNil(),
				expectIndex: Equal([]int{0}),
				expectParsedDefinitions: Equal(ParsedDefinitions{
					VariableDefinitions: document.VariableDefinitions{
						document.VariableDefinition{
							Variable: "foo",
							Type: document.NamedType{
								Name:    "bar",
								NonNull: true,
							},
						},
					},
				}.initEmptySlices()),
			},
			{
				it:          "should parse a simple, single nullable VariableDefinition",
				input:       "($color: String)",
				expectErr:   BeNil(),
				expectIndex: Equal([]int{0}),
				expectParsedDefinitions: Equal(ParsedDefinitions{
					VariableDefinitions: document.VariableDefinitions{
						document.VariableDefinition{
							Variable: "color",
							Type: document.NamedType{
								Name:    "String",
								NonNull: false,
							},
						},
					},
				}.initEmptySlices()),
			},
			{
				it:          "should parse simple VariableDefinitions",
				input:       "($foo : bar $baz : bax)",
				expectErr:   BeNil(),
				expectIndex: Equal([]int{0, 1}),
				expectParsedDefinitions: Equal(ParsedDefinitions{
					VariableDefinitions: document.VariableDefinitions{
						{
							Variable: "foo",
							Type: document.NamedType{
								Name: "bar",
							},
						},
						{
							Variable: "baz",
							Type: document.NamedType{
								Name: "bax",
							},
						},
					},
				}.initEmptySlices()),
			},
			{
				it:          "should parse simple VariableDefinitions with ListType between",
				input:       "($foo : [bar] $baz : bax)",
				expectErr:   BeNil(),
				expectIndex: Equal([]int{0, 1}),
				expectParsedDefinitions: Equal(ParsedDefinitions{
					VariableDefinitions: document.VariableDefinitions{
						{
							Variable: "foo",
							Type: document.ListType{Type: document.NamedType{
								Name: "bar",
							}},
						},
						{
							Variable: "baz",
							Type: document.NamedType{
								Name: "bax",
							},
						},
					},
				}.initEmptySlices()),
			},
			{
				it:          "should parse simple VariableDefinitions with NonNullType between",
				input:       "($foo : bar! $baz : bax)",
				expectErr:   BeNil(),
				expectIndex: Equal([]int{0, 1}),
				expectParsedDefinitions: Equal(ParsedDefinitions{
					VariableDefinitions: document.VariableDefinitions{
						{
							Variable: "foo",
							Type: document.NamedType{
								Name:    "bar",
								NonNull: true,
							},
						},
						{
							Variable: "baz",
							Type: document.NamedType{
								Name: "bax",
							},
						},
					},
				}.initEmptySlices()),
			},
			{
				it:          "should parse simple VariableDefinitions with DefaultValue between",
				input:       `($foo : bar! = "me" $baz : bax)`,
				expectErr:   BeNil(),
				expectIndex: Equal([]int{0, 1}),
				expectParsedDefinitions: Equal(ParsedDefinitions{
					VariableDefinitions: document.VariableDefinitions{
						{
							Variable: "foo",
							Type: document.NamedType{
								Name:    "bar",
								NonNull: true,
							},
							DefaultValue: document.Value{
								ValueType:   document.ValueTypeString,
								StringValue: "me",
							},
						},
						{
							Variable: "baz",
							Type: document.NamedType{
								Name: "bax",
							},
						},
					},
				}.initEmptySlices()),
			},
			{
				it:          "should not parse VariableDefinitions when no closing bracket",
				input:       "($foo : bar!",
				expectErr:   Not(BeNil()),
				expectIndex: Equal([]int{0}),
				expectParsedDefinitions: Equal(ParsedDefinitions{
					VariableDefinitions: document.VariableDefinitions{
						{
							Variable: "foo",
							Type: document.NamedType{
								Name:    "bar",
								NonNull: true,
							},
						},
					},
				}.initEmptySlices()),
			},
			{
				it:                      "should not parse optional VariableDefinitions",
				input:                   " ",
				expectErr:               BeNil(),
				expectParsedDefinitions: Equal(ParsedDefinitions{}.initEmptySlices()),
			},
		}

		for _, test := range tests {
			test := test

			g.It(test.it, func() {

				parser := NewParser()
				parser.l.SetInput(test.input)

				var index []int
				err := parser.parseVariableDefinitions(&index)
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
