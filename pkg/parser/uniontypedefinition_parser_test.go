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
					Name: []byte("SearchResult"),
					UnionMemberTypes: document.UnionMemberTypes{
						[]byte("Photo"),
						[]byte("Person"),
					},
				}),
			},
			{
				it:        "should parse multiple UnionMemberTypes in UnionTypeDefinition",
				input:     ` SearchResult = Photo | Person | Car | Planet`,
				expectErr: BeNil(),
				expectValues: Equal(document.UnionTypeDefinition{
					Name: []byte("SearchResult"),
					UnionMemberTypes: document.UnionMemberTypes{
						[]byte("Photo"),
						[]byte("Person"),
						[]byte("Car"),
						[]byte("Planet"),
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
					Name: []byte("SearchResult"),
					UnionMemberTypes: document.UnionMemberTypes{
						[]byte("Photo"),
						[]byte("Person"),
						[]byte("Car"),
						[]byte("Planet"),
					},
				}),
			},
			{
				it:        "should parse a UnionTypeDefinition with Directives",
				input:     ` SearchResult @fromTop(to: "bottom") @fromBottom(to: "top") = Photo | Person`,
				expectErr: BeNil(),
				expectValues: Equal(document.UnionTypeDefinition{
					Name: []byte("SearchResult"),
					Directives: document.Directives{
						document.Directive{
							Name: []byte("fromTop"),
							Arguments: document.Arguments{
								document.Argument{
									Name: []byte("to"),
									Value: document.StringValue{
										Val: []byte("bottom"),
									},
								},
							},
						},
						document.Directive{
							Name: []byte("fromBottom"),
							Arguments: document.Arguments{
								document.Argument{
									Name: []byte("to"),
									Value: document.StringValue{
										Val: []byte("top"),
									},
								},
							},
						},
					},
					UnionMemberTypes: document.UnionMemberTypes{
						[]byte("Photo"),
						[]byte("Person"),
					},
				}),
			},
			{
				it:        "should parse a UnionTypeDefinition with optional UnionMemberTypes",
				input:     ` SearchResult`,
				expectErr: BeNil(),
				expectValues: Equal(document.UnionTypeDefinition{
					Name: []byte("SearchResult"),
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
