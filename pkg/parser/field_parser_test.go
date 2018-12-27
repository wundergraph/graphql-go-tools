package parser

import (
	"bytes"
	. "github.com/franela/goblin"
	"github.com/jensneuse/graphql-go-tools/pkg/document"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/types"
	"io"
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
					Alias: []byte("preferredName"),
					Name:  []byte("originalName"),
					Arguments: document.Arguments{
						document.Argument{
							Name: []byte("isSet"),
							Value: document.BooleanValue{
								Val: true,
							},
						},
					},
					Directives: document.Directives{
						document.Directive{
							Name: []byte("rename"),
							Arguments: document.Arguments{
								document.Argument{
									Name: []byte("index"),
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
					Name: []byte("originalName"),
					Arguments: document.Arguments{
						document.Argument{
							Name: []byte("isSet"),
							Value: document.BooleanValue{
								Val: true,
							},
						},
					},
					Directives: document.Directives{
						document.Directive{
							Name: []byte("rename"),
							Arguments: document.Arguments{
								document.Argument{
									Name: []byte("index"),
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
					Alias: []byte("preferredName"),
					Name:  []byte("originalName"),
					Directives: document.Directives{
						document.Directive{
							Name: []byte("rename"),
							Arguments: document.Arguments{
								document.Argument{
									Name: []byte("index"),
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
					Alias: []byte("preferredName"),
					Name:  []byte("originalName"),
					Arguments: document.Arguments{
						document.Argument{
							Name: []byte("isSet"),
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
					Name: []byte("originalName"),
					SelectionSet: document.SelectionSet{
						document.Field{
							Name: []byte("unoriginalName"),
							SelectionSet: document.SelectionSet{
								document.Field{
									Name: []byte("worstNamePossible"),
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

				val, err := parser.parseField()
				Expect(err).To(test.expectErr)
				Expect(val).To(test.expectValues)
			})
		}
	})
}

var parseFieldBenchmarkInput = []byte(`t { kind name ofType { kind name ofType { kind name } } }`)

func BenchmarkParseField(b *testing.B) {

	reader := bytes.NewReader(parseFieldBenchmarkInput)
	var err error

	parser := NewParser()

	b.ReportAllocs()

	for i := 0; i < b.N; i++ {

		_, err = reader.Seek(0, io.SeekStart)
		if err != nil {
			b.Fatal(err)
		}

		parser.l.SetInput(reader)
		_, err = parser.parseField()
		if err != nil {
			b.Fatal(err)
		}
	}
}
