package parser

import (
	"bytes"
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
			it           string
			input        string
			expectErr    types.GomegaMatcher
			expectValues types.GomegaMatcher
		}{
			{
				it: "should parse a simple SelectionSet",
				input: `{
					foo
				}`,
				expectErr: BeNil(),
				expectValues: Equal(document.SelectionSet{
					document.Field{
						Name: "foo",
					},
				}),
			},
			{
				it: "should parse SelectionSet with multiple elements in it",
				input: `{
					... on Goland
					...Air
					... on Water
				}`,
				expectErr: BeNil(),
				expectValues: Equal(document.SelectionSet{
					document.InlineFragment{
						TypeCondition: document.NamedType{
							Name: "Goland",
						},
					},
					document.FragmentSpread{
						FragmentName: "Air",
					},
					document.InlineFragment{
						TypeCondition: document.NamedType{
							Name: "Water",
						},
					},
				}),
			},
			{
				it: "should parse SelectionSet with multiple different elements in it",
				input: `{
					... on Goland
					preferredName: originalName(isSet: true)
					... on Water
				}`,
				expectErr: BeNil(),
				expectValues: Equal(document.SelectionSet{
					document.InlineFragment{
						TypeCondition: document.NamedType{
							Name: "Goland",
						},
					},
					document.Field{
						Alias: "preferredName",
						Name:  "originalName",
						Arguments: document.Arguments{
							document.Argument{
								Name: "isSet",
								Value: document.BooleanValue{
									Val: true,
								},
							},
						},
					},
					document.InlineFragment{
						TypeCondition: document.NamedType{
							Name: "Water",
						},
					},
				}),
			},
			{
				it: "should parse SelectionSet with Field containing directives",
				input: `{
					... on Goland
					preferredName: originalName(isSet: true) @rename(index: 3)
					... on Water
				}`,
				expectErr: BeNil(),
				expectValues: Equal(document.SelectionSet{
					document.InlineFragment{
						TypeCondition: document.NamedType{
							Name: "Goland",
						},
					},
					document.Field{
						Alias: "preferredName",
						Name:  "originalName",
						Arguments: document.Arguments{
							document.Argument{
								Name: "isSet",
								Value: document.BooleanValue{
									Val: true,
								},
							},
						},
						Directives: document.Directives{
							document.Directive{
								Name: "rename",
								Arguments: document.Arguments{
									document.Argument{
										Name: "index",
										Value: document.IntValue{
											Val: 3,
										},
									},
								},
							},
						},
					},
					document.InlineFragment{
						TypeCondition: document.NamedType{
							Name: "Water",
						},
					},
				}),
			},
			{
				it: "should parse SelectionSet with FragmentSpread containing Directive",
				input: `{
					... on Goland
					...firstFragment @rename(index: 3)
					... on Water
				}`,
				expectErr: BeNil(),
				expectValues: Equal(document.SelectionSet{
					document.InlineFragment{
						TypeCondition: document.NamedType{
							Name: "Goland",
						},
					},
					document.FragmentSpread{
						FragmentName: "firstFragment",
						Directives: document.Directives{
							document.Directive{
								Name: "rename",
								Arguments: document.Arguments{
									document.Argument{
										Name: "index",
										Value: document.IntValue{
											Val: 3,
										},
									},
								},
							},
						},
					},
					document.InlineFragment{
						TypeCondition: document.NamedType{
							Name: "Water",
						},
					},
				}),
			},
		}

		for _, test := range tests {
			test := test

			g.It(test.it, func() {

				reader := bytes.NewReader([]byte(test.input))
				parser := NewParser()
				parser.l.SetInput(reader)

				val, err := parser.parseSelectionSet()
				Expect(err).To(test.expectErr)
				Expect(val).To(test.expectValues)
			})
		}
	})
}
