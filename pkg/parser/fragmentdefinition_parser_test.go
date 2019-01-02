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
			it           string
			input        string
			expectErr    types.GomegaMatcher
			expectValues types.GomegaMatcher
		}{
			{
				it: "should parse a simple FragmentDefinition",
				input: `
				MyFragment on SomeType @rename(index: 3){
					name
				}`,
				expectErr: BeNil(),
				expectValues: Equal(document.FragmentDefinition{
					FragmentName: "MyFragment",
					TypeCondition: document.NamedType{
						Name: "SomeType",
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
					SelectionSet: document.SelectionSet{
						document.Field{
							Name: "name",
						},
					},
				}),
			},
			{
				it: "should parse a FragmentDefinition with optional Directives",
				input: `
				MyFragment on SomeType{
					name
				}`,
				expectErr: BeNil(),
				expectValues: Equal(document.FragmentDefinition{
					FragmentName: "MyFragment",
					TypeCondition: document.NamedType{
						Name: "SomeType",
					},
					SelectionSet: document.SelectionSet{
						document.Field{
							Name: "name",
						},
					},
				}),
			},
			{
				it: "should not parse a FragmentDefinition with 'on' missing",
				input: `
				MyFragment SomeType{
					name
				}`,
				expectErr: Not(BeNil()),
				expectValues: Equal(document.FragmentDefinition{
					FragmentName: "MyFragment",
				}),
			},
			{
				it: "should not parse a FragmentDefinition with 'on' missing",
				input: `
				MyFragment un SomeType{
					name
				}`,
				expectErr: Not(BeNil()),
				expectValues: Equal(document.FragmentDefinition{
					FragmentName: "MyFragment",
				}),
			},
		}

		for _, test := range tests {
			test := test

			g.It(test.it, func() {

				parser := NewParser()
				parser.l.SetInput(test.input)

				val, err := parser.parseFragmentDefinition()
				Expect(err).To(test.expectErr)
				Expect(val).To(test.expectValues)
			})
		}
	})
}
