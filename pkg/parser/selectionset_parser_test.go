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

func TestSelectionSetParser(t *testing.T) {

	g := Goblin(t)
	RegisterFailHandler(func(m string, _ ...int) { g.Fail(m) })

	g.Describe("parser.parseSelectionSet", func() {

		tests := []struct {
			it           string
			input        string
			expectErr    types.GomegaMatcher
			expectValues types.GomegaMatcher
		}{
			{
				it: "should parse a simple SelectionSet",
				input: `{
					foo
				}`,
				expectErr: BeNil(),
				expectValues: Equal(document.SelectionSet{
					document.Field{
						Name: []byte("foo"),
					},
				}),
			},
			{
				it: "should parse SelectionSet with multiple elements in it",
				input: `{
					... on Goland
					...Air
					... on Water
				}`,
				expectErr: BeNil(),
				expectValues: Equal(document.SelectionSet{
					document.InlineFragment{
						TypeCondition: document.NamedType{
							Name: []byte("Goland"),
						},
					},
					document.FragmentSpread{
						FragmentName: []byte("Air"),
					},
					document.InlineFragment{
						TypeCondition: document.NamedType{
							Name: []byte("Water"),
						},
					},
				}),
			},
			{
				it: "should parse SelectionSet with multiple different elements in it",
				input: `{
					... on Goland
					preferredName: originalName(isSet: true)
					... on Water
				}`,
				expectErr: BeNil(),
				expectValues: Equal(document.SelectionSet{
					document.InlineFragment{
						TypeCondition: document.NamedType{
							Name: []byte("Goland"),
						},
					},
					document.Field{
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
					},
					document.InlineFragment{
						TypeCondition: document.NamedType{
							Name: []byte("Water"),
						},
					},
				}),
			},
			{
				it: "should parse SelectionSet with Field containing directives",
				input: `{
					... on Goland
					preferredName: originalName(isSet: true) @rename(index: 3)
					... on Water
				}`,
				expectErr: BeNil(),
				expectValues: Equal(document.SelectionSet{
					document.InlineFragment{
						TypeCondition: document.NamedType{
							Name: []byte("Goland"),
						},
					},
					document.Field{
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
					},
					document.InlineFragment{
						TypeCondition: document.NamedType{
							Name: []byte("Water"),
						},
					},
				}),
			},
			{
				it: "should parse SelectionSet with FragmentSpread containing Directive",
				input: `{
					... on Goland
					...firstFragment @rename(index: 3)
					... on Water
				}`,
				expectErr: BeNil(),
				expectValues: Equal(document.SelectionSet{
					document.InlineFragment{
						TypeCondition: document.NamedType{
							Name: []byte("Goland"),
						},
					},
					document.FragmentSpread{
						FragmentName: []byte("firstFragment"),
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
					},
					document.InlineFragment{
						TypeCondition: document.NamedType{
							Name: []byte("Water"),
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

				val, err := parser.parseSelectionSet()
				Expect(err).To(test.expectErr)
				Expect(val).To(test.expectValues)
			})
		}
	})
}

var selectionSetBenchmarkInput = []byte(`{
  kind
  name
  ofType {
    kind
    name
    ofType {
      kind
      name
      ofType {
        kind
        name
        ofType {
          kind
          name
          ofType {
            kind
            name
            ofType {
              kind
              name
              ofType {
                kind
                name
              }
            }
          }
        }
      }
    }
  }
}`)

func BenchmarkParseSelectionSet(b *testing.B) {

	reader := bytes.NewReader(selectionSetBenchmarkInput)
	var err error

	parser := NewParser()

	parse := func() {

		_, err = reader.Seek(0, io.SeekStart)
		if err != nil {
			b.Fatal(err)
		}

		parser.l.SetInput(reader)
		_, err = parser.parseSelectionSet()
		if err != nil {
			b.Fatal(err)
		}
	}

	for i := 0; i < 10; i++ {
		parse()
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		parse()
	}
}
