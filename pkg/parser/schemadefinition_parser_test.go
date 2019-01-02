package parser

import (
	. "github.com/franela/goblin"
	"github.com/jensneuse/graphql-go-tools/pkg/document"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/types"
	"testing"
)

func TestParseSchemaDefinition(t *testing.T) {

	g := Goblin(t)
	RegisterFailHandler(func(m string, _ ...int) { g.Fail(m) })

	g.Describe("parseSchemaDefinition", func() {
		tests := []struct {
			it           string
			input        string
			expectErr    types.GomegaMatcher
			expectValues types.GomegaMatcher
		}{
			{
				it: "should parse simple SchemaDefinition",
				input: ` {
	query: Query
	mutation: Mutation
	subscription: Subscription
}`,
				expectErr: BeNil(),
				expectValues: Equal(document.SchemaDefinition{
					Query:        "Query",
					Mutation:     "Mutation",
					Subscription: "Subscription",
				}),
			},
			{
				it: "should parse messy SchemaDefinition",
				input: ` {

	query : Query

	mutation : Mutation

	subscription : Subscription

}`,
				expectErr: BeNil(),
				expectValues: Equal(document.SchemaDefinition{
					Query:        "Query",
					Mutation:     "Mutation",
					Subscription: "Subscription",
				}),
			},
			{
				it: "should not parse messy SchemaDefinition with redeclared query",
				input: ` {

	query : Query

	mutation : Mutation

	subscription : Subscription

	query: Query2

}`,
				expectErr: Not(BeNil()),
				expectValues: Equal(document.SchemaDefinition{
					Query:        "Query",
					Mutation:     "Mutation",
					Subscription: "Subscription",
				}),
			},
			{
				it: "should parse simple SchemaDefinition",
				input: ` @fromTop(to: "bottom") @fromBottom(to: "top") {
	query: Query
	mutation: Mutation
	subscription: Subscription
}`,
				expectErr: BeNil(),
				expectValues: Equal(document.SchemaDefinition{
					Query:        "Query",
					Mutation:     "Mutation",
					Subscription: "Subscription",
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
				}),
			},
		}

		for _, test := range tests {
			test := test

			g.It(test.it, func() {

				parser := NewParser()
				parser.l.SetInput(test.input)

				val, err := parser.parseSchemaDefinition()
				Expect(err).To(test.expectErr)
				Expect(val).To(test.expectValues)
			})
		}
	})
}
