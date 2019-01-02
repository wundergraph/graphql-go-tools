package parser

import (
	. "github.com/franela/goblin"
	"github.com/jensneuse/graphql-go-tools/pkg/document"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/types"
	"testing"
)

func TestObjectValueParser(t *testing.T) {

	g := Goblin(t)
	RegisterFailHandler(func(m string, _ ...int) { g.Fail(m) })

	g.Describe("parser.parsePeekedObjectValue", func() {

		tests := []struct {
			it        string
			input     string
			expectErr types.GomegaMatcher
			expectVal types.GomegaMatcher
		}{
			{
				it:        "should parse simple object value",
				input:     `{ foo: "bar" }`,
				expectErr: BeNil(),
				expectVal: Equal(document.ObjectValue{
					Val: []document.ObjectField{
						{
							Name: "foo",
							Value: document.StringValue{
								Val: "bar",
							},
						},
					},
				}),
			},
			{
				it:        "should parse multiple values",
				input:     `{ foo: "bar" baz: "bat", bas: "bal" anEnum: NUM, smallEnum: numnum }`,
				expectErr: BeNil(),
				expectVal: Equal(document.ObjectValue{
					Val: []document.ObjectField{
						{
							Name: "foo",
							Value: document.StringValue{
								Val: "bar",
							},
						},
						{
							Name: "baz",
							Value: document.StringValue{
								Val: "bat",
							},
						},
						{
							Name: "bas",
							Value: document.StringValue{
								Val: "bal",
							},
						},
						{
							Name: "anEnum",
							Value: document.EnumValue{
								Name: "NUM",
							},
						},
						{
							Name: "smallEnum",
							Value: document.EnumValue{
								Name: "numnum",
							},
						},
					},
				}),
			},
			{
				it:        "should parse nested object value",
				input:     `{ foo: { bar: "baz" } }`,
				expectErr: BeNil(),
				expectVal: Equal(document.ObjectValue{
					Val: []document.ObjectField{
						{
							Name: "foo",
							Value: document.ObjectValue{
								Val: []document.ObjectField{
									{
										Name: "bar",
										Value: document.StringValue{
											Val: "baz",
										},
									},
								},
							},
						},
					},
				}),
			},
			{
				it: "should parse nested object value across multiple lines",
				input: `{foo	:
	{
		bar: "baz"
	}
}`,
				expectErr: BeNil(),
				expectVal: Equal(document.ObjectValue{
					Val: []document.ObjectField{
						{
							Name: "foo",
							Value: document.ObjectValue{
								Val: []document.ObjectField{
									{
										Name: "bar",
										Value: document.StringValue{
											Val: "baz",
										},
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

				val, err := parser.parsePeekedObjectValue()
				Expect(err).To(test.expectErr)
				Expect(val).To(test.expectVal)
			})
		}
	})
}
