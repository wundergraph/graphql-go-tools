package parser

import (
	. "github.com/franela/goblin"
	"github.com/jensneuse/graphql-go-tools/pkg/document"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/types"
	"testing"
)

func TestFieldParser(t *testing.T) {

	g := Goblin(t)
	RegisterFailHandler(func(m string, _ ...int) { g.Fail(m) })

	g.Describe("parser.parseField", func() {

		tests := []struct {
			it           string
			input        string
			expectErr    types.GomegaMatcher
			expectValues types.GomegaMatcher
		}{
			{
				it:        "should parse a simple Field",
				input:     "preferredName: originalName(isSet: true) @rename(index: 3)",
				expectErr: BeNil(),
				expectValues: Equal(document.Field{
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
				}),
			},
			{
				it:        "should parse Field with optional Alias",
				input:     "originalName(isSet: true) @rename(index: 3)",
				expectErr: BeNil(),
				expectValues: Equal(document.Field{
					Name: "originalName",
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
				}),
			},
			{
				it:        "should parse Field with optional Arguments",
				input:     "preferredName: originalName @rename(index: 3)",
				expectErr: BeNil(),
				expectValues: Equal(document.Field{
					Alias: "preferredName",
					Name:  "originalName",
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
				}),
			},
			{
				it:        "should parse Field with optional Directives",
				input:     "preferredName: originalName(isSet: true)",
				expectErr: BeNil(),
				expectValues: Equal(document.Field{
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
				}),
			},
			{
				it: "should parse Field with nested SelectionSets",
				input: `
				originalName {
					unoriginalName {
						worstNamePossible
					}
				}
				`,
				expectErr: BeNil(),
				expectValues: Equal(document.Field{
					Name: "originalName",
					SelectionSet: document.SelectionSet{
						document.Field{
							Name: "unoriginalName",
							SelectionSet: document.SelectionSet{
								document.Field{
									Name: "worstNamePossible",
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

				val, err := parser.parseField()
				Expect(err).To(test.expectErr)
				Expect(val).To(test.expectValues)
			})
		}
	})
}

var parseFieldBenchmarkInput = `t { kind name ofType { kind name ofType { kind name } } }`

func BenchmarkParseField(b *testing.B) {

	var err error

	parser := NewParser()

	b.ReportAllocs()

	for i := 0; i < b.N; i++ {

		parser.l.SetInput(parseFieldBenchmarkInput)
		_, err = parser.parseField()
		if err != nil {
			b.Fatal(err)
		}
	}
}
