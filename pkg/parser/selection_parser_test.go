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

func TestSelectionParser(t *testing.T) {

	g := Goblin(t)
	RegisterFailHandler(func(m string, _ ...int) { g.Fail(m) })

	g.Describe("parser.parseSelection", func() {

		tests := []struct {
			it           string
			input        string
			expectErr    types.GomegaMatcher
			expectValues types.GomegaMatcher
		}{
			{
				it:        "should parse a InlineFragment",
				input:     "...on Land",
				expectErr: BeNil(),
				expectValues: Equal(document.InlineFragment{
					TypeCondition: document.NamedType{
						Name: []byte("Land"),
					},
				}),
			},
			{
				it:        "should parse a simple Field",
				input:     "originalName",
				expectErr: BeNil(),
				expectValues: Equal(document.Field{
					Name: []byte("originalName"),
				}),
			},
			{
				it:        "should parse a nested selection",
				input:     `t { kind name ofType { kind name ofType { kind name } } }`,
				expectErr: BeNil(),
				expectValues: Equal(document.Field{
					Name: []byte("t"),
					SelectionSet: []document.Selection{
						document.Field{
							Name: []byte(`kind`),
						},
						document.Field{
							Name: []byte(`name`),
						},
						document.Field{
							Name: []byte(`ofType`),
							SelectionSet: []document.Selection{
								document.Field{
									Name: []byte(`kind`),
								},
								document.Field{
									Name: []byte(`name`),
								},
								document.Field{
									Name: []byte(`ofType`),
									SelectionSet: []document.Selection{
										document.Field{
											Name: []byte(`kind`),
										},
										document.Field{
											Name: []byte(`name`),
										},
									},
								},
							},
						},
					},
				}),
			},
			{
				it:        "should parse a simple Field with an argument",
				input:     "originalName(isSet: true)",
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
				}),
			},
			{
				it:        "should parse a FragmentSpread",
				input:     "...Land",
				expectErr: BeNil(),
				expectValues: Equal(document.FragmentSpread{
					FragmentName: []byte("Land"),
				}),
			},
		}

		for _, test := range tests {
			test := test

			g.It(test.it, func() {

				reader := bytes.NewReader([]byte(test.input))
				parser := NewParser()
				parser.l.SetInput(reader)

				val, err := parser.parseSelection()
				Expect(err).To(test.expectErr)
				Expect(val).To(test.expectValues)
			})
		}
	})
}

var parseSelectionBenchmarkInput = []byte(`t { kind name ofType { kind name ofType { kind name } } }`)

func BenchmarkParseSelection(b *testing.B) {
	reader := bytes.NewReader(parseSelectionBenchmarkInput)
	var err error

	parser := NewParser()

	b.ReportAllocs()

	for i := 0; i < b.N; i++ {

		_, err = reader.Seek(0, io.SeekStart)
		if err != nil {
			b.Fatal(err)
		}

		parser.l.SetInput(reader)
		_, err = parser.parseSelection()
		if err != nil {
			b.Fatal(err)
		}
	}
}
