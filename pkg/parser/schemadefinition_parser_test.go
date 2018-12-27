package parser

import (
	"bytes"
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
					Query:        []byte("Query"),
					Mutation:     []byte("Mutation"),
					Subscription: []byte("Subscription"),
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
					Query:        []byte("Query"),
					Mutation:     []byte("Mutation"),
					Subscription: []byte("Subscription"),
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
					Query:        []byte("Query"),
					Mutation:     []byte("Mutation"),
					Subscription: []byte("Subscription"),
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
					Query:        []byte("Query"),
					Mutation:     []byte("Mutation"),
					Subscription: []byte("Subscription"),
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
				}),
			},
		}

		for _, test := range tests {
			test := test

			g.It(test.it, func() {

				reader := bytes.NewReader([]byte(test.input))
				parser := NewParser()
				parser.l.SetInput(reader)

				val, err := parser.parseSchemaDefinition()
				Expect(err).To(test.expectErr)
				Expect(val).To(test.expectValues)
			})
		}
	})
}
