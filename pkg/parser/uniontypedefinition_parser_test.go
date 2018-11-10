package parser

import (
	"bytes"
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
			it           string
			input        string
			expectErr    types.GomegaMatcher
			expectValues types.GomegaMatcher
		}{

			{
				it:        "should parse simple UnionTypeDefinition",
				input:     ` SearchResult = Photo | Person`,
				expectErr: BeNil(),
				expectValues: Equal(document.UnionTypeDefinition{
					Name: "SearchResult",
					UnionMemberTypes: document.UnionMemberTypes{
						"Photo", "Person",
					},
				}),
			},
			{
				it:        "should parse multiple UnionMemberTypes in UnionTypeDefinition",
				input:     ` SearchResult = Photo | Person | Car | Planet`,
				expectErr: BeNil(),
				expectValues: Equal(document.UnionTypeDefinition{
					Name: "SearchResult",
					UnionMemberTypes: document.UnionMemberTypes{
						"Photo", "Person", "Car", "Planet",
					},
				}),
			},
			{
				it: "should parse multiple UnionMemberTypes spread over multiple lines in UnionTypeDefinition",
				input: ` SearchResult = Photo 
| Person 
| Car 
| Planet`,
				expectErr: BeNil(),
				expectValues: Equal(document.UnionTypeDefinition{
					Name: "SearchResult",
					UnionMemberTypes: document.UnionMemberTypes{
						"Photo", "Person", "Car", "Planet",
					},
				}),
			},
			{
				it:        "should parse a UnionTypeDefinition with Directives",
				input:     ` SearchResult @fromTop(to: "bottom") @fromBottom(to: "top") = Photo | Person`,
				expectErr: BeNil(),
				expectValues: Equal(document.UnionTypeDefinition{
					Name: "SearchResult",
					Directives: document.Directives{
						document.Directive{
							Name: "fromTop",
							Arguments: document.Arguments{
								document.Argument{
									Name: "to",
									Value: document.StringValue{
										Val: "bottom",
									},
								},
							},
						},
						document.Directive{
							Name: "fromBottom",
							Arguments: document.Arguments{
								document.Argument{
									Name: "to",
									Value: document.StringValue{
										Val: "top",
									},
								},
							},
						},
					},
					UnionMemberTypes: document.UnionMemberTypes{
						"Photo", "Person",
					},
				}),
			},
			{
				it:        "should parse a UnionTypeDefinition with optional UnionMemberTypes",
				input:     ` SearchResult`,
				expectErr: BeNil(),
				expectValues: Equal(document.UnionTypeDefinition{
					Name: "SearchResult",
				}),
			},
		}

		for _, test := range tests {
			test := test

			g.It(test.it, func() {

				reader := bytes.NewReader([]byte(test.input))
				parser := NewParser()
				parser.l.SetInput(reader)

				val, err := parser.parseUnionTypeDefinition()
				Expect(err).To(test.expectErr)
				Expect(val).To(test.expectValues)
			})
		}

	})
}
