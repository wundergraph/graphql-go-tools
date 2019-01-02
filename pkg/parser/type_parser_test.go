package parser

import (
	"testing"

	. "github.com/franela/goblin"
	"github.com/jensneuse/graphql-go-tools/pkg/document"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/types"
)

func TestTypeParser(t *testing.T) {

	g := Goblin(t)
	RegisterFailHandler(func(m string, _ ...int) { g.Fail(m) })

	g.Describe("parseType", func() {
		tests := []struct {
			it           string
			input        string
			expectErr    types.GomegaMatcher
			expectValues types.GomegaMatcher
		}{
			{
				it:        "should parse a simple Named Type",
				input:     "String",
				expectErr: BeNil(),
				expectValues: Equal(document.NamedType{
					Name: "String",
				}),
			},
			{
				it:        "should parse a NonNull Named Type",
				input:     "String!",
				expectErr: BeNil(),
				expectValues: Equal(document.NamedType{
					Name:    "String",
					NonNull: true,
				}),
			},
			{
				it:           "should not parse Type on non-IDENT/non-SQUAREBRACKETOPEN keyword",
				input:        ":String",
				expectErr:    Not(BeNil()),
				expectValues: Equal(document.NamedType{}),
			},
			{
				it:        "should parse a simple List Type",
				input:     "[String]",
				expectErr: BeNil(),
				expectValues: Equal(document.ListType{
					Type: document.NamedType{
						Name: "String",
					},
				}),
			},
			{
				it:        "should parse a NonNull List Type",
				input:     "[String]!",
				expectErr: BeNil(),
				expectValues: Equal(document.ListType{
					Type: document.NamedType{
						Name: "String",
					},
					NonNull: true,
				}),
			},
			{
				it:        "should parse a NonNull List Type including a NonNull NamedType",
				input:     "[String!]!",
				expectErr: BeNil(),
				expectValues: Equal(document.ListType{
					Type: document.NamedType{
						Name:    "String",
						NonNull: true,
					},
					NonNull: true,
				}),
			},
			{
				it:        "should parse a mixed deeply nested List type",
				input:     `[[[String!]]!]`,
				expectErr: BeNil(),
				expectValues: Equal(document.ListType{
					Type: document.ListType{
						Type: document.ListType{
							Type: document.NamedType{
								Name:    "String",
								NonNull: true,
							},
						},
						NonNull: true,
					},
				}),
			},
			{
				it:        "should not parse a List Type on missing SQUAREBRACKETCLOSE keyword",
				input:     "[String",
				expectErr: Not(BeNil()),
				expectValues: Equal(document.ListType{
					Type: document.NamedType{
						Name: "String",
					},
				}),
			},
		}

		for _, test := range tests {
			test := test

			g.It(test.it, func() {

				parser := NewParser()
				parser.l.SetInput(test.input)

				val, err := parser.parseType()
				Expect(err).To(test.expectErr)
				Expect(val).To(test.expectValues)
			})
		}
	})
}
