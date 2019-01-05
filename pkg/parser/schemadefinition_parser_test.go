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
			it                      string
			input                   string
			expectErr               types.GomegaMatcher
			expectSchemaDefinition  types.GomegaMatcher
			expectParsedDefinitions types.GomegaMatcher
		}{
			{
				it: "should parse simple SchemaDefinition",
				input: ` {
	query: Query
	mutation: Mutation
	subscription: Subscription
}`,
				expectErr: BeNil(),
				expectSchemaDefinition: Equal(document.SchemaDefinition{
					Query:        "Query",
					Mutation:     "Mutation",
					Subscription: "Subscription",
					Directives:   []int{},
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
				expectSchemaDefinition: Equal(document.SchemaDefinition{
					Query:        "Query",
					Mutation:     "Mutation",
					Subscription: "Subscription",
					Directives:   []int{},
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
				expectSchemaDefinition: Equal(document.SchemaDefinition{
					Query:        "Query",
					Mutation:     "Mutation",
					Subscription: "Subscription",
					Directives:   []int{},
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
						{
							Name:      "fromTop",
							Arguments: []int{0},
						},
						{
							Name:      "fromBottom",
							Arguments: []int{1},
						},
					},
				}.initEmptySlices()),
				expectSchemaDefinition: Equal(document.SchemaDefinition{
					Query:        "Query",
					Mutation:     "Mutation",
					Subscription: "Subscription",
					Directives:   []int{0, 1},
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
				Expect(val).To(test.expectSchemaDefinition)
				if test.expectParsedDefinitions != nil {
					Expect(parser.ParsedDefinitions).To(test.expectParsedDefinitions)
				}
			})
		}
	})
}
